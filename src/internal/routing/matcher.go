package routing

import (
	"fmt"
	"net/netip"
)

// @sk-task routing-split-tunnel#T2.2: cidr matcher (AC-002)
type CIDRMatcher struct {
	prefixes []netip.Prefix
}

// @sk-task routing-split-tunnel#T2.2: new cidr matcher (AC-002)
func NewCIDRMatcher(cidrs []string) (*CIDRMatcher, error) {
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, s := range cidrs {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("parse cidr %q: %w", s, err)
		}
		prefixes = append(prefixes, p)
	}
	return &CIDRMatcher{prefixes: prefixes}, nil
}

func (m *CIDRMatcher) Match(ip netip.Addr) bool {
	for _, p := range m.prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// @sk-task routing-split-tunnel#T2.2: exact ip matcher (AC-003)
type ExactIPMatcher struct {
	ips []netip.Addr
}

// @sk-task routing-split-tunnel#T2.2: new exact ip matcher (AC-003)
func NewExactIPMatcher(ipStrs []string) (*ExactIPMatcher, error) {
	ips := make([]netip.Addr, 0, len(ipStrs))
	for _, s := range ipStrs {
		ip, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("parse ip %q: %w", s, err)
		}
		ips = append(ips, ip)
	}
	return &ExactIPMatcher{ips: ips}, nil
}

func (m *ExactIPMatcher) Match(ip netip.Addr) bool {
	for _, candidate := range m.ips {
		if ip == candidate {
			return true
		}
	}
	return false
}
