package raster

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/surface"
)

type Scheduler struct {
	layer *frame.Layer
	grid  *frame.Grid
	pool  *Pool
	slot  frame.Plan
	prefs Pref

	vpMu sync.RWMutex

	updMu   sync.Mutex
	pending *layerUpdate

	size   frame.Size
	scale  float32
	scroll frame.Point
	dir    frame.Point

	cur frame.FramePlan

	vis     []frame.TileCoord
	need    []frame.TileCoord
	pref    []frame.TileCoord
	blits   []surface.TileBlit
	damage  []frame.Rect
	prevVis []frame.TileCoord
	prevPix []*frame.Bitmap
	doneLay []*frame.Layer

	groupScratch *frame.BitmapPool

	haveDone   bool
	doneSize   frame.Size
	doneOffset frame.Point
	doneScale  float32
	doneBg     frame.Color

	backgroundSet     bool
	background        frame.Color
	backgroundVersion uint64

	reused        int
	failedVisible int
	reusedTotal   int64

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

var ErrNoViewport = errors.New("raster: scheduler has no surface size to scroll against")

var errForeignResult = errors.New("raster: result is for a different layer")

const PrefetchOff int32 = -1

type Pref struct {
	PrefetchRows int32
	Budget       int64
}

type Stats struct {
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
	Writes     int64
	Deferred   int64
}

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
		s.scroll = clampScroll(vp.Offset, l.Bounds, vp.Size)
		if prefs.Budget > 0 && s.grid != nil {
			s.grid.SetBudget(prefs.Budget)
		}
	}
	ts := int32(frame.TileSize)
	if l != nil && l.Grid != nil {
		if n := l.Grid.TileSize(); n > 0 {
			ts = n
		}
	}
	s.groupScratch = frame.NewBitmapPool(frame.Size{W: ts, H: ts}, 32)
	s.reserveScratch(false)
	return s
}

func (s *Scheduler) SetPlan(p frame.FramePlan) { s.slot.Publish(p) }

type layerUpdate struct {
	layer      *frame.Layer
	content    frame.Content
	dirty      frame.Rect
	autoDamage bool
}

func (s *Scheduler) UpdateLayer(l *frame.Layer, content frame.Content, dirty frame.Rect) bool {
	if l == nil || content == nil {
		return false
	}
	s.updMu.Lock()
	u := &layerUpdate{layer: l, content: content, dirty: dirty, autoDamage: dirty.Empty()}
	if s.pending != nil && s.pending.layer == l {
		u.dirty = u.dirty.Union(s.pending.dirty)
		u.autoDamage = u.autoDamage || s.pending.autoDamage
	}
	s.pending = u
	s.updMu.Unlock()
	return true
}

func (s *Scheduler) applyPendingUpdate() {
	s.updMu.Lock()
	u := s.pending
	s.updMu.Unlock()
	if u == nil {
		return
	}
	if s.layer == nil || s.grid == nil || u.layer != s.layer {
		s.clearPending(u)
		return
	}
	dirty := u.dirty
	if u.autoDamage {
		dirty = dirty.Union(paint.ContentDamage(s.layer.ContentSnapshot(), u.content))
	}
	if s.grid.InFlightIn(dirty) != 0 {
		return
	}
	u.layer.SetContent(u.content)
	u.layer.InvalidateRect(dirty)
	s.clearPending(u)
}

func (s *Scheduler) clearPending(u *layerUpdate) {
	s.updMu.Lock()
	if s.pending == u {
		s.pending = nil
	}
	s.updMu.Unlock()
}

func (s *Scheduler) PlanStats() frame.PlanStats { return s.slot.Stats() }

func (s *Scheduler) SetViewport(vp frame.Viewport) {
	s.vpMu.Lock()
	defer s.vpMu.Unlock()
	if !vp.Size.Empty() {
		s.size = vp.Size
	}
	s.scroll = vp.Offset
}

func (s *Scheduler) Viewport() frame.Viewport {
	s.vpMu.RLock()
	defer s.vpMu.RUnlock()
	return s.resolveLocked()
}

func (s *Scheduler) ScrollOffset() frame.Point {
	s.vpMu.RLock()
	defer s.vpMu.RUnlock()
	return s.scroll
}

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

