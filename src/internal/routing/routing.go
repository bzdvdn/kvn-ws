package routing

import (
	"net/netip"
)

// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task routing-split-tunnel#T1.1: routing shared types (AC-001)
// @sk-task routing-split-tunnel#T1.1: route action type (AC-001)
// @sk-task dns-routing#T2.1: RouteNone sentinel (AC-001)
type RouteAction int

const (
	RouteNone RouteAction = iota
	RouteServer
	RouteDirect
)

// @sk-task routing-split-tunnel#T1.1: rule struct (AC-006)
type Rule struct {
	Matcher Matcher
	Action  RouteAction
}

// @sk-task routing-split-tunnel#T1.1: matcher interface (AC-002)
type Matcher interface {
	Match(ip netip.Addr) bool
}
