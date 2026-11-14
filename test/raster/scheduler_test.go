package raster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

func schedColor(c frame.TileCoord, version uint64) frame.Color {
	return frame.RGB(byte(version*37+11), byte(c.Col*17+5), byte(c.Row*23+3))
}

func schedRaster(j raster.Job) error {
	if j.Out == nil {
		return errors.New("test: job carried no buffer")
	}
	var v uint64
	if j.DL != nil {
		v = j.DL.Version()
	}
	j.Out.Reset()
	j.Out.FillRect(j.Out.Bounds(), schedColor(j.Coord, v), nil)
	return nil
}

func schedDoc(t *testing.T, cols, rows int32, budget int64) (*frame.Layer, *paint.LayerDL) {
	t.Helper()
	bounds := frame.Rect4(0, 0, cols*frame.TileSize, rows*frame.TileSize)
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, 4096)
	l := frame.NewLayer(frame.LayerID(7), bounds, budget*frame.TileSizeBytes(), pool)
	list := paint.NewList(1)
	list.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: bounds, Color: frame.RGB(200, 200, 200)})
	dl := list.Build(1).Publish()
	l.SetContent(dl)
	return l, dl
}

var testBackground = frame.RGB(9, 9, 9)

type schedHarness struct {
	s    *raster.Scheduler
	l    *frame.Layer
	pool *raster.Pool
	c    *surface.Composer
	size frame.Size
}

func newSched(t *testing.T, cols, rows int32, budget int64, vp frame.Size, prefs raster.Pref) *schedHarness {
	t.Helper()
	return newSchedVP(t, cols, rows, budget, frame.Viewport{Size: vp}, prefs)
}

func newSchedVP(t *testing.T, cols, rows int32, budget int64, vp frame.Viewport, prefs raster.Pref) *schedHarness {
	t.Helper()
	return newSchedRaster(t, cols, rows, budget, vp, prefs, schedRaster)
}

func newSchedRaster(t *testing.T, cols, rows int32, budget int64, vp frame.Viewport, prefs raster.Pref, fn raster.RasterFunc) *schedHarness {
	t.Helper()
	l, _ := schedDoc(t, cols, rows, budget)
	p := raster.New(2, 64, fn)
	p.Start(context.Background())
	t.Cleanup(func() { p.Close() })
	c := surface.NewComposer(vp.Size, frame.NewBitmapPool(vp.Size, 8))
	s := raster.NewScheduler(l, p, vp, 1, prefs)
	s.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{l}, Background: testBackground})
	return &schedHarness{s: s, l: l, pool: p, c: c, size: vp.Size}
}

func vsync(dy int32) surface.Event {
	return surface.Event{Kind: surface.EvVsync, Delta: frame.Point{Y: dy}, At: time.Now()}
}

func (h *schedHarness) frame(t *testing.T, ev surface.Event) surface.FrameWork {
	t.Helper()
	plan, needed := h.s.BeginFrame(ev)
	if plan == nil {
		t.Fatal("BeginFrame returned no plan after SetPlan")
	}
	h.c.Resize(plan.Viewport.Size)
	w := h.s.Submit(needed)
	h.s.ComposeInto(h.c.Backing, h.c, plan, needed)
	return w
}

func (h *schedHarness) scroll(t *testing.T, dy int32) surface.FrameWork {
	t.Helper()
	return h.frame(t, vsync(dy))
}

func (h *schedHarness) pump() surface.FrameWork {
	_, needed := h.s.BeginFrame(vsync(0))
	return h.s.Submit(needed)
}

func (h *schedHarness) drain(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for h.l.Grid.Stats().PaintingBytes > 0 {
		for {
			d, ok := h.pool.Poll()
			if !ok {
				break
			}
			_ = h.s.HandleDone(d)
		}
		if h.l.Grid.Stats().PaintingBytes == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("raster never settled: %d painting bytes left", h.l.Grid.Stats().PaintingBytes)
		}
		time.Sleep(time.Millisecond)
	}
	for {
		d, ok := h.pool.Poll()
		if !ok {
			break
		}
		_ = h.s.HandleDone(d)
	}
}

func (h *schedHarness) untilQuiet(t *testing.T) {
	t.Helper()
	if !pollUntil(h.pumpUntilQuiet) {
		t.Fatalf("raster never settled: %+v", h.l.Stats())
	}
}

