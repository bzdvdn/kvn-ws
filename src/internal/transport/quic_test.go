package transport

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestNewQUICClientBoundedOnBlackhole proves an unresponsive UDP endpoint
// cannot stall the client startup indefinitely.
func TestNewQUICClientBoundedOnBlackhole(t *testing.T) {
	sink, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer func() { _ = sink.Close() }()
	go func() {
		buf := make([]byte, 2048)
		for {
			if _, _, rerr := sink.ReadFromUDP(buf); rerr != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tlsCfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h3"}, MinVersion: tls.VersionTLS13}

	start := time.Now()
	_, err = NewQUICClient(ctx, sink.LocalAddr().String(), "token", tlsCfg, zap.NewNop())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("NewQUICClient succeeded against a blackhole endpoint")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("QUIC dial took %v, want bounded by handshake/ctx timeout", elapsed)
	}
}
