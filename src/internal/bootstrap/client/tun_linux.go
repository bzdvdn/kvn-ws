//go:build linux

package client

import (
	"net"
	"strconv"

	"github.com/bzdvdn/kvn-ws/src/internal/dnsproxy"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// @sk-task dns-setup#T4.1: linux setupDNS saves resolv.conf + returns resolvers (AC-005)
func setupDNS() (backup interface{}, resolvers []string) {
	b, _ := dnsproxy.BackupResolvConf()
	if b == nil {
		return nil, nil
	}
	for _, ns := range b.Nameservers() {
		host, _, err := net.SplitHostPort(ns)
		if err != nil {
			host = ns
		}
		ip := net.ParseIP(host)
		if ip != nil && ip.IsLoopback() {
			continue
		}
		resolvers = append(resolvers, ns)
	}
	return b, resolvers
}

// @sk-task dns-setup#T4.1: linux applyDNS overrides resolv.conf + adds resolver exclude routes (AC-005)
func applyDNS(backup interface{}, tunDev tun.TunDevice, listen string, phyGW net.IP, phyIface string, resolvers []string) {
	b, _ := backup.(*dnsproxy.ResolvConfBackup)
	if b == nil {
		return
	}
	_ = dnsproxy.OverrideResolvConf(listen)
	if tunDev == nil {
		return
	}
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
		_ = tunDev.AddExcludeRoute(host+"/"+strconv.Itoa(bits), phyGW, phyIface)
	}
}

// @sk-task dns-setup#T4.1: linux restoreDNS restores resolv.conf (AC-005)
func restoreDNS(backup interface{}) {
	if b, ok := backup.(*dnsproxy.ResolvConfBackup); ok && b != nil {
		_ = b.Restore()
	}
}
