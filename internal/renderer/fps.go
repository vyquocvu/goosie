package renderer

import "time"

// fps.go — Live frame-rate instrumentation for the renderer.
//
// Goosie renders event-driven (not on a fixed ticker): a new "frame" is
// produced every time the viewport is (re)painted — navigation, window
// resize, refresh, DOM mutation, and — most importantly — each scroll tick
// via RenderWithViewport. FPSCounter records those frame-presentation
// events and derives instantaneous / average / min / max FPS plus a
// dropped-frame count against a configurable frame budget.
//
// The type is deliberately backend-neutral: it holds no Fyne, GUI, or
// platform dependencies so it can be constructed, driven, and asserted on
// in headless unit tests with an injected clock. UI adapters read FPSStats
// to render an on-screen HUD or a dev-tools readout.
type FPSCounter struct {
	now func() time.Time

	// target is the frame budget; any frame interval greater than target is
	// counted as a "missed" / dropped frame (defaults to 1/60s = 60 fps).
	target time.Duration

	// samples holds the bounded set of recent frame timestamps used to
	// compute the average over a moving window.
	samples []time.Time
	// maxSamples bounds the number of timestamps retained per window.
	maxSamples int

	// prev is the timestamp of the most recently recorded frame.
	prev time.Time

	total   int64
	dropped int64

	lastInterval time.Duration
	minInterval  time.Duration
	maxInterval  time.Duration
}

// FPSStats is an immutable snapshot of the measured frame-rate statistics at
// the instant Snapshot was called.
type FPSStats struct {
	// Frames is the total number of frames recorded since creation/reset.
	Frames int64
	// CurrentFPS is the reciprocal of the most recent frame interval.
	CurrentFPS float64
	// AverageFPS is the reciprocal of the mean frame interval across the
	// retained sampling window.
	AverageFPS float64
	// MinFPS is the minimum instantaneous FPS observed (1 / longest interval).
	MinFPS float64
	// MaxFPS is the maximum instantaneous FPS observed (1 / shortest interval).
	MaxFPS float64
	// Dropped is the number of frames whose interval exceeded the frame
	// budget (i.e. missed the target frame rate).
	Dropped int64
	// SampleWindow is the wall-clock span covered by the retained samples.
	SampleWindow time.Duration
}

// Default sampling / budgeting constants.
const (
	// defaultTargetFPS is the assumed display refresh rate for drop counting.
	defaultTargetFPS = 60.0
	// defaultMaxSamples bounds the sliding-window length (≈ 2s at 60 fps).
	defaultMaxSamples = 128
)

// NewFPSCounter creates a frame-rate counter with the default 60 fps budget
// and 128-sample sliding window, using the system clock.
func NewFPSCounter() *FPSCounter {
	return &FPSCounter{
		now:        time.Now,
		target:     time.Second / defaultTargetFPS,
		maxSamples: defaultMaxSamples,
	}
}

// NewFPSCounterWithClock creates a counter driven by the supplied clock
// function, useful for deterministic tests.
func NewFPSCounterWithClock(now func() time.Time) *FPSCounter {
	c := NewFPSCounter()
	c.now = now
	return c
}

// SetTargetFPS overrides the drop-counting frame budget. Pass a positive
// frame rate; non-positive values are ignored.
func (c *FPSCounter) SetTargetFPS(fps float64) {
	if fps > 0 {
		c.target = time.Duration(float64(time.Second) / fps)
	}
}

// RecordFrame notifies the counter that one frame was presented at the
// current clock time. It is a no-op for the first frame (there is no prior
// interval to measure).
func (c *FPSCounter) RecordFrame() {
	now := c.now()

	if !c.prev.IsZero() {
		dt := now.Sub(c.prev)
		if dt <= 0 {
			// Zero/negative interval (same or re-ordered timestamps): record
			// the frame but do not let it skew interval statistics.
			c.samples = append(c.samples, now)
		} else {
			c.lastInterval = dt
			if c.minInterval == 0 || dt < c.minInterval {
				c.minInterval = dt
			}
			if dt > c.maxInterval {
				c.maxInterval = dt
			}
			if dt > c.target {
				c.dropped++
			}
			c.samples = append(c.samples, now)
		}

		if len(c.samples) > c.maxSamples {
			c.samples = c.samples[len(c.samples)-c.maxSamples:]
		}
	}

	c.prev = now
	c.total++
}

// Snapshot returns a copy of the current frame-rate statistics.
func (c *FPSCounter) Snapshot() FPSStats {
	s := FPSStats{
		Frames:  c.total,
		Dropped: c.dropped,
	}

	if c.total == 0 || len(c.samples) < 2 {
		return s
	}

	s.CurrentFPS = fpsFromInterval(c.lastInterval)
	s.MinFPS = fpsFromInterval(c.maxInterval)
	s.MaxFPS = fpsFromInterval(c.minInterval)

	// Average over the retained window: mean of consecutive intervals.
	var sum time.Duration
	n := 0
	for i := 1; i < len(c.samples); i++ {
		dt := c.samples[i].Sub(c.samples[i-1])
		if dt > 0 {
			sum += dt
			n++
		}
	}
	if n > 0 {
		s.AverageFPS = fpsFromInterval(sum / time.Duration(n))
	}

	s.SampleWindow = c.samples[len(c.samples)-1].Sub(c.samples[0])
	return s
}

// Reset clears all recorded statistics and restarts measurement.
func (c *FPSCounter) Reset() {
	c.samples = nil
	c.prev = time.Time{}
	c.total = 0
	c.dropped = 0
	c.lastInterval = 0
	c.minInterval = 0
	c.maxInterval = 0
}

// fpsFromInterval converts a frame interval to frames-per-second, returning
// 0 for a zero or negative interval (no measurement available).
func fpsFromInterval(dt time.Duration) float64 {
	if dt <= 0 {
		return 0
	}
	return 1.0 / dt.Seconds()
}
