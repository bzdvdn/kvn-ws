package transport

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/tunpool"
)

// Server accepts authenticated HTTP/2 streams and dials TCP/UDP targets.
type Server struct {
	tokens      TokenLookup
	fallback    http.Handler
	maxStreams  int
	sem         chan struct{}
	logger      *zap.Logger
	dialTimeout time.Duration
	pool        *tunpool.Pool
}

// ServerOption customizes optional server behavior.
type ServerOption func(*Server)

// WithTUNPool configures the server-side TUN lease pool.
func WithTUNPool(pool *tunpool.Pool) ServerOption {
	return func(s *Server) { s.pool = pool }
}

// NewServer creates a v2 stream server.
func NewServer(tokens TokenLookup, fallback http.Handler, maxStreams int, logger *zap.Logger, opts ...ServerOption) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	if maxStreams <= 0 {
		maxStreams = DefaultMaxStreams
	}
	if fallback == nil {
		fallback = http.NotFoundHandler()
	}
	s := &Server{
		tokens:      tokens,
		fallback:    fallback,
		maxStreams:  maxStreams,
		sem:         make(chan struct{}, maxStreams),
		logger:      logger,
		dialTimeout: 10 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == DefaultPath && r.Method == http.MethodGet && wantsJSON(r) && s.validToken(r):
		s.handleSession(w, r)
	case r.URL.Path == Path:
		s.handleTCP(w, r)
	case r.URL.Path == UDPPath:
		s.handleUDP(w, r)
	case r.URL.Path == DefaultPath && r.Method == http.MethodPost:
		s.handleTCP(w, r)
	case r.URL.Path == DefaultPath && r.Method == http.MethodPut:
		s.handleUDP(w, r)
	case r.URL.Path == DefaultPath && r.Method == http.MethodPatch:
		s.handleICMP(w, r)
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
	s.touchLease(r)
	pad := paddingRequested(r.Header.Get("Content-Type"))
	padSize := requestedPaddingSize(r.Header.Get("Content-Type"), 64)
	if !s.acquire(w) {
		return
	}
	defer s.release()
	dialer := net.Dialer{Timeout: s.dialTimeout}
	targetConn, err := dialer.DialContext(r.Context(), "tcp", target)
	if err != nil {
		s.logger.Warn("v2 target dial failed", zap.String("target", target), zap.Error(err))
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
	body := io.Reader(r.Body)
	var out io.Writer = fw
	if pad {
		body = newFrameReader(r.Body)
		out = newFrameWriter(fw, padSize)
	}
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		_, _ = io.CopyBuffer(out, targetConn, make([]byte, 32<<10))
	}()
	go func() {
		select {
		case <-r.Context().Done():
			_ = targetConn.Close()
		case <-copyDone:
		}
	}()
	_, _ = io.CopyBuffer(targetConn, body, make([]byte, 32<<10))
	if tc, ok := targetConn.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
	// Never return from the handler while the response goroutine may still
	// write/flush: net/http HTTP/2 panics with "Header called after Handler finished".
	<-copyDone
}

func (s *Server) handleUDP(w http.ResponseWriter, r *http.Request) {
	if (r.Method != http.MethodPost && r.Method != http.MethodPut) || !s.authorize(w, r) {
		return
	}
	target := targetFrom(r)
	if target == "" {
		http.Error(w, "missing target", http.StatusBadRequest)
		return
	}
	s.touchLease(r)
	pad := paddingRequested(r.Header.Get("Content-Type"))
	padSize := requestedPaddingSize(r.Header.Get("Content-Type"), 64)
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
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		buf := make([]byte, MaxDatagramSize)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, rerr := conn.Read(buf)
			if rerr != nil {
				var ne net.Error
				if errors.As(rerr, &ne) && ne.Timeout() {
					select {
					case <-r.Context().Done():
						return
					default:
						continue
					}
				}
				return
			}
			if werr := writeDatagram(fw, buf[:n], pad, padSize); werr != nil {
				return
			}
		}
	}()
	readDatagramStream(r.Body, pad, func(payload []byte) {
		_, _ = conn.Write(payload)
	})
	_ = conn.Close()
	<-copyDone
}

func (s *Server) validToken(r *http.Request) bool {
	token := bearerToken(r)
	if token == "" {
		return false
	}
	_, ok := s.tokens(token)
	return ok
}

func (s *Server) touchLease(r *http.Request) {
	if s.pool != nil {
		s.pool.Touch(strings.TrimSpace(r.Header.Get("X-Request-ID")))
	}
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) bool {
	if !s.validToken(r) {
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

func targetFrom(r *http.Request) string {
	if target := strings.TrimSpace(r.Header.Get(TargetHeader)); target != "" {
		return target
	}
	return strings.TrimSpace(r.Host)
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, prefix))
}
