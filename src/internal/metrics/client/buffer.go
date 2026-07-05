package client

import (
	"sync"
	"time"
)

// @sk-task kvn-web-redesign#T1.1: MetricSnapshot type for client-side metrics (AC-014)
type MetricSnapshot struct {
	TxBytes    int64   `json:"tx_bytes"`
	RxBytes    int64   `json:"rx_bytes"`
	LatencyMs  float64 `json:"latency_ms"`
	UptimeS    int64   `json:"uptime_s"`
	TxSpeed    float64 `json:"tx_speed"`
	RxSpeed    float64 `json:"rx_speed"`
	Reconnects int64   `json:"reconnects"`
}

// @sk-task kvn-web-redesign#T1.1: RingBuffer for time-series metric data (AC-014)
type RingBuffer struct {
	mu    sync.Mutex
	data  []MetricSnapshot
	cap   int
	pos   int
	count int
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data: make([]MetricSnapshot, capacity),
		cap:  capacity,
	}
}

func (rb *RingBuffer) Push(s MetricSnapshot) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.data[rb.pos] = s
	rb.pos = (rb.pos + 1) % rb.cap
	if rb.count < rb.cap {
		rb.count++
	}
}

func (rb *RingBuffer) Read() []MetricSnapshot {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.count == 0 {
		return nil
	}
	start := (rb.pos - rb.count + rb.cap) % rb.cap
	out := make([]MetricSnapshot, rb.count)
	for i := 0; i < rb.count; i++ {
		out[i] = rb.data[(start+i)%rb.cap]
	}
	return out
}

func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

func (rb *RingBuffer) Cap() int {
	return rb.cap
}

// @sk-task kvn-web-redesign#T1.1: Collector gathers client-side metrics and pushes to channel (AC-013)
type Collector struct {
	rb         *RingBuffer
	startedAt  time.Time
	reconnects int64

	mu       sync.Mutex
	txBytes  int64
	rxBytes  int64
	latencyMs float64

	done chan struct{}
}

func NewCollector() *Collector {
	return &Collector{
		rb:    NewRingBuffer(60),
		done:  make(chan struct{}),
	}
}

func (c *Collector) Start() {
	c.mu.Lock()
	c.startedAt = time.Now()
	c.mu.Unlock()
}

func (c *Collector) Stop() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

func (c *Collector) Done() <-chan struct{} {
	return c.done
}

func (c *Collector) AddTX(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.txBytes += n
}

func (c *Collector) AddRX(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rxBytes += n
}

func (c *Collector) SetLatency(ms float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latencyMs = ms
}

func (c *Collector) IncReconnects() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reconnects++
}

func (c *Collector) Snapshot() MetricSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	uptime := int64(0)
	if !c.startedAt.IsZero() {
		uptime = int64(time.Since(c.startedAt).Seconds())
	}
	return MetricSnapshot{
		TxBytes:    c.txBytes,
		RxBytes:    c.rxBytes,
		LatencyMs:  c.latencyMs,
		UptimeS:    uptime,
		TxSpeed:    0,
		RxSpeed:    0,
		Reconnects: c.reconnects,
	}
}

func (c *Collector) Ring() *RingBuffer {
	return c.rb
}
