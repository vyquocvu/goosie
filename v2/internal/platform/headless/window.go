// Package headless is a surface.Window with no display behind it.
//
// It exists so that every automated v2 run - the CI gate, a benchmark, a screenshot
// diff - goes through the same loop, scheduler, composer, and Present path a real
// window does. That is the reason it implements surface.Window rather than offering a
// narrower test API: a frame path only proved against a fake would be a frame path only
// proved against a fake.
//
// Two things are deliberately invented here rather than borrowed from a platform: the
// clock, so pacing is deterministic, and the frame ring, so a run can write the picture
// it was shown out as a PNG. Everything else is the contract and nothing more.
package headless

import (
	"errors"
	"image/png"
	"os"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/v2/internal/frame"
	"github.com/vyquocvu/goosie/v2/internal/surface"
)

const (
	// DefaultVsyncPeriod is 60Hz, so a headless run measures the same cadence the macOS
	// shim will be held to.
	DefaultVsyncPeriod = 16 * time.Millisecond
	// DefaultEventBuffer is how many events may be queued before the window starts
	// shedding vsyncs.
	DefaultEventBuffer = 128
	// DefaultHistory is how many presented frames are retained. Two is the minimum
	// that makes "the frame I am looking at is the one that was drawn" true while the
	// next frame is being composed.
	DefaultHistory = 2
)

var (
	// ErrClosed reports use of a window whose owner has closed it.
	ErrClosed = errors.New("headless: window is closed")
	// ErrNoPixels reports a present with nothing to show.
	ErrNoPixels = errors.New("headless: present with a buffer that has no pixels")
	// ErrNoFrame reports a screenshot request before anything was presented.
	ErrNoFrame = errors.New("headless: nothing has been presented yet")
	// ErrEventsFull reports an injected event that did not fit. Input is never shed: a
	// lost scroll delta is a wrong document position, so the caller is told instead.
	ErrEventsFull = errors.New("headless: the event queue is full")
	// ErrVsyncIsClocks reports an attempt to inject a vsync. Ticks come from the clock,
	// and a driver that could invent one could make a pacing test pass without waiting.
	ErrVsyncIsClocks = errors.New("headless: vsync comes from the clock, not from a driver")
)

// Config is a headless window's setup. The zero value is a 60Hz window with no size,
// which becomes whatever the first Present shows.
type Config struct {
	// Size is the initial surface size, and the size ScaleFactor and EvResize are read
	// against before the first frame.
	Size frame.Size
	// Scale is the device pixel ratio the window reports. Zero means 1.
	Scale float32
	// VsyncPeriod is the interval between ticks. Zero means DefaultVsyncPeriod; a value
	// far longer than any test's patience is how a test gets input with no ticks.
	VsyncPeriod time.Duration
	// Clock paces the window. Nil means SystemClock.
	Clock Clock
	// EventBuffer is the event queue's depth. Zero means DefaultEventBuffer.
	EventBuffer int
	// History is how many presented frames to retain. Zero means DefaultHistory.
	History int
}

// Stats is what a headless run did, which is what a gate reports alongside the frame
// timings so a reader can tell a frame that was dropped from one that was slow.
type Stats struct {
	// Presents counts frames the loop handed over.
	Presents int64
	// Injected counts events a driver queued, and excludes the clock's own ticks.
	Injected int64
	// DroppedVsyncs counts ticks the queue could not hold. It is expected to be
	// non-zero when a run draws slower than the display asks for, and that is the
	// number worth reporting rather than a queue that grew.
	DroppedVsyncs int64
	// Frames is the serial of the newest retained present, so 0 means nothing shown.
	Frames int64
	// DamagedRects is how many rects the last present was told had changed. A frame
	// path that reports the whole surface every time still displays correctly, and this
	// is the counter that shows it is doing so.
	DamagedRects int
}

// Window is a headless surface.Window.
//
// One mutex guards everything a driver thread can touch - the closed flag, the scale,
// the size, the ring, and the counters - so ScaleFactor and Stats may be read while the
// UI thread runs. It is never held across a blocking operation: every send on the event
// channel is non-blocking by construction, which is what keeps a test that injects into
// a queue nobody drains from deadlocking against the window it is testing.
type Window struct {
	events chan surface.Event
	quit   chan struct{}
	clock  Clock
	// period is fixed after New, so the vsync loop and VsyncPeriod can read it unlocked.
	period time.Duration

	wg sync.WaitGroup

	mu           sync.Mutex
	closed       bool
	scale        float32
	size         frame.Size
	cursor       surface.Cursor
	ring         []*frame.Bitmap
	last         int
	serial       int64
	presents     int64
	injected     int64
	dropped      int64
	damagedRects int
}

