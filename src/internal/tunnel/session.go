package tunnel

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
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
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	quictp "github.com/bzdvdn/kvn-ws/src/internal/transport/quic"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// @sk-task fix-critical-leaks#T5.1: sync.Pool for 4KB proxy buffers (AC-013)
var proxyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 4096)
		return b
	},
}

// @sk-task latency: sync.Pool for TUN read buffers — avoid a fresh 1500-byte
// allocation per packet on the latency-critical read path
var tunReadBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 1500)
		return &b
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

// dialStream carries the client→target queue for a proxy stream while the
// server dials the target (and afterwards). Every proxy frame for that
// stream routes to the same channel, so reads on wsToTun never block on a
// slow target dial and the ordering is preserved across the dial hand-off.
type dialStream struct {
	conn net.Conn
	ch   chan []byte
}

// OutgoingInterceptor is called before writing a data frame to the TUN device.
// If it returns true, the frame is considered handled and TUN write is skipped.
type OutgoingInterceptor func(payload []byte) (handled bool, err error)

// Session encapsulates bidirectional forwarding between a transport
// stream (WebSocket or QUIC) and a TUN device.
type Session struct {
	tunDev tun.TunDevice
	stream StreamConn
	// @sk-task dual-ws-channel#T2.2: secondary channel for UDP traffic (AC-002)
	// @sk-task dual-ws-channel#T3.3: buffering guards for late secondary bind (AC-005)
	// @sk-task perf-secondary-lock#T1: lock-free secondary lookup on the hot path (AC-002)
	secondary           atomic.Pointer[StreamConn]
	secondaryLoopActive atomic.Bool
	secondaryGen        atomic.Uint64
	runCtxMu            sync.RWMutex
	done                chan struct{}
	doneOnce            sync.Once
	runCtx              context.Context
	primaryReady        atomic.Bool
	// @sk-task secondary-batching#T2.1: batched secondary write mode (AC-001)
	batchMode atomic.Bool
	// @sk-task ws-media-queue-backpressure#T1.1: bounded per-channel writers (AC-001..AC-004)
	secondaryBatchHold time.Duration
	writersOnce        sync.Once
	secondaryWriter    *queueWriter
	primaryWriter      *queueWriter
	writeErrCh         chan error
	stats              sessionStats
	// onSecondaryLost is invoked when the secondary read-loop ends so the owner
	// can re-establish the channel (dual-ws-channel AC-004).
	onSecondaryLost atomic.Pointer[func()]
	// @sk-task quic-datagrams#T3.1: unreliable datagram path for real-time UDP (AC-001)
	datagramConn     transport.DatagramConn
	datagramsOn      atomic.Bool
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
	dialStreams      sync.Map // uint32 → *dialStream, dial in flight (client→target queue)
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
		tunDev:             tunDev,
		stream:             stream,
		sm:                 sm,
		sessionID:          sessionID,
		tokenName:          tokenName,
		prl:                prl,
		bwMgr:              bwMgr,
		collectors:         collectors,
		logger:             logger,
		cipher:             cipher,
		proxyStreams:       proxyStreams,
		proxySem:           make(chan struct{}, proxyConcurrency),
		tunnelTimeout:      tunnelTimeout,
		proxyConcurrency:   proxyConcurrency,
		clientIP:           clientIP,
		clientIP6:          clientIP6,
		dnsUpstreams:       dnsUpstreams,
		done:               make(chan struct{}),
		writeErrCh:         make(chan error, 1),
		secondaryBatchHold: time.Duration(defaultSecondaryBatchMs) * time.Millisecond,
	}
}

// @sk-task ws-media-queue-backpressure#T1.2: session queue counters (AC-007)
type sessionStats struct {
	secondary     queueStats
	primary       queueStats
	framingErrors atomic.Uint64
}

// @sk-task ws-media-queue-backpressure#T1.2: snapshot for tests and debug logs
func (s *Session) Stats() SessionStatsSnapshot {
	return SessionStatsSnapshot{
		Secondary:     s.stats.secondary.snapshot(),
		Primary:       s.stats.primary.snapshot(),
		FramingErrors: s.stats.framingErrors.Load(),
	}
}