func (h *schedHarness) pumpUntilQuiet() bool {
	w := h.pump()
	return w.Needed == 0 && h.l.Grid.Stats().PaintingBytes == 0
}

func (h *schedHarness) setOffset(t *testing.T, y int32) {
	t.Helper()
	h.frame(t, vsync(y-h.s.Viewport().Offset.Y))
	if got := h.s.Viewport().Offset.Y; got != y {
		t.Fatalf("setOffset(%d) landed at %d", y, got)
	}
}

func (h *schedHarness) warm(t *testing.T) {
	t.Helper()
	vp := h.c.Backing.Bounds()
	b := h.l.Bounds
	for y := int32(0); y <= b.Y1-vp.H(); y += frame.TileSize {
		h.setOffset(t, y)
		h.untilQuiet(t)
	}
	h.setOffset(t, 0)
	h.untilQuiet(t)
}

func TestScrollPathIsAllocationFree(t *testing.T) {
	const (
		cols, rows = 4, 20
		vp         = 4 * frame.TileSize
	)
	h := newSched(t, cols, rows, int64(cols*rows), frame.Size{W: vp, H: vp}, raster.Pref{})
	h.warm(t)
	if got := h.l.Stats().Valid; got != cols*rows {
		t.Fatalf("setup: %d tiles valid, want the whole %d tile document warm", got, cols*rows)
	}
	before := h.s.Stats()

	maxOffset := rows*frame.TileSize - vp
	leg := int(maxOffset/100) + 1
	i := 0
	tick := func() {
		dy := int32(100)
		if (i/leg)%2 == 1 {
			dy = -100
		}
		h.frame(t, vsync(dy))
		i++
	}
	for j := 0; j < 600; j++ {
		tick()
	}
	if got := h.s.Stats().Rasterized; got != before.Rasterized {
		t.Fatalf("a warm cache rasterized %d more tiles; a scroll changed a content version", got-before.Rasterized)
	}
	if got := h.s.ScratchAllocs(); got != 0 {
		t.Fatalf("ScratchAllocs() = %d, want 0: every scratch buffer is sized before the frame path runs", got)
	}
	if n := testing.AllocsPerRun(600, tick); n != 0 {
		t.Fatalf("allocs/op = %v on a warm scroll frame, want 0", n)
	}
	if got := h.s.Stats().Bytes; got != int64(cols*rows)*frame.TileSizeBytes() {
		t.Fatalf("Bytes = %d, want the warm document's own %d", got, int64(cols*rows)*frame.TileSizeBytes())
	}
}

func TestPrefetchAheadOfScrollDirection(t *testing.T) {
	const (
		cols, rows = 2, 40
		vp         = 2 * frame.TileSize
	)
	h := newSchedVP(t, cols, rows, 200, frame.Viewport{
		Offset: frame.Point{Y: rows * frame.TileSize / 2},
		Size:   frame.Size{W: vp, H: vp},
	}, raster.Pref{PrefetchRows: 2})
	plan, needed := h.s.BeginFrame(vsync(100))
	if plan == nil {
		t.Fatal("no plan")
	}
	r := plan.Viewport.Rect()
	firstRow, lastRow := r.Y0/frame.TileSize, (r.Y1-1)/frame.TileSize

	firstBelow, firstAbove, lastAbove := -1, -1, -1
	for i, c := range needed {
		switch {
		case c.Row > lastRow:
			if firstBelow < 0 {
				firstBelow = i
			}
		case c.Row < firstRow:
			if firstAbove < 0 {
				firstAbove = i
			}
			lastAbove = i
		}
	}
	if firstBelow < 0 {
		t.Fatal("scrolling down queued nothing below the viewport: the leading prefetch ring is missing")
	}
	if firstAbove < 0 {
		t.Fatal("scrolling down queued nothing above the viewport: the trailing ring is missing")
	}
	if lastAbove < firstBelow {
		t.Fatalf("the whole leading band (index %d) is not ahead of the trailing one (index %d)", firstBelow, lastAbove)
	}
	if got := h.s.Stats().Prefetched; got == 0 {
		t.Fatal("Stats().Prefetched = 0 while off-viewport tiles were queued")
	}

	off := newSchedVP(t, cols, rows, 200, frame.Viewport{
		Offset: frame.Point{Y: rows * frame.TileSize / 2},
		Size:   frame.Size{W: vp, H: vp},
	}, raster.Pref{PrefetchRows: raster.PrefetchOff})
	plan2, need2 := off.s.BeginFrame(vsync(100))
	if plan2 == nil {
		t.Fatal("no plan")
	}
	vis := off.l.VisibleCoords(plan2.Viewport, nil)
	if len(need2) == 0 {
		t.Fatal("setup: the cold viewport queued no visible tiles")
	}
	for _, c := range need2 {
		if !containsCoord(vis, c) {
			t.Fatalf("coord %v is outside the viewport but was queued with the ring disabled", c)
		}
	}
	if got := off.s.Stats().Prefetched; got != 0 {
		t.Fatalf("Prefetched = %d with the ring disabled, want 0", got)
	}
}

