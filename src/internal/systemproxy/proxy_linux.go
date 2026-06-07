//go:build linux

package systemproxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

// @sk-task system-proxy#T1.1: linux platform manager (systemd override) (AC-007)
type linuxManager struct{}

func newPlatformManager() PlatformManager {
	return &linuxManager{}
}

func (m *linuxManager) Set(ctx context.Context, logger *zap.Logger, addr string, noProxy string) error {
	return systemdOverride(logger, addr)
}

func (m *linuxManager) Restore(ctx context.Context, logger *zap.Logger) error {
	_ = os.Remove(systemdOverridePath())
	return nil
}

// @sk-task system-proxy#T1.1: write systemd override file (best-effort) (AC-007)
var systemdOverridePath = func() string {
	return "/etc/systemd/system/kvn-client.service.d/system-proxy.conf"
}

func systemdOverride(logger *zap.Logger, addr string) error {
	path := systemdOverridePath()
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Warn("systemd override: cannot create directory, skipping",
			zap.String("dir", dir),
			zap.Error(err),
		)
		return nil
	}

	content := fmt.Sprintf(`[Service]
Environment=HTTP_PROXY=http://%s
Environment=http_proxy=http://%s
Environment=HTTPS_PROXY=http://%s
Environment=https_proxy=http://%s
`, addr, addr, addr, addr)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		logger.Warn("systemd override: write failed, skipping",
			zap.String("path", path),
			zap.Error(err),
		)
		return nil
	}

	logger.Info("systemd override written", zap.String("path", path))
	return nil
}
