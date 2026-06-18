package routing

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	geoippb "github.com/bzdvdn/kvn-ws/src/internal/routing/geoip"
)

func writeGeoIPDat(t *testing.T, entries map[string][]string) string {
	t.Helper()
	list := &geoippb.GeoIPList{}
	for code, cidrs := range entries {
		ge := &geoippb.GeoIP{CountryCode: code}
		for _, c := range cidrs {
			p, err := netip.ParsePrefix(c)
			if err != nil {
				t.Fatalf("invalid CIDR %q: %v", c, err)
			}
			ip := p.Addr()
			ipBytes := ip.AsSlice()
			ge.Cidr = append(ge.Cidr, &geoippb.CIDR{Ip: ipBytes, Prefix: uint32(p.Bits())})
		}
		list.Entry = append(list.Entry, ge)
	}
	data, err := proto.Marshal(list)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "geoip.dat")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func writeTextList(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "list.txt")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// @sk-test geoip-geosite-integration#T3.2: geoip resolve (AC-002)
func TestGeoIPResolve(t *testing.T) {
	geoPath := writeGeoIPDat(t, map[string][]string{
		"RU": {"10.0.0.0/8", "192.168.0.0/16"},
		"US": {"172.16.0.0/12"},
	})

	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		GeoIPPath:    geoPath,
		ExcludeSources: []config.SourceRule{
			{GeoIP: strPtr("RU")},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(resolved.ExcludeRanges) != 2 {
		t.Fatalf("ExcludeRanges = %d, want 2", len(resolved.ExcludeRanges))
	}

	expected := map[string]bool{"10.0.0.0/8": true, "192.168.0.0/16": true}
	for _, cidr := range resolved.ExcludeRanges {
		if !expected[cidr] {
			t.Errorf("unexpected CIDR: %s", cidr)
		}
		delete(expected, cidr)
	}
}

// @sk-test geoip-geosite-integration#T3.2: geoip private alias (RQ-016)
func TestGeoIPPrivateAlias(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute: "direct",
		ExcludeSources: []config.SourceRule{
			{GeoIP: strPtr("private")},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	expectedPrivate := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",
		"fc00::/7",
	}
	if len(resolved.ExcludeRanges) != len(expectedPrivate) {
		t.Fatalf("ExcludeRanges = %d, want %d", len(resolved.ExcludeRanges), len(expectedPrivate))
	}

	have := make(map[string]bool)
	for _, c := range resolved.ExcludeRanges {
		have[c] = true
	}
	for _, exp := range expectedPrivate {
		if !have[exp] {
			t.Errorf("missing private CIDR: %s", exp)
		}
	}
}

// @sk-test geoip-geosite-integration#T3.2: cidr source (AC-004)
func TestCIDRSource(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		IncludeSources: []config.SourceRule{
			{CIDR: strPtr("10.0.0.0/8")},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(resolved.IncludeRanges) != 1 || resolved.IncludeRanges[0] != "10.0.0.0/8" {
		t.Fatalf("IncludeRanges = %v, want [\"10.0.0.0/8\"]", resolved.IncludeRanges)
	}
}

// @sk-test geoip-geosite-integration#T3.2: url source (AC-003)
func TestURLSource(t *testing.T) {
	listPath := writeTextList(t, []string{
		"# comment line",
		"",
		"10.0.0.0/8",
		"192.168.0.0/16",
		"example.com",
		".test.local",
	})

	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		IncludeSources: []config.SourceRule{
			{URL: strPtr("file://" + listPath)},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(resolved.IncludeRanges) != 2 {
		t.Fatalf("IncludeRanges = %d, want 2", len(resolved.IncludeRanges))
	}
	if len(resolved.IncludeDomains) != 2 {
		t.Fatalf("IncludeDomains = %d, want 2", len(resolved.IncludeDomains))
	}
}

// @sk-test geoip-geosite-integration#T3.2: merge dedup (AC-005)
func TestMergeDedup(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:  "server",
		IncludeRanges: []string{"10.0.0.0/8", "192.168.0.0/16"},
		IncludeSources: []config.SourceRule{
			{CIDR: strPtr("10.0.0.0/8")},
			{CIDR: strPtr("172.16.0.0/12")},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(resolved.IncludeRanges) != 3 {
		t.Fatalf("IncludeRanges = %d, want 3 (deduped)", len(resolved.IncludeRanges))
	}

	have := make(map[string]bool)
	for _, c := range resolved.IncludeRanges {
		if have[c] {
			t.Errorf("duplicate CIDR: %s", c)
		}
		have[c] = true
	}
}

// @sk-test geoip-geosite-integration#T3.2: graceful degradation (AC-006)
func TestGracefulDegradationBrokenURL(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		ExcludeSources: []config.SourceRule{
			{URL: strPtr("http://127.0.0.1:1/nonexistent")},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve should not return error on broken URL, got: %v", err)
	}

	// Source should be gracefully skipped — no error, empty ExcludeRanges
	if len(resolved.ExcludeRanges) != 0 {
		t.Errorf("ExcludeRanges = %v, want empty", resolved.ExcludeRanges)
	}
}

// @sk-test geoip-geosite-integration#T3.2: skip geoip without path/url (AC-009)
func TestSkipGeoIPWhenNoPathOrURL(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		ExcludeSources: []config.SourceRule{
			{GeoIP: strPtr("RU")},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// GeoIP source should be skipped gracefully
	if len(resolved.ExcludeRanges) != 0 {
		t.Errorf("ExcludeRanges = %v, want empty", resolved.ExcludeRanges)
	}
}

// @sk-test geoip-geosite-integration#T3.2: invalid source rule (AC-001)
func TestInvalidSourceRule(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		ExcludeSources: []config.SourceRule{
			{GeoIP: strPtr("RU"), CIDR: strPtr("10.0.0.0/8")},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Invalid source should be skipped
	if len(resolved.ExcludeRanges) != 0 {
		t.Errorf("ExcludeRanges = %v, want empty", resolved.ExcludeRanges)
	}
}

// @sk-test geoip-geosite-integration#T3.2: non-existent country code
func TestGeoIPResolveNonExistentCode(t *testing.T) {
	geoPath := writeGeoIPDat(t, map[string][]string{
		"RU": {"10.0.0.0/8"},
	})

	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		GeoIPPath:    geoPath,
		ExcludeSources: []config.SourceRule{
			{GeoIP: strPtr("ZZ")},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Non-existent code should be skipped gracefully
	if len(resolved.ExcludeRanges) != 0 {
		t.Errorf("ExcludeRanges = %v, want empty", resolved.ExcludeRanges)
	}
}

// @sk-test geoip-geosite-integration#T4.2: refresh clears cache (AC-011)
func TestRefreshClearsCache(t *testing.T) {
	geoPath := writeGeoIPDat(t, map[string][]string{
		"RU": {"10.0.0.0/8"},
	})

	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		GeoIPPath:    geoPath,
		ExcludeSources: []config.SourceRule{
			{GeoIP: strPtr("RU")},
		},
	}

	cacheDir := t.TempDir()
	r := NewResolver(cfg, cacheDir, nopLogger)

	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	if len(resolved.ExcludeRanges) != 1 {
		t.Fatalf("ExcludeRanges = %d, want 1", len(resolved.ExcludeRanges))
	}

	// Refresh should re-resolve
	resolved, err = r.Refresh()
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(resolved.ExcludeRanges) != 1 {
		t.Errorf("after Refresh ExcludeRanges = %d, want 1", len(resolved.ExcludeRanges))
	}
}

func writeGeoSiteDat(t *testing.T, entries map[string][]string) string {
	t.Helper()
	list := &geoippb.GeoSiteList{}
	for code, domains := range entries {
		gs := &geoippb.GeoSite{CategoryCode: code}
		for _, d := range domains {
			gs.Domain = append(gs.Domain, &geoippb.Domain{Value: d, Type: geoippb.Domain_Plain})
		}
		list.Entry = append(list.Entry, gs)
	}
	data, err := proto.Marshal(list)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "geosite.dat")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// @sk-test geoip-geosite-integration#T4.1: geosite resolution (AC-008)
func TestGeoSiteResolve(t *testing.T) {
	geoPath := writeGeoSiteDat(t, map[string][]string{
		"RU": {"example.com", ".ru"},
		"US": {"google.com"},
	})

	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		GeoSitePath:  geoPath,
		IncludeSources: []config.SourceRule{
			{GeoSite: strPtr("RU")},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(resolved.IncludeDomains) != 2 {
		t.Fatalf("IncludeDomains = %d, want 2", len(resolved.IncludeDomains))
	}

	expected := map[string]bool{"example.com": true, ".ru": true}
	for _, d := range resolved.IncludeDomains {
		if !expected[d] {
			t.Errorf("unexpected domain: %s", d)
		}
		delete(expected, d)
	}
}

// @sk-test geoip-geosite-integration#T4.2: TunRouter atomic swap (AC-011)
func TestTunRouterAtomicSwap(t *testing.T) {
	rs1, err := NewRuleSet(&config.RoutingCfg{DefaultRoute: "direct"}, nopLogger)
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	rs2, err := NewRuleSet(&config.RoutingCfg{DefaultRoute: "server"}, nopLogger)
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}

	tr := NewTunRouter(rs1, nil, nil, nil, nopLogger)

	// Initial ruleSet should direct
	ip := netip.MustParseAddr("8.8.8.8")
	tr.ruleSet.Store(rs1)
	action1 := tr.ruleSet.Load().Route(ip)
	if action1 != RouteDirect {
		t.Errorf("initial action = %d, want RouteDirect (%d)", action1, RouteDirect)
	}

	// Atomic swap
	tr.SetRuleSet(rs2)
	action2 := tr.ruleSet.Load().Route(ip)
	if action2 != RouteServer {
		t.Errorf("after swap action = %d, want RouteServer (%d)", action2, RouteServer)
	}

	// Old ruleSet should still route correctly
	if rs1.Route(ip) != RouteDirect {
		t.Error("old ruleSet changed after swap")
	}
}

func strPtr(s string) *string {
	return &s
}
