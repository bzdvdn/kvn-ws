package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/netip"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	pkglog "github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/auth"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
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
	paddingEnabled := r.cfg.Obfuscation != nil && r.cfg.Obfuscation.Padding != nil && r.cfg.Obfuscation.Padding.Enabled
	if !paddingEnabled && r.cfg.Obfuscation != nil && r.cfg.Obfuscation.Enabled {
		paddingEnabled = true
	}
	wsCfg := websocket.WSConfig{
		PaddingEnabled: paddingEnabled,
		PaddingSize:    paddingSizeOrDefault(r.cfg.Obfuscation),
	}
	wsConn, err := websocket.Accept(w, req, r.logger, wsCfg)
	if err != nil {
		r.logger.Error("terminator ws upgrade", zap.Error(err))
		return
	}
	r.handleTerminatorStream(req.Context(), wsConn, req.RemoteAddr, "tcp")
}

// @sk-task relay-terminator#T2.1: terminator stream handler — handshake + session (AC-001, AC-004)
// @sk-task relay-terminator#T2.3: cleanup at disconnect (AC-006)
// @sk-task relay-terminator#T5.2: transport auto-select from client (AC-004)
// @sk-task arch-fix-critical-paths#T4.1: session via SessionManager.Create (AC-004)
func (r *Relay) handleTerminatorStream(ctx context.Context, stream tunnel.StreamConn, remoteAddr, transportHint string) {
	defer func() { _ = stream.Close() }()

	if r.cfg.Transport == "" && r.clientTransport == "" {
		r.clientTransport = transportHint
		r.logger.Info("upstream transport auto-selected",
			zap.String("transport", transportHint),
			zap.String("remote", remoteAddr),
		)
	}
	if err := r.ensureUpstream(ctx); err != nil {
		r.logger.Error("lazy upstream connect failed", zap.Error(err))
		return
	}

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

	// @sk-task arch-fix-critical-paths#T4.1: create session via SessionManager (AC-004)
	sessionID := relaySessionID(clientHello.Token)
	sess, allocatedIP, allocatedIPv6, err := r.sm.Create(sessionID, tokenCfg.Name, remoteAddr, tokenCfg.MaxSessions, clientHello.Ipv6)
	if err != nil {
		r.logger.Error("session create", zap.Error(err))
		return
	}

	sessionCtx, sessionCancel := context.WithCancel(ctx)
	r.sm.SetCancel(sess.ID, sessionCancel)

	serverHello, err := handshake.EncodeServerHello(&handshake.ServerHello{
		SessionId:    sess.ID,
		AssignedIp:   allocatedIP,
		AssignedIpv6: allocatedIPv6,
		Mtu:          handshake.DefaultMTU,
	})
	if err != nil {
		r.logger.Error("encode server hello", zap.Error(err))
		sessionCancel()
		r.sm.Remove(sess.ID)
		return
	}
	helloData, err := serverHello.Encode()
	if err != nil {
		r.logger.Error("encode hello frame", zap.Error(err))
		sessionCancel()
		r.sm.Remove(sess.ID)
		return
	}
	if err := stream.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		r.logger.Error("send server hello", zap.Error(err))
		sessionCancel()
		r.sm.Remove(sess.ID)
		return
	}
	framing.ReturnBuffer(helloData)

	r.logger.Info("terminator session created",
		zap.String("session", sess.ID),
		zap.String("ip", allocatedIP.String()),
		zap.String("token", tokenCfg.Name),
		zap.String("remote", remoteAddr),
	)

	tunSess := tunnel.NewSession(r.tunDev, stream, r.sm, sess.ID, "terminator",
		nil, nil, nil, r.logger, nil, nil,
		30*time.Second, 1000, allocatedIP, allocatedIPv6, nil)
	tunSess.SetDemux(r.tunDemux)
	if r.ruleSet != nil {
		tunSess.SetOutgoingInterceptor(r.routeOutgoing)
	}
	if err := tunSess.Run(sessionCtx); err != nil {
		r.logger.Info("terminator session ended",
			zap.String("session", sess.ID),
			zap.String("ip", allocatedIP.String()),
			zap.Error(err),
		)
	}

	sessionCancel()
	r.sm.Remove(sess.ID)
}

// @sk-task relay-terminator#T8.8: routeOutgoing — DNS interception with shouldCache (AC-008)
func (r *Relay) routeOutgoing(payload []byte) (bool, error) {
	if len(payload) < 1 {
		return false, nil
	}

	destIP, ok := extractDestIP(payload)
	if !ok {
		return false, nil
	}

	// DNS interception — handle all DNS queries locally
	if r.dnsEnabled && isDNSQuery(payload) {
		domain, domainOk := routing.ParseDNSQuestion(payload)
		if domainOk {
			action := r.ruleSet.MatchDomain(domain)
			r.logger.Debug("dns query", zap.String("domain", domain), zap.Int("action", int(action)))
			if err := r.forwardDNSQuery(payload, action == routing.RouteDirect); err != nil {
				r.logger.Warn("dns forward failed", zap.Error(err))
			}
			return true, nil
		}
	}

	// DNS cache check for direct routes
	if r.dnsEnabled {
		r.dnsCacheMu.RLock()
		_, cached := r.dnsCache[destIP]
		r.dnsCacheMu.RUnlock()
		if cached {
			r.logger.Debug("route=direct (dns-cached)", zap.String("dst", destIP.String()))
			return false, nil
		}
	}

	action := r.ruleSet.Route(destIP)
	switch action {
	case routing.RouteDirect:
		r.logger.Debug("route=direct", zap.String("dst", destIP.String()))
		return false, nil

	case routing.RouteServer:
		us := r.upstream.Load()
		if us == nil || us.isClosed() {
			r.logger.Warn("upstream not available, reconnecting", zap.String("dst", destIP.String()))
			go r.reconnectUpstream()
			return true, nil
		}
		packet := payload
		if r.nat != nil && us.assignedIP != nil {
			assigned, ok := netip.AddrFromSlice(us.assignedIP)
			if ok && assigned.Is4() {
				snatBuf := make([]byte, len(payload))
				copy(snatBuf, payload)
				if r.nat.snat(snatBuf, assigned) {
					packet = snatBuf
				}
			}
		}
		r.logger.Debug("route=upstream", zap.String("dst", destIP.String()))
		if err := us.Send(packet); err != nil {
			r.logger.Warn("upstream send failed, reconnecting", zap.Error(err))
			go r.reconnectUpstream()
		}
		return true, nil

	default:
		return false, nil
	}
}

// @sk-task arch-fix-critical-paths#T4.1: generate session ID for relay (AC-004)
func relaySessionID(token string) string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		copy(buf[:], token)
	}
	return hex.EncodeToString(buf[:])
}
