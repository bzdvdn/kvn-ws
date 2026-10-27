package server

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

// @sk-task kvn-v2-transport#T1.3: default fallback site for probes (AC-003)
func NewFallback(cfg config.FallbackCfg) http.Handler {
	if cfg.Upstream != "" {
		if target, err := url.Parse(cfg.Upstream); err == nil {
			proxy := httputil.NewSingleHostReverseProxy(target)
			original := proxy.Director
			proxy.Director = func(req *http.Request) {
				original(req)
				req.Header.Del("Authorization")
			}
			proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
				writeFallbackHTML(w, cfg.Title, http.StatusBadGateway)
			}
			return proxy
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFallbackHTML(w, cfg.Title, http.StatusOK)
	})
}

func writeFallbackHTML(w http.ResponseWriter, title string, status int) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Welcome"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<!doctype html><html><head><title>%s</title></head><body><h1>%s</h1><p>Welcome.</p></body></html>", title, title)
}
