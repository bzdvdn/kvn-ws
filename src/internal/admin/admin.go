// @sk-task security-acl#T8: Admin API server
// @sk-task security-acl#T9: Admin API handlers
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/session"
	"github.com/go-chi/chi/v5"
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

func NewAdminServer(cfg AdminCfg, sm *session.SessionManager) *AdminServer {
	s := &AdminServer{
		router: chi.NewRouter(),
		sm:     sm,
		cfg:    cfg,
	}

	s.router.Use(s.authMiddleware)
	s.router.Get("/admin/sessions", s.listSessions)
	s.router.Delete("/admin/sessions/{id}", s.deleteSession)

	return s
}

func (s *AdminServer) ListenAndServe() error {
	s.srv = &http.Server{
		Addr:    s.cfg.Listen,
		Handler: s.router,
	}
	return s.srv.ListenAndServe()
}

func (s *AdminServer) Shutdown(ctx context.Context) error {
	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}

func (s *AdminServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Admin-Token") != s.cfg.Token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
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
	json.NewEncoder(w).Encode(map[string]interface{}{"sessions": resp})
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
	fmt.Fprintf(w, `{"status":"disconnected","session_id":"%s"}`, id)
}
