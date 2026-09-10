package surface_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/v2/internal/frame"
	"github.com/vyquocvu/goosie/v2/internal/paint"
	"github.com/vyquocvu/goosie/v2/internal/raster"
	"github.com/vyquocvu/goosie/v2/internal/surface"
)

// ---------------------------------------------------------------------------
// platform stand-in
// ---------------------------------------------------------------------------

// presentRecord is what the loop handed the platform. The top-left pixel
// identifies which plan produced the frame, which is enough to prove that a
// superseded plan never reaches the window.
type presentRecord struct {
	color  frame.Color
	size   frame.Size
	damage []frame.Rect
}

// fakeWindow is the platform side of the loop tests. headless.Window arrives in
// Task 11, and the plan asks for these tests to run against it; until that exists
// the loop needs a Window a second goroutine can drive, because Run always assumes
// its events arrive from somewhere else.
type fakeWindow struct {
	events chan surface.Event

	mu       sync.Mutex
	presents []presentRecord
	cursor   surface.Cursor
	fail     error
}

func newFakeWindow(buffer int) *fakeWindow {
	return &fakeWindow{events: make(chan surface.Event, buffer)}
}

func (w *fakeWindow) Events() <-chan surface.Event { return w.events }

func (w *fakeWindow) Present(buf *frame.Bitmap, damage []frame.Rect) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fail != nil {
		return w.fail
	}
	rec := presentRecord{
		size:   buf.Size(),
		damage: append([]frame.Rect(nil), damage...),
	}
	if !buf.Empty() {
		rec.color = buf.At(0, 0)
	}
	w.presents = append(w.presents, rec)
	return nil
}

func (w *fakeWindow) SetCursor(c surface.Cursor) {
	w.mu.Lock()
	w.cursor = c
	w.mu.Unlock()
}

func (w *fakeWindow) ScaleFactor() float32 { return 2 }

func (w *fakeWindow) Close() error { return nil }

func (w *fakeWindow) send(t *testing.T, ev surface.Event) {
	t.Helper()
	select {
	case w.events <- ev:
	case <-time.After(10 * time.Second):
		t.Fatal("the event queue is full: Run stopped reading events")
	}
}

// vsync queues one pacing tick stamped now, so the frame recorder has something
// real to measure against.
func (w *fakeWindow) vsync(t *testing.T) {
	t.Helper()
	w.send(t, surface.Event{Kind: surface.EvVsync, At: time.Now()})
}

func (w *fakeWindow) scroll(t *testing.T, dy int32) {
	t.Helper()
	w.send(t, surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: dy}, At: time.Now()})
}

func (w *fakeWindow) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.presents)
}

func (w *fakeWindow) last(t *testing.T) presentRecord {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.presents) == 0 {
		t.Fatal("no frame was presented")
	}
	return w.presents[len(w.presents)-1]
}

// ---------------------------------------------------------------------------
// Scheduler over real frame and raster types
// ---------------------------------------------------------------------------

// tileScheduler plays the loop's counterpart over a real layer, a real tile grid,
// and a real raster pool. Its production equivalent is raster.Scheduler, which
// surface cannot name: the import graph puts raster above surface. Testing against
// the same data structures through the same three calls means a change that breaks
// the real scheduler's contract has to break this one too.
//
// A nil layer means "content exists, tiles do not", which the plan-handoff tests
// use to identify a frame by its background colour alone.
type tileScheduler struct {
	mu    sync.Mutex
	slot  *frame.Plan
	layer *frame.Layer
	dl    *paint.LayerDL
	pool  *raster.Pool

	scroll frame.Point
	cur    frame.FramePlan
	vis    []frame.TileCoord
	need   []frame.TileCoord
	blits  []surface.TileBlit
	damage [1]frame.Rect

	reused    int
	frames    int64
	submitted int64
	refused   int64
	writes    int64
	gate      chan struct{} // when non-nil, a raster parks here before painting
}

// tileColor stamps a coordinate and a content version into four bytes, so a test
// can tell a fresh raster from a stale one and one tile from its neighbour.
func tileColor(c frame.TileCoord, version uint64) frame.Color {
	return frame.RGB(byte(version*37+1), byte(c.Col*11+7), byte(c.Row*13+3))
}

// paintTile is the test's rasterizer. It reads the version off the job's own
// display list, so the pixels say which content version produced them.
func (s *tileScheduler) paintTile(j raster.Job) error {
	if s.gate != nil {
		<-s.gate
	}
	if j.Out == nil {
		return errors.New("test: job carried no buffer")
	}
	var v uint64
	if j.DL != nil {
		v = j.DL.Version()
	}
	j.Out.FillRect(j.Out.Bounds(), tileColor(j.Coord, v), nil)
	return nil
}

