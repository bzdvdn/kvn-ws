package client

import (
	"os/exec"
	"strings"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

func applyKillSwitch(cfg *config.ClientConfig, logger *zap.Logger) {
	if cfg.KillSwitch == nil || !cfg.KillSwitch.Enabled {
		return
	}
	rules := `table ip kvn-kill {
	chain prerouting {
		type filter hook prerouting priority 0; policy accept;
		reject
	}
}
`
	if cfg.IPv6 {
		rules += `table ip6 kvn-kill {
	chain prerouting {
		type filter hook prerouting priority 0; policy accept;
		reject
	}
}
`
	}
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(rules)
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Warn("kill-switch: nft atomic apply failed", zap.ByteString("output", out), zap.Error(err))
		return
	}
	logger.Info("kill-switch enabled: all traffic blocked")
}

func removeKillSwitch(cfg *config.ClientConfig, logger *zap.Logger) {
	if cfg.KillSwitch == nil || !cfg.KillSwitch.Enabled {
		return
	}
	if err := exec.Command("nft", "delete", "table", "ip", "kvn-kill").Run(); err != nil {
		logger.Warn("kill-switch: nftables delete failed", zap.Error(err))
	}
	if cfg.IPv6 {
		if err := exec.Command("nft", "delete", "table", "ip6", "kvn-kill").Run(); err != nil {
			logger.Warn("kill-switch: ipv6 nftables delete failed", zap.Error(err))
		}
	}
}
