package tunnel

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"

	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	pkglog "github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/metrics"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
	"github.com/bzdvdn/kvn-ws/src/internal/ratelimit"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/session"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// @sk-task fix-critical-leaks#T5.1: sync.Pool for 4KB proxy buffers (AC-013)
var proxyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 4096)
		return b
	},
}

// @sk-task arch-refactoring#T3.5: magic numbers → Session fields (AC-006)
// wsTunnelTimeout and defaultProxyConcurrency replaced by Session fields set via config.

type tunReadResult struct {
	n   int
	err error
	buf []byte
}

// streamWriter provides per-stream ordered async writes to a target TCP
// connection. wsToTun enqueues data into a buffered channel; a single
// goroutine drains the channel sequentially so order is preserved and
// wsToTun never blocks on a slow target.
type streamWriter struct {
	conn net.Conn
	ch   chan []byte
}

// OutgoingInterceptor is called before writing a data frame to the TUN device.
// If it returns true, the frame is considered handled and TUN write is skipped.
type OutgoingInterceptor func(payload []byte) (handled bool, err error)

// Session encapsulates bidirectional forwarding between a transport
// stream (WebSocket or QUIC) and a TUN device.
type Session struct {
	tunDev           tun.TunDevice
	stream           StreamConn
	sm               *session.SessionManager
	sessionID        string
	tokenName        string
	prl              *ratelimit.SessionPacketLimiter
	bwMgr            *session.TokenBandwidthManager
	collectors       *metrics.Collectors
	logger           *zap.Logger
	cipher           *crypto.SessionCipher
	proxyStreams     *proxy.SessionStreams
	streamWriters    sync.Map // uint32 → *streamWriter, per-stream ordered writes
	proxySem         chan struct{}
	tunRouter        *routing.TunRouter
	tunReaderCh      chan tunReadResult
	demux            *TunDemux
	tunnelTimeout    time.Duration
	proxyConcurrency int
	clientIP         net.IP
	clientIP6        net.IP
	dnsUpstreams     []string

	outgoingInterceptor OutgoingInterceptor
}

// @sk-task dns-upstreams-list#T2.2: add dnsUpstreams param (AC-006)
// @sk-task arch-refactoring#T3.5: add tunnelTimeout and proxyConcurrency params (AC-006)
func NewSession(
	tunDev tun.TunDevice,
	stream StreamConn,
	sm *session.SessionManager,
	sessionID string,
	tokenName string,
	prl *ratelimit.SessionPacketLimiter,
	bwMgr *session.TokenBandwidthManager,
	collectors *metrics.Collectors,
	logger *zap.Logger,
	cipher *crypto.SessionCipher,
	proxyStreams *proxy.SessionStreams,
	tunnelTimeout time.Duration,
	proxyConcurrency int,
	clientIP net.IP,
	clientIP6 net.IP,
	dnsUpstreams []string,
) *Session {
	if tunnelTimeout <= 0 {
		tunnelTimeout = 30 * time.Second
	}
	if proxyConcurrency <= 0 {
		proxyConcurrency = 1000
	}
	if len(dnsUpstreams) == 0 {
		dnsUpstreams = []string{"1.1.1.1:53", "8.8.8.8:53"}
	}
	return &Session{
		tunDev:           tunDev,
		stream:           stream,
		sm:               sm,
		sessionID:        sessionID,
		tokenName:        tokenName,
		prl:              prl,
		bwMgr:            bwMgr,
		collectors:       collectors,
		logger:           logger,
		cipher:           cipher,
		proxyStreams:     proxyStreams,
		proxySem:         make(chan struct{}, proxyConcurrency),
		tunnelTimeout:    tunnelTimeout,
		proxyConcurrency: proxyConcurrency,
		clientIP:         clientIP,
		clientIP6:        clientIP6,
		dnsUpstreams:     dnsUpstreams,
	}
}

func (s *Session) SetTunRouter(tr *routing.TunRouter) {
	s.tunRouter = tr
}

func (s *Session) SetDemux(d *TunDemux) {
	s.demux = d
}

func (s *Session) SetOutgoingInterceptor(fn OutgoingInterceptor) {
	s.outgoingInterceptor = fn
}

