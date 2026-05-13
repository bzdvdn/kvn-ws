// @sk-task foundation#T1.3: internal stubs (AC-002)

package framing

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	FrameTypeData  = 0x01
	FrameTypeHello = 0x02
	FrameTypeAuth  = 0x03
	FrameTypeClose = 0x04

	FrameFlagNone = 0x00

	FrameMaxPayloadSize = 65535
	FrameHeaderSize     = 4
)

var ErrPayloadTooLarge = errors.New("payload exceeds max frame size")

// @sk-task core-tunnel-mvp#T1.1: binary frame protocol (AC-004)
type Frame struct {
	Type    byte
	Flags   byte
	Length  uint16
	Payload []byte
}

func (f *Frame) Encode() ([]byte, error) {
	if len(f.Payload) > FrameMaxPayloadSize {
		return nil, fmt.Errorf("%w: %d", ErrPayloadTooLarge, len(f.Payload))
	}
	f.Length = uint16(len(f.Payload))
	buf := make([]byte, FrameHeaderSize+len(f.Payload))
	buf[0] = f.Type
	buf[1] = f.Flags
	binary.BigEndian.PutUint16(buf[2:4], f.Length)
	copy(buf[4:], f.Payload)
	return buf, nil
}

func (f *Frame) Decode(data []byte) error {
	if len(data) < FrameHeaderSize {
		return errors.New("frame too short")
	}
	f.Type = data[0]
	f.Flags = data[1]
	f.Length = binary.BigEndian.Uint16(data[2:4])
	payloadLen := int(f.Length)
	if payloadLen > len(data)-FrameHeaderSize {
		return fmt.Errorf("frame length %d exceeds data size %d", payloadLen, len(data)-FrameHeaderSize)
	}
	f.Payload = make([]byte, payloadLen)
	copy(f.Payload, data[FrameHeaderSize:FrameHeaderSize+payloadLen])
	return nil
}
