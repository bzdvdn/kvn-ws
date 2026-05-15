// @sk-test post-hardening#T4.3: TestSessionStreams (AC-012)
package proxy

import (
	"net"
	"testing"
)

func TestSessionStreamsCRUD(t *testing.T) {
	ss := &SessionStreams{M: make(map[uint32]net.Conn)}

	_, ok := ss.Load(1)
	if ok {
		t.Error("expected Load to return false for empty map")
	}

	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close(); _ = c2.Close() }()

	ss.Store(1, c1)
	v, ok := ss.Load(1)
	if !ok {
		t.Fatal("expected Load to return true after Store")
	}
	if v != c1 {
		t.Error("expected Load to return the stored conn")
	}

	ss.Delete(1)
	_, ok = ss.Load(1)
	if ok {
		t.Error("expected Load to return false after Delete")
	}
}

func TestSessionStreamsCloseAll(t *testing.T) {
	ss := &SessionStreams{M: make(map[uint32]net.Conn)}
	c1, c2 := net.Pipe()
	c3, c4 := net.Pipe()

	ss.Store(1, c1)
	ss.Store(2, c3)

	ss.CloseAll()

	if _, err := c2.Read(make([]byte, 1)); err == nil {
		t.Error("expected c2 (pipe end) to be closed after CloseAll")
	}
	if _, err := c4.Read(make([]byte, 1)); err == nil {
		t.Error("expected c4 (pipe end) to be closed after CloseAll")
	}

	if _, ok := ss.Load(1); ok {
		t.Error("expected map to be empty after CloseAll")
	}
}
