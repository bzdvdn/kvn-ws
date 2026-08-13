package integration_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/tunnel"
)

// @sk-test production-readiness-gap#T3: integration test — full handshake + encrypted data round-trip over WebSocket (AC-001)
// @sk-test production-readiness-gap#T3: trace marker for integration test coverage (AC-001)
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
			SessionId:    strings.Repeat("ab", 16),
			AssignedIp:   net.ParseIP("10.10.0.10").To4(),
			AssignedIpv6: net.ParseIP("fd00::2").To16(),
			Mtu:          1400,
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
			hello.SessionId,
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
		Ipv6:         true,
		Token:        "test-token",
		Mtu:          1400,
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
		serverHello.AssignedIp, serverHello.AssignedIpv6, serverHello.SessionId)

	// Client derives session key
	masterKey := []byte("0123456789abcdef0123456789abcdef")
	sessionCipher, err := crypto.NewSessionCipher(masterKey, serverHello.CryptoSalt, serverHello.SessionId)
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
		SessionId:    hex.EncodeToString([]byte("0123456789abcdef")),
		AssignedIp:   net.ParseIP("10.88.0.10").To4(),
		AssignedIpv6: net.ParseIP("fd00::100").To16(),
		Mtu:          1400,
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

	if decoded.SessionId != hello.SessionId {
		t.Errorf("session id = %s, want %s", decoded.SessionId, hello.SessionId)
	}
	if !decoded.AssignedIp.Equal(hello.AssignedIp) {
		t.Errorf("assigned ip = %s, want %s", decoded.AssignedIp, hello.AssignedIp)
	}
	if !decoded.AssignedIpv6.Equal(hello.AssignedIpv6) {
		t.Errorf("assigned ipv6 = %s, want %s", decoded.AssignedIpv6, hello.AssignedIpv6)
	}
	if decoded.Mtu != hello.Mtu {
		t.Errorf("mtu = %d, want %d", decoded.Mtu, hello.Mtu)
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

func (w *WSConnTest) SetReadDeadline(t time.Time) error  { return nil }
func (w *WSConnTest) SetWriteDeadline(t time.Time) error { return nil }

// queueTun is a mock tun.TunDevice fed from a channel; it records writes.
type queueTun struct {
	mu      sync.Mutex
	feedCh  chan []byte
	written [][]byte
}

func newQueueTun() *queueTun {
	return &queueTun{feedCh: make(chan []byte, 16)}
}

func (q *queueTun) feed(pkt []byte) { q.feedCh <- pkt }

func (q *queueTun) Read(b []byte) (int, error) {
	pkt, ok := <-q.feedCh
	if !ok {
		return 0, io.EOF
	}
	n := copy(b, pkt)
	return n, nil
}

func (q *queueTun) Write(b []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	buf := make([]byte, len(b))
	copy(buf, b)
	q.written = append(q.written, buf)
	return len(b), nil
}

func (q *queueTun) writtenCopy() [][]byte {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([][]byte, len(q.written))
	copy(out, q.written)
	return out
}

func (q *queueTun) Open() error  { return nil }
func (q *queueTun) Close() error { return nil }
func (q *queueTun) SetIP(ip net.IP, m *net.IPNet) error {
	return nil
}
func (q *queueTun) SetMTU(mtu int) error { return nil }
func (q *queueTun) SetGateway(ip net.IP) error {
	return nil
}
func (q *queueTun) RemoveGateway(ip net.IP) error { return nil }
func (q *queueTun) AddExcludeRoute(cidr string, gw net.IP, iface string) error {
	return nil
}
func (q *queueTun) RemoveExcludeRoute(cidr string, gw net.IP, iface string) error {
	return nil
}
func (q *queueTun) CleanupExcludeRoutes()         {}
func (q *queueTun) SetDNS(servers []string) error { return nil }
func (q *queueTun) DisableGSO() error             { return nil }

// ipv4Packet builds a minimal IPv4 packet with the given IP proto field.
func ipv4Packet(proto byte, payload []byte) []byte {
	hdr := make([]byte, 20)
	hdr[0] = 0x45 // IPv4, IHL 5
	hdr[9] = proto
	return append(hdr, payload...)
}

const (
	dualTestMasterKey = "0123456789abcdef0123456789abcdef"
	dualTestValidTok  = "dual-test-token"
	dualTestSession   = "aa11bb22cc33dd44ee55ff6600112233"
)

// dualChannelServer implements the dual-channel handshake described in the spec:
// primary connects with a plain hello; secondary with Channel="secondary" +
// SessionId binds to the matching session (token must match). It records
// decrypted data frames received on each channel and replies on the secondary.
type dualChannelServer struct {
	t            *testing.T
	upgrader     gorillaws.Upgrader
	mu           sync.Mutex
	sessions     map[string]*dualServSession
	primaryGot   chan []byte
	secondaryGot chan []byte
}

type dualServSession struct {
	sid    string
	token  string
	cipher *crypto.SessionCipher
}

func newDualChannelServer(t *testing.T) *dualChannelServer {
	return &dualChannelServer{
		t:            t,
		upgrader:     gorillaws.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		sessions:     make(map[string]*dualServSession),
		primaryGot:   make(chan []byte, 16),
		secondaryGot: make(chan []byte, 16),
	}
}

func (s *dualChannelServer) handler(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
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
	hello, err := handshake.DecodeClientHello(&f)
	if err != nil {
		return
	}

	reject := func(reason string) {
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: reason})
		authData, _ := authFrame.Encode()
		_ = conn.WriteMessage(gorillaws.BinaryMessage, authData)
		framing.ReturnBuffer(authData)
	}

	if hello.Channel == "secondary" {
		s.mu.Lock()
		sess := s.sessions[hello.SessionId]
		s.mu.Unlock()
		if sess == nil {
			reject("session not found")
			return
		}
		if hello.Token != sess.token {
			reject("token mismatch")
			return
		}
		// AC-001: secondary binds by session_id; send back the same session id.
		serverHello, _ := handshake.EncodeServerHello(&handshake.ServerHello{
			SessionId:  sess.sid,
			AssignedIp: net.ParseIP("10.10.0.10").To4(),
		})
		shData, err := serverHello.Encode()
		if err != nil {
			return
		}
		if err := conn.WriteMessage(gorillaws.BinaryMessage, shData); err != nil {
			framing.ReturnBuffer(shData)
			return
		}
		framing.ReturnBuffer(shData)

		// Relay data frames: decrypt and record, then reply with a UDP packet
		// so the client-side secondary loop has a packet to decrypt back.
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var df framing.Frame
			if err := df.Decode(data); err != nil {
				continue
			}
			if df.Type != framing.FrameTypeData {
				continue
			}
			plain, err := sess.cipher.Decrypt(df.Payload)
			if err != nil {
				df.Release()
				continue
			}
			df.Release()
			select {
			case s.secondaryGot <- plain:
			default:
			}
			// AC-003: return UDP answer comes back over the secondary.
			var inner []byte
			if len(plain) > 20 {
				inner = plain[20:]
			}
			reply := ipv4Packet(17 /*UDP*/, append([]byte("reply:"), inner...))
			enc, err := sess.cipher.Encrypt(reply)
			if err != nil {
				continue
			}
			rf := framing.Frame{Type: framing.FrameTypeData, Payload: enc}
			rData, err := rf.Encode()
			if err != nil {
				continue
			}
			_ = conn.WriteMessage(gorillaws.BinaryMessage, rData)
			framing.ReturnBuffer(rData)
		}
	}

	// Primary channel.
	if hello.Token != dualTestValidTok {
		reject("authentication failed")
		return
	}
	salt := bytes.Repeat([]byte{0xbb}, 32)
	cipher, err := crypto.NewSessionCipher([]byte(dualTestMasterKey), salt, dualTestSession)
	if err != nil {
		return
	}
	sess := &dualServSession{sid: dualTestSession, token: hello.Token, cipher: cipher}
	s.mu.Lock()
	s.sessions[dualTestSession] = sess
	s.mu.Unlock()

	serverHello, _ := handshake.EncodeServerHello(&handshake.ServerHello{
		SessionId:  sess.sid,
		AssignedIp: net.ParseIP("10.10.0.10").To4(),
		CryptoSalt: salt,
	})
	shData, err := serverHello.Encode()
	if err != nil {
		return
	}
	if err := conn.WriteMessage(gorillaws.BinaryMessage, shData); err != nil {
		framing.ReturnBuffer(shData)
		return
	}
	framing.ReturnBuffer(shData)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var df framing.Frame
		if err := df.Decode(data); err != nil {
			continue
		}
		if df.Type != framing.FrameTypeData {
			continue
		}
		plain, err := cipher.Decrypt(df.Payload)
		if err != nil {
			df.Release()
			continue
		}
		df.Release()
		select {
		case s.primaryGot <- plain:
		default:
		}
	}
}

