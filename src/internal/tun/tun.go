// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task docs-and-release#T5.1: fix MTU=0 for TUN creation (AC-008)
// @sk-task tun-data-path#T2.1: single-buf Read (AC-002)
// @sk-task tun-data-path#T2.2: Write offset=12 virtioNetHdrLen (AC-001)
// @sk-task tun-data-path#T6.1: SetIP fix — use ip/mask not subnet/mask (AC-003)
// @sk-task arch-refactoring#T3.4: netlink for addr/link, exec.Command for routes (AC-007)
// Route management uses exec.Command("ip") because netlink.RouteDel partial matching
// can delete the physical default route instead of the TUN route.

package tun

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/tun"
)

// @sk-task core-tunnel-mvp#T1.2: TunDevice interface (AC-001)
type TunDevice interface {
	Open() error
	Close() error
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	SetIP(ip net.IP, mask *net.IPNet) error
	SetMTU(mtu int) error
	SetGateway(gateway net.IP) error
	RemoveGateway(gateway net.IP) error
	AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error
	RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error
	DisableGSO() error
}

type tunDevice struct {
	name   string
	device tun.Device
}

func NewTunDevice() TunDevice {
	return &tunDevice{}
}

func (t *tunDevice) Open() error {
	dev, err := tun.CreateTUN("kvn", 1400)
	if err != nil {
		return fmt.Errorf("create tun: %w", err)
	}
	t.device = dev
	name, err := dev.Name()
	if err != nil {
		_ = dev.Close()
		return fmt.Errorf("get tun name: %w", err)
	}
	t.name = name
	return nil
}

func (t *tunDevice) Close() error {
	if t.device == nil {
		return nil
	}
	return t.device.Close()
}

func (t *tunDevice) Read(buf []byte) (int, error) {
	sizes := make([]int, 1)
	n, err := t.device.Read([][]byte{buf}, sizes, 0)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	return sizes[0], nil
}

// @sk-task arch-refactoring#T3.4: exec.Command add default route (AC-007)
func addDefaultRoute(iface string, gateway net.IP) error {
	cmd := exec.Command("ip", "route", "replace", "default", "via", gateway.String(), "dev", iface) // #nosec G204
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("add default route via %s on %s: %w: %s", gateway, iface, err, string(out))
	}
	return nil
}

// @sk-task arch-refactoring#T3.4: exec.Command remove default route (AC-007)
func removeDefaultRoute(iface string, gateway net.IP) error {
	cmd := exec.Command("ip", "route", "del", "default", "via", gateway.String(), "dev", iface) // #nosec G204
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove default route via %s on %s: %w: %s", gateway, iface, err, string(out))
	}
	return nil
}

const virtioNetHdrLen = 12
const writeHeadroom = virtioNetHdrLen

const CIDRMaskV4Bits = 24
const CIDRMaskV4Total = 32
const CIDRMaskV6Bits = 112
const CIDRMaskV6Total = 128

func (t *tunDevice) Write(buf []byte) (int, error) {
	padded := make([]byte, writeHeadroom+len(buf))
	copy(padded[writeHeadroom:], buf)
	_, err := t.device.Write([][]byte{padded}, writeHeadroom)
	if err != nil {
		return 0, err
	}
	return len(buf), nil
}

// @sk-task arch-refactoring#T3.4: netlink AddrList+AddrDel (AC-007)
func flushV4Addrs(link netlink.Link) error {
	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	for _, a := range addrs {
		if err := netlink.AddrDel(link, &a); err != nil {
			return err
		}
	}
	return nil
}

// @sk-task arch-refactoring#T3.4: netlink AddrAdd+LinkSetUp (AC-007)
func (t *tunDevice) SetIP(ip net.IP, mask *net.IPNet) error {
	link, err := netlink.LinkByName(t.name)
	if err != nil {
		return fmt.Errorf("get link %s: %w", t.name, err)
	}
	if err := flushV4Addrs(link); err != nil {
		return fmt.Errorf("flush addr on %s: %w", t.name, err)
	}
	cidr := &net.IPNet{IP: ip, Mask: mask.Mask}
	addr := &netlink.Addr{IPNet: cidr}
	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("set ip %s on %s: %w", mask, t.name, err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("link up %s: %w", t.name, err)
	}
	return nil
}

// @sk-task arch-refactoring#T3.4: netlink LinkSetMTU (AC-007)
func (t *tunDevice) SetMTU(mtu int) error {
	link, err := netlink.LinkByName(t.name)
	if err != nil {
		return fmt.Errorf("get link %s: %w", t.name, err)
	}
	if err := netlink.LinkSetMTU(link, mtu); err != nil {
		return fmt.Errorf("set mtu %d on %s: %w", mtu, t.name, err)
	}
	return nil
}

func (t *tunDevice) SetGateway(gateway net.IP) error {
	return addDefaultRoute(t.name, gateway)
}

func (t *tunDevice) RemoveGateway(gateway net.IP) error {
	return removeDefaultRoute(t.name, gateway)
}

// @sk-task arch-refactoring#T3.4: exec.Command add exclude route (AC-007)
func (t *tunDevice) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	if ipNet.IP.To4() == nil {
		return nil
	}
	cmd := exec.Command("ip", "route", "replace", cidr, "via", phyGateway.String(), "dev", phyIface) // #nosec G204
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("add exclude route %s via %s dev %s: %w: %s", cidr, phyGateway, phyIface, err, string(out))
	}
	return nil
}

// @sk-task arch-refactoring#T3.4: exec.Command remove exclude route (AC-007)
func (t *tunDevice) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	if ipNet.IP.To4() == nil {
		return nil
	}
	cmd := exec.Command("ip", "route", "del", cidr, "via", phyGateway.String(), "dev", phyIface) // #nosec G204
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove exclude route %s via %s dev %s: %w: %s", cidr, phyGateway, phyIface, err, string(out))
	}
	return nil
}

func (t *tunDevice) DisableGSO() error {
	if t.device != nil {
		f := t.device.File()
		if f != nil {
			sc, err := f.SyscallConn()
			if err == nil {
				_ = sc.Control(func(fd uintptr) {
					_ = disableTUNOffload(fd)
				})
			}
		}
	}
	return nil
}

// SaveDefaultRoute returns the current default route's gateway and interface.
// @sk-task arch-refactoring#T3.4: exec.Command save default route (AC-007)
func SaveDefaultRoute() (net.IP, string, error) {
	out, err := exec.Command("ip", "route", "show", "default").Output() // #nosec G204
	if err != nil {
		return nil, "", fmt.Errorf("get default route: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 3 || fields[0] != "default" {
		return nil, "", fmt.Errorf("unexpected default route format: %s", strings.TrimSpace(string(out)))
	}
	gateway := net.ParseIP(fields[2])
	if gateway == nil {
		return nil, "", fmt.Errorf("parse gateway %s", fields[2])
	}
	iface := ""
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			iface = fields[i+1]
			break
		}
	}
	if iface == "" {
		return nil, "", fmt.Errorf("no dev in default route: %s", strings.TrimSpace(string(out)))
	}
	return gateway, iface, nil
}