func TestDefaultVerticalPrefetchSettlesWithinLandscapeCacheBudget(t *testing.T) {
	const (
		cols       = 12
		rows       = 40
		viewRows   = 6
		cacheTiles = 256
	)
	vp := frame.Size{W: cols * frame.TileSize, H: viewRows * frame.TileSize}
	h := newSchedVP(t, cols, rows, cacheTiles, frame.Viewport{
		Offset: frame.Point{Y: 16 * frame.TileSize},
		Size:   vp,
	}, raster.Pref{})

	h.untilQuiet(t)
	if work := h.pump(); work.Needed != 0 {
		t.Fatalf("default prefetch did not settle: %d tiles still requested", work.Needed)
	}
}

func TestBudgetPlateaus(t *testing.T) {
	const (
		cols, rows = 4, 79
		vp         = 4 * frame.TileSize
		budget     = 20
	)
	limit := int64(budget) * frame.TileSizeBytes()
	h := newSched(t, cols, rows, int64(cols*rows), frame.Size{W: vp, H: vp}, raster.Pref{PrefetchRows: 1, Budget: limit})
	if got := h.s.Stats().Budget; got != limit {
		t.Fatalf("Budget = %d, want Pref.Budget's %d applied to the grid", got, limit)
	}
	for i := 0; i < 600; i++ {
		h.frame(t, vsync(20000/600))
		if got := h.l.Stats().Bytes; got > limit {
			t.Fatalf("frame %d: tile bytes %d exceeded the budget %d", i, got, limit)
		}
	}
	st := h.l.Stats()
	if st.Evictions == 0 {
		t.Fatal("no evictions across 20,000px of scroll: the budget never bit")
	}
	if st.Rasterized == 0 {
		t.Fatal("nothing rasterized while scrolling into cold content")
	}
	if st.Bytes*10 < limit*9 {
		t.Fatalf("Bytes = %d after a long cold scroll, want it held at the plateau of %d", st.Bytes, limit)
	}
	if got := h.pool.Stats().DroppedResults; got != 0 {
		t.Fatalf("DroppedResults = %d; a lost result is a leaked tile buffer", got)
	}
	if got := h.s.Stats().Deferred; got == 0 {
		t.Fatal("Deferred = 0 under a budget that bit; work was submitted that could not fit")
	}
}

// qc:BR-174
func TestEvictedTileIsRequeued(t *testing.T) {
	const (
		cols, rows = 2, 16
		vp         = 2 * frame.TileSize
		budget     = 4
		dist       = 12 * frame.TileSize
	)
	h := newSched(t, cols, rows, budget, frame.Size{W: vp, H: vp}, raster.Pref{PrefetchRows: raster.PrefetchOff})
	h.untilQuiet(t)
	c := frame.TileCoord{Col: 0, Row: 0}
	first := h.l.Grid.Pixels(c)
	if first == nil {
		t.Fatal("setup: the top-left tile never got pixels")
	}
	want := first.At(3, 3)
	rasterized := h.s.Stats().Rasterized

	h.scroll(t, dist)
	h.untilQuiet(t)
	if h.l.Grid.Pixels(c) != nil {
		t.Fatalf("setup: the tile survived a %dpx scroll; the budget is not biting", dist)
	}
	if got := h.l.Stats().Evictions; got == 0 {
		t.Fatal("no evictions with a budget this tight over a 32-tile document")
	}

	h.scroll(t, -dist)
	h.untilQuiet(t)
	back := h.l.Grid.Pixels(c)
	if back == nil {
		t.Fatalf("the evicted tile never came back; Rasterized went %d -> %d", rasterized, h.s.Stats().Rasterized)
	}
	if got := h.s.Stats().Rasterized; got <= rasterized {
		t.Fatal("the requeue was not counted as a raster")
	}
	if got := back.At(3, 3); got != want {
		t.Fatalf("requeued tile pixel = %v, want %v: the redraw disagrees with the original", got, want)
	}
}

