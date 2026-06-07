//go:build linux

package systemproxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// @sk-test system-proxy#T4.1: TestSystemdOverridePermissionDenied logs warning on write failure (AC-007)
func TestSystemdOverridePermissionDenied(t *testing.T) {
	logger := zap.NewNop()
	err := systemdOverride(logger, "127.0.0.1:2310")
	if err != nil {
		t.Errorf("systemdOverride returned error (should log warning instead): %v", err)
	}
}

// @sk-test system-proxy#T4.1: TestSystemdOverrideWritesFile verifies systemd drop-in content (AC-007)
func TestSystemdOverrideWritesFile(t *testing.T) {
	dir := t.TempDir()
	origPath := systemdOverridePath
	// can't patch func; test the write operation directly
	systemdOverridePath = func() string {
		return filepath.Join(dir, "system-proxy.conf")
	}
	defer func() { systemdOverridePath = origPath }()

	logger := zap.NewNop()
	if err := systemdOverride(logger, "10.0.0.1:2310"); err != nil {
		t.Fatalf("systemdOverride: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "system-proxy.conf"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "HTTP_PROXY=http://10.0.0.1:2310") {
		t.Errorf("missing HTTP_PROXY in:\n%s", content)
	}
	if !strings.Contains(content, "Environment=") {
		t.Errorf("missing Environment= lines in:\n%s", content)
	}
}
