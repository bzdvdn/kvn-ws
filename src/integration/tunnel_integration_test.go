// @sk-task production-readiness-gap#T3: integration test — full handshake + encrypted data round-trip over WebSocket (AC-001)
// @sk-task production-readiness-gap#T3: trace marker for integration test coverage (AC-001)

package integration_test

import (
	"bytes"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	gorillaws "github.com/gorilla/websocket"
)

// @sk-test production-readiness-gap#T3: full handshake + encrypted data round-trip (AC-001)
func TestTunnelHandshakeAndEncryptedDataRoundtrip(t *testing.T) {
	// Start a WebSocket server
	serverUpgrader := gorillaws.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := serverUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("server upgrade: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Logf("server read: %v", err)
			return
		}

		var f framing.Frame
		if err := f.Decode(msg); err != nil {
			t.Logf("server frame decode: %v", err)
			return
		}

		clientHello, err := handshake.DecodeClientHello(&f)
		if err != nil {
			t.Logf("server decode client hello: %v", err)
			return
		}

		if clientHello.Token != "test-token" {
			t.Errorf("server got token %q, want %q", clientHello.Token, "test-token")
			return
		}

		hello := &handshake.ServerHello{
			SessionID:    strings.Repeat("ab", 16),
			AssignedIP:   net.ParseIP("10.10.0.10").To4(),
			AssignedIPv6: net.ParseIP("fd00::2").To16(),
			MTU:          1400,
			CryptoSalt:   bytes.Repeat([]byte{0xaa}, 32),
		}

		serverHello, err := handshake.EncodeServerHello(hello)
		if err != nil {
			t.Logf("server encode hello: %v", err)
			return
		}
		helloData, err := serverHello.Encode()
		if err != nil {
			t.Logf("server hello encode: %v", err)
			return
		}
		if err := conn.WriteMessage(gorillaws.BinaryMessage, helloData); err != nil {
			t.Logf("server write hello: %v", err)
			return
		}
		framing.ReturnBuffer(helloData)

		// Derive session key
		sessionCipher, err := crypto.NewSessionCipher(
			[]byte("0123456789abcdef0123456789abcdef"), // 32-byte master key
			hello.CryptoSalt,
			hello.SessionID,
		)
		if err != nil {
			t.Logf("server cipher init: %v", err)
			return
		}

		// Read encrypted data from client
		_, msg, err = conn.ReadMessage()
		if err != nil {
			t.Logf("server read data: %v", err)
			return
		}
		if err := f.Decode(msg); err != nil {
			t.Logf("server data frame decode: %v", err)
			return
		}
		if f.Type != framing.FrameTypeData {
			t.Errorf("server got frame type %d, want Data", f.Type)
			return
		}

		decrypted, err := sessionCipher.Decrypt(f.Payload)
		if err != nil {
			t.Logf("server decrypt: %v", err)
			return
		}
		if string(decrypted) != "hello-from-client" {
			t.Errorf("server got decrypted %q, want %q", string(decrypted), "hello-from-client")
			return
		}
		f.Release()

		// Send encrypted response
		encrypted, err := sessionCipher.Encrypt([]byte("hello-from-server"))
		if err != nil {
			t.Logf("server encrypt: %v", err)
			return
		}
		respFrame := framing.Frame{
			Type:    framing.FrameTypeData,
			Flags:   framing.FrameFlagNone,
			Payload: encrypted,
		}
		respData, err := respFrame.Encode()
		if err != nil {
			t.Logf("server encode resp: %v", err)
			return
		}
		if err := conn.WriteMessage(gorillaws.BinaryMessage, respData); err != nil {
			t.Logf("server write resp: %v", err)
			return
		}
		framing.ReturnBuffer(respData)
	}))
	defer server.Close()

	// Convert httptest URL to ws://
	wsURL := "ws://" + server.Listener.Addr().String() + "/tunnel"

	dialer := gorillaws.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	wsConn := &WSConnTest{conn: conn}
	defer func() { _ = conn.Close() }()

	// Client sends ClientHello
	clientHello, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		IPv6:         true,
		Token:        "test-token",
		MTU:          1400,
	})
	if err != nil {
		t.Fatalf("client hello encode: %v", err)
	}
	helloData, err := clientHello.Encode()
	if err != nil {
		t.Fatalf("client hello frame encode: %v", err)
	}
	if err := wsConn.WriteMessage(helloData); err != nil {
		t.Fatalf("client write hello: %v", err)
	}
	framing.ReturnBuffer(helloData)

	// Client receives ServerHello
	resp, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("client read response: %v", err)
	}
	var fResp framing.Frame
	if err := fResp.Decode(resp); err != nil {
		t.Fatalf("client decode response: %v", err)
	}
	if fResp.Type != framing.FrameTypeHello {
		t.Fatalf("client got frame type %d, want Hello", fResp.Type)
	}

	serverHello, err := handshake.DecodeServerHello(&fResp)
	if err != nil {
		t.Fatalf("client decode server hello: %v", err)
	}
	if len(serverHello.CryptoSalt) == 0 {
		t.Fatal("server hello missing crypto salt")
	}
	t.Logf("client assigned IP: %s, IPv6: %v, session: %s",
		serverHello.AssignedIP, serverHello.AssignedIPv6, serverHello.SessionID)

	// Client derives session key
	masterKey := []byte("0123456789abcdef0123456789abcdef")
	sessionCipher, err := crypto.NewSessionCipher(masterKey, serverHello.CryptoSalt, serverHello.SessionID)
	if err != nil {
		t.Fatalf("client cipher init: %v", err)
	}

	// Client sends encrypted data
	encrypted, err := sessionCipher.Encrypt([]byte("hello-from-client"))
	if err != nil {
		t.Fatalf("client encrypt: %v", err)
	}
	dataFrame := framing.Frame{
		Type:    framing.FrameTypeData,
		Flags:   framing.FrameFlagNone,
		Payload: encrypted,
	}
	dataEnc, err := dataFrame.Encode()
	if err != nil {
		t.Fatalf("client data frame encode: %v", err)
	}
	if err := wsConn.WriteMessage(dataEnc); err != nil {
		t.Fatalf("client write data: %v", err)
	}
	framing.ReturnBuffer(dataEnc)

	// Client reads encrypted response
	resp2, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("client read response: %v", err)
	}
	var fResp2 framing.Frame
	if err := fResp2.Decode(resp2); err != nil {
		t.Fatalf("client decode response: %v", err)
	}
	if fResp2.Type != framing.FrameTypeData {
		t.Fatalf("client got frame type %d, want Data", fResp2.Type)
	}

	decrypted, err := sessionCipher.Decrypt(fResp2.Payload)
	if err != nil {
		t.Fatalf("client decrypt: %v", err)
	}
	if string(decrypted) != "hello-from-server" {
		t.Errorf("client got decrypted %q, want %q", string(decrypted), "hello-from-server")
	}
	fResp2.Release()
}