// @sk-task ws-media-queue-backpressure#T1.1: lazily create bounded writers
func (s *Session) initWriters() {
	s.writersOnce.Do(func() {
		s.secondaryWriter = newQueueWriter(
			"secondary", secondaryQueueCapacity, queueBatchMaxBytes, secondaryQueueMaxAge, secondaryWriteTimeout,
			s.BatchEnabled, s.getSecondary, func() uint64 { return s.secondaryGen.Load() }, s.logger, &s.stats.secondary, nil,
		)
		s.secondaryWriter.setHold(s.secondaryBatchHold)
		s.primaryWriter = newQueueWriter(
			"primary", primaryQueueCapacity, 0, 0, s.tunnelTimeout,
			func() bool { return false }, func() StreamConn { return s.stream }, nil, s.logger, &s.stats.primary, s.reportWriteError,
		)
	})
}

// @sk-task ws-media-queue-backpressure#T2.2: configure secondary batching window
func (s *Session) SetSecondaryBatchHold(hold time.Duration) {
	if hold < 0 {
		hold = 0
	}
	if hold > maxSecondaryBatchMs*time.Millisecond {
		hold = maxSecondaryBatchMs * time.Millisecond
	}
	s.secondaryBatchHold = hold
	s.initWriters()
	s.secondaryWriter.setHold(hold)
}

func (s *Session) SetTunRouter(tr *routing.TunRouter) {
	s.tunRouter = tr
	// @sk-task dual-ws-channel: in TUN mode the router sends packets itself, so
	// route them through the session's stream classifier (UDP → secondary) to
	// keep RQ-003/AC-002 working outside proxy mode.
	if tr != nil {
		tr.SetTunnelSend(s.sendViaTunnel)
	}
}

// @sk-task ws-media-queue-backpressure#T3.1: dispatch TUN packets without
// blocking on a channel write (AC-004)
func (s *Session) dispatchPacket(packet []byte) error {
	// @sk-task quic-datagrams#T3.1: real-time UDP prefers the unreliable datagram path (AC-001)
	if handled, derr := s.sendDatagram(packet); handled {
		return derr
	}
	payload := packet
	if s.cipher != nil {
		encrypted, err := s.cipher.Encrypt(payload)
		if err != nil {
			return err
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
	s.initWriters()
	if s.getSecondary() != nil && parseIPProto(packet) {
		s.secondaryWriter.ensure(s.getRunCtx())
		s.secondaryWriter.enqueue(data, s.secondaryGen.Load())
		return nil
	}
	s.primaryWriter.ensure(s.getRunCtx())
	s.primaryWriter.enqueue(data, 0)
	return nil
}

// @sk-task ws-media-queue-backpressure#T3.1: router callback using bounded writers
// sendViaTunnel keeps the router callback contract while using the bounded
// primary/secondary writers.
func (s *Session) sendViaTunnel(packet []byte) error {
	return s.dispatchPacket(packet)
}

// SetOnSecondaryLost registers a callback invoked after the secondary read-loop
// ends, so the client can re-establish the channel (dual-ws-channel AC-004).
func (s *Session) SetOnSecondaryLost(fn func()) {
	if fn == nil {
		s.onSecondaryLost.Store(nil)
		return
	}
	s.onSecondaryLost.Store(&fn)
}

func (s *Session) getOnSecondaryLost() func() {
	if p := s.onSecondaryLost.Load(); p != nil {
		return *p
	}
	return nil
}

// @sk-task quic-datagrams#T3.1: enable/disable the datagram path for real-time UDP (AC-001)
func (s *Session) SetDatagrams(on bool) {
	s.datagramsOn.Store(on)
}

// @sk-task quic-datagrams#T3.1: send one real-time UDP packet as an unreliable datagram.
// Returns handled=false when the packet should use the framed stream path instead.
func (s *Session) sendDatagram(packet []byte) (handled bool, err error) {
	if !s.datagramsOn.Load() || s.datagramConn == nil || !s.datagramConn.SupportsDatagrams() || !parseIPProto(packet) {
		return false, nil
	}
	payload := packet
	if s.cipher != nil {
		encrypted, cerr := s.cipher.Encrypt(payload)
		if cerr != nil {
			return false, cerr
		}
		payload = encrypted
	}
	if serr := s.datagramConn.SendDatagram(payload); serr != nil {
		if _, tooLarge := quictp.DatagramTooLarge(serr); tooLarge {
			// Oversized for a datagram frame: fall back to the reliable stream.
			return false, nil
		}
		return true, serr
	}
	return true, nil
}

// @sk-task quic-datagrams#T3.2: read incoming datagrams into the TUN (AC-001, AC-002)
func (s *Session) startDatagramReader(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("datagram reader recovered from panic", zap.Any("panic", r))
			}
		}()
		for {
			payload, err := s.datagramConn.ReceiveDatagram(ctx)
			if err != nil {
				if ctx.Err() == nil {
					s.logger.Debug("datagram reader ended", zap.Error(err))
				}
				return
			}
			if s.cipher != nil {
				decrypted, derr := s.cipher.Decrypt(payload)
				if derr != nil {
					s.logger.Warn("datagram decrypt failed, dropping", zap.Error(derr))
					continue
				}
				payload = decrypted
			}
			if _, werr := s.tunDev.Write(payload); werr != nil {
				s.logger.Debug("datagram tun write failed", zap.Error(werr))
			}
		}
	}()
}

