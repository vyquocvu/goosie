package renderer

import (
	"sync"
	"sync/atomic"
	"time"
)

// ImageLoadBatcher coalesces image-loaded callbacks into one flush per
// window. A page with 100 <img> tags can fire 100 completion callbacks in
// a burst; without batching each one triggers a full style + layout
// recompute and canvas refresh. The batcher collapses the burst into a
// single flush — one render request — and reports how many callbacks were
// collapsed for the metrics HUD.
//
// Window semantics: the first Signal after an idle period arms a timer;
// signals arriving inside the window accumulate in a pending set (duplicate
// sources collapse to one entry); when the timer fires, the whole set is
// handed to the flush callback in one call. A window of 0 fires immediately
// on every signal (no batching).
type ImageLoadBatcher struct {
	mu      sync.Mutex
	pending map[string]struct{}
	window  time.Duration
	flush   func([]string)
	timer   *time.Timer
	stopped bool

	// batches is the number of flush calls performed; signals is the
	// number of accepted Signal calls. dropped (signals − batches) is
	// the number of callbacks collapsed into batches.
	batches atomic.Uint64
	signals atomic.Uint64
}

// NewImageLoadBatcher creates a batcher that fires flush at most once per
// window with all image sources that completed in that window. A window of
// 0 means "flush immediately on every signal" (no batching). The flush
// callback runs on its own goroutine.
func NewImageLoadBatcher(window time.Duration, flush func([]string)) *ImageLoadBatcher {
	if window < 0 {
		window = 0
	}
	return &ImageLoadBatcher{
		pending: make(map[string]struct{}),
		window:  window,
		flush:   flush,
	}
}

// Signal records an image completion. If a timer is already armed the
// source joins the pending batch; otherwise a new window starts. Safe to
// call from any goroutine (image load completions arrive on loader
// goroutines).
func (b *ImageLoadBatcher) Signal(src string) {
	if src == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped || b.flush == nil {
		return
	}
	b.signals.Add(1)
	if _, dup := b.pending[src]; !dup {
		b.pending[src] = struct{}{}
	}
	if b.window == 0 {
		// Flush immediately on a separate goroutine to avoid
		// recursive locking if the flush callback re-enters Signal.
		b.flushLocked()
		return
	}
	if b.timer != nil {
		return // already armed; the fire will pick up this source
	}
	b.timer = time.AfterFunc(b.window, b.fire)
}

// Flush drains the pending batch immediately (used by tests and on
// shutdown so no completed images are left un-presented).
func (b *ImageLoadBatcher) Flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flushLocked()
}

// flushLocked drains the pending set into the flush callback. Callers must
// hold b.mu. The callback runs on a separate goroutine so re-entrant
// Signal calls cannot deadlock.
func (b *ImageLoadBatcher) flushLocked() {
	if b.stopped || b.flush == nil || len(b.pending) == 0 {
		return
	}
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	srcs := make([]string, 0, len(b.pending))
	for src := range b.pending {
		srcs = append(srcs, src)
	}
	b.pending = make(map[string]struct{})
	b.batches.Add(1)
	go b.flush(srcs)
}

// fire is the timer's callback: captures and resets the pending set, then
// runs the flush outside the lock.
func (b *ImageLoadBatcher) fire() {
	b.mu.Lock()
	b.flushLocked()
	b.mu.Unlock()
}

// Close stops the batcher: pending work is flushed once, and further
// signals are rejected. The flush runs on its own goroutine, so callers
// must not rely on it being complete when Close returns.
func (b *ImageLoadBatcher) Close() {
	b.mu.Lock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	// Drain pending before marking stopped, so Close flushes the final
	// batch instead of discarding it.
	b.flushLocked()
	b.stopped = true
	b.mu.Unlock()
}

// Metrics returns the number of flush batches performed and the number of
// signals collapsed into those batches (signals − batches).
func (b *ImageLoadBatcher) Metrics() (batches, dropped uint64) {
	return b.batches.Load(), b.signals.Load() - b.batches.Load()
}
