package headless

import (
	"errors"
	"image/png"
	"os"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
)

const (
	Name               = "headless"
	DefaultVsyncPeriod = 16 * time.Millisecond
	DefaultEventBuffer = 128
	DefaultHistory     = 2
)

var (
	ErrClosed        = errors.New("headless: window is closed")
	ErrNoPixels      = errors.New("headless: present with a buffer that has no pixels")
	ErrNoFrame       = errors.New("headless: nothing has been presented yet")
	ErrEventsFull    = errors.New("headless: the event queue is full")
	ErrVsyncIsClocks = errors.New("headless: vsync comes from the clock, not from a driver")
)

type Config struct {
	Size        frame.Size
	Scale       float32
	VsyncPeriod time.Duration
	Clock       Clock
	EventBuffer int
	History     int
}

type Stats struct {
	Presents      int64
	Injected      int64
	DroppedVsyncs int64
	Frames        int64
	DamagedRects  int
}

type Window struct {
	events chan surface.Event
	quit   chan struct{}
	clock  Clock
	period time.Duration

	wg sync.WaitGroup

	mu           sync.Mutex
	closed       bool
	scale        float32
	size         frame.Size
	cursor       surface.Cursor
	imeEnabled   bool
	axNodes      []ax.Node
	ring         []*frame.Bitmap
	last         int
	serial       int64
	presents     int64
	injected     int64
	dropped      int64
	damagedRects int
}

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

func (w *Window) Events() <-chan surface.Event { return w.events }

func (w *Window) enqueue(ev surface.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrClosed
	}
	if ev.Scale == 0 {
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

func (w *Window) SetCursor(c surface.Cursor) {
	w.mu.Lock()
	w.cursor = c
	w.mu.Unlock()
}

func (w *Window) Cursor() surface.Cursor {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cursor
}

func (w *Window) SetIME(enabled bool) {
	w.mu.Lock()
	w.imeEnabled = enabled
	w.mu.Unlock()
}

func (w *Window) IMEEnabled() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.imeEnabled
}

func (w *Window) SetAccessibility(nodes []ax.Node) {
	w.mu.Lock()
	w.axNodes = nodes
	w.mu.Unlock()
}

func (w *Window) AccessibilityTree() []ax.Node {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.axNodes
}

func (w *Window) ScaleFactor() float32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.scale
}

func (w *Window) Size() frame.Size {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size
}

func (w *Window) Name() string { return Name }

func (w *Window) VsyncPeriod() time.Duration { return w.period }

func (w *Window) DroppedVsyncs() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dropped
}

func (w *Window) Presents() (queued, dropped, committed int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.presents, 0, w.presents
}

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

func (w *Window) Presented() *frame.Bitmap {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.serial == 0 {
		return nil
	}
	return w.ring[w.last]
}

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

func (w *Window) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	close(w.quit)
	close(w.events)
	w.mu.Unlock()
	w.wg.Wait()
	return nil
}
