package relay

import (
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

func relayBoolPtr(v bool) *bool { return &v }

// @sk-test relay-padding-parity#T1.2: only padding.enabled turns padding on (AC-001)
func TestRelayPaddingRules(t *testing.T) {
	tests := []struct {
		name              string
		cfg               *config.RelayConfig
		wantEnabled       bool
		wantSize          int
		wantRealtimeNoPad bool
	}{
		{name: "nil config", cfg: nil, wantSize: 512},
		{name: "obfuscation enabled only", cfg: &config.RelayConfig{Obfuscation: &config.ObfuscationCfg{Enabled: true}}, wantSize: 512},
		{
			name:        "padding enabled with default size",
			cfg:         &config.RelayConfig{Obfuscation: &config.ObfuscationCfg{Padding: &config.PaddingCfg{Enabled: true}}},
			wantEnabled: true, wantSize: 512, wantRealtimeNoPad: true,
		},
		{
			name:        "padding enabled with custom size",
			cfg:         &config.RelayConfig{Obfuscation: &config.ObfuscationCfg{Padding: &config.PaddingCfg{Enabled: true, Size: 1024}}},
			wantEnabled: true, wantSize: 1024, wantRealtimeNoPad: true,
		},
		{
			name:        "realtime unpadded disabled",
			cfg:         &config.RelayConfig{Obfuscation: &config.ObfuscationCfg{Padding: &config.PaddingCfg{Enabled: true, RealtimeUnpadded: relayBoolPtr(false)}}},
			wantEnabled: true, wantSize: 512,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relayPaddingEnabled(tt.cfg); got != tt.wantEnabled {
				t.Errorf("relayPaddingEnabled = %v, want %v", got, tt.wantEnabled)
			}
			if got := relayPaddingSize(tt.cfg); got != tt.wantSize {
				t.Errorf("relayPaddingSize = %d, want %d", got, tt.wantSize)
			}
			if got := relayRealtimeUnpadded(tt.cfg); got != tt.wantRealtimeNoPad {
				t.Errorf("relayRealtimeUnpadded = %v, want %v", got, tt.wantRealtimeNoPad)
			}
		})
	}
}
