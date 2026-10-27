// Package transport implements the KVN v2 HTTP/2 bidirectional stream transport.
package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	// Path is the standard HTTP/2 stream endpoint.
	Path = "/api/v2/stream"
	// TargetHeader carries the requested TCP target on an authenticated stream.
	TargetHeader = "X-KVN-Target"
	// DefaultMaxStreams bounds concurrent streams per server process.
	DefaultMaxStreams = 4096
)

// TokenLookup validates a bearer token and returns its name.
type TokenLookup func(token string) (string, bool)

// Server accepts authenticated HTTP/2 bidirectional streams and dials targets.
type Server struct {
	tokens      TokenLookup
	fallback    http.Handler
	maxStreams  int
	sem         chan struct{}
	logger      *zap.Logger
	dialTimeout time.Duration
}

// NewServer creates a v2 stream server. fallback is served for non-v2 or
// unauthenticated requests; if nil, a generic 404 is returned.
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
	if r.URL.Path != Path || r.Method != http.MethodPost {
		s.fallback.ServeHTTP(w, r)
		return
	}
	token := bearerToken(r)
	if token == "" {
		s.fallback.ServeHTTP(w, r)
		return
	}
	if _, ok := s.tokens(token); !ok {
		s.fallback.ServeHTTP(w, r)
		return
	}
	target := strings.TrimSpace(r.Header.Get(TargetHeader))
	if target == "" {
		http.Error(w, "missing target", http.StatusBadRequest)
		return
	}
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		http.Error(w, "too many streams", http.StatusTooManyRequests)
		return
	}

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
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.CopyBuffer(flushWriter{w: w}, targetConn, make([]byte, 32<<10))
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

type flushWriter struct {
	w http.ResponseWriter
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if f, ok := fw.w.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, prefix))
}

// Stream is a bidirectional v2 stream.
type Stream struct {
	body   io.ReadCloser
	writer *io.PipeWriter
	cancel context.CancelFunc
}

// Read implements io.Reader.
func (s *Stream) Read(p []byte) (int, error) { return s.body.Read(p) }

// Write implements io.Writer.
func (s *Stream) Write(p []byte) (int, error) { return s.writer.Write(p) }

// Close closes both directions.
func (s *Stream) Close() error {
	_ = s.writer.Close()
	err := s.body.Close()
	if s.cancel != nil {
		s.cancel()
	}
	return err
}

// Dial opens a v2 stream to target through endpoint.
func Dial(ctx context.Context, endpoint, token, target string, tlsCfg *tls.Config) (*Stream, error) {
	if tlsCfg == nil {
		return nil, errors.New("tls config is required")
	}
	tr := &http.Transport{
		TLSClientConfig:     tlsCfg,
		ForceAttemptHTTP2:   true,
		DisableCompression:  true,
		MaxIdleConnsPerHost: 4,
	}
	client := &http.Client{Transport: tr}
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		_ = pw.Close()
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set(TargetHeader, target)
	resp, err := client.Do(req)
	if err != nil {
		_ = pw.Close()
		return nil, fmt.Errorf("v2 stream dial: %w", err)
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/octet-stream") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		_ = pw.Close()
		return nil, fmt.Errorf("v2 stream status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	streamCtx, cancel := context.WithCancel(ctx)
	_ = streamCtx
	return &Stream{body: resp.Body, writer: pw, cancel: cancel}, nil
}
