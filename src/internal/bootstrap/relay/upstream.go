package relay

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// @sk-task relay-terminator#T3.2: upstream session (AC-004)
// @sk-task lock-optimization#T3.4: closed → atomic.Bool (AC-005)
type upstreamSession struct {
	stream     transport.StreamConn
	tunDev     tun.TunDevice
	logger     *zap.Logger
	assignedIP net.IP
	nat        *natTracker

	closed atomic.Bool
}

func (us *upstreamSession) isClosed() bool {
	return us.closed.Load()
}

// @sk-task relay-terminator#T3.2: dial upstream + handshake (AC-004)
// @sk-task relay-terminator#T5.1: QUIC upstream dial (AC-004)
func dialUpstream(ctx, sessionCtx context.Context, cfg *config.RelayConfig, tunDev tun.TunDevice, logger *zap.Logger, transportHint string, nat *natTracker) (*upstreamSession, error) {
	tlsCfg, err := relayTLSConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("upstream tls: %w", err)
	}

	upTransport := cfg.Transport
	if upTransport == "" {
		upTransport = transportHint
	}
	if upTransport == "" {
		upTransport = "tcp"
	}

	var (
		conn        transport.StreamConn
		assignedIP  net.IP
		handshakeOk bool
	)
	if upTransport == "quic" {
		var hErr error
		conn, assignedIP, hErr = dialAndHandshake(ctx, cfg, tlsCfg, logger)
		if hErr == nil {
			handshakeOk = true
		} else {
			logger.Warn("QUIC upstream failed, falling back to TCP", zap.Error(hErr))
		}
	}
	if !handshakeOk {
		var hErr error
		conn, assignedIP, hErr = dialAndHandshakeWS(ctx, cfg, tlsCfg, logger)
		if hErr != nil {
			return nil, fmt.Errorf("upstream dial: %w", hErr)
		}
	}

	us, hErr := buildSession(sessionCtx, conn, tunDev, logger, assignedIP, nat)
	if hErr != nil {
		_ = conn.Close()
		return nil, hErr
	}
	return us, nil
}

// @sk-task transport-factory#T2.3: dialAndHandshake uses QUICFactory (AC-004)
func dialAndHandshake(ctx context.Context, cfg *config.RelayConfig, tlsCfg *tls.Config, logger *zap.Logger) (transport.StreamConn, net.IP, error) {
	quicCfg := &transport.FactoryConfig{
		TLS:         tlsCfg,
		Logger:      logger,
		Obfuscation: cfg.Obfuscation != nil && cfg.Obfuscation.Enabled,
	}
	factory := transport.NewFactory("quic", quicCfg)
	conn, err := factory.Dial(ctx, cfg.Server)
	if err != nil {
		return nil, nil, err
	}
	assignedIP, hErr := doHandshake(conn, cfg, logger)
	if hErr != nil {
		_ = conn.Close()
		return nil, nil, hErr
	}
	return conn, assignedIP, nil
}

// @sk-task transport-factory#T2.3: dialAndHandshakeWS uses WSFactory (AC-004)
func dialAndHandshakeWS(ctx context.Context, cfg *config.RelayConfig, tlsCfg *tls.Config, logger *zap.Logger) (transport.StreamConn, net.IP, error) {
	paddingEnabled := cfg.Obfuscation != nil && cfg.Obfuscation.Padding != nil && cfg.Obfuscation.Padding.Enabled
	if !paddingEnabled && cfg.Obfuscation != nil && cfg.Obfuscation.Enabled {
		paddingEnabled = true
	}
	paddingSize := 512
	if cfg.Obfuscation != nil && cfg.Obfuscation.Padding != nil && cfg.Obfuscation.Padding.Size > 0 {
		paddingSize = cfg.Obfuscation.Padding.Size
	}
	wsCfg := &transport.FactoryConfig{
		TLS:               tlsCfg,
		Logger:            logger,
		KeepaliveInterval: control.DefaultPingInterval,
		KeepaliveTimeout:  control.DefaultPongTimeout,
		PaddingEnabled:    paddingEnabled,
		PaddingSize:       paddingSize,
	}
	factory := transport.NewFactory("ws", wsCfg)
	conn, err := factory.Dial(ctx, cfg.Server)
	if err != nil {
		return nil, nil, err
	}
	assignedIP, hErr := doHandshake(conn, cfg, logger)
	if hErr != nil {
		_ = conn.Close()
		return nil, nil, hErr
	}
	return conn, assignedIP, nil
}

