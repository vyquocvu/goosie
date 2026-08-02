package renderer

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestFrameMetrics_EmptySnapshot verifies the zero state.
func TestFrameMetrics_EmptySnapshot(t *testing.T) {
	m := NewFrameMetrics()
	s := m.Snapshot()
	assert.Zero(t, s.Frames)
	assert.Zero(t, s.CurrentFPS)
	assert.Zero(t, s.RenderDuration)
}

// TestFrameMetrics_ObserveFrame verifies the basic observation path:
// a frame is recorded with its duration, and the snapshot reports
// the duration, FPS, and counter values.
func TestFrameMetrics_ObserveFrame(t *testing.T) {
	clock := newFakeClockForMetrics(time.Unix(0, 0))
	m := NewFrameMetricsWithClock(clock.now)
	m.SetTargetFPS(60)

	// 60 frames at 16.67ms intervals = 1s of simulated runtime.
	const frameDur = time.Second / 60
	clock.advance(frameDur)
	m.ObserveFrame(2 * time.Millisecond)
	for i := 1; i < 60; i++ {
		clock.advance(frameDur)
		m.ObserveFrame(3 * time.Millisecond)
	}

	s := m.Snapshot()
	assert.Equal(t, int64(60), s.Frames)
	assert.InDelta(t, 60.0, s.CurrentFPS, 0.5)
	assert.Equal(t, 3*time.Millisecond, s.RenderDuration)
	assert.Equal(t, 3*time.Millisecond, s.MaxRenderDuration)
	assert.Zero(t, s.Dropped)
}

// TestFrameMetrics_LongFrames verifies that frames above the long
// threshold are counted but not classified as "dropped" (which is a
// different concept — inter-frame gap).
func TestFrameMetrics_LongFrames(t *testing.T) {
	clock := newFakeClockForMetrics(time.Unix(0, 0))
	m := NewFrameMetricsWithClock(clock.now)
	m.SetLongThreshold(10 * time.Millisecond)

	clock.advance(20 * time.Millisecond)
	m.ObserveFrame(15 * time.Millisecond) // long
	clock.advance(20 * time.Millisecond)
	m.ObserveFrame(2 * time.Millisecond) // short
	clock.advance(20 * time.Millisecond)
	m.ObserveFrame(50 * time.Millisecond) // long

	s := m.Snapshot()
	assert.Equal(t, int64(2), s.LongFrames)
}

// TestFrameMetrics_InputToPresent records input-to-present samples and
// keeps the maximum across calls. The maximum is the most useful signal:
// it tells the user "this is the worst case UI latency I saw."
func TestFrameMetrics_InputToPresent(t *testing.T) {
	m := NewFrameMetrics()
	m.RecordInputToPresent(10 * time.Millisecond)
	m.RecordInputToPresent(20 * time.Millisecond)
	m.RecordInputToPresent(15 * time.Millisecond)

	s := m.Snapshot()
	assert.Equal(t, 15*time.Millisecond, s.InputToPresent)
	assert.Equal(t, 20*time.Millisecond, s.MaxInputToPresent)
}

// TestFrameMetrics_Coalesced verifies the coalescing counters.
func TestFrameMetrics_Coalesced(t *testing.T) {
	m := NewFrameMetrics()
	m.IncCoalescedScroll(5)
	m.IncCoalescedScroll(0)
	m.IncCoalescedMutations(3)
	m.IncStaleFramesDropped()
	m.IncStaleFramesDropped()

	s := m.Snapshot()
	assert.Equal(t, int64(5), s.CoalescedScrollEvents)
	assert.Equal(t, int64(3), s.CoalescedMutations)
	assert.Equal(t, int64(2), s.StaleFramesDropped)
}

// TestFrameMetrics_ConcurrentObserve is the safety net for the hot path.
func TestFrameMetrics_ConcurrentObserve(t *testing.T) {
	m := NewFrameMetrics()
	const N = 200
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.ObserveFrame(time.Millisecond)
		}()
	}
	wg.Wait()
	s := m.Snapshot()
	assert.Equal(t, int64(N), s.Frames)
}

// TestFrameMetrics_Reset verifies the navigation-time reset path.
func TestFrameMetrics_Reset(t *testing.T) {
	clock := newFakeClockForMetrics(time.Unix(0, 0))
	m := NewFrameMetricsWithClock(clock.now)
	clock.advance(16 * time.Millisecond)
	m.ObserveFrame(5 * time.Millisecond)
	m.IncCoalescedScroll(3)

	m.Reset()
	s := m.Snapshot()
	assert.Zero(t, s.Frames)
	assert.Zero(t, s.RenderDuration)
	assert.Zero(t, s.CoalescedScrollEvents)
}

// fakeClockForMetrics provides a controllable time source for
// FrameMetrics tests. Kept local to this file to avoid coupling to
// the FPSCounter's clock helper.
type fakeClockForMetrics struct {
	t time.Time
}

func newFakeClockForMetrics(start time.Time) *fakeClockForMetrics {
	return &fakeClockForMetrics{t: start}
}

func (c *fakeClockForMetrics) now() time.Time { return c.t }
func (c *fakeClockForMetrics) advance(d time.Duration) {
	c.t = c.t.Add(d)
}
