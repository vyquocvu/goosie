package surface_test

import (
	"context"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/surface"
)

// ---------------------------------------------------------------------------
// Layer 2: integration tests through the real headless.Window + ManualClock.
//
// These prove that events injected via headless.Window.Inject flow through the
// surface loop correctly when paced by a deterministic clock. Unlike the
// fakeWindow tests above, these exercise the real event channel, the real
// vsync goroutine, and the real scale-stamping logic.
// ---------------------------------------------------------------------------

type headlessHarness struct {
	w     *headless.Window
	clk   *headless.ManualClock
	loop  *surface.Loop
	sched *tileScheduler
	join  func() error
}

func newHeadlessHarness(t *testing.T, vp frame.Viewport) *headlessHarness {
	t.Helper()
	clk := headless.NewManualClock(time.Unix(1_700_000_000, 0))
	w := headless.New(headless.Config{
		Size:        vp.Size,
		Scale:       1,
		VsyncPeriod: 16 * time.Millisecond,
		Clock:       clk,
		EventBuffer: 128,
	})
	t.Cleanup(func() { w.Close() })

	l, dl := newScene(t, 2, 12)
	s := newScheduler(l, dl, vp)
	composer := newComposerFor(vp)
	loop := surface.NewLoop(w, s, composer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()

	h := &headlessHarness{
		w:     w,
		clk:   clk,
		loop:  loop,
		sched: s,
		join: func() error {
			cancel()
			select {
			case err := <-done:
				return err
			case <-time.After(10 * time.Second):
				t.Fatal("Run did not return after cancellation")
				return nil
			}
		},
	}
	// Wait for the vsync goroutine to arm its first timer.
	waitPending(t, clk)
	return h
}

// waitPending blocks until the clock has at least one armed timer.
func waitPending(t *testing.T, clk *headless.ManualClock) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if clk.Pending() > 0 {
			return
		}
		time.Sleep(50 * time.Microsecond)
	}
	t.Fatal("clock has no pending timers: the vsync loop did not arm")
}

// tick advances the clock by one vsync period and waits for the loop to
// process the frame and re-arm the next timer.
func (h *headlessHarness) tick(t *testing.T) {
	t.Helper()
	h.clk.Advance(16 * time.Millisecond)
	waitPending(t, h.clk)
}

func TestHeadless_ScrollInjectReachesScheduler(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	h := newHeadlessHarness(t, vp)

	for i := 0; i < 3; i++ {
		if err := h.w.Inject(surface.Event{
			Kind:  surface.EvScroll,
			Delta: frame.Point{Y: 100},
		}); err != nil {
			t.Fatalf("Inject: %v", err)
		}
	}

	h.tick(t)
	waitFor(t, "frame", func() bool { return h.loop.Stats().Frames >= 1 })

	if got := h.sched.offsetY(); got != 300 {
		t.Fatalf("scroll offset = %d, want 300", got)
	}
	if err := h.join(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestHeadless_PointerEventsCoalesceIntoOneFrame(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	h := newHeadlessHarness(t, vp)

	for _, pos := range []frame.Point{{X: 10, Y: 20}, {X: 50, Y: 60}, {X: 100, Y: 200}} {
		if err := h.w.Inject(surface.Event{
			Kind:   surface.EvPointer,
			Pos:    pos,
			Button: surface.ButtonLeft,
		}); err != nil {
			t.Fatalf("Inject: %v", err)
		}
	}

	h.tick(t)
	waitFor(t, "frame", func() bool { return h.loop.Stats().Frames >= 1 })

	if got := h.loop.Stats().Frames; got != 1 {
		t.Fatalf("frames = %d, want 1: pointer events should coalesce", got)
	}
	if err := h.join(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestHeadless_MixedEventsCoalesceIntoOneFrame(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	h := newHeadlessHarness(t, vp)

	if err := h.w.Inject(surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: 50}}); err != nil {
		t.Fatal(err)
	}
	if err := h.w.Inject(surface.Event{Kind: surface.EvPointer, Pos: frame.Point{X: 10, Y: 20}}); err != nil {
		t.Fatal(err)
	}
	if err := h.w.Inject(surface.Event{Kind: surface.EvKey, Key: 'a'}); err != nil {
		t.Fatal(err)
	}
	if err := h.w.Inject(surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: 75}}); err != nil {
		t.Fatal(err)
	}

	h.tick(t)
	waitFor(t, "frame", func() bool { return h.loop.Stats().Frames >= 1 })

	if got := h.sched.offsetY(); got != 125 {
		t.Fatalf("scroll offset = %d, want 125", got)
	}
	if err := h.join(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestHeadless_ResizePropagates(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	h := newHeadlessHarness(t, vp)

	if err := h.w.Inject(surface.Event{
		Kind:  surface.EvResize,
		Size:  frame.Size{W: 512, H: 384},
		Scale: 2,
	}); err != nil {
		t.Fatalf("Inject resize: %v", err)
	}

	h.tick(t)
	waitFor(t, "frame", func() bool { return h.loop.Stats().Frames >= 1 })

	stats := h.w.Stats()
	if stats.Presents == 0 {
		t.Fatal("no frames presented after resize")
	}
	if err := h.join(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestHeadless_VsyncInjectIsRejected(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	h := newHeadlessHarness(t, vp)
	defer h.join()

	err := h.w.Inject(surface.Event{Kind: surface.EvVsync})
	if err != headless.ErrVsyncIsClocks {
		t.Fatalf("Inject(EvVsync) = %v, want ErrVsyncIsClocks", err)
	}
}

func TestHeadless_MultipleVsyncsDrawMultipleFrames(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	h := newHeadlessHarness(t, vp)

	const ticks = 5
	for i := 0; i < ticks; i++ {
		h.tick(t)
	}
	waitFor(t, "frames", func() bool { return h.loop.Stats().Frames >= int64(ticks) })

	if got := h.loop.Stats().Frames; got < int64(ticks) {
		t.Fatalf("frames = %d, want at least %d", got, ticks)
	}
	if err := h.join(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestHeadless_ScrollAccumulatesAcrossVsyncs(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	h := newHeadlessHarness(t, vp)

	for i := 0; i < 5; i++ {
		if err := h.w.Inject(surface.Event{
			Kind:  surface.EvScroll,
			Delta: frame.Point{Y: 100},
		}); err != nil {
			t.Fatalf("Inject: %v", err)
		}
		h.tick(t)
	}
	waitFor(t, "frames", func() bool { return h.loop.Stats().Frames >= 5 })

	if got := h.sched.offsetY(); got != 500 {
		t.Fatalf("scroll offset = %d, want 500", got)
	}
	if err := h.join(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestHeadless_WindowStatsReflectInjection(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	h := newHeadlessHarness(t, vp)

	for i := 0; i < 7; i++ {
		if err := h.w.Inject(surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: 10}}); err != nil {
			t.Fatal(err)
		}
	}

	stats := h.w.Stats()
	if stats.Injected != 7 {
		t.Fatalf("Injected = %d, want 7", stats.Injected)
	}

	for i := 0; i < 3; i++ {
		h.tick(t)
	}
	waitFor(t, "frames", func() bool { return h.loop.Stats().Frames >= 3 })

	if err := h.join(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
