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
	ProtoVersion = 0x02
	SessionIDLen = 16
	FlagIPv6     = 0x01
)

// @sk-task core-tunnel-mvp#T3.1: handshake messages (AC-005)
type ClientHello struct {
	ProtoVersion byte
	IPv6         bool
	Token        string
}

// @sk-task ipv6-dual-stack#T2.1: add AssignedIPv6 to handshake (AC-004)
type ServerHello struct {
	SessionID    string
	AssignedIP   net.IP
	AssignedIPv6 net.IP
}

type AuthError struct {
	Reason string
}

func EncodeClientHello(hello *ClientHello) (*framing.Frame, error) {
	tokenBytes := []byte(hello.Token)
	payload := make([]byte, 2+2+len(tokenBytes))
	payload[0] = hello.ProtoVersion
	flags := byte(0)
	if hello.IPv6 {
		flags |= FlagIPv6
	}
	payload[1] = flags
	binary.BigEndian.PutUint16(payload[2:4], uint16(len(tokenBytes)))
	copy(payload[4:], tokenBytes)
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
	if len(data) < 4 {
		return nil, errors.New("client hello too short")
	}
	hello := &ClientHello{
		ProtoVersion: data[0],
		IPv6:         (data[1] & FlagIPv6) != 0,
	}
	tokenLen := binary.BigEndian.Uint16(data[2:4])
	if int(tokenLen) > len(data)-4 {
		return nil, fmt.Errorf("token length %d exceeds payload", tokenLen)
	}
	hello.Token = string(data[4 : 4+tokenLen])
	return hello, nil
}

// @sk-task ipv6-dual-stack#T2.1: length-prefixed ServerHello encoding (AC-004)
func EncodeServerHello(hello *ServerHello) (*framing.Frame, error) {
	sidBytes, err := hex.DecodeString(hello.SessionID)
	if err != nil {
		return nil, fmt.Errorf("decode session id: %w", err)
	}
	if len(sidBytes) != SessionIDLen {
		return nil, fmt.Errorf("session id length %d != %d", len(sidBytes), SessionIDLen)
	}
	ip4 := hello.AssignedIP.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("assigned IP is not IPv4: %s", hello.AssignedIP)
	}
	// Count IPs: at least 1 (IPv4), plus IPv6 if assigned
	count := byte(1)
	if hello.AssignedIPv6 != nil && len(hello.AssignedIPv6) == net.IPv6len {
		count = 2
	}
	v6bytes := hello.AssignedIPv6.To16()
	// Payload: SessionID(16) + Count(1) + [Family(1)+Len(1)+Addr] * Count
	total := SessionIDLen + 1 + 1 + 1 + 4 // SessionID + Count + Family4 + Len4 + Addr4
	if count == 2 {
		total += 1 + 1 + 16 // Family6 + Len6 + Addr6
	}
	payload := make([]byte, total)
	pos := 0
	copy(payload[:SessionIDLen], sidBytes)
	pos += SessionIDLen
	payload[pos] = count
	pos++
	// IPv4
	payload[pos] = 4
	pos++
	payload[pos] = 4
	pos++
	copy(payload[pos:], ip4)
	pos += 4
	// IPv6 (optional)
	if count == 2 {
		payload[pos] = 6
		pos++
		payload[pos] = 16
		pos++
		copy(payload[pos:], v6bytes)
	}
	return &framing.Frame{
		Type:    framing.FrameTypeHello,
		Flags:   framing.FrameFlagNone,
		Payload: payload,
	}, nil
}

// @sk-task ipv6-dual-stack#T2.1: length-prefixed ServerHello decoding (AC-004)
func DecodeServerHello(frame *framing.Frame) (*ServerHello, error) {
	if frame.Type != framing.FrameTypeHello {
		return nil, fmt.Errorf("unexpected frame type %d", frame.Type)
	}
	data := frame.Payload
	if len(data) < SessionIDLen+1+1+1+4 {
		return nil, fmt.Errorf("server hello too short: %d bytes", len(data))
	}
	hello := &ServerHello{
		SessionID: hex.EncodeToString(data[:SessionIDLen]),
	}
	pos := SessionIDLen
	count := data[pos]
	pos++
	for i := byte(0); i < count; i++ {
		if pos+2 > len(data) {
			return nil, fmt.Errorf("server hello truncated at ip %d", i)
		}
		family := data[pos]
		ipLen := int(data[pos+1])
		pos += 2
		if pos+ipLen > len(data) {
			return nil, fmt.Errorf("server hello truncated: ip %d len %d", i, ipLen)
		}
		switch family {
		case 4:
			hello.AssignedIP = net.IP(data[pos : pos+4]).To4()
		case 6:
			ip := make(net.IP, 16)
			copy(ip, data[pos:pos+16])
			hello.AssignedIPv6 = ip
		}
		pos += ipLen
	}
	if hello.AssignedIP == nil {
		return nil, fmt.Errorf("server hello missing IPv4 address")
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
