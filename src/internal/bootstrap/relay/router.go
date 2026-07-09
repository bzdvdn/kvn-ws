package relay

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
)

const defaultMaxDNSCacheSize = 10000

// @sk-task relay-terminator#T3.1: build RuleSet from RelayRoutingCfg (AC-002)
func newDirectRuleSet(cfg *config.RelayRoutingCfg, logger *zap.Logger) (*routing.RuleSet, error) {
	if cfg == nil {
		return nil, fmt.Errorf("relay routing config is nil")
	}
	routingCfg := &config.RoutingCfg{
		ExcludeRanges:  cfg.DirectRanges,
		ExcludeDomains: cfg.DirectDomains,
		DefaultRoute:   "server",
	}
	return routing.NewRuleSetWithResolver(routingCfg, nil, logger)
}

// @sk-task relay-terminator#T6.2: forward DNS query to upstream resolver (RQ-008, AC-003)
// @sk-task relay-terminator#T8.8: forward DNS with shouldCache param for non-direct domains (AC-008)
// @sk-task arch-fix-critical-paths#T3.1: DNS upstream connection pool (AC-002)
func (r *Relay) forwardDNSQuery(packet []byte, shouldCache bool) error {
	verIHL := packet[0]
	ihl := int(verIHL&0x0f) * 4
	if len(packet) < ihl+12+12 {
		return nil
	}

	dnsPayload := packet[ihl+8:]

	conn := r.getDNSConn()
	if conn == nil {
		return fmt.Errorf("dns upstream dial failed")
	}
	defer r.putDNSConn(conn)

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := conn.Write(dnsPayload); err != nil {
		return err
	}

	respBuf := make([]byte, 1500)
	n, err := conn.Read(respBuf)
	if err != nil {
		return err
	}
	respDNS := respBuf[:n]

	if shouldCache {
		r.cacheDNSResponse(respDNS)
	}

	out := buildDNSRespPacket(packet, respDNS)
	if out == nil {
		return fmt.Errorf("build dns response packet failed")
	}

	_, err = r.tunDev.Write(out)
	return err
}

// @sk-task arch-fix-critical-paths#T3.1: getDNSConn from pool or dial (AC-002)
func (r *Relay) getDNSConn() net.Conn {
	if r.dnsConnPool == nil {
		conn, err := net.DialTimeout("udp", r.dnsUpstream, 5*time.Second)
		if err != nil {
			return nil
		}
		return conn
	}
	v := r.dnsConnPool.Get()
	if v == nil {
		return nil
	}
	conn, ok := v.(net.Conn)
	if !ok {
		return nil
	}
	return conn
}

// @sk-task arch-fix-critical-paths#T3.1: putDNSConn returns conn to pool (AC-002)
func (r *Relay) putDNSConn(conn net.Conn) {
	if r.dnsConnPool != nil {
		r.dnsConnPool.Put(conn)
	} else {
		_ = conn.Close()
	}
}

// @sk-task relay-terminator#T6.2: cache resolved IPs from DNS response (RQ-008, AC-003)
// @sk-task arch-fix-critical-paths#T1.2: boundary guards cacheDNSResponse (AC-006)
func (r *Relay) cacheDNSResponse(resp []byte) {
	if len(resp) < 12 {
		return
	}
	if resp[2]&0x80 == 0 {
		return
	}
	if resp[3]&0x0f != 0 {
		return
	}

	ancount := int(binary.BigEndian.Uint16(resp[6:8]))
	if ancount == 0 {
		return
	}

	pos := 12
	for pos < len(resp) {
		if resp[pos] == 0 {
			pos++
			break
		}
		if resp[pos]&0xc0 == 0xc0 {
			pos += 2
			break
		}
		pos += 1 + int(resp[pos])
		if pos >= len(resp) {
			return
		}
	}
	if pos+4 > len(resp) {
		return
	}
	pos += 4

	now := time.Now()
	for i := 0; i < ancount && pos < len(resp); i++ {
		if pos >= len(resp) {
			return
		}
		if resp[pos]&0xc0 == 0xc0 {
			pos += 2
		} else {
			for pos < len(resp) {
				if resp[pos] == 0 {
					pos++
					break
				}
				pos += 1 + int(resp[pos])
				if pos >= len(resp) {
					return
				}
			}
		}
		if pos+10 > len(resp) {
			return
		}
		rtype := binary.BigEndian.Uint16(resp[pos : pos+2])
		rdlen := int(binary.BigEndian.Uint16(resp[pos+8 : pos+10]))
		pos += 10
		if pos+rdlen > len(resp) {
			return
		}
		if rtype == 1 && rdlen == 4 {
			ip := netip.AddrFrom4([4]byte(resp[pos : pos+4]))
			r.insertDNSCache(ip, now.Add(r.cacheTTL))
			r.logger.Debug("dns cached direct ip", zap.String("ip", ip.String()))
		}
		if rtype == 28 && rdlen == 16 {
			ip := netip.AddrFrom16([16]byte(resp[pos : pos+16]))
			r.insertDNSCache(ip, now.Add(r.cacheTTL))
			r.logger.Debug("dns cached direct ipv6", zap.String("ip", ip.String()))
		}
		pos += rdlen
	}
}

