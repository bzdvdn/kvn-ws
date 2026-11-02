package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

func serve(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(method, target, http.NoBody))
	return rr
}

// @sk-test kvn-v2-transport#T5.1: v2 fallback serves a site and strips auth upstream (AC-003)
func TestFallbackEmbeddedSite(t *testing.T) {
	h := NewFallback(&config.FallbackCfg{})

	home := serve(t, h, http.MethodGet, "/")
	if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), "NOVA") {
		t.Fatalf("home status=%d body=%q", home.Code, home.Body.String())
	}

	login := serve(t, h, http.MethodGet, "/login.html")
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), "<form") || !strings.Contains(login.Body.String(), "Sign in") {
		t.Fatalf("login status=%d body=%q", login.Code, login.Body.String())
	}

	missing := serve(t, h, http.MethodGet, "/does-not-exist")
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "404") {
		t.Fatalf("404 status=%d body=%q", missing.Code, missing.Body.String())
	}

	css := serve(t, h, http.MethodGet, "/assets/site.css")
	if css.Code != http.StatusOK || !strings.HasPrefix(css.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("css status=%d type=%q", css.Code, css.Header().Get("Content-Type"))
	}
}

func TestFallbackDirOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>custom cover</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewFallback(&config.FallbackCfg{Dir: dir})
	rr := serve(t, h, http.MethodGet, "/")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "custom cover") {
		t.Fatalf("dir override status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestFallbackUpstreamStripsAuth(t *testing.T) {
	authCh := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCh <- r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "<html>site</html>")
	}))
	defer upstream.Close()
	h := NewFallback(&config.FallbackCfg{Upstream: upstream.URL})
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "site") {
		t.Fatalf("proxy fallback status=%d body=%q", rr.Code, rr.Body.String())
	}
	if got := <-authCh; got != "" {
		t.Fatalf("upstream received Authorization %q", got)
	}
}
