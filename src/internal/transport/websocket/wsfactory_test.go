package websocket

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/transport"
)

// @sk-test transport-factory#T3.1: TestWSFactoryDial verifies WSFactory.Dial returns a working StreamConn (AC-002)
func TestWSFactoryDial(t *testing.T) {
	var upgrader websocket.Upgrader
	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(websocket.BinaryMessage, msg)
		_ = conn.Close()
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	logger := zap.NewNop()
	factoryCfg := &transport.FactoryConfig{
		TLS:               nil,
		Logger:            logger,
		KeepaliveInterval: 10 * time.Second,
		KeepaliveTimeout:  30 * time.Second,
	}
	factory := NewWSFactory(factoryCfg)
	ctx := context.Background()

	wsURL := "ws" + server.URL[4:] + "/tunnel"
	conn, err := factory.Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("WSFactory.Dial failed: %v", err)
	}
	defer conn.Close()

	payload := []byte("hello factory")
	if err := conn.WriteMessage(payload); err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}
	resp, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if !bytes.Equal(resp, payload) {
		t.Fatalf("got %q, want %q", resp, payload)
	}
}