// qc:BR-172
func TestFailedTileRendersBlankWithCounter(t *testing.T) {
	const (
		cols, rows = 2, 8
		vp         = 2 * frame.TileSize
	)
	bad := frame.TileCoord{Col: 1, Row: 1}
	var mu sync.Mutex
	var attempts int
	draw := func(j raster.Job) error {
		if j.Coord == bad {
			mu.Lock()
			attempts++
			mu.Unlock()
			return errors.New("test: this tile cannot be drawn")
		}
		return schedRaster(j)
	}
	h := newSchedRaster(t, cols, rows, 32, frame.Viewport{Size: frame.Size{W: vp, H: vp}}, raster.Pref{PrefetchRows: raster.PrefetchOff}, draw)

	for i := 0; i < frame.MaxTileAttempts*8 && h.s.Stats().Failed == 0; i++ {
		h.frame(t, vsync(0))
		h.drain(t)
	}
	mu.Lock()
	attemptsAfterFailure := attempts
	mu.Unlock()
	if !h.l.Grid.Failed(bad) {
		t.Fatalf("tile %v was never given up on after %d attempts", bad, attemptsAfterFailure)
	}
	if attemptsAfterFailure > frame.MaxTileAttempts {
		t.Fatalf("the rasterizer ran %d times for one tile, want at most MaxTileAttempts (%d)", attemptsAfterFailure, frame.MaxTileAttempts)
	}
	if st := h.s.Stats(); st.Failed == 0 {
		t.Fatal("Stats().Failed is zero while a tile is marked failed; the gate needs the counter")
	}
	if h.l.Grid.Blittable(bad) {
		t.Fatal("a failed tile reported pixels worth compositing")
	}
	px := h.c.Backing.At(int(bad.Col*frame.TileSize)+7, int(bad.Row*frame.TileSize)+7)
	if px != testBackground {
		t.Fatalf("pixel over the failed tile = %v, want the background %v", px, testBackground)
	}

	stopped := attemptsAfterFailure
	for i := 0; i < 5; i++ {
		_, needed := h.s.BeginFrame(vsync(0))
		for _, c := range needed {
			if c == bad {
				t.Fatal("a failed tile was queued again after it was given up on")
			}
		}
		h.frame(t, vsync(0))
	}
	mu.Lock()
	if attempts != stopped {
		t.Fatalf("the failed tile was rasterized %d more times after being given up on", attempts-stopped)
	}
	mu.Unlock()

	h.l.Bump(2, bad.Rect(frame.TileSize))
	_, needed := h.s.BeginFrame(vsync(0))
	if !containsCoord(needed, bad) {
		t.Fatal("a version bump covering a failed tile did not put it back in the queue")
	}
}

// qc:BR-172
func TestVisibleFailedTileIsReportedInFrameWork(t *testing.T) {
	const (
		cols, rows = 2, 8
		vp         = 2 * frame.TileSize
	)
	bad := frame.TileCoord{Col: 1, Row: 1}
	draw := func(j raster.Job) error {
		if j.Coord == bad {
			return errors.New("test: this tile cannot be drawn")
		}
		return schedRaster(j)
	}
	h := newSchedRaster(t, cols, rows, 32, frame.Viewport{Size: frame.Size{W: vp, H: vp}}, raster.Pref{PrefetchRows: raster.PrefetchOff}, draw)

	for i := 0; i < frame.MaxTileAttempts*8 && h.s.Stats().Failed == 0; i++ {
		h.frame(t, vsync(0))
		h.drain(t)
	}
	if !h.l.Grid.Failed(bad) {
		t.Fatalf("tile %v was never given up on", bad)
	}

	w := h.frame(t, vsync(0))
	h.drain(t)
	if w.Failed != 1 {
		t.Fatalf("FrameWork.Failed = %d, want 1 for the one visible failed tile", w.Failed)
	}
	if w.Needed != 0 {
		t.Fatalf("FrameWork.Needed = %d after the visible set settled, want 0", w.Needed)
	}
}

