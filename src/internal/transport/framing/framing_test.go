package framing

import (
	"bytes"
	"testing"
)

// @sk-test core-tunnel-mvp#T5.1: TestFrameRoundTrip (AC-004)
func TestFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		frame   Frame
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
