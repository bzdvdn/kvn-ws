//go:build !linux

package proxy

import (
	"fmt"
	"net"
)

func getOriginalDst(conn net.Conn) (string, error) {
	return "", fmt.Errorf("transparent proxy is not supported on this platform")
}
