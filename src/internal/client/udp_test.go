package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"

	"go.uber.org/zap"
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

func TestUDPManagerPrefersHTTP2AfterFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := newUDPManager(ctx, nil, nil, zap.NewNop())

	if m.preferHTTP2("203.0.113.10:443") {
		t.Fatal("fresh target must use QUIC")
	}
	m.forceHTTP2("203.0.113.10:443")
	if !m.preferHTTP2("203.0.113.10:443") {
		t.Fatal("target must fall back to HTTP/2")
	}
	if m.preferHTTP2("203.0.113.11:443") {
		t.Fatal("other targets must keep using QUIC")
	}
}

func TestIsIPv4Fragment(t *testing.T) {
	pkt := make([]byte, 28)
	pkt[0] = 0x45
	if isIPv4Fragment(pkt) {
		t.Fatal("plain packet reported as fragment")
	}
	binary.BigEndian.PutUint16(pkt[6:8], 0x2000) // more fragments
	if !isIPv4Fragment(pkt) {
		t.Fatal("MF packet not detected")
	}
	binary.BigEndian.PutUint16(pkt[6:8], 0x0010) // offset 2
	if !isIPv4Fragment(pkt) {
		t.Fatal("offset packet not detected")
	}
}
