package headless

import (
	"sync"
	"time"
)

type Clock interface {
	After(d time.Duration) <-chan time.Time
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

func (SystemClock) Now() time.Time { return time.Now() }

type ManualClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*manualTimer
}

type manualTimer struct {
	at time.Time
	ch chan time.Time
}

func NewManualClock(start time.Time) *ManualClock {
	return &ManualClock{now: start}
}

func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

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

func (c *ManualClock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

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
