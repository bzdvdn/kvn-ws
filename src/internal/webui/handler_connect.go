package webui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	v2client "github.com/bzdvdn/kvn-ws/src/internal/client"
	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
)

type connectResponse struct {
	Status Status `json:"status"`
}

// @sk-task kvn-web#T2.2: connect/disconnect handlers (AC-003, AC-004)
// @sk-task multi-server#T2.2: use selected server config (AC-006)
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	s.connectMu.Lock()
	defer s.connectMu.Unlock()

	if s.state.Status() == StatusConnected || s.state.Status() == StatusConnecting {
		writeJSON(w, http.StatusConflict, connectResponse{Status: s.state.Status()})
		return
	}

	// @sk-task tun-connect-restart: release any leftover TUN from a previous
	// session (disconnect timeout) before opening a new device, otherwise
	// Open() fails with "tun device busy".
	s.stopStaleClientLocked()

	s.state.setStatus(StatusConnecting)
	writeJSON(w, http.StatusOK, connectResponse{Status: StatusConnecting})

	wcfg, err := s.loadWebUIConfig()
	if err != nil {
		s.state.setStatus(StatusError)
		s.state.PushLog(LogEntry{Line: "load webui config: " + err.Error(), Level: "error"})
		return
	}

	// @sk-task multi-server#T2.2: merge active server config with global (AC-006)
	cfg := wcfg.ClientConfig
	if wcfg.ActiveServer != "" {
		for i := range wcfg.Servers {
			if wcfg.Servers[i].Name == wcfg.ActiveServer {
				cfg = mergeConfig(&cfg, &wcfg.Servers[i].ClientConfig)
				break
			}
		}
	}
	cfg.Protocol = "v2"
	if cfg.Mode == "" {
		cfg.Mode = "tun"
	}
	if cfg.Server == "" {
		s.state.setStatus(StatusError)
		s.state.PushLog(LogEntry{Line: "no server URL configured for active server", Level: "error"})
		return
	}

	// @sk-task kvn-v2-transport#T3.2: kvn-web starts the v2 engine in-process (AC-005)
	if strings.EqualFold(cfg.Protocol, "v2") {
		v2mode := cfg.Mode
		if v2mode == "" {
			v2mode = "tun"
		}
		if v2mode == "tun" && runtime.GOOS != "linux" && runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
			s.state.setStatus(StatusError)
			s.state.PushLog(LogEntry{Line: "v2 TUN mode is not supported on " + runtime.GOOS, Level: "error"})
			return
		}
		if v2mode == "tun" && runtime.GOOS == "linux" && os.Geteuid() != 0 {
			s.state.setStatus(StatusError)
			s.state.PushLog(LogEntry{Line: "v2 TUN mode requires root privileges (run kvn-web with sudo or CAP_NET_ADMIN)", Level: "error"})
			return
		}
		logLevel := cfg.Log.Level
		if logLevel == "" {
			logLevel = "info"
		}
		baseLogger, _, lerr := logger.New(logLevel)
		if lerr != nil {
			s.state.setStatus(StatusError)
			s.state.PushLog(LogEntry{Line: "create v2 logger: " + lerr.Error(), Level: "error"})
			return
		}
		pushLog := s.state.PushLog
		hookLogger := baseLogger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(&uiLogCore{pushLog: pushLog}, core)
		}))
		active := wcfg.ActiveServer
		v2cfg := &config.V2ClientConfig{
			ActiveServer: active,
			ClientID:     cfg.ClientID,
			Servers: []config.V2ServerEntry{{
				Name:     active,
				Endpoint: cfg.Server,
				Token:    cfg.Auth.Token,
			}},
			MTU:           cfg.MTU,
			Mode:          v2mode,
			QuicEnabled:   cfg.QuicEnabled,
			QuicEndpoint:  cfg.QuicEndpoint,
			AutoReconnect: cfg.AutoReconnect,
			ProxyListen:   cfg.ProxyListen,
			ProxyAuth:     cfg.ProxyAuth,
			Routing:       cfg.Routing,
			DNSProxy:      cfg.DNSProxy,
			TLS:           cfg.TLS,
			Reconnect:     cfg.Reconnect,
			Log:           cfg.Log,
		}
		if cfg.UTLS != nil {
			v2cfg.UTLS = *cfg.UTLS
		}
		if cfg.Stealth != nil {
			v2cfg.Stealth = *cfg.Stealth
		}
		v2cfg.ApplyDefaults()
		if err := v2cfg.Validate(); err != nil {
			s.state.setStatus(StatusError)
			s.state.PushLog(LogEntry{Line: "invalid v2 config: " + err.Error(), Level: "error"})
			return
		}
		cl := v2client.New(v2cfg, hookLogger)
		ctx, cancel := context.WithCancel(s.baseCtx)
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			var lastRx, lastTx int64
			lastAt := time.Now()
			for {
				select {
				case <-ctx.Done():
					return
				case now := <-ticker.C:
					m := cl.Metrics()
					dt := now.Sub(lastAt).Seconds()
					rxSpeed, txSpeed := 0.0, 0.0
					if dt > 0 {
						rxSpeed = float64(m.RxBytes-lastRx) * 8 / 1_000_000 / dt
						txSpeed = float64(m.TxBytes-lastTx) * 8 / 1_000_000 / dt
					}
					lastRx, lastTx, lastAt = m.RxBytes, m.TxBytes, now
					s.state.PushMetric(MetricSnapshot{
						TxBytes: m.TxBytes, RxBytes: m.RxBytes, LatencyMS: m.LatencyMS,
						UptimeS: m.UptimeS, TxSpeed: txSpeed, RxSpeed: rxSpeed, Reconnects: m.Reconnects,
					})
				}
			}
		}()
		s.state.SetCancel(cancel)
		s.state.setClient(cl)
		doneCh := make(chan struct{})
		s.state.SetDoneCh(doneCh)
		go func() {
			defer close(doneCh)
			setStatus := func(st Status) {
				if s.state.Client() == cl {
					s.state.setStatus(st)
				}
			}
			defer func() {
				if r := recover(); r != nil {
					setStatus(StatusError)
					s.state.PushLog(LogEntry{Line: "v2 client panic: " + fmt.Sprint(r), Level: "error"})
				}
			}()
			setStatus(StatusConnected)
			s.state.PushLog(LogEntry{Line: "v2 connected to " + cfg.Server, Level: "info"})
			if err := cl.Run(ctx); err != nil {
				setStatus(StatusError)
				s.state.PushLog(LogEntry{Line: "v2 client error: " + err.Error(), Level: "error"})
				return
			}
			setStatus(StatusDisconnected)
		}()
		return
	}
}

