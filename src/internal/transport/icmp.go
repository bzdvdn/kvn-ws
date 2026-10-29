package transport

import (
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/icmp"

	"go.uber.org/zap"
)

func (s *Server) handleICMP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch || !s.authorize(w, r) {
		return
	}
	target := targetFrom(r)
	if target == "" {
		http.Error(w, "missing target", http.StatusBadRequest)
		return
	}
	if !s.acquire(w) {
		return
	}
	defer s.release()
	addr, err := net.ResolveIPAddr("ip4", target)
	if err != nil {
		http.Error(w, "bad target", http.StatusBadRequest)
		return
	}
	// Send response headers before reading the request body. The client uses a
	// streaming HTTP/2 request and can only start writing after Do returns.
	w.Header().Set("Content-Type", "application/icmp")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	payload, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil || len(payload) < 8 {
		s.logger.Debug("v2 icmp payload rejected", zap.Error(err), zap.Int("len", len(payload)))
		return
	}
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		s.logger.Warn("icmp listen failed", zap.Error(err))
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.WriteTo(payload, addr); err != nil {
		s.logger.Warn("icmp write failed", zap.Error(err))
		return
	}
	id := binary.BigEndian.Uint16(payload[4:6])
	seq := binary.BigEndian.Uint16(payload[6:8])
	buf := make([]byte, 64<<10)
	for {
		n, _, rerr := conn.ReadFrom(buf)
		if rerr != nil {
			s.logger.Debug("icmp reply timeout", zap.Error(rerr))
			return
		}
		if n < 8 || buf[0] != 0 || binary.BigEndian.Uint16(buf[4:6]) != id || binary.BigEndian.Uint16(buf[6:8]) != seq {
			continue
		}
		w.Header().Set("Content-Type", "application/icmp")
		_, _ = w.Write(buf[:n])
		return
	}
}
