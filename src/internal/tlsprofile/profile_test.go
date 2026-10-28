package tlsprofile

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

func TestProfiles(t *testing.T) {
	for _, name := range DefaultProfiles() {
		if _, err := ProfileID(name); err != nil {
			t.Fatalf("ProfileID(%q): %v", name, err)
		}
	}
	if _, err := ProfileID("opera"); err == nil {
		t.Fatal("unsupported profile accepted")
	}
}

func TestDialAllProfiles(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS13, NextProtos: []string{"h2"}}
	srv.StartTLS()
	defer srv.Close()

	for _, name := range DefaultProfiles() {
		t.Run(name, func(t *testing.T) {
			d, err := New(Config{
				Profile:  name,
				Fallback: true,
				TLS:      &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}, // #nosec G402 — local test
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, err := d.DialContext(ctx, "tcp", srv.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			uconn, ok := conn.(*utls.UConn)
			if !ok {
				t.Fatalf("connection type = %T, want *utls.UConn", conn)
			}
			if got := uconn.ConnectionState().NegotiatedProtocol; got != "h2" {
				t.Fatalf("ALPN = %q", got)
			}
		})
	}
}
