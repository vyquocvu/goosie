package main

import (
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

type driver struct {
	surface.Window
	s    *raster.Scheduler
	step int32
	max  int32
	n    int

	out  chan surface.Event
	done chan struct{}

	dir     int32
	mu      sync.Mutex
	sent    int
	once    sync.Once
	vsyncFn func() bool
}

func newDriver(f *framePath, frames int) *driver {
	dev := f.devSize()
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

func (d *driver) watchVsync(pump func() bool) {
	d.mu.Lock()
	d.vsyncFn = pump
	d.mu.Unlock()
}

func (d *driver) turn() bool {
	d.mu.Lock()
	pump := d.vsyncFn
	d.mu.Unlock()
	select {
	case <-d.done:
		return false
	default:
	}
	if pump == nil {
		return false
	}
	return pump()
}

func (d *driver) Events() <-chan surface.Event { return d.out }

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
			d.turn()
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
		if measured {
			d.finish()
		}
	}
}

func (d *driver) next() int32 {
	step := d.step * d.dir
	at := d.s.Viewport().Offset.Y
	if at+step > d.max || at+step < 0 {
		d.dir = -d.dir
		step = d.step * d.dir
	}
	return step
}

func (d *driver) finish() {
	d.once.Do(func() { close(d.done) })
}

var _ surface.Window = (*driver)(nil)

const steadyFloorFrames = 8

const steadyRevealHorizon = 500 * time.Millisecond

type steadyDriver struct {
	surface.Window
	out  chan surface.Event
	done chan struct{}
	n    int

	ceiling      int
	deadline     time.Duration
	settledFn    func() bool
	settled      int
	progressFn   func() (done, outstanding int64)
	stallBudget  time.Duration
	lastProgress int64
	lastMove     time.Time

	vsyncFn func() bool

	revealFn    func() (time.Duration, bool)
	revealStart time.Time

	seqFn func(time.Time) bool

	mu   sync.Mutex
	sent int
}

func newSteadyDriver(f *framePath, frames int) *steadyDriver {
	d := &steadyDriver{
		Window:      f.window,
		out:         make(chan surface.Event),
		done:        make(chan struct{}),
		n:           frames,
		ceiling:     frames * 8,
		deadline:    25 * time.Second,
		stallBudget: 5 * time.Second,
	}
	go d.pump()
	return d
}

func (d *steadyDriver) watchReveal(reveal func() (time.Duration, bool)) {
	d.mu.Lock()
	d.revealFn = reveal
	d.mu.Unlock()
}

func (d *steadyDriver) watchProgress(progress func() (done, outstanding int64)) {
	d.mu.Lock()
	d.progressFn = progress
	d.mu.Unlock()
}

func (d *steadyDriver) watchIdle(idle func() bool) {
	d.mu.Lock()
	d.settledFn = idle
	d.mu.Unlock()
}

func (d *steadyDriver) watchVsync(pump func() bool) {
	d.mu.Lock()
	d.vsyncFn = pump
	d.mu.Unlock()
}

func (d *steadyDriver) turn() bool {
	d.mu.Lock()
	pump := d.vsyncFn
	d.mu.Unlock()
	if pump == nil {
		return false
	}
	return pump()
}

func (d *steadyDriver) watchSequence(seq func(time.Time) bool, horizon time.Duration) {
	d.mu.Lock()
	d.seqFn = seq
	d.deadline += horizon
	d.mu.Unlock()
}

func (d *steadyDriver) Events() <-chan surface.Event { return d.out }

func (d *steadyDriver) pump() {
	defer close(d.out)
	started := time.Now()
	for ev := range d.Window.Events() {
		if ev.Kind == surface.EvVsync {
			if d.turn() {
				d.mu.Lock()
				d.settled = 0
				d.mu.Unlock()
			}
		}
		d.mu.Lock()
		if ev.Kind == surface.EvVsync {
			d.sent++
			d.mu.Unlock()
			if d.ready(started) {
				select {
				case d.out <- ev:
				case <-d.done:
					return
				}
				close(d.done)
				return
			}
		} else {
			d.mu.Unlock()
		}
		select {
		case d.out <- ev:
		case <-d.done:
			return
		}
	}
}

func (d *steadyDriver) ready(started time.Time) bool {
	if !d.frameReady(started) {
		return false
	}
	d.mu.Lock()
	seq := d.seqFn
	d.mu.Unlock()
	if seq == nil {
		return true
	}
	if seq(time.Now()) {
		return false
	}
	d.mu.Lock()
	d.seqFn = nil
	d.mu.Unlock()
	return true
}

func (d *steadyDriver) frameReady(started time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sent < d.n {
		return false
	}
	if d.settledFn == nil || time.Since(started) >= d.deadline {
		return true
	}
	if d.progressFn != nil {
		done, outstanding := d.progressFn()
		if d.lastMove.IsZero() || done != d.lastProgress {
			d.lastProgress = done
			d.lastMove = time.Now()
		}
		if outstanding > 0 {
			return time.Since(d.lastMove) > d.stallBudget
		}
	}
	if d.revealPending() {
		return false
	}
	if d.sent >= d.ceiling {
		return true
	}
	if d.settledFn() {
		d.settled++
	} else {
		d.settled = 0
	}
	return d.settled >= 2
}

func (d *steadyDriver) revealPending() bool {
	if d.revealFn == nil {
		return false
	}
	due, ok := d.revealFn()
	if !ok || due > steadyRevealHorizon {
		return false
	}
	if d.revealStart.IsZero() {
		d.revealStart = time.Now()
	}
	if time.Since(d.revealStart) > steadyRevealHorizon {
		return false
	}
	d.settled = 0
	return true
}

var _ surface.Window = (*steadyDriver)(nil)
