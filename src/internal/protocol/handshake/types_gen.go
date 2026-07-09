// Code generated from protocol/handshake.yaml. DO NOT EDIT.
package handshake

import "net"

// @sk-task kvn-android#T1.1: protocol handshake constants (AC-004)
const (
	FlagIPv6 = 0x01
	FlagMTU = 0x02
	DefaultMTU = 1500
	CryptoTag = 0x09
	GatewayTag = 0x0A
	TransportTag = 0x0B
	ProtoVersion = 0x02
	SessionIDLen = 16
)

// @sk-task kvn-android#T1.1: generated ClientHello (AC-004)
type ClientHello struct {
	ProtoVersion byte
	Ipv6 bool
	Token string
	Mtu int
	Transport string
}
// @sk-task kvn-android#T1.1: generated ServerHello (AC-004)
type ServerHello struct {
	SessionId string
	AssignedIp net.IP
	AssignedIpv6 net.IP
	Mtu int
	CryptoSalt []byte
	GatewayIp net.IP
	Transport string
}
// @sk-task kvn-android#T1.1: generated AuthError (AC-004)
type AuthError struct {
	Reason string
}
