//go:build linux

package proxy

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

const soOriginalDst = 80

func getOriginalDst(conn net.Conn) (string, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", fmt.Errorf("transparent: not a TCPConn (type: %T)", conn)
	}
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return "", fmt.Errorf("transparent: syscallconn: %w", err)
	}
	var addr [16]byte
	addrLen := uint32(len(addr))
	var errno syscall.Errno
	err = rawConn.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall6(
			syscall.SYS_GETSOCKOPT,
			fd,
			syscall.IPPROTO_IP,
			soOriginalDst,
			uintptr(unsafe.Pointer(&addr[0])), // #nosec G103
			uintptr(unsafe.Pointer(&addrLen)), // #nosec G103
			0,
		)
	})
	if err != nil {
		return "", fmt.Errorf("transparent: control: %w", err)
	}
	if errno != 0 {
		return "", fmt.Errorf("transparent: getsockopt: %w", errno)
	}
	if addrLen < 8 {
		return "", fmt.Errorf("transparent: short addr len %d (family=%d)", addrLen, int(addr[0])|int(addr[1])<<8)
	}
	family := int(addr[0]) | int(addr[1])<<8
	if family != syscall.AF_INET {
		return "", fmt.Errorf("transparent: unsupported addr family %d (AF_INET=%d)", family, syscall.AF_INET)
	}
	port := int(addr[2])<<8 | int(addr[3])
	ip := net.IPv4(addr[4], addr[5], addr[6], addr[7])
	return net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port)), nil
}
