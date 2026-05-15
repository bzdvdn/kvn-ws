// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task production-hardening#T4.1: keepalive support (AC-002)

// @sk-task security-acl#T4: Origin/Referer validation with configurable CheckOrigin
package websocket

import (
	"bytes"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const MultiplexSubprotocol = "kvn-ws-mux"

// @sk-task performance-and-polish#T1.1: WSConfig for Dial/Accept options (AC-004, AC-006, AC-007)
type WSConfig struct {
	Compression bool
	Multiplex   bool
	MTU         int
}

// @sk-task core-tunnel-mvp#T2.1: WebSocket connection wrapper (AC-002)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
type WSConn struct {
	conn   *websocket.Conn
	cfg    WSConfig
	logger *zap.Logger
}

// @sk-task performance-and-polish#T2.3: BatchWriter for coalescing writes (AC-003)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
// @sk-task production-readiness-hardening#T2.3: idempotent Close via sync.Once (AC-003)
type BatchWriter struct {
	conn      *WSConn
	buf       bytes.Buffer
	mu        sync.Mutex
	threshold int
	ticker    *time.Ticker
	stopCh    chan struct{}
	logger    *zap.Logger
	closeOnce sync.Once
}

// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
func NewBatchWriter(conn *WSConn, threshold int, flushInterval time.Duration, logger *zap.Logger) *BatchWriter {
	bw := &BatchWriter{
		conn:      conn,
		threshold: threshold,
		ticker:    time.NewTicker(flushInterval),
		stopCh:    make(chan struct{}),
		logger:    logger,
	}
	go bw.flushLoop()
	return bw
}

func (bw *BatchWriter) Write(data []byte) error {
	bw.mu.Lock()
	bw.buf.Write(data)
	size := bw.buf.Len()
	bw.mu.Unlock()

	if size >= bw.threshold {
		return bw.Flush()
	}
	return nil
}

func (bw *BatchWriter) Flush() error {
	bw.mu.Lock()
	if bw.buf.Len() == 0 {
		bw.mu.Unlock()
		return nil
	}
	data := make([]byte, bw.buf.Len())
	copy(data, bw.buf.Bytes())
	bw.buf.Reset()
	bw.mu.Unlock()

	return bw.conn.WriteMessage(data)
}

func (bw *BatchWriter) flushLoop() {
	for {
		select {
		case <-bw.ticker.C:
			_ = bw.Flush()
		case <-bw.stopCh:
			bw.ticker.Stop()
			return
		}
	}
}

// @sk-task production-readiness-hardening#T2.3: idempotent Close via sync.Once (AC-003)
func (bw *BatchWriter) Close() error {
	var err error
	bw.closeOnce.Do(func() {
		close(bw.stopCh)
		err = bw.Flush()
	})
	return err
}

// @sk-task production-readiness-hardening#T2.1: deadline helpers for WSConn (AC-001)
func (c *WSConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// @sk-task production-readiness-hardening#T2.1: deadline helpers for WSConn (AC-001)
func (c *WSConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *WSConn) ReadMessage() ([]byte, error) {
	_, msg, err := c.conn.ReadMessage()
	return msg, err
}

func (c *WSConn) WriteMessage(data []byte) error {
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *WSConn) Close() error {
	return c.conn.Close()
}

func (c *WSConn) Underlying() *websocket.Conn {
	return c.conn
}

func (c *WSConn) Subprotocol() string {
	return c.conn.Subprotocol()
}

// @sk-task production-hardening#T4.1: set keepalive with ping/pong (AC-002)
// @sk-task production-readiness-hardening#T2.6: log.Printf → zap (AC-006)
func (c *WSConn) SetKeepalive(interval, timeout time.Duration) {
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(timeout))
	})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.logger.Warn("ping error", zap.Error(err))
				return
			}
		}
	}()
}

// @sk-task performance-and-polish#T2.2: TCP_NODELAY via NetDial (AC-002)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
func Dial(serverURL string, tlsConfig *tls.Config, logger *zap.Logger, cfg ...WSConfig) (*WSConn, error) {
	var wsCfg WSConfig
	if len(cfg) > 0 {
		wsCfg = cfg[0]
	}
	d := websocket.Dialer{
		TLSClientConfig:   tlsConfig,
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: wsCfg.Compression,
	}
	if wsCfg.Multiplex {
		d.Subprotocols = []string{MultiplexSubprotocol}
	}
	d.NetDial = func(network, addr string) (net.Conn, error) {
		conn, err := net.Dial(network, addr)
		if err != nil {
			return conn, err
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetNoDelay(true)
		}
		return conn, nil
	}
	conn, _, err := d.Dial(serverURL, nil)
	if err != nil {
		return nil, err
	}
	if wsCfg.Compression {
		_ = conn.SetCompressionLevel(4)
	}
	// @sk-task post-hardening#T2.1: cap incoming message size (AC-005)
	conn.SetReadLimit(1 << 20) // 1MB
	return &WSConn{conn: conn, cfg: wsCfg, logger: logger}, nil
}

// @sk-task security-acl#T4: NewOriginChecker creates origin check function from whitelist
// @sk-task post-hardening#T1.3: fix origin pattern matching — use glob/fnmatch instead of path.Match (AC-003)
func NewOriginChecker(whitelist []string, allowEmpty bool) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = r.Header.Get("Referer")
		}
		if origin == "" {
			return allowEmpty
		}
		for _, p := range whitelist {
			if matchOriginPattern(p, origin) {
				return true
			}
		}
		return false
	}
}

func matchOriginPattern(pattern, origin string) bool {
	if pattern == "" {
		return false
	}
	starIdx := -1
	for i, c := range pattern {
		if c == '*' {
			starIdx = i
			break
		}
	}
	if starIdx < 0 {
		return pattern == origin
	}
	prefix := pattern[:starIdx]
	suffix := pattern[starIdx+1:]
	if !strings.HasPrefix(origin, prefix) {
		return false
	}
	if suffix == "" {
		return true
	}
	return len(origin) >= len(suffix) && origin[len(origin)-len(suffix):] == suffix
}

// @sk-task performance-and-polish#T2.2: TCP_NODELAY after Upgrade (AC-002)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
func Accept(w http.ResponseWriter, r *http.Request, logger *zap.Logger, originCheckers ...interface{}) (*WSConn, error) {
	var cfg WSConfig
	var checkOrigin func(r *http.Request) bool
	checkOrigin = func(r *http.Request) bool { return true }

	for _, arg := range originCheckers {
		switch v := arg.(type) {
		case func(r *http.Request) bool:
			checkOrigin = v
		case WSConfig:
			cfg = v
		}
	}

	upgrader := websocket.Upgrader{
		CheckOrigin:        checkOrigin,
		EnableCompression:  cfg.Compression,
	}
	if cfg.Multiplex {
		upgrader.Subprotocols = []string{MultiplexSubprotocol}
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	if tcpConn, ok := conn.UnderlyingConn().(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}
	if cfg.Compression {
		_ = conn.SetCompressionLevel(4)
	}
	conn.SetPingHandler(func(appData string) error {
		return conn.WriteMessage(websocket.PongMessage, nil)
	})
	return &WSConn{conn: conn, cfg: cfg, logger: logger}, nil
}

func ResetUpgrader() {}
