//go:build windows

package tun

import (
	"math/rand"
	"strconv"
	"testing"

	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// @sk-test win-tun#T6.1: TestDeterministicGUIDStability (AC-006)
func TestDeterministicGUIDStability(t *testing.T) {
	g1 := deterministicGUID("kvn-ws")
	g2 := deterministicGUID("kvn-ws")
	if g1 != g2 {
		t.Errorf("deterministicGUID(\"kvn-ws\") produced different results: %v vs %v", g1, g2)
	}
}

// @sk-test win-tun#T6.1: TestDeterministicGUIDDifferentInputs (AC-006)
func TestDeterministicGUIDDifferentInputs(t *testing.T) {
	g1 := deterministicGUID("server-a")
	g2 := deterministicGUID("server-b")
	if g1 == g2 {
		t.Error("deterministicGUID produced identical GUID for different inputs")
	}
}

// @sk-test win-tun#T6.1: TestDeterministicGUIDVersionBits (AC-006)
func TestDeterministicGUIDVersionBits(t *testing.T) {
	g := deterministicGUID("kvn-ws")
	// UUIDv5: version 5 (0101) is in Data3 high 4 bits -> 0x5xxx
	if g.Data3&0xf000 != 0x5000 {
		t.Errorf("expected UUIDv5 version bits 0x5xxx, got Data3=0x%04x", g.Data3)
	}
	// UUIDv5: variant RFC 4122 (10xxxxxx) is in Data4[0] high 2 bits -> 0x8x..0xbx
	if g.Data4[0]&0xc0 != 0x80 {
		t.Errorf("expected UUIDv5 variant bits 0x8x, got Data4[0]=0x%02x", g.Data4[0])
	}
}

// @sk-test win-tun#T6.1: TestParseLUIDRoundtrip (AC-004)
func TestParseLUIDRoundtrip(t *testing.T) {
	tests := []uint64{0, 1, 42, 65535, 1<<32 - 1, 1<<64 - 1}
	for _, v := range tests {
		s := strconv.FormatUint(v, 10)
		luid, err := parseLUID(s)
		if err != nil {
			t.Errorf("parseLUID(%q): unexpected error: %v", s, err)
			continue
		}
		if winipcfg.LUID(v) != luid {
			t.Errorf("parseLUID(%q) = %d, want %d", s, luid, v)
		}
	}
}

// @sk-test win-tun#T6.1: TestParseLUIDInvalid (AC-004)
func TestParseLUIDInvalid(t *testing.T) {
	invalid := []string{"", "abc", "12.5", "-1", "0x10"}
	for _, s := range invalid {
		_, err := parseLUID(s)
		if err == nil {
			t.Errorf("parseLUID(%q): expected error, got nil", s)
		}
	}
}

// @sk-test win-tun#T6.1: TestDeterministicGUIDRandom (AC-006)
func TestDeterministicGUIDRandom(t *testing.T) {
	for i := 0; i < 100; i++ {
		name := strconv.Itoa(rand.Intn(1000000))
		g1 := deterministicGUID(name)
		g2 := deterministicGUID(name)
		if g1 != g2 {
			t.Fatalf("deterministicGUID not stable for name=%q: %v vs %v", name, g1, g2)
		}
	}
}