func (s *Session) SetDemux(d *TunDemux) {
	s.demux = d
}

// @sk-task dual-ws-channel#T2.2: bind secondary channel for UDP traffic (AC-002)
// @sk-task dual-ws-channel#T3.1: late secondary bind — server binds after Run started (AC-001)
func (s *Session) SetSecondary(sc StreamConn) {
	s.initWriters()
	s.secondaryGen.Add(1)
	s.secondary.Store(nil)
	s.secondaryWriter.clear()
	s.secondary.Store(&sc)
	if s.hasRunCtx() {
		s.startSecondaryLoop()
	}
}

// @sk-task perf-secondary-lock#T1: session end signal (closed when Run returns) (AC-003)
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// @sk-task secondary-batching#T2.1: enable batched secondary writes (AC-001)
func (s *Session) SetBatchMode(on bool) {
	s.batchMode.Store(on)
}

// @sk-task secondary-batching#T2.1: report batched secondary write mode (AC-001)
func (s *Session) BatchEnabled() bool {
	return s.batchMode.Load()
}

// @sk-task dual-ws-channel#T3.1: secondary loop runs independently of the primary errgroup (AC-004)
// Restartable: when the secondary read-loop ends (error or gorilla panic on a
// failed connection) the dead conn is torn down and the owner is asked to
// re-bind instead of leaving a stuck dead conn / silent media loss.
func (s *Session) startSecondaryLoop() {
	if !s.secondaryLoopActive.CompareAndSwap(false, true) {
		return
	}
	go func() {
		gen := s.secondaryGen.Load()
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.logger.Error("secondaryToTun recovered from panic", zap.Any("panic", r))
				}
			}()
			if err := s.secondaryToTun(s.getRunCtx()); err != nil {
				s.logger.Debug("secondary channel ended", zap.Error(err))
			}
		}()

		// Only tear down the binding this loop owned; if a newer secondary was
		// bound meanwhile, leave it alone.
		if s.secondaryGen.Load() == gen {
			if sc := s.secondary.Load(); sc != nil {
				_ = (*sc).Close()
			}
			s.secondaryGen.Add(1)
			s.secondary.Store(nil)
			s.secondaryWriter.clear()
		}
		s.secondaryLoopActive.Store(false)

		if ctx := s.getRunCtx(); ctx.Err() == nil {
			if fn := s.getOnSecondaryLost(); fn != nil {
				fn()
			}
		}
	}()
}

