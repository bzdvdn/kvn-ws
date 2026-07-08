//go:build darwin

package dnsproxy

import (
	"net"
	"os/exec"
	"strings"
)

// @sk-task dns-setup#T3.2: darwin CleanupStaleDNS via networksetup (AC-009)
func CleanupStaleDNS(proxyListen string) {
	host, _, err := net.SplitHostPort(proxyListen)
	if err != nil {
		host = proxyListen
	}
	if host == "" {
		return
	}

	for _, utun := range findUTUNInterfaces() {
		svc := findServiceForDevice(utun)
		if svc == "" {
			svc = utun
		}
		for _, d := range getDNSServers(svc) {
			if d == host {
				_ = exec.Command("networksetup", "-setdnsservers", svc, "Empty").Run()
				break
			}
		}
	}
}

func findUTUNInterfaces() []string {
	out, err := exec.Command("ifconfig", "-l").Output()
	if err != nil {
		return nil
	}
	var result []string
	for _, name := range strings.Fields(string(out)) {
		if strings.HasPrefix(name, "utun") {
			result = append(result, name)
		}
	}
	return result
}

func findServiceForDevice(device string) string {
	out, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		return ""
	}
	var currentDevice string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Device:") {
			currentDevice = strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
		} else if strings.HasPrefix(line, "Port:") || strings.HasPrefix(line, "Network Service:") || strings.HasPrefix(line, "Service:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && currentDevice == device {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func getDNSServers(svc string) []string {
	out, err := exec.Command("networksetup", "-getdnsservers", svc).Output()
	if err != nil {
		return nil
	}
	var servers []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "There") && line != "Empty" {
			servers = append(servers, line)
		}
	}
	return servers
}
