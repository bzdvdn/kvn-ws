package transport

import (
	"crypto/tls"

	"github.com/bzdvdn/kvn-ws/src/internal/tlsprofile"
)

// DefaultPath is the browser-like HTTP/2 stream path for 2.0 clients.
const DefaultPath = "/"

// DefaultUDPPath is the browser-like UDP flow path.
const DefaultUDPPath = "/"

// ClientOptions configures the v2 HTTP/2 client surface.
type ClientOptions struct {
	TLS          *tls.Config
	UTLSProfile  string
	UTLSFallback bool
	StreamPath   string
	UDPPath      string
	TargetHeader string
	ClientID     string
	Padding      bool
	PaddingSize  int
}

// DefaultClientOptions returns the v2 defaults.
func DefaultClientOptions(tlsCfg *tls.Config) ClientOptions {
	return ClientOptions{
		TLS:          tlsCfg,
		UTLSProfile:  tlsprofile.Chrome,
		UTLSFallback: true,
		StreamPath:   DefaultPath,
		UDPPath:      DefaultUDPPath,
	}
}
