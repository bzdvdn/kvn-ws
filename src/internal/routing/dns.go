package routing

import "strings"

// @sk-task dns-routing#T4.1: DNS question parser (AC-001, AC-002)
// ParseDNSQuestion extracts the first QNAME from an IPv4 UDP DNS packet.
// Returns the domain name and true on success.
func ParseDNSQuestion(packet []byte) (string, bool) {
	if len(packet) < 20 {
		return "", false
	}
	verIHL := packet[0]
	ihl := int(verIHL&0x0f) * 4
	if len(packet) < ihl+8+12+5 {
		return "", false
	}
	// Skip IP header + UDP header (8 bytes)
	dnsStart := ihl + 8
	if dnsStart+12 > len(packet) {
		return "", false
	}
	// DNS header is 12 bytes, question section starts after
	qStart := dnsStart + 12
	var labels []string
	pos := qStart
	for {
		if pos >= len(packet) {
			return "", false
		}
		labelLen := int(packet[pos])
		if labelLen == 0 {
			break
		}
		// DNS name compression (0xC0 prefix) — stop, not supported
		if labelLen&0xc0 == 0xc0 {
			break
		}
		if pos+1+labelLen > len(packet) {
			return "", false
		}
		labels = append(labels, string(packet[pos+1:pos+1+labelLen]))
		pos += 1 + labelLen
	}
	if len(labels) == 0 {
		return "", false
	}
	return strings.Join(labels, "."), true
}
