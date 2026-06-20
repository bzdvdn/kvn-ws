package proxy

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

var (
	ErrInvalidSOCKSVersion = errors.New("invalid SOCKS version")
	ErrUnsupportedCmd      = errors.New("unsupported command")
	ErrUnsupportedAddrType = errors.New("unsupported address type")
)

const (
	socksVersion5     = 0x05
	socksCmdConnect   = 0x01
	socksAtypIPv4     = 0x01
	socksAtypDomain   = 0x03
	socksAtypIPv6     = 0x04
	socksAuthNone     = 0x00
	socksAuthUserPass = 0x02
	socksRepSuccess   = 0x00
)

type ProxyAuth struct {
	Username string
	Password string
}

// @sk-task local-proxy-mode#T2.1: SOCKS5 listener (AC-001)
// @sk-task local-proxy-mode#T3.1: HTTP CONNECT handler (AC-002)
// @sk-task local-proxy-mode#T3.2: SOCKS5 auth (AC-005)
// @sk-task arch-refactoring#T3.5: defaultProxyConcurrency removed → Listener field (AC-006)
const defaultProxyConcurrency = 1000

type Listener struct {
	addr        string
	auth        *ProxyAuth
	ln          net.Listener
	onConn      func(conn net.Conn, dst string)
	sem         chan struct{}
	transparent bool
	logFn       func(format string, args ...any)
}

// @sk-task transparent-proxy#T2.2: enable transparent detection (AC-002, AC-003)
func (l *Listener) SetTransparent(v bool) {
	l.transparent = v
}

// @sk-task transparent-proxy#T5.2: debug logging for transparent connections
func (l *Listener) SetLogFn(fn func(format string, args ...any)) {
	l.logFn = fn
}

// @sk-task transparent-proxy#T5.2: debug logging for transparent connections
func (l *Listener) logf(format string, args ...any) {
	if l.logFn != nil {
		l.logFn(format, args...)
	}
}

// @sk-task arch-refactoring#T3.5: NewListener accepts optional concurrency param (AC-006)
func NewListener(addr string, auth *ProxyAuth, onConn func(net.Conn, string), proxyConcurrency ...int) *Listener {
	if addr == "" {
		addr = "127.0.0.1:2310"
	}
	maxConcurrency := defaultProxyConcurrency
	if len(proxyConcurrency) > 0 && proxyConcurrency[0] > 0 {
		maxConcurrency = proxyConcurrency[0]
	}
	return &Listener{
		addr:   addr,
		auth:   auth,
		onConn: onConn,
		sem:    make(chan struct{}, maxConcurrency),
	}
}

func (l *Listener) Start() error {
	var err error
	l.ln, err = net.Listen("tcp", l.addr)
	if err != nil {
		return fmt.Errorf("proxy listen %s: %w", l.addr, err)
	}
	return nil
}

func (l *Listener) Addr() net.Addr {
	if l.ln != nil {
		return l.ln.Addr()
	}
	return nil
}

func (l *Listener) Close() error {
	if l.ln != nil {
		return l.ln.Close()
	}
	return nil
}

// @sk-task local-proxy-mode#T3.1: detect SOCKS vs HTTP CONNECT (AC-002)
// @sk-task fix-critical-leaks#T3.2: proxy semaphore (AC-002)
func (l *Listener) AcceptLoop() error {
	for {
		client, err := l.ln.Accept()
		if err != nil {
			return err
		}
		l.sem <- struct{}{}
		go l.handleClient(client)
	}
}

// @sk-task fix-critical-leaks#T3.2: proxy semaphore (AC-002)
func (l *Listener) handleClient(client net.Conn) {
	defer func() { <-l.sem }()
	handedOff := false
	defer func() {
		if !handedOff {
			_ = client.Close()
		}
	}()

	buf := make([]byte, 1)
	_, err := io.ReadFull(client, buf)
	if err != nil {
		return
	}

	switch buf[0] {
	case socksVersion5:
		handedOff = l.handleSOCKS5(client, buf)
	case 'C':
		handedOff = l.handleHTTPConnect(client, buf)
	default:
		if l.transparent {
			handedOff = l.handleTransparent(client, buf[:1])
		}
		return
	}
}