// New returns a running window: its vsync loop starts here, so Close is required to
// stop it, and t.Cleanup is enough in a test.
func New(cfg Config) *Window {
	if cfg.Clock == nil {
		cfg.Clock = SystemClock{}
	}
	if cfg.VsyncPeriod == 0 {
		cfg.VsyncPeriod = DefaultVsyncPeriod
	}
	if cfg.VsyncPeriod < 0 {
		cfg.VsyncPeriod = DefaultVsyncPeriod
	}
	if cfg.Scale <= 0 {
		cfg.Scale = 1
	}
	if cfg.EventBuffer <= 0 {
		cfg.EventBuffer = DefaultEventBuffer
	}
	if cfg.History <= 0 {
		cfg.History = DefaultHistory
	}
	w := &Window{
		events: make(chan surface.Event, cfg.EventBuffer),
		quit:   make(chan struct{}),
		clock:  cfg.Clock,
		period: cfg.VsyncPeriod,
		scale:  cfg.Scale,
		size:   cfg.Size,
		ring:   make([]*frame.Bitmap, cfg.History),
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// run is the window's only goroutine: it turns clock ticks into events, one per period,
// and never runs anything else. It does not touch pixels, because a headless backend
// that rasterized on the way to the queue would hide the one thing these runs exist to
// measure.
func (w *Window) run() {
	defer w.wg.Done()
	for {
		ch := w.clock.After(w.period)
		select {
		case <-w.quit:
			return
		case at, ok := <-ch:
			if !ok {
				return
			}
			if err := w.enqueue(surface.Event{Kind: surface.EvVsync, At: at}); err != nil {
				return
			}
		}
	}
}

// Events implements surface.Window.
//
// The channel is closed by Close, which is how a loop learns the platform is gone. A
// driver that wants to stop drawing without that signal cancels its context instead.
func (w *Window) Events() <-chan surface.Event { return w.events }

// enqueue queues one event. A vsync that does not fit is dropped and counted rather than
// queued: a tick the UI thread has not taken is a frame it has already decided not to
// draw, and holding it would turn a slow frame into a growing backlog and every later
// timing measurement into a measurement of queue depth.
func (w *Window) enqueue(ev surface.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrClosed
	}
	if ev.Scale == 0 {
		// An event built without a ratio is the driver's mistake, and the fix has to
		// happen before the loop sees it: a scroll read at DPR 1 moves the page half as
		// far as the finger did.
		ev.Scale = w.scale
	}
	select {
	case w.events <- ev:
		return nil
	default:
	}
	if ev.Kind == surface.EvVsync {
		w.dropped++
		return nil
	}
	return ErrEventsFull
}

// Inject queues driver-authored input: a scroll, a pointer, a key, a resize. A resize
// also updates the size and scale the window reports afterwards, because a platform
// that resized does not keep answering questions with the old geometry.
func (w *Window) Inject(ev surface.Event) error {
	if ev.Kind == surface.EvVsync {
		return ErrVsyncIsClocks
	}
	if err := w.enqueue(ev); err != nil {
		return err
	}
	w.mu.Lock()
	w.injected++
	if ev.Scale != 0 {
		w.scale = ev.Scale
	}
	if ev.Kind == surface.EvResize && !ev.Size.Empty() {
		w.size = ev.Size
	}
	w.mu.Unlock()
	return nil
}

// Present records the frame. The headless backend uploads the whole buffer and ignores
// which rects changed - the honest behaviour for a file-format output - but it counts
// them, because "damage was reported at all" is a property worth watching.
//
// The pixels are copied into a per-serial buffer rather than retained by reference: the
// loop hands the same backing store to the next frame, so keeping the caller's bitmap
// would mean every screenshot showed the last frame drawn instead of the one presented.
func (w *Window) Present(buf *frame.Bitmap, damage []frame.Rect) error {
	if buf == nil || buf.Empty() {
		return ErrNoPixels
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrClosed
	}
	i := int(w.serial % int64(len(w.ring)))
	slot := w.ring[i]
	if slot == nil || slot.W != buf.W || slot.H != buf.H {
		slot = frame.NewBitmap(buf.W, buf.H)
		w.ring[i] = slot
	}
	copy(slot.RGBA, buf.RGBA)
	w.last = i
	w.serial++
	w.presents++
	w.size = buf.Size()
	w.damagedRects = len(damage)
	return nil
}

// SetCursor implements surface.Window. A headless window has no pointer, so the shape
// is only remembered, which is exactly enough for a test that asserts the engine asked
// for a text cursor over an input.
func (w *Window) SetCursor(c surface.Cursor) {
	w.mu.Lock()
	w.cursor = c
	w.mu.Unlock()
}

// Cursor returns the shape last requested.
func (w *Window) Cursor() surface.Cursor {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cursor
}

// ScaleFactor implements surface.Window.
func (w *Window) ScaleFactor() float32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.scale
}

// Size returns the surface size the window last reported, which is the configured one
// until a present or a resize changes it.
func (w *Window) Size() frame.Size {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size
}

// VsyncPeriod returns the interval the clock is driven at. It never changes, so a
// measurement of a run can name the rate it was paced at.
func (w *Window) VsyncPeriod() time.Duration { return w.period }

// Stats returns the counters. Safe to call while the window runs.
func (w *Window) Stats() Stats {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Stats{
		Presents:      w.presents,
		Injected:      w.injected,
		DroppedVsyncs: w.dropped,
		Frames:        w.serial,
		DamagedRects:  w.damagedRects,
	}
}

// Presented returns the retained copy of the newest presented frame. The bitmap belongs
// to the window: it stays valid for History further presents, after which the ring
// reuses it.
func (w *Window) Presented() *frame.Bitmap {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.serial == 0 {
		return nil
	}
	return w.ring[w.last]
}

// WritePNG writes the newest presented frame to path as a PNG.
//
// The pixels are copied out under the lock and encoded without it: a screenshot of a
// frame the loop is repainting halfway through would be a screenshot of a torn image,
// and holding the lock through an encoder would block the loop behind a file.
func (w *Window) WritePNG(path string) error {
	w.mu.Lock()
	if w.serial == 0 {
		w.mu.Unlock()
		return ErrNoFrame
	}
	src := w.ring[w.last]
	out := frame.NewBitmap(src.W, src.H)
	copy(out.RGBA, src.RGBA)
	w.mu.Unlock()

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, out.AsRGBA()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Close stops the vsync loop and closes the event channel. It is idempotent, and it
// waits for the loop to exit so a caller can be sure nothing is still sending.
func (w *Window) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	// Both closes happen under the lock, and every send checks the flag under the same
	// lock, so no goroutine can be about to send on the channel being closed here.
	close(w.quit)
	close(w.events)
	w.mu.Unlock()
	w.wg.Wait()
	return nil
}
