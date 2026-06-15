package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	pkglog "github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/auth"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tunnel"
)

// @sk-task relay-terminator#T2.1: terminator WS handler (AC-001)
func (r *Relay) handleTerminatorWS(w http.ResponseWriter, req *http.Request) {
	if !isWebSocketRequest(req) {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}
	wsConn, err := websocket.Accept(w, req, r.logger)
	if err != nil {
		r.logger.Error("terminator ws upgrade", zap.Error(err))
		return
	}
	r.handleTerminatorStream(req.Context(), wsConn, req.RemoteAddr)
}

// @sk-task relay-terminator#T2.1: terminator stream handler — handshake + session (AC-001, AC-004)
// @sk-task relay-terminator#T2.3: cleanup at disconnect (AC-006)
func (r *Relay) handleTerminatorStream(ctx context.Context, stream tunnel.StreamConn, remoteAddr string) {
	defer func() { _ = stream.Close() }()

	data, err := stream.ReadMessage()
	if err != nil {
		r.logger.Error("read client hello", zap.Error(err))
		return
	}
	var frame framing.Frame
	if err := frame.Decode(data); err != nil {
		r.logger.Error("decode client hello frame", zap.Error(err))
		return
	}
	clientHello, err := handshake.DecodeClientHello(&frame)
	if err != nil {
		r.logger.Error("decode client hello", zap.Error(err))
		return
	}

	tokenCfg := auth.FindToken(clientHello.Token, r.cfg.Auth.Tokens)
	if tokenCfg == nil {
		pkglog.Audit(r.logger, zapcore.WarnLevel, "auth failed",
			zap.String("reason", "invalid token"),
			zap.String("remote", remoteAddr),
		)
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: "authentication failed"})
		authData, _ := authFrame.Encode()
		_ = stream.WriteMessage(authData)
		return
	}

	var sidBuf [16]byte
	if _, rerr := rand.Read(sidBuf[:]); rerr != nil {
		copy(sidBuf[:], clientHello.Token)
	}
	sessionID := hex.EncodeToString(sidBuf[:])

	allocatedIP, err := r.pool.Allocate(sessionID)
	if err != nil {
		r.logger.Error("allocate ip", zap.Error(err))
		return
	}

	var allocatedIPv6 net.IP
	if clientHello.Ipv6 && r.pool6 != nil {
		allocatedIPv6, err = r.pool6.Allocate(sessionID)
		if err != nil {
			r.logger.Warn("allocate ipv6, running ipv4-only", zap.Error(err))
		}
	}

	serverHello, err := handshake.EncodeServerHello(&handshake.ServerHello{
		SessionId:  sessionID,
		AssignedIp: allocatedIP,
		AssignedIpv6: allocatedIPv6,
		Mtu:        handshake.DefaultMTU,
	})
	if err != nil {
		r.logger.Error("encode server hello", zap.Error(err))
		r.pool.Release(sessionID)
		return
	}
	helloData, err := serverHello.Encode()
	if err != nil {
		r.logger.Error("encode hello frame", zap.Error(err))
		r.pool.Release(sessionID)
		return
	}
	if err := stream.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		r.logger.Error("send server hello", zap.Error(err))
		r.pool.Release(sessionID)
		return
	}
	framing.ReturnBuffer(helloData)

	r.logger.Info("terminator session created",
		zap.String("session", sessionID),
		zap.String("ip", allocatedIP.String()),
		zap.String("remote", remoteAddr),
	)

	tunDevForClient := r.tunDev
	if r.rt != nil {
		tunDevForClient = r.rt
	}
	tunSess := tunnel.NewSession(tunDevForClient, stream, r.sm, sessionID, "terminator",
		nil, nil, nil, r.logger, nil, nil,
		30*time.Second, 1000, allocatedIP, allocatedIPv6)
	tunSess.SetDemux(r.tunDemux)
	if err := tunSess.Run(ctx); err != nil {
		r.logger.Info("terminator session ended",
			zap.String("session", sessionID),
			zap.String("ip", allocatedIP.String()),
			zap.Error(err),
		)
	}

	r.pool.Release(sessionID)
	if allocatedIPv6 != nil {
		r.pool6.Release(sessionID)
	}
	r.sm.Remove(sessionID)
}