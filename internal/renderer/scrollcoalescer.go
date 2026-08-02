package renderer

import (
	"sync"
	"sync/atomic"
	"time"
)

// ScrollCoalescer collapses a burst of scroll events into a single
// scheduled render. It guarantees:
//   - At most one pending render is queued at any time.
//   - The latest viewport (y, height) is always the one rendered.
//   - Coalesced events are counted for the FrameMetrics HUD.
//
// Without coalescing, every OnScrolled tick walks the full display
// list, builds the Fyne object tree, and triggers a refresh — even
// when the user is rapidly scrolling. On a real page this serializes
// behind the Fyne main thread and degrades to single-digit FPS.
//
// The coalescer is intentionally passive: it does not own a timer.
// The owning canvas (or tab) calls Schedule from its OnScrolled
// handler and Run from its Fyne-thread tick. This keeps the
// scheduler and the Fyne presentation logic in the same hands.
type ScrollCoalescer struct {
	mu       sync.Mutex
	viewport ScrollViewport
	hasView  bool
	pending  bool

	// Coalesced counts how many Schedule calls were collapsed into
	// the last Run. Surfaced to FrameMetrics via IncCoalescedScroll.
	coalesced atomic.Int64
}

// ScrollViewport captures the bits of viewport state that affect
// scroll rendering. Kept narrow so callers can pass the values
// directly from the Fyne scroll callback.
type ScrollViewport struct {
	Y      float32
	Height float32
}

// NewScrollCoalescer creates an empty coalescer.
func NewScrollCoalescer() *ScrollCoalescer {
	return &ScrollCoalescer{}
}

// Schedule records a new viewport and ensures the next Run call will
// process it. Multiple Schedule calls before Run increment the
// coalesced counter but only retain the last viewport.
func (c *ScrollCoalescer) Schedule(v ScrollViewport) {
	c.mu.Lock()
	c.viewport = v
	c.hasView = true
	if c.pending {
		c.coalesced.Add(1)
	} else {
		c.pending = true
	}
	c.mu.Unlock()
}

// TryClaim returns the current viewport and resets the pending flag,
// so the caller can perform the render. The boolean return tells the
// caller whether a viewport was actually pending (i.e. a render
// should run). If two callers race, only one wins the claim.
func (c *ScrollCoalescer) TryClaim() (ScrollViewport, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pending || !c.hasView {
		return ScrollViewport{}, false
	}
	v := c.viewport
	c.pending = false
	return v, true
}

// Coalesced returns how many scroll events were collapsed into the
// last successful claim. Used to feed FrameMetrics.
func (c *ScrollCoalescer) Coalesced() int {
	// Reset the counter on read so per-frame accumulation is correct.
	n := c.coalesced.Swap(0)
	return int(n)
}

// Pending reports whether a viewport is currently queued.
func (c *ScrollCoalescer) Pending() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pending
}

// FrameThrottler lets a renderer bound how often it accepts work to
// once per frame. Unlike a coalescer, it accumulates inputs (rather
// than discarding them) but only releases a signal at most once per
// frame interval. The renderer drains queued work in bulk when the
// throttle opens.
//
// The freeze-fix use case is DOM mutations: the JS runtime may fire
// 1000 mutations/sec from a timer, but the renderer can only paint
// at 60Hz. FrameThrottler ensures we render the latest snapshot once
// per frame instead of either re-rendering 16 times or starving
// continuous updates behind a trailing debounce.
type FrameThrottler struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
	now      func() time.Time

	open    atomic.Bool // true when a render is allowed this frame
	waiting atomic.Int64
}

// NewFrameThrottler creates a throttler with the supplied interval.
// A non-positive interval is treated as 16.67ms (60Hz).
func NewFrameThrottler(interval time.Duration) *FrameThrottler {
	if interval <= 0 {
		interval = time.Second / 60
	}
	return &FrameThrottler{
		interval: interval,
		now:      time.Now,
		open:     atomic.Bool{},
	}
}

// SetClock overrides the throttler's clock (for tests).
func (t *FrameThrottler) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	t.mu.Lock()
	t.now = now
	t.mu.Unlock()
}

// Signal records that work is available. If the throttle is closed,
// the work is remembered but no immediate release is granted.
// Returns true if a render is permitted this call.
func (t *FrameThrottler) Signal() bool {
	t.mu.Lock()
	now := t.now()
	if t.last.IsZero() || now.Sub(t.last) >= t.interval {
		t.last = now
		t.open.Store(true)
		t.mu.Unlock()
		return true
	}
	t.waiting.Add(1)
	t.mu.Unlock()
	return false
}

// TryClaim returns whether a render is allowed and resets the open
// flag. Owners should call this once per frame at most.
func (t *FrameThrottler) TryClaim() bool {
	if t.open.CompareAndSwap(true, false) {
		return true
	}
	return false
}

// Waiting reports how many Signal calls were dropped because the
// throttle was closed.
func (t *FrameThrottler) Waiting() int {
	return int(t.waiting.Load())
}

// ResetWait clears the dropped-work counter; the throttle interval
// and last-time are preserved.
func (t *FrameThrottler) ResetWait() {
	t.waiting.Store(0)
}
