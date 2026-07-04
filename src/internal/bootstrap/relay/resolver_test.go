package relay

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	geoippb "github.com/bzdvdn/kvn-ws/src/internal/routing/geoip"
)

func strPtr(s string) *string {
	return &s
}

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

func newRelayForTest(t *testing.T, rc *config.RelayRoutingCfg) *Relay {
	t.Helper()
	cfg := &config.RelayConfig{
		Mode: "relay",
		Relay: config.RelayTermCfg{
			Mode:    "terminator",
			Routing: rc,
		},
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "relay.yaml")
	return NewFromConfig(cfg, zap.NewNop(), cfgPath)
}

// @sk-test geoip-geosite-integration#T3.3: resolve direct_sources with geoip (AC-002)
func TestResolveDirectSourcesGeoIP(t *testing.T) {
	geoPath := writeGeoIPDat(t, map[string][]string{
		"RU": {"10.0.0.0/8", "192.168.0.0/16"},
	})

	rc := &config.RelayRoutingCfg{
		DirectRanges: []string{"172.16.0.0/12"},
		DirectSources: []config.SourceRule{
			{GeoIP: strPtr("RU")},
		},
		GeoIPPath: geoPath,
	}

	r := newRelayForTest(t, rc)
	r.resolveDirectSources(rc)

	if len(rc.DirectRanges) != 3 {
		t.Fatalf("DirectRanges = %d, want 3 (1 static + 2 geoip)", len(rc.DirectRanges))
	}

	seen := map[string]bool{}
	for _, cidr := range rc.DirectRanges {
		seen[cidr] = true
	}
	if !seen["172.16.0.0/12"] {
		t.Error("missing static 172.16.0.0/12")
	}
	if !seen["10.0.0.0/8"] {
		t.Error("missing geoip 10.0.0.0/8")
	}
	if !seen["192.168.0.0/16"] {
		t.Error("missing geoip 192.168.0.0/16")
	}
}

// @sk-test geoip-geosite-integration#T3.3: resolve direct_sources with geosite (AC-008)
func TestResolveDirectSourcesGeoSite(t *testing.T) {
	gsPath := writeGeoSiteDat(t, map[string][]string{
		"YANDEX": {"yandex.ru", ".yandex.net"},
	})

	rc := &config.RelayRoutingCfg{
		DirectDomains: []string{".local"},
		DirectSources: []config.SourceRule{
			{GeoSite: strPtr("YANDEX")},
		},
		GeoSitePath: gsPath,
	}

	r := newRelayForTest(t, rc)
	r.resolveDirectSources(rc)

	if len(rc.DirectDomains) != 3 {
		t.Fatalf("DirectDomains = %d, want 3 (1 static + 2 geosite)", len(rc.DirectDomains))
	}

	seen := map[string]bool{}
	for _, d := range rc.DirectDomains {
		seen[d] = true
	}
	if !seen[".local"] {
		t.Error("missing static .local")
	}
	if !seen["yandex.ru"] {
		t.Error("missing geosite yandex.ru")
	}
	if !seen[".yandex.net"] {
		t.Error("missing geosite .yandex.net")
	}
}

// @sk-test geoip-geosite-integration#T3.3: resolve direct_sources with CIDR (AC-004)
func TestResolveDirectSourcesCIDR(t *testing.T) {
	rc := &config.RelayRoutingCfg{
		DirectRanges: []string{"10.0.0.0/8"},
		DirectSources: []config.SourceRule{
			{CIDR: strPtr("192.168.0.0/16")},
		},
	}

	r := newRelayForTest(t, rc)
	r.resolveDirectSources(rc)

	if len(rc.DirectRanges) != 2 {
		t.Fatalf("DirectRanges = %d, want 2", len(rc.DirectRanges))
	}
	if rc.DirectRanges[1] != "192.168.0.0/16" {
		t.Errorf("DirectRanges[1] = %s, want 192.168.0.0/16", rc.DirectRanges[1])
	}
}

// @sk-test geoip-geosite-integration#T3.3: resolve direct_sources with URL (file://) (AC-003)
func TestResolveDirectSourcesURL(t *testing.T) {
	listPath := writeTextList(t, []string{
		"10.10.0.0/16",
		".custom.domain",
	})

	rc := &config.RelayRoutingCfg{
		DirectRanges:  []string{"10.0.0.0/8"},
		DirectDomains: []string{".local"},
		DirectSources: []config.SourceRule{
			{URL: strPtr("file://" + listPath)},
		},
	}

	r := newRelayForTest(t, rc)
	r.resolveDirectSources(rc)

	if len(rc.DirectRanges) != 2 {
		t.Fatalf("DirectRanges = %d, want 2", len(rc.DirectRanges))
	}
	if len(rc.DirectDomains) != 2 {
		t.Fatalf("DirectDomains = %d, want 2", len(rc.DirectDomains))
	}
	if rc.DirectRanges[1] != "10.10.0.0/16" {
		t.Errorf("DirectRanges[1] = %s, want 10.10.0.0/16", rc.DirectRanges[1])
	}
}

// @sk-test geoip-geosite-integration#T3.3: resolve geoip:private alias (AC-002)
func TestResolveDirectSourcesGeoIPPrivate(t *testing.T) {
	rc := &config.RelayRoutingCfg{
		DirectSources: []config.SourceRule{
			{GeoIP: strPtr("private")},
		},
	}

	r := newRelayForTest(t, rc)
	r.resolveDirectSources(rc)

	if len(rc.DirectRanges) == 0 {
		t.Fatal("DirectRanges empty after geoip:private resolution")
	}

	seen := map[string]bool{}
	for _, cidr := range rc.DirectRanges {
		seen[cidr] = true
	}
	if !seen["10.0.0.0/8"] {
		t.Error("missing 10.0.0.0/8 from private alias")
	}
	if !seen["192.168.0.0/16"] {
		t.Error("missing 192.168.0.0/16 from private alias")
	}
}

