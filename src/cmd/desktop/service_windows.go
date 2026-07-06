//go:build windows

package main

import (
	"fmt"
	"net"
	"os/exec"
	"time"
)

const kvnWebPort = 2311

// checkPort dials the local TCP port and returns true if something is listening.
func checkPort() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", kvnWebPort), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Start triggers the scheduled task for kvn-web. If kvn-web is already
// listening on the target port this is a no-op.
func (s *ServiceManager) Start() error {
	if checkPort() {
		return nil
	}
	return exec.Command("schtasks", "/Run", "/TN", "kvn-web").Run()
}

// Stop kills all kvn-web.exe processes by image name.
func (s *ServiceManager) Stop() error {
	return exec.Command("taskkill", "/IM", "kvn-web.exe", "/F").Run()
}

func (s *ServiceManager) Restart() error {
	_ = s.Stop()
	return s.Start()
}
