//go:build !linux && !darwin && !windows

package systemproxy

import (
	"context"

	"go.uber.org/zap"
)

// @sk-task system-proxy#T1.1: stub platform manager for unsupported platforms
type stubManager struct{}

func newPlatformManager() PlatformManager {
	return &stubManager{}
}

func (m *stubManager) Set(ctx context.Context, logger *zap.Logger, addr string, noProxy string) error {
	logger.Warn("system proxy platform API not supported on this platform")
	return nil
}

func (m *stubManager) Restore(ctx context.Context, logger *zap.Logger) error {
	return nil
}
