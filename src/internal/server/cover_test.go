package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newCover(t *testing.T) (handler http.Handler, rootDir string) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "index.html"), "<html>cover</html>")
	writeFile(t, filepath.Join(root, "404.html"), "<html>not found page</html>")
	writeFile(t, filepath.Join(root, "robots.txt"), "User-agent: *\n")
	writeFile(t, filepath.Join(root, "media", "index.m3u8"), "#EXTM3U\n")
	writeFile(t, filepath.Join(root, "media", "segment.ts"), strings.Repeat("x", 2048))
	return NewCoverSite(root, "", "nginx"), root
}

func TestCoverSiteServesFiles(t *testing.T) {
	h, _ := newCover(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	cases := []struct {
		path        string
		status      int
		contentType string
	}{
		{"/", http.StatusOK, "text/html"},
		{"/robots.txt", http.StatusOK, "text/plain"},
		{"/media/index.m3u8", http.StatusOK, "application/vnd.apple.mpegurl"},
		{"/media/segment.ts", http.StatusOK, "video/mp2t"},
	}
	for _, tc := range cases {
		resp, err := http.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("%s status = %d, want %d", tc.path, resp.StatusCode, tc.status)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, tc.contentType) {
			t.Fatalf("%s content-type = %q, want %q", tc.path, ct, tc.contentType)
		}
		if resp.Header.Get("Strict-Transport-Security") == "" {
			t.Fatalf("%s missing HSTS", tc.path)
		}
	}
}

func TestCoverSiteRangeAndNotFound(t *testing.T) {
	h, _ := newCover(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/media/segment.ts", http.NoBody)
	req.Header.Set("Range", "bytes=0-9")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", resp.StatusCode)
	}
	if resp.Header.Get("Content-Range") == "" {
		t.Fatal("missing Content-Range")
	}

	for _, path := range []string{"/missing", "/media/", "/../outside.txt"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, resp.StatusCode)
		}
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
			t.Fatalf("%s content-type = %q", path, resp.Header.Get("Content-Type"))
		}
	}
}

func TestCoverSiteRejectsNonGet(t *testing.T) {
	h, _ := newCover(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/", "application/x-www-form-urlencoded", strings.NewReader("x=1"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST status = %d, want 404", resp.StatusCode)
	}
}

func TestCoverSiteNilForInvalidDir(t *testing.T) {
	if NewCoverSite("", "", "") != nil {
		t.Fatal("empty dir must return nil")
	}
	if NewCoverSite(filepath.Join(t.TempDir(), "missing"), "", "") != nil {
		t.Fatal("missing dir must return nil")
	}
}
