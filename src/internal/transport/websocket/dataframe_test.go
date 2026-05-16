// @sk-test docs-and-release#T5.1: TestDataFrameRoundTrip — full WS handshake + data frame (AC-008)

package websocket

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

func TestDataFrameRoundTrip(t *testing.T) {
	helloCh := make(chan *handshake.ClientHello, 1)
	dataCh := make(chan []byte, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel", func(w http.ResponseWriter, r *http.Request) {
		conn, err := Accept(w, r, nopLogger)
		if err != nil {
			t.Errorf("server accept: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		raw, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("server read client hello: %v", err)
			return
		}
		var f framing.Frame
		if err := f.Decode(raw); err != nil {
			t.Errorf("server decode frame: %v", err)
			return
		}
		hello, err := handshake.DecodeClientHello(&f)
		if err != nil {
			t.Errorf("server decode client hello: %v", err)
			return
		}
		helloCh <- hello

		var sid [16]byte
		rand.Read(sid[:])
		resp, _ := handshake.EncodeServerHello(&handshake.ServerHello{
			SessionID:  hex.EncodeToString(sid[:]),
			AssignedIP: net.ParseIP("10.10.0.2"),
			MTU:        1400,
		})
		encoded, _ := resp.Encode()
		if err := conn.WriteMessage(encoded); err != nil {
			t.Errorf("server write server hello: %v", err)
			return
		}

		raw, err = conn.ReadMessage()
		if err != nil {
			t.Errorf("server read data frame: %v", err)
			return
		}
		dataCh <- raw

		if err := conn.WriteMessage(raw); err != nil {
			t.Errorf("server echo data frame: %v", err)
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	wsURL := "ws" + srv.URL[len("http"):] + "/tunnel"

	conn, err := Dial(wsURL, nil, nopLogger)
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	helloFrame, _ := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Token:        "test-token",
		MTU:          1400,
	})
	helloData, _ := helloFrame.Encode()
	if err := conn.WriteMessage(helloData); err != nil {
		t.Fatalf("client write hello: %v", err)
	}

	recvHello := <-helloCh
	if recvHello.Token != "test-token" {
		t.Errorf("client hello token = %q, want %q", recvHello.Token, "test-token")
	}
	if recvHello.ProtoVersion != handshake.ProtoVersion {
		t.Errorf("client hello version = %d, want %d", recvHello.ProtoVersion, handshake.ProtoVersion)
	}

	rawResp, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("client read server hello: %v", err)
	}
	var respFrame framing.Frame
	if err := respFrame.Decode(rawResp); err != nil {
		t.Fatalf("client decode response frame: %v", err)
	}
	serverHello, err := handshake.DecodeServerHello(&respFrame)
	if err != nil {
		t.Fatalf("client decode server hello: %v", err)
	}
	if !serverHello.AssignedIP.Equal(net.ParseIP("10.10.0.2")) {
		t.Errorf("assigned IP = %s, want 10.10.0.2", serverHello.AssignedIP)
	}
	if len(serverHello.SessionID) != 32 {
		t.Errorf("session id length = %d, want 32 (hex)", len(serverHello.SessionID))
	}

	payload := []byte("hello kvn-ws data frame test")
	dataFrame := &framing.Frame{
		Type:    framing.FrameTypeData,
		Flags:   framing.FrameFlagNone,
		Payload: payload,
	}
	dataEncoded, _ := dataFrame.Encode()
	if err := conn.WriteMessage(dataEncoded); err != nil {
		t.Fatalf("client write data frame: %v", err)
	}

	recvData := <-dataCh
	var recvFrame framing.Frame
	if err := recvFrame.Decode(recvData); err != nil {
		t.Fatalf("decode echoed data frame: %v", err)
	}
	if recvFrame.Type != framing.FrameTypeData {
		t.Errorf("echoed frame type = %d, want FrameTypeData (%d)", recvFrame.Type, framing.FrameTypeData)
	}
	if !bytes.Equal(recvFrame.Payload, payload) {
		t.Errorf("echoed payload = %q, want %q", recvFrame.Payload, payload)
	}

	echoed, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("client read echo: %v", err)
	}
	var echoFrame framing.Frame
	if err := echoFrame.Decode(echoed); err != nil {
		t.Fatalf("decode echoed frame: %v", err)
	}
	if !bytes.Equal(echoFrame.Payload, payload) {
		t.Errorf("echo payload = %q, want %q", echoFrame.Payload, payload)
	}
}
