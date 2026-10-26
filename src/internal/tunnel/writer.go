package tunnel

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

// @sk-task ws-media-queue-backpressure#T1.1: bounded per-channel writers (AC-001, AC-002, AC-003, AC-004)
const (
	secondaryQueueCapacity  = 512
	queueBatchMaxBytes      = 4096
	secondaryQueueMaxAge    = 100 * time.Millisecond
	secondaryWriteTimeout   = time.Second
	defaultSecondaryBatchMs = 2
	maxSecondaryBatchMs     = 10
)

// @sk-task ws-media-queue-backpressure#T1.1: frame queued for a channel writer
type queuedFrame struct {
	data     []byte
	enqueued time.Time
	gen      uint64
}

// @sk-task ws-media-queue-backpressure#T1.2: per-queue counters exposed via SessionStats
type queueStats struct {
	enqueued        atomic.Uint64
	writes          atomic.Uint64
	droppedFull     atomic.Uint64
	droppedStaleGen atomic.Uint64
	droppedStaleAge atomic.Uint64
	droppedExit     atomic.Uint64
	cleared         atomic.Uint64
	writeErrors     atomic.Uint64
	writeNanos      atomic.Uint64
	maxWriteNanos   atomic.Uint64
	maxDepth        atomic.Int64
}

// @sk-task ws-media-queue-backpressure#T1.2: immutable snapshot for tests and logs
type QueueStatsSnapshot struct {
	Enqueued        uint64
	Writes          uint64
	DroppedFull     uint64
	DroppedStaleGen uint64
	DroppedStaleAge uint64
	DroppedExit     uint64
	Cleared         uint64
	WriteErrors     uint64
	WriteNanos      uint64
	MaxWriteNanos   uint64
	MaxDepth        int64
}

// @sk-task ws-media-queue-backpressure#T1.2: session-wide snapshot
type SessionStatsSnapshot struct {
	Secondary     QueueStatsSnapshot
	FramingErrors uint64
}

// @sk-task ws-media-queue-backpressure#T1.2: read a consistent counter snapshot
func (s *queueStats) snapshot() QueueStatsSnapshot {
	if s == nil {
		return QueueStatsSnapshot{}
	}
	return QueueStatsSnapshot{
		Enqueued:        s.enqueued.Load(),
		Writes:          s.writes.Load(),
		DroppedFull:     s.droppedFull.Load(),
		DroppedStaleGen: s.droppedStaleGen.Load(),
		DroppedStaleAge: s.droppedStaleAge.Load(),
		DroppedExit:     s.droppedExit.Load(),
		Cleared:         s.cleared.Load(),
		WriteErrors:     s.writeErrors.Load(),
		WriteNanos:      s.writeNanos.Load(),
		MaxWriteNanos:   s.maxWriteNanos.Load(),
		MaxDepth:        s.maxDepth.Load(),
	}
}

// @sk-task ws-media-queue-backpressure#T1.1: bounded queue writer with optional
// batching, max age, generation filtering and non-blocking enqueue.
type queueWriter struct {
	name         string
	ch           chan queuedFrame
	batchMaxSize int
	maxAge       time.Duration
	writeTimeout time.Duration
	coalesce     func() bool
	target       func() StreamConn
	currentGen   func() uint64
	logger       *zap.Logger
	stats        *queueStats
	onError      func(error)
	holdNanos    atomic.Int64
	active       atomic.Bool
	lastDropLog  atomic.Int64
}

// @sk-task ws-media-queue-backpressure#T1.1: construct a bounded channel writer
func newQueueWriter(
	name string,
	capacity int,
	batchMaxSize int,
	maxAge time.Duration,
	writeTimeout time.Duration,
	coalesce func() bool,
	target func() StreamConn,
	currentGen func() uint64,
	logger *zap.Logger,
	stats *queueStats,
	onError func(error),
) *queueWriter {
	return &queueWriter{
		name:         name,
		ch:           make(chan queuedFrame, capacity),
		batchMaxSize: batchMaxSize,
		maxAge:       maxAge,
		writeTimeout: writeTimeout,
		coalesce:     coalesce,
		target:       target,
		currentGen:   currentGen,
		logger:       logger,
		stats:        stats,
		onError:      onError,
	}
}

