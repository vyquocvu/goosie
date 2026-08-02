package renderer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeClock provides a controllable time source for deterministic counter
// tests. Advance moves the clock by a fixed delta and returns the new time.
type fakeClock struct {
	t time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{t: start}
}

func (c *fakeClock) now() time.Time {
	return c.t
}

func (c *fakeClock) advance(d time.Duration) time.Time {
	c.t = c.t.Add(d)
	return c.t
}

// recordAt drives the clock forward by dt before recording one frame.
func recordAt(c *FPSCounter, clock *fakeClock, dt time.Duration) {
	clock.advance(dt)
	c.RecordFrame()
}

func TestFPSCounterEmptySnapshot(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	c := NewFPSCounterWithClock(clock.now)

	s := c.Snapshot()
	assert.Zero(t, s.Frames)
	assert.Zero(t, s.CurrentFPS)
	assert.Zero(t, s.Dropped)
}

func TestFPSCounterSingleFrameNoInterval(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	c := NewFPSCounterWithClock(clock.now)

	c.RecordFrame()

	s := c.Snapshot()
	assert.Equal(t, int64(1), s.Frames)
	// No prior interval yet, so no FPS is measurable.
	assert.Zero(t, s.CurrentFPS)
}

func TestFPSCounterSteadyFrameRate(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	c := NewFPSCounterWithClock(clock.now)

	// 60 frames at 60 fps (16.67ms apart) covering one second.
	const frameDur = time.Second / 60
	c.RecordFrame()
	for i := 0; i < 59; i++ {
		recordAt(c, clock, frameDur)
	}

	s := c.Snapshot()
	assert.Equal(t, int64(60), s.Frames)
	assert.InDelta(t, 60.0, s.CurrentFPS, 0.01)
	assert.InDelta(t, 60.0, s.AverageFPS, 0.01)
	assert.InDelta(t, 60.0, s.MinFPS, 0.01)
	assert.InDelta(t, 60.0, s.MaxFPS, 0.01)
	assert.Zero(t, s.Dropped)
	assert.InDelta(t, time.Second.Seconds(), s.SampleWindow.Seconds(), 0.05)
}

func TestFPSCounterMinMaxFPS(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	c := NewFPSCounterWithClock(clock.now)

	// Start strongly: 100 fps.
	c.RecordFrame()
	recordAt(c, clock, 10*time.Millisecond) // 100 fps
	recordAt(c, clock, 10*time.Millisecond) // 100 fps
	// Then a slow frame: 20 fps.
	recordAt(c, clock, 50*time.Millisecond) // 20 fps

	s := c.Snapshot()
	// Fastest interval (10ms) => max 100 fps; slowest (50ms) => min 20 fps.
	// The most recent interval (50ms) is the instantaneous/current fps.
	assert.InDelta(t, 100.0, s.MaxFPS, 0.01)
	assert.InDelta(t, 20.0, s.MinFPS, 0.01)
	assert.InDelta(t, 20.0, s.CurrentFPS, 0.01)
}

func TestFPSCounterDroppedFrames(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	c := NewFPSCounterWithClock(clock.now)

	c.RecordFrame()
	// On-budget frame (~16ms).
	recordAt(c, clock, 16*time.Millisecond)
	assert.Zero(t, c.Snapshot().Dropped)

	// A stall of 100ms exceeds the 60fps budget (16.67ms) => dropped.
	recordAt(c, clock, 100*time.Millisecond)
	assert.Equal(t, int64(1), c.Snapshot().Dropped)

	// Another on-budget frame does not add a drop.
	recordAt(c, clock, 16*time.Millisecond)
	assert.Equal(t, int64(1), c.Snapshot().Dropped)
}

func TestFPSCounterReset(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	c := NewFPSCounterWithClock(clock.now)

	c.RecordFrame()
	recordAt(c, clock, 16*time.Millisecond)
	recordAt(c, clock, 100*time.Millisecond)
	assert.NotZero(t, c.Snapshot().Frames)
	assert.NotZero(t, c.Snapshot().Dropped)

	c.Reset()
	s := c.Snapshot()
	assert.Zero(t, s.Frames)
	assert.Zero(t, s.Dropped)
	assert.Zero(t, s.CurrentFPS)
}

func TestFPSCounterWindowBounded(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	c := NewFPSCounterWithClock(clock.now)
	// Override bounds to something tiny for an easier assertion.
	c.maxSamples = 4
	c.target = 0

	const frameDur = time.Second / 60
	c.RecordFrame()
	for i := 0; i < 100; i++ {
		recordAt(c, clock, frameDur)
	}

	assert.Equal(t, int64(101), c.Snapshot().Frames)
	// The retained sample slice is capped.
	assert.LessOrEqual(t, len(c.samples), 4)
}

func TestFPSCounterSetTargetFPSIgnoresInvalid(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	c := NewFPSCounterWithClock(clock.now)

	orig := c.target
	c.SetTargetFPS(0)
	assert.Equal(t, orig, c.target)
	c.SetTargetFPS(-5)
	assert.Equal(t, orig, c.target)

	c.SetTargetFPS(120)
	fps := 120.0
	want := time.Duration(float64(time.Second) / fps)
	assert.Equal(t, want, c.target)
}

func TestFPSFromInterval(t *testing.T) {
	assert.InDelta(t, 60.0, fpsFromInterval(time.Second/60), 0.01)
	assert.Zero(t, fpsFromInterval(0))
	assert.Zero(t, fpsFromInterval(-time.Millisecond))
}
