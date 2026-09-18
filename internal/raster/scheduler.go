package raster

import (
	"errors"
	"math"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/surface"
)

// Scheduler is the M1 frame path in one object: the per-vsync sequence that turns
// coalesced input into a plan, a list of tiles, a set of worker submissions, and a
// damage list, without ever waiting for a pixel to be drawn.
//
// It sits above surface.Loop's Scheduler interface and is the only place that is
// allowed to know about the tile grid, the worker pool, and the byte budget at the
// same time. That coupling is the reason it lives in raster rather than in frame:
// frame is the dependency leaf the grid's validity rule is proved against, and a
// scheduler that imported it upward would put the pool below the composer.
//
// One Scheduler belongs to one UI thread. Nothing in it locks, with a single
// documented exception: SetPlan publishes into a frame.Plan, which is the handoff a
// content-producing goroutine uses. Every other method must be called from the thread
// that runs the frame, including HandleDone - which is also why Submit's drain loop
// is a Poll and never a Wait.
type Scheduler struct {
	layer *frame.Layer
	grid  *frame.Grid
	pool  *Pool
	slot  frame.Plan
	prefs Pref

	// size and scale are the surface the platform last reported, and scroll is the
	// accumulated input in device pixels. The two are kept apart so a flick that ran
	// past the end of the page does not lose the overshoot the user would feel on
	// scroll-back: the clamped position is resolved per frame rather than stored.
	size   frame.Size
	scale  float32
	scroll frame.Point
	// dir is the direction of the last non-zero scroll, which is what the prefetch
	// ring is measured against. A zero delta must not erase it.
	dir frame.Point

	// cur is this frame's resolved plan. BeginFrame returns a pointer into it, so it
	// must outlive Submit and ComposeInto, which it does because the loop calls all
	// three before the next BeginFrame.
	cur frame.FramePlan

	// The scratch buffers are the frame path's only mutable state beyond counters, and
	// they are retained across frames: an append that could allocate is an append that
	// would break invariant 6 on a warm scroll.
	vis     []frame.TileCoord
	need    []frame.TileCoord
	pref    []frame.TileCoord
	blits   []surface.TileBlit
	damage  []frame.Rect
	prevVis []frame.TileCoord
	prevPix []*frame.Bitmap
	doneLay []*frame.Layer

	// The damage decision is made against the last *composition*, not the last plan:
	// a driver may resolve several plans per frame or draw one plan across several
	// vsyncs, and the backing store only knows what it is holding.
	haveDone   bool
	doneSize   frame.Size
	doneOffset frame.Point
	doneScale  float32
	doneBg     frame.Color

	reused      int
	reusedTotal int64

	scratch    int64
	frames     int64
	submitted  int64
	refused    int64
	deferred   int64
	prefetched int64
	rasterized int64
	failed     int64
	staleNews  int64
	writes     int64
}

// ErrNoViewport is what SubmitScroll reports before the scheduler has a surface to
// scroll against. It is an error rather than a silent no-op because a driver that
// injects input before it has sized the window has a setup bug worth hearing about.
var ErrNoViewport = errors.New("raster: scheduler has no surface size to scroll against")

// errForeignResult is returned for a completion that belongs to another layer. It is
// a value rather than a formatted error because HandleDone runs inside Submit, on the
// UI thread, and a result that should never happen is not worth an allocation even
// when it does.
var errForeignResult = errors.New("raster: result is for a different layer")

// PrefetchOff switches the prefetch ring off. Any negative PrefetchRows means the
// same thing, so a caller does not have to know the ring's default size to disable it.
const PrefetchOff int32 = -1

// Pref is the scheduler's tuning surface. Both fields are zero-value safe: the
// default ring is one viewport of tile rows on each side, and the default budget is
// whatever the layer was built with.
type Pref struct {
	// PrefetchRows is how many tile rows to warm ahead of and behind the viewport.
	// Zero means one viewport; a negative value disables the ring.
	PrefetchRows int32
	// Budget, when positive, is applied to the layer's tile grid with
	// Grid.SetBudget. It is the knob a running browser turns when memory gets tight.
	Budget int64
}

