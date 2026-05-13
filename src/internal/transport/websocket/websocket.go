// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task production-hardening#T4.1: keepalive support (AC-002)

package websocket

import (
	"crypto/tls"
	"log"
	"net/http"
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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func Accept(w http.ResponseWriter, r *http.Request) (*WSConn, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	conn.SetPingHandler(func(appData string) error {
		return conn.WriteMessage(websocket.PongMessage, nil)
	})
	return &WSConn{conn: conn}, nil
}

func ResetUpgrader() {
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
}
