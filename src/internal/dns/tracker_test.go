package dns

import (
	"encoding/binary"
	"net/netip"
	"testing"
	"time"
)

// @sk-test dns-response-tracker#T1.1: TestTrackerTrackAndLookup (AC-001)
func TestTrackerTrackAndLookup(t *testing.T) {
	tr := NewTracker(60 * time.Second)
	ip := netip.MustParseAddr("95.163.249.123")
	tr.Track("ozon.ru", []netip.Addr{ip})

	domain, ok := tr.Lookup(ip)
	if !ok {
		t.Fatal("expected lookup hit")
	}
	if domain != "ozon.ru" {
		t.Errorf("expected ozon.ru, got %s", domain)
	}

	_, ok = tr.Lookup(netip.MustParseAddr("1.1.1.1"))
	if ok {
		t.Fatal("expected lookup miss for unknown IP")
	}
}

// @sk-test dns-response-tracker#T1.1: TestTrackerTTL (AC-006)
func TestTrackerTTL(t *testing.T) {
	tr := NewTracker(50 * time.Millisecond)
	ip := netip.MustParseAddr("95.163.249.123")
	tr.Track("ozon.ru", []netip.Addr{ip})

	time.Sleep(10 * time.Millisecond)
	_, ok := tr.Lookup(ip)
	if !ok {
		t.Fatal("expected lookup hit before TTL expiry")
	}

	time.Sleep(60 * time.Millisecond)
	_, ok = tr.Lookup(ip)
	if ok {
		t.Fatal("expected lookup miss after TTL expiry")
	}
}

// @sk-test dns-response-tracker#T1.1: TestTrackerRace (AC-007)
func TestTrackerRace(t *testing.T) {
	tr := NewTracker(60 * time.Second)
	done := make(chan struct{}, 2)
	go func() {
		for i := 0; i < 100; i++ {
			tr.Track("example.com", []netip.Addr{netip.MustParseAddr("10.0.0.1")})
			tr.Lookup(netip.MustParseAddr("10.0.0.1"))
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 100; i++ {
			tr.Track("other.com", []netip.Addr{netip.MustParseAddr("10.0.0.2")})
			tr.Lookup(netip.MustParseAddr("10.0.0.2"))
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}

// @sk-test dns-response-tracker#T1.1: TestTrackerTrackResponse (AC-001)
func TestTrackerTrackResponse(t *testing.T) {
	tr := NewTracker(60 * time.Second)
	// Build a minimal DNS response for ozon.ru -> 95.163.249.123
	raw := buildDNSResponse("ozon.ru", netip.MustParseAddr("95.163.249.123"), 120)
	tr.TrackResponse("ozon.ru", raw)

	domain, ok := tr.Lookup(netip.MustParseAddr("95.163.249.123"))
	if !ok {
		t.Fatal("expected lookup hit after TrackResponse")
	}
	if domain != "ozon.ru" {
		t.Errorf("expected ozon.ru, got %s", domain)
	}
}

// @sk-test dns-response-tracker#T1.1: TestTrackerTrackResponseAAAA (AC-001)
func TestTrackerTrackResponseAAAA(t *testing.T) {
	tr := NewTracker(60 * time.Second)
	ip := netip.MustParseAddr("2a00:1450:4001:830::200e")
	raw := buildDNSResponse("google.com", ip, 60)
	tr.TrackResponse("google.com", raw)

	domain, ok := tr.Lookup(ip)
	if !ok {
		t.Fatal("expected lookup hit for AAAA")
	}
	if domain != "google.com" {
		t.Errorf("expected google.com, got %s", domain)
	}
}

// buildDNSResponse constructs a minimal DNS response for testing.
// It encodes qname as uncompressed DNS name and adds a single answer record.
func buildDNSResponse(domain string, ip netip.Addr, ttlSec uint32) []byte {
	var buf []byte
	// header: ID=0x1234, QR=1, OPCODE=0, AA=0, TC=0, RD=1, RA=1, Z=0, RCODE=0
	hdr := []byte{
		0x12, 0x34, // ID
		0x81, 0x80, // flags: QR+RD+RA
		0x00, 0x01, // QDCOUNT
		0x00, 0x01, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
	}
	buf = append(buf, hdr...)
	buf = append(buf, encodeDNSName(domain)...)
	buf = binary.BigEndian.AppendUint16(buf, 1) // QTYPE A
	buf = binary.BigEndian.AppendUint16(buf, 1) // QCLASS IN
	// answer: NAME pointer (c0 0c = offset 12)
	buf = append(buf, 0xc0, 0x0c)
	if ip.Is4() {
		buf = binary.BigEndian.AppendUint16(buf, 1)      // TYPE A
		buf = binary.BigEndian.AppendUint16(buf, 1)      // CLASS IN
		buf = binary.BigEndian.AppendUint32(buf, ttlSec) // TTL
		buf = binary.BigEndian.AppendUint16(buf, 4)      // RDLENGTH
		buf = append(buf, ip.AsSlice()...)               // RDATA
	} else {
		buf = binary.BigEndian.AppendUint16(buf, 28)     // TYPE AAAA
		buf = binary.BigEndian.AppendUint16(buf, 1)      // CLASS IN
		buf = binary.BigEndian.AppendUint32(buf, ttlSec) // TTL
		buf = binary.BigEndian.AppendUint16(buf, 16)     // RDLENGTH
		buf = append(buf, ip.AsSlice()...)               // RDATA
	}
	return buf
}

func encodeDNSName(domain string) []byte {
	buf := make([]byte, 0, len(domain)+1)
	for _, label := range splitLabels(domain) {
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0)
	return buf
}

func splitLabels(domain string) []string {
	var labels []string
	start := 0
	for i := 0; i <= len(domain); i++ {
		if i == len(domain) || domain[i] == '.' {
			if i > start {
				labels = append(labels, domain[start:i])
			}
			start = i + 1
		}
	}
	return labels
}