func (s *Session) SetOutgoingInterceptor(fn OutgoingInterceptor) {
	s.outgoingInterceptor = fn
}

// @sk-task fix-critical-leaks#T3.1: TUN reader — permanent goroutine (AC-001)
func (s *Session) startTunReader(ctx context.Context) {
	s.tunReaderCh = make(chan tunReadResult, 512)
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
			buf := getTunReadBuf()
			n, err := s.tunDev.Read(buf)
			select {
			case s.tunReaderCh <- tunReadResult{n, err, buf}:
			case <-ctx.Done():
				putTunReadBuf(buf)
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
	defer s.doneOnce.Do(func() { close(s.done) })
	s.initWriters()
	s.startTunReader(ctx)
	s.setRunCtx(ctx)
	// @sk-task quic-datagrams#T3.2: resolve datagram capability and start the reader (AC-001)
	if dc, ok := s.stream.(transport.DatagramConn); ok {
		s.datagramConn = dc
	}
	if s.datagramsOn.Load() && s.datagramConn != nil && s.datagramConn.SupportsDatagrams() {
		s.startDatagramReader(ctx)
	}
	eg, ctx := errgroup.WithContext(ctx)
	if s.writeErrCh != nil {
		eg.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case werr := <-s.writeErrCh:
				return werr
			}
		})
	}
	if s.logger != nil {
		go s.logStats(ctx)
	}
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
	s.primaryReady.Store(true)
	if s.hasSecondary() {
		s.startSecondaryLoop()
	}
	return eg.Wait()
}

// @sk-task ws-media-queue-backpressure#T3.4: bounded-rate queue diagnostics (AC-007)
func (s *Session) logStats(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st := s.Stats()
			s.logger.Debug("session queue stats",
				zap.String("session_id", s.sessionID),
				zap.Uint64("secondary_enqueued", st.Secondary.Enqueued),
				zap.Uint64("secondary_dropped_full", st.Secondary.DroppedFull),
				zap.Uint64("secondary_dropped_stale_age", st.Secondary.DroppedStaleAge),
				zap.Uint64("secondary_dropped_stale_gen", st.Secondary.DroppedStaleGen),
				zap.Uint64("secondary_write_errors", st.Secondary.WriteErrors),
				zap.Int64("secondary_max_depth", st.Secondary.MaxDepth),
				zap.Duration("secondary_max_write", time.Duration(st.Secondary.MaxWriteNanos)),
				zap.Uint64("primary_dropped_full", st.Primary.DroppedFull),
				zap.Uint64("primary_write_errors", st.Primary.WriteErrors),
				zap.Int64("primary_max_depth", st.Primary.MaxDepth),
				zap.Duration("primary_max_write", time.Duration(st.Primary.MaxWriteNanos)),
				zap.Uint64("framing_errors", st.FramingErrors),
			)
		}
	}
}

// @sk-task ws-media-queue-backpressure#T1.1: surface fatal primary write errors to Run
func (s *Session) reportWriteError(err error) {
	if s.writeErrCh == nil || err == nil {
		return
	}
	select {
	case s.writeErrCh <- err:
	default:
	}
}

func (s *Session) setRunCtx(ctx context.Context) {
	s.runCtxMu.Lock()
	s.runCtx = ctx
	s.runCtxMu.Unlock()
}

func (s *Session) getRunCtx() context.Context {
	s.runCtxMu.RLock()
	defer s.runCtxMu.RUnlock()
	if s.runCtx != nil {
		return s.runCtx
	}
	return context.Background()
}

func (s *Session) hasRunCtx() bool {
	s.runCtxMu.RLock()
	defer s.runCtxMu.RUnlock()
	return s.runCtx != nil
}

func (s *Session) hasSecondary() bool {
	return s.secondary.Load() != nil
}

func (s *Session) getSecondary() StreamConn {
	sc := s.secondary.Load()
	if sc == nil {
		return nil
	}
	return *sc
}