// @sk-test dual-ws-channel#T4.1: two-channel round-trip — UDP on secondary, TCP on primary (AC-001/002/003)
func TestTunnelDualChannelRoundtrip(t *testing.T) {
	srv := newDualChannelServer(t)
	server := httptest.NewServer(http.HandlerFunc(srv.handler))
	defer server.Close()

	wsURL := "ws://" + server.Listener.Addr().String() + "/tunnel"
	dialer := gorillaws.Dialer{HandshakeTimeout: 5 * time.Second}

	// Client primary channel.
	primaryConn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial primary: %v", err)
	}
	defer func() { _ = primaryConn.Close() }()
	primaryWS := &WSConnTest{conn: primaryConn}

	primaryHello, _ := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Token:        dualTestValidTok,
	})
	phData, _ := primaryHello.Encode()
	_ = primaryWS.WriteMessage(phData)
	framing.ReturnBuffer(phData)

	resp, err := primaryWS.ReadMessage()
	if err != nil {
		t.Fatalf("primary read server hello: %v", err)
	}
	var pf framing.Frame
	if err := pf.Decode(resp); err != nil {
		t.Fatalf("primary decode hello: %v", err)
	}
	serverHello, err := handshake.DecodeServerHello(&pf)
	if err != nil {
		t.Fatalf("primary decode server hello: %v", err)
	}
	if serverHello.SessionId != dualTestSession {
		t.Fatalf("session id = %q, want %q", serverHello.SessionId, dualTestSession)
	}

	// Client secondary channel bound by session_id.
	secondaryConn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial secondary: %v", err)
	}
	defer func() { _ = secondaryConn.Close() }()
	secondaryWS := &WSConnTest{conn: secondaryConn}

	secHello, _ := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Token:        dualTestValidTok,
		Channel:      "secondary",
		SessionId:    dualTestSession,
	})
	shData, _ := secHello.Encode()
	_ = secondaryWS.WriteMessage(shData)
	framing.ReturnBuffer(shData)

	sresp, err := secondaryWS.ReadMessage()
	if err != nil {
		t.Fatalf("secondary read server hello: %v", err)
	}
	var sf framing.Frame
	if err := sf.Decode(sresp); err != nil {
		t.Fatalf("secondary decode hello: %v", err)
	}
	sServerHello, err := handshake.DecodeServerHello(&sf)
	if err != nil {
		t.Fatalf("secondary decode server hello: %v", err)
	}
	if sServerHello.SessionId != dualTestSession {
		t.Fatalf("secondary session id = %q, want %q", sServerHello.SessionId, dualTestSession)
	}

	cipher, err := crypto.NewSessionCipher([]byte(dualTestMasterKey), serverHello.CryptoSalt, dualTestSession)
	if err != nil {
		t.Fatalf("client cipher: %v", err)
	}

	// Client tunnel session with a fed TUN: TCP must go via primary, UDP via secondary.
	tun := newQueueTun()
	clientSess := tunnel.NewSession(tun, primaryWS, nil, dualTestSession, "", nil, nil, nil,
		zap.NewNop(), cipher, nil, 5*time.Second, 1000, nil, nil, nil)
	clientSess.SetSecondary(secondaryWS)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _ = clientSess.Run(ctx) }()

	tcp := ipv4Packet(6 /*TCP*/, []byte("tcp-payload"))
	udp := ipv4Packet(17 /*UDP*/, []byte("udp-payload"))
	tun.feed(tcp)
	tun.feed(udp)

	// AC-002: TCP frame arrives on primary, UDP frame on secondary.
	gotPrimary := <-srv.primaryGot
	if len(gotPrimary) < 20 || gotPrimary[9] != 6 {
		t.Errorf("primary got proto %v, want TCP", gotPrimary)
	}
	gotSecondary := <-srv.secondaryGot
	if len(gotSecondary) < 20 || gotSecondary[9] != 17 {
		t.Errorf("secondary got proto %v, want UDP", gotSecondary)
	}

	// AC-003: return UDP answer is written back on the secondary to client TUN.
	deadline := time.Now().Add(3 * time.Second)
	var written [][]byte
	for time.Now().Before(deadline) {
		written = tun.writtenCopy()
		if len(written) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(written) == 0 {
		t.Fatal("client TUN did not receive the return UDP reply via secondary")
	}
	reply := written[0]
	if len(reply) < 20 || string(reply[20:]) != "reply:udp-payload" {
		t.Errorf("client TUN reply = %q, want %q", reply[20:], "reply:udp-payload")
	}
}

