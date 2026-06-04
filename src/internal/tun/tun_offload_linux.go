//go:build linux

package tun

import "golang.org/x/sys/unix"

func disableTUNOffload(fd uintptr) error {
	return unix.IoctlSetInt(int(fd), unix.TUNSETOFFLOAD, 0)
}
