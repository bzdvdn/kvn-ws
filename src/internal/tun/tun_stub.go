//go:build !linux

package tun

import (
	"errors"
	"net"
)

// @sk-task foundation#T1.3: internal stubs (AC-002)
type tunStub struct{}

func NewTunDevice() TunDevice {
	return &tunStub{}
}

func (t *tunStub) Open() error {
	return errors.New("TUN is not supported on this platform")
}

func (t *tunStub) Close() error {
	return nil
}

func (t *tunStub) Read(buf []byte) (int, error) {
	return 0, errors.New("TUN is not supported on this platform")
}

func (t *tunStub) Write(buf []byte) (int, error) {
	return 0, errors.New("TUN is not supported on this platform")
}

func (t *tunStub) SetIP(ip net.IP, mask *net.IPNet) error {
	return errors.New("TUN is not supported on this platform")
}

func (t *tunStub) SetMTU(mtu int) error {
	return errors.New("TUN is not supported on this platform")
}

func (t *tunStub) SetGateway(gateway net.IP) error {
	return errors.New("TUN is not supported on this platform")
}

func (t *tunStub) RemoveGateway(gateway net.IP) error {
	return errors.New("TUN is not supported on this platform")
}

func (t *tunStub) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	return errors.New("TUN is not supported on this platform")
}

func (t *tunStub) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	return errors.New("TUN is not supported on this platform")
}

func (t *tunStub) DisableGSO() error {
	return nil
}

func SaveDefaultRoute() (net.IP, string, error) {
	return nil, "", errors.New("TUN is not supported on this platform")
}
