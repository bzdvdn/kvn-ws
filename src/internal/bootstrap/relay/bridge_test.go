package relay

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
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

// @sk-test relay-padding-parity#T3.3: bridge keeps both hops in the same padding state (AC-005, AC-006)
func TestBridgePaddingNegotiation(t *testing.T) {
	const (
		sessionID = "00112233445566778899aabbccddeeff"
		padSize   = 64
	)

	tests := []struct {
		name       string
		upstreamOK bool
		request    bool
		wantActive bool
	}{
		{name: "negotiated both hops", upstreamOK: true, request: true, wantActive: false},
		{name: "upstream denied", upstreamOK: false, request: true, wantActive: true},
		{name: "client did not request", upstreamOK: true, request: false, wantActive: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstreamProbeCh := make(chan bool, 1)
			upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				conn, err := wst.Accept(w, req, zap.NewNop(), wst.WSConfig{
					PaddingEnabled: true, PaddingSize: padSize, AllowNoPad: tt.upstreamOK,
				})
				if err != nil {
					upstreamProbeCh <- true
					return
				}
				defer func() { _ = conn.Close() }()

				hello, err := conn.ReadMessage()
				if err != nil {
					upstreamProbeCh <- true
					return
				}
				var frame framing.Frame
				if err := frame.Decode(hello); err != nil {
					upstreamProbeCh <- true
					return
				}

				serverHello, err := handshake.EncodeServerHello(&handshake.ServerHello{
					SessionId:  sessionID,
					AssignedIp: net.IPv4(10, 0, 0, 2),
					Mtu:        handshake.DefaultMTU,
				})
				if err != nil {
					upstreamProbeCh <- true
					return
				}
				out, err := serverHello.Encode()
				if err != nil {
					upstreamProbeCh <- true
					return
				}
				if err := conn.WriteMessage(out); err != nil {
					upstreamProbeCh <- true
					return
				}

				upstreamProbeCh <- conn.PaddingActive()
				for {
					msg, err := conn.ReadMessage()
					if err != nil {
						return
					}
					if err := conn.WriteMessage(msg); err != nil {
						return
					}
				}
			}))
			defer upstreamSrv.Close()

			cfg := &config.RelayConfig{
				Server: "ws" + upstreamSrv.URL[len("http"):],
				Relay:  config.RelayTermCfg{WSPaths: []string{"/"}},
				Obfuscation: &config.ObfuscationCfg{
					Padding: &config.PaddingCfg{Enabled: true, Size: padSize},
				},
			}
			r := &Relay{cfg: cfg, logger: zap.NewNop()}

			relaySrv := httptest.NewServer(http.HandlerFunc(r.relayWSHandler))
			defer relaySrv.Close()
			relayURL := "ws" + relaySrv.URL[len("http"):]

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			client, err := wst.DialContext(ctx, relayURL, nil, zap.NewNop(), wst.WSConfig{
				PaddingEnabled: true, PaddingSize: padSize, RequestNoPad: tt.request,
			})
			if err != nil {
				t.Fatalf("DialContext: %v", err)
			}
			defer func() { _ = client.Close() }()

			if got := client.PaddingActive(); got != tt.wantActive {
				t.Errorf("client PaddingActive = %v, want %v", got, tt.wantActive)
			}

			if err := client.WriteMessage(buildTestClientHello("token")); err != nil {
				t.Fatalf("write client hello: %v", err)
			}
			_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
			serverHello, err := client.ReadMessage()
			if err != nil {
				t.Fatalf("read server hello: %v", err)
			}
			var serverFrame framing.Frame
			if err := serverFrame.Decode(serverHello); err != nil {
				t.Fatalf("decode server hello frame: %v", err)
			}
			if _, err := handshake.DecodeServerHello(&serverFrame); err != nil {
				t.Fatalf("decode server hello: %v", err)
			}

			payload := []byte("bridge-echo")
			if err := client.WriteMessage(payload); err != nil {
				t.Fatalf("write echo payload: %v", err)
			}
			echo, err := client.ReadMessage()
			if err != nil {
				t.Fatalf("read echo: %v", err)
			}
			if !bytes.Equal(echo, payload) {
				t.Errorf("echo = %q, want %q", echo, payload)
			}

			select {
			case active := <-upstreamProbeCh:
				if active != tt.wantActive {
					t.Errorf("upstream PaddingActive = %v, want %v", active, tt.wantActive)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("timeout waiting for upstream probe")
			}
		})
	}
}