// @sk-test geoip-geosite-integration#T3.3: empty direct_sources is no-op
func TestResolveDirectSourcesEmpty(t *testing.T) {
	rc := &config.RelayRoutingCfg{
		DirectRanges:  []string{"10.0.0.0/8"},
		DirectDomains: []string{".local"},
	}

	r := newRelayForTest(t, rc)
	r.resolveDirectSources(rc)

	if len(rc.DirectRanges) != 1 {
		t.Fatalf("DirectRanges = %d, want 1", len(rc.DirectRanges))
	}
	if len(rc.DirectDomains) != 1 {
		t.Fatalf("DirectDomains = %d, want 1", len(rc.DirectDomains))
	}
}

// @sk-test geoip-geosite-integration#T3.3: graceful degradation with broken geoip path (AC-006)
func TestResolveDirectSourcesGracefulDegradation(t *testing.T) {
	rc := &config.RelayRoutingCfg{
		DirectRanges: []string{"10.0.0.0/8"},
		DirectSources: []config.SourceRule{
			{GeoIP: strPtr("RU")},
		},
		GeoIPPath: "/nonexistent/geoip.dat",
	}

	r := newRelayForTest(t, rc)
	r.resolveDirectSources(rc)

	if len(rc.DirectRanges) != 1 {
		t.Fatalf("DirectRanges = %d, want 1 (original preserved)", len(rc.DirectRanges))
	}
	if rc.DirectRanges[0] != "10.0.0.0/8" {
		t.Errorf("DirectRanges[0] = %s, want 10.0.0.0/8", rc.DirectRanges[0])
	}
}

// @sk-test geoip-geosite-integration#T3.3: mixed sources resolved together
func TestResolveDirectSourcesMixed(t *testing.T) {
	geoPath := writeGeoIPDat(t, map[string][]string{
		"RU": {"10.0.0.0/8"},
	})
	gsPath := writeGeoSiteDat(t, map[string][]string{
		"YANDEX": {"yandex.ru"},
	})
	listPath := writeTextList(t, []string{
		"172.16.0.0/12",
		".custom",
	})

	rc := &config.RelayRoutingCfg{
		DirectRanges:  []string{"192.168.0.0/16"},
		DirectDomains: []string{".local"},
		DirectSources: []config.SourceRule{
			{GeoIP: strPtr("RU")},
			{GeoSite: strPtr("YANDEX")},
			{CIDR: strPtr("10.10.0.0/24")},
			{URL: strPtr("file://" + listPath)},
		},
		GeoIPPath:   geoPath,
		GeoSitePath: gsPath,
	}

	r := newRelayForTest(t, rc)
	r.resolveDirectSources(rc)

	if len(rc.DirectRanges) != 4 {
		t.Fatalf("DirectRanges = %d, want 4 (1 static + 1 geoip + 1 cidr + 1 url)", len(rc.DirectRanges))
	}
	if len(rc.DirectDomains) != 3 {
		t.Fatalf("DirectDomains = %d, want 3 (1 static + 1 geosite + 1 url)", len(rc.DirectDomains))
	}
}

// @sk-test geoip-geosite-integration#T3.3: dedup overlapping CIDRs from sources
func TestResolveDirectSourcesMergeDedup(t *testing.T) {
	geoPath := writeGeoIPDat(t, map[string][]string{
		"RU": {"10.0.0.0/8", "10.0.0.0/8"},
	})

	rc := &config.RelayRoutingCfg{
		DirectRanges: []string{"10.0.0.0/8"},
		DirectSources: []config.SourceRule{
			{GeoIP: strPtr("RU")},
			{CIDR: strPtr("10.0.0.0/8")},
		},
		GeoIPPath: geoPath,
	}

	r := newRelayForTest(t, rc)
	r.resolveDirectSources(rc)

	if len(rc.DirectRanges) != 1 {
		t.Fatalf("DirectRanges = %d, want 1 (dedup)", len(rc.DirectRanges))
	}
}

// @sk-test geoip-geosite-integration#T3.3: resolveDirectSources updates RuleSet behaviour
func TestResolveDirectSourcesUpdatesRuleSet(t *testing.T) {
	geoPath := writeGeoIPDat(t, map[string][]string{
		"RU": {"10.0.0.0/8"},
	})

	rc := &config.RelayRoutingCfg{
		DirectRanges: []string{"192.168.0.0/16"},
		DirectSources: []config.SourceRule{
			{GeoIP: strPtr("RU")},
		},
		GeoIPPath: geoPath,
	}

	r := newRelayForTest(t, rc)
	r.resolveDirectSources(rc)

	rs, err := newDirectRuleSet(rc, zap.NewNop())
	if err != nil {
		t.Fatalf("newDirectRuleSet: %v", err)
	}

	tests := []struct {
		ip   string
		want routing.RouteAction
	}{
		{"10.0.0.1", routing.RouteDirect},
		{"192.168.0.1", routing.RouteDirect},
		{"8.8.8.8", routing.RouteServer},
	}
	for _, tt := range tests {
		action := rs.Route(netip.MustParseAddr(tt.ip))
		if action != tt.want {
			t.Errorf("Route(%s) = %d, want %d", tt.ip, action, tt.want)
		}
	}
}
