package tun

import "net"

// @sk-task foundation#T1.3: internal stubs (AC-002)
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

const CIDRMaskV4Bits = 24
const CIDRMaskV4Total = 32
const CIDRMaskV6Bits = 112
const CIDRMaskV6Total = 128
