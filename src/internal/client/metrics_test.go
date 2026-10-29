package client

import (
	"sync/atomic"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

type countingFakeTun struct {
	tun.TunDevice
}

func (f *countingFakeTun) Read(p []byte) (int, error)  { return copy(p, "out"), nil }
func (f *countingFakeTun) Write(p []byte) (int, error) { return len(p), nil }

func TestCountingTunDirections(t *testing.T) {
	var rx, tx atomic.Int64
	dev := &countingTun{TunDevice: &countingFakeTun{}, rx: &rx, tx: &tx}
	buf := make([]byte, 16)
	if _, err := dev.Read(buf); err != nil {
		t.Fatal(err)
	}
	if tx.Load() != 3 || rx.Load() != 0 {
		t.Fatalf("Read: rx=%d tx=%d, want rx=0 tx=3", rx.Load(), tx.Load())
	}
	if _, err := dev.Write([]byte("inbound")); err != nil {
		t.Fatal(err)
	}
	if rx.Load() != 7 || tx.Load() != 3 {
		t.Fatalf("Write: rx=%d tx=%d, want rx=7 tx=3", rx.Load(), tx.Load())
	}
}