func (s *tileScheduler) BeginFrame(ev surface.Event) (*frame.FramePlan, []frame.TileCoord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames++
	s.scroll.X += ev.Delta.X
	s.scroll.Y += ev.Delta.Y

	p, ok := s.slot.Take()
	if !ok {
		return nil, nil
	}
	// The plan is authoritative about the surface's size and scale, because that is
	// what the composer's buffer has to match. The scroll offset is the scheduler's
	// own state, folded in here.
	vp := p.Viewport
	if s.layer != nil {
		b := s.layer.Bounds
		vp.Offset = frame.Point{
			X: clamp32(s.scroll.X, 0, max(0, b.X1-vp.Size.W)),
			Y: clamp32(s.scroll.Y, 0, max(0, b.Y1-vp.Size.H)),
		}
	} else {
		// No layer means no document to clamp against, so the offset is the raw
		// scroll. A test watching only for coalescing still sees its deltas arrive.
		vp.Offset = frame.Point{X: s.scroll.X, Y: s.scroll.Y}
	}
	s.cur = frame.FramePlan{
		Serial:     p.Serial,
		Viewport:   vp,
		Scale:      p.Scale,
		Layers:     p.Layers,
		Background: p.Background,
	}
	if s.layer == nil {
		s.vis, s.need = s.vis[:0], s.need[:0]
		s.reused = 0
		return &s.cur, nil
	}

	g := s.layer.Grid
	s.vis = s.layer.VisibleCoords(vp, s.vis[:0])
	s.need = s.need[:0]
	reused := 0
	for _, c := range s.vis {
		if !s.layer.Needs(c) {
			reused++
			continue
		}
		// A tile with a job already on order is being painted right now. Queueing
		// it again pays for a second raster of pixels already arriving, and under
		// budget pressure the duplicate evicts the original.
		if g.InFlight(c) {
			continue
		}
		s.need = append(s.need, c)
	}
	s.reused = reused
	return &s.cur, s.need
}

func (s *tileScheduler) Submit(needed []frame.TileCoord) surface.FrameWork {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := surface.FrameWork{Needed: len(needed), Reused: s.reused}
	if s.layer == nil || s.pool == nil {
		return w
	}
	g := s.layer.Grid
	for _, c := range needed {
		pixels, ok := g.Acquire(c)
		if !ok {
			continue
		}
		err := s.pool.Submit(raster.Job{
			Layer:  s.layer,
			Coord:  c,
			DL:     s.dl,
			Bounds: c.Rect(g.TileSize()),
			Out:    pixels,
		})
		if err != nil {
			// A refused tile keeps its own buffer - the grid still owns it - so
			// Release only clears the in-flight mark. The next frame asks again and
			// Acquire hands out the same memory instead of allocating.
			g.Release(c, pixels)
			if errors.Is(err, raster.ErrQueueFull) {
				w.Refused++
				s.refused++
			}
			continue
		}
		w.Submitted++
		s.submitted++
	}
	// Everything that has finished is collected and nothing waits for the rest.
	// This is the one place the loop could accidentally block on rasterization,
	// which is why the only receive here is a non-blocking Poll.
	for {
		d, ok := s.pool.Poll()
		if !ok {
			break
		}
		if d.Err != nil {
			// Whether to retry is the real scheduler's policy; here a failure only
			// has to be reported rather than fatal.
			g.MarkFailed(d.Coord)
			g.Release(d.Coord, d.Out)
			continue
		}
		if g.MarkValid(d.Coord, s.layer.ContentVersion, d.Out) {
			w.Accepted++
		} else {
			g.Release(d.Coord, d.Out)
		}
	}
	return w
}

func (s *tileScheduler) ComposeInto(backing *frame.Bitmap, c *surface.Composer, plan *frame.FramePlan, _ []frame.TileCoord) []frame.Rect {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blits = s.blits[:0]
	if s.layer != nil {
		g := s.layer.Grid
		vp := plan.Viewport
		for _, coord := range s.vis {
			t := g.Peek(coord)
			// Stale tiles are blitted exactly as much as valid ones. That is the
			// mechanism behind "present now, fix next vsync": a version bump costs a
			// re-raster, never a blank rectangle.
			if t == nil || t.Pixels == nil {
				continue
			}
			dst := coord.Rect(g.TileSize()).Translate(-vp.Offset.X, -vp.Offset.Y)
			s.blits = append(s.blits, surface.TileBlit{Pixels: t.Pixels, Dst: dst})
		}
	}
	// Whole-surface damage. The real scheduler narrows this to the exposed band
	// plus the prefetch ring; the loop only carries the list to the platform, so a
	// superset is correct here and cheaper to keep honest.
	s.damage[0] = frame.Rect4(0, 0, plan.Viewport.Size.W, plan.Viewport.Size.H)
	return c.Compose(plan.Background, s.blits, s.damage[:], &s.writes)
}

