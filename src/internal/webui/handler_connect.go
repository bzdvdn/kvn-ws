package webui

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/bzdvdn/kvn-ws/src/internal/bootstrap/client"
	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
	metricclient "github.com/bzdvdn/kvn-ws/src/internal/metrics/client"
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
	if cfg.Server == "" {
		s.state.setStatus(StatusError)
		s.state.PushLog(LogEntry{Line: "no server URL configured for active server", Level: "error"})
		return
	}

	// @sk-task kvn-web-redesign#T1.2: start metrics sender with client lifecycle (AC-013)
	collector := s.state.MetricCollector()
	collector.Start()

	cl, err := client.NewFromConfig(&cfg)
	if err != nil {
		s.state.setStatus(StatusError)
		s.state.PushLog(LogEntry{Line: "create client: " + err.Error(), Level: "error"})
		return
	}
	cl.SetMetricCollector(collector)

	baseLogger, _, err := logger.New(cfg.Log.Level)
	if err != nil {
		s.state.setStatus(StatusError)
		s.state.PushLog(LogEntry{Line: "create logger: " + err.Error(), Level: "error"})
		return
	}
	pushLog := s.state.PushLog
	// @sk-task webui-log-level-filter#T1.1: SSE gets all levels, syslog only the configured level (AC-003)
	hookLogger := baseLogger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(&uiLogCore{pushLog: pushLog}, core)
	}))
	cl.SetLogger(hookLogger)

	// @sk-task win-tun#T5.1: unblock TUN mode on Windows (AC-011)
	if cfg.Mode == "tun" {
		if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
			s.state.setStatus(StatusError)
			s.state.PushLog(LogEntry{Line: "TUN mode is not supported on " + runtime.GOOS, Level: "error"})
			return
		}
		if runtime.GOOS == "linux" && os.Geteuid() != 0 {
			s.state.setStatus(StatusError)
			s.state.PushLog(LogEntry{Line: "TUN mode requires root privileges (run with sudo or set CAP_NET_ADMIN)", Level: "error"})
			return
		}
	}

	ctx, cancel := context.WithCancel(s.baseCtx)
	s.state.SetCancel(cancel)
	s.state.setClient(cl)
	doneCh := make(chan struct{})
	s.state.SetDoneCh(doneCh)

	metricCtx, metricCancel := context.WithCancel(ctx)
	metricOut := make(chan metricclient.MetricSnapshot, 32)
	sender := metricclient.NewSender(collector, metricOut, time.Second)
	go sender.Run(metricCtx)

	go func() {
		defer close(doneCh)
		defer metricCancel()

		// Only publish lifecycle status while this client is still the active
		// one, so a late finisher from a previous session cannot overwrite the
		// status of a newer connection.
		setStatus := func(st Status) {
			if s.state.Client() == cl {
				s.state.setStatus(st)
			}
		}

		setStatus(StatusConnected)
		s.state.PushLog(LogEntry{Line: "connected to " + cfg.Server, Level: "info"})

		// forward metrics from sender to state broadcast
		go func() {
			for {
				select {
				case <-metricCtx.Done():
					return
				case m := <-metricOut:
					s.state.PushMetric(m)
				}
			}
		}()

		if err := cl.Run(ctx); err != nil {
			setStatus(StatusError)
			s.state.PushLog(LogEntry{Line: "client error: " + err.Error(), Level: "error"})
			return
		}
		setStatus(StatusDisconnected)
	}()
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
