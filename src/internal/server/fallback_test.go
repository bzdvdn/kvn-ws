package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

// @sk-test kvn-v2-transport#T5.1: v2 fallback serves a site and strips auth upstream (AC-003)
func TestFallback(t *testing.T) {
	h := NewFallback(config.FallbackCfg{Title: "Acme"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Acme") {
		t.Fatalf("static fallback status=%d body=%q", rr.Code, rr.Body.String())
	}

	authCh := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCh <- r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "<html>site</html>")
	}))
	defer upstream.Close()
	h = NewFallback(config.FallbackCfg{Upstream: upstream.URL})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "site") {
		t.Fatalf("proxy fallback status=%d body=%q", rr.Code, rr.Body.String())
	}
	if got := <-authCh; got != "" {
		t.Fatalf("upstream received Authorization %q", got)
	}
}
