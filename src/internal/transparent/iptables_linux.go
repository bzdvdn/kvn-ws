//go:build linux

package transparent

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"

	"go.uber.org/zap"
)

const chainName = "KVN_TPROXY"

type iptablesLinuxManager struct{}

// @sk-task transparent-proxy#T2.1: iptables REDIRECT rules (AC-001, AC-004)
func newManager() TransparentManager {
	return &iptablesLinuxManager{}
}

func (m *iptablesLinuxManager) Set(ctx context.Context, logger *zap.Logger, port int, excludes []string) error {
	bin, err := detectIptables()
	if err != nil {
		return fmt.Errorf("detect iptables: %w", err)
	}
	logger.Info("setting up iptables transparent proxy",
		zap.String("bin", bin),
		zap.Int("port", port),
		zap.Strings("excludes", excludes),
	)

	// create custom chain
	_ = exec.CommandContext(ctx, bin, "-t", "nat", "-N", chainName).Run()

	// exclude CIDR rules
	for _, cidr := range excludes {
		_ = exec.CommandContext(ctx, bin, "-t", "nat", "-A", chainName, "-d", cidr, "-j", "RETURN").Run()
	}

	// REDIRECT all remaining TCP to local proxy port
	portStr := strconv.Itoa(port)
	if err := exec.CommandContext(ctx, bin, "-t", "nat", "-A", chainName, "-p", "tcp", "-j", "REDIRECT", "--to-port", portStr).Run(); err != nil {
		return fmt.Errorf("add REDIRECT rule: %w", err)
	}

	// apply chain to PREROUTING (routed traffic — Docker, VMs)
	if err := exec.CommandContext(ctx, bin, "-t", "nat", "-A", "PREROUTING", "-j", chainName).Run(); err != nil {
		return fmt.Errorf("add PREROUTING jump: %w", err)
	}

	// apply chain to OUTPUT (locally-generated traffic — browser, apps)
	if err := exec.CommandContext(ctx, bin, "-t", "nat", "-A", "OUTPUT", "-j", chainName).Run(); err != nil {
		return fmt.Errorf("add OUTPUT jump: %w", err)
	}

	logger.Info("iptables transparent proxy rules installed")
	return nil
}

func (m *iptablesLinuxManager) Restore(ctx context.Context, logger *zap.Logger) error {
	bin, err := detectIptables()
	if err != nil {
		return fmt.Errorf("detect iptables: %w", err)
	}

	// remove jumps from PREROUTING and OUTPUT
	_ = exec.CommandContext(ctx, bin, "-t", "nat", "-D", "PREROUTING", "-j", chainName).Run()
	_ = exec.CommandContext(ctx, bin, "-t", "nat", "-D", "OUTPUT", "-j", chainName).Run()

	// flush and delete custom chain
	_ = exec.CommandContext(ctx, bin, "-t", "nat", "-F", chainName).Run()
	_ = exec.CommandContext(ctx, bin, "-t", "nat", "-X", chainName).Run()

	logger.Info("iptables transparent proxy rules removed")
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
