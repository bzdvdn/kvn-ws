//go:build windows

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
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

// startKvnWeb launches kvn-web.exe directly with no visible window.
// The scheduled task handles autostart at logon; this is used only for
// Start/Restart triggered from the desktop UI (error page button).
func startKvnWeb() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	kvnWebPath := filepath.Join(filepath.Dir(exe), "kvn-web.exe")
	cmd := exec.Command(kvnWebPath, "--no-browser", "--port", fmt.Sprint(kvnWebPort))
	// HideWindow prevents a console window from appearing.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	return cmd.Start()
}

// Start launches kvn-web.exe if it is not already listening on the target port.
func (s *ServiceManager) Start() error {
	if checkPort() {
		return nil
	}
	return startKvnWeb()
}

// Stop kills all kvn-web.exe processes by image name.
func (s *ServiceManager) Stop() error {
	return exec.Command("taskkill", "/IM", "kvn-web.exe", "/F").Run()
}

func (s *ServiceManager) Restart() error {
	_ = s.Stop()
	return s.Start()
}
