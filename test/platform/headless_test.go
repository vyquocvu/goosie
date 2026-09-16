package platform_test

import (
	"errors"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/surface"
)

// The headless window is what every v2 automated run presents through, so its
// contract is the same surface.Window the macOS shim has to satisfy - and its
// determinism is what lets a pacing test say "16ms" instead of "eventually".
//
// These tests share a package with platform_test.go because Go allows one test
// package per directory, and the two are the two halves of one claim: the backend
// that always works, and the choice that picks it.

// hourPeriod is a vsync period long enough that no tick can arrive during a test
// that is only interested in injected input.
const hourPeriod = time.Hour

func newWindow(t *testing.T, cfg headless.Config) (*headless.Window, *headless.ManualClock) {
	t.Helper()
	if cfg.VsyncPeriod == 0 {
		cfg.VsyncPeriod = hourPeriod
	}
	if cfg.Size.W == 0 {
		cfg.Size = frame.Size{W: 4 * frame.TileSize, H: 2 * frame.TileSize}
	}
	clk := headless.NewManualClock(time.Unix(1_700_000_000, 0))
	cfg.Clock = clk
	w := headless.New(cfg)
	t.Cleanup(func() { _ = w.Close() })
	return w, clk
}

// waitPending blocks until the window's vsync loop has armed its next timer, which
// is the handshaking a manual clock needs: the loop re-arms only after taking the
// previous tick, so a test that advanced immediately could be counting a tick that
// was never scheduled.
func waitPending(t *testing.T, clk *headless.ManualClock) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for clk.Pending() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the window never armed a vsync timer")
		}
		time.Sleep(time.Millisecond)
	}
}

func nextEvent(t *testing.T, w *headless.Window, within time.Duration) surface.Event {
	t.Helper()
	select {
	case ev, ok := <-w.Events():
		if !ok {
			t.Fatal("the event channel closed")
		}
		return ev
	case <-time.After(within):
		t.Fatalf("no event within %v", within)
	}
	return surface.Event{}
}

func expectNoEvent(t *testing.T, w *headless.Window, within time.Duration) {
	t.Helper()
	select {
	case ev, ok := <-w.Events():
		if !ok {
			// A closed channel is the window saying the platform is gone, which is the
			// orderly end of a shutdown rather than an event nobody asked for.
			return
		}
		t.Fatalf("an event arrived within %v: %v", within, ev.Kind)
	case <-time.After(within):
	}
}

// TestVsyncTicksAtRequestedRate is the pacing contract in both directions: a tick
// per period, and no tick before the period is up. The second half matters as much
// as the first - a window that emits vsyncs as fast as the loop drains them turns
// every timing measurement in the project into a measurement of nothing.
func TestVsyncTicksAtRequestedRate(t *testing.T) {
	const period = 16 * time.Millisecond
	w, clk := newWindow(t, headless.Config{Scale: 1, VsyncPeriod: period})
	start := clk.Now()

	waitPending(t, clk)
	if n := clk.Advance(period / 2); n != 0 {
		t.Fatalf("half a period fired %d timers, want 0", n)
	}
	expectNoEvent(t, w, 50*time.Millisecond)

	// The other half of the same period, not a fresh one: a clock that reschedules
	// rather than accumulates would be quietly wrong about every rate below.
	if n := clk.Advance(period / 2); n != 1 {
		t.Fatalf("a full period fired %d timers, want 1", n)
	}
	for i, want := 0, period; i < 6; i, want = i+1, want+period {
		ev := nextEvent(t, w, 2*time.Second)
		if ev.Kind != surface.EvVsync {
			t.Fatalf("event %d was %v, want a vsync", i, ev.Kind)
		}
		if !ev.At.Equal(start.Add(want)) {
			t.Fatalf("vsync %d timestamped %v, want %v", i, ev.At, start.Add(want))
		}
		if ev.Scale != 1 {
			t.Fatalf("vsync %d carried scale %v, want the window's 1", i, ev.Scale)
		}
		if i == 5 {
			break
		}
		waitPending(t, clk)
		if n := clk.Advance(period); n != 1 {
			t.Fatalf("advance %d fired %d timers, want 1", i+2, n)
		}
	}
}

// TestScrollEventsDelivered is the input half of the window contract: what a driver
// injects is what the loop reads, in order, with nothing invented in between.
func TestScrollEventsDelivered(t *testing.T) {
	w, _ := newWindow(t, headless.Config{Scale: 1})
	deltas := []int32{100, -40, 7}
	for i, dy := range deltas {
		if err := w.Inject(surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: dy}}); err != nil {
			t.Fatalf("inject %d: %v", i, err)
		}
	}
	for i, dy := range deltas {
		ev := nextEvent(t, w, 2*time.Second)
		if ev.Kind != surface.EvScroll {
			t.Fatalf("event %d was %v, want the scroll that was injected", i, ev.Kind)
		}
		if ev.Delta.Y != dy {
			t.Fatalf("event %d delta = %v, want %d", i, ev.Delta, dy)
		}
	}
	expectNoEvent(t, w, 50*time.Millisecond)

	// A vsync is the clock's business, not a driver's: letting one be injected
	// would let a test appear to drive pacing while actually just queueing events.
	if err := w.Inject(surface.Event{Kind: surface.EvVsync}); !errors.Is(err, headless.ErrVsyncIsClocks) {
		t.Fatalf("Inject(EvVsync) = %v, want ErrVsyncIsClocks", err)
	}
}

