package client

import (
	"testing"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

func TestPingConcurrencyBounded(t *testing.T) {
	c := New(&config.V2ClientConfig{}, zap.NewNop())
	acquired := 0
	for c.tryAcquirePing() {
		acquired++
	}
	if acquired != maxConcurrentPings {
		t.Fatalf("acquired %d ping slots, want %d", acquired, maxConcurrentPings)
	}
	if c.tryAcquirePing() {
		t.Fatal("acquired a ping slot beyond the limit")
	}
	c.releasePing()
	if !c.tryAcquirePing() {
		t.Fatal("released slot was not reusable")
	}
	for i := 0; i < acquired; i++ {
		c.releasePing()
	}
}
