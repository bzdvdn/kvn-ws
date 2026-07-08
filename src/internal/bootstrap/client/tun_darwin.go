//go:build darwin

package client

import (
	"net"
	"strconv"

	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// @sk-task dns-setup#T4.2: darwin setupDNS no-op (DNS restored in Close) (AC-007)
func setupDNS() (interface{}, []string) { return nil, nil }

// @sk-task dns-setup#T4.2: darwin applyDNS via networksetup + exclude routes for resolvers (AC-007)
func applyDNS(_ interface{}, tunDev tun.TunDevice, listen string, phyGW net.IP, phyIface string, resolvers []string) {
	if tunDev == nil {
		return
	}
	dnsIP := listen
	if host, _, err := net.SplitHostPort(listen); err == nil {
		dnsIP = host
	}
	_ = tunDev.SetDNS([]string{dnsIP})
	for _, ns := range resolvers {
		host, _, err := net.SplitHostPort(ns)
		if err != nil {
			host = ns
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.IsPrivate() || ip.IsLoopback() {
			continue
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		_ = tunDev.AddExcludeRoute(ip.String()+"/"+strconv.Itoa(bits), phyGW, phyIface)
	}
}

// @sk-task dns-setup#T4.2: darwin restoreDNS no-op (handled by Close) (AC-007)
func restoreDNS(_ interface{}) {}
