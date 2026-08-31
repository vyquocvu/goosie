package js_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/js"
)

// TestFrameScheduler_BasicTick verifies the happy path: register, tick,
// callback fires once with a non-zero timestamp.
func TestFrameScheduler_BasicTick(t *testing.T) {
	s := js.NewFrameScheduler(0)
	var called int
	var ts time.Time
	s.RequestAnimationFrame(func(t time.Time) {
		called++
		ts = t
	})
	assert.Equal(t, 1, s.Pending())
	assert.True(t, s.Tick(), "Tick should report a callback ran")
	assert.Equal(t, 1, called)
	assert.False(t, ts.IsZero(), "callback timestamp should be populated")
	assert.Equal(t, 0, s.Pending(), "callback removed after firing")
}

// TestFrameScheduler_MultipleCallbacksFireInOrder verifies that registered
// callbacks run in FIFO order and all see the same Tick batch.
func TestFrameScheduler_MultipleCallbacksFireInOrder(t *testing.T) {
	s := js.NewFrameScheduler(0)
	var order []int
	for i := 0; i < 5; i++ {
		i := i
		s.RequestAnimationFrame(func(_ time.Time) { order = append(order, i) })
	}
	s.Tick()
	assert.Equal(t, []int{0, 1, 2, 3, 4}, order)
	assert.Equal(t, int64(1), s.FiredFrames())
}

// TestFrameScheduler_CancelBeforeTick verifies that CancelAnimationFrame
// before a Tick suppresses the callback without firing it.
func TestFrameScheduler_CancelBeforeTick(t *testing.T) {
	s := js.NewFrameScheduler(0)
	var called int
	id := s.RequestAnimationFrame(func(_ time.Time) { called++ })
	s.CancelAnimationFrame(id)
	assert.Equal(t, 0, s.Pending(), "cancelled callback should not appear pending")
	s.Tick()
	assert.Zero(t, called, "cancelled callback must not fire")
	assert.Equal(t, int64(1), s.Cancelled())
}

// TestFrameScheduler_CancelAfterTick verifies that cancelling after a Tick
// is a no-op (the callback already fired and was removed from the set).
func TestFrameScheduler_CancelAfterTick(t *testing.T) {
	s := js.NewFrameScheduler(0)
	var called int
	id := s.RequestAnimationFrame(func(_ time.Time) { called++ })
	s.Tick()
	assert.Equal(t, 1, called)
	s.CancelAnimationFrame(id) // no-op
	assert.Equal(t, 0, s.Pending())
}

// TestFrameScheduler_RecursionDeferredToNextTick guards the freeze fix:
// a callback that re-registers inside Tick must not run synchronously.
// The recursive request must defer to the next frame; otherwise a long
// animation loop would become unbounded stack recursion, which is the
// exact "stuck app" symptom the freeze fix is meant to address.
func TestFrameScheduler_RecursionDeferredToNextTick(t *testing.T) {
	s := js.NewFrameScheduler(0)
	var ticks int32
	// Outer re-registers an inner callback on every fire, but the inner
	// itself never re-registers. The chain is therefore:
	//   tick1: outer → count=1, queues inner
	//   tick2: inner → count=2, nothing queued
	//   tick3: no work
	s.RequestAnimationFrame(func(_ time.Time) {
		atomic.AddInt32(&ticks, 1)
		s.RequestAnimationFrame(func(_ time.Time) {
			atomic.AddInt32(&ticks, 1)
		})
	})

	// Critical assertion: the inner registration from inside the outer
	// callback must NOT have fired by the time Tick returns. Otherwise
	// animation loops become synchronous stack recursion.
	s.Tick()
	assert.Equal(t, int32(1), atomic.LoadInt32(&ticks), "first Tick must fire only the outer; inner must be deferred")

	s.Tick()
	assert.Equal(t, int32(2), atomic.LoadInt32(&ticks), "second Tick fires the inner")

	s.Tick()
	assert.Equal(t, int32(2), atomic.LoadInt32(&ticks), "no further work")
}

// TestFrameScheduler_ConcurrentRegister verifies that the public API is
// goroutine-safe (the freeze fix must not introduce a new data race).
func TestFrameScheduler_ConcurrentRegister(t *testing.T) {
	s := js.NewFrameScheduler(0)
	const N = 200
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.RequestAnimationFrame(func(_ time.Time) {})
		}()
	}
	wg.Wait()
	assert.Equal(t, N, s.Pending())
	s.Tick()
	assert.Equal(t, 0, s.Pending())
}

// TestFrameScheduler_DeterministicClock ensures the timestamp supplied to
// callbacks matches the clock that was set, so tests can rely on it.
func TestFrameScheduler_DeterministicClock(t *testing.T) {
	s := js.NewFrameScheduler(0)
	fixed := time.Unix(1700000000, 0)
	s.SetClock(func() time.Time { return fixed })
	var observed time.Time
	s.RequestAnimationFrame(func(ts time.Time) { observed = ts })
	s.Tick()
	assert.True(t, observed.Equal(fixed))
}

// TestFrameScheduler_Reset verifies the navigation-time reset path.
func TestFrameScheduler_Reset(t *testing.T) {
	s := js.NewFrameScheduler(0)
	for i := 0; i < 3; i++ {
		s.RequestAnimationFrame(func(_ time.Time) {})
	}
	s.Tick()
	assert.Equal(t, int64(1), s.FiredFrames())
	s.Reset()
	assert.Equal(t, 0, s.Pending())
	assert.Equal(t, int64(0), s.FiredFrames())
}

// TestFrameScheduler_IntervalDefault verifies the default 60Hz budget.
func TestFrameScheduler_IntervalDefault(t *testing.T) {
	s := js.NewFrameScheduler(0)
	assert.Equal(t, time.Second/60, s.Interval())
	s2 := js.NewFrameScheduler(33 * time.Millisecond)
	assert.Equal(t, 33*time.Millisecond, s2.Interval())
}

// TestFrameScheduler_TickNoWorkNoFire verifies the boolean contract: Tick
// returns false when there's no work. Owners can skip canvas refreshes.
func TestFrameScheduler_TickNoWorkNoFire(t *testing.T) {
	s := js.NewFrameScheduler(0)
	assert.False(t, s.Tick())
	s.RequestAnimationFrame(func(_ time.Time) {})
	assert.True(t, s.Tick())
	assert.False(t, s.Tick())
}
