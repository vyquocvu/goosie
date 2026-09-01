package js_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/js"
)

// TestRuntime_LongTaskCount verifies that scripts exceeding the
// longTaskThreshold are counted but allowed to complete. The metric
// is non-fatal; it exists for DevTools observation.
func TestRuntime_LongTaskCount(t *testing.T) {
	rt := js.NewRuntime()
	rt.SetLongTaskThreshold(5 * time.Millisecond)

	// A quick script should not count.
	_, err := rt.RunScript(`1+1`)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), rt.LongTaskCount(), "fast script should not count")

	// A script that busy-loops in Goja for long enough should count.
	// We use a tight loop with no I/O so Goja is forced to interpret it.
	_, err = rt.RunScript(`var x=0; while (x<1e7) { x++; }`)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, rt.LongTaskCount(), uint64(1), "long script should bump the counter")
}

// TestRuntime_MaxTaskDurationInterrupts verifies that scripts exceeding
// the hard budget are interrupted. The interrupt is delivered as a
// Goja error containing the configured budget string, and the
// InterruptedCount is incremented.
//
// This is the freeze-fix safety net: a runaway script can no longer
// hold the Fyne UI thread indefinitely.
func TestRuntime_MaxTaskDurationInterrupts(t *testing.T) {
	rt := js.NewRuntime()
	rt.SetMaxTaskDuration(5 * time.Millisecond)

	_, err := rt.RunScript(`while (true) {}`)
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}
	if !strings.Contains(err.Error(), "budget") {
		t.Errorf("expected budget error, got %q", err.Error())
	}
	assert.Equal(t, uint64(1), rt.InterruptedCount())
}

// TestRuntime_LongTaskThresholdSetter validates the zero/negative input
// contract: non-positive values disable the metric.
func TestRuntime_LongTaskThresholdSetter(t *testing.T) {
	rt := js.NewRuntime()
	rt.SetLongTaskThreshold(0)
	assert.Zero(t, rt.LongTaskThreshold(), "zero threshold should disable")
	rt.SetLongTaskThreshold(-1)
	assert.Zero(t, rt.LongTaskThreshold(), "negative threshold should disable")
	rt.SetLongTaskThreshold(10 * time.Millisecond)
	assert.Equal(t, 10*time.Millisecond, rt.LongTaskThreshold())
}

// TestRuntime_MaxTaskDurationSetter validates the same contract for the
// hard budget.
func TestRuntime_MaxTaskDurationSetter(t *testing.T) {
	rt := js.NewRuntime()
	rt.SetMaxTaskDuration(0)
	assert.Zero(t, rt.MaxTaskDuration())
	rt.SetMaxTaskDuration(-1)
	assert.Zero(t, rt.MaxTaskDuration())
	rt.SetMaxTaskDuration(10 * time.Millisecond)
	assert.Equal(t, 10*time.Millisecond, rt.MaxTaskDuration())
}

// TestRuntime_CleanupResetsFrameScheduler verifies that the Cleanup
// method drops any pending RAF callbacks. This is the navigation-safety
// path: closing a runtime must not leave an animation loop alive on the
// new page.
func TestRuntime_CleanupResetsFrameScheduler(t *testing.T) {
	rt := js.NewRuntime()
	rt.RunScript(`requestAnimationFrame(function() {});`)

	assert.Equal(t, 1, rt.FrameScheduler().Pending(), "RAF should be pending")
	rt.Cleanup()
	assert.Equal(t, 0, rt.FrameScheduler().Pending(), "Cleanup must clear pending RAF callbacks")
}

// TestRuntime_FrameSchedulerIsPerInstance verifies that two runtimes do
// not share their RAF queues. (Sanity check; the runtime creates its
// own FrameScheduler in NewRuntime.)
func TestRuntime_FrameSchedulerIsPerInstance(t *testing.T) {
	a := js.NewRuntime()
	b := js.NewRuntime()
	assert.NotSame(t, a.FrameScheduler(), b.FrameScheduler())
}