// Stats is the scheduler's summary, which is what the gate tests and the nightly
// report read. The five counters the design document names - Rasterized, Reused,
// Failed, Bytes, Budget - are a partition of what happened to tiles; the rest
// explain why they moved.
type Stats struct {
	// Rasterized counts tiles whose pixels were installed, Reused counts visible tiles
	// that needed nothing, and Failed counts tiles given up on after MaxTileAttempts.
	// None of them count work that was merely queued.
	Rasterized int64
	Reused     int64
	Failed     int64
	Bytes      int64
	Budget     int64

	Frames     int64
	Submitted  int64
	Refused    int64
	Prefetched int64
	StaleNews  int64
	// Writes counts pixels repainted by the composer - fills and blits over damage. It
	// deliberately excludes the rows a scroll memmoves, which cost in proportion to
	// the surface rather than to the exposed band and belong to the frame's own timing,
	// not to a recompose. This is the counter that distinguishes a damage blit from a
	// full recompose, and mixing the memmove into it would blur exactly that.
	Writes int64
	// Deferred is how many tiles a frame wanted to queue and did not, because the byte
	// budget could not pay for another in-flight raster. It is the counter that
	// distinguishes "the cache is full" from "the cache is losing pages".
	Deferred int64
}

// NewScheduler returns a scheduler that draws one layer through one pool.
//
// vp is the surface's initial geometry and may carry an initial scroll offset, which
// is taken as the starting position rather than as a delta. scale is the device pixel
// ratio the layer's content was produced for. prefs may be the zero value.
func NewScheduler(l *frame.Layer, pool *Pool, vp frame.Viewport, scale float32, prefs Pref) *Scheduler {
	s := &Scheduler{
		layer:  l,
		pool:   pool,
		size:   vp.Size,
		scale:  scale,
		prefs:  prefs,
		scroll: vp.Offset,
		dir:    frame.Point{Y: 1},
	}
	if s.scale <= 0 {
		s.scale = 1
	}
	if l != nil {
		s.grid = l.Grid
		s.scroll = frame.Point{
			X: clampOffset(vp.Offset.X, l.Bounds.X0, l.Bounds.W()-vp.Size.W),
			Y: clampOffset(vp.Offset.Y, l.Bounds.Y0, l.Bounds.H()-vp.Size.H),
		}
		if prefs.Budget > 0 && s.grid != nil {
			s.grid.SetBudget(prefs.Budget)
		}
	}
	// Sizing the scratch here, where the geometry is known and nothing is being
	// measured, is what lets ScratchAllocs stay at zero for the life of a
	// steady-state frame path.
	s.reserveScratch(false)
	return s
}

// SetPlan hands the scheduler the newest description of the content. It is the one
// method safe to call from another goroutine, and the only thing it does is publish
// into a depth-1 slot: a plan superseded before the next frame is drawn is counted
// rather than queued, which is the frame path's whole back-pressure story.
//
// The plan's Viewport and Scale are deliberately ignored. The scroll offset belongs to
// whoever is watching the page, and a producer that republished a plan with its own
// idea of the offset would erase input the user gave a frame earlier. A scripted
// scroll arrives as input instead, through SubmitScroll or a window event.
func (s *Scheduler) SetPlan(p frame.FramePlan) { s.slot.Publish(p) }

// PlanStats reports publishes and superseded plans, which is how a caller tells
// "content outpaces display" apart from a page that is simply idle.
func (s *Scheduler) PlanStats() frame.PlanStats { return s.slot.Stats() }

// SetViewport is how a driver that is not a window says "show this much, starting
// here": a headless run, a test, an anchor jump. An empty size leaves the size alone,
// so a caller that only wants to move can pass an offset.
func (s *Scheduler) SetViewport(vp frame.Viewport) {
	if !vp.Size.Empty() {
		s.size = vp.Size
		s.reserveScratch(true)
	}
	s.scroll = vp.Offset
}

// Viewport returns the viewport the next frame will draw, with the scroll offset
// already clamped into the document. It is the position the composer's tiles will be
// laid over, which is why it is resolved rather than stored.
func (s *Scheduler) Viewport() frame.Viewport { return s.resolve() }

