//go:build windows

package dnsproxy

import (
	"net"
	"net/netip"
	"strings"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// @sk-task dns-setup#T2.2: windows CleanupStaleDNS via winipcfg (AC-008)
func CleanupStaleDNS(proxyListen string) {
	host, _, err := net.SplitHostPort(proxyListen)
	if err != nil {
		host = proxyListen
	}
	if host == "" {
		return
	}
	targetIP := netip.MustParseAddr(host)

	adapters, err := winipcfg.GetAdaptersAddresses(windows.AF_INET, winipcfg.GAAFlagDefault)
	if err != nil {
		return
	}
	for _, a := range adapters {
		name := a.FriendlyName()
		if !strings.HasPrefix(name, "KVN") {
			continue
		}
		dnsAddrs, err := a.LUID.DNS()
		if err != nil {
			continue
		}
		for _, d := range dnsAddrs {
			if d == targetIP {
				_ = a.LUID.FlushDNS(windows.AF_INET)
				return
			}
		}
	}
}