func (s *Scheduler) GridStats() frame.GridStats {
	if s.grid == nil {
		return frame.GridStats{}
	}
	return s.grid.Stats()
}

func (s *Scheduler) ScratchAllocs() int64 { return s.scratch }

func (s *Scheduler) Damage() []frame.Rect { return s.damage }

func (s *Scheduler) BeginFrame(ev surface.Event) (*frame.FramePlan, []frame.TileCoord) {
	s.frames++
	s.vpMu.Lock()
	defer s.vpMu.Unlock()
	s.fold(ev.Delta)
	if !ev.Size.Empty() {
		s.size = ev.Size
	}
	if ev.Scale > 0 {
		s.scale = ev.Scale
	}

	s.vis, s.need, s.pref = s.vis[:0], s.need[:0], s.pref[:0]
	s.reused = 0
	s.failedVisible = 0

	p, ok := s.slot.Take()
	if !ok {
		s.cur = frame.FramePlan{}
		return nil, nil
	}
	if len(p.Layers) > 0 && p.Layers[0] != s.layer {
		s.layer = p.Layers[0]
		s.grid = s.layer.Grid
		s.haveDone = false
		s.backgroundSet = false
	}
	vp := s.resolveLocked()
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
	if !s.backgroundSet || s.background != p.Background {
		s.layer.InvalidateBackground()
		s.backgroundVersion = s.layer.BackgroundVersion()
		s.background = p.Background
		s.backgroundSet = true
	}
	s.applyPendingUpdate()
	s.reserveScratch(true)
	s.grid.Tick()

	s.vis = s.grid.VisibleCoords(vp, s.vis)
	backward := s.dir.Y > 0 || (s.dir.Y == 0 && s.dir.X > 0)
	for i := range s.vis {
		j := i
		if backward {
			j = len(s.vis) - 1 - i
		}
		c := s.vis[j]
		s.grid.Touch(c)
		if !s.layer.Needs(c) {
			s.reused++
			s.reusedTotal++
			continue
		}
		if s.grid.InFlight(c) {
			continue
		}
		if s.grid.Failed(c) {
			s.failedVisible++
			continue
		}
		s.need = append(s.need, c)
	}
	s.prefetched += int64(s.ring(vp))
	return &s.cur, s.need
}