// @sk-task fix-critical-leaks#T3.1: TUN reader — permanent goroutine (AC-001)
func (s *Session) startTunReader(ctx context.Context) {
	s.tunReaderCh = make(chan tunReadResult, 64)
	if s.demux != nil {
		s.demux.Register(s.clientIP, s.clientIP6, s.tunReaderCh)
		go func() {
			<-ctx.Done()
			s.demux.Unregister(s.clientIP, s.clientIP6)
		}()
		return
	}
	go func() {
		for {
			buf := make([]byte, 1500)
			n, err := s.tunDev.Read(buf)
			select {
			case s.tunReaderCh <- tunReadResult{n, err, buf}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
}

// Run spawns the two forwarding goroutines (WS→TUN and TUN→WS) and
// blocks until one fails or ctx is cancelled.
// @sk-task relay-terminator#T8.5: recover from panics in Run() and errgroup goroutines
func (s *Session) Run(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("session panic: %v", r)
			s.logger.Error("session recovered from panic", zap.Any("panic", r))
		}
	}()
	s.startTunReader(ctx)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("wsToTun panic: %v", r)
				s.logger.Error("wsToTun recovered from panic", zap.Any("panic", r))
			}
		}()
		return s.wsToTun(ctx)
	})
	eg.Go(func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("tunToWS panic: %v", r)
				s.logger.Error("tunToWS recovered from panic", zap.Any("panic", r))
			}
		}()
		return s.tunToWS(ctx)
	})
	return eg.Wait()
}

// @sk-task fix-ping-drops#T2.1: treat read timeout as non-fatal, continue instead of aborting session
// @sk-task relay-terminator#T8.4: timeout hardening — net.Error.Timeout() + max 10 consecutive (RQ-016)
// @sk-task arch-refactoring#T3.3: decomposed wsToTun with handler methods (AC-005)
func (s *Session) wsToTun(ctx context.Context) error {
	var lastRateLimitLog time.Time
	var consecutiveTimeouts int
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if s.sm != nil {
			s.sm.UpdateActivity(s.sessionID)
		}
		if s.prl != nil && !s.prl.Allow(s.sessionID) {
			if time.Since(lastRateLimitLog) > time.Second {
				lastRateLimitLog = time.Now()
				pkglog.Audit(s.logger, zapcore.WarnLevel, "packet rate limited",
					zap.String("session_id", s.sessionID),
					zap.String("reason", "packet rate exceeded"),
				)
			}
			continue
		}
		if err := s.stream.SetReadDeadline(time.Now().Add(s.tunnelTimeout)); err != nil {
			return err
		}
		data, err := s.stream.ReadMessage()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				consecutiveTimeouts++
				if consecutiveTimeouts >= 10 {
					s.logger.Warn("too many consecutive timeouts, ending session",
						zap.Int("count", consecutiveTimeouts), zap.Error(err))
					return err
				}
				s.logger.Debug("read timeout, continuing", zap.Error(err))
				continue
			}
			return err
		}
		consecutiveTimeouts = 0
		var f framing.Frame
		if err := f.Decode(data); err != nil {
			return err
		}
		if f.Type == framing.FrameTypeData && s.cipher != nil {
			decrypted, err := s.cipher.Decrypt(f.Payload)
			if err != nil {
				s.logger.Warn("decrypt failed, dropping packet", zap.Error(err))
				f.Release()
				continue
			}
			f.Release()
			f.Payload = decrypted
		}
		switch f.Type {
		case framing.FrameTypeData:
			if err := s.handleDataFrame(&f); err != nil {
				return err
			}
		case framing.FrameTypeClose:
			s.handleCloseFrame()
			return nil
		case framing.FrameTypeProxy:
			s.handleProxyFrame(ctx, &f)
		case framing.FrameTypeDNS:
			go func() {
				payload := make([]byte, len(f.Payload))
				copy(payload, f.Payload)
				f.Release()
				s.handleDNSFrame(ctx, payload)
			}()
		default:
			f.Release()
		}
	}
}

// @sk-task arch-refactoring#T3.3: extracted data frame handler (AC-005)
func (s *Session) handleDataFrame(f *framing.Frame) error {
	defer f.Release()

	if s.outgoingInterceptor != nil {
		handled, err := s.outgoingInterceptor(f.Payload)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
	}

	n, err := s.tunDev.Write(f.Payload)
	if err != nil {
		return err
	}
	if s.collectors != nil {
		s.collectors.AddThroughput("rx", float64(n))
	}
	return nil
}

// @sk-task arch-refactoring#T3.3: extracted close frame handler (AC-005)
func (s *Session) handleCloseFrame() {
	s.logger.Debug("session close frame received", zap.String("session_id", s.sessionID))
}

