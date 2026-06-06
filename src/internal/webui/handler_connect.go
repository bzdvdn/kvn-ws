// @sk-task kvn-web#T2.2: connect/disconnect handlers (AC-003, AC-004)
package webui

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
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

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if s.state.Status() == StatusConnected || s.state.Status() == StatusConnecting {
		writeJSON(w, http.StatusConflict, connectResponse{Status: s.state.Status()})
		return
	}

	s.state.setStatus(StatusConnecting)
	writeJSON(w, http.StatusOK, connectResponse{Status: StatusConnecting})

	cfgPath := filepath.Join(s.configDir, "config.yaml")
	cfg, err := config.LoadClientConfig(cfgPath)
	if err != nil {
		s.state.setStatus(StatusError)
		s.state.PushLog(LogEntry{Line: "load config: " + err.Error(), Level: "error"})
		return
	}

	cl, err := client.NewFromConfig(cfg)
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

	if cfg.Mode == "tun" && os.Geteuid() != 0 {
		s.state.setStatus(StatusError)
		s.state.PushLog(LogEntry{Line: "TUN mode requires root privileges (run with sudo or set CAP_NET_ADMIN)", Level: "error"})
		return
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