// accessors for assertions, taking the same lock the loop's calls take

func (s *tileScheduler) offsetY() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.Viewport.Offset.Y
}

func (s *tileScheduler) counters() (frames, submitted, refused int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frames, s.submitted, s.refused
}

func clamp32(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// newScene returns a layer of cols x rows tiles holding one whole-extent fill at
// content version 1. The budget is twice the document's own tiles rather than
// exactly them, because a redraw holds both the pixels being presented and the
// replacement on order: a grid sized to the document to the byte cannot survive
// invalidating all of it at once, and eviction would eat the visible tiles.
func newScene(t *testing.T, cols, rows int32) (*frame.Layer, *paint.LayerDL) {
	t.Helper()
	bounds := frame.Rect4(0, 0, cols*frame.TileSize, rows*frame.TileSize)
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, int(cols*rows)*2+8)
	l := frame.NewLayer(frame.LayerID(1), bounds, 2*int64(cols*rows)*frame.TileSizeBytes(), pool)
	dl := fillList(bounds, 1)
	l.SetContent(dl)
	return l, dl
}

// fillList returns a frozen whole-extent fill at the given content version. One
// solid colour per tile is all these tests need to tell a fresh raster from a
// stale one, and a second version of it is how a content change is expressed
// without an engine.
func fillList(bounds frame.Rect, version uint64) *paint.LayerDL {
	lis := paint.NewList(1)
	lis.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: bounds, Color: frame.RGB(200, 200, 200)})
	return lis.Build(version).Publish()
}

func published(serial uint64, vp frame.Viewport, bg frame.Color, layers ...*frame.Layer) frame.FramePlan {
	return frame.FramePlan{Serial: serial, Viewport: vp, Scale: 1, Layers: layers, Background: bg}
}

func newSlot(p frame.FramePlan) *frame.Plan {
	slot := &frame.Plan{}
	slot.Publish(p)
	return slot
}

// newScheduler returns a scheduler with no pool. A pool is started separately
// because a test that wants a warm grid must not have workers competing for the
// tiles it paints itself.
func newScheduler(layer *frame.Layer, dl *paint.LayerDL, vp frame.Viewport) *tileScheduler {
	return &tileScheduler{
		slot:  newSlot(published(1, vp, frame.RGB(9, 9, 9), layer)),
		layer: layer,
		dl:    dl,
	}
}

// startPool gives the scheduler a raster pool running s.paintTile. Anything the
// rasterizer reads - the gate, above all - must be set before this call, because
// launching the workers is the point at which another thread starts reading the
// scheduler's fields.
func (s *tileScheduler) startPool(t *testing.T, workers, queueDepth int) *raster.Pool {
	t.Helper()
	pool := raster.New(workers, queueDepth, s.paintTile)
	pool.Start(context.Background())
	t.Cleanup(func() { pool.Close() })
	s.mu.Lock()
	s.pool = pool
	s.mu.Unlock()
	return pool
}

// warm rasterizes every tile in the layer synchronously. That is what "cache
// warm" means for a gate test: the pixels exist before measurement starts and no
// worker is involved. Pairing Acquire with MarkValid also checks that the grid's
// in-flight bookkeeping balances on the happy path.
func (s *tileScheduler) warm(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.layer == nil {
		t.Fatal("warm needs a layer")
	}
	g := s.layer.Grid
	v := s.layer.ContentVersion
	for row := int32(0); row*frame.TileSize < s.layer.Bounds.Y1; row++ {
		for col := int32(0); col*frame.TileSize < s.layer.Bounds.X1; col++ {
			c := frame.TileCoord{Col: col, Row: row}
			pixels, ok := g.Acquire(c)
			if !ok {
				t.Fatalf("Acquire(%v) refused a coordinate inside the layer", c)
			}
			if err := s.paintTile(raster.Job{Coord: c, DL: s.dl, Out: pixels}); err != nil {
				t.Fatalf("warm raster of %v: %v", c, err)
			}
			if !g.MarkValid(c, v, pixels) {
				t.Fatalf("warm raster of %v was rejected", c)
			}
		}
	}
	if got := g.Stats(); got.PaintingBytes != 0 {
		t.Fatalf("warming left %d bytes on order, want Acquire and MarkValid to balance per tile", got.PaintingBytes)
	}
}

// ---------------------------------------------------------------------------
// harness helpers
// ---------------------------------------------------------------------------

// start runs the loop on its own goroutine and returns a join that reports why it
// stopped.
func start(t *testing.T, loop *surface.Loop) func() error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()
	return func() error {
		t.Helper()
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(10 * time.Second):
			t.Fatal("Run did not return after cancellation")
			return nil
		}
	}
}

