// @sk-task foundation#T1.3: internal stubs (AC-002)

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
	dev, err := tun.CreateTUN("kvn", 0)
	if err != nil {
		return fmt.Errorf("create tun: %w", err)
	}
	t.device = dev
	name, err := dev.Name()
	if err != nil {
		dev.Close()
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
	batchSize := t.device.BatchSize()
	if batchSize < 1 {
		batchSize = 1
	}
	bufs := make([][]byte, batchSize)
	for i := range bufs {
		bufs[i] = buf
	}
	sizes := make([]int, batchSize)
	n, err := t.device.Read(bufs, sizes, 0)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	return sizes[0], nil
}

func (t *tunDevice) Write(buf []byte) (int, error) {
	_, err := t.device.Write([][]byte{buf}, 0)
	if err != nil {
		return 0, err
	}
	return len(buf), nil
}

func (t *tunDevice) SetIP(ip net.IP, mask *net.IPNet) error {
	ipCmd := exec.Command("ip", "addr", "add", mask.String(), "dev", t.name)
	if out, err := ipCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set ip %s on %s: %w: %s", mask, t.name, err, string(out))
	}
	link := exec.Command("ip", "link", "set", "dev", t.name, "up")
	if out, err := link.CombinedOutput(); err != nil {
		return fmt.Errorf("link up %s: %w: %s", t.name, err, string(out))
	}
	return nil
}

func (t *tunDevice) SetMTU(mtu int) error {
	cmd := exec.Command("ip", "link", "set", "dev", t.name, "mtu", strconv.Itoa(mtu))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set mtu %d on %s: %w: %s", mtu, t.name, err, string(out))
	}
	return nil
}
