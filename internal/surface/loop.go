package surface

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
)

type FrameWork struct {
	Needed        int
	Submitted     int
	Refused       int
	Accepted      int
	Reused        int
	Failed        int
	PaintingBytes int64
}

type Scheduler interface {
	BeginFrame(ev Event) (*frame.FramePlan, []frame.TileCoord)
	Submit(needed []frame.TileCoord) FrameWork
	ComposeInto(backing *frame.Bitmap, c *Composer, plan *frame.FramePlan, needed []frame.TileCoord) []frame.Rect
}

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

type Loop struct {
	w   Window
	s   Scheduler
	c   *Composer
	rec *frame.FrameRecorder

	pending Event

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

type tally struct {
	frame   bool
	present bool
	idle    bool
	work    FrameWork
	plan    uint64
}

type FrameScheduler interface {
	AnimationsActive() bool
}

type FrameRequester interface {
	ScheduleFrame()
}

func NewLoop(w Window, s Scheduler, c *Composer, rec *frame.FrameRecorder) *Loop {
	return &Loop{w: w, s: s, c: c, rec: rec}
}

func (l *Loop) Window() Window { return l.w }

func (l *Loop) Composer() *Composer { return l.c }

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
				if handler, ok := l.s.(interface{ HandleEvent(Event) bool }); ok && handler.HandleEvent(ev) {
					continue
				}
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

func mergeEvent(dst, src Event) Event {
	out := dst
	out.Kind = src.Kind
	if src.Kind == EvScroll || src.Delta != (frame.Point{}) {
		out.Delta.X += src.Delta.X
		out.Delta.Y += src.Delta.Y
	}
	if src.Pos != (frame.Point{}) || src.Kind == EvPointer || src.Kind == EvAXSetValue {
		out.Pos = src.Pos
		out.Button = src.Button
	}
	if src.Key != 0 {
		out.Key = src.Key
		out.Mods = src.Mods
	}
	if src.Kind == EvIME || src.Kind == EvAXSetValue {
		out.Text = src.Text
		if src.Kind == EvIME {
			out.IME = src.IME
		} else {
			out.AXTarget = src.AXTarget
		}
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

	if plan != nil {
		l.c.Resize(plan.Viewport.Size)
	}

	w := l.s.Submit(needed)
	m.SubmitAt = time.Now()
	t.work = w

	if plan == nil {
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

	if fs, ok := l.s.(FrameScheduler); ok && fs.AnimationsActive() {
		if fr, ok := l.w.(FrameRequester); ok {
			fr.ScheduleFrame()
		}
	}

	if l.rec != nil {
		l.rec.Record(m)
	}
	return nil
}

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
