package dns

import (
	"encoding/binary"
	"net/netip"
	"sync"
	"time"
)

// @sk-task dns-response-tracker#T1.1: Tracker struct (AC-001, AC-006, AC-007)
type Tracker struct {
	mu      sync.RWMutex
	entries map[netip.Addr]trackedEntry
	ttl     time.Duration
}

type trackedEntry struct {
	domain  string
	expires time.Time
}

func NewTracker(ttl time.Duration) *Tracker {
	return &Tracker{
		entries: make(map[netip.Addr]trackedEntry),
		ttl:     ttl,
	}
}

// @sk-task dns-response-tracker#T1.1: Track stores IP→domain mapping (AC-001)
func (t *Tracker) Track(domain string, ips []netip.Addr) {
	t.mu.Lock()
	defer t.mu.Unlock()
	deadline := time.Now().Add(t.ttl)
	for _, ip := range ips {
		t.entries[ip] = trackedEntry{domain: domain, expires: deadline}
	}
}

// @sk-task dns-response-tracker#T1.1: TrackResponse parses a DNS response and tracks IPs (AC-001)
func (t *Tracker) TrackResponse(qname string, raw []byte) {
	ips := ParseDNSResponse(raw)
	if len(ips) > 0 {
		t.Track(qname, ips)
	}
}

// @sk-task dns-response-tracker#T1.1: Lookup returns domain for IP (AC-003)
func (t *Tracker) Lookup(ip netip.Addr) (string, bool) {
	t.mu.RLock()
	e, ok := t.entries[ip]
	if !ok {
		t.mu.RUnlock()
		return "", false
	}
	if time.Now().After(e.expires) {
		t.mu.RUnlock()
		t.mu.Lock()
		delete(t.entries, ip)
		t.mu.Unlock()
		return "", false
	}
	t.mu.RUnlock()
	return e.domain, true
}

// ParseDNSResponse extracts A and AAAA IPs from a DNS response (RFC 1035 wire format).
func ParseDNSResponse(raw []byte) []netip.Addr {
	if len(raw) < 12 {
		return nil
	}
	// skip header (12 bytes) + question section
	offset := 12
	// parse question count from header
	qdcount := int(binary.BigEndian.Uint16(raw[4:6]))
	for i := 0; i < qdcount; i++ {
		if offset >= len(raw) {
			return nil
		}
		n, ok := skipDNSName(raw[offset:])
		if !ok {
			return nil
		}
		offset += n + 4 // name + QTYPE(2) + QCLASS(2)
	}

	ancount := int(binary.BigEndian.Uint16(raw[6:8]))
	var ips []netip.Addr
	for i := 0; i < ancount; i++ {
		if offset+2 > len(raw) {
			break
		}
		// NAME (may be compressed pointer)
		if raw[offset]&0xc0 == 0xc0 {
			offset += 2
		} else {
			n, ok := skipDNSName(raw[offset:])
			if !ok {
				break
			}
			offset += n
		}
		if offset+10 > len(raw) {
			break
		}
		rtype := binary.BigEndian.Uint16(raw[offset:])
		offset += 8 // TYPE(2) + CLASS(2) + TTL(4)
		rdlength := int(binary.BigEndian.Uint16(raw[offset:]))
		offset += 2
		if offset+rdlength > len(raw) {
			break
		}
		if rtype == 1 && rdlength == 4 { // A
			ip, ok := netip.AddrFromSlice(raw[offset : offset+4])
			if ok {
				ips = append(ips, ip.Unmap())
			}
		} else if rtype == 28 && rdlength == 16 { // AAAA
			ip, ok := netip.AddrFromSlice(raw[offset : offset+16])
			if ok {
				ips = append(ips, ip)
			}
		}
		offset += rdlength
	}
	return ips
}

// skipDNSName returns the number of bytes consumed by a DNS name (or pointer).
// It follows pointer chains but does not seek the underlying slice.
func skipDNSName(raw []byte) (int, bool) {
	n := 0
	for {
		if n >= len(raw) {
			return 0, false
		}
		b := raw[n]
		if b == 0 {
			return n + 1, true
		}
		if b&0xc0 == 0xc0 {
			return n + 2, true
		}
		if n+1+int(b) > len(raw) {
			return 0, false
		}
		n += 1 + int(b)
	}
}