// @sk-task relay-terminator#T3.2: padding size from obfuscation config (upstream helper)
func paddingSizeOrDefault(oc *config.ObfuscationCfg) int {
	if oc != nil && oc.Padding != nil && oc.Padding.Size > 0 {
		return oc.Padding.Size
	}
	return 512
}

// @sk-task relay-terminator#T7.1: upstream handshake with token from config or env (AC-004)
func doHandshake(conn transport.StreamConn, cfg *config.RelayConfig, logger *zap.Logger) (net.IP, error) {
	token := cfg.UpstreamToken
	if token == "" {
		token = os.Getenv("KVN_RELAY_AUTH_TOKEN")
	}
	clientHello, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Token:        token,
		Mtu:          handshake.DefaultMTU,
	})
	if err != nil {
		return nil, fmt.Errorf("encode client hello: %w", err)
	}
	helloData, err := clientHello.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode hello frame: %w", err)
	}
	if err := conn.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		return nil, fmt.Errorf("send client hello: %w", err)
	}
	framing.ReturnBuffer(helloData)

	resp, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read server hello: %w", err)
	}
	var frame framing.Frame
	if err := frame.Decode(resp); err != nil {
		return nil, fmt.Errorf("decode server hello frame: %w", err)
	}
	sh, err := handshake.DecodeServerHello(&frame)
	if err != nil {
		return nil, fmt.Errorf("decode server hello: %w", err)
	}
	return sh.AssignedIp, nil
}

// @sk-task relay-terminator#T3.2: build upstreamSession from connected stream (AC-004)
func buildSession(ctx context.Context, conn transport.StreamConn, tunDev tun.TunDevice, logger *zap.Logger, assignedIP net.IP, nat *natTracker) (*upstreamSession, error) {
	var sidBuf [16]byte
	if _, rerr := rand.Read(sidBuf[:]); rerr != nil {
		return nil, fmt.Errorf("generate session id: %w", rerr)
	}
	sessionID := hex.EncodeToString(sidBuf[:])
	logger.Info("upstream handshake OK",
		zap.String("session", sessionID),
		zap.String("assigned_ip", assignedIP.String()),
	)
	us := &upstreamSession{
		stream:     conn,
		tunDev:     tunDev,
		logger:     logger,
		assignedIP: assignedIP,
		nat:        nat,
	}
	go us.receiveLoop(ctx)
	return us, nil
}

// @sk-task relay-terminator#T3.2: send packet upstream (AC-004)
func (us *upstreamSession) Send(packet []byte) error {
	if us.closed.Load() {
		return fmt.Errorf("upstream closed")
	}
	f := framing.Frame{
		Type:    framing.FrameTypeData,
		Flags:   framing.FrameFlagNone,
		Payload: packet,
	}
	data, err := f.Encode()
	if err != nil {
		return err
	}
	defer framing.ReturnBuffer(data)
	return us.stream.WriteMessage(data)
}

// @sk-task relay-terminator#T3.2: receive responses from upstream (AC-004)
func (us *upstreamSession) receiveLoop(ctx context.Context) {
	defer func() {
		us.closed.Store(true)
		_ = us.stream.Close()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := us.stream.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			return
		}
		data, err := us.stream.ReadMessage()
		if err != nil {
			us.logger.Warn("upstream read error", zap.Error(err))
			return
		}
		var f framing.Frame
		if err := f.Decode(data); err != nil {
			us.logger.Warn("upstream decode frame error", zap.Error(err))
			continue
		}
		if f.Type != framing.FrameTypeData {
			f.Release()
			continue
		}
		payload := f.Payload
		us.logger.Debug("upstream recv frame", zap.Int("len", len(payload)))
		if us.nat != nil {
			dnatBuf := make([]byte, len(payload))
			copy(dnatBuf, payload)
			if us.nat.dnat(dnatBuf) {
				us.logger.Debug("nat dnat applied")
				payload = dnatBuf
			} else {
				us.logger.Debug("nat dnat skipped")
			}
		}
		if _, err := us.tunDev.Write(payload); err != nil {
			us.logger.Warn("upstream tun write error", zap.Error(err))
		}
		f.Release()
	}
}
