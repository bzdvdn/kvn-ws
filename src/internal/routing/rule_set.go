package routing

import (
	"fmt"
	"log"
	"net/netip"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

// @sk-task routing-split-tunnel#T2.2: ruleset struct (AC-006)
type RuleSet struct {
	rules           []Rule
	defaultAction   RouteAction
	domainResolver  DomainResolver
}

// @sk-task routing-split-tunnel#T2.2: new ruleset from config (AC-006)
func NewRuleSet(cfg *config.RoutingCfg) (*RuleSet, error) {
	return NewRuleSetWithResolver(cfg, nil)
}

// @sk-task routing-split-tunnel#T3.1: new ruleset with dns resolver (AC-005)
func NewRuleSetWithResolver(cfg *config.RoutingCfg, resolver DomainResolver) (*RuleSet, error) {
	if cfg == nil {
		return nil, fmt.Errorf("routing config is nil")
	}
	rs := &RuleSet{domainResolver: resolver}

	// exclude rules (checked first)
	if err := rs.addRules(cfg.ExcludeRanges, cfg.ExcludeIPs, cfg.ExcludeDomains, RouteDirect); err != nil {
		return nil, fmt.Errorf("exclude rules: %w", err)
	}
	// include rules (checked second)
	if err := rs.addRules(cfg.IncludeRanges, cfg.IncludeIPs, cfg.IncludeDomains, RouteServer); err != nil {
		return nil, fmt.Errorf("include rules: %w", err)
	}
	// default rule
	switch cfg.DefaultRoute {
	case "server":
		rs.defaultAction = RouteServer
	case "direct":
		rs.defaultAction = RouteDirect
	default:
		return nil, fmt.Errorf("unknown default_route: %q", cfg.DefaultRoute)
	}
	return rs, nil
}

func (rs *RuleSet) addRules(cidrs, ips, domains []string, action RouteAction) error {
	if len(cidrs) > 0 {
		m, err := NewCIDRMatcher(cidrs)
		if err != nil {
			return err
		}
		rs.rules = append(rs.rules, Rule{Matcher: m, Action: action})
	}
	if len(ips) > 0 {
		m, err := NewExactIPMatcher(ips)
		if err != nil {
			return err
		}
		rs.rules = append(rs.rules, Rule{Matcher: m, Action: action})
	}
	// @sk-task routing-split-tunnel#T3.1: domain matcher integration (AC-005)
	if len(domains) > 0 && rs.domainResolver != nil {
		m := NewDomainMatcher(domains, rs.domainResolver)
		rs.rules = append(rs.rules, Rule{Matcher: m, Action: action})
	}
	return nil
}

// @sk-task routing-split-tunnel#T2.2: route decision (AC-001)
func (rs *RuleSet) Route(ip netip.Addr) RouteAction {
	for _, r := range rs.rules {
		if r.Matcher.Match(ip) {
			log.Printf("[routing] matched rule action=%d ip=%s", r.Action, ip)
			return r.Action
		}
	}
	log.Printf("[routing] default action=%d ip=%s", rs.defaultAction, ip)
	return rs.defaultAction
}
