package documentloader

import (
	"sync"
	"time"
)

// MutationCoalescer collapses a burst of mutation notifications into a
// single debounced render. The plan for M6 of the resource pipeline
// calls for coalescing: when JS fires multiple DOM mutations in quick
// succession (typical for batched scripts), we wait for a quiet window
// before re-rendering, instead of paying the cost per mutation.
//
// MutationCoalescer is safe for concurrent use. Calls to Trigger and
// Flush may come from any goroutine. The supplied render callback runs
// on the goroutine that fires the timer; callers that need to marshal
// to a UI thread must do so inside the callback.
//
// Lifecycle:
//   - NewMutationCoalescer constructs with a window (e.g. 16ms).
//   - Trigger records a mutation and (re)starts the timer.
//   - Flush cancels the pending timer and runs the render synchronously
//     (useful at navigation end / tab close).
//   - Stop cancels the timer without firing.
//
// The render callback receives the count of mutations that arrived
// since the last fire, so the renderer can log "rendered after N
// coalesced mutations" if it cares.
type MutationCoalescer struct {
	window time.Duration
	render func(n int)
	mu     sync.Mutex
	timer  *time.Timer
	pending int
	stopped bool
}

// NewMutationCoalescer creates a coalescer that fires render at most
// once per window. A window of 0 means "fire immediately on every
// mutation" (no coalescing). Negative windows are clamped to 0.
func NewMutationCoalescer(window time.Duration, render func(n int)) *MutationCoalescer {
	if window < 0 {
		window = 0
	}
	return &MutationCoalescer{
		window: window,
		render: render,
	}
}

// Trigger records a mutation. If a timer is already armed, it is
// reset; otherwise a new timer is started. The render callback will
// fire after the configured window of inactivity, with the total
// mutation count since the last fire.
func (c *MutationCoalescer) Trigger() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.render == nil {
		return
	}
	c.pending++
	if c.window == 0 {
		// Fire immediately on a separate goroutine to avoid
		// recursive locking if the render callback re-enters Trigger.
		n := c.pending
		c.pending = 0
		go c.render(n)
		return
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timer = time.AfterFunc(c.window, c.fire)
}

// Flush fires the render immediately if there are pending mutations,
// and resets the timer. Use at navigation end or before discarding
// the coalescer to ensure the final render is not lost.
func (c *MutationCoalescer) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.render == nil || c.pending == 0 {
		return
	}
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	n := c.pending
	c.pending = 0
	go c.render(n)
}

// Stop cancels any pending timer. Trigger becomes a no-op until a
// future Trigger resets stopped (which it doesn't — Stop is final).
func (c *MutationCoalescer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

// fire is the timer's callback. Holds the lock briefly to capture
// and reset pending count, then runs the render outside the lock so
// re-entrant Trigger calls don't deadlock.
func (c *MutationCoalescer) fire() {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	n := c.pending
	c.pending = 0
	c.timer = nil
	c.mu.Unlock()
	if c.render != nil {
		c.render(n)
	}
}

// Pending returns the number of mutations received since the last
// fire. Useful for tests.
func (c *MutationCoalescer) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pending
}