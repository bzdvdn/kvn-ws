// @sk-task security-acl#T2: CIDR matcher package
package acl

import (
	"fmt"
	"net"
)

type action int

const (
	actionNone action = iota
	actionDeny
	actionAllow
)

type node struct {
	left  *node
	right *node
	act   action
}

// @sk-task production-gap#T3.1: radix tree for O(k) CIDR matching (AC-004)
type CIDRMatcher struct {
	root     *node
	hasAllow bool
}

func NewCIDRMatcher(allowCIDRs, denyCIDRs []string) (*CIDRMatcher, error) {
	m := &CIDRMatcher{root: &node{}}
	for _, cidr := range denyCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("parse deny CIDR %s: %w", cidr, err)
		}
		m.insert(n, actionDeny)
	}
	for _, cidr := range allowCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("parse allow CIDR %s: %w", cidr, err)
		}
		m.insert(n, actionAllow)
		m.hasAllow = true
	}
	return m, nil
}

func (m *CIDRMatcher) insert(n *net.IPNet, act action) {
	ones, bitsLen := n.Mask.Size()
	ip := n.IP
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	cur := m.root
	for i := 0; i < ones; i++ {
		if bit(ip, i) {
			if cur.right == nil {
				cur.right = &node{}
			}
			cur = cur.right
		} else {
			if cur.left == nil {
				cur.left = &node{}
			}
			cur = cur.left
		}
	}
	cur.act = act
	_ = bitsLen
}

func (m *CIDRMatcher) Allowed(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	matchedAllow := false
	cur := m.root
	for i := 0; i < len(ip)*8; i++ {
		if cur.act == actionDeny {
			return false
		}
		if cur.act == actionAllow {
			matchedAllow = true
		}
		if bit(ip, i) {
			if cur.right == nil {
				break
			}
			cur = cur.right
		} else {
			if cur.left == nil {
				break
			}
			cur = cur.left
		}
	}
	if cur.act == actionDeny {
		return false
	}
	if cur.act == actionAllow || matchedAllow {
		return true
	}
	return !m.hasAllow
}

func bit(ip []byte, i int) bool {
	return ip[i/8]>>(7-i%8)&1 == 1
}
