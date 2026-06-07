package dnsproxy

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

var resolvConfPath = "/etc/resolv.conf"

// @sk-task transparent-proxy#T1.3: DNS proxy server skeleton (DEC-003)
type Server struct {
	listenAddr string
	upstream   string
	conn       *net.UDPConn
	mu         sync.Mutex
}

func New(listenAddr string) *Server {
	return &Server{listenAddr: listenAddr}
}

// @sk-task transparent-proxy#T2.3: DNS forwarder via TCP to upstream (AC-009)
func (s *Server) Run(ctx context.Context) error {
	upstream, err := readNameserver()
	if err != nil {
		return fmt.Errorf("dnsproxy: read upstream: %w", err)
	}
	s.mu.Lock()
	s.upstream = upstream
	s.mu.Unlock()

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

		s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, raddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
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
	s.mu.Lock()
	upstream := s.upstream
	s.mu.Unlock()

	dialer := net.Dialer{Timeout: 5 * time.Second}
	upConn, err := dialer.DialContext(ctx, "tcp", upstream)
	if err != nil {
		return
	}
	defer upConn.Close()

	// DNS TCP format: 2-byte length prefix + message
	wire := make([]byte, 2+len(query))
	wire[0] = byte(len(query) >> 8)
	wire[1] = byte(len(query) & 0xff)
	copy(wire[2:], query)

	upConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := upConn.Write(wire); err != nil {
		return
	}

	upConn.SetReadDeadline(time.Now().Add(5 * time.Second))
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

	s.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, _ = s.conn.WriteToUDP(resp, raddr)
}

// @sk-task transparent-proxy#T2.3: resolv.conf backup/restore (AC-009)
type ResolvConfBackup struct {
	original string
	saved    bool
}

func BackupResolvConf() (*ResolvConfBackup, error) {
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return nil, err
	}
	return &ResolvConfBackup{original: string(data), saved: true}, nil
}

func (b *ResolvConfBackup) Restore() error {
	if !b.saved {
		return nil
	}
	return os.WriteFile(resolvConfPath, []byte(b.original), 0644)
}

func OverrideResolvConf() error {
	return os.WriteFile(resolvConfPath, []byte("nameserver 127.0.0.53\n"), 0644)
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
					ns = ns + ":53"
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
