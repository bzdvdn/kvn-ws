package transport

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Server accepts authenticated HTTP/2 streams and dials TCP/UDP targets.
type Server struct {
	tokens      TokenLookup
	fallback    http.Handler
	maxStreams  int
	sem         chan struct{}
	logger      *zap.Logger
	dialTimeout time.Duration
}

// NewServer creates a v2 stream server.
func NewServer(tokens TokenLookup, fallback http.Handler, maxStreams int, logger *zap.Logger) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	if maxStreams <= 0 {
		maxStreams = DefaultMaxStreams
	}
	if fallback == nil {
		fallback = http.NotFoundHandler()
	}
	return &Server{
		tokens:      tokens,
		fallback:    fallback,
		maxStreams:  maxStreams,
		sem:         make(chan struct{}, maxStreams),
		logger:      logger,
		dialTimeout: 10 * time.Second,
	}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case Path:
		s.handleTCP(w, r)
	case UDPPath:
		s.handleUDP(w, r)
	default:
		s.fallback.ServeHTTP(w, r)
	}
}

func (s *Server) handleTCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.authorize(w, r) {
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
	dialer := net.Dialer{Timeout: s.dialTimeout}
	targetConn, err := dialer.DialContext(r.Context(), "tcp", target)
	if err != nil {
		s.logger.Debug("v2 target dial failed", zap.String("target", target), zap.Error(err))
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer func() { _ = targetConn.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	fw := flushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		fw.fw = f
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.CopyBuffer(fw, targetConn, make([]byte, 32<<10))
	}()
	_, _ = io.CopyBuffer(targetConn, r.Body, make([]byte, 32<<10))
	if tc, ok := targetConn.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
	select {
	case <-done:
	case <-r.Context().Done():
	}
}

func (s *Server) handleUDP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.authorize(w, r) {
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
	addr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		http.Error(w, "bad target", http.StatusBadRequest)
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer func() { _ = conn.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	fw := flushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		fw.fw = f
	}
	fw.fw.Flush()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, MaxDatagramSize)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, rerr := conn.Read(buf)
			if rerr != nil {
				if ne, ok := rerr.(net.Error); ok && ne.Timeout() {
					select {
					case <-r.Context().Done():
						return
					default:
						continue
					}
				}
				return
			}
			if werr := writeDatagram(fw, buf[:n]); werr != nil {
				return
			}
		}
	}()
	readDatagramStream(r.Body, func(payload []byte) {
		_, _ = conn.Write(payload)
	})
	select {
	case <-done:
	case <-r.Context().Done():
	}
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) bool {
	token := bearerToken(r)
	if token == "" {
		s.fallback.ServeHTTP(w, r)
		return false
	}
	if _, ok := s.tokens(token); !ok {
		s.fallback.ServeHTTP(w, r)
		return false
	}
	return true
}

func (s *Server) acquire(w http.ResponseWriter) bool {
	select {
	case s.sem <- struct{}{}:
		return true
	default:
		http.Error(w, "too many streams", http.StatusTooManyRequests)
		return false
	}
}

func (s *Server) release() { <-s.sem }

func targetFrom(r *http.Request) string { return strings.TrimSpace(r.Header.Get(TargetHeader)) }

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, prefix))
}
