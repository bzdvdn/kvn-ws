package routing

import (
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing/geoip"
)

var privateCIDRs = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"100.64.0.0/10",
	"fc00::/7",
}

// @sk-task geoip-geosite-integration#T2.1: resolver struct (AC-002)
type Resolver struct {
	origCfg  *config.RoutingCfg
	cacheDir string
	logger   *zap.Logger
	mu       sync.Mutex
}

// @sk-task geoip-geosite-integration#T2.1: new resolver (AC-002)
func NewResolver(cfg *config.RoutingCfg, cacheDir string, logger *zap.Logger) *Resolver {
	return &Resolver{
		origCfg:  cfg,
		cacheDir: cacheDir,
		logger:   logger,
	}
}

func (r *Resolver) resolveInternal() (*config.RoutingCfg, error) {
	merged := &config.RoutingCfg{
		DefaultRoute:   r.origCfg.DefaultRoute,
		IncludeRanges:  copySlice(r.origCfg.IncludeRanges),
		ExcludeRanges:  copySlice(r.origCfg.ExcludeRanges),
		IncludeIPs:     copySlice(r.origCfg.IncludeIPs),
		ExcludeIPs:     copySlice(r.origCfg.ExcludeIPs),
		IncludeDomains: copySlice(r.origCfg.IncludeDomains),
		ExcludeDomains: copySlice(r.origCfg.ExcludeDomains),
	}

	geoipDB, err := r.loadGeoIPDB()
	if err != nil {
		r.logger.Warn("geoip database unavailable, skipping geoip sources", zap.Error(err))
	}

	geositeDB, err := r.loadGeoSiteDB()
	if err != nil {
		r.logger.Debug("geosite database unavailable, skipping geosite sources", zap.Error(err))
	}

	for _, src := range r.origCfg.IncludeSources {
		cidrs, domains, ok := r.resolveSource(src, geoipDB, geositeDB)
		if !ok {
			continue
		}
		merged.IncludeRanges = append(merged.IncludeRanges, cidrs...)
		merged.IncludeDomains = append(merged.IncludeDomains, domains...)
	}
	for _, src := range r.origCfg.ExcludeSources {
		cidrs, domains, ok := r.resolveSource(src, geoipDB, geositeDB)
		if !ok {
			continue
		}
		merged.ExcludeRanges = append(merged.ExcludeRanges, cidrs...)
		merged.ExcludeDomains = append(merged.ExcludeDomains, domains...)
	}

	merged.IncludeRanges = dedupStrings(merged.IncludeRanges)
	merged.ExcludeRanges = dedupStrings(merged.ExcludeRanges)
	merged.IncludeDomains = dedupStrings(merged.IncludeDomains)
	merged.ExcludeDomains = dedupStrings(merged.ExcludeDomains)

	return merged, nil
}

// @sk-task geoip-geosite-integration#T2.1: resolve sources (AC-002, AC-003, AC-004, AC-005, AC-006, AC-009)
func (r *Resolver) Resolve() (*config.RoutingCfg, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resolveInternal()
}

// @sk-task geoip-geosite-integration#T3.3: resolve arbitrary source rules (AC-002, AC-003, AC-004)
type SourcesResult struct {
	CIDRs   []string
	Domains []string
}

// @sk-task geoip-geosite-integration#T3.3: resolve arbitrary source list (AC-002, AC-003, AC-004)
func (r *Resolver) ResolveSources(sources []config.SourceRule) (*SourcesResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	geoipDB, err := r.loadGeoIPDB()
	if err != nil {
		r.logger.Warn("geoip database unavailable, skipping geoip sources", zap.Error(err))
	}

	geositeDB, err := r.loadGeoSiteDB()
	if err != nil {
		r.logger.Debug("geosite database unavailable, skipping geosite sources", zap.Error(err))
	}

	var allCIDRs, allDomains []string
	for _, src := range sources {
		cidrs, domains, ok := r.resolveSource(src, geoipDB, geositeDB)
		if !ok {
			continue
		}
		allCIDRs = append(allCIDRs, cidrs...)
		allDomains = append(allDomains, domains...)
	}

	return &SourcesResult{
		CIDRs:   dedupStrings(allCIDRs),
		Domains: dedupStrings(allDomains),
	}, nil
}

