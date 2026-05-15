package framing

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// @sk-test performance-and-polish#T3.2: TestFrameEncodeSegmented (AC-005)
func TestFrameEncodeSegmented(t *testing.T) {
	payload := make([]byte, 3000)
	for i := range payload {
		payload[i] = byte(i)
	}
	f := Frame{
		Type:    FrameTypeData,
		Flags:   FrameFlagNone,
		Payload: payload,
	}

	segments, err := f.EncodeSegmented(1400)
	if err != nil {
		t.Fatalf("EncodeSegmented: %v", err)
	}

	if len(segments) < 2 {
		t.Errorf("expected >=2 segments for 3000 bytes at MTU 1400, got %d", len(segments))
	}

	var assembled []byte
	for i, data := range segments {
		var sf Frame
		if err := sf.Decode(data); err != nil {
			t.Fatalf("segment %d decode: %v", i, err)
		}
		if !sf.IsSegment() {
			t.Errorf("segment %d: expected segment flag", i)
		}
		if i == len(segments)-1 && !sf.IsLastSegment() {
			t.Errorf("segment %d: expected last segment flag", i)
		}
		assembled = append(assembled, sf.Payload...)
	}

	if len(assembled) != 3000 {
		t.Errorf("assembled length = %d, want 3000", len(assembled))
	}
	for i := range assembled {
		if assembled[i] != byte(i) {
			t.Errorf("byte %d mismatch: %d vs %d", i, assembled[i], byte(i))
			break
		}
	}
}

// @sk-test performance-and-polish#T3.2: TestFrameEncodeSegmentedSmallPayload (AC-005)
func TestFrameEncodeSegmentedSmallPayload(t *testing.T) {
	f := Frame{
		Type:    FrameTypeData,
		Flags:   FrameFlagNone,
		Payload: []byte("small payload under mtu"),
	}

	segments, err := f.EncodeSegmented(1400)
	if err != nil {
		t.Fatalf("EncodeSegmented: %v", err)
	}

	if len(segments) != 1 {
		t.Errorf("expected 1 segment for small payload, got %d", len(segments))
	}
}

// @sk-test core-tunnel-mvp#T5.1: TestFrameRoundTrip (AC-004)
func TestFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		frame Frame
	}{
		{
			name: "data frame",
			frame: Frame{
				Type:    FrameTypeData,
				Flags:   FrameFlagNone,
				Payload: []byte{0x45, 0x00, 0x00, 0x3c},
			},
		},
		{
			name: "hello frame",
			frame: Frame{
				Type:    FrameTypeHello,
				Flags:   0x01,
				Payload: []byte("hello data"),
			},
		},
		{
			name: "auth error frame",
			frame: Frame{
				Type:    FrameTypeAuth,
				Flags:   FrameFlagNone,
				Payload: []byte("invalid token"),
			},
		},
		{
			name: "max payload",
			frame: Frame{
				Type:    FrameTypeData,
				Flags:   FrameFlagNone,
				Payload: make([]byte, FrameMaxPayloadSize),
			},
		},
		{
			name: "empty payload",
			frame: Frame{
				Type:    FrameTypeClose,
				Flags:   FrameFlagNone,
				Payload: []byte{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := tt.frame.Encode()
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			if len(encoded) != FrameHeaderSize+len(tt.frame.Payload) {
				t.Errorf("encoded length = %d, want %d", len(encoded), FrameHeaderSize+len(tt.frame.Payload))
			}

			if encoded[0] != tt.frame.Type {
				t.Errorf("Type = %d, want %d", encoded[0], tt.frame.Type)
			}

			var decoded Frame
			if err := decoded.Decode(encoded); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			if decoded.Type != tt.frame.Type {
				t.Errorf("Decoded Type = %d, want %d", decoded.Type, tt.frame.Type)
			}
			if decoded.Flags != tt.frame.Flags {
				t.Errorf("Decoded Flags = %d, want %d", decoded.Flags, tt.frame.Flags)
			}
			if decoded.Length != uint16(len(tt.frame.Payload)) {
				t.Errorf("Decoded Length = %d, want %d", decoded.Length, len(tt.frame.Payload))
			}
			if !bytes.Equal(decoded.Payload, tt.frame.Payload) {
				t.Errorf("Decoded Payload = %v, want %v", decoded.Payload, tt.frame.Payload)
			}
		})
	}
}

