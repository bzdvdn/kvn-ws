//go:build !linux

package tun

func disableTUNOffload(fd uintptr) error {
	return nil
}