func TestFailedTileOutsideViewportIsNotReportedInFrameWork(t *testing.T) {
	const (
		cols, rows = 2, 8
		vp         = 2 * frame.TileSize
	)
	bad := frame.TileCoord{Col: 0, Row: 0}
	draw := func(j raster.Job) error {
		if j.Coord == bad {
			return errors.New("test: this tile cannot be drawn")
		}
		return schedRaster(j)
	}
	h := newSchedRaster(t, cols, rows, 32, frame.Viewport{Size: frame.Size{W: vp, H: vp}}, raster.Pref{PrefetchRows: raster.PrefetchOff}, draw)

	for i := 0; i < frame.MaxTileAttempts*8 && h.s.Stats().Failed == 0; i++ {
		h.frame(t, vsync(0))
		h.drain(t)
	}
	if !h.l.Grid.Failed(bad) {
		t.Fatalf("tile %v was never given up on", bad)
	}
	if w := h.frame(t, vsync(0)); w.Failed != 1 {
		t.Fatalf("FrameWork.Failed = %d while the failed tile is visible, want 1", w.Failed)
	}
	h.drain(t)

	h.setOffset(t, 6*frame.TileSize)
	w := h.frame(t, vsync(0))
	h.drain(t)
	if got := h.s.Stats().Failed; got == 0 {
		t.Fatal("Stats().Failed = 0, want the out-of-view failure retained")
	}
	if w.Failed != 0 {
		t.Fatalf("FrameWork.Failed = %d for an off-screen failure, want 0", w.Failed)
	}
}

// qc:BR-174
func TestScrollDamagesOnlyTheExposedBand(t *testing.T) {
	const (
		cols, rows = 4, 12
		vp         = 4 * frame.TileSize
		dy         = 100
	)
	h := newSched(t, cols, rows, int64(cols*rows), frame.Size{W: vp, H: vp}, raster.Pref{PrefetchRows: raster.PrefetchOff})
	h.warm(t)
	writes := h.s.Stats().Writes

	plan, needed := h.s.BeginFrame(vsync(dy))
	if plan == nil {
		t.Fatal("no plan")
	}
	h.c.Resize(plan.Viewport.Size)
	h.s.Submit(needed)
	damage := h.s.ComposeInto(h.c.Backing, h.c, plan, needed)

	want := frame.Rect4(0, vp-dy, vp, vp)
	if len(damage) != 1 || damage[0] != want {
		t.Fatalf("damage = %v, want the exposed band %v", damage, want)
	}
	if got := h.s.Damage(); len(got) != 1 || got[0] != want {
		t.Fatalf("Damage() = %v, want the band the frame composed", got)
	}
	off := plan.Viewport.Offset.Y
	owner := frame.CoordFor(frame.Point{X: 4, Y: off + 4}, frame.TileSize)
	if got := h.c.Backing.At(4, 4); got != schedColor(owner, 1) {
		t.Fatalf("pixel (4,4) = %v, want tile %v's %v: the shift landed in the wrong place", got, owner, schedColor(owner, 1))
	}
	used := h.s.Stats().Writes - writes
	full := int64(vp) * int64(vp)
	if used == 0 || used > full/4 {
		t.Fatalf("pixel writes = %d for a %dpx scroll of a %dpx surface, want far under a recompose", used, dy, full)
	}
}

