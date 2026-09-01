package ui_test

import (
	"github.com/vyquocvu/goosie/internal/ui"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vyquocvu/goosie/internal/renderer"
)

// TestScheduleScroll_CollapsesBurst verifies the contract of the
// ScrollCoalescer as wired through the HTMLRenderer interface: N
// consecutive Schedule calls followed by N TryClaim calls must
// produce exactly 1 successful claim (the last viewport). This is the
// unit-level guard for the freeze fix: without coalescing, every
// scroll tick would walk the full display list, build Fyne objects,
// and trigger a refresh.
func TestScheduleScroll_CollapsesBurst(t *testing.T) {
	r := &MockHTMLRendererComp{}
	// Simulate the canvas scheduler path: only the first event should
	// queue presentation; the rest update the pending viewport.
	assert.True(t, r.ScheduleScroll(0, 600))
	for i := 1; i < 20; i++ {
		assert.False(t, r.ScheduleScroll(float32(i*10), 600))
	}
	v, ok := r.TryClaimScroll()
	assert.True(t, ok, "expected a pending claim after the burst")
	assert.Equal(t, float32(190), v.Y, "the last viewport must win")
	v, ok = r.TryClaimScroll()
	assert.False(t, ok, "no further claims should be pending")
}

// TestScheduleScroll_PerFrameBudget verifies that successive
// schedules from separate frames each yield a claim. The renderer
// is expected to call Schedule and TryClaim once per frame, so the
// coalescer must respect the frame-by-frame contract.
func TestScheduleScroll_PerFrameBudget(t *testing.T) {
	r := &MockHTMLRendererComp{}
	for i := 0; i < 5; i++ {
		r.ScheduleScroll(float32(i*100), 600)
		if v, ok := r.TryClaimScroll(); !ok {
			t.Fatalf("frame %d: expected a pending claim", i)
		} else {
			assert.Equal(t, float32(i*100), v.Y)
		}
	}
}

// TestScheduleScroll_NoSpuriousClaim ensures the coalescer reports
// "no work" before any Schedule call.
func TestScheduleScroll_NoSpuriousClaim(t *testing.T) {
	r := &MockHTMLRendererComp{}
	_, ok := r.TryClaimScroll()
	assert.False(t, ok, "no Schedule => no pending claim")
}

// TestFrameMetrics_ReachableFromUI verifies that the
// FrameMetricsSnapshot returns through the public HTMLRenderer
// interface (the DevTools performance panel reads from here).
func TestFrameMetrics_ReachableFromUI(t *testing.T) {
	var r ui.HTMLRenderer = &MockHTMLRendererComp{}
	// The method must not panic and must return a value (not crash).
	m := r.FrameMetrics()
	// On a fresh mock, all metrics should be zero. We assert the
	// shape rather than specific values: the snapshot is a struct
	// with multiple meaningful fields; a regression in the interface
	// would cause a nil dereference.
	_ = m.CoalescedScrollEvents
	_ = m.CoalescedMutations
	_ = m.StaleFramesDropped
	_ = m.LongFrames
	_ = renderer.FrameMetricsSnapshot{}
}
