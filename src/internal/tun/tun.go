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
