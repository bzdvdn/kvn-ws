package nat

import (
	"fmt"
	"os/exec"
	"strings"
)

// @sk-task routing-split-tunnel#T2.3: nftables nat manager (AC-007)
// @sk-task ipv6-dual-stack#T2.3: add IPv6 NAT support (AC-003)
type NFTManager struct{}

func NewNFTManager() *NFTManager {
	return &NFTManager{}
}

// @sk-task routing-split-tunnel#T2.3: setup nftables nat (AC-007)
func (m *NFTManager) Setup() error {
	if err := m.checkNft(); err != nil {
		return fmt.Errorf("nftables check: %w", err)
	}
	if err := m.runNft("add", "table", "ip", "kvn-nat"); err != nil {
		return err
	}
	chainDef := "{ type nat hook postrouting priority srcnat; }"
	if err := m.runNft("add", "chain", "ip", "kvn-nat", "postrouting", chainDef); err != nil {
		return fmt.Errorf("add chain: %w", err)
	}
	if err := m.runNft("add", "rule", "ip", "kvn-nat", "postrouting", "masquerade"); err != nil {
		return err
	}
	return nil
}

// @sk-task routing-split-tunnel#T2.3: teardown nftables nat (AC-007)
func (m *NFTManager) Teardown() error {
	return m.runNft("delete", "table", "ip", "kvn-nat")
}

// @sk-task ipv6-dual-stack#T2.3: setup IPv6 masquerade (AC-003)
func (m *NFTManager) Setup6() error {
	if err := m.checkNft(); err != nil {
		return fmt.Errorf("nftables check: %w", err)
	}
	if err := m.runNft("add", "table", "ip6", "kvn-nat"); err != nil {
		return err
	}
	chainDef := "{ type nat hook postrouting priority srcnat; }"
	if err := m.runNft("add", "chain", "ip6", "kvn-nat", "postrouting", chainDef); err != nil {
		return fmt.Errorf("add ip6 chain: %w", err)
	}
	if err := m.runNft("add", "rule", "ip6", "kvn-nat", "postrouting", "masquerade"); err != nil {
		return err
	}
	return nil
}

// @sk-task ipv6-dual-stack#T2.3: teardown IPv6 masquerade (AC-003)
func (m *NFTManager) Teardown6() error {
	return m.runNft("delete", "table", "ip6", "kvn-nat")
}

func (m *NFTManager) checkNft() error {
	return exec.Command("nft", "--version").Run()
}

func (m *NFTManager) runNft(args ...string) error {
	cmd := exec.Command("nft", args...) // #nosec G204 — whitelisted nft binary for NAT
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}
