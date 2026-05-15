// @sk-task local-proxy-mode#T2.1: SOCKS5 listener (AC-001)
// @sk-task local-proxy-mode#T3.1: HTTP CONNECT handler (AC-002)
// @sk-task local-proxy-mode#T3.2: SOCKS5 auth (AC-005)
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

	"github.com/bzdvdn/kvn-ws/src/internal/config"
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

type Listener struct {
	cfg    config.ClientConfig
	ln     net.Listener
	onConn func(conn net.Conn, dst string)
}

func NewListener(cfg config.ClientConfig, onConn func(net.Conn, string)) *Listener {
	return &Listener{cfg: cfg, onConn: onConn}
}

func (l *Listener) Start() error {
	addr := l.cfg.ProxyListen
	if addr == "" {
		addr = "127.0.0.1:2310"
	}
	var err error
	l.ln, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("proxy listen %s: %w", addr, err)
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
func (l *Listener) AcceptLoop() error {
	for {
		client, err := l.ln.Accept()
		if err != nil {
			return err
		}
		go l.handleClient(client)
	}
}

func (l *Listener) handleClient(client net.Conn) {
	defer func() { _ = client.Close() }()

	buf := make([]byte, 1)
	_, err := io.ReadFull(client, buf)
	if err != nil {
		return
	}

	switch buf[0] {
	case socksVersion5:
		l.handleSOCKS5(client, buf)
	case 'C':
		l.handleHTTPConnect(client, buf)
	default:
		return
	}
}

// @sk-task local-proxy-mode#T3.2: SOCKS5 with optional auth (AC-005)
func (l *Listener) handleSOCKS5(client net.Conn, firstByte []byte) {
	buf := make([]byte, 1024)
	buf[0] = firstByte[0]

	n, err := io.ReadAtLeast(client, buf, 2)
	if err != nil {
		return
	}
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
	if l.cfg.ProxyAuth != nil {
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

		if ver != 0x01 || uname != l.cfg.ProxyAuth.Username || pass != l.cfg.ProxyAuth.Password {
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
	}
}

// @sk-task local-proxy-mode#T3.1: HTTP CONNECT handler (AC-002)
func (l *Listener) handleHTTPConnect(client net.Conn, firstByte []byte) {
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
		l.onConn(client, dst)
	}
}
