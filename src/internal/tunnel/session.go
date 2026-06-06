// @sk-task production-readiness-hardening#T2.5: tunnel session abstraction (AC-005)
package tunnel

import (
	"context"
	"encoding/binary"
	"math"
	"net"
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

const wsTunnelTimeout = 30 * time.Second
const defaultProxyConcurrency = 1000

// Session encapsulates bidirectional forwarding between a transport
// stream (WebSocket or QUIC) and a TUN device.
type Session struct {
	tunDev         tun.TunDevice
	stream         StreamConn
	sm             *session.SessionManager
	sessionID      string
	tokenName      string
	prl            *ratelimit.SessionPacketLimiter
	bwMgr          *session.TokenBandwidthManager
	collectors     *metrics.Collectors
	logger         *zap.Logger
	cipher         *crypto.SessionCipher
	proxyStreams   *proxy.SessionStreams
	proxySem       chan struct{}
	tunRouter      *routing.TunRouter
	interruptRead  bool
}

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
) *Session {
	return &Session{
		tunDev:       tunDev,
		stream:       stream,
		sm:           sm,
		sessionID:    sessionID,
		tokenName:    tokenName,
		prl:          prl,
		bwMgr:        bwMgr,
		collectors:   collectors,
		logger:       logger,
		cipher:       cipher,
		proxyStreams: proxyStreams,
		proxySem:     make(chan struct{}, defaultProxyConcurrency),
	}
}

func (s *Session) SetTunRouter(tr *routing.TunRouter) {
	s.tunRouter = tr
}

func (s *Session) SetInterruptibleRead(enabled bool) {
	s.interruptRead = enabled
}

func (s *Session) tunReadInterruptible(ctx context.Context, buf []byte) (int, error) {
	readBuf := make([]byte, len(buf))
	type result struct {
		n   int
		err error
	}
	ch := make(chan result, 1)
	go func() {
		n, err := s.tunDev.Read(readBuf)
		ch <- result{n, err}
	}()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case r := <-ch:
		copy(buf, readBuf[:r.n])
		return r.n, r.err
	}
}

// Run spawns the two forwarding goroutines (WS→TUN and TUN→WS) and
// blocks until one fails or ctx is cancelled.
func (s *Session) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return s.wsToTun(ctx) })
	eg.Go(func() error { return s.tunToWS(ctx) })
	return eg.Wait()
}

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
		if err := s.stream.SetReadDeadline(time.Now().Add(wsTunnelTimeout)); err != nil {
			return err
		}
		data, err := s.stream.ReadMessage()
		if err != nil {
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
			n, err := s.tunDev.Write(f.Payload)
			f.Release()
			if err != nil {
				return err
			}
			if s.collectors != nil {
				s.collectors.AddThroughput("rx", float64(n))
			}
		case framing.FrameTypeClose:
			f.Release()
			s.logger.Debug("session close frame received", zap.String("session_id", s.sessionID))
			return nil
		case framing.FrameTypeProxy:
			if s.proxyStreams == nil {
				f.Release()
				continue
			}
			payload := f.Payload
			if len(payload) < 6 {
				f.Release()
				continue
			}
			streamID := binary.BigEndian.Uint32(payload[0:4])
			dstLen := binary.BigEndian.Uint16(payload[4:6])
			if int(6+dstLen) > len(payload) {
				f.Release()
				continue
			}
			dst := string(payload[6 : 6+dstLen])
			data := payload[6+dstLen:]

			if v, ok := s.proxyStreams.Load(streamID); ok {
				_, _ = v.Write(data)
				f.Release()
			} else {
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
						_ = s.stream.WriteMessage(encoded)
						framing.ReturnBuffer(encoded)
					}
					f.Release()
					continue
				}
				s.proxyStreams.Store(streamID, tcpConn)
				s.logger.Info("proxy tunnel", zap.String("dst", dst), zap.String("ip", dst))
				if len(data) > 0 {
					_, _ = tcpConn.Write(data)
				}
				f.Release()

				select {
				case s.proxySem <- struct{}{}:
				default:
					s.logger.Warn("proxy concurrency limit reached, dropping stream", zap.Uint32("stream_id", streamID))
					_ = tcpConn.Close()
					s.proxyStreams.Delete(streamID)
					continue
				}

				go func(sid uint32, tcp net.Conn, stream StreamConn, streams *proxy.SessionStreams, parentCtx context.Context) {
					defer func() {
						<-s.proxySem
						_ = tcp.Close()
						streams.Delete(sid)
						closeFrame := framing.Frame{
							Type:    framing.FrameTypeProxy,
							Payload: make([]byte, 6),
						}
						binary.BigEndian.PutUint32(closeFrame.Payload[0:4], sid)
						binary.BigEndian.PutUint16(closeFrame.Payload[4:6], 0)
						if encoded, encErr := closeFrame.Encode(); encErr == nil {
							_ = stream.WriteMessage(encoded)
							framing.ReturnBuffer(encoded)
						}
					}()
					buf := make([]byte, 4096)
					for {
						if err := tcp.SetReadDeadline(time.Now().Add(wsTunnelTimeout)); err != nil {
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
						frame := framing.Frame{
							Type:    framing.FrameTypeProxy,
							Flags:   framing.FrameFlagNone,
							Payload: make([]byte, 4+2+len(dst)+n),
						}
						binary.BigEndian.PutUint32(frame.Payload[0:4], sid)
						binary.BigEndian.PutUint16(frame.Payload[4:6], uint16(len(dst)))
						copy(frame.Payload[6:], dst)
						copy(frame.Payload[6+len(dst):], buf[:n])
						encoded, err := frame.Encode()
						if err != nil {
							return
						}
						if err := stream.WriteMessage(encoded); err != nil {
							framing.ReturnBuffer(encoded)
							return
						}
						framing.ReturnBuffer(encoded)
					}
				}(streamID, tcpConn, s.stream, s.proxyStreams, ctx)
			}
		default:
			f.Release()
		}
	}
}

func (s *Session) tunToWS(ctx context.Context) error {
	buf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		var n int
		var err error
		if s.interruptRead {
			n, err = s.tunReadInterruptible(ctx, buf)
		} else {
			n, err = s.tunDev.Read(buf)
		}
		if err != nil {
			return err
		}
		if s.sm != nil {
			s.sm.UpdateActivity(s.sessionID)
		}
		if s.tunRouter != nil {
			if rerr := s.tunRouter.RoutePacket(buf[:n]); rerr != nil {
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
		payload := buf[:n]
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
		if err := s.stream.SetWriteDeadline(time.Now().Add(wsTunnelTimeout)); err != nil {
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
