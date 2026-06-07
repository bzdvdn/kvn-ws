package transparent

import (
	"context"

	"go.uber.org/zap"
)

// @sk-task transparent-proxy#T1.2: TransparentManager interface (DEC-001)
type TransparentManager interface {
	Set(ctx context.Context, logger *zap.Logger, port int, excludes []string) error
	Restore(ctx context.Context, logger *zap.Logger) error
}

type noopManager struct{}

func (m *noopManager) Set(_ context.Context, _ *zap.Logger, _ int, _ []string) error {
	return nil
}

func (m *noopManager) Restore(_ context.Context, _ *zap.Logger) error {
	return nil
}

func New() TransparentManager {
	return newManager()
}