// @sk-task geoip-geosite-integration#T4.2: refresh sources (AC-011)
func (r *Resolver) Refresh() (*config.RoutingCfg, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("refreshing source databases")
	if err := r.removeCachedDatabases(); err != nil {
		r.logger.Warn("failed to clear cached databases", zap.Error(err))
	}

	return r.resolveInternal()
}

func (r *Resolver) loadGeoIPDB() (map[string][]netip.Prefix, error) {
	path := r.findGeoIPPath()
	if path == "" {
		return nil, fmt.Errorf("no geoip path or url configured")
	}
	return geoip.ReadGeoIP(path)
}

func (r *Resolver) loadGeoSiteDB() (map[string][]string, error) {
	path := r.findGeoSitePath()
	if path == "" {
		return nil, fmt.Errorf("no geosite path or url configured")
	}
	return geoip.ReadGeoSite(path)
}

func (r *Resolver) findGeoIPPath() string {
	if r.origCfg.GeoIPPath != "" {
		if _, err := os.Stat(r.origCfg.GeoIPPath); err == nil {
			return r.origCfg.GeoIPPath
		}
		r.logger.Warn("geoip_path not found, trying geoip_url", zap.String("path", r.origCfg.GeoIPPath))
	}
	if r.origCfg.GeoIPURL != "" {
		return r.downloadIfNeeded("geoip", r.origCfg.GeoIPURL, "geoip.dat")
	}
	return ""
}

func (r *Resolver) findGeoSitePath() string {
	if r.origCfg.GeoSitePath != "" {
		if _, err := os.Stat(r.origCfg.GeoSitePath); err == nil {
			return r.origCfg.GeoSitePath
		}
		r.logger.Warn("geosite_path not found, trying geosite_url", zap.String("path", r.origCfg.GeoSitePath))
	}
	if r.origCfg.GeoSiteURL != "" {
		return r.downloadIfNeeded("geosite", r.origCfg.GeoSiteURL, "geosite.dat")
	}
	return ""
}

func (r *Resolver) downloadIfNeeded(kind, url, filename string) string {
	cacheFile := filepath.Join(r.cacheDir, filename)

	ttl := r.origCfg.SourceTTL
	if ttl <= 0 {
		ttl = 24
	}

	if fi, err := os.Stat(cacheFile); err == nil {
		if time.Since(fi.ModTime()).Hours() < float64(ttl) {
			r.logger.Debug("using cached database", zap.String("kind", kind), zap.String("file", cacheFile))
			return cacheFile
		}
		r.logger.Debug("cached database expired, re-downloading", zap.String("kind", kind))
	}

	r.logger.Info("downloading database", zap.String("kind", kind), zap.String("url", url))
	if err := r.downloadFile(url, cacheFile); err != nil {
		r.logger.Warn("failed to download database", zap.String("kind", kind), zap.Error(err))
		return ""
	}
	return cacheFile
}

func (r *Resolver) downloadFile(url, dest string) error {
	resp, err := http.Get(url) // #nosec G107 — URL from user config
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}

	out, err := os.Create(dest) // #nosec G304 — dest is filepath.Join(r.cacheDir, hardcoded filename)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (r *Resolver) removeCachedDatabases() error {
	items := []string{"geoip.dat", "geosite.dat"}
	for _, name := range items {
		p := filepath.Join(r.cacheDir, name)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// resolveSource processes a single SourceRule and returns resolved CIDRs and domains.
// The bool return indicates whether the source was successfully resolved.
//
// @sk-task geoip-geosite-integration#T2.1: source resolver (AC-002, AC-003, AC-004, AC-005, AC-009)
func (r *Resolver) resolveSource(src config.SourceRule, geoipDB map[string][]netip.Prefix, geositeDB map[string][]string) (cidrs, domains []string, ok bool) {
	if !src.Valid() {
		r.logger.Warn("invalid source rule: exactly one field must be set", zap.String("type", src.Type()), zap.String("value", src.Value()))
		return nil, nil, false
	}

	switch src.Type() {
	case "geoip":
		return r.resolveGeoIP(src.Value(), geoipDB)
	case "geosite":
		return r.resolveGeosite(src.Value(), geositeDB)
	case "cidr":
		return r.resolveCIDR(src.Value())
	case "url":
		return r.resolveURL(src.Value())
	default:
		r.logger.Warn("unknown source type", zap.String("type", src.Type()))
		return nil, nil, false
	}
}

func (r *Resolver) resolveGeoIP(code string, geoipDB map[string][]netip.Prefix) (cidrs, domains []string, ok bool) {
	if strings.EqualFold(code, "private") {
		r.logger.Debug("resolved geoip:private -> built-in private ranges")
		return privateCIDRs, nil, true
	}

	if geoipDB == nil {
		r.logger.Warn("geoip database not available, skipping", zap.String("code", code))
		return nil, nil, false
	}

	prefixes, ok := geoipDB[code]
	if !ok {
		// try uppercase
		prefixes, ok = geoipDB[strings.ToUpper(code)]
		if !ok {
			r.logger.Warn("country code not found in geoip database", zap.String("code", code))
			return nil, nil, false
		}
	}

	cidrs = make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		cidrs = append(cidrs, p.String())
	}
	r.logger.Debug("resolved geoip source", zap.String("code", code), zap.Int("cidrs", len(cidrs)))
	return cidrs, nil, true
}