// @sk-task arch-refactoring#T3.3: extracted proxy frame handler (AC-005)
func (s *Session) handleProxyFrame(ctx context.Context, f *framing.Frame) {
	defer f.Release()
	if s.proxyStreams == nil {
		return
	}
	payload := f.Payload
	if len(payload) < 6 {
		ack := framing.Frame{
			Type:  framing.FrameTypeProxy,
			Flags: framing.FrameFlagNone,
		}
		data, err := ack.Encode()
		if err == nil {
			_ = s.stream.WriteMessage(data)
			framing.ReturnBuffer(data)
		}
		return
	}
	streamID := binary.BigEndian.Uint32(payload[0:4])
	dstLen := binary.BigEndian.Uint16(payload[4:6])
	if int(6+dstLen) > len(payload) {
		return
	}
	dst := string(payload[6 : 6+dstLen])
	data := payload[6+dstLen:]

	if _, ok := s.proxyStreams.Load(streamID); ok {
		// Async ordered write via per-stream goroutine.
		sw, ok := s.streamWriters.Load(streamID)
		if !ok {
			return
		}
		bw := sw.(*streamWriter)
		tmp := make([]byte, len(data))
		copy(tmp, data)
		bw.ch <- tmp
		return
	}

	tcpConn, err := net.DialTimeout("tcp", dst, 10*time.Second)
	if err != nil {
		s.logger.Warn("proxy dial failed", zap.String("dst", dst), zap.String("ip", dst), zap.Error(err))
		closeFrame := framing.Frame{
			Type:    framing.FrameTypeProxy,
			Payload: make([]byte, 6),
		}
		binary.BigEndian.PutUint32(closeFrame.Payload[0:4], streamID)
		binary.BigEndian.PutUint16(closeFrame.Payload[4:6], 0)
		if encoded, encErr := closeFrame.Encode(); encErr == nil {
			_ = s.stream.SetWriteDeadline(time.Now().Add(s.tunnelTimeout))
			if err := s.stream.WriteMessage(encoded); err != nil {
				s.logger.Warn("write close frame failed", zap.Error(err))
			}
			framing.ReturnBuffer(encoded)
		}
		return
	}
	s.proxyStreams.Store(streamID, tcpConn)
	s.logger.Info("proxy tunnel", zap.String("dst", dst), zap.String("ip", dst))

	// Start per-stream ordered writer goroutine.
	sw := &streamWriter{
		conn: tcpConn,
		ch:   make(chan []byte, 64),
	}
	s.streamWriters.Store(streamID, sw)
	go func() {
		for buf := range sw.ch {
			_ = sw.conn.SetWriteDeadline(time.Now().Add(s.tunnelTimeout))
			if _, err := sw.conn.Write(buf); err != nil {
				break
			}
		}
	}()

	if len(data) > 0 {
		tmp := make([]byte, len(data))
		copy(tmp, data)
		sw.ch <- tmp
	}

	select {
	case s.proxySem <- struct{}{}:
	default:
		s.logger.Warn("proxy concurrency limit reached, dropping stream", zap.Uint32("stream_id", streamID))
		_ = tcpConn.Close()
		s.proxyStreams.Delete(streamID)
		s.streamWriters.Delete(streamID)
		return
	}

	go s.forwardProxyStream(streamID, tcpConn, dst, ctx)
}

// @sk-task dns-upstreams-list#T2.2: use s.dnsUpstreams instead of hardcoded addr (AC-006)
// @sk-task transparent-proxy#T2.3: server-side DNS forwarder (FrameTypeDNS)
// must be called in a goroutine to avoid blocking wsToTun read-loop
func (s *Session) handleDNSFrame(ctx context.Context, payload []byte) {
	if len(payload) < 5 {
		return
	}
	streamID := binary.BigEndian.Uint32(payload[0:4])
	query := payload[4:]

	upstreams := s.dnsUpstreams
	resp, err := s.forwardDNS(ctx, query, upstreams)
	if err != nil {
		s.logger.Debug("dns forward error", zap.Error(err))
		return
	}

	respPayload := make([]byte, 4+len(resp))
	binary.BigEndian.PutUint32(respPayload[0:4], streamID)
	copy(respPayload[4:], resp)

	frame := framing.Frame{
		Type:    framing.FrameTypeDNS,
		Payload: respPayload,
	}
	encoded, encErr := frame.Encode()
	if encErr != nil {
		return
	}
	defer framing.ReturnBuffer(encoded)
	_ = s.stream.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = s.stream.WriteMessage(encoded)
}

// @sk-task dns-upstreams-list#T2.2: forwardDNS with upstream list + fallback (AC-006)
func (s *Session) forwardDNS(ctx context.Context, query []byte, upstreams []string) ([]byte, error) {
	if len(upstreams) == 0 {
		upstreams = []string{"1.1.1.1:53", "8.8.8.8:53"}
	}
	var lastErr error
	for _, addr := range upstreams {
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			lastErr = err
			continue
		}
		conn, err := net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			lastErr = err
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write(query); err != nil {
			conn.Close() // #nosec G104
			lastErr = err
			continue
		}
		resp := make([]byte, 1500)
		n, err := conn.Read(resp)
		conn.Close() // #nosec G104
		if err != nil {
			lastErr = err
			continue
		}
		return resp[:n], nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("dns: no upstream available")
}

