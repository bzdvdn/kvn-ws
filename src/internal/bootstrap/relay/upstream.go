package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// @sk-task relay-terminator#T3.2: upstream session (AC-004)
type upstreamSession struct {
	stream transport.StreamConn
	tunDev tun.TunDevice
	logger *zap.Logger
	assignedIP net.IP

	mu     sync.Mutex
	closed bool
}

// @sk-task relay-terminator#T3.2: dial upstream + handshake (AC-004)
func dialUpstream(ctx context.Context, cfg *config.RelayConfig, tunDev tun.TunDevice, logger *zap.Logger) (*upstreamSession, error) {
	tlsCfg, err := relayTLSConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("upstream tls: %w", err)
	}

	conn, err := websocket.Dial(cfg.Server, tlsCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("upstream dial: %w", err)
	}
	conn.SetKeepalive(control.DefaultPingInterval, control.DefaultPongTimeout)

	var sidBuf [16]byte
	if _, rerr := rand.Read(sidBuf[:]); rerr != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("generate session id: %w", rerr)
	}
	sessionID := hex.EncodeToString(sidBuf[:])

	token := os.Getenv("KVN_RELAY_AUTH_TOKEN")
	clientHello, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Token:        token,
		Mtu:          handshake.DefaultMTU,
	})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("encode client hello: %w", err)
	}
	helloData, err := clientHello.Encode()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("encode hello frame: %w", err)
	}
	if err := conn.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		_ = conn.Close()
		return nil, fmt.Errorf("send client hello: %w", err)
	}
	framing.ReturnBuffer(helloData)

	resp, err := conn.ReadMessage()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read server hello: %w", err)
	}
	var frame framing.Frame
	if err := frame.Decode(resp); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("decode server hello frame: %w", err)
	}
	serverHello, err := handshake.DecodeServerHello(&frame)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("decode server hello: %w", err)
	}

	logger.Info("upstream handshake OK",
		zap.String("session", sessionID),
		zap.String("assigned_ip", serverHello.AssignedIp.String()),
	)

	us := &upstreamSession{
		stream:     conn,
		tunDev:     tunDev,
		logger:     logger,
		assignedIP: serverHello.AssignedIp,
	}

	go us.receiveLoop(ctx)
	return us, nil
}

// @sk-task relay-terminator#T3.2: send packet upstream (AC-004)
func (us *upstreamSession) Send(packet []byte) error {
	us.mu.Lock()
	defer us.mu.Unlock()
	if us.closed {
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
		us.mu.Lock()
		us.closed = true
		us.mu.Unlock()
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
		if _, err := us.tunDev.Write(f.Payload); err != nil {
			us.logger.Warn("upstream tun write error", zap.Error(err))
		}
		f.Release()
	}
}
