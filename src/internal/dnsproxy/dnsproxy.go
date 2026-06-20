package dnsproxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

var resolvConfPath = "/etc/resolv.conf"

var systemdResolvedLinks = []string{
	"/run/systemd/resolve/stub-resolv.conf",
	"/usr/lib/systemd/resolv.conf",
}

type StreamConn interface {
	ReadMessage() ([]byte, error)
	WriteMessage([]byte) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Close() error
}

// @sk-task transparent-proxy#T1.3: DNS proxy server skeleton (DEC-003)
type Server struct {
	listenAddr   string
	upstream     string
	conn         *net.UDPConn
	mu           sync.Mutex
	stream       StreamConn
	nextID       uint32
	pending      map[uint32]chan []byte
	routeDirect  func(domain string) bool
	origResolves []string
}

func New(listenAddr, upstream string) *Server {
	if upstream == "" {
		upstream = "1.1.1.1:53"
	}
	return &Server{listenAddr: listenAddr, upstream: upstream, pending: make(map[uint32]chan []byte)}
}

func (s *Server) SetStream(stream StreamConn) {
	s.mu.Lock()
	s.stream = stream
	s.mu.Unlock()
}

func (s *Server) ClearStream() {
	s.mu.Lock()
	s.stream = nil
	s.mu.Unlock()
}

// @sk-task transparent-proxy#T5.4: domain-based DNS routing — RouteFunc for excluded domains
func (s *Server) SetRouteFunc(fn func(domain string) bool) {
	s.mu.Lock()
	s.routeDirect = fn
	s.mu.Unlock()
}

// @sk-task transparent-proxy#T5.4: original nameservers for local DNS resolution of excluded domains
func (s *Server) SetOrigResolvers(resolvers []string) {
	s.mu.Lock()
	s.origResolves = resolvers
	s.mu.Unlock()
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
	s.mu.Lock()
	ch, ok := s.pending[streamID]
	if ok {
		delete(s.pending, streamID)
	}
	s.mu.Unlock()
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
	s.mu.Lock()
	stream := s.stream
	routeDirect := s.routeDirect
	origResolves := s.origResolves
	s.mu.Unlock()

	// Check domain-based routing first
	if stream != nil && routeDirect != nil {
		if domain := extractDNSDomain(query); domain != "" && routeDirect(domain) {
			s.resolveDirect(ctx, query, raddr, origResolves)
			return
		}
	}

	if stream != nil {
		s.forwardViaTunnel(ctx, query, raddr, stream)
		return
	}

	// fallback: direct TCP to upstream (used when tunnel not available)
	s.mu.Lock()
	upstream := s.upstream
	s.mu.Unlock()

	dialer := net.Dialer{Timeout: 5 * time.Second}
	upConn, err := dialer.DialContext(ctx, "tcp", upstream)
	if err != nil {
		return
	}
	defer upConn.Close()

	qlen := len(query)
	if qlen > math.MaxUint16 {
		return
	}
	wire := make([]byte, 2+qlen)
	wire[0] = byte(qlen >> 8)
	wire[1] = byte(qlen & 0xff)
	copy(wire[2:], query)

	_ = upConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := upConn.Write(wire); err != nil {
		return
	}

	_ = upConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(upConn)
	respLen, err := readUint16(br)
	if err != nil {
		return
	}
	if respLen > 1500 {
		return
	}
	resp := make([]byte, respLen)
	if _, err := br.Read(resp); err != nil {
		return
	}

	_ = s.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, _ = s.conn.WriteToUDP(resp, raddr)
}

// @sk-task transparent-proxy#T5.4: local UDP DNS resolution for excluded domains
func (s *Server) resolveDirect(ctx context.Context, query []byte, raddr *net.UDPAddr, resolvers []string) {
	if len(resolvers) == 0 {
		return
	}
	for _, ns := range resolvers {
		func(ns string) {
			nsAddr, err := net.ResolveUDPAddr("udp", ns)
			if err != nil {
				return
			}
			conn, err := net.DialUDP("udp", nil, nsAddr)
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			if _, err := conn.Write(query); err != nil {
				return
			}
			resp := make([]byte, 1500)
			n, err := conn.Read(resp)
			if err != nil {
				return
			}
			_ = s.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
			_, _ = s.conn.WriteToUDP(resp[:n], raddr)
		}(ns)
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
	s.mu.Lock()
	streamID := s.nextID
	s.nextID++
	ch := make(chan []byte, 1)
	s.pending[streamID] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pending, streamID)
		s.mu.Unlock()
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
		_ = s.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		_, _ = s.conn.WriteToUDP(resp, raddr)
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
	}
}

// @sk-task transparent-proxy#T2.3: resolv.conf backup/restore (AC-009)
type ResolvConfBackup struct {
	original    string
	saved       bool
	nameservers []string
}

func BackupResolvConf() (*ResolvConfBackup, error) {
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return nil, err
	}
	var nss []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ns := parts[1]
				if !strings.Contains(ns, ":") {
					ns += ":53"
				}
				nss = append(nss, ns)
			}
		}
	}
	return &ResolvConfBackup{original: string(data), saved: true, nameservers: nss}, nil
}

func (b *ResolvConfBackup) Nameservers() []string {
	return b.nameservers
}

func (b *ResolvConfBackup) Restore() error {
	if !b.saved {
		return nil
	}
	if isSystemdResolved() {
		return resolvectlRevert()
	}
	return os.WriteFile(resolvConfPath, []byte(b.original), 0o644) // #nosec G306
}

func isSystemdResolved() bool {
	target, err := filepath.EvalSymlinks(resolvConfPath)
	if err != nil {
		return false
	}
	for _, p := range systemdResolvedLinks {
		if target == p {
			return true
		}
	}
	return false
}

func resolvectlSet(host string) error {
	return exec.Command("resolvectl", "dns", "lo", host).Run()
}

func resolvectlRevert() error {
	return exec.Command("resolvectl", "revert", "lo").Run()
}

func OverrideResolvConf(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return fmt.Errorf("dnsproxy: cannot override resolv.conf with empty address")
	}

	if isSystemdResolved() {
		return resolvectlSet(host)
	}

	return os.WriteFile(resolvConfPath, []byte("nameserver "+host+"\n"), 0o644) // #nosec G306
}

func readNameserver() (string, error) {
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ns := parts[1]
				if !strings.Contains(ns, ":") {
					ns += ":53"
				}
				return ns, nil
			}
		}
	}
	return "", fmt.Errorf("dnsproxy: no nameserver found in /etc/resolv.conf")
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