// Stats returns the scheduler's counters with the grid's memory read live, so a
// caller watching a plateau does not have to reach past the scheduler to do it.
func (s *Scheduler) Stats() Stats {
	st := Stats{
		Rasterized: s.rasterized,
		Reused:     s.reusedTotal,
		Failed:     s.failed,
		Frames:     s.frames,
		Submitted:  s.submitted,
		Refused:    s.refused,
		Prefetched: s.prefetched,
		StaleNews:  s.staleNews,
		Writes:     s.writes,
		Deferred:   s.deferred,
	}
	if s.grid != nil {
		g := s.grid.Stats()
		st.Bytes, st.Budget = g.Bytes, g.Budget
	}
	return st
}

// ScratchAllocs reports how many times a scratch buffer had to grow after
// construction. Zero is the steady-state contract the CI gate asserts on: a warm
// scroll must not need a bigger list than the last one, and the only way to prove
// that without a benchmark is to make the growth observable.
func (s *Scheduler) ScratchAllocs() int64 { return s.scratch }

// Damage returns the rects the last ComposeInto asked the composer to repaint. It is
// the scheduler's own list, in surface coordinates and unclipped, which is the
// version worth asserting on: the composer narrows it further.
//
// The slice is owned by the scheduler and valid until the next ComposeInto.
func (s *Scheduler) Damage() []frame.Rect { return s.damage }

// BeginFrame folds coalesced input in and resolves the frame's plan and the tiles it
// still needs. It is the loop's first call each vsync and never the last: nothing here
// waits for a worker, and nothing here allocates on a steady-state frame.
func (s *Scheduler) BeginFrame(ev surface.Event) (*frame.FramePlan, []frame.TileCoord) {
	s.frames++
	s.fold(ev.Delta)
	if !ev.Size.Empty() {
		s.size = ev.Size
	}
	if ev.Scale > 0 {
		s.scale = ev.Scale
	}

	s.vis, s.need, s.pref = s.vis[:0], s.need[:0], s.pref[:0]
	s.reused = 0

	p, ok := s.slot.Take()
	if !ok {
		// No content yet. An ordinary first-vsync-before-first-plan, not a failure.
		s.cur = frame.FramePlan{}
		return nil, nil
	}
	// A plan whose first layer changed is a page navigation: swap the grid to
	// match. This runs on the UI thread inside BeginFrame, so no lock is needed;
	// the cross-thread publish is handled by the Plan slot's own mutex.
	if len(p.Layers) > 0 && p.Layers[0] != s.layer {
		s.layer = p.Layers[0]
		s.grid = s.layer.Grid
		s.haveDone = false
	}
	vp := s.resolve()
	s.cur = frame.FramePlan{
		Serial:     p.Serial,
		Viewport:   vp,
		Scale:      s.scale,
		Layers:     p.Layers,
		Background: p.Background,
	}
	if s.grid == nil {
		return &s.cur, s.need
	}
	s.reserveScratch(true)
	s.grid.Tick()

	s.vis = s.grid.VisibleCoords(vp, s.vis)
	// Walking the visible set from the leading edge inward means the queue the workers
	// read is ordered by what the user is about to look at, which is the cheapest
	// available win for p99: a first row is composited as soon as it is ready rather
	// than after everything behind it.
	backward := s.dir.Y > 0 || (s.dir.Y == 0 && s.dir.X > 0)
	for i := range s.vis {
		j := i
		if backward {
			j = len(s.vis) - 1 - i
		}
		c := s.vis[j]
		// Recency is recorded for every tile on screen, valid or not. It is what makes
		// the byte budget's eviction pick the prefetch ring rather than the page: the
		// ring is never touched, so it is always the oldest thing in the cache.
		s.grid.Touch(c)
		if !s.layer.Needs(c) {
			s.reused++
			s.reusedTotal++
			continue
		}
		// A tile with a buffer already on order is being painted right now, and a
		// failed tile is a decision not to paint it again until content changes.
		if s.grid.InFlight(c) || s.grid.Failed(c) {
			continue
		}
		s.need = append(s.need, c)
	}
	s.prefetched += int64(s.ring(vp))
	return &s.cur, s.need
}

