package routing

import (
	"testing"
)

// buildDNSQuery builds a minimal IPv4 UDP DNS query packet for the given domain and qtype.
func buildDNSQuery(domain string, qtype uint16) []byte {
	labels := make([]byte, 0, len(domain))
	for _, label := range splitDomain(domain) {
		labels = append(labels, byte(len(label)))
		labels = append(labels, []byte(label)...)
	}
	labels = append(labels, 0)

	dnsPktLen := 12 + len(labels) + 4
	udpLen := 8 + dnsPktLen
	ipIHL := 5
	totalLen := ipIHL*4 + udpLen

	pkt := make([]byte, totalLen)
	pkt[0] = byte(0x40 | ipIHL) // ver=4, ihl=5
	pkt[2] = byte(totalLen >> 8)
	pkt[3] = byte(totalLen)
	pkt[9] = 17 // UDP

	ihlBytes := ipIHL * 4
	// UDP header
	pkt[ihlBytes+0] = 0x00 // src port high
	pkt[ihlBytes+1] = 0x35 // src port 53
	pkt[ihlBytes+2] = 0x00 // dst port high
	pkt[ihlBytes+3] = 0x35 // dst port 53
	pkt[ihlBytes+4] = byte(udpLen >> 8)
	pkt[ihlBytes+5] = byte(udpLen)

	dnsStart := ihlBytes + 8
	// DNS header: ID=0x1234, flags=0x0100 (standard query), QDCOUNT=1
	pkt[dnsStart+0] = 0x12
	pkt[dnsStart+1] = 0x34
	pkt[dnsStart+2] = 0x01
	pkt[dnsStart+3] = 0x00
	pkt[dnsStart+4] = 0x00
	pkt[dnsStart+5] = 0x01 // QDCOUNT=1
	// Question
	qStart := dnsStart + 12
	copy(pkt[qStart:], labels)
	qtStart := qStart + len(labels)
	pkt[qtStart+0] = byte(qtype >> 8) // QTYPE
	pkt[qtStart+1] = byte(qtype)
	pkt[qtStart+2] = 0x00 // QCLASS=IN
	pkt[qtStart+3] = 0x01

	return pkt
}

func splitDomain(domain string) []string {
	var labels []string
	start := 0
	for i := 0; i < len(domain); i++ {
		if domain[i] == '.' {
			labels = append(labels, domain[start:i])
			start = i + 1
		}
	}
	if start < len(domain) {
		labels = append(labels, domain[start:])
	}
	return labels
}

// @sk-test dns-routing#T7.3: ParseDNSQuestion valid A query (AC-001)
func TestParseDNSQuestionValid(t *testing.T) {
	pkt := buildDNSQuery("hh.ru", 1) // A record
	qname, ok := ParseDNSQuestion(pkt)
	if !ok {
		t.Fatal("expected parse success")
	}
	if qname != "hh.ru" {
		t.Errorf("expected hh.ru, got %s", qname)
	}
}

// @sk-test dns-routing#T7.3: ParseDNSQuestion multi-label domain (AC-001)
func TestParseDNSQuestionMultiLabel(t *testing.T) {
	pkt := buildDNSQuery("api.ozon.ru", 1)
	qname, ok := ParseDNSQuestion(pkt)
	if !ok {
		t.Fatal("expected parse success")
	}
	if qname != "api.ozon.ru" {
		t.Errorf("expected api.ozon.ru, got %s", qname)
	}
}

// @sk-test dns-routing#T7.3: ParseDNSQuestion truncated packet (AC-003)
func TestParseDNSQuestionTruncated(t *testing.T) {
	_, ok := ParseDNSQuestion([]byte{0, 1, 2})
	if ok {
		t.Error("expected parse failure for truncated packet")
	}
}

// @sk-test dns-routing#T7.3: ParseDNSQuestion non-DNS packet (AC-003)
func TestParseDNSQuestionNoQuestion(t *testing.T) {
	pkt := make([]byte, 28)
	pkt[0] = 0x45 // IPv4, IHL=5
	pkt[9] = 17   // UDP
	pkt[22] = 0
	pkt[23] = 53 // dst port 53
	// valid IP+UDP header but no DNS question
	_, ok := ParseDNSQuestion(pkt)
	if ok {
		t.Error("expected parse failure for packet with no DNS question")
	}
}
