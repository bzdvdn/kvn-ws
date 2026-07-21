package dnsproxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/dns"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

type StreamConn interface {
	ReadMessage() ([]byte, error)
	WriteMessage([]byte) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Close() error
}

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
	pendingMu     sync.Mutex
	stream        StreamConn
	nextID        uint32
	pending       map[uint32]chan []byte
	routeDirect   func(domain string) bool
	origResolves  []string
	tracker       *dns.Tracker
	directRouteFn func(ips []netip.Addr)
}

// @sk-task dns-upstreams-list#T2.1: variadic New with fallback defaults (AC-005)
func New(listenAddr string, upstreams ...string) *Server {
	if len(upstreams) == 0 {
		upstreams = []string{"1.1.1.1:53", "8.8.8.8:53"}
	}
	return &Server{listenAddr: listenAddr, upstreams: upstreams, pending: make(map[uint32]chan []byte)}
}

func (s *Server) SetStream(stream StreamConn) {
	s.configMu.Lock()
	s.stream = stream
	s.configMu.Unlock()
}

func (s *Server) ClearStream() {
	s.configMu.Lock()
	s.stream = nil
	s.configMu.Unlock()
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

// HandleDNSResponse delivers a DNS response from the tunnel to a pending query.
// resp is owned by the caller; we copy it here because the caller may return
// its buffer to a pool after this call returns.
func (s *Server) HandleDNSResponse(streamID uint32, resp []byte) {
	s.pendingMu.Lock()
	ch, ok := s.pending[streamID]
	if ok {
		delete(s.pending, streamID)
	}
	s.pendingMu.Unlock()
	if ok {
		respCopy := make([]byte, len(resp))
		copy(respCopy, resp)
		select {
		case ch <- respCopy:
		default:
		}
	}
}

// @sk-task transparent-proxy#T2.3: forward DNS query to upstream via TCP (AC-009)
func (s *Server) forward(ctx context.Context, query []byte, raddr *net.UDPAddr) {
	s.configMu.RLock()
	stream := s.stream
	routeDirect := s.routeDirect
	origResolves := s.origResolves
	upstreams := s.upstreams
	s.configMu.RUnlock()

	// Check domain-based routing first (no stream needed for direct resolution)
	if routeDirect != nil {
		if domain := extractDNSDomain(query); domain != "" && routeDirect(domain) {
			s.resolveDirect(ctx, query, raddr, origResolves)
			return
		}
	}

	if stream != nil {
		s.forwardViaTunnel(ctx, query, raddr, stream)
		return
	}

	// fallback: direct TCP to upstreams in order (used when tunnel not available)

	if len(upstreams) == 0 {
		return
	}

	dialer := net.Dialer{Timeout: 5 * time.Second}

	qlen := len(query)
	if qlen > math.MaxUint16 {
		return
	}
	wire := make([]byte, 2+qlen)
	wire[0] = byte(qlen >> 8)
	wire[1] = byte(qlen & 0xff)
	copy(wire[2:], query)

	var resp []byte
	var lastErr error
	for _, addr := range upstreams {
		upConn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			lastErr = err
			continue
		}

		_ = upConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := upConn.Write(wire); err != nil {
			upConn.Close() // #nosec G104
			lastErr = err
			continue
		}

		_ = upConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		br := bufio.NewReader(upConn)
		respLen, err := readUint16(br)
		if err != nil {
			upConn.Close() // #nosec G104
			lastErr = err
			continue
		}
		if respLen > 1500 {
			upConn.Close() // #nosec G104
			continue
		}
		resp = make([]byte, respLen)
		if _, err := br.Read(resp); err != nil {
			upConn.Close() // #nosec G104
			lastErr = err
			continue
		}
		upConn.Close() // #nosec G104
		lastErr = nil
		break
	}
	if lastErr != nil || resp == nil {
		return
	}

	// @sk-task dns-response-tracker#T3.2: track IPs from direct DNS response
	if domain := extractDNSDomain(query); domain != "" {
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

// @sk-task transparent-proxy#T5.4: local UDP DNS resolution for excluded domains
// @sk-task dns-response-tracker#T2.2: track excluded domain IPs (AC-005)
// @sk-task dns-response-tracker#T3.5: try all resolvers, not only first (TUN multi-resolver fallback)
func (s *Server) resolveDirect(ctx context.Context, query []byte, raddr *net.UDPAddr, resolvers []string) {
	if len(resolvers) == 0 {
		return
	}
	domain := extractDNSDomain(query)
	for _, ns := range resolvers {
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

func (s *Server) forwardViaTunnel(ctx context.Context, query []byte, raddr *net.UDPAddr, stream StreamConn) {
	streamID := atomic.AddUint32(&s.nextID, 1)
	ch := make(chan []byte, 1)
	s.pendingMu.Lock()
	s.pending[streamID] = ch
	s.pendingMu.Unlock()

	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, streamID)
		s.pendingMu.Unlock()
	}()

	payload := make([]byte, 4+len(query))
	binary.BigEndian.PutUint32(payload[0:4], streamID)
	copy(payload[4:], query)

	f := framing.Frame{
		Type:    framing.FrameTypeDNS,
		Payload: payload,
	}
	encoded, err := f.Encode()
	if err != nil {
		return
	}
	defer framing.ReturnBuffer(encoded)

	if err := stream.WriteMessage(encoded); err != nil {
		return
	}

	select {
	case resp := <-ch:
		// @sk-task dns-response-tracker#T3.2: track IPs from tunnel-forwarded DNS response
		if domain := extractDNSDomain(query); domain != "" {
			s.configMu.RLock()
			tracker := s.tracker
			s.configMu.RUnlock()
			if tracker != nil {
				tracker.TrackResponse(domain, resp)
			}
		}
		_ = s.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		_, _ = s.conn.WriteToUDP(resp, raddr)
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
	}
}

func readUint16(r *bufio.Reader) (uint16, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	b2, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	return uint16(b)<<8 | uint16(b2), nil
}