// @sk-test production-readiness-gap#T3: handshake rejects invalid token (AC-001)
func TestTunnelHandshakeRejectsInvalidToken(t *testing.T) {
	serverUpgrader := gorillaws.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := serverUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var f framing.Frame
		if err := f.Decode(msg); err != nil {
			return
		}
		clientHello, err := handshake.DecodeClientHello(&f)
		if err != nil {
			return
		}

		validTokens := map[string]bool{"valid-token": true}
		if !validTokens[clientHello.Token] {
			authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: "authentication failed"})
			authData, _ := authFrame.Encode()
			_ = conn.WriteMessage(gorillaws.BinaryMessage, authData)
			framing.ReturnBuffer(authData)
			return
		}
	}))
	defer server.Close()

	wsURL := "ws://" + server.Listener.Addr().String() + "/tunnel"
	dialer := gorillaws.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	wsConn := &WSConnTest{conn: conn}

	hello, _ := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Token:        "invalid-token",
	})
	helloData, _ := hello.Encode()
	_ = wsConn.WriteMessage(helloData)
	framing.ReturnBuffer(helloData)

	resp, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var fResp framing.Frame
	if err := fResp.Decode(resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if fResp.Type != framing.FrameTypeAuth {
		t.Fatalf("expected Auth frame, got %d", fResp.Type)
	}
	authErr, err := handshake.DecodeAuthError(&fResp)
	if err != nil {
		t.Fatalf("decode auth error: %v", err)
	}
	if authErr.Reason != "authentication failed" {
		t.Errorf("auth reason = %q, want %q", authErr.Reason, "authentication failed")
	}
}

// @sk-test production-readiness-gap#T3: frame max payload boundary (AC-001)
func TestTunnelFrameMaxPayload(t *testing.T) {
	f := framing.Frame{
		Type:    framing.FrameTypeData,
		Flags:   framing.FrameFlagNone,
		Payload: make([]byte, framing.FrameMaxPayloadSize),
	}
	encoded, err := f.Encode()
	if err != nil {
		t.Fatalf("encode max frame: %v", err)
	}

	var decoded framing.Frame
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode max frame: %v", err)
	}
	if decoded.Type != framing.FrameTypeData {
		t.Errorf("type = %d, want %d", decoded.Type, framing.FrameTypeData)
	}
	if len(decoded.Payload) != framing.FrameMaxPayloadSize {
		t.Errorf("payload len = %d, want %d", len(decoded.Payload), framing.FrameMaxPayloadSize)
	}
	decoded.Release()
}