func TestFrameEncodePayloadTooLarge(t *testing.T) {
	f := Frame{
		Type:    FrameTypeData,
		Flags:   FrameFlagNone,
		Payload: make([]byte, FrameMaxPayloadSize+1),
	}
	_, err := f.Encode()
	if err == nil {
		t.Error("expected ErrPayloadTooLarge")
	}
}

func TestFrameDecodeTooShort(t *testing.T) {
	var f Frame
	if err := f.Decode([]byte{0x01}); err == nil {
		t.Error("expected error for short frame")
	}
}

func TestFrameDecodeTruncatedPayload(t *testing.T) {
	data := []byte{
		FrameTypeData,
		FrameFlagNone,
		0x00, 0x10,
		0x01, 0x02, 0x03,
	}
	var f Frame
	if err := f.Decode(data); err == nil {
		t.Error("expected error for truncated payload")
	}
}

func benchPayload(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i)
	}
	return p
}

// @sk-test performance-and-polish#T4.1: BenchmarkEncode (AC-001)
func BenchmarkEncode(b *testing.B) {
	payload := benchPayload(1400)
	f := Frame{Type: FrameTypeData, Payload: payload}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		data, err := f.Encode()
		if err != nil {
			b.Fatal(err)
		}
		ReturnBuffer(data)
	}
}

// @sk-test performance-and-polish#T4.1: BenchmarkDecode (AC-001)
func BenchmarkDecode(b *testing.B) {
	payload := benchPayload(1400)
	f := Frame{Type: FrameTypeData, Payload: payload}
	encoded, _ := f.Encode()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var decoded Frame
		if err := decoded.Decode(encoded); err != nil {
			b.Fatal(err)
		}
		decoded.Release()
	}
	ReturnBuffer(encoded)
}

// @sk-test performance-and-polish#T4.1: BenchmarkEncodeDecodeRoundTrip (AC-001)
func BenchmarkEncodeDecodeRoundTrip(b *testing.B) {
	payload := benchPayload(1400)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		f := Frame{Type: FrameTypeData, Payload: payload}
		encoded, err := f.Encode()
		if err != nil {
			b.Fatal(err)
		}
		var decoded Frame
		if err := decoded.Decode(encoded); err != nil {
			b.Fatal(err)
		}
		if len(decoded.Payload) != 1400 {
			b.Fatalf("payload length %d", len(decoded.Payload))
		}
		ReturnBuffer(encoded)
		decoded.Release()
	}
}

// @sk-test performance-and-polish#T4.1: BenchmarkEncodeSegmented (AC-005)
func BenchmarkEncodeSegmented(b *testing.B) {
	payload := benchPayload(9000)
	f := Frame{Type: FrameTypeData, Payload: payload}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		segments, err := f.EncodeSegmented(1400)
		if err != nil {
			b.Fatal(err)
		}
		for _, s := range segments {
			ReturnBuffer(s)
		}
	}
}

func BenchmarkEncodeOld(b *testing.B) {
	payload := benchPayload(1400)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		f := Frame{Type: FrameTypeData, Payload: payload}
		f.Length = uint16(len(f.Payload))
		buf := make([]byte, FrameHeaderSize+len(f.Payload))
		buf[0] = f.Type
		buf[1] = f.Flags
		binary.BigEndian.PutUint16(buf[2:4], f.Length)
		copy(buf[4:], f.Payload)
		_ = buf
	}
}
