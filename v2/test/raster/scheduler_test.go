package raster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/v2/internal/frame"
	"github.com/vyquocvu/goosie/v2/internal/paint"
	"github.com/vyquocvu/goosie/v2/internal/raster"
	"github.com/vyquocvu/goosie/v2/internal/surface"
)

// The scheduler is the M1 frame path: the one object that both knows what a tile is
// and is allowed to talk to the UI thread. These tests drive it the way
// surface.Loop does - BeginFrame, Submit, ComposeInto - rather than reaching into
// its internals, because the loop's contract is the reason the internals exist.

// ---------------------------------------------------------------------------
// scene
// ---------------------------------------------------------------------------

// schedColor stamps a coordinate and a content version into four bytes, so a test
// can tell which tile produced a pixel and whether it was drawn for the version it
// is being credited with.
func schedColor(c frame.TileCoord, version uint64) frame.Color {
	return frame.RGB(byte(version*37+11), byte(c.Col*17+5), byte(c.Row*23+3))
}

// schedRaster is the worker's side. It fills the buffer with the tile's identity
// rather than running the real rasterizer: which pixels a display list produces is
// Task 7's claim to make, and the scheduler's claims are about which tiles are
// drawn when, at which version, and with how much memory.
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

// schedDoc returns a document cols tiles wide and rows tiles tall - one fill over
// the whole extent at content version 1 - with a budget of budget tile buffers.
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

// testBackground is deliberately unlike any tile color, so "this pixel is still the
// background" is an unambiguous assertion.
var testBackground = frame.RGB(9, 9, 9)

type schedHarness struct {
	s    *raster.Scheduler
	l    *frame.Layer
	pool *raster.Pool
	c    *surface.Composer
	size frame.Size
}

// newSched returns a harness wired exactly like the loop: a real layer, a real worker
// pool, and a real composer whose backing store is the size the scheduler reports.
func newSched(t *testing.T, cols, rows int32, budget int64, vp frame.Size, prefs raster.Pref) *schedHarness {
	t.Helper()
	return newSchedVP(t, cols, rows, budget, frame.Viewport{Size: vp}, prefs)
}

// newSchedVP is the same for a scheduler that starts somewhere other than the top of
// the document, which is how a test gets a cold viewport without burning the frames it
// is about to measure.
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

// vsync is a pacing tick carrying an optional scroll delta, which is how the loop
// hands coalesced input to a scheduler.
func vsync(dy int32) surface.Event {
	return surface.Event{Kind: surface.EvVsync, Delta: frame.Point{Y: dy}, At: time.Now()}
}

// frame runs one vsync the way the loop does: plan, resize, submit, compose.
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

// pump resolves a frame and submits it without compositing, for a loop that is only
// waiting for asynchronous work to land.
func (h *schedHarness) pump() surface.FrameWork {
	_, needed := h.s.BeginFrame(vsync(0))
	return h.s.Submit(needed)
}

// untilQuiet draws until the frame path asks for nothing, which is the only way to be
// sure an asynchronous pool has finished with what is on screen.
func (h *schedHarness) untilQuiet(t *testing.T) {
	t.Helper()
	if !pollUntil(h.pumpUntilQuiet) {
		t.Fatalf("raster never settled: %+v", h.l.Stats())
	}
}

// The predicate is "nothing left to ask for", not "no tile is stale or empty". A budget
// that evicts leaves tiles empty on purpose, far off screen, and nobody will ever ask
// for them again - waiting on those is a livelock rather than a settle.
func (h *schedHarness) pumpUntilQuiet() bool {
	w := h.pump()
	return w.Needed == 0 && h.l.Grid.Stats().PaintingBytes == 0
}

// setOffset scrolls by whatever it takes to land exactly on y and draws the frame
// that gets there.
func (h *schedHarness) setOffset(t *testing.T, y int32) {
	t.Helper()
	h.frame(t, vsync(y-h.s.Viewport().Offset.Y))
	if got := h.s.Viewport().Offset.Y; got != y {
		t.Fatalf("setOffset(%d) landed at %d", y, got)
	}
}

// warm rasterizes the whole document by stepping the viewport down it one tile row
// at a time. It is the setup half of every "warm cache" claim in this file, and the
// only way to know the measured phase has no work left rather than no work noticed.
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

// ---------------------------------------------------------------------------
// the plan's five named scheduler tests
// ---------------------------------------------------------------------------