// @sk-task ws-media-queue-backpressure#T2.2: configure batching window
func (w *queueWriter) setHold(hold time.Duration) {
	w.holdNanos.Store(int64(hold))
}

// @sk-task ws-media-queue-backpressure#T2.2: current batching window
func (w *queueWriter) hold() time.Duration {
	return time.Duration(w.holdNanos.Load())
}

// @sk-task ws-media-queue-backpressure#T1.1: whether frame coalescing is enabled
func (w *queueWriter) coalesces() bool {
	return w.coalesce != nil && w.coalesce()
}

// enqueue takes ownership of data. When the queue is full the packet is
// dropped and its buffer is returned to the framing pool.
// @sk-task ws-media-queue-backpressure#T1.1: non-blocking enqueue with drop accounting
func (w *queueWriter) enqueue(data []byte, gen uint64) bool {
	f := queuedFrame{data: data, enqueued: time.Now(), gen: gen}
	select {
	case w.ch <- f:
		w.stats.enqueued.Add(1)
		w.updateMaxDepth(len(w.ch))
		return true
	default:
		w.stats.droppedFull.Add(1)
		framing.ReturnBuffer(data)
		w.logDrop("full", 0)
		return false
	}
}

// @sk-task ws-media-queue-backpressure#T1.2: track queue high-watermark
func (w *queueWriter) updateMaxDepth(depth int) {
	for {
		old := w.stats.maxDepth.Load()
		if int64(depth) <= old || w.stats.maxDepth.CompareAndSwap(old, int64(depth)) {
			return
		}
	}
}

// clear drops everything currently queued. Frames enqueued concurrently with
// an old generation are filtered by the writer via currentGen.
// @sk-task ws-media-queue-backpressure#T2.1: drop queued frames on rebind/loss
func (w *queueWriter) clear() {
	for {
		select {
		case f := <-w.ch:
			framing.ReturnBuffer(f.data)
			w.stats.cleared.Add(1)
		default:
			return
		}
	}
}

// @sk-task ws-media-queue-backpressure#T1.1: start the writer once per lifecycle
func (w *queueWriter) ensure(ctx context.Context) {
	if w.active.CompareAndSwap(false, true) {
		go w.run(ctx)
	}
}

// @sk-task ws-media-queue-backpressure#T1.1: writer loop with batching/age/generation checks
func (w *queueWriter) run(ctx context.Context) {
	defer w.active.Store(false)
	defer w.dropQueued()

	hold := w.hold()
	var (
		batch      []byte
		batchFirst time.Time
		batchGen   uint64
		timer      *time.Timer
		timerCh    <-chan time.Time
	)

	resetBatch := func() {
		batch = batch[:0]
		batchFirst = time.Time{}
		batchGen = 0
		if timer != nil {
			timer.Stop()
			timer = nil
			timerCh = nil
		}
	}

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if w.currentGen != nil && batchGen != w.currentGen() {
			resetBatch()
			return nil
		}
		if w.maxAge > 0 && !batchFirst.IsZero() && time.Since(batchFirst) > w.maxAge {
			w.stats.droppedStaleAge.Add(1)
			w.logDrop("stale-age", len(batch))
			resetBatch()
			return nil
		}
		target := w.target()
		if target == nil {
			resetBatch()
			return nil
		}
		err := w.writeFrame(target, batch)
		resetBatch()
		return err
	}

	// drainCoalesce picks up frames already queued without waiting.
	drainCoalesce := func() {
		for len(batch) < w.batchMaxSize {
			select {
			case f := <-w.ch:
				if !w.accept(f) {
					continue
				}
				if len(batch) > 0 && w.currentGen != nil && f.gen != batchGen {
					resetBatch()
				}
				if len(batch) == 0 {
					batchFirst = f.enqueued
					batchGen = f.gen
				}
				batch = append(batch, f.data...)
				framing.ReturnBuffer(f.data)
			default:
				return
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-timerCh:
			if err := flush(); err != nil {
				w.logWriteError(err)
				w.reportError(err)
				return
			}
		case f := <-w.ch:
			if !w.accept(f) {
				continue
			}
			if !w.coalesces() {
				err := w.writeFrame(w.target(), f.data)
				framing.ReturnBuffer(f.data)
				if err != nil {
					w.logWriteError(err)
					w.reportError(err)
					return
				}
				continue
			}
			if len(batch) > 0 && w.currentGen != nil && f.gen != batchGen {
				resetBatch()
			}
			if len(batch) == 0 {
				batchFirst = f.enqueued
				batchGen = f.gen
			}
			batch = append(batch, f.data...)
			framing.ReturnBuffer(f.data)

			if len(batch) >= w.batchMaxSize {
				if err := flush(); err != nil {
					w.logWriteError(err)
					w.reportError(err)
					return
				}
				continue
			}
			if hold <= 0 {
				drainCoalesce()
				if err := flush(); err != nil {
					w.logWriteError(err)
					w.reportError(err)
					return
				}
				continue
			}
			if timer == nil {
				timer = time.NewTimer(hold)
				timerCh = timer.C
			}
		}
	}
}

