package server

import (
	"net/http"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

// @sk-test arch-fix-critical-paths#T2.2: isWebSocketRequest rejects non-WS requests (AC-001)
func TestIsWebSocketRequest(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", http.NoBody)
	if isWebSocketRequest(req) {
		t.Error("isWebSocketRequest without upgrade header = true, want false")
	}
	req.Header.Set("Upgrade", "websocket")
	if !isWebSocketRequest(req) {
		t.Error("isWebSocketRequest with upgrade header = false, want true")
	}
	req.Header.Set("Upgrade", "WebSocket")
	if !isWebSocketRequest(req) {
		t.Error("isWebSocketRequest with mixed case = false, want true")
	}
}

// @sk-test arch-fix-critical-paths#T2.2: allowedWSPath matches configured paths (AC-001)
func TestAllowedWSPath(t *testing.T) {
	s := &Server{
		cfg: &config.ServerConfig{
			WSPaths: []string{"/tunnel", "/ws"},
		},
	}
	if !s.allowedWSPath("/tunnel") {
		t.Error("allowedWSPath(/tunnel) = false, want true")
	}
	if s.allowedWSPath("/other") {
		t.Error("allowedWSPath(/other) = true, want false")
	}
}