// @sk-task geoip-geosite-integration#T4.1: geosite source resolution (AC-008)
func (r *Resolver) resolveGeosite(category string, geositeDB map[string][]string) (cidrs, domains []string, ok bool) {
	if geositeDB == nil {
		r.logger.Warn("geosite database not available, skipping", zap.String("category", category))
		return nil, nil, false
	}

	domains, ok = geositeDB[category]
	if !ok {
		domains, ok = geositeDB[strings.ToUpper(category)]
		if !ok {
			r.logger.Warn("category not found in geosite database", zap.String("category", category))
			return nil, nil, false
		}
	}
	r.logger.Debug("resolved geosite source", zap.String("category", category), zap.Int("domains", len(domains)))
	return nil, domains, true
}

func (r *Resolver) resolveCIDR(cidr string) (cidrs, domains []string, ok bool) {
	_, err := netip.ParsePrefix(cidr)
	if err != nil {
		r.logger.Warn("invalid CIDR in source", zap.String("cidr", cidr), zap.Error(err))
		return nil, nil, false
	}
	r.logger.Debug("resolved CIDR source", zap.String("cidr", cidr))
	return []string{cidr}, nil, true
}

func (r *Resolver) resolveURL(url string) (cidrs, domains []string, ok bool) {
	var err error
	if strings.HasPrefix(url, "file://") {
		localPath := strings.TrimPrefix(url, "file://")
		f, openErr := os.Open(localPath) // #nosec G304 — path from user config
		if openErr != nil {
			r.logger.Warn("failed to open URL source file", zap.String("url", url), zap.Error(openErr))
			return nil, nil, false
		}
		defer f.Close()
		cidrs, domains, err = geoip.ReadURLList(f)
	} else {
		resp, getErr := http.Get(url) // #nosec G107 — URL from user config
		if getErr != nil {
			r.logger.Warn("failed to fetch URL source", zap.String("url", url), zap.Error(getErr))
			return nil, nil, false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			r.logger.Warn("URL source returned non-200", zap.String("url", url), zap.Int("status", resp.StatusCode))
			return nil, nil, false
		}
		cidrs, domains, err = geoip.ReadURLList(resp.Body)
	}

	if err != nil {
		r.logger.Warn("failed to parse URL source", zap.String("url", url), zap.Error(err))
		return nil, nil, false
	}

	r.logger.Debug("resolved URL source", zap.String("url", url), zap.Int("cidrs", len(cidrs)), zap.Int("domains", len(domains)))

	// validate CIDRs
	validCIDRs := make([]string, 0, len(cidrs))
	for _, c := range cidrs {
		if _, parseErr := netip.ParsePrefix(c); parseErr == nil {
			validCIDRs = append(validCIDRs, c)
		} else {
			r.logger.Warn("invalid CIDR in URL source, skipping", zap.String("cidr", c), zap.Error(parseErr))
		}
	}

	return validCIDRs, domains, true
}

func copySlice(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}

func dedupStrings(s []string) []string {
	if len(s) == 0 {
		return s
	}
	seen := make(map[string]bool, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
