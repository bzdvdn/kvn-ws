package handshake

import (
	"encoding/hex"
	"net"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

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