// @sk-task arch-fix-critical-paths#T3.2: insertDNSCache with size limit eviction (AC-003)
func (r *Relay) insertDNSCache(ip netip.Addr, expiry time.Time) {
	r.dnsCacheMu.Lock()
	defer r.dnsCacheMu.Unlock()

	if len(r.dnsCache) >= defaultMaxDNSCacheSize {
		now := time.Now()
		for cachedIP, exp := range r.dnsCache {
			if now.After(exp) {
				delete(r.dnsCache, cachedIP)
			}
		}
		if len(r.dnsCache) >= defaultMaxDNSCacheSize {
			var oldestIP netip.Addr
			var oldestExp time.Time
			first := true
			for cachedIP, exp := range r.dnsCache {
				if first || exp.Before(oldestExp) {
					oldestIP = cachedIP
					oldestExp = exp
					first = false
				}
			}
			if !first {
				delete(r.dnsCache, oldestIP)
			}
		}
	}
	r.dnsCache[ip] = expiry
}

// @sk-task arch-fix-critical-paths#T1.2: boundary guards raw packet parsing (AC-006)
func isDNSQuery(packet []byte) bool {
	if len(packet) < 20 {
		return false
	}
	if packet[9] != 17 {
		return false
	}
	verIHL := packet[0]
	ihl := int(verIHL&0x0f) * 4
	if ihl < 20 {
		return false
	}
	if len(packet) < ihl+4 {
		return false
	}
	dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
	return dstPort == 53
}

// @sk-task arch-fix-critical-paths#T1.1: overflow guard buildDNSRespPacket (AC-005)
func buildDNSRespPacket(origQuery, dnsResp []byte) []byte {
	verIHL := origQuery[0]
	ihl := int(verIHL&0x0f) * 4
	if ihl < 20 || len(origQuery) < ihl+8 {
		return nil
	}

	totalLen := ihl + 8 + len(dnsResp)
	if totalLen > 65535 {
		return nil
	}

	out := make([]byte, totalLen)
	copy(out[:ihl], origQuery[:ihl])
	copy(out[12:16], origQuery[16:20])
	copy(out[16:20], origQuery[12:16])
	binary.BigEndian.PutUint16(out[2:4], uint16(totalLen))
	out[4] = 0
	out[5] = 0
	out[8] = 64
	out[10] = 0
	out[11] = 0
	cs := ipv4Checksum(out[:ihl])
	binary.BigEndian.PutUint16(out[10:12], cs)

	udpStart := ihl
	copy(out[udpStart:udpStart+8], origQuery[ihl:ihl+8])
	copy(out[udpStart:udpStart+2], origQuery[ihl+2:ihl+4])
	copy(out[udpStart+2:udpStart+4], origQuery[ihl:ihl+2])
	udpLen := 8 + len(dnsResp)
	if udpLen > 65535 {
		return nil
	}
	binary.BigEndian.PutUint16(out[udpStart+4:udpStart+6], uint16(udpLen))
	out[udpStart+6] = 0
	out[udpStart+7] = 0

	copy(out[udpStart+8:], dnsResp)
	return out
}

func dedupStrings(s []string) []string {
	if len(s) == 0 {
		return s
	}
	seen := make(map[string]bool, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func ipv4Checksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}
	sum = (sum >> 16) + (sum & 0xffff)
	sum += sum >> 16
	return ^uint16(sum)
}

// @sk-task geoip-geosite-integration#T3.3: resolve DirectSources via Resolver (AC-002, AC-003, AC-004)
func (r *Relay) resolveDirectSources(rc *config.RelayRoutingCfg) {
	if rc == nil || len(rc.DirectSources) == 0 {
		return
	}

	cacheDir := filepath.Join(filepath.Dir(r.configPath), ".source-cache")
	tmpCfg := &config.RoutingCfg{
		GeoIPPath:   rc.GeoIPPath,
		GeoSitePath: rc.GeoSitePath,
		GeoIPURL:    rc.GeoIPURL,
		GeoSiteURL:  rc.GeoSiteURL,
		SourceTTL:   rc.SourceTTL,
	}
	srcResolver := routing.NewResolver(tmpCfg, cacheDir, r.logger)
	resolved, err := srcResolver.ResolveSources(rc.DirectSources)
	if err != nil {
		r.logger.Warn("direct sources resolve failed, using original config", zap.Error(err))
		return
	}

	rc.DirectRanges = append(rc.DirectRanges, resolved.CIDRs...)
	rc.DirectDomains = append(rc.DirectDomains, resolved.Domains...)

	rc.DirectRanges = dedupStrings(rc.DirectRanges)
	rc.DirectDomains = dedupStrings(rc.DirectDomains)

	r.logger.Info("direct sources resolved",
		zap.Int("direct_ranges", len(rc.DirectRanges)),
		zap.Int("direct_domains", len(rc.DirectDomains)),
	)
}

// @sk-task relay-terminator#T3.1: parse dest IP from raw packet (AC-002)
// @sk-task arch-fix-critical-paths#T1.2: boundary guards extractDestIP (AC-006)
func extractDestIP(packet []byte) (netip.Addr, bool) {
	if len(packet) < 1 {
		return netip.Addr{}, false
	}
	switch packet[0] >> 4 {
	case 4:
		if len(packet) < 20 {
			return netip.Addr{}, false
		}
		return netip.AddrFrom4([4]byte(packet[16:20])), true
	case 6:
		if len(packet) < 40 {
			return netip.Addr{}, false
		}
		return netip.AddrFrom16([16]byte(packet[24:40])), true
	default:
		return netip.Addr{}, false
	}
}
