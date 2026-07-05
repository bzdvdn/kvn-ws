package client

import (
	"context"
	"time"
)

// @sk-task kvn-web-redesign#T1.1: Sender periodically computes speed and pushes snapshots to channel (AC-013)
type Sender struct {
	collector *Collector
	out       chan MetricSnapshot
	interval  time.Duration
}

func NewSender(collector *Collector, out chan MetricSnapshot, interval time.Duration) *Sender {
	return &Sender{
		collector: collector,
		out:       out,
		interval:  interval,
	}
}

func (s *Sender) Run(ctx context.Context) {
	var prev MetricSnapshot
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cur := s.collector.Snapshot()
			elapsed := s.interval.Seconds()
			if elapsed > 0 {
				cur.TxSpeed = float64(cur.TxBytes-prev.TxBytes) * 8 / (elapsed * 1_000_000)
				cur.RxSpeed = float64(cur.RxBytes-prev.RxBytes) * 8 / (elapsed * 1_000_000)
			}
			s.collector.Ring().Push(cur)
			prev = cur

			select {
			case s.out <- cur:
			default:
			}
		}
	}
}
