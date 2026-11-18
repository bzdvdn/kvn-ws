//go:build linux

package dnsproxy

import (
	"os"
	"path/filepath"
	"strings"
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

func TestParseDefaultRouteIfaces(t *testing.T) {
	out := "default via 192.168.1.1 dev eno1 proto dhcp src 192.168.1.10 metric 100 \n" +
		"default via 10.0.0.1 dev ppp0 proto static metric 50\n" +
		"default dev lo scope link\n" +
		"default via 192.168.1.1 dev eno1 proto dhcp metric 200\n"
	got := parseDefaultRouteIfaces(out)
	want := []string{"eno1", "ppp0"}
	if len(got) != len(want) {
		t.Fatalf("parseDefaultRouteIfaces: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseDefaultRouteIfaces: got %v, want %v", got, want)
		}
	}
}

func TestSystemdResolvedOverrideUsesDefaultRouteIface(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "stub-resolv.conf")
	if err := os.WriteFile(target, []byte("nameserver 127.0.0.53\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "resolv.conf")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	origPath := resolvConfPath
	origLinks := systemdResolvedLinks
	resolvConfPath = link
	systemdResolvedLinks = []string{target}
	t.Cleanup(func() {
		resolvConfPath = origPath
		systemdResolvedLinks = origLinks
	})

	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"dns\" ] && [ $# -eq 2 ]; then\n" +
		"  echo 'Link 2 (eth0): 10.0.0.1 10.0.0.2'\n" +
		"  exit 0\n" +
		"fi\n" +
		"printf '%s\\n' \"$*\" >> '" + logPath + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "resolvectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	backup, err := BackupResolvConf("eth0")
	if err != nil {
		t.Fatalf("BackupResolvConf: %v", err)
	}
	if err := OverrideResolvConf("127.0.0.153:53", "eth0"); err != nil {
		t.Fatalf("OverrideResolvConf: %v", err)
	}
	if err := backup.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "dns eth0 127.0.0.153\ndns eth0 10.0.0.1 10.0.0.2\n"
	if string(data) != want {
		t.Fatalf("resolvectl calls: got %q, want %q", string(data), want)
	}
}

func TestSystemdResolvedOverrideReportsResolvectlError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "stub-resolv.conf")
	if err := os.WriteFile(target, []byte("nameserver 127.0.0.53\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "resolv.conf")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	origPath := resolvConfPath
	origLinks := systemdResolvedLinks
	resolvConfPath = link
	systemdResolvedLinks = []string{target}
	t.Cleanup(func() {
		resolvConfPath = origPath
		systemdResolvedLinks = origLinks
	})

	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho 'Link lo is loopback device.' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "resolvectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := OverrideResolvConf("127.0.0.153:53", "eth0")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "loopback device") {
		t.Fatalf("error %q does not include resolvectl stderr", err)
	}
}

func TestCleanupStaleResolvedRevertsOnlyProxyLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "stub-resolv.conf")
	if err := os.WriteFile(target, []byte("nameserver 127.0.0.53\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "resolv.conf")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	origPath := resolvConfPath
	origLinks := systemdResolvedLinks
	resolvConfPath = link
	systemdResolvedLinks = []string{target}
	t.Cleanup(func() {
		resolvConfPath = origPath
		systemdResolvedLinks = origLinks
	})

	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"dns\" ]; then\n" +
		"  echo 'Link 2 (eno1): 127.0.0.153'\n" +
		"  echo 'Link 3 (wlp3s0): 8.8.8.8'\n" +
		"elif [ \"$1\" = \"revert\" ]; then\n" +
		"  printf '%s\\n' \"$*\" >> '" + logPath + "'\n" +
		"fi\n"
	if err := os.WriteFile(filepath.Join(bin, "resolvectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	CleanupStaleDNS("127.0.0.153:53")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "revert eno1\n" {
		t.Fatalf("resolvectl calls: got %q, want %q", string(data), "revert eno1\n")
	}
}

func TestLineHasDNSServer(t *testing.T) {
	if !lineHasDNSServer(": 127.0.0.153 8.8.8.8", "127.0.0.153") {
		t.Fatal("expected exact server match")
	}
	if lineHasDNSServer(": 127.0.0.1530 8.8.8.8", "127.0.0.153") {
		t.Fatal("prefix must not match")
	}
	if !lineHasDNSServer(": 127.0.0.153:5353", "127.0.0.153") {
		t.Fatal("expected host:port match")
	}
}

func TestResolvectlRestore(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\nprintf '%s|' \"$@\" >> '" + logPath + "'\necho >> '" + logPath + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "resolvectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := resolvectlRestore([]string{"eth0", "ppp0"}, map[string][]string{"eth0": {}}); err != nil {
		t.Fatalf("resolvectlRestore: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "dns|eth0||\nrevert|ppp0|\n"
	if string(data) != want {
		t.Fatalf("resolvectl calls: got %q, want %q", string(data), want)
	}
}