// Submit queues the tiles a frame named and collects whatever has already finished.
//
// The contract is the one the loop cannot verify: it never waits. Both directions are
// non-blocking by construction - Submit refuses a full queue rather than parking on
// it, and the drain loop below reads a completion channel with a non-blocking Poll,
// because a receive that could wait here would put tile rasterization on the UI thread
// through the back door.
//
// The byte budget is enforced here rather than left to the grid, and that is not
// duplication. Grid.Acquire evicts to make room, but it can only evict presentable
// pixels: a buffer a worker holds is not evictable, so a frame that queued its whole
// visible set against a full cache would overshoot the ceiling by however many tiles it
// asked for. Admitting an acquire only while the buffers already on order plus one more
// still fit is what keeps used <= budget true across the whole frame path, which is the
// plateau the memory criterion is stated as.
func (s *Scheduler) Submit(needed []frame.TileCoord) surface.FrameWork {
	w := surface.FrameWork{Needed: len(needed), Reused: s.reused}
	if s.grid == nil || s.pool == nil || s.layer == nil {
		return w
	}
	dl, _ := s.layer.Content.(*paint.LayerDL)
	budget := s.grid.Stats().Budget
	tileBytes := s.tileBytes()
	ts := s.grid.TileSize()
	// The running total rather than a fresh grid read per tile: only the drain below
	// lowers it, and that runs after this loop.
	painting := s.grid.Stats().PaintingBytes

	for i, c := range needed {
		if painting+tileBytes > budget {
			// The list is priority-ordered, so nothing later in it fits either. Stop
			// rather than walk the rest of the frame declining one tile at a time.
			s.deferred += int64(len(needed) - i)
			break
		}
		if s.grid.InFlight(c) {
			continue
		}
		out, ok := s.grid.Acquire(c)
		if !ok {
			continue
		}
		painting += tileBytes
		w.Submitted++
		s.submitted++
		if err := s.pool.Submit(Job{Layer: s.layer, Coord: c, DL: dl, Bounds: c.Rect(ts), Out: out}); err != nil {
			s.grid.Release(c, out)
			painting -= tileBytes
			w.Refused++
			s.refused++
			// ErrQueueFull and ErrPoolClosed are handled identically here: the tile
			// stays needed and the next frame asks again. The pool's own stats are where
			// the two are told apart.
		}
	}

	for {
		d, ok := s.pool.Poll()
		if !ok {
			break
		}
		before := s.rasterized
		// The per-tile error is returned to a caller that asks for it. Submit has no
		// error to report: the tile is already counted in Failed and will be retried or
		// given up on by the grid's own rule.
		_ = s.HandleDone(d)
		if s.rasterized > before {
			w.Accepted++
		}
	}
	return w
}

// ComposeInto paints the frame into the composer's backing store and returns the rects
// worth presenting.
//
// A pure vertical scroll is the case the whole design is shaped around, so it is worth
// stating why the exposed band is the only damage it needs. After the backing store is
// shifted by the scroll, screen row y holds the pixel that was at y+dy, which is the
// content row off.Y+y - exactly what the new viewport wants at screen y. Every shifted
// pixel is already right. What is left is the band of garbage the shift exposed and the
// tiles whose pixels changed underneath a pixel that stayed on screen.
//
// Anything that breaks that argument - a horizontal move, a size change, a scale
// change, a background change, a different layer set, or the first frame - damages the
// whole surface instead.
func (s *Scheduler) ComposeInto(backing *frame.Bitmap, c *surface.Composer, plan *frame.FramePlan, needed []frame.TileCoord) []frame.Rect {
	s.damage = s.damage[:0]
	s.blits = s.blits[:0]
	if backing == nil || c == nil || plan == nil {
		return nil
	}
	// The composer returns its own scratch list, and this frame can hand it everything
	// the damage buffer holds. Reserving the bound rather than this frame's length keeps
	// the widening out of the frames that follow: see Composer.ReserveDamage.
	c.ReserveDamage(cap(s.damage))
	if s.grid == nil {
		// No layer to composite is still a surface that must show the background
		// rather than whatever the last document left there.
		s.damage = append(s.damage, backing.Bounds())
		return s.finish(backing, c, plan, frame.Point{})
	}

	off := plan.Viewport.Offset
	sz := backing.Size()
	full := !s.haveDone || s.doneSize != sz || s.doneScale != plan.Scale ||
		s.doneBg != plan.Background || !s.sameLayers(plan.Layers) || off.X != s.doneOffset.X

	dy := int32(0)
	if !full {
		dy = off.Y - s.doneOffset.Y
		if dy >= sz.H || -dy >= sz.H {
			// A jump of more than a surface height moves no row that is still wanted.
			full, dy = true, 0
		}
	}
	switch {
	case full:
		s.damage = append(s.damage, backing.Bounds())
	case dy > 0:
		// The memmove is deliberately not charged to Writes; see Stats.
		c.ShiftY(dy, nil)
		s.damage = append(s.damage, frame.Rect4(0, sz.H-dy, sz.W, sz.H))
	case dy < 0:
		c.ShiftY(dy, nil)
		s.damage = append(s.damage, frame.Rect4(0, 0, sz.W, -dy))
	default:
		// Nothing moved: only tiles that changed can need repainting, and if none did
		// the damage list stays empty and the composer writes no pixel at all.
	}

	ts := s.grid.TileSize()
	if !full {
		s.damage = s.damageTiles(off, ts)
	}
	for _, cv := range s.vis {
		if !s.grid.Blittable(cv) {
			continue
		}
		s.blits = append(s.blits, surface.TileBlit{
			Pixels: s.grid.Pixels(cv),
			Dst:    cv.Rect(ts).Translate(-off.X, -off.Y),
		})
	}
	return s.finish(backing, c, plan, off)
}

