package nat

import "os/exec"

// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task routing-split-tunnel#T1.1: nat manager interface (AC-007)
// @sk-task ipv6-dual-stack#T2.3: add Setup6/Teardown6 (AC-003)
type Manager interface {
	Setup() error
	Setup6() error
	Teardown() error
	Teardown6() error
}

// @sk-task ubuntu-22-fallback#T1.1: auto-detect nftables → iptables fallback (AC-007)
func NewManager() Manager {
	if exec.Command("nft", "--version").Run() == nil {
		return NewNFTManager()
	}
	return NewIPTablesManager()
}
