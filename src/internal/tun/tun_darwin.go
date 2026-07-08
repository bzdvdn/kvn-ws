//go:build darwin

package tun

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"golang.zx2c4.com/wireguard/tun"
)

const defaultMTU = 1400

// @sk-task mac-tun#T1.1: implement utun core data path (AC-001, AC-009)
type tunDevice struct {
	device    tun.Device
	name      string
	mu        sync.Mutex
	routes    []routeMeta
	dnsMu     sync.Mutex
	dnsSvc    string
	origDNS   []string
}

func NewTunDevice() TunDevice {
	return &tunDevice{}
}

// @sk-task mac-tun#T1.1: implement utun core data path (AC-001, AC-009)
func (t *tunDevice) Open() error {
	dev, err := tun.CreateTUN("KVN", defaultMTU)
	if err != nil {
		return err
	}
	t.device = dev
	name, err := dev.Name()
	if err != nil {
		_ = dev.Close()
		return err
	}
	t.name = name
	return nil
}

// @sk-task mac-tun#T3.2: cleanup on Close (AC-006)
// @sk-task dns-setup#T3.1: restore DNS on Close (AC-004)
func (t *tunDevice) Close() error {
	if t.device == nil {
		return nil
	}
	t.CleanupExcludeRoutes()
	t.restoreDNS()
	return t.device.Close()
}

// @sk-task dns-setup#T3.1: restore original DNS on utun (AC-004)
func (t *tunDevice) restoreDNS() {
	t.dnsMu.Lock()
	svc := t.dnsSvc
	dns := t.origDNS
	t.dnsSvc = ""
	t.origDNS = nil
	t.dnsMu.Unlock()
	if svc == "" {
		return
	}
	if len(dns) > 0 {
		args := append([]string{"-setdnsservers", svc}, dns...)
		_ = exec.Command("networksetup", args...).Run()
	} else {
		_ = exec.Command("networksetup", "-setdnsservers", svc, "Empty").Run()
	}
}

// @sk-task mac-tun#T1.1: implement utun core data path (AC-001, AC-009)
func (t *tunDevice) Read(buf []byte) (int, error) {
	sizes := make([]int, 1)
	_, err := t.device.Read([][]byte{buf}, sizes, 0)
	if err != nil {
		return 0, err
	}
	return sizes[0], nil
}

// @sk-task mac-tun#T1.1: implement utun core data path (AC-001, AC-009)
func (t *tunDevice) Write(buf []byte) (int, error) {
	_, err := t.device.Write([][]byte{buf}, 0)
	if err != nil {
		return 0, err
	}
	return len(buf), nil
}

// @sk-task mac-tun#T2.1: implement SetIP via ifconfig (AC-003)
func (t *tunDevice) SetIP(ip net.IP, mask *net.IPNet) error {
	cmd := exec.Command("ifconfig", t.name, "inet", ip.String(), ip.String(), "netmask", net.IP(mask.Mask).String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ifconfig set ip: %w: %s", err, string(out))
	}
	return nil
}

// @sk-task mac-tun#T2.1: implement SetMTU via ifconfig (AC-003)
func (t *tunDevice) SetMTU(mtu int) error {
	cmd := exec.Command("ifconfig", t.name, "mtu", strconv.Itoa(mtu))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ifconfig set mtu: %w: %s", err, string(out))
	}
	return nil
}

// @sk-task mac-tun#T3.1: implement SetGateway via route (AC-004)
func (t *tunDevice) SetGateway(gateway net.IP) error {
	cmd := exec.Command("route", "add", "-net", "0.0.0.0/0", gateway.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("route add default: %w: %s", err, string(out))
	}
	return nil
}

// @sk-task mac-tun#T3.1: implement RemoveGateway via route (AC-004)
func (t *tunDevice) RemoveGateway(gateway net.IP) error {
	cmd := exec.Command("route", "delete", "-net", "0.0.0.0/0", gateway.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("route delete default: %w: %s", err, string(out))
	}
	return nil
}

