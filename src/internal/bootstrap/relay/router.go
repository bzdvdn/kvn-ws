package relay

import (
	"fmt"
	"net"
	"net/netip"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// @sk-task relay-terminator#T3.1: routing tun wrapper (AC-002, AC-003)
type routingTun struct {
	inner tun.TunDevice
	ruleSet *routing.RuleSet
	upstream atomic.Pointer[upstreamSession]
	logger  *zap.Logger
}

func (r *routingTun) Open() error { return r.inner.Open() }
func (r *routingTun) Close() error { return r.inner.Close() }
func (r *routingTun) Read(buf []byte) (int, error) { return r.inner.Read(buf) }
func (r *routingTun) SetIP(ip net.IP, mask *net.IPNet) error { return r.inner.SetIP(ip, mask) }
func (r *routingTun) SetMTU(mtu int) error { return r.inner.SetMTU(mtu) }
func (r *routingTun) SetGateway(gateway net.IP) error { return r.inner.SetGateway(gateway) }
func (r *routingTun) RemoveGateway(gateway net.IP) error { return r.inner.RemoveGateway(gateway) }
func (r *routingTun) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	return r.inner.AddExcludeRoute(cidr, phyGateway, phyIface)
}
func (r *routingTun) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	return r.inner.RemoveExcludeRoute(cidr, phyGateway, phyIface)
}
func (r *routingTun) DisableGSO() error { return r.inner.DisableGSO() }

// @sk-task relay-terminator#T3.1: route packet on TUN write (AC-002, AC-003)
func (r *routingTun) Write(buf []byte) (int, error) {
	destIP, ok := extractDestIP(buf)
	if !ok {
		return r.inner.Write(buf)
	}

	action := r.ruleSet.Route(destIP)
	switch action {
	case routing.RouteDirect:
		r.logger.Debug("route=direct", zap.String("dst", destIP.String()))
		return r.inner.Write(buf)
	case routing.RouteServer:
		us := r.upstream.Load()
		if us == nil {
			r.logger.Warn("upstream not available, dropping packet", zap.String("dst", destIP.String()))
			return len(buf), nil
		}
		r.logger.Debug("route=upstream", zap.String("dst", destIP.String()))
		if err := us.Send(buf); err != nil {
			r.logger.Warn("upstream send failed", zap.Error(err))
			return len(buf), nil
		}
		return len(buf), nil
	default:
		return r.inner.Write(buf)
	}
}

// @sk-task relay-terminator#T3.1: build RuleSet from RelayRoutingCfg (AC-002)
func newDirectRuleSet(cfg *config.RelayRoutingCfg, logger *zap.Logger) (*routing.RuleSet, error) {
	if cfg == nil {
		return nil, fmt.Errorf("relay routing config is nil")
	}
	routingCfg := &config.RoutingCfg{
		ExcludeRanges:  cfg.DirectRanges,
		ExcludeDomains: cfg.DirectDomains,
		DefaultRoute:   "server",
	}
	return routing.NewRuleSetWithResolver(routingCfg, nil, logger)
}

// @sk-task relay-terminator#T3.1: create routing tun (AC-002, AC-003)
func newRoutingTun(inner tun.TunDevice, ruleSet *routing.RuleSet, logger *zap.Logger) *routingTun {
	return &routingTun{
		inner:   inner,
		ruleSet: ruleSet,
		logger:  logger,
	}
}

// @sk-task relay-terminator#T3.1: parse dest IP from raw packet (AC-002)
func extractDestIP(packet []byte) (netip.Addr, bool) {
	if len(packet) < 1 {
		return netip.Addr{}, false
	}
	switch packet[0] >> 4 {
	case 4:
		if len(packet) < 20 {
			return netip.Addr{}, false
		}
		return netip.AddrFrom4([4]byte(packet[16:20])), true
	case 6:
		if len(packet) < 40 {
			return netip.Addr{}, false
		}
		return netip.AddrFrom16([16]byte(packet[24:40])), true
	default:
		return netip.Addr{}, false
	}
}