// @sk-task multi-server#T2.2: mergeConfig — merge server over global (AC-006)
func mergeConfig(global, server *config.ClientConfig) config.ClientConfig {
	merged := *global
	if server.Server != "" {
		merged.Server = server.Server
	}
	if server.Transport != "" {
		merged.Transport = server.Transport
	}
	if server.Mode != "" {
		merged.Mode = server.Mode
	}
	if server.MTU != 0 {
		merged.MTU = server.MTU
	}
	if server.Auth.Token != "" {
		merged.Auth.Token = server.Auth.Token
	}
	if server.ProxyListen != "" {
		merged.ProxyListen = server.ProxyListen
	}
	if server.MaxMessageSize != 0 {
		merged.MaxMessageSize = server.MaxMessageSize
	}
	if server.TunnelTimeout != 0 {
		merged.TunnelTimeout = server.TunnelTimeout
	}
	if server.ProxyMaxConcurrency != 0 {
		merged.ProxyMaxConcurrency = server.ProxyMaxConcurrency
	}
	if server.Log.Level != "" {
		merged.Log.Level = server.Log.Level
	}
	if server.IPv6 {
		merged.IPv6 = true
	}
	if server.Multiplex {
		merged.Multiplex = true
	}
	if server.MultiChannel {
		merged.MultiChannel = true
	}
	// @sk-task quic-datagrams#T4.3: per-server udp_datagrams overrides global (AC-001)
	if server.UDPDatagrams != nil {
		merged.UDPDatagrams = server.UDPDatagrams
	}
	// @sk-task ws-media-queue-backpressure#T2.2: per-server batching window (AC-001)
	if server.SecondaryBatchHoldMS != nil {
		merged.SecondaryBatchHoldMS = server.SecondaryBatchHoldMS
	}
	if server.Transparent {
		merged.Transparent = true
	}
	if server.AutoReconnect != nil {
		merged.AutoReconnect = server.AutoReconnect
	}
	if server.SystemProxy != nil {
		merged.SystemProxy = server.SystemProxy
	}
	if server.Obfuscation != nil {
		merged.Obfuscation = server.Obfuscation
	}
	// @sk-task stealth-surface-hardening#T4.1: per-server surface settings override global (AC-009, AC-010)
	if len(server.MultiplexSubprotocols) > 0 {
		merged.MultiplexSubprotocols = server.MultiplexSubprotocols
	}
	if server.HeadersProfile != "" {
		merged.HeadersProfile = server.HeadersProfile
	}
	// @sk-task kvn-v2-transport#T3.2: per-server engine and TUN fields (AC-005)
	if server.Protocol != "" {
		merged.Protocol = server.Protocol
	}
	if server.QuicEnabled {
		merged.QuicEnabled = true
	}
	if server.QuicEndpoint != "" {
		merged.QuicEndpoint = server.QuicEndpoint
	}
	// @sk-task fallback-auth-stealth#T3.1: per-server stealth settings override global (AC-006)
	if server.Stealth != nil {
		merged.Stealth = server.Stealth
	}
	if server.KeepaliveJitterPct != 0 {
		merged.KeepaliveJitterPct = server.KeepaliveJitterPct
	}
	if server.ReconnectJitterPct != 0 {
		merged.ReconnectJitterPct = server.ReconnectJitterPct
	}
	if server.TLS.CAFile != "" || server.TLS.ServerName != "" || server.TLS.VerifyMode != "" || len(server.TLS.SNI) > 0 {
		merged.TLS = server.TLS
	}
	if server.Routing != nil {
		merged.Routing = server.Routing
	}
	if merged.Routing != nil {
		if merged.Routing.DefaultRoute == "" {
			merged.Routing.DefaultRoute = "server"
		}
		if len(merged.Routing.ExcludeRanges) == 0 {
			merged.Routing.ExcludeRanges = config.DefaultExcludeRanges
		}
		if merged.Routing.DNSRouting == nil {
			if len(merged.Routing.ExcludeDomains) > 0 || len(merged.Routing.IncludeDomains) > 0 {
				merged.Routing.DNSRouting = &config.DNSRoutingCfg{Enabled: true, TTL: 60}
			} else {
				merged.Routing.DNSRouting = &config.DNSRoutingCfg{Enabled: false, TTL: 60}
			}
		} else if !merged.Routing.DNSRouting.Enabled && (len(merged.Routing.ExcludeDomains) > 0 || len(merged.Routing.IncludeDomains) > 0) {
			merged.Routing.DNSRouting.Enabled = true
		}
	} else {
		merged.Routing = &config.RoutingCfg{
			DefaultRoute:  "server",
			ExcludeRanges: config.DefaultExcludeRanges,
		}
	}
	if server.KillSwitch != nil {
		merged.KillSwitch = server.KillSwitch
	}
	if server.Reconnect != nil {
		merged.Reconnect = server.Reconnect
	}
	if server.ProxyAuth != nil {
		merged.ProxyAuth = server.ProxyAuth
	}
	if server.Crypto.Enabled || server.Crypto.Key != "" {
		merged.Crypto = server.Crypto
	}
	// @sk-task dns-upstreams-list#T3.3: check Upstreams instead of deprecated Upstream (AC-009)
	if server.DNSProxy.Listen != "" || len(server.DNSProxy.Upstreams) > 0 {
		merged.DNSProxy = server.DNSProxy
	}
	if server.Relay != nil {
		merged.Relay = server.Relay
	}
	// @sk-task kvn-web-config-update#T3.2: dedup routing strings in merged config (AC-004, AC-007)
	dedupRoutingStrings(merged.Routing)
	return merged
}

