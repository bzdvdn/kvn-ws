package framing

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

var bufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, FrameHeaderSize+1500)
		return &b
	},
}

func getBuffer(size int) []byte {
	bufPtr, ok := bufferPool.Get().(*[]byte)
	if !ok {
		buf := make([]byte, size)
		return buf
	}
	buf := *bufPtr
	if cap(buf) < size {
		buf = make([]byte, size)
	}
	buf = buf[:size]
	return buf
}

func ReturnBuffer(buf []byte) {
	bufferPool.Put(&buf)
}

// @sk-task fix-critical-leaks#T5.1: export GetBuffer from existing pool (AC-013)
func GetBuffer(size int) []byte {
	return getBuffer(size)
}

// @sk-task performance-and-polish#T2.1: Release returns Payload to pool (AC-001)
func (f *Frame) Release() {
	if f.Payload == nil {
		return
	}
	saved := f.Payload
	f.Payload = nil
	bufferPool.Put(&saved)
}

// @sk-task performance-and-polish#T2.1: sync.Pool for Encode buffer (AC-001)
func (f *Frame) Encode() ([]byte, error) {
	if len(f.Payload) > FrameMaxPayloadSize {
		return nil, fmt.Errorf("%w: %d", ErrPayloadTooLarge, len(f.Payload))
	}
	f.Length = uint16(len(f.Payload)) // #nosec G115 — bounded by FrameMaxPayloadSize check above
	totalLen := FrameHeaderSize + len(f.Payload)
	buf := getBuffer(totalLen)
	buf[0] = f.Type
	buf[1] = f.Flags
	binary.BigEndian.PutUint16(buf[2:4], f.Length)
	copy(buf[4:], f.Payload)
	return buf, nil
}

// @sk-task performance-and-polish#T2.1: sync.Pool for Decode payload (AC-001)
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
	f.Payload = getBuffer(payloadLen)
	copy(f.Payload, data[FrameHeaderSize:FrameHeaderSize+payloadLen])
	return nil
}

// @sk-task performance-and-polish#T3.2: segment large payload by MTU (AC-005)
func (f *Frame) EncodeSegmented(mtu int) ([][]byte, error) {
	if mtu <= 0 {
		mtu = DefaultPMTU
	}
	if len(f.Payload) <= mtu {
		data, err := f.Encode()
		if err != nil {
			return nil, err
		}
		return [][]byte{data}, nil
	}
	var segments [][]byte
	remaining := f.Payload
	for len(remaining) > 0 {
		chunkSize := mtu
		if chunkSize > len(remaining) {
			chunkSize = len(remaining)
		}
		seg := &Frame{
			Type:    f.Type,
			Flags:   FrameFlagSegment,
			Payload: remaining[:chunkSize],
		}
		remaining = remaining[chunkSize:]
		if len(remaining) == 0 {
			seg.Flags |= FrameFlagSegmentLast
		}
		data, err := seg.Encode()
		if err != nil {
			return nil, err
		}
		segments = append(segments, data)
	}
	return segments, nil
}

func (f *Frame) IsSegment() bool {
	return f.Flags&FrameFlagSegment != 0
}

func (f *Frame) IsLastSegment() bool {
	return f.Flags&FrameFlagSegmentLast != 0
}

const DefaultPMTU = 1500
