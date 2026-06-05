package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	pkglog "github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/auth"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tunnel"
)

func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request, wsCfg websocket.WSConfig) {
	wsConn, err := websocket.Accept(w, r, s.logger, s.originChecker, wsCfg)
	if err != nil {
		s.logger.Error("ws upgrade", zap.Error(err))
		return
	}

	data, err := wsConn.ReadMessage()
	if err != nil {
		s.logger.Error("read client hello", zap.Error(err))
		_ = wsConn.Close()
		return
	}
	var frame framing.Frame
	if err := frame.Decode(data); err != nil {
		s.logger.Error("decode client hello frame", zap.Error(err))
		_ = wsConn.Close()
		return
	}
	clientHello, err := handshake.DecodeClientHello(&frame)
	if err != nil {
		s.logger.Error("decode client hello", zap.Error(err))
		_ = wsConn.Close()
		return
	}

	tokenCfg := auth.FindToken(clientHello.Token, s.cfg.Auth.Tokens)
	if tokenCfg == nil {
		pkglog.Audit(s.logger, zapcore.WarnLevel, "auth failed",
			zap.String("session_id", ""),
			zap.String("reason", "invalid token"),
			zap.String("remote_addr", r.RemoteAddr),
		)
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: "authentication failed"})
		authData, _ := authFrame.Encode()
		_ = wsConn.WriteMessage(authData)
		framing.ReturnBuffer(authData)
		_ = wsConn.Close()
		return
	}

	tokenName := tokenCfg.Name
	var sidBuf [16]byte
	if _, rerr := rand.Read(sidBuf[:]); rerr != nil {
		copy(sidBuf[:], clientHello.Token)
	}
	sessionID := hex.EncodeToString(sidBuf[:])
	sess, assignedIP, assignedIPv6, err := s.sm.Create(sessionID, tokenName, r.RemoteAddr, tokenCfg.MaxSessions, clientHello.IPv6)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "max sessions exceeded") {
			pkglog.Audit(s.logger, zapcore.WarnLevel, "max sessions exceeded",
				zap.String("token_name", tokenName),
				zap.String("remote_addr", r.RemoteAddr),
			)
		} else {
			s.logger.Error("session create", zap.Error(err))
		}
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: "authentication failed"})
		authData, _ := authFrame.Encode()
		_ = wsConn.WriteMessage(authData)
		framing.ReturnBuffer(authData)
		_ = wsConn.Close()
		return
	}

	var sessionCipher *crypto.SessionCipher
	var cryptoSalt []byte
	if len(s.masterKey) > 0 {
		cryptoSalt, err = crypto.GenerateSalt()
		if err != nil {
			s.logger.Error("generate crypto salt", zap.Error(err))
			_ = wsConn.Close()
			return
		}
	}

	mtu := wsCfg.MTU
	if mtu <= 0 {
		mtu = handshake.DefaultMTU
	}
	serverHello, err := handshake.EncodeServerHello(&handshake.ServerHello{
		SessionID:    sess.ID,
		AssignedIP:   assignedIP,
		AssignedIPv6: assignedIPv6,
		MTU:          mtu,
		CryptoSalt:   cryptoSalt,
		GatewayIP:    s.gatewayIP,
	})
	if err != nil {
		s.logger.Error("encode server hello", zap.Error(err))
		_ = wsConn.Close()
		return
	}
	helloData, err := serverHello.Encode()
	if err != nil {
		s.logger.Error("encode hello frame", zap.Error(err))
		_ = wsConn.Close()
		return
	}
	if err := wsConn.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		s.logger.Error("send server hello", zap.Error(err))
		_ = wsConn.Close()
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
			_ = wsConn.Close()
			return
		}
	}

	sessionCtx, sessionCancel := context.WithCancel(r.Context())
	s.sm.SetCancel(sess.ID, sessionCancel)
	sessionStreams := proxy.NewSessionStreams()

	tunSess := tunnel.NewSession(s.tunDev, wsConn, s.sm, sess.ID, tokenName, s.prl, s.bwMgr, s.collectors, s.logger, sessionCipher, sessionStreams)
	if err := tunSess.Run(sessionCtx); err != nil {
		s.logger.Info("session ended",
			zap.String("session", sess.ID),
			zap.String("token", tokenName),
			zap.String("ip", assignedIP.String()),
			zap.Error(err),
		)
	}
	s.collectors.ActiveSessions.Dec()

	sessionStreams.CloseAll()
	s.sm.Remove(sess.ID)
	_ = wsConn.Close()
}