// Invariant 6 on the scroll path, twice over: the runtime says nothing was
// allocated, and the scheduler says none of its own scratch buffers grew. A warm
// scroll is the frame this whole design exists for, so this is the test that fails
// first when someone adds a convenience allocation back into it.
func TestScrollPathIsAllocationFree(t *testing.T) {
	const (
		cols, rows = 4, 20
		vp         = 4 * frame.TileSize
	)
	// Every tile in the document fits the budget, so a warm scroll has no memory
	// excuse for allocating: the budget is the document's own size.
	h := newSched(t, cols, rows, int64(cols*rows), frame.Size{W: vp, H: vp}, raster.Pref{})
	h.warm(t)
	if got := h.l.Stats().Valid; got != cols*rows {
		t.Fatalf("setup: %d tiles valid, want the whole %d tile document warm", got, cols*rows)
	}
	before := h.s.Stats()

	// 600 ticks of 100px, reversing at each end of the clamped range. A one-way
	// flick would press against the bottom of the document and stop moving, which
	// would pass this test while proving nothing about a scrolling frame path.
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
	// AllocsPerRun counts process-wide, which is another reason the cache has to be
	// warm: with nothing in flight there is no other goroutine to blame for a hit.
	if n := testing.AllocsPerRun(600, tick); n != 0 {
		t.Fatalf("allocs/op = %v on a warm scroll frame, want 0", n)
	}
	if got := h.s.Stats().Bytes; got != int64(cols*rows)*frame.TileSizeBytes() {
		t.Fatalf("Bytes = %d, want the warm document's own %d", got, int64(cols*rows)*frame.TileSizeBytes())
	}
}

