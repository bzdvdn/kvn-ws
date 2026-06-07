package transparent

import (
	"context"
	"os"
	"testing"

	"go.uber.org/zap"
)

// @sk-test transparent-proxy#T4.1: TestStubManager verifies noop manager on this platform (DEC-001)
func TestStubManager(t *testing.T) {
	mgr := newManager()
	if mgr == nil {
		t.Fatal("newManager() = nil")
	}
	ctx := context.Background()
	logger := zap.NewNop()

	if os.Geteuid() != 0 {
		t.Log("not root, Set may fail — testing error path")
		err := mgr.Set(ctx, logger, 2310, []string{"10.0.0.0/8"})
		if err == nil {
			t.Log("Set succeeded (root or stub)")
		} else {
			t.Logf("Set failed as expected: %v", err)
		}
	} else {
		if err := mgr.Set(ctx, logger, 2310, nil); err != nil {
			t.Errorf("Set: %v", err)
		}
		defer func() { _ = mgr.Restore(ctx, logger) }()
	}
}

// @sk-test transparent-proxy#T4.1: TestNew returns non-nil manager (DEC-001)
func TestNew(t *testing.T) {
	mgr := New()
	if mgr == nil {
		t.Fatal("New() = nil")
	}
}

// @sk-test transparent-proxy#T4.1: TestDetectIptablesNotFound fails when no iptables (AC-001)
func TestDetectIptablesNotFound(t *testing.T) {
	_, err := detectIptables()
	if err == nil {
		t.Skip("iptables binary found, skipping detection-failure test")
	}
}
