// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task production-hardening#T4.1: keepalive support (AC-002)

// @sk-task security-acl#T4: Origin/Referer validation with configurable CheckOrigin
package websocket

import (
	"crypto/tls"
	"log"
	"net/http"
	"path"
	"time"

	"github.com/gorilla/websocket"
)

// @sk-task core-tunnel-mvp#T2.1: WebSocket connection wrapper (AC-002)
type WSConn struct {
	conn *websocket.Conn
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

// @sk-task production-hardening#T4.1: set keepalive with ping/pong (AC-002)
func (c *WSConn) SetKeepalive(interval, timeout time.Duration) {
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(timeout))
	})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[ws] ping error: %v", err)
				return
			}
		}
	}()
}

func Dial(serverURL string, tlsConfig *tls.Config) (*WSConn, error) {
	d := websocket.Dialer{
		TLSClientConfig: tlsConfig,
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := d.Dial(serverURL, nil)
	if err != nil {
		return nil, err
	}
	return &WSConn{conn: conn}, nil
}

// @sk-task security-acl#T4: NewOriginChecker creates origin check function from whitelist
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
			matched, err := path.Match(p, origin)
			if err == nil && matched {
				return true
			}
		}
		return false
	}
}

func Accept(w http.ResponseWriter, r *http.Request, originCheckers ...func(r *http.Request) bool) (*WSConn, error) {
	var checkOrigin func(r *http.Request) bool
	if len(originCheckers) > 0 && originCheckers[0] != nil {
		checkOrigin = originCheckers[0]
	} else {
		checkOrigin = func(r *http.Request) bool { return true }
	}
	upgrader := websocket.Upgrader{
		CheckOrigin: checkOrigin,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	conn.SetPingHandler(func(appData string) error {
		return conn.WriteMessage(websocket.PongMessage, nil)
	})
	return &WSConn{conn: conn}, nil
}

func ResetUpgrader() {}