// waitFor blocks until cond holds or the deadline passes.
//
// The condition may only read state that is safe to touch from a second goroutine:
// the fake window and the scheduler counters, both mutex-guarded, and Loop.Stats.
// A tile grid, a composer's backing store, and the frame recorder belong to
// whichever thread runs the loop, so a test reads them after joining it - the same
// rule production has. Asserting on the grid mid-frame would be a data race
// wearing a test's clothes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Microsecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func newComposerFor(vp frame.Viewport) *surface.Composer {
	return surface.NewComposer(vp.Size, frame.NewBitmapPool(vp.Size, 4))
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestVsyncFrameCountEqualsPresentCount(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	l, dl := newScene(t, 2, 4)
	s := newScheduler(l, dl, vp)
	pool := s.startPool(t, 2, 64)

	w := newFakeWindow(64)
	loop := surface.NewLoop(w, s, newComposerFor(vp), frame.NewFrameRecorder(16))
	stop := start(t, loop)

	const ticks = 12
	for i := 0; i < ticks; i++ {
		w.vsync(t)
		waitFor(t, "a present", func() bool { return w.count() == i+1 })
	}
	got := loop.Stats()
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Frames != ticks || got.Presents != ticks {
		t.Fatalf("frames/presents = %d/%d, want %d/%d: a vsync that does not present is a dropped frame",
			got.Frames, got.Presents, ticks, ticks)
	}
	if got := loop.Composer().Backing.Size(); got != vp.Size {
		t.Fatalf("backing store = %v, want %v", got, vp.Size)
	}
	// Twelve frames needed four tiles once. Everything else was a reuse.
	if got := l.Stats(); got.Valid != 4 || got.Failed != 0 {
		t.Fatalf("grid = %+v, want 4 valid tiles and no failures", got)
	}
	if got := pool.Stats(); got.Rasterized != 4 {
		t.Fatalf("workers ran %d jobs, want the 4 tiles the first frame needed", got.Rasterized)
	}
}

func TestWarmScrollRasterizesNoTiles(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	// 26 rows: the whole run below - down 24 tiles and back up 6000px - stays inside
	// the document, so what is measured is the cache rather than a scroll off the end
	// of it.
	l, dl := newScene(t, 2, 26)
	s := newScheduler(l, dl, vp)
	s.warm(t)
	pool := s.startPool(t, 2, 64)

	w := newFakeWindow(128)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	stop := start(t, loop)

	// Phase 1, one tile per tick: every frame lands on a tile boundary, so the
	// visible set is exactly four tiles and the reuse count is an exact number.
	const down = 24
	for i := 0; i < down; i++ {
		w.scroll(t, frame.TileSize)
		w.vsync(t)
	}
	waitFor(t, "24 presents", func() bool { return w.count() == down })
	if got := s.offsetY(); got != down*frame.TileSize {
		t.Fatalf("scroll offset = %d, want %d: the deltas did not reach the viewport", got, down*frame.TileSize)
	}
	if got := loop.Stats(); got.Reused != int64(down)*4 {
		t.Fatalf("Reused = %d after %d tile-aligned frames, want %d: four cached tiles per frame went uncounted",
			got.Reused, down, int64(down)*4)
	}

	// Phase 2, the plan's own numbers: 60 ticks of 100px deltas, back up so no
	// offset is tile-aligned and a viewport straddles three rows. The invariant is
	// what has to hold there, not the count, so the reuse total only gets a floor -
	// phase 1 already pinned it exactly.
	const up = 60
	for i := 0; i < up; i++ {
		w.scroll(t, -100)
		w.vsync(t)
	}
	waitFor(t, "84 presents", func() bool { return w.count() == down+up })
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := s.offsetY(); got != down*frame.TileSize-up*100 {
		t.Fatalf("scroll offset = %d, want %d: negative deltas did not coalesce", got, down*frame.TileSize-up*100)
	}
	got := loop.Stats()
	if got.Frames != down+up || got.Presents != down+up {
		t.Fatalf("frames/presents = %d/%d, want %d/%d", got.Frames, got.Presents, down+up, down+up)
	}
	if got.Rasterize != 0 {
		t.Fatalf("Rasterize = %d over %d warm scroll frames, want 0", got.Rasterize, down+up)
	}
	if got.Reused < int64(down+up)*4 {
		t.Fatalf("Reused = %d over %d frames, want at least the four minimum visible tiles per frame",
			got.Reused, down+up)
	}
	// The structural form of the same claim: not one worker ran and not one job was
	// queued. A scheduler that re-rasterized and then reported zero would pass the
	// loop's counter and fail here.
	if st := pool.Stats(); st.Rasterized != 0 || st.Queued != 0 || st.DroppedResults != 0 {
		t.Fatalf("the pool did %d rasters with %d queued and %d results dropped on warm content",
			st.Rasterized, st.Queued, st.DroppedResults)
	}
	if _, submitted, _ := s.counters(); submitted != 0 {
		t.Fatalf("submitted %d jobs for warm content, want 0", submitted)
	}
	if st := l.Stats(); st.Evictions != 0 || st.Bytes != int64(52)*frame.TileSizeBytes() {
		t.Fatalf("a warm scroll moved the grid to %d bytes with %d evictions", st.Bytes, st.Evictions)
	}
}