// @sk-task ws-media-queue-backpressure#T1.1: reject stale/extinct frames before batching
func (w *queueWriter) accept(f queuedFrame) bool {
	if w.currentGen != nil && f.gen != w.currentGen() {
		w.stats.droppedStaleGen.Add(1)
		framing.ReturnBuffer(f.data)
		w.logDrop("stale-gen", 0)
		return false
	}
	if w.maxAge > 0 && !f.enqueued.IsZero() && time.Since(f.enqueued) > w.maxAge {
		w.stats.droppedStaleAge.Add(1)
		framing.ReturnBuffer(f.data)
		w.logDrop("stale-age", 0)
		return false
	}
	return true
}

// @sk-task ws-media-queue-backpressure#T1.1: deadline-safe channel write with timings
func (w *queueWriter) writeFrame(target StreamConn, data []byte) error {
	if target == nil {
		w.stats.droppedExit.Add(1)
		return nil
	}
	start := time.Now()
	err := transport.WriteMessageWithDeadline(target, data, start.Add(w.writeTimeout))
	elapsed := time.Since(start)
	if elapsed > 0 {
		w.stats.writeNanos.Add(uint64(elapsed))
		for {
			old := w.stats.maxWriteNanos.Load()
			if uint64(elapsed) <= old || w.stats.maxWriteNanos.CompareAndSwap(old, uint64(elapsed)) {
				break
			}
		}
	}
	w.stats.writes.Add(1)
	if err != nil {
		w.stats.writeErrors.Add(1)
		return err
	}
	return nil
}

// @sk-task ws-media-queue-backpressure#T1.1: return queued buffers on writer exit
func (w *queueWriter) dropQueued() {
	for {
		select {
		case f := <-w.ch:
			framing.ReturnBuffer(f.data)
			w.stats.droppedExit.Add(1)
		default:
			return
		}
	}
}

// @sk-task ws-media-queue-backpressure#T1.1: propagate fatal primary write errors
func (w *queueWriter) reportError(err error) {
	if w.onError != nil {
		w.onError(err)
	}
}

// @sk-task ws-media-queue-backpressure#T3.4: log a channel write failure
func (w *queueWriter) logWriteError(err error) {
	if w.logger == nil {
		return
	}
	w.logger.Warn(w.name+" writer failed, dropping queued packets",
		zap.Int("queued", len(w.ch)),
		zap.Uint64("write_errors", w.stats.writeErrors.Load()),
		zap.Error(err),
	)
}

// @sk-task ws-media-queue-backpressure#T3.4: rate-limited drop diagnostics
func (w *queueWriter) logDrop(reason string, bytes int) {
	if w.logger == nil {
		return
	}
	now := time.Now().UnixNano()
	last := w.lastDropLog.Load()
	if last != 0 && now-last < int64(time.Second) {
		return
	}
	if !w.lastDropLog.CompareAndSwap(last, now) {
		return
	}
	dropFields := []zap.Field{
		zap.String("reason", reason),
		zap.Int("queued", len(w.ch)),
		zap.Int("bytes", bytes),
		zap.Uint64("dropped_full", w.stats.droppedFull.Load()),
		zap.Uint64("dropped_stale_age", w.stats.droppedStaleAge.Load()),
		zap.Uint64("dropped_stale_gen", w.stats.droppedStaleGen.Load()),
	}
	w.logger.Warn(w.name+" queue drop", dropFields...)
}