// @sk-task arch-refactoring#T3.3: decomposed wsToTun with handler methods (AC-005)
// @sk-task dual-ws-channel follow-up: gorilla marks a WS conn corrupt after a
// read timeout ("all future reads will return an error"), so continuing to read
// spins to the 1000-read panic. Treat a timeout as fatal to detect a dead
// primary in tunnelTimeout instead of black-holing traffic for up to 10x that.
func (s *Session) wsToTun(ctx context.Context) error {
	var lastRateLimitLog time.Time
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
				s.logger.Warn("primary read timeout, ending session",
					zap.Duration("timeout", s.tunnelTimeout),
					zap.String("session_id", s.sessionID),
					zap.Error(err))
			}
			return err
		}
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
// @sk-task proxy-slow-dial: async target dial — the read-loop must never
// block on net.DialTimeout; new streams are queued and dialed in a goroutine.
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
			_ = transport.WriteMessageWithDeadline(s.stream, data, time.Now().Add(s.tunnelTimeout))
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

	// Existing completed stream: ordered async write via its per-stream goroutine.
	if _, ok := s.proxyStreams.Load(streamID); ok {
		sw, ok := s.streamWriters.Load(streamID)
		if !ok {
			return
		}
		bw, _ := sw.(*streamWriter)
		tmp := make([]byte, len(data))
		copy(tmp, data)
		bw.ch <- tmp
		return
	}
	if len(data) == 0 {
		return
	}

	// Dial already in flight: enqueue (read-loop never blocks, drop on overflow).
	if sw, ok := s.dialStreams.Load(streamID); ok {
		ds, _ := sw.(*dialStream)
		tmp := make([]byte, len(data))
		copy(tmp, data)
		select {
		case ds.ch <- tmp:
		default:
		}
		return
	}

	// New stream: create the queue, start an async dial, then route initial data.
	ds := &dialStream{
		conn: nil,
		ch:   make(chan []byte, 2048),
	}
	s.dialStreams.Store(streamID, ds)
	tmp := make([]byte, len(data))
	copy(tmp, data)
	select {
	case ds.ch <- tmp:
	default:
	}
	go s.dialProxyStream(ctx, streamID, dst, ds)
}

