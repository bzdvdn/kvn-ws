package geoip

import (
	"io"
	"net/netip"
	"os"

	"google.golang.org/protobuf/proto"
)

// ReadGeoIP reads a v2fly geoip.dat file and returns a map of country code to CIDR prefixes.
//
// @sk-task geoip-geosite-integration#T2.1: geoip dat parser (AC-002)
func ReadGeoIP(path string) (map[string][]netip.Prefix, error) {
	data, err := os.ReadFile(path) // #nosec G304 — path from config, validated upstream
	if err != nil {
		return nil, err
	}
	var list GeoIPList
	if err := proto.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	result := make(map[string][]netip.Prefix, len(list.Entry))
	for _, entry := range list.Entry {
		code := entry.CountryCode
		cidrs := make([]netip.Prefix, 0, len(entry.Cidr))
		for _, c := range entry.Cidr {
			if len(c.Ip) == 4 {
				var addr [4]byte
				copy(addr[:], c.Ip)
				cidrs = append(cidrs, netip.PrefixFrom(netip.AddrFrom4(addr), int(c.Prefix)))
			} else if len(c.Ip) == 16 {
				var addr [16]byte
				copy(addr[:], c.Ip)
				cidrs = append(cidrs, netip.PrefixFrom(netip.AddrFrom16(addr), int(c.Prefix)))
			}
		}
		result[code] = cidrs
	}
	return result, nil
}

// ReadGeoSite reads a v2fly geosite.dat file and returns a map of category to domain values.
//
// @sk-task geoip-geosite-integration#T2.1: geosite dat parser (AC-008)
// @sk-task geoip-geosite-integration#T4.1: geosite source support (AC-008)
func ReadGeoSite(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 — path from config, validated upstream
	if err != nil {
		return nil, err
	}
	var list GeoSiteList
	if err := proto.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(list.Entry))
	for _, entry := range list.Entry {
		code := entry.CategoryCode
		domains := make([]string, 0, len(entry.Domain))
		for _, d := range entry.Domain {
			domains = append(domains, d.Value)
		}
		result[code] = domains
	}
	return result, nil
}

// ReadURLList reads a URL-list format file (one entry per line).
// Lines with '#' prefix are comments. Empty lines are ignored.
// Entries with '/' are CIDRs, without '/' are domains.
func ReadURLList(r io.Reader) (cidrs, domains []string, err error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	lines := splitLines(string(data))
	for _, line := range lines {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if containsSlash(line) {
			cidrs = append(cidrs, line)
		} else {
			domains = append(domains, line)
		}
	}
	return cidrs, domains, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}

func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}
