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
	payload, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil || len(payload) < 8 {
		http.Error(w, "bad icmp payload", http.StatusBadRequest)
		return
	}
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		s.logger.Warn("icmp listen failed", zap.Error(err))
		http.Error(w, "icmp unavailable", http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.WriteTo(payload, addr); err != nil {
		http.Error(w, "icmp write failed", http.StatusBadGateway)
		return
	}
	id := binary.BigEndian.Uint16(payload[4:6])
	seq := binary.BigEndian.Uint16(payload[6:8])
	buf := make([]byte, 64<<10)
	for {
		n, _, rerr := conn.ReadFrom(buf)
		if rerr != nil {
			http.Error(w, "icmp timeout", http.StatusGatewayTimeout)
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
