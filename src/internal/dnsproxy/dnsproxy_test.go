package dnsproxy

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dnsQuery(domain string) []byte {
	header := []byte{0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	buf := make([]byte, 0, len(header)+len(domain)+len(domain)+8)
	buf = append(buf, header...)
	for _, label := range strings.Split(domain, ".") {
		buf = append(buf, byte(len(label)))
		buf = append(buf, []byte(label)...)
	}
	buf = append(buf, 0x00, 0x00, 0x01, 0x00, 0x01) // terminator + QTYPE A + QCLASS IN
	return buf
}

func dnsQueryCompressed() []byte {
	// A query with a compression pointer (0xc0 0x0c) instead of a regular label
	buf := []byte{
		0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0xc0, 0x0c,
		0x00, 0x01, 0x00, 0x01,
	}
	return buf
}

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

// @sk-test transparent-proxy#T4.3: TestReadUint16 (AC-009)
func TestReadUint16(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\x00\xff"))
	v, err := readUint16(r)
	if err != nil {
		t.Fatalf("readUint16: %v", err)
	}
	if v != 255 {
		t.Errorf("got %d, want %d", v, 255)
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
	if string(data) != "nameserver 127.0.0.53\n" {
		t.Errorf("after override: got %q, want %q", string(data), "nameserver 127.0.0.53\n")
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

// @sk-test transparent-proxy#T5.4: TestExtractDNSDomain simple domain (AC-011)
func TestExtractDNSDomainSimple(t *testing.T) {
	got := extractDNSDomain(dnsQuery("google.com"))
	want := "google.com"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// @sk-test transparent-proxy#T5.4: TestExtractDNSDomain multi-level domain (AC-011)
func TestExtractDNSDomainMultiLevel(t *testing.T) {
	got := extractDNSDomain(dnsQuery("www.example.co.uk"))
	want := "www.example.co.uk"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// @sk-test transparent-proxy#T5.4: TestExtractDNSDomain empty msg (AC-011)
func TestExtractDNSDomainEmpty(t *testing.T) {
	if got := extractDNSDomain(nil); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if got := extractDNSDomain([]byte{1, 2, 3}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// @sk-test transparent-proxy#T5.4: TestExtractDNSDomain compressed pointer (AC-011)
func TestExtractDNSDomainCompressed(t *testing.T) {
	got := extractDNSDomain(dnsQueryCompressed())
	if got != "" {
		t.Errorf("compressed pointer: got %q, want empty", got)
	}
}

// @sk-test transparent-proxy#T5.4: TestSetRouteFunc stores function (AC-011)
func TestSetRouteFunc(t *testing.T) {
	s := New("127.0.0.1:0", "8.8.8.8:53")

	// Verify default is nil (routeDirect not set)
	if s.routeDirect != nil {
		t.Fatal("routeDirect should be nil initially")
	}

	called := false
	s.SetRouteFunc(func(domain string) bool {
		called = true
		return domain == "blocked.com"
	})

	if s.routeDirect == nil {
		t.Fatal("routeDirect should not be nil after SetRouteFunc")
	}

	s.mu.Lock()
	fn := s.routeDirect
	s.mu.Unlock()
	if fn == nil {
		t.Fatal("stored routeDirect is nil")
	}
	if !fn("blocked.com") {
		t.Error("expected routeDirect('blocked.com') = true")
	}
	if fn("allowed.com") {
		t.Error("expected routeDirect('allowed.com') = false")
	}
	if !called {
		t.Error("routeDirect function was not called")
	}
}

// @sk-test transparent-proxy#T5.4: TestSetOrigResolvers stores resolvers (AC-011)
func TestSetOrigResolvers(t *testing.T) {
	s := New("127.0.0.1:0", "8.8.8.8:53")

	if s.origResolves != nil {
		t.Fatal("origResolves should be nil initially")
	}

	resolvers := []string{"192.168.1.1:53", "10.0.0.1:53"}
	s.SetOrigResolvers(resolvers)

	if len(s.origResolves) != 2 {
		t.Fatalf("got %d resolvers, want 2", len(s.origResolves))
	}
	if s.origResolves[0] != "192.168.1.1:53" {
		t.Errorf("resolvers[0]=%q, want %q", s.origResolves[0], "192.168.1.1:53")
	}
	if s.origResolves[1] != "10.0.0.1:53" {
		t.Errorf("resolvers[1]=%q, want %q", s.origResolves[1], "10.0.0.1:53")
	}
}