func TestScrollChangesNoContentVersion(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	l, dl := newScene(t, 2, 12)
	s := newScheduler(l, dl, vp)
	s.warm(t)
	before := l.ContentVersion
	pool := s.startPool(t, 2, 64)

	w := newFakeWindow(64)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	stop := start(t, loop)

	for i := 0; i < 20; i++ {
		w.scroll(t, 97)
		w.vsync(t)
	}
	waitFor(t, "20 presents", func() bool { return w.count() == 20 })
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if l.ContentVersion != before {
		t.Fatalf("invariant 1 broken: scrolling moved the content version from %d to %d", before, l.ContentVersion)
	}
	if dl.Version() != before {
		t.Fatalf("the frozen display list reports version %d, want %d", dl.Version(), before)
	}
	if got := l.Stats(); got.Evictions != 0 {
		t.Fatalf("scrolling cost %d evictions on a grid budgeted for the whole document", got.Evictions)
	}
	if got := pool.Stats(); got.Rasterized != 0 {
		t.Fatalf("a scroll re-rasterized %d tiles", got.Rasterized)
	}
}

func TestLateTileNeverBlocksPresent(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	l, dl := newScene(t, 2, 4)
	s := newScheduler(l, dl, vp)
	s.warm(t)
	at := frame.TileCoord{Col: 0, Row: 0}
	wantStale := tileColor(at, l.ContentVersion)

	// A content change makes every visible tile stale while it keeps the pixels it
	// already had, and one worker that never gets to finish: every job parks on the
	// gate. If the loop waited on a completion, the first vsync would hang and this
	// test would fail on its deadline rather than on an assertion, which is a
	// stronger proof than any latency budget.
	gate := make(chan struct{})
	s.gate = gate
	// The rasterizer stamps the version of the display list it was handed, so new
	// content has to be a version-2 list: a layer bump over the same frozen list
	// would repaint exactly the pixels the stale tiles already hold.
	s.dl = fillList(l.Bounds, 2)
	pool := s.startPool(t, 1, 64)
	// Registered after the pool's own cleanup so it runs first: a worker still
	// parked on the gate would otherwise strand pool.Close and the failure would
	// read as a timeout instead of as whatever this test actually asserts.
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	})
	l.Bump(2, l.Bounds)
	if vis := l.VisibleCoords(vp, nil); len(vis) != 4 {
		t.Fatalf("the viewport covers %d tiles, want 4", len(vis))
	}

	w := newFakeWindow(64)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	stop := start(t, loop)

	const ticks = 5
	for i := 0; i < ticks; i++ {
		w.vsync(t)
		waitFor(t, "a present while every tile is in flight", func() bool { return w.count() == i+1 })
	}

	frames, submitted, refused := s.counters()
	if frames != ticks {
		t.Fatalf("frames = %d, want %d", frames, ticks)
	}
	if submitted != 4 {
		t.Fatalf("submitted %d jobs over %d frames, want 4: an in-flight tile was queued again", submitted, ticks)
	}
	if refused != 0 {
		t.Fatalf("refused %d jobs from a 64-deep queue", refused)
	}
	if got := w.last(t).color; got != wantStale {
		t.Fatalf("presented pixel = %v, want the stale tile's %v", got, wantStale)
	}
	if got := loop.Stats(); got.Rasterize != 0 || got.Presents != ticks {
		t.Fatalf("rasterize/presents = %d/%d, want 0/%d while every raster is late",
			got.Rasterize, got.Presents, ticks)
	}

	// The other half: the late tiles do arrive, and the same coordinate is still
	// only rasterized once.
	close(gate)
	wantFresh := tileColor(at, 2)
	for i := 0; i < 20; i++ {
		w.vsync(t)
	}
	waitFor(t, "the fresh tile content", func() bool { return w.last(t).color == wantFresh })
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := pool.Stats(); got.Rasterized != 4 {
		t.Fatalf("workers ran %d jobs, want the 4 tiles once each", got.Rasterized)
	}
	// The four off-screen tiles stay stale: the content change dirtied the whole
	// document and only what the viewport can show was worth re-rasterizing.
	if got := l.Stats(); got.Valid != 4 || got.Stale != 4 || got.Failed != 0 {
		t.Fatalf("grid after the late tiles reported = %+v, want 4 valid and 4 stale off screen", got)
	}
}

