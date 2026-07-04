package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	pkglog "github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/auth"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tunnel"
)

func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request, wsCfg websocket.WSConfig) {
	if !isWebSocketRequest(r) {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}
	wsConn, err := websocket.Accept(w, r, s.logger, s.originChecker, wsCfg)
	if err != nil {
		s.logger.Error("ws upgrade", zap.Error(err))
		return
	}
	// @sk-task relay-terminator#T9.1: WS keepalive on accepted server connections (AC-004)
	wsConn.SetKeepalive(control.DefaultPingInterval, control.DefaultPongTimeout)
	s.handleStream(r.Context(), wsConn, wsCfg.MTU, r.RemoteAddr)
}

// @sk-task decoy-hardening: return 404 for non-WebSocket requests
func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// @sk-task whitelist-obfuscation#T2.2: WS path allowlist check (AC-003)
func (s *Server) allowedWSPath(path string) bool {
	for _, p := range s.cfg.WSPaths {
		if p == path {
			return true
		}
	}
	return false
}

// @sk-task whitelist-obfuscation#T3.2: padding size default helper (AC-005)
func paddingSizeOrDefault(oc *config.ObfuscationCfg) int {
	if oc != nil && oc.Padding != nil && oc.Padding.Size > 0 {
		return oc.Padding.Size
	}
	return 512
}

// @sk-task quic-transport#T3.1: shared stream handler for WS and QUIC (AC-001)
func (s *Server) handleStream(ctx context.Context, stream tunnel.StreamConn, mtu int, remoteAddr string) {
	data, err := stream.ReadMessage()
	if err != nil {
		s.logger.Error("read client hello", zap.Error(err))
		_ = stream.Close()
		return
	}
	var frame framing.Frame
	if err := frame.Decode(data); err != nil {
		s.logger.Error("decode client hello frame", zap.Error(err))
		_ = stream.Close()
		return
	}
	clientHello, err := handshake.DecodeClientHello(&frame)
	if err != nil {
		s.logger.Error("decode client hello", zap.Error(err))
		_ = stream.Close()
		return
	}

	tokenCfg := auth.FindToken(clientHello.Token, s.cfg.Auth.Tokens)
	if tokenCfg == nil {
		pkglog.Audit(s.logger, zapcore.WarnLevel, "auth failed",
			zap.String("session_id", ""),
			zap.String("reason", "invalid token"),
			zap.String("remote_addr", remoteAddr),
		)
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: "authentication failed"})
		authData, _ := authFrame.Encode()
		_ = stream.WriteMessage(authData)
		framing.ReturnBuffer(authData)
		_ = stream.Close()
		return
	}

	tokenName := tokenCfg.Name
	var sidBuf [16]byte
	if _, rerr := rand.Read(sidBuf[:]); rerr != nil {
		copy(sidBuf[:], clientHello.Token)
	}
	sessionID := hex.EncodeToString(sidBuf[:])
	sess, assignedIP, assignedIPv6, err := s.sm.Create(sessionID, tokenName, remoteAddr, tokenCfg.MaxSessions, clientHello.Ipv6)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "max sessions exceeded") {
			pkglog.Audit(s.logger, zapcore.WarnLevel, "max sessions exceeded",
				zap.String("token_name", tokenName),
				zap.String("remote_addr", remoteAddr),
			)
		} else {
			s.logger.Error("session create", zap.Error(err))
		}
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: errMsg})
		authData, _ := authFrame.Encode()
		_ = stream.WriteMessage(authData)
		framing.ReturnBuffer(authData)
		_ = stream.Close()
		return
	}

	var sessionCipher *crypto.SessionCipher
	var cryptoSalt []byte
	if len(s.masterKey) > 0 {
		cryptoSalt, err = crypto.GenerateSalt()
		if err != nil {
			s.logger.Error("generate crypto salt", zap.Error(err))
			_ = stream.Close()
			return
		}
	}

	if mtu <= 0 {
		mtu = handshake.DefaultMTU
	}
	serverHello, err := handshake.EncodeServerHello(&handshake.ServerHello{
		SessionId:    sess.ID,
		AssignedIp:   assignedIP,
		AssignedIpv6: assignedIPv6,
		Mtu:          mtu,
		CryptoSalt:   cryptoSalt,
		GatewayIp:    s.gatewayIP,
	})
	if err != nil {
		s.logger.Error("encode server hello", zap.Error(err))
		_ = stream.Close()
		return
	}
	helloData, err := serverHello.Encode()
	if err != nil {
		s.logger.Error("encode hello frame", zap.Error(err))
		_ = stream.Close()
		return
	}
	if err := stream.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		s.logger.Error("send server hello", zap.Error(err))
		_ = stream.Close()
		return
	}
	framing.ReturnBuffer(helloData)

	s.logger.Info("session created",
		zap.String("session", sess.ID),
		zap.String("token", tokenName),
		zap.String("ip", assignedIP.String()),
	)
	s.collectors.ActiveSessions.Inc()

	if len(cryptoSalt) > 0 {
		sessionCipher, err = crypto.NewSessionCipher(s.masterKey, cryptoSalt, sess.ID)
		if err != nil {
			s.logger.Error("session cipher init", zap.Error(err))
			_ = stream.Close()
			return
		}
	}

	sessionCtx, sessionCancel := context.WithCancel(ctx)
	s.sm.SetCancel(sess.ID, sessionCancel)
	sessionStreams := proxy.NewSessionStreams()

	tunnelTimeout := 30 * time.Second
	if s.cfg.Transport == "quic" {
		tunnelTimeout = 60 * time.Second
	}
	// @sk-task dns-upstreams-list#T3.2: pass DNSUpstreams from server config (AC-006)
	tunSess := tunnel.NewSession(s.tunDev, stream, s.sm, sess.ID, tokenName, s.prl, s.bwMgr, s.collectors, s.logger, sessionCipher, sessionStreams,
		tunnelTimeout, 1000, assignedIP, assignedIPv6, s.cfg.DNSUpstreams)
	tunSess.SetDemux(s.tunDemux)
	if err := tunSess.Run(sessionCtx); err != nil {
		s.logger.Info("session ended",
			zap.String("session", sess.ID),
			zap.String("token", tokenName),
			zap.String("ip", assignedIP.String()),
			zap.Error(err),
		)
	}
	s.collectors.ActiveSessions.Dec()

	sessionCancel()
	sessionStreams.CloseAll()
	s.sm.Remove(sess.ID)
	_ = stream.Close()
}
