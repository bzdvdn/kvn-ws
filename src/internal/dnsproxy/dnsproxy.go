package dnsproxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/dns"
)

// @sk-task dns-upstreams-list#T2.1: upstream string → upstreams []string + fallback (AC-005)
// @sk-task transparent-proxy#T1.3: DNS proxy server skeleton (DEC-003)
// @sk-task dns-response-tracker#T2.2: tracker field (AC-005)
// @sk-task lock-optimization#T3.1: nextID → atomic.AddUint32 (AC-003)
// @sk-task lock-optimization#T3.2: mu → RWMutex (AC-006)
// @sk-task performance-scope-p2#T3.1: configMu + pendingMu split (AC-008)
// Lock ordering: configMu → pendingMu (forward), pendingMu → configMu (HandleDNSResponse)
// Always release before acquire to avoid deadlock.
type Server struct {
	listenAddr    string
	upstreams     []string
	conn          *net.UDPConn
	configMu      sync.RWMutex
	routeDirect   func(domain string) bool
	origResolves  []string
	corpDomains   []string
	tracker       *dns.Tracker
	directRouteFn func(ips []netip.Addr)
}

// @sk-task dns-upstreams-list#T2.1: variadic New with fallback defaults (AC-005)
func New(listenAddr string, upstreams ...string) *Server {
	if len(upstreams) == 0 {
		upstreams = []string{"1.1.1.1:53", "8.8.8.8:53"}
	}
	return &Server{listenAddr: listenAddr, upstreams: upstreams}
}

// @sk-task transparent-proxy#T5.4: domain-based DNS routing — RouteFunc for excluded domains
func (s *Server) SetRouteFunc(fn func(domain string) bool) {
	s.configMu.Lock()
	s.routeDirect = fn
	s.configMu.Unlock()
}

// @sk-task transparent-proxy#T5.4: original nameservers for local DNS resolution of excluded domains
func (s *Server) SetOrigResolvers(resolvers []string) {
	s.configMu.Lock()
	s.origResolves = resolvers
	s.configMu.Unlock()
}

// SetCorporateDomains sets the domain suffixes served by the corporate
// resolvers (for example the search domains of another VPN). Names outside
// these suffixes are resolved via public upstreams first; names inside them
// use the corporate resolvers first. When empty, every name is public-first.
func (s *Server) SetCorporateDomains(domains []string) {
	s.configMu.Lock()
	s.corpDomains = append([]string(nil), domains...)
	s.configMu.Unlock()
}

// @sk-task dns-response-tracker#T2.2: SetTracker sets the DNS tracker for IP→domain mapping (AC-005)
func (s *Server) SetTracker(t *dns.Tracker) {
	s.configMu.Lock()
	s.tracker = t
	s.configMu.Unlock()
}

// @sk-task dns-response-tracker#T3.5: SetDirectRouteFunc callback for kernel exclude routes
func (s *Server) SetDirectRouteFunc(fn func(ips []netip.Addr)) {
	s.configMu.Lock()
	s.directRouteFn = fn
	s.configMu.Unlock()
}

// @sk-task transparent-proxy#T2.3: DNS forwarder via TCP to upstream (AC-009)
func (s *Server) Run(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", s.listenAddr)
	if err != nil {
		return err
	}
	s.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer s.conn.Close()

	buf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		_ = s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, raddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}

		query := make([]byte, n)
		copy(query, buf[:n])

		go s.forward(ctx, query, raddr)
	}
}

func (s *Server) Shutdown() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// @sk-task transparent-proxy#T2.3: forward DNS query to upstream via TCP (AC-009)
func (s *Server) forward(ctx context.Context, query []byte, raddr *net.UDPAddr) {
	s.configMu.RLock()
	routeDirect := s.routeDirect
	origResolvers := s.origResolves
	upstreams := s.upstreams
	corpDomains := s.corpDomains
	s.configMu.RUnlock()

	domain := extractDNSDomain(query)

	// Check domain-based routing first (no stream needed for direct resolution)
	if routeDirect != nil && domain != "" && routeDirect(domain) {
		s.resolveDirect(ctx, query, raddr, origResolvers)
		return
	}

	// Corporate resolvers only own their own domains. Public names must not
	// inherit corporate filtering (for example a FortiGate blocking youtube
	// with NXDOMAIN), so the order depends on the query name.
	candidates := orderUpstreams(domain, upstreams, origResolvers, corpDomains)
	resp := exchangeTCP(ctx, query, candidates)
	if resp == nil {
		return
	}

	// @sk-task dns-response-tracker#T3.2: track IPs from direct DNS response
	if domain != "" {
		s.configMu.RLock()
		tracker := s.tracker
		s.configMu.RUnlock()
		if tracker != nil {
			tracker.TrackResponse(domain, resp)
		}
	}

	_ = s.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, _ = s.conn.WriteToUDP(resp, raddr)
}

const (
	rcodeNoError  = 0
	rcodeServFail = 2
	rcodeNXDomain = 3
	rcodeRefused  = 5
)

func dnsRcode(resp []byte) int {
	if len(resp) < 4 {
		return -1
	}
	return int(resp[3] & 0x0f)
}

// retryableRcode reports whether another upstream may still answer better.
// A corporate resolver often returns NXDOMAIN for names blocked by policy;
// that answer must not shadow a public resolver that still resolves them.
func retryableRcode(rcode int) bool {
	switch rcode {
	case rcodeServFail, rcodeNXDomain, rcodeRefused:
		return true
	default:
		return false
	}
}