// @sk-task mac-tun#T3.2: implement AddExcludeRoute via route (AC-005)
func (t *tunDevice) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	if !prefix.Addr().Is4() {
		return nil
	}
	cmd := exec.Command("route", "add", "-net", cidr, phyGateway.String(), "-ifscope", phyIface)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("route add exclude %s: %w: %s", cidr, err, string(out))
	}
	t.mu.Lock()
	t.routes = append(t.routes, routeMeta{cidr: cidr, gw: phyGateway.String(), iface: phyIface})
	t.mu.Unlock()
	return nil
}

// @sk-task mac-tun#T3.2: implement RemoveExcludeRoute via route (AC-005)
func (t *tunDevice) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	cmd := exec.Command("route", "delete", "-net", cidr, phyGateway.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("route delete exclude %s: %w: %s", cidr, err, string(out))
	}
	return nil
}

// @sk-task mac-tun#T3.2: implement CleanupExcludeRoutes (AC-005, AC-006)
func (t *tunDevice) CleanupExcludeRoutes() {
	t.mu.Lock()
	routes := t.routes
	t.routes = nil
	t.mu.Unlock()
	for _, r := range routes {
		cmd := exec.Command("route", "delete", "-net", r.cidr, r.gw)
		_ = cmd.Run()
	}
}

// @sk-task dns-setup#T3.1: implement SetDNS via networksetup (AC-003)
func (t *tunDevice) SetDNS(dnsServers []string) error {
	svc := t.findDNSService(t.name)
	if svc == "" {
		svc = t.name
	}
	t.dnsMu.Lock()
	t.dnsSvc = svc
	t.origDNS = t.readCurrentDNS(svc)
	t.dnsMu.Unlock()

	args := append([]string{"-setdnsservers", svc}, dnsServers...)
	out, err := exec.Command("networksetup", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("networksetup set dns: %w: %s", err, string(out))
	}
	return nil
}

// @sk-task dns-setup#T3.1: find networksetup service name for utun (AC-003)
func (t *tunDevice) findDNSService(iface string) string {
	out, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		return ""
	}
	return parseHardwarePorts(string(out), iface)
}

// @sk-task dns-setup#T5.1: parse -listallhardwareports output to find service for device
func parseHardwarePorts(output string, iface string) string {
	var currentDevice string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Device:") {
			currentDevice = strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
		} else if strings.HasPrefix(line, "Port:") || strings.HasPrefix(line, "Network Service:") || strings.HasPrefix(line, "Service:") {
			svc := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			if currentDevice == iface {
				return svc
			}
		}
	}
	return ""
}

// @sk-task dns-setup#T3.1: read current DNS servers for a service (AC-004)
func (t *tunDevice) readCurrentDNS(svc string) []string {
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

func (t *tunDevice) DisableGSO() error {
	return nil
}

// @sk-task mac-tun#T3.1: implement SaveDefaultRoute via route -n get default (AC-010)
func SaveDefaultRoute() (net.IP, string, error) {
	cmd := exec.Command("route", "-n", "get", "default")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, "", fmt.Errorf("route get default: %w", err)
	}
	gateway, iface := parseRouteGetOutput(string(out))
	if gateway == nil {
		return nil, "", errors.New("no default route found")
	}
	return gateway, iface, nil
}

func parseRouteGetOutput(output string) (net.IP, string) {
	gateway := parseField(output, "gateway:")
	iface := parseField(output, "interface:")
	if gateway == "" {
		return nil, ""
	}
	ip := net.ParseIP(gateway)
	if ip == nil {
		return nil, ""
	}
	return ip, iface
}

// @sk-task mac-tun#T3.2: implement CleanupStaleExcludeRoutes (AC-005)
func CleanupStaleExcludeRoutes(serverIP string) {
	cmd := exec.Command("route", "delete", "-net", serverIP+"/32")
	_ = cmd.Run()
}

func parseField(output, prefix string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
