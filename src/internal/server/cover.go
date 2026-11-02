package server

import (
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func init() {
	// Cover sites commonly serve HLS video; Go's mime table is missing some.
	_ = mime.AddExtensionType(".m3u8", "application/vnd.apple.mpegurl")
	_ = mime.AddExtensionType(".m4s", "video/iso.segment")
	_ = mime.AddExtensionType(".ts", "video/mp2t")
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// NewCoverSite returns a static site handler rooted at dir. It serves real
// files with Range support, hides directory listings and returns the configured
// 404 document (or a small HTML 404) for unknown paths.
func NewCoverSite(dir, notFound, serverHeader string) http.Handler {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	return newCoverHandler(os.DirFS(root), notFound, serverHeader)
}

// NewEmbeddedCoverSite returns the built-in streaming site handler. It is used
// when no fallback directory or upstream is configured, so a fresh deployment
// already looks like a real website.
func NewEmbeddedCoverSite(notFound, serverHeader string) http.Handler {
	return newCoverHandler(embeddedCover, notFound, serverHeader)
}

func newCoverHandler(fsys fs.FS, notFound, serverHeader string) http.Handler {
	if notFound == "" {
		notFound = "404.html"
	}
	files := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		if serverHeader != "" {
			header.Set("Server", serverHeader)
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			serveNotFound(w, r, fsys, notFound)
			return
		}
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if rel == "" {
			rel = "."
		}
		if !fs.ValidPath(rel) {
			serveNotFound(w, r, fsys, notFound)
			return
		}
		info, err := fs.Stat(fsys, rel)
		if err != nil {
			serveNotFound(w, r, fsys, notFound)
			return
		}
		if info.IsDir() {
			if _, ierr := fs.Stat(fsys, path.Join(rel, "index.html")); ierr != nil {
				serveNotFound(w, r, fsys, notFound)
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

func serveNotFound(w http.ResponseWriter, r *http.Request, fsys fs.FS, notFound string) {
	rel := strings.TrimPrefix(path.Clean("/"+notFound), "/")
	if fs.ValidPath(rel) {
		if data, err := fs.ReadFile(fsys, rel); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(data)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("<!doctype html><html><head><title>404 Not Found</title></head><body><h1>404 Not Found</h1></body></html>"))
}
