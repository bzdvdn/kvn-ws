//go:build darwin

package tun

import (
	"net"
	"testing"
)

// @sk-test mac-tun#T6.1: TestParseRouteGetOutput (AC-010)
func TestParseRouteGetOutput(t *testing.T) {
	output := `   route to: default
destination: default
       mask: default
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC>
 recvpipe  sendpipe  ssthresh  rtt,msec    rttvar  hopcount      mtu     expire
       0        0        0        0        0        0      1500        0`
	ip, iface := parseRouteGetOutput(output)
	if ip == nil || !ip.Equal(net.ParseIP("192.168.1.1")) {
		t.Errorf("expected gateway 192.168.1.1, got %v", ip)
	}
	if iface != "en0" {
		t.Errorf("expected interface en0, got %q", iface)
	}
}

// @sk-test mac-tun#T6.1: TestParseRouteGetOutputNoGateway (AC-010)
func TestParseRouteGetOutputNoGateway(t *testing.T) {
	ip, iface := parseRouteGetOutput("")
	if ip != nil {
		t.Errorf("expected nil gateway, got %v", ip)
	}
	if iface != "" {
		t.Errorf("expected empty interface, got %q", iface)
	}
}

// @sk-test mac-tun#T6.1: TestParseField (AC-010)
func TestParseField(t *testing.T) {
	output := "gateway: 10.0.0.1\ninterface: en1\nflags: <UP>"
	v := parseField(output, "gateway:")
	if v != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %q", v)
	}
	v = parseField(output, "interface:")
	if v != "en1" {
		t.Errorf("expected en1, got %q", v)
	}
	v = parseField(output, "nonexistent:")
	if v != "" {
		t.Errorf("expected empty, got %q", v)
	}
}

// @sk-test mac-tun#T6.1: TestParseFieldEmpty (AC-010)
func TestParseFieldEmpty(t *testing.T) {
	v := parseField("", "gateway:")
	if v != "" {
		t.Errorf("expected empty, got %q", v)
	}
	v = parseField("\n\n\n", "gateway:")
	if v != "" {
		t.Errorf("expected empty, got %q", v)
	}
}

// @sk-test mac-tun#T6.1: TestParseRouteGetOutputPartial (AC-010)
func TestParseRouteGetOutputPartial(t *testing.T) {
	ip, iface := parseRouteGetOutput("gateway: 10.0.0.1\n")
	if ip == nil {
		t.Error("expected gateway, got nil")
	}
	if !ip.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("expected 10.0.0.1, got %v", ip)
	}
	if iface != "" {
		t.Errorf("expected empty interface, got %q", iface)
	}
}

// @sk-test dns-setup#T5.1: TestParseHardwarePortsDeviceMatch (AC-003)
func TestParseHardwarePortsDeviceMatch(t *testing.T) {
	output := `Hardware Ports:

Hardware Port: Wi-Fi
Device: en0

Hardware Port: USB 10/100/1000 LAN
Device: en5

Hardware Port: KVN
Device: utun1
`
	svc := parseHardwarePorts(output, "utun1")
	if svc != "KVN" {
		t.Errorf("expected 'KVN', got %q", svc)
	}
}

// @sk-test dns-setup#T5.1: TestParseHardwarePortsNoMatch (AC-003)
func TestParseHardwarePortsNoMatch(t *testing.T) {
	output := `Hardware Port: Wi-Fi
Device: en0

Hardware Port: Bluetooth PAN
Device: en2
`
	svc := parseHardwarePorts(output, "utun1")
	if svc != "" {
		t.Errorf("expected empty, got %q", svc)
	}
}

// @sk-test dns-setup#T5.1: TestParseHardwarePortsNetworkServicePrefix (AC-003)
func TestParseHardwarePortsNetworkServicePrefix(t *testing.T) {
	output := `Hardware Port: Thunderbolt Bridge
Device: bridge0

Network Service: KVN VPN
Device: utun2
`
	svc := parseHardwarePorts(output, "utun2")
	if svc != "KVN VPN" {
		t.Errorf("expected 'KVN VPN', got %q", svc)
	}
}

// @sk-test dns-setup#T5.1: TestParseHardwarePortsEmpty (AC-003)
func TestParseHardwarePortsEmpty(t *testing.T) {
	svc := parseHardwarePorts("", "utun0")
	if svc != "" {
		t.Errorf("expected empty, got %q", svc)
	}
}

// @sk-test dns-setup#T5.1: TestParseHardwarePortsServicePrefix (AC-003)
func TestParseHardwarePortsServicePrefix(t *testing.T) {
	output := `Service: KVN Service
Device: utun0
`
	svc := parseHardwarePorts(output, "utun0")
	if svc != "KVN Service" {
		t.Errorf("expected 'KVN Service', got %q", svc)
	}
}