// dialProxyStream dials the target asynchronously, then wires the client→target
// writer and the target→client forwarder. All frames route through ds.ch, so
// ordering is preserved and wsToTun stays non-blocking.
func (s *Session) dialProxyStream(ctx context.Context, sid uint32, dst string, ds *dialStream) {
	defer func() {
		// Remove only if it's the same dial we registered (defensive).
		if cur, ok := s.dialStreams.Load(sid); ok && cur == ds {
			s.dialStreams.Delete(sid)
		}
	}()

	var tcpConn net.Conn
	var err error
	if ctx.Err() == nil {
		d := &net.Dialer{Timeout: 10 * time.Second}
		tcpConn, err = d.DialContext(ctx, "tcp", dst)
	} else {
		err = ctx.Err()
	}
	if err != nil {
		s.logger.Warn("proxy dial failed", zap.String("dst", dst), zap.String("ip", dst), zap.Error(err))
		if ctx.Err() == nil {
			s.writeProxyCloseFrame(sid)
		}
		return
	}

	select {
	case s.proxySem <- struct{}{}:
	default:
		s.logger.Warn("proxy concurrency limit reached, dropping stream", zap.Uint32("stream_id", sid))
		_ = tcpConn.Close()
		s.writeProxyCloseFrame(sid)
		return
	}

	ds.conn = tcpConn
	s.proxyStreams.Store(sid, tcpConn)
	s.streamWriters.Store(sid, &streamWriter{conn: tcpConn, ch: ds.ch})
	s.dialStreams.Delete(sid)

	s.logger.Info("proxy tunnel", zap.String("dst", dst), zap.String("ip", dst))

	// Client→target writer: drains ds.ch sequentially, exits on write error or
	// session cancellation. Never closed here — the router stops feeding frames
	// once the stream is torn down.
	go func() {
		for {
			select {
			case buf := <-ds.ch:
				if ds.conn == nil {
					return
				}
				_ = ds.conn.SetWriteDeadline(time.Now().Add(s.tunnelTimeout))
				if _, err := ds.conn.Write(buf); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Target→client forward loop (owns teardown of the stream).
	go s.forwardProxyStream(sid, tcpConn, dst, ctx)
}

// writeProxyCloseFrame sends a proxy close frame with no destination length,
// signalling to the client the stream is done.
func (s *Session) writeProxyCloseFrame(sid uint32) {
	closeFrame := framing.Frame{
		Type:    framing.FrameTypeProxy,
		Payload: make([]byte, 6),
	}
	binary.BigEndian.PutUint32(closeFrame.Payload[0:4], sid)
	binary.BigEndian.PutUint16(closeFrame.Payload[4:6], 0)
	if encoded, encErr := closeFrame.Encode(); encErr == nil {
		if err := transport.WriteMessageWithDeadline(s.stream, encoded, time.Now().Add(s.tunnelTimeout)); err != nil {
			s.logger.Warn("write close frame failed", zap.Error(err))
		}
		framing.ReturnBuffer(encoded)
	}
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
	_ = transport.WriteMessageWithDeadline(s.stream, encoded, time.Now().Add(10*time.Second))
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
		s.streamWriters.Delete(sid)
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
			if err := transport.WriteMessageWithDeadline(s.stream, encoded, time.Now().Add(s.tunnelTimeout)); err != nil {
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
		if err := transport.WriteMessageWithDeadline(s.stream, encoded, time.Now().Add(s.tunnelTimeout)); err != nil {
			framing.ReturnBuffer(encoded)
			return
		}
		framing.ReturnBuffer(encoded)
	}
}

// @sk-task ws-media-queue-backpressure#T3.2: strict framing check for a full
// no-pad batch; rejects partial frames, trailing bytes and non-data frames (AC-005)
func validateSecondaryBatch(data []byte) error {
	for len(data) > 0 {
		if len(data) < framing.FrameHeaderSize {
			return fmt.Errorf("secondary framing: %d trailing bytes", len(data))
		}
		frameType := data[0]
		length := int(binary.BigEndian.Uint16(data[2:4]))
		if frameType != framing.FrameTypeData && frameType != framing.FrameTypeClose {
			return fmt.Errorf("secondary framing: unexpected frame type %d", frameType)
		}
		next := framing.FrameHeaderSize + length
		if next > len(data) {
			return fmt.Errorf("secondary framing: frame length %d exceeds remaining %d", length, len(data)-framing.FrameHeaderSize)
		}
		data = data[next:]
	}
	return nil
}

// @sk-task dual-ws-channel#T2.2: secondary read-loop — FrameTypeData → decrypt → TUN (AC-002)
// @sk-task dual-ws-channel#T3.3: buffer until primaryReady; 300ms / 1024 packets then flush or drop (AC-005)
func (s *Session) secondaryToTun(ctx context.Context) error {
	if !s.hasSecondary() {
		return nil
	}
	const (
		bufferDuration = 300 * time.Millisecond
		bufferMax      = 1024
	)
	var (
		buffered    [][]byte
		bufferStart time.Time
	)
	flush := func() error {
		for _, p := range buffered {
			if _, err := s.tunDev.Write(p); err != nil {
				return err
			}
		}
		buffered = buffered[:0]
		return nil
	}
	var lastRateLimitLog time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		secondary := s.getSecondary()
		if secondary == nil {
			return nil
		}
		if s.prl != nil && !s.prl.Allow(s.sessionID) {
			if time.Since(lastRateLimitLog) > time.Second {
				lastRateLimitLog = time.Now()
				pkglog.Audit(s.logger, zapcore.WarnLevel, "secondary packet rate limited",
					zap.String("session_id", s.sessionID),
				)
			}
			continue
		}
		if err := secondary.SetReadDeadline(time.Now().Add(s.tunnelTimeout)); err != nil {
			return err
		}
		data, err := secondary.ReadMessage()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// gorilla corrupts the connection state after a read timeout and
				// panics if read again; end the loop so the owner re-binds.
				s.logger.Warn("secondary read timeout, re-binding channel", zap.Error(err))
			}
			return err
		}
		// @sk-task ws-media-queue-backpressure#T3.2: validate the whole message
		// before writing anything to TUN (AC-005)
		if verr := validateSecondaryBatch(data); verr != nil {
			s.stats.framingErrors.Add(1)
			return verr
		}
		// @sk-task secondary-batching#T2.2: decode every frame in a batched WS message (AC-003)
		for len(data) >= framing.FrameHeaderSize {
			var f framing.Frame
			if err := f.Decode(data); err != nil {
				s.stats.framingErrors.Add(1)
				return fmt.Errorf("secondary framing: %w", err)
			}
			data = data[framing.FrameHeaderSize+int(f.Length):]
			if f.Type == framing.FrameTypeClose {
				f.Release()
				return nil
			}
			if f.Type != framing.FrameTypeData {
				f.Release()
				s.stats.framingErrors.Add(1)
				return fmt.Errorf("secondary framing: unexpected frame type %d", f.Type)
			}
			if s.cipher != nil {
				decrypted, err := s.cipher.Decrypt(f.Payload)
				if err != nil {
					s.logger.Warn("secondary decrypt failed, dropping packet", zap.Error(err))
					f.Release()
					continue
				}
				f.Release()
				f.Payload = decrypted
			}
			if !s.primaryReady.Load() {
				if len(buffered) == 0 {
					bufferStart = time.Now()
				}
				if len(buffered) < bufferMax && time.Since(bufferStart) <= bufferDuration {
					payload := make([]byte, len(f.Payload))
					copy(payload, f.Payload)
					buffered = append(buffered, payload)
					f.Release()
					continue
				}
				// Buffer full or primary not ready in time: drop the new packet
				// (kept buffered packets are flushed once primary becomes ready).
				s.logger.Debug("secondary buffer full or timeout, dropping incoming",
					zap.Int("buffered", len(buffered)),
				)
				f.Release()
				continue
			}
			if len(buffered) > 0 {
				if err := flush(); err != nil {
					f.Release()
					return err
				}
			}
			if _, err := s.tunDev.Write(f.Payload); err != nil {
				f.Release()
				return err
			}
			f.Release()
		}
		if len(data) != 0 {
			s.stats.framingErrors.Add(1)
			return fmt.Errorf("secondary framing: %d trailing bytes", len(data))
		}
	}
}

