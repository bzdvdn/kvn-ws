//go:build linux

package dnsproxy

import (
	"errors"
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
	original       string
	saved          bool
	nameservers    []string
	resolvedIfaces []string
	resolvedDNS    map[string][]string
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func BackupResolvConf(iface ...string) (*ResolvConfBackup, error) {
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
	b := &ResolvConfBackup{original: string(data), saved: true, nameservers: nss}
	if isSystemdResolved() {
		b.resolvedIfaces = resolvedIfaces(iface...)
		b.resolvedDNS = make(map[string][]string, len(b.resolvedIfaces))
		for _, name := range b.resolvedIfaces {
			if servers, ok := resolvedDNSFor(name); ok {
				b.resolvedDNS[name] = servers
			}
		}
	}
	return b, nil
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
		return resolvectlRestore(b.resolvedIfaces, b.resolvedDNS)
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
func resolvectlSet(iface, host string) error {
	out, err := exec.Command("resolvectl", "dns", iface, host).CombinedOutput() // #nosec G204 — iface/host validated by caller
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func resolvectlRevert(ifaces []string) error {
	var errs []error
	for _, iface := range ifaces {
		if iface == "" {
			continue
		}
		out, err := exec.Command("resolvectl", "revert", iface).CombinedOutput() // #nosec G204 — iface from route table
		if err != nil {
			errs = append(errs, fmt.Errorf("interface %s: %w: %s", iface, err, strings.TrimSpace(string(out))))
		}
	}
	return errors.Join(errs...)
}

func resolvedDNSFor(iface string) ([]string, bool) {
	out, err := exec.Command("resolvectl", "dns", iface).Output() // #nosec G204 — iface from route table
	if err != nil {
		return nil, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		idx := strings.Index(line, "):")
		if idx < 0 {
			continue
		}
		return strings.Fields(line[idx+2:]), true
	}
	return nil, true
}

func resolvectlRestore(ifaces []string, saved map[string][]string) error {
	var errs []error
	for _, iface := range ifaces {
		if iface == "" {
			continue
		}
		servers, known := saved[iface]
		if !known {
			if err := resolvectlRevert([]string{iface}); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		args := []string{"dns", iface}
		if len(servers) == 0 {
			args = append(args, "")
		} else {
			args = append(args, servers...)
		}
		out, err := exec.Command("resolvectl", args...).CombinedOutput() // #nosec G204 — iface/servers from resolvectl
		if err != nil {
			errs = append(errs, fmt.Errorf("interface %s: %w: %s", iface, err, strings.TrimSpace(string(out))))
		}
	}
	return errors.Join(errs...)
}

// resolvedIfaces returns the interface(s) whose DNS should point at the local
// proxy. systemd-resolved rejects SetLinkDNS for loopback ("Link lo is
// loopback device"), so use the physical default-route interface instead.
func resolvedIfaces(preferred ...string) []string {
	for _, name := range preferred {
		if name != "" && name != "lo" {
			return []string{name}
		}
	}
	out, err := exec.Command("ip", "-o", "route", "show", "default").Output() // #nosec G204
	if err != nil {
		return nil
	}
	return parseDefaultRouteIfaces(string(out))
}

func cleanupStaleResolved(host string) {
	out, err := exec.Command("resolvectl", "dns").Output() // #nosec G204
	if err != nil {
		return
	}
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		openIdx := strings.Index(line, "(")
		closeIdx := strings.Index(line, ")")
		if !strings.HasPrefix(line, "Link ") || openIdx < 0 || closeIdx < openIdx {
			continue
		}
		if !lineHasDNSServer(line[closeIdx+1:], host) {
			continue
		}
		iface := line[openIdx+1 : closeIdx]
		if iface == "" || iface == "lo" {
			continue
		}
		if _, ok := seen[iface]; ok {
			continue
		}
		seen[iface] = struct{}{}
		_ = resolvectlRevert([]string{iface})
	}
}

func lineHasDNSServer(rest, host string) bool {
	idx := strings.Index(rest, ":")
	if idx < 0 {
		return false
	}
	for _, field := range strings.Fields(rest[idx+1:]) {
		if field == host || strings.HasPrefix(field, host+":") {
			return true
		}
	}
	return false
}

func parseDefaultRouteIfaces(out string) []string {
	seen := make(map[string]struct{})
	var ifaces []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] != "dev" {
				continue
			}
			name := fields[i+1]
			if name == "" || name == "lo" {
				break
			}
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				ifaces = append(ifaces, name)
			}
			break
		}
	}
	return ifaces
}

// @sk-task dns-setup#T1.1: extract Linux resolv.conf/systemd-resolved code (AC-005)
func OverrideResolvConf(addr string, iface ...string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return fmt.Errorf("dnsproxy: cannot override resolv.conf with empty address")
	}

	if isSystemdResolved() {
		ifaces := resolvedIfaces(iface...)
		if len(ifaces) == 0 {
			return fmt.Errorf("dnsproxy: systemd-resolved: no non-loopback default-route interface found")
		}
		var errs []error
		var applied []string
		for _, name := range ifaces {
			if err := resolvectlSet(name, host); err != nil {
				errs = append(errs, fmt.Errorf("interface %s: %w", name, err))
				continue
			}
			applied = append(applied, name)
		}
		if len(errs) > 0 {
			_ = resolvectlRevert(applied)
			return errors.Join(errs...)
		}
		return nil
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
	if isSystemdResolved() {
		cleanupStaleResolved(host)
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
