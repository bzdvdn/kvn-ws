package routing

import (
	"encoding/binary"
	"log"
	"net/netip"
)

const (
	ipHeaderDstOffset  = 16
	ipv6HeaderDstOffset = 24
)

// @sk-task routing-split-tunnel#T2.4: tun router (AC-001)
type TunRouter struct {
	ruleSet    *RuleSet
	tunRead    func([]byte) (int, error)
	tunWrite   func([]byte) (int, error)
	tunnelSend func([]byte) error
	dnsOverride bool
}

// @sk-task routing-split-tunnel#T2.4: new tun router (AC-001)
func NewTunRouter(rs *RuleSet, tunRead, tunWrite func([]byte) (int, error), tunnelSend func([]byte) error) *TunRouter {
	return &TunRouter{
		ruleSet:    rs,
		tunRead:    tunRead,
		tunWrite:   tunWrite,
		tunnelSend: tunnelSend,
	}
}

// @sk-task routing-split-tunnel#T3.3: set dns override (AC-008)
func (r *TunRouter) SetDNSOverride(enabled bool) {
	r.dnsOverride = enabled
}

// @sk-task routing-split-tunnel#T2.4: route one packet (AC-001)
// @sk-task routing-split-tunnel#T3.3: dns override route (AC-008)
// @sk-task ipv6-dual-stack#T3.1: dual-stack routing dispatch (AC-005)
func (r *TunRouter) RoutePacket(packet []byte) error {
	if len(packet) < 1 {
		return r.sendDirect(packet)
	}
	ipVersion := packet[0] >> 4
	if r.dnsOverride && ipVersion == 4 && isDNSQuery(packet) {
		log.Printf("[routing] dns override: routing through tunnel")
		return r.sendTunnel(packet)
	}
	switch ipVersion {
	case 4:
		dstIP, err := parseDstIP(packet)
		if err != nil {
			log.Printf("[routing] parse dst ip: %v", err)
			return r.sendDirect(packet)
		}
		return r.routeByRule(dstIP, packet)
	case 6:
		dstIP, err := parseDstIP6(packet)
		if err != nil {
			log.Printf("[routing] parse dst ip6: %v", err)
			return r.sendDirect(packet)
		}
		return r.routeByRule(dstIP, packet)
	default:
		return r.sendDirect(packet)
	}
}

func (r *TunRouter) routeByRule(dstIP netip.Addr, packet []byte) error {
	action := r.ruleSet.Route(dstIP)
	switch action {
	case RouteServer:
		return r.sendTunnel(packet)
	case RouteDirect:
		return r.sendDirect(packet)
	default:
		return r.sendDirect(packet)
	}
}

// @sk-task routing-split-tunnel#T3.3: dns query detection (AC-008)
func isDNSQuery(packet []byte) bool {
	if len(packet) < 20 {
		return false
	}
	// IP protocol field at byte 9
	proto := packet[9]
	if proto != 17 { // UDP
		return false
	}
	verIHL := packet[0]
	ihl := int(verIHL&0x0f) * 4
	if len(packet) < ihl+4 {
		return false
	}
	// UDP dst port
	dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
	return dstPort == 53
}

func parseDstIP(packet []byte) (netip.Addr, error) {
	if len(packet) < 20 {
		return netip.Addr{}, nil
	}
	ip := packet[ipHeaderDstOffset : ipHeaderDstOffset+4]
	if len(ip) < 4 {
		return netip.Addr{}, nil
	}
	return netip.AddrFrom4([4]byte{ip[0], ip[1], ip[2], ip[3]}), nil
}

// @sk-task ipv6-dual-stack#T3.1: parse IPv6 destination address (AC-005)
func parseDstIP6(packet []byte) (netip.Addr, error) {
	if len(packet) < 40 {
		return netip.Addr{}, nil
	}
	ip := packet[ipv6HeaderDstOffset : ipv6HeaderDstOffset+16]
	if len(ip) < 16 {
		return netip.Addr{}, nil
	}
	var addr [16]byte
	copy(addr[:], ip)
	return netip.AddrFrom16(addr), nil
}

// parseDstIPPort returns destination IP and port from TCP/UDP header
// nolint: unused
func parseDstIPPort(packet []byte) (netip.Addr, int, bool) {
	if len(packet) < 20 {
		return netip.Addr{}, 0, false
	}
	verIHL := packet[0]
	ihl := int(verIHL&0x0f) * 4
	if len(packet) < ihl+4 {
		return netip.Addr{}, 0, false
	}
	ip := packet[ihl-4 : ihl]
	addr := netip.AddrFrom4([4]byte{ip[0], ip[1], ip[2], ip[3]})
	proto := packet[9]
	if proto == 6 || proto == 17 {
		if len(packet) >= ihl+4 {
			port := binary.BigEndian.Uint16(packet[ihl+0 : ihl+2])
			return addr, int(port), true
		}
	}
	return addr, 0, true
}

func (r *TunRouter) sendDirect(packet []byte) error {
	_, err := r.tunWrite(packet)
	return err
}

func (r *TunRouter) sendTunnel(packet []byte) error {
	return r.tunnelSend(packet)
}
