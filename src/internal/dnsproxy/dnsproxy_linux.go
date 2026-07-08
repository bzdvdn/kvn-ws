//go:build linux

package dnsproxy

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var resolvConfPath = "/etc/resolv.conf"

var systemdResolvedLinks = []string{
	"/run/systemd/resolve/stub-resolv.conf",
	"/usr/lib/systemd/resolv.conf",
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
type ResolvConfBackup struct {
	original    string
	saved       bool
	nameservers []string
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func BackupResolvConf() (*ResolvConfBackup, error) {
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return nil, err
	}
	var nss []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ns := parts[1]
				if !strings.Contains(ns, ":") {
					ns += ":53"
				}
				nss = append(nss, ns)
			}
		}
	}
	return &ResolvConfBackup{original: string(data), saved: true, nameservers: nss}, nil
}

func (b *ResolvConfBackup) Nameservers() []string {
	return b.nameservers
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func (b *ResolvConfBackup) Restore() error {
	if !b.saved {
		return nil
	}
	if isSystemdResolved() {
		return resolvectlRevert()
	}
	return os.WriteFile(resolvConfPath, []byte(b.original), 0o644) // #nosec G306
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func isSystemdResolved() bool {
	target, err := filepath.EvalSymlinks(resolvConfPath)
	if err != nil {
		return false
	}
	for _, p := range systemdResolvedLinks {
		if target == p {
			return true
		}
	}
	return false
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func resolvectlSet(host string) error {
	return exec.Command("resolvectl", "dns", "lo", host).Run() // #nosec G204 — validated as IP by caller
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func resolvectlRevert() error {
	return exec.Command("resolvectl", "revert", "lo").Run()
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func OverrideResolvConf(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return fmt.Errorf("dnsproxy: cannot override resolv.conf with empty address")
	}

	if isSystemdResolved() {
		return resolvectlSet(host)
	}

	nsLine := "nameserver " + host
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return os.WriteFile(resolvConfPath, []byte(nsLine+"\n"), 0o644) // #nosec G306
	}

	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	out = append(out, nsLine)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == nsLine {
			continue
		}
		out = append(out, line)
	}
	content := strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
	return os.WriteFile(resolvConfPath, []byte(content), 0o644) // #nosec G306,G703
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func readNameserver() (string, error) {
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ns := parts[1]
				if !strings.Contains(ns, ":") {
					ns += ":53"
				}
				return ns, nil
			}
		}
	}
	return "", fmt.Errorf("dnsproxy: no nameserver found in /etc/resolv.conf")
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func CleanupStaleDNS(proxyListen string) {
	host, _, err := net.SplitHostPort(proxyListen)
	if err != nil {
		host = proxyListen
	}
	if host == "" {
		return
	}
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "nameserver") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 && parts[1] == host {
				changed = true
				continue
			}
		}
		out = append(out, line)
	}
	if changed {
		content := strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
		_ = os.WriteFile(resolvConfPath, []byte(content), 0o644) // #nosec G306,G703
	}
}
