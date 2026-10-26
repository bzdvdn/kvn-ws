package relay

import (
	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
)

// @sk-task relay-padding-parity#T1.2: relay padding rules match client/server (AC-001, AC-007)
func relayPaddingEnabled(cfg *config.RelayConfig) bool {
	if cfg == nil || cfg.Obfuscation == nil || cfg.Obfuscation.Padding == nil {
		return false
	}
	return cfg.Obfuscation.Padding.Enabled
}

// @sk-task relay-padding-parity#T1.2: alignment size defaults to 512 (AC-007)
func relayPaddingSize(cfg *config.RelayConfig) int {
	if cfg != nil && cfg.Obfuscation != nil && cfg.Obfuscation.Padding != nil && cfg.Obfuscation.Padding.Size > 0 {
		return cfg.Obfuscation.Padding.Size
	}
	return 512
}

// @sk-task relay-padding-parity#T1.2: unpadded negotiation requires padding and realtime opt-in (AC-001, AC-002)
func relayRealtimeUnpadded(cfg *config.RelayConfig) bool {
	if !relayPaddingEnabled(cfg) {
		return false
	}
	return cfg.Obfuscation.Padding.RealtimeUnpaddedEnabled()
}

// @sk-task relay-padding-parity#T3.1: read the negotiated padding state for WS connections (AC-005)
func streamPaddingActive(conn transport.StreamConn) (active, known bool) {
	if state, ok := conn.(interface{ PaddingActive() bool }); ok {
		return state.PaddingActive(), true
	}
	return true, false
}
