package transport

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

type mockFactory struct {
	dialFn   func(ctx context.Context, endpoint string) (StreamConn, error)
	listenFn func(ctx context.Context, addr string) (TransportListener, error)
}

func (m *mockFactory) Dial(ctx context.Context, endpoint string) (StreamConn, error) {
	return m.dialFn(ctx, endpoint)
}

func (m *mockFactory) Listen(ctx context.Context, addr string) (TransportListener, error) {
	return m.listenFn(ctx, addr)
}

type mockStreamConn struct {
	StreamConn
}

var errMockFail = errors.New("mock dial failed")

// @sk-test transport-factory#T3.2: TestFallbackFactoryDialPrimaryFail verifies secondary is called (AC-007)
func TestFallbackFactoryDialPrimaryFail(t *testing.T) {
	logger := zap.NewNop()
	primary := &mockFactory{
		dialFn: func(ctx context.Context, endpoint string) (StreamConn, error) {
			return nil, errMockFail
		},
	}
	secondary := &mockFactory{
		dialFn: func(ctx context.Context, endpoint string) (StreamConn, error) {
			return &mockStreamConn{}, nil
		},
	}
	factory := NewFallbackFactory(primary, secondary, logger)
	conn, err := factory.Dial(context.Background(), "test")
	if err != nil {
		t.Fatalf("FallbackFactory.Dial failed: %v", err)
	}
	if conn == nil {
		t.Fatal("FallbackFactory.Dial returned nil conn")
	}
}

// @sk-test transport-factory#T3.2: TestFallbackFactoryDialPrimaryOK uses primary when it succeeds (AC-007)
func TestFallbackFactoryDialPrimaryOK(t *testing.T) {
	logger := zap.NewNop()
	primaryUsed := false
	primary := &mockFactory{
		dialFn: func(ctx context.Context, endpoint string) (StreamConn, error) {
			primaryUsed = true
			return &mockStreamConn{}, nil
		},
	}
	secondary := &mockFactory{
		dialFn: func(ctx context.Context, endpoint string) (StreamConn, error) {
			t.Fatal("secondary should not be called")
			return nil, nil
		},
	}
	factory := NewFallbackFactory(primary, secondary, logger)
	conn, err := factory.Dial(context.Background(), "test")
	if err != nil {
		t.Fatalf("FallbackFactory.Dial failed: %v", err)
	}
	if conn == nil {
		t.Fatal("FallbackFactory.Dial returned nil conn")
	}
	if !primaryUsed {
		t.Fatal("primary was not called")
	}
}
