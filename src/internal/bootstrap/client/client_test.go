package client

import (
	"testing"
)

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
