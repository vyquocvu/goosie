package main

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
)

type stubWindow struct{ events chan surface.Event }

func (w *stubWindow) Events() <-chan surface.Event                         { return w.events }
func (w *stubWindow) Present(buf *frame.Bitmap, damage []frame.Rect) error { return nil }
func (w *stubWindow) SetCursor(surface.Cursor)                             {}
func (w *stubWindow) SetIME(bool)                                          {}
func (w *stubWindow) ScaleFactor() float32                                 { return 1 }
func (w *stubWindow) Close() error                                         { return nil }

func newTestDriver() *steadyDriver {
	f := &framePath{window: &stubWindow{events: make(chan surface.Event)}}
	return newSteadyDriver(f, 8)
}

// The frame ceiling must not fire while raster work is still landing: under
// parallel load the vsyncs pass in microseconds (a screenshot sweep observed
// 64 frames in 8ms with 26 tiles still on order), so a raw frame budget
// exits before the first rows finish and the PNG shows background.
func TestSteadyCeilingWaitsForProgress(t *testing.T) {
	d := newTestDriver()
	d.stallBudget = time.Hour // stall must not matter while progressing
	d.watchIdle(func() bool { return false })
	done := int64(0)
	d.watchProgress(func() (int64, int64) { return done, 1 })
	started := time.Now()
	for i := 0; i < 1000; i++ {
		d.mu.Lock()
		d.sent++
		d.mu.Unlock()
		done++ // every frame lands more raster work
		if d.ready(started) {
			t.Fatalf("ceiling fired at sent=%d while raster was progressing", d.sent)
		}
	}
}

func TestSteadyCeilingFiresOnStall(t *testing.T) {
	d := newTestDriver()
	d.stallBudget = time.Millisecond
	d.watchIdle(func() bool { return false })
	d.watchProgress(func() (int64, int64) { return 42, 1 }) // stuck, work outstanding
	started := time.Now()
	for i := 0; i < 8; i++ {
		d.mu.Lock()
		d.sent++
		d.mu.Unlock()
		if d.ready(started) {
			t.Fatalf("floor not respected at sent=%d", d.sent)
		}
	}
	time.Sleep(5 * time.Millisecond)
	d.mu.Lock()
	d.sent = 1000000 // past any frame ceiling; only stall logic may decide
	d.mu.Unlock()
	if !d.ready(started) {
		t.Fatal("stalled run with outstanding work did not exit past the stall budget")
	}
}

func TestSteadyIdleStillExits(t *testing.T) {
	d := newTestDriver()
	d.watchProgress(func() (int64, int64) { return 10, 0 })
	d.watchIdle(func() bool { return true })
	started := time.Now()
	exited := false
	for i := 0; i < 20; i++ {
		d.mu.Lock()
		d.sent++
		d.mu.Unlock()
		if d.ready(started) {
			exited = true
			break
		}
	}
	if !exited {
		t.Fatal("idle run did not exit")
	}
}