// damageTiles appends a rect for every tile that is on screen now whose pixels are not
// the ones the last composition put there - the only on-screen change a shift cannot
// account for. A coordinate that was not on screen before needs nothing extra: any part
// of it the shift could not reach is inside the band that was just added.
//
// Both lists are row-major, so this is a merge rather than a lookup, and it allocates
// nothing.
func (s *Scheduler) damageTiles(off frame.Point, ts int32) []frame.Rect {
	i := 0
	for _, c := range s.vis {
		for i < len(s.prevVis) && coordLess(s.prevVis[i], c) {
			i++
		}
		if i >= len(s.prevVis) || s.prevVis[i] != c {
			continue
		}
		if i < len(s.prevPix) && s.prevPix[i] != s.grid.Pixels(c) {
			s.damage = append(s.damage, c.Rect(ts).Translate(-off.X, -off.Y))
		}
		i++
	}
	return s.damage
}

// finish runs the compose and records what the backing store now depicts, which is what
// the next frame's damage decision is measured against.
func (s *Scheduler) finish(backing *frame.Bitmap, c *surface.Composer, plan *frame.FramePlan, off frame.Point) []frame.Rect {
	out := c.Compose(plan.Background, s.blits, s.damage, &s.writes)
	s.haveDone = true
	s.doneSize = backing.Size()
	s.doneOffset = off
	s.doneScale = plan.Scale
	s.doneBg = plan.Background
	s.doneLay = append(s.doneLay[:0], plan.Layers...)
	s.prevVis = append(s.prevVis[:0], s.vis...)
	s.prevPix = s.prevPix[:0]
	if s.grid != nil {
		for _, cv := range s.vis {
			s.prevPix = append(s.prevPix, s.grid.Pixels(cv))
		}
	}
	return out
}