// qc:BR-079
func TestLongPageScrollRasterizesOnlyTheExposedRowAndReusesTheRest(t *testing.T) {
	const (
		cols, rows = 4, 24
		vp         = cols * frame.TileSize
		step       = frame.TileSize
	)
	h := newSched(t, cols, rows, int64(cols*rows), frame.Size{W: vp, H: vp}, raster.Pref{PrefetchRows: raster.PrefetchOff})
	h.untilQuiet(t)

	scrollOneRow := func(dy int32) surface.FrameWork {
		t.Helper()
		before := h.s.Stats().Rasterized
		plan, needed := h.s.BeginFrame(vsync(dy))
		if plan == nil {
			t.Fatal("no plan")
		}
		h.c.Resize(plan.Viewport.Size)
		w := h.s.Submit(needed)
		h.s.ComposeInto(h.c.Backing, h.c, plan, needed)
		if len(needed) != cols {
			t.Fatalf("scroll of %d asked for %d tiles, want exactly the exposed row of %d", dy, len(needed), cols)
		}
		wantRow := (plan.Viewport.Offset.Y + vp - 1) / frame.TileSize
		if dy < 0 {
			wantRow = plan.Viewport.Offset.Y / frame.TileSize
		}
		for _, c := range needed {
			if c.Row != wantRow {
				t.Fatalf("scroll of %d rasterized tile row %d, want the exposed row %d", dy, c.Row, wantRow)
			}
		}
		if w.Reused == 0 {
			t.Fatalf("scroll of %d reused no warm tiles", dy)
		}
		h.untilQuiet(t)
		if got := h.s.Stats().Rasterized - before; got != int64(cols) {
			t.Fatalf("scroll of %d rasterized %d tiles, want only the %d exposed ones", dy, got, cols)
		}
		return w
	}

	lastY := int32(rows-cols) * frame.TileSize
	for y := int32(0); y < lastY; y += step {
		scrollOneRow(step)
		c := frame.CoordFor(frame.Point{X: 4, Y: h.s.Viewport().Offset.Y + step + 4}, frame.TileSize)
		if px := h.l.Grid.Pixels(c); px == nil || px.At(3, 3) != schedColor(c, 1) {
			t.Fatalf("tile %v lost its cached pixels at offset %d", c, h.s.Viewport().Offset.Y)
		}
	}
}

func TestSchedulerOwnsTheViewportAndClampsScroll(t *testing.T) {
	const (
		cols, rows = 2, 8
		vp         = 2 * frame.TileSize
	)
	h := newSched(t, cols, rows, 32, frame.Size{W: vp, H: vp}, raster.Pref{PrefetchRows: raster.PrefetchOff})
	if got := h.s.Viewport().Offset.Y; got != 0 {
		t.Fatalf("initial offset = %d, want 0", got)
	}
	h.scroll(t, 1<<20)
	limit := rows*frame.TileSize - vp
	if got := h.s.Viewport().Offset.Y; got != limit {
		t.Fatalf("offset after a 1Mpx scroll = %d, want clamped to %d", got, limit)
	}
	h.scroll(t, -(1 << 20))
	if got := h.s.Viewport().Offset.Y; got != 0 {
		t.Fatalf("offset after scrolling past the top = %d, want 0", got)
	}

	h.s.SetViewport(frame.Viewport{Offset: frame.Point{Y: 600}, Size: h.size})
	if got := h.s.Viewport().Offset.Y; got != 600 {
		t.Fatalf("SetViewport ignored: offset = %d, want 600", got)
	}
	bigger := frame.Size{W: vp, H: vp + frame.TileSize}
	plan, needed := h.s.BeginFrame(surface.Event{Kind: surface.EvVsync, Size: bigger, At: time.Now()})
	if plan.Viewport.Size != bigger {
		t.Fatalf("resize not adopted: size = %v, want %v", plan.Viewport.Size, bigger)
	}
	plan, needed = h.s.BeginFrame(vsync(0))
	h.c.Resize(plan.Viewport.Size)
	h.s.Submit(needed)
	damage := h.s.ComposeInto(h.c.Backing, h.c, plan, needed)
	if len(damage) != 1 || damage[0] != h.c.Backing.Bounds() {
		t.Fatalf("damage after a resize = %v, want the whole surface", damage)
	}
}

