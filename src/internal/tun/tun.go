// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task docs-and-release#T5.1: fix MTU=0 for TUN creation (AC-008)
// @sk-task tun-data-path#T2.1: single-buf Read (AC-002)
// @sk-task tun-data-path#T2.2: Write offset=12 virtioNetHdrLen (AC-001)
// @sk-task tun-data-path#T6.1: SetIP fix — use ip/mask not subnet/mask (AC-003)

package tun

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

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

func addDefaultRoute(iface string, gateway net.IP) error {
	cmd := exec.Command("ip", "route", "replace", "default", "via", gateway.String(), "dev", iface) // #nosec G204
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("add default route via %s on %s: %w: %s", gateway, iface, err, string(out))
	}
	return nil
}

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

func (t *tunDevice) Write(buf []byte) (int, error) {
	padded := make([]byte, writeHeadroom+len(buf))
	copy(padded[writeHeadroom:], buf)
	_, err := t.device.Write([][]byte{padded}, writeHeadroom)
	if err != nil {
		return 0, err
	}
	return len(buf), nil
}

func (t *tunDevice) SetIP(ip net.IP, mask *net.IPNet) error {
	flush := exec.Command("ip", "-4", "addr", "flush", "dev", t.name) // #nosec G204
	_ = flush.Run()
	cidr := &net.IPNet{IP: ip, Mask: mask.Mask}
	ipCmd := exec.Command("ip", "addr", "add", cidr.String(), "dev", t.name) // #nosec G204 — whitelisted ip binary for TUN
	if out, err := ipCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set ip %s on %s: %w: %s", mask, t.name, err, string(out))
	}
	link := exec.Command("ip", "link", "set", "dev", t.name, "up") // #nosec G204 — whitelisted ip binary for TUN
	if out, err := link.CombinedOutput(); err != nil {
		return fmt.Errorf("link up %s: %w: %s", t.name, err, string(out))
	}
	return nil
}

func (t *tunDevice) SetMTU(mtu int) error {
	cmd := exec.Command("ip", "link", "set", "dev", t.name, "mtu", strconv.Itoa(mtu)) // #nosec G204 — whitelisted ip binary for TUN
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set mtu %d on %s: %w: %s", mtu, t.name, err, string(out))
	}
	return nil
}

func (t *tunDevice) SetGateway(gateway net.IP) error {
	return addDefaultRoute(t.name, gateway)
}

func (t *tunDevice) RemoveGateway(gateway net.IP) error {
	return removeDefaultRoute(t.name, gateway)
}

func (t *tunDevice) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	is6 := ipNet.IP.To4() == nil
	if is6 {
		return nil // skip IPv6 routes — kernel handles link-local/multicast natively
	}
	cmd := exec.Command("ip", "route", "replace", cidr, "via", phyGateway.String(), "dev", phyIface) // #nosec G204
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("add exclude route %s via %s dev %s: %w: %s", cidr, phyGateway, phyIface, err, string(out))
	}
	return nil
}

func (t *tunDevice) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	is6 := ipNet.IP.To4() == nil
	if is6 {
		return nil // skip IPv6 — kernel handles natively
	}
	cmd := exec.Command("ip", "route", "del", cidr, "via", phyGateway.String(), "dev", phyIface) // #nosec G204
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove exclude route %s via %s dev %s: %w: %s", cidr, phyGateway, phyIface, err, string(out))
	}
	return nil
}

func (t *tunDevice) DisableGSO() error {
	// Disable TUN driver offloading via ioctl (the definitive fix).
	// This undoes the TUNSETOFFLOAD that tun.CreateTUN enables.
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

	// Also try via ip link (best-effort, unsupported on some kernels)
	for _, opt := range []string{"gso", "gro"} {
		_ = exec.Command("ip", "link", "set", "dev", t.name, opt, "off").Run() // #nosec G204
	}
	return nil
}

// SaveDefaultRoute returns the current default route's gateway and interface.
func SaveDefaultRoute() (net.IP, string, error) {
	out, err := exec.Command("ip", "route", "show", "default").Output() // #nosec G204
	if err != nil {
		return nil, "", fmt.Errorf("get default route: %w", err)
	}
	// Parse: "default via 192.168.1.1 dev eno1 proto dhcp metric 100"
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