func (s *Scheduler) Submit(needed []frame.TileCoord) surface.FrameWork {
	w := surface.FrameWork{Needed: len(needed), Reused: s.reused, Failed: s.failedVisible}
	if s.grid == nil || s.pool == nil || s.layer == nil {
		return w
	}
	dl, _ := s.layer.ContentSnapshot().(*paint.LayerDL)
	budget := s.grid.Stats().Budget
	tileBytes := s.tileBytes()
	ts := s.grid.TileSize()
	painting := s.grid.Stats().PaintingBytes
	w.PaintingBytes = painting

	for i, c := range needed {
		if painting+tileBytes > budget {
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
		if err := s.pool.Submit(Job{Layer: s.layer, Coord: c, DL: dl, Bounds: c.Rect(ts), Out: out, Background: s.cur.Background, BackgroundVersion: s.backgroundVersion, Scratch: s.groupScratch}); err != nil {
			s.grid.Release(c, out)
			painting -= tileBytes
			w.Refused++
			s.refused++
		}
	}

	for {
		d, ok := s.pool.Poll()
		if !ok {
			break
		}
		before := s.rasterized
		_ = s.HandleDone(d)
		if s.rasterized > before {
			w.Accepted++
		}
	}
	return w
}

func (s *Scheduler) ComposeInto(backing *frame.Bitmap, c *surface.Composer, plan *frame.FramePlan, needed []frame.TileCoord) []frame.Rect {
	s.damage = s.damage[:0]
	s.blits = s.blits[:0]
	if backing == nil || c == nil || plan == nil {
		return nil
	}
	c.ReserveDamage(cap(s.damage))
	if s.grid == nil {
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
			full, dy = true, 0
		}
	}
	switch {
	case full:
		s.damage = append(s.damage, backing.Bounds())
	case dy > 0:
		c.ShiftY(dy, nil)
		s.damage = append(s.damage, frame.Rect4(0, sz.H-dy, sz.W, sz.H))
	case dy < 0:
		c.ShiftY(dy, nil)
		s.damage = append(s.damage, frame.Rect4(0, 0, sz.W, -dy))
	default:
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

func (s *Scheduler) HandleDone(d Done) error {
	if s.grid == nil || s.layer == nil {
		return nil
	}
	if d.Layer != nil && d.Layer.Grid != nil && d.Layer.Grid != s.grid {
		d.Layer.Grid.Release(d.Coord, d.Out)
		s.staleNews++
		return errForeignResult
	}
	if d.LayerID != s.layer.ID {
		if d.Layer != nil && d.Layer.Grid != nil {
			d.Layer.Grid.Release(d.Coord, d.Out)
		}
		s.staleNews++
		return errForeignResult
	}
	if d.BackgroundVersion != s.backgroundVersion {
		s.grid.Release(d.Coord, d.Out)
		s.staleNews++
		return nil
	}
	if d.Err != nil {
		s.grid.Release(d.Coord, d.Out)
		if s.grid.Attempts(d.Coord) >= frame.MaxTileAttempts {
			s.grid.MarkFailed(d.Coord)
			s.failed++
		}
		return d.Err
	}
	if d.Version < s.layer.VersionSnapshot() {
		s.grid.Release(d.Coord, d.Out)
		s.staleNews++
		return nil
	}
	if s.grid.MarkValid(d.Coord, d.Version, d.Out) {
		s.rasterized++
		return nil
	}
	s.grid.Release(d.Coord, d.Out)
	s.staleNews++
	return nil
}

func (s *Scheduler) SubmitScroll(delta frame.Point, at time.Time) error {
	s.vpMu.Lock()
	defer s.vpMu.Unlock()
	if s.size.Empty() {
		return ErrNoViewport
	}
	s.fold(delta)
	return nil
}

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

func (s *Scheduler) resolveLocked() frame.Viewport {
	v := frame.Viewport{Offset: s.scroll, Size: s.size}
	if s.layer == nil {
		return v
	}
	b := s.layer.Bounds
	v.Offset = clampScroll(v.Offset, b, v.Size)
	return v
}

func (s *Scheduler) ring(vp frame.Viewport) int {
	rows := s.ringRowsFor(s.dir)
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

func (s *Scheduler) ringRowsFor(dir frame.Point) int {
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
	span := s.size.H
	if dir.Y == 0 && dir.X != 0 {
		span = s.size.W
	}
	return int(span/ts) + 1
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

func (s *Scheduler) reserveScratch(count bool) {
	n := s.maxCoords()
	reserve(&s.vis, n, count, &s.scratch)
	reserve(&s.need, n, count, &s.scratch)
	reserve(&s.pref, n, count, &s.scratch)
	reserve(&s.blits, n, count, &s.scratch)
	reserve(&s.damage, n+1, count, &s.scratch)
	reserve(&s.prevVis, n, count, &s.scratch)
	reserve(&s.prevPix, n, count, &s.scratch)
	reserve(&s.doneLay, maxInt(len(s.doneLay), 4), count, &s.scratch)
}

func (s *Scheduler) maxCoords() int {
	ts := int(s.tileSize())
	if ts <= 0 {
		ts = int(frame.TileSize)
	}
	cols := maxInt(int(s.size.W)/ts+3, 4)
	rows := maxInt(int(s.size.H)/ts+3, 4)
	n := cols * rows
	verticalRing := s.ringRowsFor(frame.Point{Y: 1})
	horizontalRing := s.ringRowsFor(frame.Point{X: 1})
	n += maxInt(2*verticalRing*cols, 2*horizontalRing*rows)
	return n
}

func reserve[T any](buf *[]T, n int, count bool, grew *int64) {
	if n <= cap(*buf) {
		return
	}
	*buf = make([]T, 0, n)
	if count {
		*grew++
	}
}

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

func clampScroll(p frame.Point, b frame.Rect, size frame.Size) frame.Point {
	loX, loY := min(int32(0), b.X0), min(int32(0), b.Y0)
	return frame.Point{
		X: clampOffset(p.X, loX, b.X1-size.W-loX),
		Y: clampOffset(p.Y, loY, b.Y1-size.H-loY),
	}
}

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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