func TestHandleDoneIgnoresAResultForAnOldVersion(t *testing.T) {
	const (
		cols, rows = 2, 4
		size       = frame.TileSize
	)
	c0 := frame.TileCoord{Col: 0, Row: 0}
	l, dl := schedDoc(t, cols, rows, 32)

	p0 := raster.New(2, 8, schedRaster)
	p0.Start(context.Background())
	t.Cleanup(func() { p0.Close() })
	out, ok := l.Grid.Acquire(c0)
	if !ok {
		t.Fatal("setup: Acquire refused the only tile")
	}
	if err := p0.Submit(raster.Job{Layer: l, Coord: c0, DL: dl, Bounds: c0.Rect(frame.TileSize), Out: out}); err != nil {
		t.Fatalf("setup: Submit: %v", err)
	}
	for _, d := range collect(t, p0, 1) {
		if !l.Grid.MarkValid(d.Coord, d.Version, d.Out) {
			t.Fatal("setup: MarkValid refused the current result")
		}
	}

	gate := make(chan struct{})
	var once sync.Once
	draw := func(j raster.Job) error {
		once.Do(func() { <-gate })
		return schedRaster(j)
	}
	p := raster.New(1, 64, draw)
	p.Start(context.Background())
	t.Cleanup(func() { p.Close() })

	s := raster.NewScheduler(l, p, frame.Viewport{Size: frame.Size{W: size, H: size}}, 1, raster.Pref{PrefetchRows: raster.PrefetchOff})
	s.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{l}, Background: testBackground})

	if got := l.Bump(2, l.Bounds); got == 0 {
		t.Fatal("setup: the bump staled nothing; the warm tile never had pixels")
	}
	plan, needed := s.BeginFrame(vsync(0))
	if plan == nil {
		t.Fatal("no plan")
	}
	if len(needed) != 1 || needed[0] != c0 {
		t.Fatalf("needed = %v, want exactly the one stale tile", needed)
	}
	s.Submit(needed)
	if got := s.Stats().Rasterized; got != 0 {
		t.Fatalf("Rasterized = %d before the worker was even released", got)
	}
	close(gate)
	d := collect(t, p, 1)[0]
	if d.Version != 1 {
		t.Fatalf("Done.Version = %d, want the 1 the job was submitted against", d.Version)
	}
	if err := s.HandleDone(d); err != nil {
		t.Fatalf("HandleDone: %v", err)
	}
	if got := s.Stats().Rasterized; got != 0 {
		t.Fatalf("Rasterized = %d; a result for a superseded version was counted as installed", got)
	}
	if !l.Needs(d.Coord) {
		t.Fatal("the tile the newer version needs was marked current by an older raster")
	}
	if got := l.Grid.Stats().PaintingBytes; got != 0 {
		t.Fatalf("PaintingBytes = %d after a refused result; its buffer never went back to the pool", got)
	}
	if got := s.Stats().StaleNews; got == 0 {
		t.Fatal("a refused result was not counted; the gate needs to see stale news")
	}
	_, needed = s.BeginFrame(vsync(0))
	if !containsCoord(needed, d.Coord) {
		t.Fatalf("the superseded tile %v was not requeued for the current version", d.Coord)
	}
}

func TestSetPlanSupersedesAndKeepsTheScrollPosition(t *testing.T) {
	const (
		cols, rows = 2, 8
		vp         = 2 * frame.TileSize
	)
	h := newSched(t, cols, rows, 32, frame.Size{W: vp, H: vp}, raster.Pref{PrefetchRows: raster.PrefetchOff})
	h.scroll(t, 500)
	h.s.SetPlan(frame.FramePlan{Serial: 2, Background: testBackground})
	h.s.SetPlan(frame.FramePlan{Serial: 3, Layers: []*frame.Layer{h.l}, Background: testBackground})
	plan, _ := h.s.BeginFrame(vsync(0))
	if plan.Serial != 3 {
		t.Fatalf("plan serial = %d, want the newest published 3", plan.Serial)
	}
	if got := plan.Viewport.Offset.Y; got != 500 {
		t.Fatalf("a new plan moved the scroll to %d, want the 500 the input asked for", got)
	}
	if ps := h.s.PlanStats(); ps.Dropped != 1 {
		t.Fatalf("PlanStats.Dropped = %d, want the one superseded publish counted", ps.Dropped)
	}
}

func TestPublishingFromAnotherGoroutineIsRaceClean(t *testing.T) {
	const (
		cols, rows = 2, 8
		vp         = 2 * frame.TileSize
	)
	h := newSched(t, cols, rows, 32, frame.Size{W: vp, H: vp}, raster.Pref{})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			h.s.SetPlan(frame.FramePlan{Serial: uint64(i + 1), Layers: []*frame.Layer{h.l}, Background: testBackground})
		}
	}()
	for i := 0; i < 200; i++ {
		h.frame(t, vsync(50))
	}
	close(stop)
	wg.Wait()
	if got := h.s.Stats().Frames; got < 200 {
		t.Fatalf("Frames = %d, want every drawn frame counted", got)
	}
}

