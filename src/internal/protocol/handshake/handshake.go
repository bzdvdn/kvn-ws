// @sk-task foundation#T1.3: internal stubs (AC-002)

package handshake

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"

	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

const (
	ProtoVersion = 0x01
	SessionIDLen = 16
)

// @sk-task core-tunnel-mvp#T3.1: handshake messages (AC-005)
type ClientHello struct {
	ProtoVersion byte
	Token        string
}

type ServerHello struct {
	SessionID  string
	AssignedIP net.IP
}

type AuthError struct {
	Reason string
}

func EncodeClientHello(hello *ClientHello) (*framing.Frame, error) {
	tokenBytes := []byte(hello.Token)
	payload := make([]byte, 1+2+len(tokenBytes))
	payload[0] = hello.ProtoVersion
	binary.BigEndian.PutUint16(payload[1:3], uint16(len(tokenBytes)))
	copy(payload[3:], tokenBytes)
	return &framing.Frame{
		Type:    framing.FrameTypeHello,
		Flags:   framing.FrameFlagNone,
		Payload: payload,
	}, nil
}

func DecodeClientHello(frame *framing.Frame) (*ClientHello, error) {
	if frame.Type != framing.FrameTypeHello {
		return nil, fmt.Errorf("unexpected frame type %d", frame.Type)
	}
	data := frame.Payload
	if len(data) < 3 {
		return nil, errors.New("client hello too short")
	}
	hello := &ClientHello{
		ProtoVersion: data[0],
	}
	tokenLen := binary.BigEndian.Uint16(data[1:3])
	if int(tokenLen) > len(data)-3 {
		return nil, fmt.Errorf("token length %d exceeds payload", tokenLen)
	}
	hello.Token = string(data[3 : 3+tokenLen])
	return hello, nil
}

func EncodeServerHello(hello *ServerHello) (*framing.Frame, error) {
	ip4 := hello.AssignedIP.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("assigned IP is not IPv4: %s", hello.AssignedIP)
	}
	sidBytes, err := hex.DecodeString(hello.SessionID)
	if err != nil {
		return nil, fmt.Errorf("decode session id: %w", err)
	}
	if len(sidBytes) != SessionIDLen {
		return nil, fmt.Errorf("session id length %d != %d", len(sidBytes), SessionIDLen)
	}
	payload := make([]byte, SessionIDLen+4)
	copy(payload[:SessionIDLen], sidBytes)
	copy(payload[SessionIDLen:], ip4)
	return &framing.Frame{
		Type:    framing.FrameTypeHello,
		Flags:   framing.FrameFlagNone,
		Payload: payload,
	}, nil
}

func DecodeServerHello(frame *framing.Frame) (*ServerHello, error) {
	if frame.Type != framing.FrameTypeHello {
		return nil, fmt.Errorf("unexpected frame type %d", frame.Type)
	}
	data := frame.Payload
	if len(data) < SessionIDLen+4 {
		return nil, fmt.Errorf("server hello too short: %d bytes", len(data))
	}
	hello := &ServerHello{
		SessionID:  hex.EncodeToString(data[:SessionIDLen]),
		AssignedIP: net.IP(data[SessionIDLen : SessionIDLen+4]).To4(),
	}
	return hello, nil
}

func EncodeAuthError(authErr *AuthError) (*framing.Frame, error) {
	return &framing.Frame{
		Type:    framing.FrameTypeAuth,
		Flags:   framing.FrameFlagNone,
		Payload: []byte(authErr.Reason),
	}, nil
}

func DecodeAuthError(frame *framing.Frame) (*AuthError, error) {
	if frame.Type != framing.FrameTypeAuth {
		return nil, fmt.Errorf("unexpected frame type %d", frame.Type)
	}
	return &AuthError{Reason: string(frame.Payload)}, nil
}
