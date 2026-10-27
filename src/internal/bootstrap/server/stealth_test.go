package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/acl"
	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/ratelimit"
)

func newStealthTestServer(t *testing.T, cfg *config.ServerConfig) *Server {
	t.Helper()
	matcher, err := acl.NewCIDRMatcher(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		cfg:         cfg,
		logger:      zap.NewNop(),
		cidrMatcher: matcher,
		rl:          ratelimit.NewIPRateLimiter(100, 100),
		startTime:   time.Now(),
	}
}

// @sk-test fallback-auth-stealth#T4.1: valid Bearer opens a WS tunnel, invalid gets fallback (AC-001, AC-002)
func TestStealthAuthBeforeUpgrade(t *testing.T) {
	cfg := &config.ServerConfig{
		WSPaths:     []string{"/legacy"},
		StealthPath: "/stealth",
		Auth:        config.ServerAuth{Tokens: []config.TokenCfg{{Name: "t", Secret: "secret"}}},
		Fallback:    config.FallbackCfg{Title: "Acme"},
	}
	s := newStealthTestServer(t, cfg)
	srv := httptest.NewServer(s.buildMux())
	defer srv.Close()
	wsURL := "ws" + srv.URL[len("http"):] + "/stealth"

	// Invalid/missing token: fallback HTML, no upgrade.
	client := &http.Client{}
	req, _ := http.NewRequest(http.MethodGet, "http"+srv.URL[len("http"):]+"/stealth", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("invalid request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Acme") {
		t.Fatalf("fallback status=%d body=%q", resp.StatusCode, body)
	}

	// Valid token: 101, then close without ClientHello; handler exits on read error.
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Authorization": []string{"Bearer secret"}})
	if err != nil {
		t.Fatalf("valid dial: %v (status %v)", err, resp)
	}
	_ = conn.Close()
}

// @sk-test fallback-auth-stealth#T4.1: fallback reverse proxy strips Authorization (AC-004)
func TestFallbackUpstreamStripsAuthorization(t *testing.T) {
	gotAuth := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>site</html>")
	}))
	defer upstream.Close()

	cfg := &config.ServerConfig{Fallback: config.FallbackCfg{Upstream: upstream.URL, Title: "Acme"}}
	s := newStealthTestServer(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	s.handleFallback(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "site") {
		t.Fatalf("fallback status=%d body=%q", rr.Code, rr.Body.String())
	}
	select {
	case auth := <-gotAuth:
		if auth != "" {
			t.Fatalf("upstream received Authorization %q", auth)
		}
	case <-time.After(time.Second):
		t.Fatal("upstream not called")
	}
}

// @sk-test fallback-auth-stealth#T4.1: health/admin hidden from external clients in stealth mode (AC-008)
func TestStealthHidesHealthExternally(t *testing.T) {
	hide := true
	cfg := &config.ServerConfig{StealthPath: "/stealth", StealthHideAdmin: &hide, Fallback: config.FallbackCfg{Title: "Acme"}}
	s := newStealthTestServer(t, cfg)
	mux := s.buildMux()

	external := httptest.NewRequest(http.MethodGet, "/livez", nil)
	external.RemoteAddr = "203.0.113.10:4444"
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, external)
	if rr.Code != http.StatusOK || strings.Contains(rr.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("external health status=%d content-type=%q body=%q", rr.Code, rr.Header().Get("Content-Type"), rr.Body.String())
	}

	loopback := httptest.NewRequest(http.MethodGet, "/livez", nil)
	loopback.RemoteAddr = "127.0.0.1:4444"
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, loopback)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("loopback health status=%d content-type=%q", rr.Code, rr.Header().Get("Content-Type"))
	}
}

// @sk-test fallback-auth-stealth#T4.1: Bearer comparison accepts only exact token (AC-001)
func TestAuthorizedBearer(t *testing.T) {
	s := newStealthTestServer(t, &config.ServerConfig{Auth: config.ServerAuth{Tokens: []config.TokenCfg{{Name: "t", Secret: "secret"}}}})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	if name, ok := s.authorizedBearer(req); !ok || name != "t" {
		t.Fatalf("valid bearer = %q,%v", name, ok)
	}
	req.Header.Set("Authorization", "Bearer nope")
	if _, ok := s.authorizedBearer(req); ok {
		t.Fatal("invalid bearer accepted")
	}
}
