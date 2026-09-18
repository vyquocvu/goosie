package main

import (
	"sync"

	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

// driver is a window that scrolls itself.
//
// A paced run cannot wait for a finger, and the measurement it is here for is a frame
// drawn in answer to a real vsync - so the driver invents no frames. It sits between
// the window and surface.Loop and stamps a scroll delta onto the ticks the display
// already produced, which is what a trackpad does to a vsync-carried event in every
// backend's own accounting. The loop, the scheduler, the composer, and the present
// path are then the ones an interactive run exercises, on the same channel and the
// same pacing, and that identity is the reason this file is a window decorator rather
// than a second frame loop.
//
// The channel to the loop is unbuffered and the loop draws before it receives again,
// so the driver learns a frame is finished by being allowed to hand over the next one.
// It reports completion one tick after the last measured delta, which is why a
// 600-frame run forwards 601 ticks.
type driver struct {
	surface.Window
	s    *raster.Scheduler
	step int32
	max  int32
	n    int

	out  chan surface.Event
	done chan struct{}

	// dir is the travel direction, reversed at the ends of the document.
	dir  int32
	mu   sync.Mutex
	sent int
	once sync.Once
}

// newDriver starts a driver over f's window. The window has to exist first: dropping
// frames the display never produced would be a gate that measured nothing.
func newDriver(f *framePath, frames int) *driver {
	dev := f.config.devSize()
	d := &driver{
		Window: f.window,
		s:      f.sched,
		step:   gateScrollStep,
		max:    f.layer.Bounds.H() - dev.H,
		n:      frames,
		out:    make(chan surface.Event),
		done:   make(chan struct{}),
		dir:    1,
	}
	if d.max < 0 {
		d.max = 0
	}
	go d.pump()
	return d
}

// Events implements surface.Window, replacing the wrapped window's channel with the
// transformed one. surface.Loop calls it exactly once, which is why the pump is
// started here rather than lazily.
func (d *driver) Events() <-chan surface.Event { return d.out }

// Frames reports how many vsyncs left the driver carrying a scroll delta.
func (d *driver) Frames() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sent
}

func (d *driver) pump() {
	defer close(d.out)
	defer d.finish()
	for ev := range d.Window.Events() {
		measured := false
		if ev.Kind == surface.EvVsync {
			d.mu.Lock()
			if d.sent < d.n {
				ev.Delta.Y += d.next()
				d.sent++
				measured = d.sent == d.n
			}
			d.mu.Unlock()
		}
		select {
		case d.out <- ev:
		case <-d.done:
			return
		}
		// The send returning is the proof: the loop took this tick, so it had already
		// returned from drawing the frame before it.
		if measured {
			d.finish()
		}
	}
}

// next returns one frame's scroll delta and reverses at the ends of the document, so a
// long run walks the page rather than spending its last frames pressed against a
// clamp. A clamped frame damages nothing and presents the same pixels, and a gate that
// averaged those would be averaging an idle vsync and calling it a scroll.
func (d *driver) next() int32 {
	step := d.step * d.dir
	at := d.s.Viewport().Offset.Y
	if at+step > d.max || at+step < 0 {
		d.dir = -d.dir
		step = d.step * d.dir
	}
	return step
}

// finish ends the run once. Closing done both releases the pump's send and tells
// run() to cancel the context and close the window.
func (d *driver) finish() {
	d.once.Do(func() { close(d.done) })
}

var _ surface.Window = (*driver)(nil)

// steadyDriver is a window that passes vsyncs through without scrolling and
// signals done after a fixed number of frames. It is the screenshot mode's
// driver: the page loads in the background, the loop presents frames, and
// after enough frames for tiles to rasterize the run exits and writes a PNG.
type steadyDriver struct {
	surface.Window
	out  chan surface.Event
	done chan struct{}
	n    int

	mu   sync.Mutex
	sent int
}

func newSteadyDriver(f *framePath, frames int) *steadyDriver {
	d := &steadyDriver{
		Window: f.window,
		out:    make(chan surface.Event),
		done:   make(chan struct{}),
		n:      frames,
	}
	go d.pump()
	return d
}

func (d *steadyDriver) Events() <-chan surface.Event { return d.out }

func (d *steadyDriver) pump() {
	defer close(d.out)
	for ev := range d.Window.Events() {
		d.mu.Lock()
		if ev.Kind == surface.EvVsync {
			d.sent++
			if d.sent >= d.n {
				d.mu.Unlock()
				select {
				case d.out <- ev:
				case <-d.done:
					return
				}
				close(d.done)
				return
			}
		}
		d.mu.Unlock()
		select {
		case d.out <- ev:
		case <-d.done:
			return
		}
	}
}

var _ surface.Window = (*steadyDriver)(nil)
