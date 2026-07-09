package quic

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

type mockAcceptor struct {
	failCount atomic.Int64
	err       error
}

func (m *mockAcceptor) accept(ctx context.Context) (*QUICConn, error) {
	if m.failCount.Add(-1) >= 0 {
		return nil, m.err
	}
	return NewQUICConn(nil, &mockStream{}), nil
}

// @sk-test arch-fix-critical-paths#T2.1: AcceptWithBackoff succeeds after transient errors (AC-001)
func TestAcceptWithBackoffTransientErrors(t *testing.T) {
	m := &mockAcceptor{}
	m.failCount.Store(3)
	m.err = errors.New("transient error")

	logger := zap.NewNop()
	ctx := context.Background()

	conn, err := AcceptWithBackoff(ctx, m.accept, logger)
	if err != nil {
		t.Fatalf("AcceptWithBackoff failed after transient errors: %v", err)
	}
	if conn == nil {
		t.Fatal("AcceptWithBackoff returned nil conn")
	}
}

// @sk-test arch-fix-critical-paths#T2.1: AcceptWithBackoff returns context.Canceled immediately (AC-001)
func TestAcceptWithBackoffContextCanceled(t *testing.T) {
	m := &mockAcceptor{}
	m.failCount.Store(100)
	m.err = context.Canceled

	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := AcceptWithBackoff(ctx, m.accept, logger)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// @sk-test arch-fix-critical-paths#T2.1: AcceptWithBackoff succeeds on first try (AC-001)
func TestAcceptWithBackoffFirstTry(t *testing.T) {
	m := &mockAcceptor{}
	m.failCount.Store(0)

	logger := zap.NewNop()
	ctx := context.Background()

	conn, err := AcceptWithBackoff(ctx, m.accept, logger)
	if err != nil {
		t.Fatalf("AcceptWithBackoff failed on first try: %v", err)
	}
	if conn == nil {
		t.Fatal("AcceptWithBackoff returned nil conn")
	}
}

// @sk-test arch-fix-critical-paths#T2.1: AcceptWithBackoff stops on context timeout during backoff (AC-001)
func TestAcceptWithBackoffContextTimeout(t *testing.T) {
	m := &mockAcceptor{}
	m.failCount.Store(100)
	m.err = errors.New("transient")

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := AcceptWithBackoff(ctx, m.accept, logger)
	if err == nil {
		t.Fatal("expected error on context timeout, got nil")
	}
}
