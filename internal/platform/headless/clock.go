package headless

import (
	"sync"
	"time"
)

// Clock is the only time source a headless window has. Vsync pacing is the thing
// automated runs measure, and a measurement of a wall clock is a measurement of
// whatever else the machine was doing, so the window takes its ticks from here rather
// than from time.After directly.
type Clock interface {
	// After returns a channel that receives once, d after the call, with the time the
	// tick was due. One-shot semantics match time.After, which is what lets the window's
	// pacing loop re-arm after each tick rather than manage a ticker's drift.
	After(d time.Duration) <-chan time.Time
	// Now reports the current time.
	Now() time.Time
}

// SystemClock is the real clock. It is the default so that a headless run which wants
// to be paced by wall time rather than by a test gets exactly that.
type SystemClock struct{}

// After implements Clock.
func (SystemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Now implements Clock.
func (SystemClock) Now() time.Time { return time.Now() }

// ManualClock is a clock a test drives. Advancing it is the only way time passes, so a
// pacing assertion can say "sixteen milliseconds" and mean it.
type ManualClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*manualTimer
}

// manualTimer is one armed After call. The channel is buffered by one so Advance never
// blocks on a receiver that has not come back around yet.
type manualTimer struct {
	at time.Time
	ch chan time.Time
}

// NewManualClock returns a clock whose time starts at the given instant. A fixed start
// is worth choosing over time.Now(): a timestamp that appears in a failure message can
// then be read as an offset rather than decoded.
func NewManualClock(start time.Time) *ManualClock {
	return &ManualClock{now: start}
}

// Now implements Clock.
func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After implements Clock. The timer stays armed until Advance passes its deadline,
// which is the whole point: nothing fires on its own.
func (c *ManualClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d < 0 {
		d = 0
	}
	ch := make(chan time.Time, 1)
	c.timers = append(c.timers, &manualTimer{at: c.now.Add(d), ch: ch})
	return ch
}

// Pending reports how many armed timers have not fired. A driver needs it to know that
// the window has asked for its next tick before advancing: a loop re-arms only after
// taking the previous one, so a test that advanced blindly could report a tick that was
// never scheduled.
func (c *ManualClock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// Advance moves the clock d into the future and fires every timer whose deadline has
// come, in the order they were armed, returning how many fired.
//
// It fires nothing when d is too small, which is the half of the pacing contract worth
// testing: a clock that always fired the next timer whatever d was could not tell a
// 16ms vsync from a busy loop.
func (c *ManualClock) Advance(d time.Duration) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	fired := 0
	keep := c.timers[:0]
	for _, t := range c.timers {
		if t.at.After(c.now) {
			keep = append(keep, t)
			continue
		}
		t.ch <- t.at
		fired++
	}
	c.timers = keep
	return fired
}
