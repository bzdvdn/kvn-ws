//go:build linux

package dnsproxy

import (
	"os"
	"path/filepath"
	"testing"
)

// @sk-test transparent-proxy#T4.3: TestReadNameserver parses resolv.conf (AC-009)
func TestReadNameserver(t *testing.T) {
	dir := t.TempDir()
	origPath := resolvConfPath
	resolvConfPath = filepath.Join(dir, "resolv.conf")
	defer func() { resolvConfPath = origPath }()

	content := "nameserver 8.8.8.8\nnameserver 8.8.4.4\n"
	if err := os.WriteFile(resolvConfPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ns, err := readNameserver()
	if err != nil {
		t.Fatalf("readNameserver: %v", err)
	}
	if ns != "8.8.8.8:53" {
		t.Errorf("got %q, want %q", ns, "8.8.8.8:53")
	}
}

// @sk-test transparent-proxy#T4.3: TestReadNameserverNoFile returns error (AC-009)
func TestReadNameserverNoFile(t *testing.T) {
	origPath := resolvConfPath
	resolvConfPath = "/nonexistent/resolv.conf"
	defer func() { resolvConfPath = origPath }()

	_, err := readNameserver()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// @sk-test transparent-proxy#T4.3: TestReadNameserverEmptyNoNameserver (AC-009)
func TestReadNameserverEmptyNoNameserver(t *testing.T) {
	dir := t.TempDir()
	origPath := resolvConfPath
	resolvConfPath = filepath.Join(dir, "resolv.conf")
	defer func() { resolvConfPath = origPath }()

	if err := os.WriteFile(resolvConfPath, []byte("# no nameserver here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readNameserver()
	if err == nil {
		t.Fatal("expected error when no nameserver line")
	}
}

// @sk-test transparent-proxy#T4.3: TestBackupResolvConfRestore round-trips content (AC-009)
func TestBackupResolvConfRestore(t *testing.T) {
	dir := t.TempDir()
	origPath := resolvConfPath
	resolvConfPath = filepath.Join(dir, "resolv.conf")
	defer func() { resolvConfPath = origPath }()

	original := "nameserver 192.168.1.1\n"
	if err := os.WriteFile(resolvConfPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	backup, err := BackupResolvConf()
	if err != nil {
		t.Fatalf("BackupResolvConf: %v", err)
	}
	if !backup.saved {
		t.Fatal("backup.saved = false")
	}

	if err := OverrideResolvConf("127.0.0.53:53"); err != nil {
		t.Fatalf("OverrideResolvConf: %v", err)
	}

	data, _ := os.ReadFile(resolvConfPath)
	if string(data) != "nameserver 127.0.0.53\nnameserver 192.168.1.1\n" {
		t.Errorf("after override: got %q, want %q", string(data), "nameserver 127.0.0.53\nnameserver 192.168.1.1\n")
	}

	if err := backup.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, _ = os.ReadFile(resolvConfPath)
	if string(data) != original {
		t.Errorf("after restore: got %q, want %q", string(data), original)
	}
}

// @sk-test transparent-proxy#T5.3: TestBackupResolvConfNameservers returns parsed nameservers (AC-011)
func TestBackupResolvConfNameservers(t *testing.T) {
	dir := t.TempDir()
	origPath := resolvConfPath
	resolvConfPath = filepath.Join(dir, "resolv.conf")
	defer func() { resolvConfPath = origPath }()

	content := "nameserver 10.0.0.1\nnameserver 10.0.0.2\n"
	if err := os.WriteFile(resolvConfPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	backup, err := BackupResolvConf()
	if err != nil {
		t.Fatalf("BackupResolvConf: %v", err)
	}

	nss := backup.Nameservers()
	if len(nss) != 2 {
		t.Fatalf("Nameservers: got %d, want 2", len(nss))
	}
	if nss[0] != "10.0.0.1:53" {
		t.Errorf("ns[0]=%q, want %q", nss[0], "10.0.0.1:53")
	}
	if nss[1] != "10.0.0.2:53" {
		t.Errorf("ns[1]=%q, want %q", nss[1], "10.0.0.2:53")
	}
}
