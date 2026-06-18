//go:build integration

package routing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing/geoip"
)

func geoipDBURL() string {
	return "https://github.com/v2fly/geoip/releases/latest/download/geoip.dat"
}

func geositeDBURL() string {
	return "https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat"
}

func downloadToDir(t *testing.T, urlStr, destFile string) string {
	t.Helper()
	dir := t.TempDir()
	dest := filepath.Join(dir, destFile)
	r := &Resolver{}
	if err := r.downloadFile(urlStr, dest); err != nil {
		t.Fatalf("download %q: %v", urlStr, err)
	}
	return dest
}

// @sk-test geoip-geosite-integration#T3.2-integration: real geoip.dat download + ReadGeoIP parsing
// @sk-test geoip-geosite-integration#T3.2: integration geoip real db download (AC-002)
func TestIntegrationGeoIPRealDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	path := downloadToDir(t, geoipDBURL(), "geoip.dat")
	result, err := geoip.ReadGeoIP(path)
	if err != nil {
		t.Fatalf("ReadGeoIP: %v", err)
	}

	known := []string{"RU", "US", "CN", "DE", "GB", "JP"}
	for _, code := range known {
		cidrs, ok := result[code]
		if !ok {
			t.Errorf("country %q not found in geoip.dat", code)
			continue
		}
		if len(cidrs) == 0 {
			t.Errorf("country %q has empty CIDR list", code)
		}
		t.Logf("  %s: %d CIDRs", code, len(cidrs))
	}

	if len(result) < 50 {
		t.Errorf("expected 50+ countries in geoip.dat, got %d", len(result))
	}
}

// @sk-test geoip-geosite-integration#T4.1-integration: real geosite.dat download + ReadGeoSite parsing
// @sk-test geoip-geosite-integration#T3.2: integration geosite real db download (AC-008)
func TestIntegrationGeoSiteRealDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	path := downloadToDir(t, geositeDBURL(), "geosite.dat")
	result, err := geoip.ReadGeoSite(path)
	if err != nil {
		t.Fatalf("ReadGeoSite: %v", err)
	}

	// Uppercase company/org names (v2fly domain-list-community convention)
	known := []string{"CN", "FACEBOOK", "YOUTUBE", "NETFLIX", "APPLE", "MICROSOFT", "AMAZON", "SPOTIFY", "INSTAGRAM"}
	for _, code := range known {
		domains, ok := result[code]
		if !ok {
			t.Errorf("category %q not found in geosite.dat", code)
			continue
		}
		if len(domains) == 0 {
			t.Errorf("category %q has empty domain list", code)
		}
	}

	// Uppercase category-* prefixed names
	prefixed := []string{"CATEGORY-ADS", "CATEGORY-ADS-ALL", "CATEGORY-GAMES", "CATEGORY-PORN"}
	for _, code := range prefixed {
		domains, ok := result[code]
		if !ok {
			t.Errorf("category %q not found in geosite.dat", code)
			continue
		}
		if len(domains) == 0 {
			t.Errorf("category %q has empty domain list", code)
		}
	}

	if len(result) < 50 {
		t.Errorf("expected 50+ categories in geosite.dat, got %d", len(result))
	}
}

// @sk-test geoip-geosite-integration#T3.2-integration: real geoip.dat integration with full Resolver
// @sk-test geoip-geosite-integration#T3.2: integration geoip resolver (AC-002)
func TestIntegrationGeoIPResolver(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	geoPath := downloadToDir(t, geoipDBURL(), "geoip.dat")

	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		GeoIPPath:    geoPath,
		ExcludeSources: []config.SourceRule{
			{GeoIP: strPtr("RU")},
			{GeoIP: strPtr("CN")},
		},
		IncludeSources: []config.SourceRule{
			{CIDR: strPtr("10.0.0.0/8")},
		},
	}

	r := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(resolved.ExcludeRanges) < 2 {
		t.Errorf("expected 2+ CIDRs for RU+CN, got %d", len(resolved.ExcludeRanges))
	}
	if len(resolved.IncludeRanges) != 1 || resolved.IncludeRanges[0] != "10.0.0.0/8" {
		t.Errorf("IncludeRanges = %v, want [10.0.0.0/8]", resolved.IncludeRanges)
	}

	t.Logf("Resolved %d exclude CIDRs, %d include CIDRs",
		len(resolved.ExcludeRanges), len(resolved.IncludeRanges))
}

// @sk-test geoip-geosite-integration#T3.2-integration: real URL list download from GitHub
// @sk-test geoip-geosite-integration#T3.2: integration url source download (AC-003)
func TestIntegrationURLSourceDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	urlPath := filepath.Join(dir, "url.list")

	r := &Resolver{}
	if err := r.downloadFile(geoipDBURL(), urlPath); err != nil {
		t.Fatalf("download: %v", err)
	}

	if fi, err := os.Stat(urlPath); err != nil || fi.Size() == 0 {
		t.Errorf("downloaded file missing or empty: %v", err)
	}

	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		IncludeSources: []config.SourceRule{
			{URL: strPtr(geoipDBURL())},
		},
	}

	rr := NewResolver(cfg, t.TempDir(), nopLogger)
	resolved, err := rr.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	t.Logf("URL source resolved: %d CIDRs, %d domains",
		len(resolved.IncludeRanges), len(resolved.IncludeDomains))
}
