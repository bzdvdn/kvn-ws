package client

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

// @sk-test kvn-v2-transport#T5.1: IPv4 UDP reply packet is well-formed (AC-008)
func TestBuildUDPReply(t *testing.T) {
	payload := []byte("hello")
	pkt := buildUDPReply(net.ParseIP("1.2.3.4"), net.ParseIP("10.66.0.2"), 53, 12345, payload)
	if pkt[0]>>4 != 4 || pkt[9] != 17 {
		t.Fatalf("bad IPv4/UDP header: %x", pkt[:20])
	}
	if got := binary.BigEndian.Uint16(pkt[20:22]); got != 53 {
		t.Fatalf("src port = %d", got)
	}
	if got := binary.BigEndian.Uint16(pkt[22:24]); got != 12345 {
		t.Fatalf("dst port = %d", got)
	}
	if !bytes.Equal(pkt[28:], payload) {
		t.Fatalf("payload = %q", pkt[28:])
	}
	if binary.BigEndian.Uint16(pkt[10:12]) == 0 {
		t.Fatal("IP checksum is zero")
	}
}

func TestIsUDPv4(t *testing.T) {
	if !isUDPv4(append([]byte{0x45, 0, 0, 0, 0, 0, 0, 0, 64, 17}, make([]byte, 10)...)) {
		t.Fatal("UDPv4 not detected")
	}
	if isUDPv4(append([]byte{0x45, 0, 0, 0, 0, 0, 0, 0, 64, 6}, make([]byte, 10)...)) {
		t.Fatal("TCP detected as UDP")
	}
}