func TestPresentUsesLatestPlanAndDropsSuperseded(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	slot := &frame.Plan{}
	s := newScheduler(nil, nil, vp)
	s.slot = slot
	w := newFakeWindow(16)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	stop := start(t, loop)

	red := frame.RGB(255, 0, 0)
	green := frame.RGB(0, 255, 0)
	blue := frame.RGB(0, 0, 255)
	// Three content versions with no vsync between them and nothing consumed yet:
	// a depth-1 slot means the display sees the newest, and the two it replaced are
	// counted as superseded rather than queued up and presented in turn.
	for i, bg := range []frame.Color{red, green, blue} {
		slot.Publish(published(uint64(i+1), vp, bg))
	}

	w.vsync(t)
	waitFor(t, "one present", func() bool { return w.count() == 1 })

	if got := loop.Stats().LastPlan; got != 3 {
		t.Fatalf("presented plan serial = %d, want 3 (the newest)", got)
	}
	if got := w.last(t).color; got != blue {
		t.Fatalf("presented background = %v, want the newest plan's %v", got, blue)
	}
	if got := slot.Stats().Dropped; got != 2 {
		t.Fatalf("superseded plans = %d, want 2: the depth-1 slot must count what it replaced", got)
	}
	if got := loop.Stats().Frames; got != 1 {
		t.Fatalf("frames = %d, want 1: three pending plans are one frame of work", got)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestScrollDeltasCoalesceIntoOneFrame(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	l, dl := newScene(t, 2, 12)
	s := newScheduler(l, dl, vp)
	w := newFakeWindow(16)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	stop := start(t, loop)

	// A flick: six deltas between two vsyncs. The document moves by their sum and
	// the surface is composited once.
	for i := 0; i < 6; i++ {
		w.scroll(t, 100)
	}
	w.vsync(t)
	waitFor(t, "the coalesced frame", func() bool { return w.count() == 1 })

	if got := s.offsetY(); got != 600 {
		t.Fatalf("scroll offset = %d, want 600: a coalesced flick lost deltas", got)
	}
	if got := loop.Stats(); got.Frames != 1 {
		t.Fatalf("frames = %d, want 1: an intermediate scroll position cannot be seen and costs a present", got.Frames)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestNonVsyncEventsDoNotDraw(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	s := newScheduler(nil, nil, vp)
	w := newFakeWindow(64)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	stop := start(t, loop)

	for i := 0; i < 10; i++ {
		w.send(t, surface.Event{Kind: surface.EvPointer, Pos: frame.Point{X: int32(i), Y: 1}, Button: surface.ButtonLeft})
		w.send(t, surface.Event{Kind: surface.EvKey, Key: 'a'})
		w.send(t, surface.Event{Kind: surface.EvResize, Size: vp.Size})
		w.send(t, surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: 5}})
	}
	w.vsync(t)
	waitFor(t, "the frame after 41 queued inputs", func() bool { return w.count() == 1 })

	// The vsync itself carries no delta, so what reached BeginFrame is the sum of
	// the ten scrolls that preceded it.
	if got := s.offsetY(); got != 50 {
		t.Fatalf("coalesced offset = %d, want 50", got)
	}
	if got := loop.Stats(); got.Frames != 1 {
		t.Fatalf("frames = %d, want 1: input alone must not draw, only the display's tick may", got.Frames)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIdleVsyncWithoutAPlanPresentsNothing(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	s := newScheduler(nil, nil, vp)
	s.slot = &frame.Plan{} // nothing published yet: a cold start
	w := newFakeWindow(8)
	loop := surface.NewLoop(w, s, newComposerFor(vp), frame.NewFrameRecorder(8))
	stop := start(t, loop)

	w.vsync(t)
	w.vsync(t)
	waitFor(t, "two idle frames", func() bool { return loop.Stats().Idle == 2 })

	if got := loop.Stats(); got.Presents != 0 || got.Frames != 2 {
		t.Fatalf("frames/presents = %d/%d, want 2/0: an uncomposited buffer must not be shown", got.Frames, got.Presents)
	}
	if got := loop.Stats().LastPlan; got != 0 {
		t.Fatalf("LastPlan = %d after two idle frames, want 0", got)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if w.count() != 0 {
		t.Fatalf("the window received %d presents with no plan to show", w.count())
	}
}

func TestQueueFullIsRefusedNotBlocked(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	l, dl := newScene(t, 2, 4)
	s := newScheduler(l, dl, vp)
	// One tile per job against a queue two deep, with the single worker parked:
	// every frame has to be told no at least twice, and being told no has to cost
	// the loop nothing and cost the tile nothing but a delay.
	gate := make(chan struct{})
	s.gate = gate
	pool := s.startPool(t, 1, 2)
	// Same as in TestLateTileNeverBlocksPresent: unblock the worker before the pool
	// is closed, or a mid-test failure hangs the whole package.
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	})

	w := newFakeWindow(64)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	stop := start(t, loop)

	const ticks = 8
	for i := 0; i < ticks; i++ {
		w.vsync(t)
		waitFor(t, "a present over a full queue", func() bool { return w.count() == i+1 })
	}
	if got := loop.Stats(); got.Refused == 0 {
		t.Fatal("Refused = 0 with a 2-deep queue and 4 tiles needed: nothing was turned away, so this proved nothing")
	}

	// A refused tile must stay requestable, or a refusal would have become a
	// permanent blank. Open the gate and the whole viewport has to fill in.
	close(gate)
	for i := 0; i < 40; i++ {
		w.vsync(t)
	}
	waitFor(t, "all four tiles after the queue drained", func() bool {
		return loop.Stats().Rasterize == 4
	})
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// A refused tile is asked for again next frame, so the jobs that reached the
	// queue are exactly the four tiles - one successful submission each - and
	// everything else the frames asked for was turned away. A fifth submission
	// would mean a tile was painted twice, which is what the in-flight mark exists
	// to prevent.
	_, submitted, refused := s.counters()
	if submitted != 4 {
		t.Fatalf("submitted %d jobs, want the 4 tiles exactly once each", submitted)
	}
	if refused == 0 {
		t.Fatal("no submission was refused over a 2-deep queue with the worker parked")
	}
	if got := pool.Stats(); got.Rasterized != 4 || got.DroppedResults != 0 {
		t.Fatalf("workers ran %d jobs and lost %d results, want 4 and 0", got.Rasterized, got.DroppedResults)
	}
	if got := l.Stats(); got.Valid != 4 {
		t.Fatalf("grid holds %d valid tiles after the refusals, want 4", got.Valid)
	}
}

func TestStatsIsSafeToReadWhileTheLoopRuns(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	s := newScheduler(nil, nil, vp)
	w := newFakeWindow(1024)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	stop := start(t, loop)

	// The counters exist for a gate harness and an inspector, both of which read
	// them from another thread while a frame is in flight. Under -race this is the
	// test that says so.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 4000; i++ {
			_ = loop.Stats()
		}
	}()
	const ticks = 500
	for i := 0; i < ticks; i++ {
		w.vsync(t)
	}
	wg.Wait()
	waitFor(t, "500 presents", func() bool { return w.count() == ticks })
	if got := loop.Stats(); got.Frames != ticks || got.Presents != ticks {
		t.Fatalf("frames/presents = %d/%d, want %d/%d", got.Frames, got.Presents, ticks, ticks)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestLoopResizesTheBackingForAChangedViewport(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	slot := newSlot(published(1, vp, frame.RGB(9, 9, 9)))
	s := newScheduler(nil, nil, vp)
	s.slot = slot
	w := newFakeWindow(8)
	c := newComposerFor(vp)
	loop := surface.NewLoop(w, s, c, nil)
	stop := start(t, loop)

	w.vsync(t)
	waitFor(t, "the first present", func() bool { return w.count() == 1 })
	if got := w.last(t).size; got != vp.Size {
		t.Fatalf("presented %v, want %v", got, vp.Size)
	}

	bigger := frame.Size{W: 320, H: 200}
	slot.Publish(published(2, frame.Viewport{Size: bigger}, frame.RGB(1, 2, 3)))
	w.vsync(t)
	waitFor(t, "the resized present", func() bool { return w.count() == 2 })

	if got := w.last(t).size; got != bigger {
		t.Fatalf("presented %v after a resize to %v: the window was handed the old buffer", got, bigger)
	}
	// A swapped buffer is garbage everywhere, so a resize frame must damage the
	// whole new surface rather than only the rows the new size exposes.
	if got := w.last(t).damage; len(got) != 1 || got[0] != frame.Rect4(0, 0, 320, 200) {
		t.Fatalf("damage after a resize = %v, want the whole new surface", got)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := c.Backing.Size(); got != bigger {
		t.Fatalf("composer backing = %v, want %v", got, bigger)
	}
}

func TestFrameMarkRecordsFiveTimestamps(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	l, dl := newScene(t, 2, 4)
	s := newScheduler(l, dl, vp)
	pool := s.startPool(t, 2, 64)

	w := newFakeWindow(16)
	rec := frame.NewFrameRecorder(16)
	loop := surface.NewLoop(w, s, newComposerFor(vp), rec)
	stop := start(t, loop)

	const ticks = 8
	for i := 0; i < ticks; i++ {
		w.vsync(t)
		waitFor(t, "a present", func() bool { return w.count() == i+1 })
	}
	// Which frame the four rasters are accepted on is the workers' business, so the
	// cold tiles are counted across the marks rather than pinned to one of them. One
	// extra frame runs after everything is warm, so the last mark is a pure reuse.
	waitFor(t, "the four tiles to be accepted", func() bool { return loop.Stats().Rasterize == 4 })
	w.vsync(t)
	waitFor(t, "the warm frame", func() bool { return w.count() == ticks+1 })
	// The recorder belongs to the thread that runs the loop, so it is read after
	// joining it.
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	marks := rec.Marks()
	if len(marks) != ticks+1 {
		t.Fatalf("recorder holds %d marks, want %d", len(marks), ticks+1)
	}
	// Five timestamps per frame are the gate's raw material, and they are only five
	// because plan, submit, compose, and present are separate calls. One
	// do-everything scheduler method would collapse them into a single measurement
	// and make a stall impossible to attribute.
	for i, m := range marks {
		if m.VsyncAt.IsZero() {
			t.Fatalf("mark %d has no vsync stamp", i)
		}
		stamps := []time.Time{m.VsyncAt, m.PlanAt, m.SubmitAt, m.ComposedAt, m.PresentedAt}
		for j, name := range []string{"vsync", "plan", "submit", "composed", "present"} {
			if stamps[j].IsZero() {
				t.Fatalf("mark %d has no %s stamp", i, name)
			}
		}
		for j := 1; j < len(stamps); j++ {
			if !stamps[j].After(stamps[j-1]) {
				t.Fatalf("mark %d: stamp %d (%v) does not follow stamp %d (%v)", i, j, stamps[j], j-1, stamps[j-1])
			}
		}
		if m.Serial == 0 {
			t.Fatalf("mark %d carries no plan serial", i)
		}
		if m.Work() <= 0 {
			t.Fatalf("mark %d reports zero vsync-to-present work", i)
		}
	}
	if marks[0].Serial == 0 {
		t.Fatal("the first mark carries no plan serial")
	}
	rasterized := int32(0)
	for _, m := range marks {
		rasterized += m.TilesRasterized
	}
	if rasterized != 4 {
		t.Fatalf("the marks account for %d rasterized tiles, want the scene's 4 counted exactly once", rasterized)
	}
	last := marks[len(marks)-1]
	if last.TilesRasterized != 0 || last.TilesReused != 4 {
		t.Fatalf("the warm frame rasterized %d and reused %d, want 0 and 4", last.TilesRasterized, last.TilesReused)
	}
	if last.StylePasses != 0 || last.LayoutPasses != 0 {
		t.Fatal("a frame ran style or layout, which M1 has no engine for")
	}
	if got := pool.Stats(); got.Rasterized != 4 || got.DroppedResults != 0 {
		t.Fatalf("workers ran %d jobs and lost %d results, want 4 and 0", got.Rasterized, got.DroppedResults)
	}
}

func TestPresentErrorEndsRunWithContext(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	s := newScheduler(nil, nil, vp)
	w := newFakeWindow(8)
	boom := errors.New("window server said no")
	w.fail = boom
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)

	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { done <- loop.Run(ctx) }()
	w.vsync(t)

	var err error
	select {
	case err = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run kept going after the platform refused a present")
	}
	if err == nil {
		t.Fatal("Run returned nil after a failed present")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("Run returned %v, want the platform error wrapped", err)
	}
	if !strings.Contains(err.Error(), "present frame 1") {
		t.Fatalf("error %q does not say which frame failed", err)
	}
	// A frame that failed to present is still a frame, and not a present.
	if got := loop.Stats(); got.Frames != 1 || got.Presents != 0 {
		t.Fatalf("a failed frame counted %d frames and %d presents, want 1 and 0", got.Frames, got.Presents)
	}
}

func TestClosedEventChannelEndsRun(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	s := newScheduler(nil, nil, vp)
	w := newFakeWindow(8)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { done <- loop.Run(ctx) }()

	w.vsync(t)
	waitFor(t, "a present before shutdown", func() bool { return w.count() == 1 })
	close(w.events)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v after the window closed, want a clean stop", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return when the event channel closed")
	}
}

func TestCancelEndsRun(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 256, H: 256}}
	s := newScheduler(nil, nil, vp)
	w := newFakeWindow(64)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	stop := start(t, loop)

	for i := 0; i < 5; i++ {
		w.vsync(t)
	}
	waitFor(t, "5 presents", func() bool { return w.count() == 5 })
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// A loop that had already exited on its own would also return nil, so check
	// that cancellation is what ended it: nothing further is presented.
	w.vsync(t)
	time.Sleep(50 * time.Millisecond)
	if got := w.count(); got != 5 {
		t.Fatalf("the loop presented %d frames after Run returned", got)
	}
}
