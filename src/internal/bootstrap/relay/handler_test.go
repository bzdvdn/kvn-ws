package relay

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/session"
	wst "github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
)

type mockStreamConn struct {
	readData []byte
	readErr  error
	written  []byte
	mu       sync.Mutex
}

func (m *mockStreamConn) ReadMessage() ([]byte, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	return m.readData, nil
}

func (m *mockStreamConn) WriteMessage(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.written = append(m.written, data...)
	return nil
}

func (m *mockStreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockStreamConn) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockStreamConn) Close() error                       { return nil }

type mockTunDev struct{}

func (m *mockTunDev) Open() error                              { return nil }
func (m *mockTunDev) Close() error                             { return nil }
func (m *mockTunDev) Read([]byte) (int, error)                 { return 0, nil }
func (m *mockTunDev) Write([]byte) (int, error)                { return 0, nil }
func (m *mockTunDev) SetIP(ip net.IP, subnet *net.IPNet) error { return nil }
func (m *mockTunDev) SetMTU(mtu int) error                     { return nil }
func (m *mockTunDev) SetGateway(gateway net.IP) error          { return nil }
func (m *mockTunDev) RemoveGateway(gateway net.IP) error       { return nil }
func (m *mockTunDev) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	return nil
}
func (m *mockTunDev) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	return nil
}
func (m *mockTunDev) SetDNS(dnsServers []string) error { return nil }
func (m *mockTunDev) CleanupExcludeRoutes()            {}
func (m *mockTunDev) DisableGSO() error                { return nil }

// Build a minimal client hello frame for testing
func buildTestClientHello(token string) []byte {
	// This is a simplified ClientHello encoded per the protocol
	// ProtoVersion(1) + flags(1) + tokenLen(2) + token(N)
	tokenBytes := []byte(token)
	payload := make([]byte, 4+len(tokenBytes))
	payload[0] = 1 // ProtoVersion
	payload[1] = 0 // flags
	payload[2] = byte(len(tokenBytes) >> 8)
	payload[3] = byte(len(tokenBytes))
	copy(payload[4:], tokenBytes)
	// Wrap in a frame: type(1) + flags(1) + len(2) + payload
	frame := make([]byte, 4+len(payload))
	frame[0] = 1 // FrameTypeHello
	frame[1] = 0 // FrameFlagNone
	frame[2] = byte(len(payload) >> 8)
	frame[3] = byte(len(payload))
	copy(frame[4:], payload)
	return frame
}

func newMockPool() *session.IPPool {
	pool, _ := session.NewIPPool(session.PoolCfg{
		Subnet:     "10.0.0.0/24",
		Gateway:    "10.0.0.1",
		RangeStart: "10.0.0.10",
		RangeEnd:   "10.0.0.20",
	}, zap.NewNop())
	return pool
}

// @sk-test arch-fix-critical-paths#T4.1: handleTerminatorStream creates and removes session (AC-004)
// @sk-test arch-fix-critical-paths#T5.1: handler happy path — valid token + session lifecycle (AC-007)
func TestHandleTerminatorStreamSessionLifecycle(t *testing.T) {
	pool := newMockPool()
	sm := session.NewSessionManager(pool, zap.NewNop())

	r := &Relay{
		cfg: &config.RelayConfig{
			Auth: config.ServerAuth{
				Tokens: []config.TokenCfg{
					{Name: "test-token", Secret: "valid-secret"},
				},
			},
		},
		pool:   pool,
		sm:     sm,
		logger: zap.NewNop(),
		tunDev: &mockTunDev{},
	}

	helloData := buildTestClientHello("valid-secret")
	stream := &mockStreamConn{readData: helloData}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r.handleTerminatorStream(ctx, stream, "127.0.0.1:12345", "tcp")

	// Session should have been removed
	sessions := sm.List()
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions after disconnect, got %d", len(sessions))
	}
}

// @sk-test arch-fix-critical-paths#T5.1: handler rejects invalid token (AC-007)
func TestHandleTerminatorStreamInvalidToken(t *testing.T) {
	pool := newMockPool()
	sm := session.NewSessionManager(pool, zap.NewNop())

	r := &Relay{
		cfg: &config.RelayConfig{
			Auth: config.ServerAuth{
				Tokens: []config.TokenCfg{
					{Name: "test-token", Secret: "valid-secret"},
				},
			},
		},
		pool:   pool,
		sm:     sm,
		logger: zap.NewNop(),
		tunDev: &mockTunDev{},
	}

	helloData := buildTestClientHello("invalid-secret")
	stream := &mockStreamConn{readData: helloData}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r.handleTerminatorStream(ctx, stream, "127.0.0.1:12345", "tcp")

	// No session should have been created
	sessions := sm.List()
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions for invalid token, got %d", len(sessions))
	}
}

// @sk-test relay-padding-parity#T2.2: terminator accepts no-pad only when realtime unpadded is on (AC-002, AC-006)
func TestAcceptTerminatorWSNoPad(t *testing.T) {
	tests := []struct {
		name       string
		realtime   bool
		wantActive bool
	}{
		{name: "negotiated", realtime: true, wantActive: false},
		{name: "realtime disabled", realtime: false, wantActive: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.RelayConfig{Obfuscation: &config.ObfuscationCfg{
				Padding: &config.PaddingCfg{Enabled: true, Size: 64, RealtimeUnpadded: relayBoolPtr(tt.realtime)},
			}}
			r := &Relay{cfg: cfg, logger: zap.NewNop()}

			serverCh := make(chan bool, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				conn, err := r.acceptTerminatorWS(w, req)
				if err != nil {
					t.Errorf("acceptTerminatorWS: %v", err)
					serverCh <- true
					return
				}
				serverCh <- conn.PaddingActive()
				_ = conn.Close()
			}))
			defer srv.Close()

			wsURL := "ws" + srv.URL[len("http"):]
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			client, err := wst.DialContext(ctx, wsURL, nil, zap.NewNop(), wst.WSConfig{
				PaddingEnabled: true, PaddingSize: 64, RequestNoPad: true,
			})
			if err != nil {
				t.Fatalf("DialContext: %v", err)
			}
			defer func() { _ = client.Close() }()

			if got := client.PaddingActive(); got != tt.wantActive {
				t.Errorf("client PaddingActive = %v, want %v", got, tt.wantActive)
			}

			select {
			case got := <-serverCh:
				if got != tt.wantActive {
					t.Errorf("terminator PaddingActive = %v, want %v", got, tt.wantActive)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("timeout waiting for terminator accept")
			}
		})
	}
}