// HandleDone applies one worker's result. It is exported because a driver that owns its
// own drain - a headless run collecting a finished document, a test holding a worker on
// a channel - calls it directly, and Submit calls it too on the UI thread. Both are the
// same single-threaded path.
//
// The version test is the reason Done carries a version at all: content bumps while a
// job runs, and installing the older rendering under the newer version would leave a
// tile that looks finished forever.
func (s *Scheduler) HandleDone(d Done) error {
	if s.grid == nil || s.layer == nil {
		return nil
	}
	// Check if the buffer came from a different grid. This can happen when layers
	// have the same ID but are different instances (e.g., after navigation), or when
	// the LayerID differs. Release foreign buffers back to their originating grid.
	if d.Layer != nil && d.Layer.Grid != nil && d.Layer.Grid != s.grid {
		d.Layer.Grid.Release(d.Coord, d.Out)
		s.staleNews++
		return errForeignResult
	}
	if d.LayerID != s.layer.ID {
		// The buffer came from another layer's pool, so this scheduler must not put it
		// anywhere. Release it back to the originating grid so the buffer returns to
		// its pool.
		if d.Layer != nil && d.Layer.Grid != nil {
			d.Layer.Grid.Release(d.Coord, d.Out)
		}
		s.staleNews++
		return errForeignResult
	}
	if d.Err != nil {
		// Release first: the buffer is no longer anybody's, and MarkFailed only drops
		// presentable pixels.
		s.grid.Release(d.Coord, d.Out)
		// The attempt count is the grid's, so the decision to stop trying stays in one
		// place. A tile that has not yet spent its attempts simply stays needed and is
		// requeued by the next frame.
		if s.grid.Attempts(d.Coord) >= frame.MaxTileAttempts {
			s.grid.MarkFailed(d.Coord)
			s.failed++
		}
		return d.Err
	}
	if d.Version < s.layer.ContentVersion {
		s.grid.Release(d.Coord, d.Out)
		s.staleNews++
		return nil
	}
	if s.grid.MarkValid(d.Coord, d.Version, d.Out) {
		s.rasterized++
		return nil
	}
	// MarkValid took none of it: the tile vanished, or the buffer was never this grid's.
	// It documents that the caller owes the buffer a release, which is what stops a
	// superseded raster from leaking its 256 KB.
	s.grid.Release(d.Coord, d.Out)
	s.staleNews++
	return nil
}

// SubmitScroll folds a scroll delta in without drawing a frame, for a driver that is not
// running surface.Loop and cannot wait for the next vsync to move the document. The fold
// is the same one BeginFrame applies to event deltas, so a caller that uses both moves
// the page once rather than twice.
//
// at is the event's timestamp, forwarded from surface.Event so a driver can hand over an
// event's fields unchanged. Nothing in this fold is time-dependent: a scroll is a
// position, and coalescing deltas is exact.
func (s *Scheduler) SubmitScroll(delta frame.Point, at time.Time) error {
	if s.size.Empty() {
		return ErrNoViewport
	}
	s.fold(delta)
	return nil
}

// fold accumulates a coalesced scroll delta. Saturating arithmetic rather than wrapping:
// a trackpad that reported a wild delta must not be able to move the page to the
// opposite end of an int32.
func (s *Scheduler) fold(delta frame.Point) {
	if delta.X == 0 && delta.Y == 0 {
		return
	}
	s.scroll.X = addSaturate(s.scroll.X, delta.X)
	s.scroll.Y = addSaturate(s.scroll.Y, delta.Y)
	if delta.X != 0 {
		s.dir.X = intSign(delta.X)
	}
	if delta.Y != 0 {
		s.dir.Y = intSign(delta.Y)
	}
}

// resolve turns the accumulated scroll into the viewport this frame draws, clamped so
// the document always covers the surface it is larger than.
func (s *Scheduler) resolve() frame.Viewport {
	v := frame.Viewport{Offset: s.scroll, Size: s.size}
	if s.layer == nil {
		return v
	}
	b := s.layer.Bounds
	v.Offset.X = clampOffset(v.Offset.X, b.X0, b.W()-v.Size.W)
	v.Offset.Y = clampOffset(v.Offset.Y, b.Y0, b.H()-v.Size.H)
	return v
}

// ring appends the prefetch band around the viewport to the needed list, leading edge
// first, and reports how many tiles it added. The coordinates go through the same
// filters as the visible set: a tile that is already on order, already given up on, or
// already current is not work, and counting it would make Prefetched mean something it
// does not.
func (s *Scheduler) ring(vp frame.Viewport) int {
	rows := s.ringRows()
	if rows <= 0 {
		return 0
	}
	r := vp.Rect()
	ts := s.grid.TileSize()
	dirs := [2]frame.Point{s.dir, {X: -s.dir.X, Y: -s.dir.Y}}
	added := 0
	for _, d := range dirs {
		if d.X == 0 && d.Y == 0 {
			continue
		}
		before := len(s.pref)
		s.pref = s.grid.PrefetchCoords(vp, d, rows, s.pref)
		for _, c := range s.pref[before:] {
			// The band's first row can be the viewport's last one when the offset is not
			// tile-aligned. That tile is already handled by the visible walk.
			if c.Rect(ts).Intersects(r) {
				continue
			}
			if s.grid.InFlight(c) || s.grid.Failed(c) || !s.layer.Needs(c) {
				continue
			}
			s.need = append(s.need, c)
			added++
		}
	}
	return added
}

