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
)

type connectResponse struct {
	Status Status `json:"status"`
}

// @sk-task kvn-web#T2.2: connect/disconnect handlers (AC-003, AC-004)
// @sk-task multi-server#T2.2: use selected server config (AC-006)
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if s.state.Status() == StatusConnected || s.state.Status() == StatusConnecting {
		writeJSON(w, http.StatusConflict, connectResponse{Status: s.state.Status()})
		return
	}

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

	cl, err := client.NewFromConfig(&cfg)
	if err != nil {
		s.state.setStatus(StatusError)
		s.state.PushLog(LogEntry{Line: "create client: " + err.Error(), Level: "error"})
		return
	}

	baseLogger, _, err := logger.New(cfg.Log.Level)
	if err != nil {
		s.state.setStatus(StatusError)
		s.state.PushLog(LogEntry{Line: "create logger: " + err.Error(), Level: "error"})
		return
	}
	pushLog := s.state.PushLog
	hookLogger := baseLogger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return &uiLogCore{Core: core, pushLog: pushLog}
	}))
	cl.SetLogger(hookLogger)

	if cfg.Mode == "tun" {
		if runtime.GOOS != "linux" {
			s.state.setStatus(StatusError)
			s.state.PushLog(LogEntry{Line: "TUN mode is not supported on " + runtime.GOOS, Level: "error"})
			return
		}
		if os.Geteuid() != 0 {
			s.state.setStatus(StatusError)
			s.state.PushLog(LogEntry{Line: "TUN mode requires root privileges (run with sudo or set CAP_NET_ADMIN)", Level: "error"})
			return
		}
	}

	ctx, cancel := context.WithCancel(s.baseCtx)
	s.state.SetCancel(cancel)
	s.state.setClient(cl)

	go func() {
		s.state.setStatus(StatusConnected)
		s.state.PushLog(LogEntry{Line: "connected to " + cfg.Server, Level: "info"})

		if err := cl.Run(ctx); err != nil {
			s.state.setStatus(StatusError)
			s.state.PushLog(LogEntry{Line: "client error: " + err.Error(), Level: "error"})
			return
		}
		s.state.setStatus(StatusDisconnected)
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
	if server.DNSProxy.Listen != "" || server.DNSProxy.Upstream != "" {
		merged.DNSProxy = server.DNSProxy
	}
	if server.Relay != nil {
		merged.Relay = server.Relay
	}
	return merged
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	cancel := s.state.Cancel()
	if cancel != nil {
		cancel()
	}

	time.Sleep(500 * time.Millisecond)

	s.state.setStatus(StatusDisconnected)
	s.state.setClient(nil)
	s.state.SetCancel(nil)
	s.state.PushLog(LogEntry{Line: "disconnected", Level: "info"})

	writeJSON(w, http.StatusOK, connectResponse{Status: StatusDisconnected})
}
