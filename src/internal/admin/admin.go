// @sk-task security-acl#T8: Admin API server
// @sk-task security-acl#T9: Admin API handlers
package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bzdvdn/kvn-ws/src/internal/session"
)

type AdminServer struct {
	router *chi.Mux
	sm     *session.SessionManager
	cfg    AdminCfg
	srv    *http.Server
}

type AdminCfg struct {
	Enabled bool
	Listen  string
	Token   string
}

const TokenHeader = "X-Admin-Token"

func NewAdminServer(cfg AdminCfg, sm *session.SessionManager) *AdminServer {
	s := &AdminServer{
		router: chi.NewRouter(),
		sm:     sm,
		cfg:    cfg,
	}

	s.router.Use(s.authMiddleware)
	s.router.Get("/admin/sessions", s.listSessions)
	s.router.Delete("/admin/sessions/{id}", s.deleteSession)

	// @sk-task production-readiness-hardening#T3.5: pprof profiles (AC-011)
	s.router.Get("/debug/pprof/", pprof.Index)
	s.router.Get("/debug/pprof/cmdline", pprof.Cmdline)
	s.router.Get("/debug/pprof/profile", pprof.Profile)
	s.router.Get("/debug/pprof/symbol", pprof.Symbol)
	s.router.Get("/debug/pprof/trace", pprof.Trace)
	s.router.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	s.router.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	s.router.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	s.router.Handle("/debug/pprof/block", pprof.Handler("block"))
	s.router.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))

	return s
}

func (s *AdminServer) ListenAndServe() error {
	s.srv = &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s.srv.ListenAndServe()
}

func (s *AdminServer) Shutdown(ctx context.Context) error {
	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}

// @sk-task production-gap#T3.1: shared operational token gate for admin and metrics (AC-005)
func TokenMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" || r.Header.Get(TokenHeader) != token {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *AdminServer) authMiddleware(next http.Handler) http.Handler {
	return TokenMiddleware(s.cfg.Token)(next)
}

type sessionResponse struct {
	ID          string `json:"id"`
	TokenName   string `json:"token_name"`
	RemoteAddr  string `json:"remote_addr"`
	AssignedIP  string `json:"assigned_ip"`
	ConnectedAt string `json:"connected_at"`
}

func (s *AdminServer) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions := s.sm.List()
	resp := make([]sessionResponse, 0, len(sessions))
	for _, sess := range sessions {
		resp = append(resp, sessionResponse{
			ID:          sess.ID,
			TokenName:   sess.TokenName,
			RemoteAddr:  sess.RemoteAddr,
			AssignedIP:  sess.AssignedIP.String(),
			ConnectedAt: sess.ConnectedAt.Format(time.RFC3339),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"sessions": resp})
}

func (s *AdminServer) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, `{"error":"session id required"}`, http.StatusBadRequest)
		return
	}
	sess := s.sm.Get(id)
	if sess == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	s.sm.Remove(id)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "disconnected", "session_id": id})
}