// @sk-test production-readiness-gap#T3: frame segment fragmentation / reassembly (AC-001)
func TestTunnelSegmentFragmentation(t *testing.T) {
	payload := make([]byte, 3000)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	f := framing.Frame{
		Type:    framing.FrameTypeData,
		Flags:   framing.FrameFlagNone,
		Payload: payload,
	}
	segments, err := f.EncodeSegmented(1000)
	if err != nil {
		t.Fatalf("encode segments: %v", err)
	}
	if len(segments) < 2 {
		t.Fatalf("expected multiple segments, got %d", len(segments))
	}

	var reassembled []byte
	for i, seg := range segments {
		var sf framing.Frame
		if err := sf.Decode(seg); err != nil {
			t.Fatalf("decode segment %d: %v", i, err)
		}
		if !sf.IsSegment() && i != len(segments)-1 {
			t.Errorf("segment %d: expected segment flag", i)
		}
		if sf.IsLastSegment() && i != len(segments)-1 {
			t.Errorf("segment %d: unexpected last flag", i)
		}
		reassembled = append(reassembled, sf.Payload...)
		sf.Release()
	}

	if !bytes.Equal(reassembled, payload) {
		t.Error("reassembled payload does not match original")
	}
}

// @sk-test production-readiness-gap#T3: crypto key derivation deterministic test (AC-001)
func TestCryptoKeyDerivationDeterministic(t *testing.T) {
	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = 0x42
	}
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = 0xaa
	}
	sessionID := "abcdef0123456789abcdef0123456789"

	c1, err := crypto.NewSessionCipher(masterKey, salt, sessionID)
	if err != nil {
		t.Fatalf("cipher 1: %v", err)
	}
	c2, err := crypto.NewSessionCipher(masterKey, salt, sessionID)
	if err != nil {
		t.Fatalf("cipher 2: %v", err)
	}

	data := []byte("deterministic-test-data")
	enc1, err := c1.Encrypt(data)
	if err != nil {
		t.Fatalf("encrypt 1: %v", err)
	}
	enc2, err := c2.Encrypt(data)
	if err != nil {
		t.Fatalf("encrypt 2: %v", err)
	}

	// Nonces differ each Encrypt call
	if bytes.Equal(enc1, enc2) {
		t.Error("expected different ciphertexts due to random nonces")
	}

	dec1, err := c2.Decrypt(enc1)
	if err != nil {
		t.Fatalf("cross-decrypt from c2 to c1: %v", err)
	}
	if !bytes.Equal(dec1, data) {
		t.Errorf("cross-decrypt = %q, want %q", string(dec1), string(data))
	}
}

// @sk-test production-readiness-gap#T3: ServerHello full encode/decode round-trip (AC-001)
func TestTLSServerHelloRoundtrip(t *testing.T) {
	hello := &handshake.ServerHello{
		SessionID:    hex.EncodeToString([]byte("0123456789abcdef")),
		AssignedIP:   net.ParseIP("10.88.0.10").To4(),
		AssignedIPv6: net.ParseIP("fd00::100").To16(),
		MTU:          1400,
		CryptoSalt:   bytes.Repeat([]byte{0xbb}, 32),
	}

	frame, err := handshake.EncodeServerHello(hello)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	encoded, err := frame.Encode()
	if err != nil {
		t.Fatalf("frame encode: %v", err)
	}

	var decodedFrame framing.Frame
	if err := decodedFrame.Decode(encoded); err != nil {
		t.Fatalf("frame decode: %v", err)
	}

	decoded, err := handshake.DecodeServerHello(&decodedFrame)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.SessionID != hello.SessionID {
		t.Errorf("session id = %s, want %s", decoded.SessionID, hello.SessionID)
	}
	if !decoded.AssignedIP.Equal(hello.AssignedIP) {
		t.Errorf("assigned ip = %s, want %s", decoded.AssignedIP, hello.AssignedIP)
	}
	if !decoded.AssignedIPv6.Equal(hello.AssignedIPv6) {
		t.Errorf("assigned ipv6 = %s, want %s", decoded.AssignedIPv6, hello.AssignedIPv6)
	}
	if decoded.MTU != hello.MTU {
		t.Errorf("mtu = %d, want %d", decoded.MTU, hello.MTU)
	}
	if !bytes.Equal(decoded.CryptoSalt, hello.CryptoSalt) {
		t.Errorf("crypto salt mismatch: %x vs %x", decoded.CryptoSalt, hello.CryptoSalt)
	}
	decodedFrame.Release()
}

// WSConnTest wraps gorilla/gorillaws.Conn to match gorillaws.WSConn interface subset
type WSConnTest struct {
	conn *gorillaws.Conn
}

func (w *WSConnTest) ReadMessage() ([]byte, error) {
	_, msg, err := w.conn.ReadMessage()
	return msg, err
}

func (w *WSConnTest) WriteMessage(data []byte) error {
	return w.conn.WriteMessage(gorillaws.BinaryMessage, data)
}

func (w *WSConnTest) Close() error {
	return w.conn.Close()
}