// @sk-task arch-refactoring#T3.3: extracted proxy stream forwarding (AC-005)
// @sk-task fix-proxy-drops#T1.1: skip close frame write if session ctx is cancelled (AC-001)
func (s *Session) forwardProxyStream(sid uint32, tcp net.Conn, dst string, parentCtx context.Context) {
	defer func() {
		<-s.proxySem
		if sw, ok := s.streamWriters.Load(sid); ok {
			s.streamWriters.Delete(sid)
			close(sw.(*streamWriter).ch)
		}
		_ = tcp.Close()
		s.proxyStreams.Delete(sid)
		if parentCtx.Err() != nil {
			return
		}
		closeFrame := framing.Frame{
			Type:    framing.FrameTypeProxy,
			Payload: make([]byte, 6),
		}
		binary.BigEndian.PutUint32(closeFrame.Payload[0:4], sid)
		binary.BigEndian.PutUint16(closeFrame.Payload[4:6], 0)
		if encoded, encErr := closeFrame.Encode(); encErr == nil {
			_ = s.stream.SetWriteDeadline(time.Now().Add(s.tunnelTimeout))
			if err := s.stream.WriteMessage(encoded); err != nil {
				s.logger.Debug("write close frame failed (stream closed)", zap.Error(err))
			}
			framing.ReturnBuffer(encoded)
		}
	}()
	buf, ok := proxyBufPool.Get().([]byte)
	if !ok {
		return
	}
	defer proxyBufPool.Put(buf) //nolint:staticcheck // SA6002: []byte is acceptable in Go 1.23+
	for {
		if err := tcp.SetReadDeadline(time.Now().Add(s.tunnelTimeout)); err != nil {
			return
		}
		select {
		case <-parentCtx.Done():
			return
		default:
		}
		n, err := tcp.Read(buf)
		if err != nil {
			return
		}
		if len(dst) > math.MaxUint16 {
			return
		}
		payload := framing.GetBuffer(4 + 2 + len(dst) + n)
		binary.BigEndian.PutUint32(payload[0:4], sid)
		binary.BigEndian.PutUint16(payload[4:6], uint16(len(dst))) // #nosec G115 — checked at line 382
		copy(payload[6:], dst)
		copy(payload[6+len(dst):], buf[:n])
		frame := framing.Frame{
			Type:    framing.FrameTypeProxy,
			Flags:   framing.FrameFlagNone,
			Payload: payload,
		}
		encoded, err := frame.Encode()
		frame.Release()
		if err != nil {
			return
		}
		_ = s.stream.SetWriteDeadline(time.Now().Add(s.tunnelTimeout))
		if err := s.stream.WriteMessage(encoded); err != nil {
			framing.ReturnBuffer(encoded)
			return
		}
		framing.ReturnBuffer(encoded)
	}
}

// @sk-task fix-critical-leaks#T3.1: TUN reader — channel-based (AC-001)
func (s *Session) tunToWS(ctx context.Context) error {
	for {
		var r tunReadResult
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r = <-s.tunReaderCh:
		}
		if r.err != nil {
			return r.err
		}
		n := r.n
		payload := r.buf[:n]
		if s.sm != nil {
			s.sm.UpdateActivity(s.sessionID)
		}
		if s.tunRouter != nil {
			if rerr := s.tunRouter.RoutePacket(payload); rerr != nil {
				s.logger.Debug("route packet error", zap.Error(rerr))
			}
			continue
		}
		if s.bwMgr != nil {
			delay, ok := s.bwMgr.Reserve(s.tokenName, n)
			if !ok {
				continue
			}
			if delay > 0 {
				time.Sleep(delay)
			}
		}
		if s.cipher != nil {
			encrypted, err := s.cipher.Encrypt(payload)
			if err != nil {
				s.logger.Error("encrypt failed, dropping packet", zap.Error(err))
				continue
			}
			payload = encrypted
		}
		f := framing.Frame{
			Type:    framing.FrameTypeData,
			Flags:   framing.FrameFlagNone,
			Payload: payload,
		}
		data, err := f.Encode()
		if err != nil {
			return err
		}
		if err := s.stream.SetWriteDeadline(time.Now().Add(s.tunnelTimeout)); err != nil {
			framing.ReturnBuffer(data)
			return err
		}
		if err := s.stream.WriteMessage(data); err != nil {
			framing.ReturnBuffer(data)
			return err
		}
		framing.ReturnBuffer(data)
	}
}