// @sk-test dual-ws-channel#T4.1: wrong token rejected on secondary; primary keeps working (AC-001/004)
func TestTunnelDualChannelForeignTokenAndPrimarySurvives(t *testing.T) {
	srv := newDualChannelServer(t)
	server := httptest.NewServer(http.HandlerFunc(srv.handler))
	defer server.Close()

	wsURL := "ws://" + server.Listener.Addr().String() + "/tunnel"
	dialer := gorillaws.Dialer{HandshakeTimeout: 5 * time.Second}

	// Primary established first.
	primaryConn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial primary: %v", err)
	}
	defer func() { _ = primaryConn.Close() }()
	primaryWS := &WSConnTest{conn: primaryConn}
	primaryHello, _ := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Token:        dualTestValidTok,
	})
	phData, _ := primaryHello.Encode()
	_ = primaryWS.WriteMessage(phData)
	framing.ReturnBuffer(phData)
	resp, err := primaryWS.ReadMessage()
	if err != nil {
		t.Fatalf("primary read server hello: %v", err)
	}
	var pf framing.Frame
	if err := pf.Decode(resp); err != nil {
		t.Fatalf("primary decode hello: %v", err)
	}
	serverHello, err := handshake.DecodeServerHello(&pf)
	if err != nil {
		t.Fatalf("primary decode server hello: %v", err)
	}

	// AC-001: secondary with a foreign token is rejected.
	foreignConn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial foreign secondary: %v", err)
	}
	defer func() { _ = foreignConn.Close() }()
	foreignWS := &WSConnTest{conn: foreignConn}
	fHello, _ := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Token:        "wrong-token",
		Channel:      "secondary",
		SessionId:    dualTestSession,
	})
	fhData, _ := fHello.Encode()
	_ = foreignWS.WriteMessage(fhData)
	framing.ReturnBuffer(fhData)
	fresp, err := foreignWS.ReadMessage()
	if err != nil {
		t.Fatalf("foreign secondary read: %v", err)
	}
	var ff framing.Frame
	if err := ff.Decode(fresp); err != nil {
		t.Fatalf("foreign decode: %v", err)
	}
	if ff.Type != framing.FrameTypeAuth {
		t.Errorf("foreign secondary got frame %d, want Auth", ff.Type)
	}

	// AC-004: after the failed secondary handshake, primary traffic keeps flowing.
	cipher, err := crypto.NewSessionCipher([]byte(dualTestMasterKey), serverHello.CryptoSalt, dualTestSession)
	if err != nil {
		t.Fatalf("client cipher: %v", err)
	}
	enc, err := cipher.Encrypt(ipv4Packet(17, []byte("udp-after-reject")))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	df := framing.Frame{Type: framing.FrameTypeData, Payload: enc}
	dData, _ := df.Encode()
	if err := primaryWS.WriteMessage(dData); err != nil {
		t.Fatalf("primary write data: %v", err)
	}
	framing.ReturnBuffer(dData)

	select {
	case got := <-srv.primaryGot:
		if len(got) < 20 || got[9] != 17 {
			t.Errorf("primary got proto %v, want UDP", got[9])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("primary session did not survive foreign-token secondary rejection")
	}
}
