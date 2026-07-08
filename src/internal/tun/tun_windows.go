//go:build windows

package tun

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

const defaultMTU = 1400

// @sk-task win-tun#T1.1: implement Wintun core data path (AC-002, AC-009)
type tunDevice struct {
	device    tun.Device
	name      string
	mu        sync.Mutex
	routes    []routeMeta
	closeOnce sync.Once
}

func NewTunDevice() TunDevice {
	return &tunDevice{}
}

// @sk-task win-tun#T4.1: implement deterministic GUID via UUIDv5 (AC-006)
func deterministicGUID(name string) windows.GUID {
	var dnsNamespace = [...]byte{
		0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1,
		0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
	}
	h := sha1.New()
	h.Write(dnsNamespace[:])
	h.Write([]byte(name))
	sum := h.Sum(nil)
	return windows.GUID{
		Data1: binary.BigEndian.Uint32(sum[0:4]),
		Data2: binary.BigEndian.Uint16(sum[4:6]),
		Data3: binary.BigEndian.Uint16(sum[6:8]) & 0x0fff | 0x5000,
		Data4: func() (b [8]byte) {
			copy(b[:], sum[8:16])
			b[0] = b[0] & 0x3f | 0x80
			return
		}(),
	}
}

// @sk-task win-tun#T1.1: implement Wintun core data path (AC-002, AC-009)
func (t *tunDevice) Open() error {
	guid := deterministicGUID("kvn-ws")
	dev, err := tun.CreateTUNWithRequestedGUID("KVN", &guid, defaultMTU)
	if err != nil {
		return err
	}
	t.device = dev
	name, err := dev.Name()
	if err != nil {
		_ = dev.Close()
		return err
	}
	t.name = name
	return nil
}

// @sk-task win-tun#T4.2: implement graceful shutdown (AC-007)
func (t *tunDevice) Close() error {
	var err error
	t.closeOnce.Do(func() {
		t.CleanupExcludeRoutes()
		if t.device == nil {
			return
		}
		err = t.device.Close()
	})
	return err
}

// @sk-task win-tun#T1.1: implement Wintun core data path (AC-002, AC-009)
func (t *tunDevice) Read(buf []byte) (int, error) {
	sizes := make([]int, 1)
	_, err := t.device.Read([][]byte{buf}, sizes, 0)
	if err != nil {
		return 0, err
	}
	return sizes[0], nil
}

// @sk-task win-tun#T1.1: implement Wintun core data path (AC-002, AC-009)
func (t *tunDevice) Write(buf []byte) (int, error) {
	// Windows Wintun does not use virtio-net-header; write raw IP packet directly
	_, err := t.device.Write([][]byte{buf}, 0)
	if err != nil {
		return 0, err
	}
	return len(buf), nil
}

func (t *tunDevice) luid() (winipcfg.LUID, error) {
	nativeTun, ok := t.device.(*tun.NativeTun)
	if !ok {
		return 0, errors.New("tun device is not a NativeTun")
	}
	return winipcfg.LUID(nativeTun.LUID()), nil
}

// @sk-task win-tun#T2.1: implement SetIP via winipcfg (AC-001)
func (t *tunDevice) SetIP(ip net.IP, mask *net.IPNet) error {
	luid, err := t.luid()
	if err != nil {
		return err
	}
	if err := luid.FlushIPAddresses(windows.AF_INET); err != nil {
		return err
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return errors.New("invalid IP address")
	}
	ones, _ := mask.Mask.Size()
	prefix := netip.PrefixFrom(addr, ones)
	return luid.SetIPAddressesForFamily(windows.AF_INET, []netip.Prefix{prefix})
}

// @sk-task win-tun#T2.1: implement SetMTU via winipcfg (AC-005)
func (t *tunDevice) SetMTU(mtu int) error {
	luid, err := t.luid()
	if err != nil {
		return err
	}
	ipif, err := luid.IPInterface(windows.AF_INET)
	if err != nil {
		return err
	}
	ipif.NLMTU = uint32(mtu)
	return ipif.Set()
}

// @sk-task win-tun#T3.1: implement SetGateway via winipcfg (AC-003)
func (t *tunDevice) SetGateway(gateway net.IP) error {
	luid, err := t.luid()
	if err != nil {
		return err
	}
	addr, ok := netip.AddrFromSlice(gateway)
	if !ok {
		return errors.New("invalid gateway IP")
	}
	return luid.AddRoute(
		netip.PrefixFrom(netip.IPv4Unspecified(), 0),
		addr,
		0,
	)
}

