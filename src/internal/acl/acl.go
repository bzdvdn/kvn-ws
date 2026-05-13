// @sk-task security-acl#T2: CIDR matcher package
package acl

import (
	"fmt"
	"net"
)

type CIDRMatcher struct {
	allow []*net.IPNet
	deny  []*net.IPNet
}

func NewCIDRMatcher(allowCIDRs, denyCIDRs []string) (*CIDRMatcher, error) {
	m := &CIDRMatcher{}
	for _, cidr := range denyCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("parse deny CIDR %s: %w", cidr, err)
		}
		m.deny = append(m.deny, n)
	}
	for _, cidr := range allowCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("parse allow CIDR %s: %w", cidr, err)
		}
		m.allow = append(m.allow, n)
	}
	return m, nil
}

func (m *CIDRMatcher) Allowed(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range m.deny {
		if n.Contains(ip) {
			return false
		}
	}
	if len(m.allow) == 0 {
		return true
	}
	for _, n := range m.allow {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
