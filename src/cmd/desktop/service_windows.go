//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func (s *ServiceManager) Start() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	kvnWebPath := filepath.Join(filepath.Dir(exe), "kvn-web.exe")
	cmd := exec.Command(kvnWebPath, "--no-browser", "--port", "2311")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

func (s *ServiceManager) Stop() error {
	return exec.Command("taskkill", "/IM", "kvn-web.exe", "/F").Run()
}

func (s *ServiceManager) Restart() error {
	_ = s.Stop()
	return s.Start()
}
