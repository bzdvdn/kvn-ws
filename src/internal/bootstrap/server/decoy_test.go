package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// @sk-test stealth-surface-hardening#T3.2: decoy API returns plausible responses (AC-004)
func TestHandleDecoy(t *testing.T) {
	s := &Server{}
	tests := []struct {
		path   string
		status int
	}{
		{path: "/", status: http.StatusOK},
		{path: "/api/v1/health", status: http.StatusOK},
		{path: "/api/v1/events", status: http.StatusUnauthorized},
		{path: "/unknown", status: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()
			s.handleDecoy(rr, req)
			if rr.Code != tt.status {
				t.Fatalf("status = %d, want %d", rr.Code, tt.status)
			}
			if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", ct)
			}
			var body map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid JSON: %v (%s)", err, rr.Body.String())
			}
			if body["error"] == nil && body["status"] == nil {
				t.Fatalf("unexpected decoy body: %v", body)
			}
		})
	}
}
