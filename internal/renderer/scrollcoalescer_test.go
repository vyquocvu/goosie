package renderer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestScrollCoalescer_CollapseAndClaim(t *testing.T) {
	c := NewScrollCoalescer()
	// No pending claim yet.
	v, ok := c.TryClaim()
	assert.False(t, ok)
	assert.False(t, c.Pending())

	// First schedule opens one pending frame.
	assert.True(t, c.Schedule(ScrollViewport{Y: 0, Height: 600}))
	assert.True(t, c.Pending())
	v, ok = c.TryClaim()
	assert.True(t, ok)
	assert.Equal(t, float32(0), v.Y)
	assert.Equal(t, float32(600), v.Height)

	// After claim, no pending.
	_, ok = c.TryClaim()
	assert.False(t, ok)
}

func TestScrollCoalescer_LastViewportWins(t *testing.T) {
	c := NewScrollCoalescer()
	assert.True(t, c.Schedule(ScrollViewport{Y: 0, Height: 600}))
	assert.False(t, c.Schedule(ScrollViewport{Y: 10, Height: 600}))
	assert.False(t, c.Schedule(ScrollViewport{Y: 20, Height: 600}))

	v, ok := c.TryClaim()
	assert.True(t, ok)
	assert.Equal(t, float32(20), v.Y, "the last-scheduled viewport must win")
}

func TestScrollCoalescer_CountsCoalesced(t *testing.T) {
	c := NewScrollCoalescer()
	c.Schedule(ScrollViewport{Y: 0, Height: 600})
	// The next four are coalesced; only the first Schedule increments.
	c.Schedule(ScrollViewport{Y: 1, Height: 600})
	c.Schedule(ScrollViewport{Y: 2, Height: 600})
	c.Schedule(ScrollViewport{Y: 3, Height: 600})
	c.Schedule(ScrollViewport{Y: 4, Height: 600})
	c.Schedule(ScrollViewport{Y: 5, Height: 600})

	// 5 follow-up schedules after the first: that's 5 coalesced.
	assert.Equal(t, 5, c.Coalesced())
	assert.Zero(t, c.Coalesced(), "coalesced counter is read-once")
}

func TestScrollCoalescer_ConcurrentSchedule(t *testing.T) {
	c := NewScrollCoalescer()
	const N = 200
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Schedule(ScrollViewport{Y: 1, Height: 600})
		}()
	}
	wg.Wait()
	// After the dust settles exactly one render is pending.
	v, ok := c.TryClaim()
	assert.True(t, ok)
	assert.Equal(t, float32(1), v.Y)
}

func TestFrameThrottler_AllowsFirstSignalImmediately(t *testing.T) {
	t0 := time.Unix(0, 0)
	th := NewFrameThrottler(16 * time.Millisecond)
	th.SetClock(func() time.Time { return t0 })
	assert.True(t, th.Signal(), "first signal of a frame should be allowed")
	// TryClaim consumes the open slot.
	assert.True(t, th.TryClaim(), "claim after open signal should succeed")
	// Subsequent claims are denied until the next signal opens the gate.
	assert.False(t, th.TryClaim(), "second claim in the same frame should be denied")
}

func TestFrameThrottler_DropsSignalsWithinInterval(t *testing.T) {
	t0 := time.Unix(0, 0)
	now := t0
	th := NewFrameThrottler(16 * time.Millisecond)
	th.SetClock(func() time.Time { return now })

	assert.True(t, th.Signal())
	assert.False(t, th.Signal(), "second signal in the same frame must drop")
	assert.False(t, th.Signal())
	assert.Equal(t, 2, th.Waiting())
	th.ResetWait()

	// Advance past the frame interval: next signal must succeed.
	now = t0.Add(20 * time.Millisecond)
	assert.True(t, th.Signal())
}

func TestFrameThrottler_ConcurrentSignals(t *testing.T) {
	th := NewFrameThrottler(16 * time.Millisecond)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if th.Signal() {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	// Only the first signal can possibly succeed; the rest fall into
	// the same frame. Without a clock advance, allowed == 1.
	assert.GreaterOrEqual(t, allowed.Load(), int64(1))
}