// ringRows is the configured band thickness: an explicit count, one viewport when unset,
// or nothing at all when switched off.
func (s *Scheduler) ringRows() int {
	if s.prefs.PrefetchRows < 0 || s.grid == nil {
		return 0
	}
	if s.prefs.PrefetchRows > 0 {
		return int(s.prefs.PrefetchRows)
	}
	ts := s.grid.TileSize()
	if ts <= 0 {
		return 0
	}
	return int(max32Of(s.size.H/ts, s.size.W/ts)) + 1
}

func (s *Scheduler) tileSize() int32 {
	if s.grid == nil {
		return frame.TileSize
	}
	return s.grid.TileSize()
}

func (s *Scheduler) tileBytes() int64 {
	t := int64(s.tileSize())
	return t * t * 4
}

// sameLayers reports whether the composition is over the same set of layers. A swapped
// layer set can put different content behind the same pixels, which a shift cannot
// discover.
func (s *Scheduler) sameLayers(next []*frame.Layer) bool {
	if len(next) != len(s.doneLay) {
		return false
	}
	for i := range next {
		if next[i] != s.doneLay[i] {
			return false
		}
	}
	return true
}

// reserveScratch grows every frame-path buffer to what this geometry can ask for. count
// is false only at construction, where growth is the setup the counter below
// deliberately does not charge itself with.
func (s *Scheduler) reserveScratch(count bool) {
	n := s.maxCoords()
	reserve(&s.vis, n, count, &s.scratch)
	reserve(&s.need, n, count, &s.scratch)
	reserve(&s.pref, n, count, &s.scratch)
	reserve(&s.blits, n, count, &s.scratch)
	reserve(&s.damage, n+1, count, &s.scratch)
	reserve(&s.prevVis, n, count, &s.scratch)
	reserve(&s.prevPix, n, count, &s.scratch)
	// Layer sets are bounded by the plan, not by the geometry, and a few slots is the
	// whole of M1: sizing from len(cur.Layers) would grow the first time a plan arrived
	// and blame it on the frame path.
	reserve(&s.doneLay, maxInt(len(s.doneLay), 4), count, &s.scratch)
}

// maxCoords bounds what VisibleCoords and PrefetchCoords can return for the current
// surface: the visible span with a tile of slack on each axis, plus the ring bands on
// both sides.
func (s *Scheduler) maxCoords() int {
	ts := int(s.tileSize())
	if ts <= 0 {
		ts = int(frame.TileSize)
	}
	cols := maxInt(int(s.size.W)/ts+3, 4)
	rows := maxInt(int(s.size.H)/ts+3, 4)
	n := cols * rows
	if r := s.ringRows(); r > 0 {
		n += 2 * r * cols
	}
	return n
}

// reserve gives a retained buffer at least n capacity without touching its length,
// counting the growth when the caller is measuring rather than setting up.
func reserve[T any](buf *[]T, n int, count bool, grew *int64) {
	if n <= cap(*buf) {
		return
	}
	*buf = make([]T, 0, n)
	if count {
		*grew++
	}
}

// coordLess orders tile coordinates the way VisibleCoords emits them: row-major.
func coordLess(a, b frame.TileCoord) bool {
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.Col < b.Col
}

func intSign(v int32) int32 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

func addSaturate(a, b int32) int32 {
	sum := int64(a) + int64(b)
	if sum > math.MaxInt32 {
		return math.MaxInt32
	}
	if sum < math.MinInt32 {
		return math.MinInt32
	}
	return int32(sum)
}

// clampOffset confines a raw scroll position to [lo, lo+span]. A span below zero means
// the document is smaller than the surface in that axis, which clamps to the origin:
// there is no way to scroll a page that does not fill the window, and showing the void
// beside it is not an alternative.
func clampOffset(cur, lo, span int32) int32 {
	if span < 0 {
		span = 0
	}
	hi := lo + span
	switch {
	case cur < lo:
		return lo
	case cur > hi:
		return hi
	}
	return cur
}

func max32Of(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