// @sk-task fix-critical-leaks#T3.1: TUN reader — channel-based (AC-001)
// @sk-task secondary-batching#T2.1: batched secondary writes (AC-002, AC-005)
// @sk-task secondary-batching-fix#T1.1: secondary writes routed to a dedicated writer goroutine (AC-002)
func (s *Session) tunToWS(ctx context.Context) error {
	for {
		var r tunReadResult
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r = <-s.tunReaderCh:
		}
		if r.err != nil {
			if r.buf != nil {
				putTunReadBuf(r.buf)
			}
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
			putTunReadBuf(r.buf)
			continue
		}
		if s.bwMgr != nil {
			delay, ok := s.bwMgr.Reserve(s.tokenName, n)
			if !ok {
				putTunReadBuf(r.buf)
				continue
			}
			if delay > 0 {
				time.Sleep(delay)
			}
		}
		// @sk-task ws-media-queue-backpressure#T3.1: enqueue without blocking on a channel write (AC-004)
		if derr := s.dispatchPacket(payload); derr != nil {
			s.logger.Debug("dispatch packet error", zap.Error(derr))
		}
		putTunReadBuf(r.buf)
	}
}

// @sk-task latency: pool get/put helpers for TUN read buffers
func getTunReadBuf() []byte {
	ptr, ok := tunReadBufPool.Get().(*[]byte)
	if !ok {
		return make([]byte, 1500)
	}
	return *ptr
}

func putTunReadBuf(buf []byte) {
	tunReadBufPool.Put(&buf)
}
