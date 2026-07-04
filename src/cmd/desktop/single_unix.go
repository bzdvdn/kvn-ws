//go:build linux || darwin

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

var guardFile *os.File

// @sk-task desktop-tray#T4.1: unix single-instance pidfile+flock guard (AC-006)
func guardSingleInstance() bool {
	f, err := os.OpenFile("/tmp/kvn-desktop.pid", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return true
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		var buf [16]byte
		n, _ := f.Read(buf[:])
		f.Close()
		if n > 0 {
			pidStr := strings.TrimSpace(string(buf[:n]))
			if pid, parseErr := strconv.Atoi(pidStr); parseErr == nil {
				if proc, findErr := os.FindProcess(pid); findErr == nil {
					if proc.Signal(syscall.Signal(0)) == nil {
						return false
					}
				}
			}
		}
		f, err = os.OpenFile("/tmp/kvn-desktop.pid", os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return true
		}
		syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	}

	f.Truncate(0)
	f.Seek(0, 0)
	fmt.Fprintf(f, "%d\n", os.Getpid())
	guardFile = f
	return true
}

func releaseGuard() {
	if guardFile != nil {
		guardFile.Close()
		guardFile = nil
	}
}
