package webui

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

//go:embed all:frontend/dist
var reactDist embed.FS

// @sk-task kvn-web#T1.2: webui server package (AC-001, AC-002)
type Server struct {
	httpServer *http.Server
	state      *AppState
	configDir  string
	baseCtx    context.Context
}

func New(port int) (*Server, error) {
	userCfgDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	configDir := filepath.Join(userCfgDir, "kvn")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, err
	}

	state := NewAppState()

	s := &Server{
		state:     state,
		configDir: configDir,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config/global", s.handleSaveGlobalConfig)
	mux.HandleFunc("GET /api/servers", s.handleListServers)
	mux.HandleFunc("POST /api/servers", s.handleCreateServer)
	mux.HandleFunc("PUT /api/servers/{name}", s.handleUpdateServer)
	mux.HandleFunc("DELETE /api/servers/{name}", s.handleDeleteServer)
	mux.HandleFunc("POST /api/connect", s.handleConnect)
	mux.HandleFunc("POST /api/disconnect", s.handleDisconnect)
	mux.HandleFunc("GET /api/logs", s.handleLogs)
	mux.HandleFunc("GET /api/platform", s.handlePlatform)
	mux.HandleFunc("POST /api/config/refresh-sources", s.handleRefreshSources)

	subFS, err := fs.Sub(reactDist, "frontend/dist")
	if err != nil {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body><h1>KVN Web UI</h1><p>React build not found. Run npm build in frontend/ first.</p></body></html>"))
		})
	} else {
		fileServer := http.FileServer(http.FS(subFS))
		mux.Handle("GET /", fileServer)
	}

	s.httpServer = &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s, nil
}

// @sk-task fix-critical-leaks#T2.3: WebUI broadcast uses ctx from Serve (AC-006)
func (s *Server) Serve(ctx context.Context) error {
	s.baseCtx = ctx
	go s.state.broadcastLogs(ctx)
	go s.state.broadcastStatus(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// @sk-task kvn-web#T2.5: platform info endpoint (AC-009)
func (s *Server) handlePlatform(w http.ResponseWriter, r *http.Request) {
	transparentSupported := runtime.GOOS == "linux"
	writeJSON(w, http.StatusOK, map[string]any{
		"os":                    runtime.GOOS,
		"transparent_supported": transparentSupported,
	})
}
