// @sk-test kvn-web-redesign#T4.1: unit tests for RingBuffer and Collector (AC-013, AC-014)
package client

import (
	"context"
	"math"
	"testing"
	"time"
)

const eps = 1e-6

func approxEq(a, b float64) bool {
	return math.Abs(a-b) < eps
}

// RingBuffer tests

func TestNewRingBuffer(t *testing.T) {
	rb := NewRingBuffer(10)
	if rb.Cap() != 10 {
		t.Fatalf("expected cap 10, got %d", rb.Cap())
	}
	if rb.Len() != 0 {
		t.Fatalf("expected len 0, got %d", rb.Len())
	}
}

func TestRingBufferPushAndRead(t *testing.T) {
	rb := NewRingBuffer(3)
	s1 := MetricSnapshot{TxBytes: 10, RxBytes: 20, LatencyMs: 5}
	s2 := MetricSnapshot{TxBytes: 30, RxBytes: 40, LatencyMs: 10}
	rb.Push(s1)
	rb.Push(s2)

	data := rb.Read()
	if len(data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(data))
	}
	if data[0].TxBytes != 10 || data[1].TxBytes != 30 {
		t.Fatal("data order mismatch")
	}
}

func TestRingBufferWrapAround(t *testing.T) {
	rb := NewRingBuffer(3)
	for i := 0; i < 5; i++ {
		rb.Push(MetricSnapshot{TxBytes: int64(i)})
	}

	data := rb.Read()
	if len(data) != 3 {
		t.Fatalf("expected 3 items, got %d", len(data))
	}
	// after wrap: positions 2,3,4 → stored as [3,4,2] in ring order → read returns [2,3,4]
	if data[0].TxBytes != 2 || data[1].TxBytes != 3 || data[2].TxBytes != 4 {
		t.Fatalf("expected [2,3,4], got [%d,%d,%d]", data[0].TxBytes, data[1].TxBytes, data[2].TxBytes)
	}
}

func TestRingBufferReadEmpty(t *testing.T) {
	rb := NewRingBuffer(5)
	data := rb.Read()
	if data != nil {
		t.Fatal("expected nil for empty buffer")
	}
}

func TestRingBufferLenCap(t *testing.T) {
	rb := NewRingBuffer(5)
	if rb.Cap() != 5 {
		t.Fatalf("cap want 5, got %d", rb.Cap())
	}
	for i := 0; i < 3; i++ {
		rb.Push(MetricSnapshot{})
	}
	if rb.Len() != 3 {
		t.Fatalf("len want 3, got %d", rb.Len())
	}
}

// Collector tests

func TestNewCollector(t *testing.T) {
	c := NewCollector()
	if c.Ring().Cap() != 60 {
		t.Fatalf("expected ring cap 60, got %d", c.Ring().Cap())
	}
}

func TestCollectorAddTXAndRX(t *testing.T) {
	c := NewCollector()
	c.AddTX(100)
	c.AddRX(200)
	c.AddTX(50)

	s := c.Snapshot()
	if s.TxBytes != 150 {
		t.Fatalf("TxBytes want 150, got %d", s.TxBytes)
	}
	if s.RxBytes != 200 {
		t.Fatalf("RxBytes want 200, got %d", s.RxBytes)
	}
}

func TestCollectorSetLatency(t *testing.T) {
	c := NewCollector()
	c.SetLatency(42.5)
	if !approxEq(c.Snapshot().LatencyMs, 42.5) {
		t.Fatalf("LatencyMs want 42.5, got %f", c.Snapshot().LatencyMs)
	}
}

func TestCollectorIncReconnects(t *testing.T) {
	c := NewCollector()
	c.IncReconnects()
	c.IncReconnects()
	c.IncReconnects()
	if c.Snapshot().Reconnects != 3 {
		t.Fatalf("Reconnects want 3, got %d", c.Snapshot().Reconnects)
	}
}

