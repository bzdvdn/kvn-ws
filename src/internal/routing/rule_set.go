package routing

import (
	"fmt"
	"net/netip"
	"strings"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/dns"
)

// @sk-task routing-split-tunnel#T2.2: ruleset struct (AC-006)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
// @sk-task dns-routing#T3.1: suffixDomains map (AC-001, AC-002)
// @sk-task dns-response-tracker#T2.1: tracker field (AC-003)
type RuleSet struct {
	rules          []Rule
	defaultAction  RouteAction
	domainResolver dns.Resolver
	logger         *zap.Logger
	suffixDomains  map[RouteAction][]string
	tracker        *dns.Tracker
}

// @sk-task routing-split-tunnel#T2.2: new ruleset from config (AC-006)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
func NewRuleSet(cfg *config.RoutingCfg, logger *zap.Logger) (*RuleSet, error) {
	return NewRuleSetWithResolver(cfg, nil, logger)
}

// @sk-task routing-split-tunnel#T3.1: new ruleset with dns resolver (AC-005)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
// @sk-task dns-routing#T3.1: init suffixDomains map (AC-001, AC-002)
func NewRuleSetWithResolver(cfg *config.RoutingCfg, resolver dns.Resolver, logger *zap.Logger) (*RuleSet, error) {
	if cfg == nil {
		return nil, fmt.Errorf("routing config is nil")
	}
	rs := &RuleSet{
		domainResolver: resolver,
		logger:         logger,
		suffixDomains:  make(map[RouteAction][]string),
	}

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
	// @sk-task dns-routing#T3.1: suffix domain matching (AC-001, AC-002)
	if len(domains) > 0 {
		var suffixes []string
		for _, d := range domains {
			if strings.HasPrefix(d, ".") {
				suffixes = append(suffixes, d)
			}
		}
		if len(suffixes) > 0 {
			rs.suffixDomains[action] = append(rs.suffixDomains[action], suffixes...)
		}
	}
	// @sk-task routing-split-tunnel#T3.1: domain matcher integration (AC-005)
	if len(domains) > 0 && rs.domainResolver != nil {
		m := NewDomainMatcher(domains, rs.domainResolver, rs.logger)
		rs.rules = append(rs.rules, Rule{Matcher: m, Action: action})
	}
	return nil
}

// @sk-task dns-response-tracker#T2.1: SetTracker sets the DNS tracker (AC-003)
func (rs *RuleSet) SetTracker(t *dns.Tracker) {
	rs.tracker = t
}

// @sk-task dns-routing#T3.1: domain match for suffix rules (AC-001, AC-002)
func (rs *RuleSet) MatchDomain(domain string) RouteAction {
	for action, suffixes := range rs.suffixDomains {
		for _, suffix := range suffixes {
			if strings.HasSuffix(domain, suffix) {
				rs.logger.Debug("domain matched", zap.String("domain", domain), zap.String("suffix", suffix), zap.Int("action", int(action)))
				return action
			}
		}
	}
	return RouteNone
}

// @sk-task routing-split-tunnel#T2.2: route decision (AC-001)
// @sk-task production-readiness-hardening#T2.6: log.Printf → zap (AC-006)
// @sk-task dns-response-tracker#T2.1: tracker lookup before default (AC-003)
func (rs *RuleSet) Route(ip netip.Addr) RouteAction {
	if rs.tracker != nil {
		if domain, ok := rs.tracker.Lookup(ip); ok {
			rs.logger.Debug("tracker lookup", zap.String("ip", ip.String()), zap.String("domain", domain))
			if action := rs.MatchDomain(domain); action != RouteNone {
				return action
			}
		}
	}
	for _, r := range rs.rules {
		if r.Matcher.Match(ip) {
			rs.logger.Debug("matched rule", zap.Int("action", int(r.Action)), zap.String("ip", ip.String()))
			return r.Action
		}
	}
	rs.logger.Debug("default action", zap.Int("action", int(rs.defaultAction)), zap.String("ip", ip.String()))
	return rs.defaultAction
}