// @sk-task tun-connect-restart: cancel and force-release a client left over
// from a previous session. Cancels the run context, force-closes the TUN device
// so its OS handle is released, and waits (bounded) for the client goroutine to
// exit. Must be called with connectMu held.
func (s *Server) stopStaleClientLocked() {
	cl := s.state.Client()
	cancel := s.state.Cancel()
	doneCh := s.state.DoneCh()
	if cl == nil && cancel == nil && doneCh == nil {
		return
	}

	if cancel != nil {
		cancel()
	}
	if cl != nil {
		cl.StopTun()
	}
	if doneCh != nil {
		select {
		case <-doneCh:
		case <-time.After(3 * time.Second):
			// Keep the client reference so the next Connect can retry the
			// force-release instead of losing track of the held TUN device.
			s.state.PushLog(LogEntry{Line: "previous client shutdown timeout, TUN force-released", Level: "warn"})
			return
		}
	}

	s.state.setClient(nil)
	s.state.SetCancel(nil)
	s.state.SetDoneCh(nil)
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	s.connectMu.Lock()
	defer s.connectMu.Unlock()

	s.stopStaleClientLocked()

	s.state.setStatus(StatusDisconnected)
	s.state.PushLog(LogEntry{Line: "disconnected", Level: "info"})

	writeJSON(w, http.StatusOK, connectResponse{Status: StatusDisconnected})
}