func TestSubmitScrollAndEventDeltasFoldIdentically(t *testing.T) {
	const (
		cols, rows = 2, 8
		vp         = 2 * frame.TileSize
	)
	a := newSched(t, cols, rows, 32, frame.Size{W: vp, H: vp}, raster.Pref{PrefetchRows: 1})
	b := newSched(t, cols, rows, 32, frame.Size{W: vp, H: vp}, raster.Pref{PrefetchRows: 1})
	for i := 0; i < 30; i++ {
		a.frame(t, vsync(37))
		if err := b.s.SubmitScroll(frame.Point{Y: 37}, time.Now()); err != nil {
			t.Fatalf("SubmitScroll: %v", err)
		}
		b.frame(t, vsync(0))
	}
	if got, want := b.s.Viewport().Offset, a.s.Viewport().Offset; got != want {
		t.Fatalf("SubmitScroll path offset = %v, the event path gave %v", got, want)
	}
	if got, want := b.s.Stats().Frames, a.s.Stats().Frames; got != want {
		t.Fatalf("frames = %d vs %d; only BeginFrame should draw", got, want)
	}
	if err := a.s.SubmitScroll(frame.Point{}, time.Now()); err != nil {
		t.Fatalf("SubmitScroll with no delta: %v", err)
	}
	a.frame(t, vsync(0))
	if got := a.s.Stats().Prefetched; got == 0 {
		t.Fatal("prefetch stopped after a zero delta folded onto a real scroll direction")
	}
}

func TestViewportConcurrentAccess(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 800, H: 600}}
	s := raster.NewScheduler(nil, nil, vp, 1, raster.Pref{})
	s.SetPlan(frame.FramePlan{Serial: 1})
	var wg sync.WaitGroup
	for i := int32(0); i < 8; i++ {
		wg.Add(1)
		go func(i int32) {
			defer wg.Done()
			for j := int32(0); j < 50; j++ {
				s.SetViewport(frame.Viewport{Offset: frame.Point{X: i*100 + j}, Size: frame.Size{W: 800, H: 600}})
				_ = s.Viewport()
				_ = s.SubmitScroll(frame.Point{Y: 1}, time.Now())
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			s.SetPlan(frame.FramePlan{Serial: uint64(j + 2)})
		}
	}()
	for j := 0; j < 100; j++ {
		s.BeginFrame(surface.Event{Delta: frame.Point{Y: 3}})
	}
	wg.Wait()
	s.SetViewport(frame.Viewport{Offset: frame.Point{X: 42, Y: 7}, Size: frame.Size{W: 800, H: 600}})
	got := s.Viewport()
	if got.Offset.X != 42 || got.Offset.Y != 7 {
		t.Fatalf("Viewport() = %+v, want offset {42 7}", got)
	}
	if got.Size.W != 800 || got.Size.H != 600 {
		t.Fatalf("Viewport() = %+v, want size {800 600}", got)
	}
}

func containsCoord(list []frame.TileCoord, c frame.TileCoord) bool {
	for _, x := range list {
		if x == c {
			return true
		}
	}
	return false
}

// qc:BR-174
func TestSameLayerContentUpdateReusesTiles(t *testing.T) {
	h := newSched(t, 4, 4, 32, frame.Size{W: 512, H: 512}, raster.Pref{PrefetchRows: 0})
	h.warm(t)
	before := h.s.Stats()
	h.l.Bump(2, frame.Rect4(0, 0, 10, 10))
	w := h.frame(t, vsync(0))
	if w.Needed > 2 {
		t.Fatalf("Needed = %d after a one-tile dirty bump, want only the dirty tile(s)", w.Needed)
	}
	if w.Reused == 0 {
		t.Fatal("Reused = 0 after a one-tile dirty bump; everything re-rasterized")
	}
	after := h.s.Stats()
	if after.Rasterized-before.Rasterized > 2 {
		t.Fatalf("rasterized %d tiles for a one-tile change", after.Rasterized-before.Rasterized)
	}
}
