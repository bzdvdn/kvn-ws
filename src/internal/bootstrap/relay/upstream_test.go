package relay

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	wst "github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
)

type upstreamProbe struct {
	paddingActive bool
	err           error
}

// @sk-test relay-padding-parity#T2.3: upstream WS dial negotiates no-pad per relay config (AC-003, AC-004)
func TestDialAndHandshakeWSNoPad(t *testing.T) {
	const sessionID = "00112233445566778899aabbccddeeff"

	tests := []struct {
		name       string
		allowNoPad bool
		realtime   bool
		wantActive bool
	}{
		{name: "negotiated", allowNoPad: true, realtime: true, wantActive: false},
		{name: "upstream denied", allowNoPad: false, realtime: true, wantActive: true},
		{name: "not requested", allowNoPad: true, realtime: false, wantActive: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probeCh := make(chan upstreamProbe, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				conn, err := wst.Accept(w, req, zap.NewNop(), wst.WSConfig{
					PaddingEnabled: true, PaddingSize: 64, AllowNoPad: tt.allowNoPad,
				})
				if err != nil {
					probeCh <- upstreamProbe{err: err}
					return
				}
				defer func() { _ = conn.Close() }()

				data, err := conn.ReadMessage()
				if err != nil {
					probeCh <- upstreamProbe{err: err}
					return
				}
				var frame framing.Frame
				if err := frame.Decode(data); err != nil {
					probeCh <- upstreamProbe{err: err}
					return
				}
				if _, err := handshake.DecodeClientHello(&frame); err != nil {
					probeCh <- upstreamProbe{err: err}
					return
				}

				hello, err := handshake.EncodeServerHello(&handshake.ServerHello{
					SessionId:  sessionID,
					AssignedIp: net.IPv4(10, 0, 0, 2),
					Mtu:        handshake.DefaultMTU,
				})
				if err != nil {
					probeCh <- upstreamProbe{err: err}
					return
				}
				out, err := hello.Encode()
				if err != nil {
					probeCh <- upstreamProbe{err: err}
					return
				}
				if err := conn.WriteMessage(out); err != nil {
					probeCh <- upstreamProbe{err: err}
					return
				}

				probeCh <- upstreamProbe{paddingActive: conn.PaddingActive()}
				for {
					if _, err := conn.ReadMessage(); err != nil {
						return
					}
				}
			}))
			defer srv.Close()

			cfg := &config.RelayConfig{
				Server:        "ws" + srv.URL[len("http"):],
				UpstreamToken: "test-token",
				Obfuscation: &config.ObfuscationCfg{
					Padding: &config.PaddingCfg{Enabled: true, Size: 64, RealtimeUnpadded: relayBoolPtr(tt.realtime)},
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			conn, assignedIP, err := dialAndHandshakeWS(ctx, cfg, nil, zap.NewNop())
			if err != nil {
				t.Fatalf("dialAndHandshakeWS: %v", err)
			}
			defer func() { _ = conn.Close() }()

			if !assignedIP.Equal(net.IPv4(10, 0, 0, 2)) {
				t.Errorf("assigned IP = %v, want 10.0.0.2", assignedIP)
			}
			state, ok := conn.(interface{ PaddingActive() bool })
			if !ok {
				t.Fatal("upstream conn does not expose PaddingActive")
			}
			if got := state.PaddingActive(); got != tt.wantActive {
				t.Errorf("relay upstream PaddingActive = %v, want %v", got, tt.wantActive)
			}

			select {
			case probe := <-probeCh:
				if probe.err != nil {
					t.Fatalf("fake upstream: %v", probe.err)
				}
				if probe.paddingActive != tt.wantActive {
					t.Errorf("upstream PaddingActive = %v, want %v", probe.paddingActive, tt.wantActive)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("timeout waiting for upstream probe")
			}
		})
	}
}