// TestPresentWritesPNG checks the headless backend end to end, because a frame that
// never leaves the process proves nothing: the pixels are decoded back out of the
// file the run wrote.
func TestPresentWritesPNG(t *testing.T) {
	const (
		w0, h0 = 8, 6
	)
	ink := frame.RGB(10, 20, 30)
	w, _ := newWindow(t, headless.Config{Scale: 1, Size: frame.Size{W: w0, H: h0}})
	if err := w.WritePNG(filepath.Join(t.TempDir(), "none.png")); err == nil {
		t.Fatal("WritePNG succeeded before anything was presented")
	}
	if err := w.Present(nil, nil); err == nil {
		t.Fatal("Present(nil) reported success")
	}

	buf := frame.NewBitmap(w0, h0)
	buf.FillRect(buf.Bounds(), ink, nil)
	if err := w.Present(buf, []frame.Rect{frame.Rect4(0, 0, 2, 2)}); err != nil {
		t.Fatalf("Present: %v", err)
	}
	// The loop hands the same backing store to every frame, so a backend that kept
	// the caller's buffer instead of copying it would report a later frame as the
	// one it was shown.
	buf.FillRect(buf.Bounds(), frame.RGB(255, 255, 255), nil)

	dir := t.TempDir()
	path := filepath.Join(dir, "frame.png")
	if err := w.WritePNG(path); err != nil {
		t.Fatalf("WritePNG: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := img.Bounds().Dx(); got != w0 {
		t.Fatalf("PNG width = %d, want %d", got, w0)
	}
	cx, cy := w0/2, h0/2
	r, g, b, a := img.At(cx, cy).RGBA()
	if got := frame.AsColor(color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}); got != ink {
		t.Fatalf("centre pixel = %d,%d,%d,%d, want %v", r>>8, g>>8, b>>8, a>>8, ink)
	}
	if got := w.Stats().Presents; got != 1 {
		t.Fatalf("Presents = %d after one present, want 1", got)
	}
}

// TestScaleFactorHonored is the DPR plumbing at the only two points a platform can
// get it wrong: what the window reports, and what it stamps on the events it emits.
func TestScaleFactorHonored(t *testing.T) {
	w, clk := newWindow(t, headless.Config{Scale: 2, VsyncPeriod: time.Millisecond})
	if got := w.ScaleFactor(); got != 2 {
		t.Fatalf("ScaleFactor() = %v, want the configured 2", got)
	}
	waitPending(t, clk)
	if n := clk.Advance(time.Millisecond); n != 1 {
		t.Fatalf("advance fired %d timers, want 1", n)
	}
	if got := nextEvent(t, w, 2*time.Second).Scale; got != 2 {
		t.Fatalf("a vsync carried scale %v, want 2", got)
	}

	// An event built without a scale is the driver's mistake, and the fix has to
	// happen before the loop sees it: a scroll read at DPR 1 moves the page half as
	// far as the user's finger did.
	if err := w.Inject(surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: 10}}); err != nil {
		t.Fatalf("Inject: %v", err)
	}
	if got := nextEvent(t, w, 2*time.Second).Scale; got != 2 {
		t.Fatalf("an injected event arrived with scale %v, want the window's 2", got)
	}

	// A resize reports the platform's new ratio, and the window follows it.
	if err := w.Inject(surface.Event{Kind: surface.EvResize, Size: frame.Size{W: 512, H: 256}, Scale: 3}); err != nil {
		t.Fatalf("Inject resize: %v", err)
	}
	nextEvent(t, w, 2*time.Second)
	if got := w.ScaleFactor(); got != 3 {
		t.Fatalf("ScaleFactor() = %v after a resize to 3, want 3", got)
	}
	if got := w.Size(); got != (frame.Size{W: 512, H: 256}) {
		t.Fatalf("Size() = %v after a resize, want 512x256", got)
	}
}

// TestCloseIsIdempotentAndStopsTheClock is the shutdown half of the contract. A
// vsync goroutine that survives Close keeps sending into a channel nobody reads,
// which turns a clean exit into a blocked sender.
func TestCloseIsIdempotentAndStopsTheClock(t *testing.T) {
	w, clk := newWindow(t, headless.Config{Scale: 1, VsyncPeriod: time.Millisecond})
	waitPending(t, clk)
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := w.Inject(surface.Event{Kind: surface.EvScroll}); !errors.Is(err, headless.ErrClosed) {
		t.Fatalf("Inject after Close = %v, want ErrClosed", err)
	}
	clk.Advance(time.Hour)
	clk.Advance(time.Hour)
	expectNoEvent(t, w, 100*time.Millisecond)
	if got := w.Stats().DroppedVsyncs; got != 0 {
		t.Fatalf("DroppedVsyncs = %d after Close, want 0: a closed window has no clock to drop", got)
	}
	if err := w.Present(frame.NewBitmap(4, 4), nil); !errors.Is(err, headless.ErrClosed) {
		t.Fatalf("Present after Close = %v, want ErrClosed", err)
	}
}

// TestVsyncsDropRatherThanQueue covers the back-pressure rule. A queued vsync is a
// frame the UI thread has already decided not to draw, so a full event queue has to
// shed ticks rather than grow, and the count of what it shed has to be visible.
func TestVsyncsDropRatherThanQueue(t *testing.T) {
	w, clk := newWindow(t, headless.Config{Scale: 1, VsyncPeriod: time.Millisecond, EventBuffer: 2})
	for i := 0; i < 10; i++ {
		waitPending(t, clk)
		clk.Advance(time.Millisecond)
	}
	deadline := time.Now().Add(2 * time.Second)
	for w.Stats().DroppedVsyncs == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := w.Stats().DroppedVsyncs; got == 0 {
		t.Fatal("ten vsyncs into a two-deep queue were all queued; the queue is unbounded")
	}
	if got := w.Stats().Injected; got != 0 {
		t.Fatalf("Injected = %d from vsync ticks, want 0", got)
	}
}