// @sk-task local-proxy-mode#T3.2: SOCKS5 with optional auth (AC-005)
func (l *Listener) handleSOCKS5(client net.Conn, firstByte []byte) (handedOff bool) {
	buf := make([]byte, 1024)
	buf[0] = firstByte[0]

	n, err := io.ReadAtLeast(client, buf[1:], 2)
	if err != nil {
		return
	}
	n++
	if buf[0] != socksVersion5 {
		return
	}
	numMethods := int(buf[1])
	if n < 2+numMethods {
		_, err = io.ReadFull(client, buf[n:2+numMethods])
		if err != nil {
			return
		}
	}

	var authMethod byte = socksAuthNone
	if l.auth != nil {
		authMethod = socksAuthUserPass
	}
	_, _ = client.Write([]byte{socksVersion5, authMethod})

	// @sk-task local-proxy-mode#T3.2: RFC 1929 username/password auth (AC-005)
	if authMethod == socksAuthUserPass {
		n, err = io.ReadAtLeast(client, buf, 2)
		if err != nil {
			return
		}
		ver := buf[0]
		ulen := int(buf[1])
		if n < 2+ulen {
			_, err = io.ReadFull(client, buf[n:2+ulen])
			if err != nil {
				return
			}
		}
		uname := string(buf[2 : 2+ulen])
		offset := 2 + ulen
		if offset >= n {
			_, err = io.ReadFull(client, buf[n:offset+1])
			if err != nil {
				return
			}
		}
		plen := int(buf[offset])
		offset++
		if offset+plen > len(buf) {
			return
		}
		if offset+plen > n {
			_, err = io.ReadFull(client, buf[n:offset+plen])
			if err != nil {
				return
			}
		}
		pass := string(buf[offset : offset+plen])

		if ver != 0x01 || uname != l.auth.Username || pass != l.auth.Password {
			_, _ = client.Write([]byte{0x01, 0x01})
			return
		}
		_, _ = client.Write([]byte{0x01, 0x00})
	}

	n, err = io.ReadAtLeast(client, buf, 4)
	if err != nil {
		return
	}
	if buf[0] != socksVersion5 || buf[1] != socksCmdConnect {
		return
	}
	atyp := buf[3]

	var dst string
	switch atyp {
	case socksAtypIPv4:
		if n < 4+4+2 {
			_, err = io.ReadFull(client, buf[n:4+4+2])
			if err != nil {
				return
			}
		}
		ip := net.IP(buf[4:8])
		port := binary.BigEndian.Uint16(buf[8:10])
		dst = fmt.Sprintf("%s:%d", ip, port)
	case socksAtypIPv6:
		if n < 4+16+2 {
			_, err = io.ReadFull(client, buf[n:4+16+2])
			if err != nil {
				return
			}
		}
		ip := net.IP(buf[4:20])
		port := binary.BigEndian.Uint16(buf[20:22])
		dst = fmt.Sprintf("[%s]:%d", ip, port)
	case socksAtypDomain:
		if n < 5 {
			_, err = io.ReadFull(client, buf[n:5])
			if err != nil {
				return
			}
		}
		dlen := int(buf[4])
		if n < 5+dlen+2 {
			_, err = io.ReadFull(client, buf[n:5+dlen+2])
			if err != nil {
				return
			}
		}
		domain := string(buf[5 : 5+dlen])
		port := binary.BigEndian.Uint16(buf[5+dlen : 5+dlen+2])
		dst = fmt.Sprintf("%s:%d", domain, port)
	default:
		return
	}

	localAddr := client.LocalAddr()
	var bnd []byte
	if localAddr != nil {
		tcpAddr, ok := localAddr.(*net.TCPAddr)
		if !ok {
			_ = client.Close()
			return
		}
		localIP := tcpAddr.IP
		localPort := tcpAddr.Port
		if ip4 := localIP.To4(); ip4 != nil {
			bnd = []byte{socksVersion5, socksRepSuccess, 0x00, socksAtypIPv4,
				ip4[0], ip4[1], ip4[2], ip4[3],
				byte(localPort >> 8 & 0xff), byte(localPort & 0xff),
			}
		} else if ip6 := localIP.To16(); ip6 != nil {
			bnd = make([]byte, 4+16+2)
			bnd[0] = socksVersion5
			bnd[1] = socksRepSuccess
			bnd[2] = 0x00
			bnd[3] = socksAtypIPv6
			copy(bnd[4:20], ip6)
			bnd[20] = byte(localPort >> 8 & 0xff)
			bnd[21] = byte(localPort & 0xff)
		}
	}
	if bnd == nil {
		bnd = []byte{socksVersion5, socksRepSuccess, 0x00, socksAtypIPv4, 0, 0, 0, 0, 0, 0}
	}
	_, _ = client.Write(bnd)

	if l.onConn != nil {
		l.onConn(client, dst)
		return true
	}
	return false
}

// @sk-task transparent-proxy#T2.2: transparent connection handler via SO_ORIGINAL_DST (AC-002, AC-003)
func (l *Listener) handleTransparent(client net.Conn, firstByte []byte) bool {
	dst, err := getOriginalDst(client)
	if err != nil {
		l.logf("getOriginalDst failed: %v", err)
		return false
	}
	localAddr := client.LocalAddr()
	remoteAddr := client.RemoteAddr()
	l.logf("transparent dst=%s local=%s remote=%s first_byte=0x%02x", dst, localAddr, remoteAddr, firstByte[0])
	if l.onConn != nil {
		wrapped := &prependConn{
			Conn:      client,
			pending:   firstByte,
			logf:      l.logf,
		}
		l.onConn(wrapped, dst)
		return true
	}
	return false
}

type prependConn struct {
	net.Conn
	pending  []byte
	logf     func(format string, args ...any)
	done     bool
}

func (c *prependConn) Read(b []byte) (int, error) {
	if !c.done && len(c.pending) > 0 {
		c.done = true
		n := copy(b, c.pending)
		remaining := b[n:]
		if len(remaining) > 0 {
			more, err := c.Conn.Read(remaining)
			n += more
			if c.logf != nil {
				c.logf("prependConn.Read: n=%d err=%v", n, err)
			}
			return n, err
		}
		if c.logf != nil {
			c.logf("prependConn.Read: n=%d", n)
		}
		return n, nil
	}
	n, err := c.Conn.Read(b)
	if n > 0 && c.logf != nil {
		c.logf("prependConn.Read: n=%d err=%v", n, err)
	}
	return n, err
}

// @sk-task local-proxy-mode#T3.1: HTTP CONNECT handler (AC-002)
func (l *Listener) handleHTTPConnect(client net.Conn, firstByte []byte) (handedOff bool) {
	br := bufio.NewReader(io.MultiReader(
		strings.NewReader(string(firstByte)),
		client,
	))

	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if req.Method != "CONNECT" {
		return
	}
	dst := req.Host
	if dst == "" {
		dst = req.RequestURI
	}

	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	if l.onConn != nil {
		buffered := make([]byte, br.Buffered())
		_, _ = br.Read(buffered)
		wrapped := &prependConn{
			Conn:      client,
			pending:   buffered,
		}
		l.onConn(wrapped, dst)
		return true
	}
	return false
}