func readPaddedRaw(conn *wst.WSConn) ([]byte, int, error) {
	_, raw, err := conn.Underlying().ReadMessage()
	if err != nil {
		return nil, 0, err
	}
	if len(raw) < 4 {
		return nil, 0, errShortPaddedFrame
	}
	n := int(binary.BigEndian.Uint32(raw[:4]))
	if n < 0 || n > len(raw)-4 {
		return nil, 0, errShortPaddedFrame
	}
	return raw[4 : 4+n], len(raw), nil
}

var errShortPaddedFrame = errors.New("short padded frame")

// @sk-test relay-padding-parity#T4.1: bridge aligns padding to the configured size on both hops (AC-007)
func TestBridgePaddingSizeAlignment(t *testing.T) {
	const payloadLen = 100

	tests := []struct {
		name       string
		paddingLen int
		wantWire   int
	}{
		{name: "default 512", paddingLen: 0, wantWire: 512},
		{name: "custom 1024", paddingLen: 1024, wantWire: 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wireCh := make(chan int, 1)
			upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				conn, err := wst.Accept(w, req, zap.NewNop(), wst.WSConfig{
					PaddingEnabled: true, PaddingSize: tt.paddingLen,
				})
				if err != nil {
					wireCh <- 0
					return
				}
				defer func() { _ = conn.Close() }()

				hello, _, err := readPaddedRaw(conn)
				if err != nil {
					wireCh <- 0
					return
				}
				var frame framing.Frame
				if err := frame.Decode(hello); err != nil {
					wireCh <- 0
					return
				}
				serverHello, err := handshake.EncodeServerHello(&handshake.ServerHello{
					SessionId:  "00112233445566778899aabbccddeeff",
					AssignedIp: net.IPv4(10, 0, 0, 2),
					Mtu:        handshake.DefaultMTU,
				})
				if err != nil {
					wireCh <- 0
					return
				}
				out, err := serverHello.Encode()
				if err != nil {
					wireCh <- 0
					return
				}
				if err := conn.WriteMessage(out); err != nil {
					wireCh <- 0
					return
				}

				msg, wireLen, err := readPaddedRaw(conn)
				if err != nil {
					wireCh <- 0
					return
				}
				wireCh <- wireLen
				if err := conn.WriteMessage(msg); err != nil {
					return
				}
				for {
					if _, _, err := readPaddedRaw(conn); err != nil {
						return
					}
				}
			}))
			defer upstreamSrv.Close()

			cfg := &config.RelayConfig{
				Server: "ws" + upstreamSrv.URL[len("http"):],
				Relay:  config.RelayTermCfg{WSPaths: []string{"/"}},
				Obfuscation: &config.ObfuscationCfg{
					Padding: &config.PaddingCfg{Enabled: true, Size: tt.paddingLen},
				},
			}
			r := &Relay{cfg: cfg, logger: zap.NewNop()}

			relaySrv := httptest.NewServer(http.HandlerFunc(r.relayWSHandler))
			defer relaySrv.Close()
			relayURL := "ws" + relaySrv.URL[len("http"):]

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			client, err := wst.DialContext(ctx, relayURL, nil, zap.NewNop(), wst.WSConfig{
				PaddingEnabled: true, PaddingSize: tt.paddingLen,
			})
			if err != nil {
				t.Fatalf("DialContext: %v", err)
			}
			defer func() { _ = client.Close() }()

			if err := client.WriteMessage(buildTestClientHello("token")); err != nil {
				t.Fatalf("write client hello: %v", err)
			}
			_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
			if _, err := client.ReadMessage(); err != nil {
				t.Fatalf("read server hello: %v", err)
			}

			payload := bytes.Repeat([]byte{0x5a}, payloadLen)
			if err := client.WriteMessage(payload); err != nil {
				t.Fatalf("write payload: %v", err)
			}
			echo, err := client.ReadMessage()
			if err != nil {
				t.Fatalf("read echo: %v", err)
			}
			if !bytes.Equal(echo, payload) {
				t.Errorf("echo = %q, want %q", echo, payload)
			}

			select {
			case got := <-wireCh:
				if got != tt.wantWire {
					t.Errorf("upstream wire size = %d, want %d", got, tt.wantWire)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("timeout waiting for upstream wire size")
			}
		})
	}
}
