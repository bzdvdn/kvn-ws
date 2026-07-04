//go:build linux || darwin

package main

import (
	"os/exec"
	"runtime"
)

// @sk-task kvn-desktop#T1.1: unix service start/stop/restart (AC-001, AC-002, AC-009, AC-012)
func (s *ServiceManager) Start() error {
	if runtime.GOOS == "linux" {
		return exec.Command("pkexec", "systemctl", "start", "kvn-web").Run()
	}
	return exec.Command("osascript", "-e",
		`do shell script "launchctl load /Library/LaunchDaemons/kvn-web.plist" with administrator privileges`).Run()
}

func (s *ServiceManager) Stop() error {
	if runtime.GOOS == "linux" {
		return exec.Command("pkexec", "systemctl", "stop", "kvn-web").Run()
	}
	return exec.Command("osascript", "-e",
		`do shell script "launchctl unload /Library/LaunchDaemons/kvn-web.plist" with administrator privileges`).Run()
}

func (s *ServiceManager) Restart() error {
	if runtime.GOOS == "linux" {
		return exec.Command("pkexec", "systemctl", "restart", "kvn-web").Run()
	}
	_ = s.Stop()
	return s.Start()
}
