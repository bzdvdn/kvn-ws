// @sk-test security-acl#T9: Admin API handler tests (AC-007, AC-008, AC-011)
package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/session"
)

func newTestSM(t *testing.T) *session.SessionManager {
	t.Helper()
	pool, err := session.NewIPPool(session.PoolCfg{
		Subnet:     "10.10.0.0/24",
		Gateway:    "10.10.0.1",
		RangeStart: "10.10.0.10",
		RangeEnd:   "10.10.0.20",
	})
	if err != nil {
		t.Fatalf("NewIPPool: %v", err)
	}
	return session.NewSessionManager(pool)
}

// @sk-test security-acl#AC-007: Admin API — list sessions (GET /admin/sessions)
func TestAdminListSessions(t *testing.T) {
	sm := newTestSM(t)
	cfg := AdminCfg{Enabled: true, Listen: "localhost:0", Token: "admin-tok"}
	srv := NewAdminServer(cfg, sm)

	_, _, err := sm.Create("sess-1", "user1", "10.0.0.1:1234", 0)
	if err != nil {
		t.Fatalf("Create sess-1: %v", err)
	}
	_, _, err = sm.Create("sess-2", "user2", "10.0.0.2:1234", 0)
	if err != nil {
		t.Fatalf("Create sess-2: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/sessions", nil)
	req.Header.Set("X-Admin-Token", "admin-tok")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sessions, ok := resp["sessions"].([]interface{})
	if !ok {
		t.Fatal("sessions not an array")
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
}

// @sk-test security-acl#AC-008: Admin API — delete session (DELETE /admin/sessions/{id})
func TestAdminDeleteSession(t *testing.T) {
	sm := newTestSM(t)
	cfg := AdminCfg{Enabled: true, Listen: "localhost:0", Token: "admin-tok"}
	srv := NewAdminServer(cfg, sm)

	_, _, err := sm.Create("sess-del", "user1", "10.0.0.1:1234", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if sm.Get("sess-del") == nil {
		t.Fatal("session should exist before delete")
	}

	req := httptest.NewRequest("DELETE", "/admin/sessions/sess-del", nil)
	req.Header.Set("X-Admin-Token", "admin-tok")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	if sm.Get("sess-del") != nil {
		t.Error("session still exists after delete")
	}
}

// @sk-test security-acl#AC-011: Admin API — 401 without token
func TestAdminNoAuth(t *testing.T) {
	sm := newTestSM(t)
	cfg := AdminCfg{Enabled: true, Listen: "localhost:0", Token: "admin-tok"}
	srv := NewAdminServer(cfg, sm)

	req := httptest.NewRequest("GET", "/admin/sessions", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

// @sk-test security-acl#AC-008: Admin API — 404 for nonexistent session
func TestAdminDeleteNotFound(t *testing.T) {
	sm := newTestSM(t)
	cfg := AdminCfg{Enabled: true, Listen: "localhost:0", Token: "admin-tok"}
	srv := NewAdminServer(cfg, sm)

	req := httptest.NewRequest("DELETE", "/admin/sessions/nonexistent", nil)
	req.Header.Set("X-Admin-Token", "admin-tok")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// @sk-test security-acl#AC-007: Admin API — empty session list
func TestAdminListEmpty(t *testing.T) {
	sm := newTestSM(t)
	cfg := AdminCfg{Enabled: true, Listen: "localhost:0", Token: "admin-tok"}
	srv := NewAdminServer(cfg, sm)

	req := httptest.NewRequest("GET", "/admin/sessions", nil)
	req.Header.Set("X-Admin-Token", "admin-tok")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	sessions := resp["sessions"].([]interface{})
	if len(sessions) != 0 {
		t.Fatalf("got %d sessions, want 0", len(sessions))
	}
}

// @sk-test security-acl#AC-007: Admin API — session response fields
func TestAdminSessionFields(t *testing.T) {
	sm := newTestSM(t)
	cfg := AdminCfg{Enabled: true, Listen: "localhost:0", Token: "admin-tok"}
	srv := NewAdminServer(cfg, sm)

	_, _, err := sm.Create("sess-fields", "test-user", "10.0.0.1:9999", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/sessions", nil)
	req.Header.Set("X-Admin-Token", "admin-tok")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var resp struct {
		Sessions []sessionResponse `json:"sessions"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("got %d sessions", len(resp.Sessions))
	}
	s := resp.Sessions[0]
	if s.ID != "sess-fields" {
		t.Errorf("ID = %s", s.ID)
	}
	if s.TokenName != "test-user" {
		t.Errorf("TokenName = %s", s.TokenName)
	}
	if s.RemoteAddr != "10.0.0.1:9999" {
		t.Errorf("RemoteAddr = %s", s.RemoteAddr)
	}
	if _, err := time.Parse(time.RFC3339, s.ConnectedAt); err != nil {
		t.Errorf("invalid ConnectedAt: %v", err)
	}
}