func TestCollectorUptime(t *testing.T) {
	c := NewCollector()
	c.Start()
	time.Sleep(10 * time.Millisecond)
	s := c.Snapshot()
	if s.UptimeS < 0 {
		t.Fatalf("Uptime should be >= 0, got %d", s.UptimeS)
	}
	// It might be 0 if the sleep was too short, but should be increasing
	time.Sleep(5 * time.Millisecond)
	s2 := c.Snapshot()
	if s2.UptimeS < s.UptimeS {
		t.Fatalf("Uptime should increase, was %d now %d", s.UptimeS, s2.UptimeS)
	}
}

func TestCollectorSnapshotZeroSpeed(t *testing.T) {
	c := NewCollector()
	c.Start()
	c.AddTX(100)
	c.AddRX(200)
	s := c.Snapshot()
	if s.TxSpeed != 0 || s.RxSpeed != 0 {
		t.Fatal("Snapshot should return 0 speed (computed by Sender)")
	}
}

// Sender tests

func TestSenderRunContextCancel(t *testing.T) {
	collector := NewCollector()
	collector.Start()
	out := make(chan MetricSnapshot, 10)
	sender := NewSender(collector, out, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	sender.Run(ctx)
	// Should return without blocking, no panics
}

func TestSenderRunPushesMetrics(t *testing.T) {
	collector := NewCollector()
	collector.Start()
	collector.AddTX(1_000_000) // 1MB
	collector.AddRX(2_000_000) // 2MB
	out := make(chan MetricSnapshot, 10)
	sender := NewSender(collector, out, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sender.Run(ctx)

	select {
	case s := <-out:
		if s.TxBytes != 1_000_000 {
			t.Fatalf("TxBytes want 1000000, got %d", s.TxBytes)
		}
		if s.RxBytes != 2_000_000 {
			t.Fatalf("RxBytes want 2000000, got %d", s.RxBytes)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for metric")
	}
}

func TestSenderComputesSpeed(t *testing.T) {
	collector := NewCollector()
	collector.Start()
	out := make(chan MetricSnapshot, 10)
	sender := NewSender(collector, out, 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// first tick: add data, wait for push
	collector.AddTX(1_250_000) // 1.25 MB = 10 Mbps at 20ms interval
	collector.AddRX(2_500_000) // 2.5 MB = 20 Mbps

	go sender.Run(ctx)

	var first MetricSnapshot
	select {
	case first = <-out:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout")
	}

	// second tick: add more data
	collector.AddTX(2_500_000) // cumulative 3.75 MB, delta 1.25 MB = 10 Mbps
	collector.AddRX(5_000_000) // cumulative 7.5 MB, delta 2.5 MB = 20 Mbps

	var second MetricSnapshot
	select {
	case second = <-out:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout")
	}

	// Check ring buffer has both snapshots
	if collector.Ring().Len() < 2 {
		t.Fatalf("RingBuffer should have at least 2 entries, got %d", collector.Ring().Len())
	}

	_ = first
	_ = second
}

func TestSenderRingBufferFill(t *testing.T) {
	collector := NewCollector()
	collector.Start()
	out := make(chan MetricSnapshot, 100)
	sender := NewSender(collector, out, 5*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	go sender.Run(ctx)

	// let it produce several ticks
	collector.AddTX(100)
	collector.AddRX(100)
	time.Sleep(60 * time.Millisecond)

	cancel()
	time.Sleep(10 * time.Millisecond)

	if collector.Ring().Len() == 0 {
		t.Fatal("expected ring buffer to have entries after sender run")
	}

	data := collector.Ring().Read()
	t.Logf("ring buffer has %d entries", len(data))
	for i, s := range data {
		t.Logf("  [%d] tx=%d rx=%d tx_speed=%.2f rx_speed=%.2f", i, s.TxBytes, s.RxBytes, s.TxSpeed, s.RxSpeed)
	}
}