// orderUpstreams returns the candidate list for a query name: corporate
// resolvers first for their own domains, public upstreams first otherwise.
// Duplicates are removed while preserving order.
func orderUpstreams(domain string, public, corporate, corpDomains []string) []string {
	first, second := public, corporate
	if isCorporateDomain(domain, corpDomains) {
		first, second = corporate, public
	}
	out := make([]string, 0, len(first)+len(second))
	out = appendUnique(out, first...)
	out = appendUnique(out, second...)
	return out
}

func isCorporateDomain(domain string, corpDomains []string) bool {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	if d == "" {
		return false
	}
	for _, suffix := range corpDomains {
		s := strings.ToLower(strings.Trim(strings.TrimSpace(suffix), "."))
		if s == "" {
			continue
		}
		if d == s || strings.HasSuffix(d, "."+s) {
			return true
		}
	}
	return false
}

func appendUnique(dst []string, src ...string) []string {
	for _, v := range src {
		duplicate := false
		for _, existing := range dst {
			if existing == v {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst = append(dst, v)
		}
	}
	return dst
}

// exchangeTCP resolves query via the first upstream that returns a usable
// answer, falling through on transport errors and retryable RCODEs.
func exchangeTCP(ctx context.Context, query []byte, upstreams []string) []byte {
	qlen := len(query)
	if qlen > math.MaxUint16 {
		return nil
	}
	wire := make([]byte, 2+qlen)
	wire[0] = byte(qlen >> 8)
	wire[1] = byte(qlen & 0xff)
	copy(wire[2:], query)

	var lastResp []byte
	for _, addr := range upstreams {
		resp, err := exchangeTCPOne(ctx, addr, wire)
		if err != nil {
			continue
		}
		lastResp = resp
		if retryableRcode(dnsRcode(resp)) {
			continue
		}
		return resp
	}
	// All upstreams gave a retryable RCODE: return the last answer so the
	// client still receives a definitive response instead of a timeout.
	return lastResp
}

func exchangeTCPOne(ctx context.Context, addr string, wire []byte) ([]byte, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	upConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer upConn.Close() // #nosec G104

	_ = upConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := upConn.Write(wire); err != nil {
		return nil, err
	}

	_ = upConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(upConn)
	respLen, err := readUint16(br)
	if err != nil {
		return nil, err
	}
	if respLen > 1500 {
		return nil, fmt.Errorf("dnsproxy: upstream response too large: %d", respLen)
	}
	resp := make([]byte, respLen)
	if _, err := io.ReadFull(br, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// @sk-task transparent-proxy#T5.4: local UDP DNS resolution for excluded domains
// @sk-task dns-response-tracker#T2.2: track excluded domain IPs (AC-005)
// @sk-task dns-response-tracker#T3.5: try all resolvers, not only first (TUN multi-resolver fallback)
func (s *Server) resolveDirect(ctx context.Context, query []byte, raddr *net.UDPAddr, resolvers []string) {
	// Direct domains should still resolve when the original resolver is
	// unavailable (for example, another VPN's DNS is not reachable). Try the
	// original list first, then the configured public upstreams.
	candidates := append([]string{}, resolvers...)
	for _, upstream := range s.upstreams {
		duplicate := false
		for _, existing := range candidates {
			if existing == upstream {
				duplicate = true
				break
			}
		}
		if !duplicate {
			candidates = append(candidates, upstream)
		}
	}
	if len(candidates) == 0 {
		return
	}
	domain := extractDNSDomain(query)
	for _, ns := range candidates {
		nsAddr, err := net.ResolveUDPAddr("udp", ns)
		if err != nil {
			continue
		}
		conn, err := net.DialUDP("udp", nil, nsAddr)
		if err != nil {
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.Write(query); err != nil {
			conn.Close() // #nosec G104
			continue
		}
		resp := make([]byte, 1500)
		n, err := conn.Read(resp)
		conn.Close() // #nosec G104
		if err != nil {
			continue
		}
		if n < 12 {
			continue
		}
		if domain != "" {
			s.configMu.RLock()
			tracker := s.tracker
			fn := s.directRouteFn
			s.configMu.RUnlock()
			if tracker != nil {
				tracker.TrackResponse(domain, resp[:n])
			}
			if fn != nil {
				fn(dns.ParseDNSResponse(resp[:n]))
			}
		}
		_ = s.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		_, _ = s.conn.WriteToUDP(resp[:n], raddr)
		return
	}
}

// @sk-task transparent-proxy#T5.4: extract QNAME from raw DNS message for domain routing
func extractDNSDomain(msg []byte) string {
	if len(msg) < 12 {
		return ""
	}
	pos := 12
	var labels []string
	for {
		if pos >= len(msg) {
			return ""
		}
		labelLen := int(msg[pos])
		if labelLen == 0 {
			break
		}
		if labelLen&0xc0 == 0xc0 {
			break
		}
		if pos+1+labelLen > len(msg) {
			return ""
		}
		labels = append(labels, string(msg[pos+1:pos+1+labelLen]))
		pos += 1 + labelLen
	}
	if len(labels) == 0 {
		return ""
	}
	return strings.Join(labels, ".")
}

func readUint16(r io.Reader) (uint16, error) {
	var b [2]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b[:]), nil
}
