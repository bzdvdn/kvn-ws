// Code generated from protocol/frames.yaml. DO NOT EDIT.
package framing

import "errors"

// @sk-task kvn-android#T1.1: protocol frame types (AC-004)
const (
	FrameTypeAuth  = 0x03
	FrameTypeClose = 0x04
	FrameTypeDNS   = 0x06
	FrameTypeData  = 0x01
	FrameTypeHello = 0x02
	FrameTypeProxy = 0x05

	FrameFlagNone        = 0x00
	FrameFlagSegment     = 0x40
	FrameFlagSegmentLast = 0x80

	FrameMaxPayloadSize = 65535
	FrameHeaderSize     = 4
)

// @sk-task kvn-android#T1.1: protocol error vars (AC-004)
var (
	ErrPayloadTooLarge = errors.New("payload exceeds max frame size")
	ErrFrameTooShort   = errors.New("frame too short")
)

// @sk-task kvn-android#T1.1: generated Frame type (AC-004)
type Frame struct {
	Type    byte
	Flags   byte
	Length  uint16
	Payload []byte
}