// @sk-task win-tun#T3.1: implement RemoveGateway via winipcfg (AC-003)
func (t *tunDevice) RemoveGateway(gateway net.IP) error {
	luid, err := t.luid()
	if err != nil {
		return err
	}
	addr, ok := netip.AddrFromSlice(gateway)
	if !ok {
		return errors.New("invalid gateway IP")
	}
	return luid.DeleteRoute(
		netip.PrefixFrom(netip.IPv4Unspecified(), 0),
		addr,
	)
}

// @sk-task win-tun#T3.2: implement AddExcludeRoute via winipcfg (AC-004)
func (t *tunDevice) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	if !prefix.Addr().Is4() {
		return nil
	}
	luid, err := parseLUID(phyIface)
	if err != nil {
		return fmt.Errorf("parse iface luid %s: %w", phyIface, err)
	}
	gw, ok := netip.AddrFromSlice(phyGateway)
	if !ok {
		return errors.New("invalid phyGateway IP")
	}
	if err := luid.AddRoute(prefix, gw, 0); err != nil {
		return fmt.Errorf("add exclude route %s: %w", cidr, err)
	}
	t.mu.Lock()
	t.routes = append(t.routes, routeMeta{cidr: cidr, gw: phyGateway.String(), iface: phyIface})
	t.mu.Unlock()
	return nil
}

// @sk-task win-tun#T3.2: implement RemoveExcludeRoute via winipcfg (AC-004)
func (t *tunDevice) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	if !prefix.Addr().Is4() {
		return nil
	}
	luid, err := parseLUID(phyIface)
	if err != nil {
		return fmt.Errorf("parse iface luid %s: %w", phyIface, err)
	}
	gw, ok := netip.AddrFromSlice(phyGateway)
	if !ok {
		return errors.New("invalid phyGateway IP")
	}
	return luid.DeleteRoute(prefix, gw)
}

// @sk-task win-tun#T3.2: implement CleanupExcludeRoutes (AC-004)
func (t *tunDevice) CleanupExcludeRoutes() {
	t.mu.Lock()
	routes := t.routes
	t.routes = nil
	t.mu.Unlock()
	for _, r := range routes {
		prefix, err := netip.ParsePrefix(r.cidr)
		if err != nil {
			continue
		}
		luid, err := parseLUID(r.iface)
		if err != nil {
			continue
		}
		gw, _ := netip.ParseAddr(r.gw)
		_ = luid.DeleteRoute(prefix, gw)
	}
}

// @sk-task dns-setup#T2.1: implement SetDNS via luid.SetDNS (AC-001)
func (t *tunDevice) SetDNS(dnsServers []string) error {
	luid, err := t.luid()
	if err != nil {
		return err
	}
	var addrs []netip.Addr
	for _, s := range dnsServers {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return fmt.Errorf("parse dns server %s: %w", s, err)
		}
		addrs = append(addrs, addr)
	}
	return luid.SetDNS(windows.AF_INET, addrs, nil)
}

func (t *tunDevice) DisableGSO() error {
	return nil
}

func parseLUID(s string) (winipcfg.LUID, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return winipcfg.LUID(v), nil
}

// @sk-task win-tun#T3.1: implement SaveDefaultRoute via winipcfg (AC-010)
func SaveDefaultRoute() (net.IP, string, error) {
	routes, err := winipcfg.GetIPForwardTable2(windows.AF_INET)
	if err != nil {
		return nil, "", fmt.Errorf("get ip forward table: %w", err)
	}
	var best *winipcfg.MibIPforwardRow2
	for i := range routes {
		if routes[i].DestinationPrefix.PrefixLength != 0 {
			continue
		}
		if best == nil || routes[i].Metric < best.Metric {
			best = &routes[i]
		}
	}
	if best == nil {
		return nil, "", errors.New("no default route found")
	}
	gw := best.NextHop.Addr()
	luidStr := strconv.FormatUint(uint64(best.InterfaceLUID), 10)
	return net.IP(gw.AsSlice()), luidStr, nil
}

// @sk-task win-tun#T3.2: implement CleanupStaleExcludeRoutes (AC-004)
func CleanupStaleExcludeRoutes(serverIP string) {
	prefix, err := netip.ParsePrefix(serverIP + "/32")
	if err != nil {
		return
	}
	if !prefix.Addr().Is4() {
		prefix, _ = netip.ParsePrefix(serverIP + "/128")
	}
	routes, err := winipcfg.GetIPForwardTable2(windows.AF_INET)
	if err != nil {
		return
	}
	for _, r := range routes {
		dst := r.DestinationPrefix.RawPrefix.Addr()
		bits := int(r.DestinationPrefix.PrefixLength)
		if dst == prefix.Addr() && bits == prefix.Bits() {
			_ = r.InterfaceLUID.DeleteRoute(prefix, r.NextHop.Addr())
		}
	}
}
