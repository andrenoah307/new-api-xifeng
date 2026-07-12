package service

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// moderationIncidentBatcher amortises moderation_incidents writes so the
// OpenAI worker pool is decoupled from PG write latency. Per DEV_GUIDE
// §14 the batcher flushes whichever comes first: maxBatch rows accumulated
// or flushAt time elapsed since the last flush.
//
// Crash safety: the inbound channel is buffered, but rows that have not
// yet been flushed are lost if the process exits abnormally. Persistence
// of the underlying event lives at a different layer (Redis queue) so an
// unflushed batch only loses *audit metadata*, not the moderation
// decision itself — the user-facing block/auto-ban already happened in
// the worker before the row was queued for batcher.
type moderationIncidentBatcher struct {
	in       chan *model.ModerationIncident
	stopCh   chan struct{}
	doneCh   chan struct{}
	mu       sync.Mutex
	pending  int
	lastFlushAt time.Time
	lastFlushSize int
	totalFlushed int64
}

func newModerationIncidentBatcher(bufferSize int) *moderationIncidentBatcher {
	if bufferSize <= 0 {
		bufferSize = 1024
	}
	return &moderationIncidentBatcher{
		in:     make(chan *model.ModerationIncident, bufferSize),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// submit is non-blocking. When the inbound buffer is full we fall back to
// a synchronous insert — under sustained pressure that's the safe path:
// drop into the slow lane rather than discard the row outright.
func (b *moderationIncidentBatcher) submit(row *model.ModerationIncident) {
	if row == nil {
		return
	}
	select {
	case b.in <- row:
	default:
		// Buffer full: synchronous fallback so we don't lose audit data
		// during a write spike. Logged so admins notice the pressure.
		if err := model.CreateModerationIncident(row); err != nil {
			common.SysError("moderation incident sync fallback failed: " + err.Error())
		}
	}
}

func (b *moderationIncidentBatcher) start() {
	go b.run()
}

func (b *moderationIncidentBatcher) stop() {
	close(b.stopCh)
	<-b.doneCh
}

func (b *moderationIncidentBatcher) run() {
	defer close(b.doneCh)
	flushInterval := 500 * time.Millisecond
	maxBatch := 100
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	pending := make([]*model.ModerationIncident, 0, maxBatch)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		b.flushBatch(pending)
		pending = pending[:0]
	}

	for {
		select {
		case row, ok := <-b.in:
			if !ok {
				flush()
				return
			}
			pending = append(pending, row)
			if len(pending) >= maxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-b.stopCh:
			// drain remaining queued rows before exiting so SIGTERM
			// shutdown doesn't lose audit data.
			for {
				select {
				case row := <-b.in:
					pending = append(pending, row)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (b *moderationIncidentBatcher) flushBatch(rows []*model.ModerationIncident) {
	if len(rows) == 0 {
		return
	}
	now := common.GetTimestamp()
	for _, r := range rows {
		if r.CreatedAt == 0 {
			r.CreatedAt = now
		}
	}
	if err := model.DB.Session(&gorm.Session{}).CreateInBatches(rows, len(rows)).Error; err != nil {
		common.SysError("moderation incident batch insert failed: " + err.Error())
		// Fallback: try one-by-one so we don't drop the whole batch on a
		// single bad row. CreateModerationIncident logs its own error.
		for _, r := range rows {
			_ = model.CreateModerationIncident(r)
		}
	}
	b.mu.Lock()
	b.lastFlushAt = time.Now()
	b.lastFlushSize = len(rows)
	b.totalFlushed += int64(len(rows))
	b.mu.Unlock()
}

// stats exposes batcher state for the UI queue_stats endpoint.
func (b *moderationIncidentBatcher) stats() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	return map[string]any{
		"pending":         len(b.in),
		"last_flush_at":   b.lastFlushAt.Unix(),
		"last_flush_size": b.lastFlushSize,
		"total_flushed":   b.totalFlushed,
	}
}
