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
	FlagMTU      = 0x02
	DefaultMTU   = 1500
)

// @sk-task core-tunnel-mvp#T3.1: handshake messages (AC-005)
// @sk-task performance-and-polish#T3.1: add MTU field (AC-004)
type ClientHello struct {
	ProtoVersion byte
	IPv6         bool
	Token        string
	MTU          int
}

// @sk-task ipv6-dual-stack#T2.1: add AssignedIPv6 to handshake (AC-004)
// @sk-task performance-and-polish#T3.1: add MTU field (AC-004)
// @sk-task app-crypto#T2: add CryptoSalt field (AC-006)
type ServerHello struct {
	SessionID    string
	AssignedIP   net.IP
	AssignedIPv6 net.IP
	MTU          int
	CryptoSalt   []byte
}

type AuthError struct {
	Reason string
}

const CryptoTag byte = 0x09

// @sk-task performance-and-polish#T3.1: encode MTU in ClientHello (AC-004)
func EncodeClientHello(hello *ClientHello) (*framing.Frame, error) {
	tokenBytes := []byte(hello.Token)
	flags := byte(0)
	if hello.IPv6 {
		flags |= FlagIPv6
	}
	mtuSize := 0
	if hello.MTU > 0 {
		flags |= FlagMTU
		mtuSize = 2
	}
	payload := make([]byte, 2+2+len(tokenBytes)+mtuSize)
	payload[0] = hello.ProtoVersion
	payload[1] = flags
	binary.BigEndian.PutUint16(payload[2:4], uint16(len(tokenBytes))) // #nosec G115 — bounded by config
	copy(payload[4:], tokenBytes)
	if mtuSize > 0 {
		binary.BigEndian.PutUint16(payload[4+len(tokenBytes):], uint16(hello.MTU)) // #nosec G115 — bounded by config
	}
	return &framing.Frame{
		Type:    framing.FrameTypeHello,
		Flags:   framing.FrameFlagNone,
		Payload: payload,
	}, nil
}

// @sk-task performance-and-polish#T3.1: decode MTU from ClientHello (AC-004)
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
		MTU:          DefaultMTU,
	}
	tokenLen := binary.BigEndian.Uint16(data[2:4])
	if int(tokenLen) > len(data)-4 {
		return nil, fmt.Errorf("token length %d exceeds payload", tokenLen)
	}
	hello.Token = string(data[4 : 4+tokenLen])
	if (data[1] & FlagMTU) != 0 {
		mtuStart := 4 + int(tokenLen)
		if len(data) >= mtuStart+2 {
			hello.MTU = int(binary.BigEndian.Uint16(data[mtuStart:]))
		}
	}
	return hello, nil
}

// @sk-task ipv6-dual-stack#T2.1: length-prefixed ServerHello encoding (AC-004)
// @sk-task performance-and-polish#T3.1: encode MTU in ServerHello (AC-004)
// @sk-task app-crypto#T2: encode CryptoSalt (AC-006)
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
	count := byte(1)
	if hello.AssignedIPv6 != nil && len(hello.AssignedIPv6) == net.IPv6len {
		count = 2
	}
	v6bytes := hello.AssignedIPv6.To16()
	total := SessionIDLen + 1 + 1 + 1 + 4
	if count == 2 {
		total += 1 + 1 + 16
	}
	hasMTU := hello.MTU > 0 || len(hello.CryptoSalt) > 0
	if hasMTU {
		total += 2
	}
	var cryptoLen int
	if len(hello.CryptoSalt) > 0 {
		cryptoLen = 2 + len(hello.CryptoSalt)
		total += cryptoLen
	}
	payload := make([]byte, total)
	pos := 0
	copy(payload[:SessionIDLen], sidBytes)
	pos += SessionIDLen
	payload[pos] = count
	pos++
	payload[pos] = 4
	pos++
	payload[pos] = 4
	pos++
	copy(payload[pos:], ip4)
	pos += 4
	if count == 2 {
		payload[pos] = 6
		pos++
		payload[pos] = 16
		pos++
		copy(payload[pos:], v6bytes)
		pos += 16
	}
	if hasMTU {
		binary.BigEndian.PutUint16(payload[pos:], uint16(hello.MTU)) // #nosec G115 — bounded by config
		pos += 2
	}
	if cryptoLen > 0 {
		payload[pos] = CryptoTag
		payload[pos+1] = byte(len(hello.CryptoSalt)) // #nosec G115 — fixed salt length (32 bytes)
		copy(payload[pos+2:], hello.CryptoSalt)
	}
	return &framing.Frame{
		Type:    framing.FrameTypeHello,
		Flags:   framing.FrameFlagNone,
		Payload: payload,
	}, nil
}

// @sk-task ipv6-dual-stack#T2.1: length-prefixed ServerHello decoding (AC-004)
// @sk-task performance-and-polish#T3.1: decode MTU from ServerHello (AC-004)
// @sk-task app-crypto#T2: decode CryptoSalt (AC-006)
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
		MTU:       DefaultMTU,
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
	if pos+2 <= len(data) {
		hello.MTU = int(binary.BigEndian.Uint16(data[pos:]))
		pos += 2
	}
	for pos < len(data) {
		if pos+2 > len(data) {
			break
		}
		tag := data[pos]
		length := int(data[pos+1])
		pos += 2
		if pos+length > len(data) {
			break
		}
		if tag == CryptoTag {
			salt := make([]byte, length)
			copy(salt, data[pos:pos+length])
			hello.CryptoSalt = salt
		}
		pos += length
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
