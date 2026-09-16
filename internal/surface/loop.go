package surface

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
)

// FrameWork counts the tile work one frame asked for and got. It is reported by
// the scheduler rather than measured by the loop, because the loop cannot see a
// tile grid: it holds a Window, a Composer, and a Scheduler, and knowing which
// tiles are valid is nobody's business but the grid's.
//
// The five fields are a partition with overlaps deliberately allowed: Needed is
// what the frame wanted, Submitted what actually reached a worker, Refused what
// a full queue turned away, Accepted what finished in time to be composited, and
// Reused what was already valid and needed nothing.
type FrameWork struct {
	Needed    int
	Submitted int
	Refused   int
	Accepted  int
	Reused    int
}

// Scheduler is the loop's entire view of content production. It is an interface
// rather than the raster scheduler itself for the reason the import graph states:
// surface sits below raster and cannot name it. Everything the loop does not need
// to know - pools, atlases, budgets, versions - stays behind these three calls.
//
// The three steps are separate methods so that the frame's five timestamps are
// five measurements rather than four. BeginFrame learns the input and produces a
// plan, Submit hands the tiles it named to workers and collects whatever already
// finished, and ComposeInto paints the damage. Each is called exactly once per
// frame, in that order, on the UI thread.
//
// The contract on Submit is the strong one: it must never wait for a tile. A
// scheduler that blocked here would put tile rasterization on the UI thread
// through the back door, and the loop would look identical right up until the
// first hitch.
type Scheduler interface {
	// BeginFrame coalesced input in, newest plan out. A nil plan means content
	// has not produced anything yet, and the frame is presented as nothing.
	BeginFrame(ev Event) (*frame.FramePlan, []frame.TileCoord)
	// Submit queues the named tiles and collects the results that are already
	// ready, without waiting for any of them.
	Submit(needed []frame.TileCoord) FrameWork
	// ComposeInto paints the plan's tiles into backing over the damage the input
	// implies, and returns the rects worth presenting. A viewport whose size
	// differs from the last one it saw must widen that damage to the whole
	// surface, because the loop swapped the buffer it is drawing into.
	ComposeInto(backing *frame.Bitmap, c *Composer, plan *frame.FramePlan, needed []frame.TileCoord) []frame.Rect
}

// LoopStats is the loop's running summary, which is what the gate tests read
// instead of instrumenting the loop's internals. Pixel writes are not here: the
// composer counts those, and a loop that had to reach into its own composer to
// report them would be a sign the split is in the wrong place.
type LoopStats struct {
	Frames    int64
	Presents  int64
	Idle      int64
	Rasterize int64
	Reused    int64
	Refused   int64
	Last      FrameWork
	LastPlan  uint64
}

// Loop is the UI thread's whole body: it takes vsyncs, asks for a plan, keeps the
// backing store the right size, and presents. It owns no pixels and no tiles, so
// there is nothing in it that could hold a frame hostage to rasterization.
//
// The loop is deliberately ignorant of what a tile is. That ignorance is the
// design: the invariants about tiles are enforced by the scheduler and the pool,
// and the loop's only obligation is to give them one chance per vsync and never
// wait for them.
type Loop struct {
	w   Window
	s   Scheduler
	c   *Composer
	rec *frame.FrameRecorder

	// pending is the coalescing buffer: everything that arrived since the last
	// vsync, folded into one event. It is a value rather than a queue because
	// the fold is lossless for the fields that matter, and a queue would only
	// reintroduce the ordering question the fold exists to settle.
	pending Event

	// mu guards only the counters, so that Stats can be read from another thread
	// - a gate harness, an inspector, a test - while the loop runs. Each frame
	// takes it exactly once, at the end, for a handful of additions.
	mu        sync.Mutex
	frames    int64
	presents  int64
	idle      int64
	rasterize int64
	reused    int64
	refused   int64
	last      FrameWork
	lastPlan  uint64
}

// tally is one frame's worth of counter movement. draw collects it as it goes and
// commits it once, which is what keeps the loop's counters readable from another
// thread without a lock around the frame itself.
type tally struct {
	frame   bool
	present bool
	idle    bool
	work    FrameWork
	plan    uint64
}

// NewLoop returns a loop that presents through w, asks s for frames, composes
// into c, and records into rec. rec may be nil, for a caller that wants the frame
// path without the timing ring; the loop then runs the same code with the
// timestamps dropped, which is the only honest way to measure nothing.
func NewLoop(w Window, s Scheduler, c *Composer, rec *frame.FrameRecorder) *Loop {
	return &Loop{w: w, s: s, c: c, rec: rec}
}

// Window returns the platform the loop presents to.
func (l *Loop) Window() Window { return l.w }

// Composer returns the composer whose backing store is presented.
func (l *Loop) Composer() *Composer { return l.c }

