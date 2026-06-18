package relay

import (
	"encoding/binary"
	"math"
	"net/netip"
	"sync"
	"time"

	"go.uber.org/zap"
)

// @sk-task relay-terminator#T8.1: NAT connection tracking entry (AC-009)
type natEntry struct {
	origSrc   netip.Addr
	origSPort uint16
	expires   time.Time
}

// @sk-task relay-terminator#T8.1: NAT tracking map key (AC-009)
type natKey struct {
	proto      uint8  // IP protocol (6=TCP, 17=UDP, 1=ICMP)
	clientPort uint16 // TCP/UDP source port or ICMP identifier
	dstIP      uint32 // destination IP (network byte order)
	dstPort    uint16 // TCP/UDP destination port or 0 for ICMP
}

// @sk-task relay-terminator#T8.1: connection tracker for SNAT/DNAT (AC-009)
type natTracker struct {
	mu      sync.RWMutex
	entries map[natKey]natEntry
	logger  *zap.Logger
}

func newNATTracker(logger *zap.Logger) *natTracker {
	return &natTracker{
		entries: make(map[natKey]natEntry),
		logger:  logger,
	}
}

const natTimeout = 5 * time.Minute

// @sk-task relay-terminator#T8.1: SNAT — rewrite src IP, store tracking (AC-009)
// buf is modified in place; returns true on success
func (n *natTracker) snat(buf []byte, assignedIP netip.Addr) bool {
	if len(buf) < 20 || buf[0]>>4 != 4 {
		return false
	}
	srcIP := netip.AddrFrom4([4]byte(buf[12:16]))

	proto := buf[9]
	var srcPort, dstPort uint16
	headerLen := int(buf[0]&0x0f) * 4

	switch proto {
	case 6, 17: // TCP, UDP
		if len(buf) < headerLen+4 {
			return false
		}
		srcPort = binary.BigEndian.Uint16(buf[headerLen : headerLen+2])
		dstPort = binary.BigEndian.Uint16(buf[headerLen+2 : headerLen+4])
	case 1: // ICMP
		if len(buf) < headerLen+4 {
			return false
		}
		icmpType := buf[headerLen]
		if icmpType != 8 && icmpType != 13 && icmpType != 15 {
			return false
		}
		srcPort = binary.BigEndian.Uint16(buf[headerLen+4 : headerLen+6])
	default:
		return false
	}

	key := natKey{
		proto:      proto,
		clientPort: srcPort,
		dstIP:      binary.BigEndian.Uint32(buf[16:20]),
		dstPort:    dstPort,
	}

	n.mu.Lock()
	n.entries[key] = natEntry{
		origSrc:   srcIP,
		origSPort: srcPort,
		expires:   time.Now().Add(natTimeout),
	}
	n.mu.Unlock()

	assigned4 := assignedIP.As4()
	copy(buf[12:16], assigned4[:])
	fixIPv4Checksum(buf)

	n.logger.Debug("nat snat",
		zap.String("from", srcIP.String()),
		zap.String("to", assignedIP.String()),
		zap.Uint8("proto", proto),
	)
	return true
}

// @sk-task relay-terminator#T8.1: DNAT — rewrite dst IP from tracking (AC-009)
// buf is modified in place; returns true on success
func (n *natTracker) dnat(buf []byte) bool {
	if len(buf) < 20 || buf[0]>>4 != 4 {
		return false
	}
	dstIP := netip.AddrFrom4([4]byte(buf[16:20]))
	proto := buf[9]
	headerLen := int(buf[0]&0x0f) * 4

	var clientPort uint16
	var srcIPArr [4]byte
	var dstPort uint16

	switch proto {
	case 6, 17:
		if len(buf) < headerLen+4 {
			return false
		}
		clientPort = binary.BigEndian.Uint16(buf[headerLen+2 : headerLen+4])
		dstPort = binary.BigEndian.Uint16(buf[headerLen : headerLen+2])
		copy(srcIPArr[:], buf[12:16])
	case 1:
		if len(buf) < headerLen+4 {
			return false
		}
		clientPort = binary.BigEndian.Uint16(buf[headerLen+4 : headerLen+6])
		dstPort = 0
		copy(srcIPArr[:], buf[12:16])
	default:
		return false
	}

	key := natKey{
		proto:      proto,
		clientPort: clientPort,
		dstIP:      binary.BigEndian.Uint32(srcIPArr[:]),
		dstPort:    dstPort,
	}

	n.mu.RLock()
	entry, ok := n.entries[key]
	n.mu.RUnlock()
	if !ok {
		return false
	}

	orig4 := entry.origSrc.As4()
	copy(buf[16:20], orig4[:])
	fixIPv4Checksum(buf)

	n.logger.Debug("nat dnat",
		zap.String("from", dstIP.String()),
		zap.String("to", entry.origSrc.String()),
		zap.Uint8("proto", proto),
	)
	return true
}

// @sk-task relay-terminator#T8.1: fix IPv4 checksum after IP header modification (AC-009)
func fixIPv4Checksum(data []byte) {
	binary.BigEndian.PutUint16(data[10:12], 0)
	sum := uint32(0)
	for i := 0; i < len(data); i += 2 {
		if i >= 20 {
			break
		}
		if i+1 < 20 {
			sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
		}
	}
	for (sum >> 16) > 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	cksum := ^uint16(sum)
	if cksum == 0 {
		cksum = math.MaxUint16
	}
	binary.BigEndian.PutUint16(data[10:12], cksum)
}