// The ring is what protects p99 rather than the mean: the tile the user is about to
// reveal is already warm. Scrolling down has to queue the band below the viewport
// ahead of the one above it, or the prefetch spends budget on content the user has
// already left behind.
func TestPrefetchAheadOfScrollDirection(t *testing.T) {
	const (
		cols, rows = 2, 40
		vp         = 2 * frame.TileSize
	)
	h := newSchedVP(t, cols, rows, 200, frame.Viewport{
		Offset: frame.Point{Y: rows * frame.TileSize / 2},
		Size:   frame.Size{W: vp, H: vp},
	}, raster.Pref{PrefetchRows: 2})
	// Parked mid-document and measured on the very first frame, so both ring bands are
	// still cold. A scheduler that had already been scrolling would have drawn one of
	// them by now, and "nothing is queued that the viewport does not need" is not the
	// same claim as "the trailing band is missing".
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

	// With the ring switched off nothing outside the viewport is queued at all:
	// prefetch has to be a preference, not an unremovable tax on the budget.
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

// The memory half of M1 exit criterion 1: a long scroll into content nobody has
// looked at yet holds tile memory at the ceiling rather than growing with the
// document. The scheduler, not the grid, is what guarantees it, because the grid is
// allowed to overshoot when a raster is already on order.
func TestBudgetPlateaus(t *testing.T) {
	const (
		cols, rows = 4, 79 // 79 * 256 = 20,224px of document
		vp         = 4 * frame.TileSize
		budget     = 20
	)
	limit := int64(budget) * frame.TileSizeBytes()
	// The layer starts with a budget that could hold the document; the scheduler's
	// Pref.Budget is what clamps it, which is the knob a running browser turns.
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

// Eviction is only a win if the tile comes back. A user scrolling up must find
// content re-requested rather than permanently blank, which is the difference
// between a cache that shrinks and a cache that loses the page.
func TestEvictedTileIsRequeued(t *testing.T) {
	const (
		cols, rows = 2, 16
		vp         = 2 * frame.TileSize
		// Exactly the visible set, which is what makes the assertion below provable
		// rather than lucky: arriving at a fresh page has to evict every tile of the old
		// one, and the grid's eviction order is oldest-first with the most recently drawn
		// tile last. A budget even one tile larger leaves that tile behind and the test
		// starts asserting about LRU tie-breaking.
		budget = 4
		// A whole number of tile rows on purpose. An offset that is not tile aligned
		// puts one row more of tiles on screen than the budget can ever hold, and a cache
		// that cannot hold the viewport is a permanent thrash rather than something that
		// settles - so untilQuiet below would be waiting for a frame path that cannot
		// exist instead of checking that the eviction came back.
		dist = 12 * frame.TileSize
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

// A tile that cannot be drawn is a blank rectangle with a counter - not a crash, and
// not a job the scheduler re-asks for every vsync for the rest of the session.
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

	// More frames than MaxTileAttempts: the retries have to stop somewhere.
	for i := 0; i < frame.MaxTileAttempts*8 && h.s.Stats().Failed == 0; i++ {
		h.frame(t, vsync(0))
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
	// Blank means the layer background, not black and not the old content. The tile is
	// at Col 1/Row 1, which is still inside a two-tile-square viewport.
	px := h.c.Backing.At(int(bad.Col*frame.TileSize)+7, int(bad.Row*frame.TileSize)+7)
	if px != testBackground {
		t.Fatalf("pixel over the failed tile = %v, want the background %v", px, testBackground)
	}

	// Giving up is permanent for that content version: later frames must not re-ask.
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

	// Content changing under it is a fair retry, which is frame.Grid's rule; the
	// scheduler has to take the tile back rather than keep a blank rectangle.
	h.l.Bump(2, bad.Rect(frame.TileSize))
	_, needed := h.s.BeginFrame(vsync(0))
	if !containsCoord(needed, bad) {
		t.Fatal("a version bump covering a failed tile did not put it back in the queue")
	}
}

// ---------------------------------------------------------------------------
// damage, versions, and geometry ownership
// ---------------------------------------------------------------------------

// A scroll must cost the exposed band and nothing else. This is the claim behind
// "damage keeps a frame near the budget instead of a full recompose", stated as a
// pixel count so that it does not depend on a machine.
func TestScrollDamagesOnlyTheExposedBand(t *testing.T) {
	const (
		cols, rows = 4, 12
		vp         = 4 * frame.TileSize
		dy         = 100
	)
	h := newSched(t, cols, rows, int64(cols*rows), frame.Size{W: vp, H: vp}, raster.Pref{PrefetchRows: raster.PrefetchOff})
	// The whole document, not just the first screen: a tile that is still cold at the
	// offset being measured can install mid-Submit and turn into a second damaged tile,
	// which is real behaviour but not what this test is about.
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
	// The shifted rows must carry the tile that belongs there now, which is the only
	// way to tell a correct memmove from one that slid the wrong content up.
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

// The scheduler owns the scroll offset, because the tile offsets it hands the pool
// and the buffer the composer presents have to agree with each other. A plan
// therefore carries layers, not geometry, and a scroll past the end of the document
// clamps instead of showing the void.
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

	// An absolute viewport is how a driver that is not a window says "show this",
	// and a resize event widens the surface.
	h.s.SetViewport(frame.Viewport{Offset: frame.Point{Y: 600}, Size: h.size})
	if got := h.s.Viewport().Offset.Y; got != 600 {
		t.Fatalf("SetViewport ignored: offset = %d, want 600", got)
	}
	bigger := frame.Size{W: vp, H: vp + frame.TileSize}
	plan, needed := h.s.BeginFrame(surface.Event{Kind: surface.EvVsync, Size: bigger, At: time.Now()})
	if plan.Viewport.Size != bigger {
		t.Fatalf("resize not adopted: size = %v, want %v", plan.Viewport.Size, bigger)
	}
	// A second BeginFrame with no new event must still know the buffer it is
	// drawing into changed: the damage decision belongs to the last compose, not
	// the last plan.
	plan, needed = h.s.BeginFrame(vsync(0))
	h.c.Resize(plan.Viewport.Size)
	h.s.Submit(needed)
	damage := h.s.ComposeInto(h.c.Backing, h.c, plan, needed)
	if len(damage) != 1 || damage[0] != h.c.Backing.Bounds() {
		t.Fatalf("damage after a resize = %v, want the whole surface", damage)
	}
}

// A result is only worth installing if it depicts the version the layer is on now.
// Content can bump while a job runs, and crediting older pixels with the newer
// version would hide a raster that is still owed - permanently.
func TestHandleDoneIgnoresAResultForAnOldVersion(t *testing.T) {
	// One tile on screen, so exactly one job can be in flight and the refusal below has
	// one buffer to account for.
	const (
		cols, rows = 2, 4
		size       = frame.TileSize
	)
	c0 := frame.TileCoord{Col: 0, Row: 0}
	l, dl := schedDoc(t, cols, rows, 32)

	// Draw that one tile with an ordinary pool first. The version bump the test needs
	// stales pixels that exist, and until something has been installed there are no
	// pixels to stale - which is exactly the confusion this test exists to avoid.
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

	// The bump is what makes the running job's answer obsolete: the layer moves to
	// version 2 while the frozen list the job carries stays at version 1.
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
	// Nothing can have been collected during Submit: the only worker is parked on the
	// only job, so what it installed is provably nothing.
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
	// Re-asking is what recovers, so the coord has to come back into the needed list.
	_, needed = s.BeginFrame(vsync(0))
	if !containsCoord(needed, d.Coord) {
		t.Fatalf("the superseded tile %v was not requeued for the current version", d.Coord)
	}
}

// SetPlan hands the scheduler a new description of the content without moving the
// scroll position, and a plan superseded before it was drawn is counted rather than
// silently presented out of order.
func TestSetPlanSupersedesAndKeepsTheScrollPosition(t *testing.T) {
	const (
		cols, rows = 2, 8
		vp         = 2 * frame.TileSize
	)
	h := newSched(t, cols, rows, 32, frame.Size{W: vp, H: vp}, raster.Pref{PrefetchRows: raster.PrefetchOff})
	h.scroll(t, 500)
	// Two publishes with no frame between them: the first is superseded.
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

// The plan slot is also the only thread-crossing surface the scheduler offers:
// SetPlan is documented as safe from a producer goroutine, so a test that publishes
// while frames are being drawn has to be race-clean.
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

// SubmitScroll is the handoff a driver that is not running the loop uses, and it has
// to agree with the same fold BeginFrame applies to event deltas - or a caller that
// used both would move the document twice.
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
	// The direction of the last scroll is what the ring keys off, and a zero delta
	// must not erase it - otherwise the prefetch stops the instant a flick coalesces
	// to no net movement.
	if err := a.s.SubmitScroll(frame.Point{}, time.Now()); err != nil {
		t.Fatalf("SubmitScroll with no delta: %v", err)
	}
	a.frame(t, vsync(0))
	if got := a.s.Stats().Prefetched; got == 0 {
		t.Fatal("prefetch stopped after a zero delta folded onto a real scroll direction")
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
