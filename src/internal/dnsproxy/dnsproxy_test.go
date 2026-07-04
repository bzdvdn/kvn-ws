package dnsproxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/dns"
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

// @sk-test dns-response-tracker#T2.3: TestDNSProxyTracksExcludedDomains tracks excluded domain IPs (AC-005)
func TestDNSProxyTracksExcludedDomains(t *testing.T) {
	domain := "ozon.ru"
	query := dnsQuery(domain)

	// Build DNS response: header QR=1, ANCOUNT=1, answer A 1.2.3.4
	resp := make([]byte, len(query))
	copy(resp, query)
	resp[2] = 0x81 // QR=1, RD=1
	resp[3] = 0x80 // RA=1
	binary.BigEndian.PutUint16(resp[6:8], 1)
	answer := []byte{
		0xc0, 0x0c, // name pointer to question
		0x00, 0x01, // TYPE A
		0x00, 0x01, // CLASS IN
		0x00, 0x00, 0x00, 0x3c, // TTL 60
		0x00, 0x04, // RDLENGTH 4
		0x01, 0x02, 0x03, 0x04, // 1.2.3.4
	}
	resp = append(resp, answer...)

	// Start fake DNS server (responds to any query with pre-built response)
	fakeAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fakeConn, err := net.ListenUDP("udp", fakeAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer fakeConn.Close()

	fakePort := fakeConn.LocalAddr().(*net.UDPAddr).Port
	fakeServerAddr := fmt.Sprintf("127.0.0.1:%d", fakePort)

	ctxFake, cancelFake := context.WithCancel(context.Background())
	defer cancelFake()
	go func() {
		buf := make([]byte, 1500)
		for {
			_ = fakeConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			_, addr, err := fakeConn.ReadFromUDP(buf)
			if err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					select {
					case <-ctxFake.Done():
						return
					default:
						continue
					}
				}
				return
			}
			_, _ = fakeConn.WriteToUDP(resp, addr)
		}
	}()

	// Create proxy — call resolveDirect directly instead of full Run
	s := New("127.0.0.1:0", "8.8.8.8:53")
	s.SetOrigResolvers([]string{fakeServerAddr})

	// s.conn must be non-nil for resolveDirect's WriteToUDP call
	testConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer testConn.Close()
	s.conn = testConn

	tracker := dns.NewTracker(60 * time.Second)
	s.SetTracker(tracker)

	raddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:1")
	s.resolveDirect(context.Background(), query, raddr, []string{fakeServerAddr})

	// Verify tracker has the mapping
	wantIP := netip.MustParseAddr("1.2.3.4")
	gotDomain, ok := tracker.Lookup(wantIP)
	if !ok {
		t.Errorf("Tracker.Lookup(%v): not found", wantIP)
	}
	if gotDomain != domain {
		t.Errorf("Tracker.Lookup(%v) = %q, want %q", wantIP, gotDomain, domain)
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

// @sk-test dns-upstreams-list#T4.1: TestDNSProxyVariadicNew (AC-005)
func TestDNSProxyVariadicNew(t *testing.T) {
	// no upstreams → defaults
	s1 := New("127.0.0.1:0")
	if len(s1.upstreams) == 0 {
		t.Fatal("New() with no args produced empty upstreams")
	}

	// single upstream
	s2 := New("127.0.0.1:0", "10.0.0.1:53")
	if len(s2.upstreams) != 1 || s2.upstreams[0] != "10.0.0.1:53" {
		t.Fatalf("New() with single upstream = %v, want [10.0.0.1:53]", s2.upstreams)
	}

	// multiple upstreams
	s3 := New("127.0.0.1:0", "10.0.0.1:53", "1.1.1.1:53")
	if len(s3.upstreams) != 2 {
		t.Fatalf("New() with two upstreams = %v, want [10.0.0.1:53 1.1.1.1:53]", s3.upstreams)
	}
}

// @sk-test dns-upstreams-list#T4.1: TestDNSProxyNilUpstreams (AC-005)
func TestDNSProxyNilUpstreams(t *testing.T) {
	// nil/empty slice → defaults
	s := New("127.0.0.1:0", []string{}...)
	if len(s.upstreams) == 0 {
		t.Fatal("New() with empty variadic produced empty upstreams")
	}
}

// @sk-test dns-upstreams-list#T4.1: TestDNSProxyFallback (AC-005)
func TestDNSProxyFallback(t *testing.T) {
	// Start a mock TCP upstream that records the received query
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstreamLn.Close()
	upstreamAddr := upstreamLn.Addr().String()

	var mu sync.Mutex
	var gotQuery []byte
	upstreamDone := make(chan struct{})
	go func() {
		conn, err := upstreamLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read 2-byte length-prefixed DNS query
		var lenBuf [2]byte
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			return
		}
		qlen := int(binary.BigEndian.Uint16(lenBuf[:]))
		query := make([]byte, qlen)
		if _, err := io.ReadFull(conn, query); err != nil {
			return
		}

		mu.Lock()
		gotQuery = query
		mu.Unlock()

		// Minimal DNS response to unblock forward
		resp := make([]byte, 16)
		copy(resp[:2], query[:2]) // copy TXID
		resp[2] = 0x81            // response flags
		resp[3] = 0x80
		wire := make([]byte, 2+len(resp))
		binary.BigEndian.PutUint16(wire, uint16(len(resp)))
		copy(wire[2:], resp)
		conn.Write(wire)
		close(upstreamDone)
	}()

	// Create Server with first upstream unreachable, second is our mock
	s := New("127.0.0.1:0", "198.51.100.1:53", upstreamAddr)

	// Manually set UDP conn so we can call forward
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()
	s.conn = udpConn

	ctx := context.Background()
	query := dnsQuery("fallback.example.com")
	clientAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9999}
	s.forward(ctx, query, clientAddr)

	select {
	case <-upstreamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: upstream did not receive query")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(gotQuery) == 0 {
		t.Fatal("upstream received empty query")
	}
	domain := extractDNSDomain(gotQuery)
	if domain != "fallback.example.com" {
		t.Errorf("upstream got domain %q, want %q", domain, "fallback.example.com")
	}
}
