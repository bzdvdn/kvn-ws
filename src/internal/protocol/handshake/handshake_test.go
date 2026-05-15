package handshake

import (
	"encoding/hex"
	"net"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

// @sk-test core-tunnel-mvp#T5.1: TestHandshakeClientServerHello (AC-005)
// @sk-test ipv6-dual-stack#T4.1: TestClientHelloIPv6Flag (AC-004)
func TestClientHelloIPv6Flag(t *testing.T) {
	original := &ClientHello{
		ProtoVersion: ProtoVersion,
		IPv6:         true,
		Token:        "test-token-123",
	}

	frame, err := EncodeClientHello(original)
	if err != nil {
		t.Fatalf("EncodeClientHello: %v", err)
	}

	decoded, err := DecodeClientHello(frame)
	if err != nil {
		t.Fatalf("DecodeClientHello: %v", err)
	}

	if !decoded.IPv6 {
		t.Error("IPv6 flag not preserved")
	}
	if decoded.Token != original.Token {
		t.Errorf("Token = %s, want %s", decoded.Token, original.Token)
	}
}

// @sk-test core-tunnel-mvp#T5.1: TestHandshakeClientServerHello (AC-005)
func TestClientHelloRoundTrip(t *testing.T) {
	original := &ClientHello{
		ProtoVersion: ProtoVersion,
		Token:        "test-token-123",
	}

	frame, err := EncodeClientHello(original)
	if err != nil {
		t.Fatalf("EncodeClientHello: %v", err)
	}

	if frame.Type != framing.FrameTypeHello {
		t.Errorf("frame type = %d, want %d", frame.Type, framing.FrameTypeHello)
	}

	decoded, err := DecodeClientHello(frame)
	if err != nil {
		t.Fatalf("DecodeClientHello: %v", err)
	}

	if decoded.ProtoVersion != original.ProtoVersion {
		t.Errorf("ProtoVersion = %d, want %d", decoded.ProtoVersion, original.ProtoVersion)
	}
	if decoded.Token != original.Token {
		t.Errorf("Token = %s, want %s", decoded.Token, original.Token)
	}
}

func TestClientHelloEmptyToken(t *testing.T) {
	original := &ClientHello{
		ProtoVersion: ProtoVersion,
		Token:        "",
	}

	frame, err := EncodeClientHello(original)
	if err != nil {
		t.Fatalf("EncodeClientHello: %v", err)
	}

	decoded, err := DecodeClientHello(frame)
	if err != nil {
		t.Fatalf("DecodeClientHello: %v", err)
	}

	if decoded.Token != "" {
		t.Errorf("Token = %s, want empty", decoded.Token)
	}
}

// @sk-test ipv6-dual-stack#T4.1: TestServerHelloIPv6RoundTrip (AC-004)
func TestServerHelloIPv6RoundTrip(t *testing.T) {
	original := &ServerHello{
		SessionID:    "0102030405060708090a0b0c0d0e0f10",
		AssignedIP:   net.ParseIP("10.10.0.5").To4(),
		AssignedIPv6: net.ParseIP("fd00::2").To16(),
	}

	frame, err := EncodeServerHello(original)
	if err != nil {
		t.Fatalf("EncodeServerHello: %v", err)
	}

	decoded, err := DecodeServerHello(frame)
	if err != nil {
		t.Fatalf("DecodeServerHello: %v", err)
	}

	if decoded.SessionID != original.SessionID {
		t.Errorf("SessionID = %s, want %s", decoded.SessionID, original.SessionID)
	}
	if !decoded.AssignedIP.Equal(original.AssignedIP) {
		t.Errorf("AssignedIP = %s, want %s", decoded.AssignedIP, original.AssignedIP)
	}
	if !decoded.AssignedIPv6.Equal(original.AssignedIPv6) {
		t.Errorf("AssignedIPv6 = %s, want %s", decoded.AssignedIPv6, original.AssignedIPv6)
	}
}

func TestServerHelloRoundTrip(t *testing.T) {
	original := &ServerHello{
		SessionID:  "0102030405060708090a0b0c0d0e0f10",
		AssignedIP: net.ParseIP("10.10.0.5").To4(),
	}

	frame, err := EncodeServerHello(original)
	if err != nil {
		t.Fatalf("EncodeServerHello: %v", err)
	}

	if frame.Type != framing.FrameTypeHello {
		t.Errorf("frame type = %d, want %d", frame.Type, framing.FrameTypeHello)
	}

	decoded, err := DecodeServerHello(frame)
	if err != nil {
		t.Fatalf("DecodeServerHello: %v", err)
	}

	if decoded.SessionID != original.SessionID {
		t.Errorf("SessionID = %s, want %s", decoded.SessionID, original.SessionID)
	}
	if !decoded.AssignedIP.Equal(original.AssignedIP) {
		t.Errorf("AssignedIP = %s, want %s", decoded.AssignedIP, original.AssignedIP)
	}
}

func TestServerHelloInvalidSessionID(t *testing.T) {
	sid := hex.EncodeToString([]byte("short"))
	original := &ServerHello{
		SessionID:  sid,
		AssignedIP: net.ParseIP("10.0.0.1").To4(),
	}

	_, err := EncodeServerHello(original)
	if err == nil {
		t.Error("expected error for short session ID")
	}
}

func TestAuthErrorRoundTrip(t *testing.T) {
	original := &AuthError{
		Reason: "invalid token",
	}

	frame, err := EncodeAuthError(original)
	if err != nil {
		t.Fatalf("EncodeAuthError: %v", err)
	}

	if frame.Type != framing.FrameTypeAuth {
		t.Errorf("frame type = %d, want %d", frame.Type, framing.FrameTypeAuth)
	}

	decoded, err := DecodeAuthError(frame)
	if err != nil {
		t.Fatalf("DecodeAuthError: %v", err)
	}

	if decoded.Reason != original.Reason {
		t.Errorf("Reason = %s, want %s", decoded.Reason, original.Reason)
	}
}

func TestDecodeClientHelloWrongType(t *testing.T) {
	f := &framing.Frame{
		Type:    framing.FrameTypeData,
		Flags:   framing.FrameFlagNone,
		Payload: []byte{0x01, 0x00, 0x00},
	}

	_, err := DecodeClientHello(f)
	if err == nil {
		t.Error("expected error for wrong frame type")
	}
}

// @sk-test performance-and-polish#T3.1: TestClientHelloMTU (AC-004)
func TestClientHelloMTU(t *testing.T) {
	original := &ClientHello{
		ProtoVersion: ProtoVersion,
		Token:        "test-token",
		MTU:          1400,
	}

	frame, err := EncodeClientHello(original)
	if err != nil {
		t.Fatalf("EncodeClientHello: %v", err)
	}

	decoded, err := DecodeClientHello(frame)
	if err != nil {
		t.Fatalf("DecodeClientHello: %v", err)
	}

	if decoded.MTU != 1400 {
		t.Errorf("MTU = %d, want 1400", decoded.MTU)
	}
}

// @sk-test performance-and-polish#T3.1: TestServerHelloMTU (AC-004)
func TestServerHelloMTU(t *testing.T) {
	original := &ServerHello{
		SessionID:  "0102030405060708090a0b0c0d0e0f10",
		AssignedIP: net.ParseIP("10.10.0.5").To4(),
		MTU:        1400,
	}

	frame, err := EncodeServerHello(original)
	if err != nil {
		t.Fatalf("EncodeServerHello: %v", err)
	}

	decoded, err := DecodeServerHello(frame)
	if err != nil {
		t.Fatalf("DecodeServerHello: %v", err)
	}

	if decoded.MTU != 1400 {
		t.Errorf("MTU = %d, want 1400", decoded.MTU)
	}
}

// @sk-test performance-and-polish#T3.1: TestClientHelloDefaultMTU (AC-004)
func TestClientHelloDefaultMTU(t *testing.T) {
	original := &ClientHello{
		ProtoVersion: ProtoVersion,
		Token:        "test-token",
	}

	frame, err := EncodeClientHello(original)
	if err != nil {
		t.Fatalf("EncodeClientHello: %v", err)
	}

	decoded, err := DecodeClientHello(frame)
	if err != nil {
		t.Fatalf("DecodeClientHello: %v", err)
	}

	if decoded.MTU != DefaultMTU {
		t.Errorf("MTU = %d, want %d", decoded.MTU, DefaultMTU)
	}
}

func TestDecodeServerHelloTruncated(t *testing.T) {
	f := &framing.Frame{
		Type:    framing.FrameTypeHello,
		Payload: []byte{0x01, 0x02, 0x03},
	}

	_, err := DecodeServerHello(f)
	if err == nil {
		t.Error("expected error for truncated payload")
	}
}

// @sk-test app-crypto#T2: TestServerHelloCryptoSalt (AC-006)
func TestServerHelloCryptoSalt(t *testing.T) {
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = byte(i)
	}
	original := &ServerHello{
		SessionID:  "0102030405060708090a0b0c0d0e0f10",
		AssignedIP: net.ParseIP("10.10.0.5").To4(),
		MTU:        1400,
		CryptoSalt: salt,
	}

	frame, err := EncodeServerHello(original)
	if err != nil {
		t.Fatalf("EncodeServerHello: %v", err)
	}

	decoded, err := DecodeServerHello(frame)
	if err != nil {
		t.Fatalf("DecodeServerHello: %v", err)
	}

	if decoded.SessionID != original.SessionID {
		t.Errorf("SessionID = %s, want %s", decoded.SessionID, original.SessionID)
	}
	if !decoded.AssignedIP.Equal(original.AssignedIP) {
		t.Errorf("AssignedIP = %s, want %s", decoded.AssignedIP, original.AssignedIP)
	}
	if decoded.MTU != 1400 {
		t.Errorf("MTU = %d, want 1400", decoded.MTU)
	}
	if len(decoded.CryptoSalt) != 32 {
		t.Fatalf("CryptoSalt length = %d, want 32", len(decoded.CryptoSalt))
	}
	for i, b := range decoded.CryptoSalt {
		if b != byte(i) {
			t.Errorf("CryptoSalt[%d] = %d, want %d", i, b, i)
		}
	}
}

