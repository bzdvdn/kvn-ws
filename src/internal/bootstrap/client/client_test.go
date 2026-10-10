package client

import (
	"testing"
)

// @sk-test game-latency#T1.4: effectiveMTU clamps client MTU by advertised MTU (AC-003)
func TestEffectiveMTU(t *testing.T) {
	tests := []struct {
		name       string
		cfgMTU     int
		advertised int
		want       int
	}{
		{"advertised smaller", 1500, 1400, 1400},
		{"config smaller", 1300, 1400, 1300},
		{"advertised zero keeps config", 1300, 0, 1300},
		{"config zero uses advertised", 0, 1400, 1400},
		{"both zero", 0, 0, 0},
		{"equal", 1400, 1400, 1400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveMTU(tt.cfgMTU, tt.advertised); got != tt.want {
				t.Errorf("effectiveMTU(%d, %d) = %d, want %d", tt.cfgMTU, tt.advertised, got, tt.want)
			}
		})
	}
}

// @sk-test transparent-proxy#T4.5: TestPortFromAddr parses port from listen addr (AC-001)
func TestPortFromAddr(t *testing.T) {
	tests := []struct {
		addr string
		want int
	}{
		{"127.0.0.1:2310", 2310},
		{"0.0.0.0:8080", 8080},
		{":9999", 9999},
		{"", 2310},
		{"invalid", 2310},
	}
	for _, tt := range tests {
		got := portFromAddr(tt.addr)
		if got != tt.want {
			t.Errorf("portFromAddr(%q) = %d, want %d", tt.addr, got, tt.want)
		}
	}
}
