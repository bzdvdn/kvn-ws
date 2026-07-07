package nat

import (
	"fmt"
	"os/exec"
	"strings"
)

const kvnNatChain = "KVN_NAT"

type IPTablesManager struct{}

func NewIPTablesManager() *IPTablesManager {
	return &IPTablesManager{}
}

func (m *IPTablesManager) Setup() error {
	bin, err := detectIptables()
	if err != nil {
		return err
	}
	_ = exec.Command(bin, "-t", "nat", "-N", kvnNatChain).Run()
	if err := exec.Command(bin, "-t", "nat", "-A", kvnNatChain, "-i", "kvn", "-j", "MASQUERADE").Run(); err != nil {
		return fmt.Errorf("add MASQUERADE rule: %s: %w", strings.TrimSpace(err.Error()), err)
	}
	if err := exec.Command(bin, "-t", "nat", "-A", "POSTROUTING", "-j", kvnNatChain).Run(); err != nil {
		return fmt.Errorf("add POSTROUTING jump: %s: %w", strings.TrimSpace(err.Error()), err)
	}
	return nil
}

func (m *IPTablesManager) Teardown() error {
	bin, err := detectIptables()
	if err != nil {
		return err
	}
	_ = exec.Command(bin, "-t", "nat", "-D", "POSTROUTING", "-j", kvnNatChain).Run()
	_ = exec.Command(bin, "-t", "nat", "-F", kvnNatChain).Run()
	_ = exec.Command(bin, "-t", "nat", "-X", kvnNatChain).Run()
	return nil
}

func (m *IPTablesManager) Setup6() error {
	bin, err := detectIp6tables()
	if err != nil {
		return err
	}
	_ = exec.Command(bin, "-t", "nat", "-N", kvnNatChain).Run()
	if err := exec.Command(bin, "-t", "nat", "-A", kvnNatChain, "-i", "kvn", "-j", "MASQUERADE").Run(); err != nil {
		return fmt.Errorf("add ipv6 MASQUERADE rule: %s: %w", strings.TrimSpace(err.Error()), err)
	}
	if err := exec.Command(bin, "-t", "nat", "-A", "POSTROUTING", "-j", kvnNatChain).Run(); err != nil {
		return fmt.Errorf("add ipv6 POSTROUTING jump: %s: %w", strings.TrimSpace(err.Error()), err)
	}
	return nil
}

func (m *IPTablesManager) Teardown6() error {
	bin, err := detectIp6tables()
	if err != nil {
		return err
	}
	_ = exec.Command(bin, "-t", "nat", "-D", "POSTROUTING", "-j", kvnNatChain).Run()
	_ = exec.Command(bin, "-t", "nat", "-F", kvnNatChain).Run()
	_ = exec.Command(bin, "-t", "nat", "-X", kvnNatChain).Run()
	return nil
}

func detectIptables() (string, error) {
	if err := exec.Command("iptables", "--version").Run(); err == nil {
		return "iptables", nil
	}
	if err := exec.Command("iptables-legacy", "--version").Run(); err == nil {
		return "iptables-legacy", nil
	}
	return "", fmt.Errorf("no iptables binary found (tried iptables, iptables-legacy)")
}

func detectIp6tables() (string, error) {
	if err := exec.Command("ip6tables", "--version").Run(); err == nil {
		return "ip6tables", nil
	}
	if err := exec.Command("ip6tables-legacy", "--version").Run(); err == nil {
		return "ip6tables-legacy", nil
	}
	return "", fmt.Errorf("no ip6tables binary found (tried ip6tables, ip6tables-legacy)")
}
