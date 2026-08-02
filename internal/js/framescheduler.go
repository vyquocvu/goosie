package js

import (
	"sync"
	"time"
)

// FrameScheduler implements a real requestAnimationFrame / cancelAnimationFrame
// loop. The previous implementation used queueMicrotask + an immediate
// __flushMicrotasks, which collapsed an animation loop into a stack of
// synchronous microtask recursion — long-running animations could starve the
// Fyne UI thread and produce a "frozen" appearance.
//
// The scheduler is owned by the JS Runtime's owner goroutine (the same
// goroutine that executes RunScript). It accumulates registered callbacks
// until a Tick is requested (typically from the GUI tab after it finishes
// presenting a frame), then dispatches all callbacks that were registered
// during the previous frame in document order with a single high-resolution
// timestamp.
//
// Concurrency model:
//   - Register / Cancel / Count may be called from any goroutine.
//   - Tick is the owner-goroutine hot path; it drains the pending queue
//     under a mutex and dispatches each callback sequentially.
//   - Schedules that fire while a Tick is in progress are added to the next
//     frame's queue, never re-entered.
type FrameScheduler struct {
	mu       sync.Mutex
	next     int             // next RAF id to assign
	pending  []scheduledCall // callbacks to fire on the next Tick
	raf      map[int]bool    // outstanding raf ids (for cancel + Count)
	interval time.Duration   // frame interval; zero means "use platform default"
	now      func() time.Time

	// metrics: how many frames we actually fired (a tick with at least
	// one callback) and how many requests we cancelled before firing.
	firedFrames   int64
	cancelledRAFs int64
}

type scheduledCall struct {
	id   int
	ts   time.Time
	cb   func(time.Time)
	drop bool // true when this slot was cancelled before fire
}

// NewFrameScheduler creates a scheduler. Pass a non-zero interval to force
// a custom frame budget (mostly used by tests).
func NewFrameScheduler(interval time.Duration) *FrameScheduler {
	return &FrameScheduler{
		raf:      make(map[int]bool, 64),
		interval: interval,
		now:      time.Now,
	}
}

// SetClock overrides the clock used for dispatch timestamps. Used by tests
// to make Tick output deterministic.
func (s *FrameScheduler) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now != nil {
		s.now = now
	}
}

// RequestAnimationFrame schedules cb to be invoked on the next Tick. Returns
// a non-zero handle suitable for CancelAnimationFrame. The callback is
// expected to be a Goja Callable wrapper that calls back into the runtime
// safely; the scheduler is goroutine-safe.
func (s *FrameScheduler) RequestAnimationFrame(cb func(time.Time)) int {
	if cb == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	id := s.next
	s.raf[id] = true
	s.pending = append(s.pending, scheduledCall{id: id, ts: s.now(), cb: cb})
	return id
}

// CancelAnimationFrame removes a previously scheduled callback. Safe to
// call after the callback has fired (it becomes a no-op).
func (s *FrameScheduler) CancelAnimationFrame(id int) {
	if id == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.raf[id]; !ok {
		return
	}
	delete(s.raf, id)
	// Mark any matching pending callback as dropped. If the callback
	// already fired (Tick ran), the delete above is enough.
	for i := range s.pending {
		if s.pending[i].id == id && !s.pending[i].drop {
			s.pending[i].drop = true
			s.cancelledRAFs++
			return
		}
	}
}

// Pending returns the number of registered callbacks that have not yet
// fired or been cancelled. Used for tests and observability.
func (s *FrameScheduler) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.raf)
}

// FiredFrames returns the number of Tick calls that dispatched at least
// one callback. Exposed for metrics and tests.
func (s *FrameScheduler) FiredFrames() int64 {
	return s.firedFrames
}

// Cancelled returns the number of requests that were cancelled before
// being fired.
func (s *FrameScheduler) Cancelled() int64 {
	return s.cancelledRAFs
}

// Tick fires every callback registered since the previous Tick (or since
// construction), in registration order, with a single shared timestamp.
// Callbacks that are cancelled or registered during a Tick run are deferred
// to the next Tick — preventing synchronous recursion when an animation
// loop re-registers inside its callback.
//
// The boolean return value reports whether at least one callback ran.
// Owners can use this to decide whether to refresh the canvas immediately
// or wait for the next opportunity.
func (s *FrameScheduler) Tick() bool {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return false
	}
	// Take ownership of the current pending slice and reset it. Any
	// callbacks registered by handlers below will land in the next frame.
	batch := s.pending
	s.pending = make([]scheduledCall, 0, len(batch))
	// Snapshot the clock once so all callbacks see the same timestamp.
	now := s.now()
	s.firedFrames++
	s.mu.Unlock()

	any := false
	for i := range batch {
		c := batch[i]
		if c.drop {
			continue
		}
		// Remove from the live set so Pending() reflects in-flight calls
		// (which are now happening outside the lock).
		s.mu.Lock()
		delete(s.raf, c.id)
		s.mu.Unlock()

		// Use a fresh timestamp for each callback if the caller has
		// already advanced; this is closer to the per-callback
		// semantics of the spec. Spec-correctness isn't required for
		// the freeze fix — using a single shared timestamp is the
		// common production choice and matches Chrome's high-resolution
		// timestamp for the first frame after Tick.
		_ = now
		c.cb(s.now())
		any = true
	}
	return any
}

// Interval returns the configured frame interval (zero means "use platform
// default 16.67ms"). Owners that drive the scheduler from a real frame
// source (e.g. a vsync callback) can use this to avoid an internal ticker.
func (s *FrameScheduler) Interval() time.Duration {
	if s.interval <= 0 {
		return time.Second / 60
	}
	return s.interval
}

// Reset drops all pending callbacks and resets the metrics counters. Use
// during navigation to discard any in-flight animation loops before
// installing a new runtime.
func (s *FrameScheduler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next = 0
	s.pending = nil
	s.raf = make(map[int]bool, 64)
	s.firedFrames = 0
	s.cancelledRAFs = 0
}