// @sk-test app-crypto#T2: TestServerHelloCryptoSaltWithoutMTU (AC-006)
func TestServerHelloCryptoSaltWithoutMTU(t *testing.T) {
	salt := make([]byte, 32)
	copy(salt, []byte("this-is-a-32-byte-salt-for-test!!"))
	original := &ServerHello{
		SessionID:  "0102030405060708090a0b0c0d0e0f10",
		AssignedIP: net.ParseIP("10.10.0.5").To4(),
		CryptoSalt: salt,
	}

	frame, err := EncodeServerHello(original)
	if err != nil {
		t.Fatalf("EncodeServerHello: %v", err)
	}

	decoded, err := DecodeServerHello(frame)
	if err != nil {
		t.Fatalf("DecodeServerHello: %v", err)
	}

	// MTU field is written as 0 (explicitly, to anchor crypto TLV), decoder reads 0
	if decoded.MTU != 0 {
		t.Errorf("MTU = %d, want 0 (explicitly written for TLV alignment)", decoded.MTU)
	}
	if len(decoded.CryptoSalt) != 32 {
		t.Fatalf("CryptoSalt length = %d, want 32", len(decoded.CryptoSalt))
	}
}

// @sk-test app-crypto#T2: TestServerHelloBackwardCompatNoSalt (AC-006)
func TestServerHelloBackwardCompatNoSalt(t *testing.T) {
	original := &ServerHello{
		SessionID:  "0102030405060708090a0b0c0d0e0f10",
		AssignedIP: net.ParseIP("10.10.0.5").To4(),
	}

	frame, err := EncodeServerHello(original)
	if err != nil {
		t.Fatalf("EncodeServerHello: %v", err)
	}

	decoded, err := DecodeServerHello(frame)
	if err != nil {
		t.Fatalf("DecodeServerHello: %v", err)
	}

	if len(decoded.CryptoSalt) != 0 {
		t.Errorf("CryptoSalt = %v, want empty", decoded.CryptoSalt)
	}
}