// Stats returns the loop's running summary. It is safe to call while Run is
// executing; the frame recorder alongside it is not, and belongs to whichever
// thread owns Run.
func (l *Loop) Stats() LoopStats {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LoopStats{
		Frames:    l.frames,
		Presents:  l.presents,
		Idle:      l.idle,
		Rasterize: l.rasterize,
		Reused:    l.reused,
		Refused:   l.refused,
		Last:      l.last,
		LastPlan:  l.lastPlan,
	}
}

// Run drives frames until the context is cancelled or the window stops sending
// events. Both of those are orderly shutdowns and return nil; an error means the
// platform refused a present, which is the only failure inside the loop that a
// caller can do anything about.
//
// The one blocking receive here is on the window's event channel. That is the
// channel whose emptiness is a display not asking for a frame, so waiting on it
// costs nothing the machine was going to spend anyway. No receive on a raster
// completion channel appears in this function, and adding one is the change that
// breaks invariant 2 fastest.
func (l *Loop) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	events := l.w.Events()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if ev.Kind != EvVsync {
				// Input that arrives between vsyncs is state, not work: a scroll
				// delta is only meaningful once the display is ready for another
				// frame, and drawing an intermediate one costs a present nobody
				// can see.
				l.pending = mergeEvent(l.pending, ev)
				continue
			}
			if err := l.draw(mergeEvent(l.pending, ev)); err != nil {
				return err
			}
			l.pending = Event{}
		}
	}
}

// mergeEvent folds src into dst. Scroll deltas accumulate, because a flick that
// delivered six deltas between two vsyncs moved the document by their sum, and
// presenting the intermediate positions would cost six frames of work for one
// visible result. Every other field takes the newer value, since the newest
// pointer, key, size, and scale are the only ones still true when the frame is
// drawn.
func mergeEvent(dst, src Event) Event {
	out := dst
	out.Kind = src.Kind
	if src.Kind == EvScroll || src.Delta != (frame.Point{}) {
		out.Delta.X += src.Delta.X
		out.Delta.Y += src.Delta.Y
	}
	if src.Pos != (frame.Point{}) || src.Kind == EvPointer {
		out.Pos = src.Pos
		out.Button = src.Button
	}
	if src.Key != 0 {
		out.Key = src.Key
	}
	if src.Size.W != 0 && src.Size.H != 0 {
		out.Size = src.Size
	}
	if src.Scale != 0 {
		out.Scale = src.Scale
	}
	if !src.At.IsZero() {
		out.At = src.At
	}
	return out
}

// draw runs one vsync-to-present frame.
//
// Counters are collected into a tally and committed under the lock at the end
// rather than written as the frame proceeds. The loop's own thread is the only
// writer either way, but Stats is documented as safe to call while Run executes,
// and a plain int64 incremented across five sites and read from another goroutine
// is a data race that the race detector finds in the first gate harness that
// tries it.
func (l *Loop) draw(ev Event) error {
	var t tally
	defer func() { l.commit(t) }()
	t.frame = true

	m := frame.FrameMark{VsyncAt: ev.At}

	plan, needed := l.s.BeginFrame(ev)
	m.PlanAt = time.Now()
	if plan != nil {
		m.Serial = plan.Serial
		t.plan = plan.Serial
	}

	// Only the loop holds the composer, so resizing its buffer is the loop's job
	// even though the reason for the resize came from the plan.
	if plan != nil {
		l.c.Resize(plan.Viewport.Size)
	}

	w := l.s.Submit(needed)
	m.SubmitAt = time.Now()
	t.work = w

	if plan == nil {
		// Nothing to show yet. This is an ordinary state rather than a failure:
		// a window can receive its first vsync before the producer has published
		// its first plan.
		t.idle = true
		if l.rec != nil {
			l.rec.Record(m)
		}
		return nil
	}

	damage := l.s.ComposeInto(l.c.Backing, l.c, plan, needed)
	m.ComposedAt = time.Now()
	if err := l.w.Present(l.c.Backing, damage); err != nil {
		return fmt.Errorf("surface: present frame %d: %w", l.frames+1, err)
	}
	t.present = true
	m.PresentedAt = time.Now()
	m.TilesRasterized = int32(w.Accepted)
	m.TilesReused = int32(w.Reused)
	// StylePasses and LayoutPasses stay zero through M1: there is no engine on
	// this path yet, and the recorder's report reads those zeros as the assertion
	// that a scroll frame ran neither.
	if l.rec != nil {
		l.rec.Record(m)
	}
	return nil
}

// commit folds one frame's tally into the running totals. Called once per frame
// from draw's defer, including on the error path, so a frame that failed to
// present still counts as a frame and not as a present.
func (l *Loop) commit(t tally) {
	if !t.frame {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.frames++
	if t.present {
		l.presents++
	}
	if t.idle {
		l.idle++
	}
	l.rasterize += int64(t.work.Accepted)
	l.reused += int64(t.work.Reused)
	l.refused += int64(t.work.Refused)
	l.last = t.work
	if t.plan != 0 {
		l.lastPlan = t.plan
	}
}
