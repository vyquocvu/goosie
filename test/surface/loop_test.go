package surface_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

type presentRecord struct {
	color  frame.Color
	size   frame.Size
	damage []frame.Rect
}

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

func (w *fakeWindow) SetIME(bool) {}

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
	gate      chan struct{}
}

func tileColor(c frame.TileCoord, version uint64) frame.Color {
	return frame.RGB(byte(version*37+1), byte(c.Col*11+7), byte(c.Row*13+3))
}

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
	vp := p.Viewport
	if s.layer != nil {
		b := s.layer.Bounds
		vp.Offset = frame.Point{
			X: clamp32(s.scroll.X, 0, max(0, b.X1-vp.Size.W)),
			Y: clamp32(s.scroll.Y, 0, max(0, b.Y1-vp.Size.H)),
		}
	} else {
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
	for {
		d, ok := s.pool.Poll()
		if !ok {
			break
		}
		if d.Err != nil {
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
			if t == nil || t.Pixels == nil {
				continue
			}
			dst := coord.Rect(g.TileSize()).Translate(-vp.Offset.X, -vp.Offset.Y)
			s.blits = append(s.blits, surface.TileBlit{Pixels: t.Pixels, Dst: dst})
		}
	}
	s.damage[0] = frame.Rect4(0, 0, plan.Viewport.Size.W, plan.Viewport.Size.H)
	return c.Compose(plan.Background, s.blits, s.damage[:], &s.writes)
}

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

func newScene(t *testing.T, cols, rows int32) (*frame.Layer, *paint.LayerDL) {
	t.Helper()
	bounds := frame.Rect4(0, 0, cols*frame.TileSize, rows*frame.TileSize)
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, int(cols*rows)*2+8)
	l := frame.NewLayer(frame.LayerID(1), bounds, 2*int64(cols*rows)*frame.TileSizeBytes(), pool)
	dl := fillList(bounds, 1)
	l.SetContent(dl)
	return l, dl
}

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

func newScheduler(layer *frame.Layer, dl *paint.LayerDL, vp frame.Viewport) *tileScheduler {
	return &tileScheduler{
		slot:  newSlot(published(1, vp, frame.RGB(9, 9, 9), layer)),
		layer: layer,
		dl:    dl,
	}
}

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

func waitForCommitted(t *testing.T, loop *surface.Loop, n int64) {
	t.Helper()
	waitFor(t, "the loop counters to catch up", func() bool { return loop.Stats().Frames >= n })
}

func newComposerFor(vp frame.Viewport) *surface.Composer {
	return surface.NewComposer(vp.Size, frame.NewBitmapPool(vp.Size, 4))
}

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
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := loop.Stats()
	if got.Frames != ticks || got.Presents != ticks {
		t.Fatalf("frames/presents = %d/%d, want %d/%d: a vsync that does not present is a dropped frame",
			got.Frames, got.Presents, ticks, ticks)
	}
	if got := loop.Composer().Backing.Size(); got != vp.Size {
		t.Fatalf("backing store = %v, want %v", got, vp.Size)
	}
	if got := l.Stats(); got.Valid != 4 || got.Failed != 0 {
		t.Fatalf("grid = %+v, want 4 valid tiles and no failures", got)
	}
	if got := pool.Stats(); got.Rasterized != 4 {
		t.Fatalf("workers ran %d jobs, want the 4 tiles the first frame needed", got.Rasterized)
	}
}

func TestWarmScrollRasterizesNoTiles(t *testing.T) {
	vp := frame.Viewport{Size: frame.Size{W: 512, H: 512}}
	l, dl := newScene(t, 2, 26)
	s := newScheduler(l, dl, vp)
	s.warm(t)
	pool := s.startPool(t, 2, 64)

	w := newFakeWindow(128)
	loop := surface.NewLoop(w, s, newComposerFor(vp), nil)
	stop := start(t, loop)

	const down = 24
	for i := 0; i < down; i++ {
		w.scroll(t, frame.TileSize)
		w.vsync(t)
	}
	waitFor(t, "24 presents", func() bool { return w.count() == down })
	waitForCommitted(t, loop, down)
	if got := s.offsetY(); got != down*frame.TileSize {
		t.Fatalf("scroll offset = %d, want %d: the deltas did not reach the viewport", got, down*frame.TileSize)
	}
	if got := loop.Stats(); got.Reused != int64(down)*4 {
		t.Fatalf("Reused = %d after %d tile-aligned frames, want %d: four cached tiles per frame went uncounted",
			got.Reused, down, int64(down)*4)
	}

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

	gate := make(chan struct{})
	s.gate = gate
	s.dl = fillList(l.Bounds, 2)
	pool := s.startPool(t, 1, 64)
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
	waitForCommitted(t, loop, ticks)

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
	if got := l.Stats(); got.Valid != 4 || got.Stale != 4 || got.Failed != 0 {
		t.Fatalf("grid after the late tiles reported = %+v, want 4 valid and 4 stale off screen", got)
	}
}

// qc:BR-171
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
	for i, bg := range []frame.Color{red, green, blue} {
		slot.Publish(published(uint64(i+1), vp, bg))
	}

	w.vsync(t)
	waitFor(t, "one present", func() bool { return w.count() == 1 })
	waitForCommitted(t, loop, 1)

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

	for i := 0; i < 6; i++ {
		w.scroll(t, 100)
	}
	w.vsync(t)
	waitFor(t, "the coalesced frame", func() bool { return w.count() == 1 })
	waitForCommitted(t, loop, 1)

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
	waitForCommitted(t, loop, 1)

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
	s.slot = &frame.Plan{}
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
	gate := make(chan struct{})
	s.gate = gate
	pool := s.startPool(t, 1, 2)
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
	waitForCommitted(t, loop, ticks)
	if got := loop.Stats(); got.Refused == 0 {
		t.Fatal("Refused = 0 with a 2-deep queue and 4 tiles needed: nothing was turned away, so this proved nothing")
	}

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
	waitForCommitted(t, loop, ticks)
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
	waitFor(t, "the four tiles to be accepted", func() bool { return loop.Stats().Rasterize == 4 })
	w.vsync(t)
	waitFor(t, "the warm frame", func() bool { return w.count() == ticks+1 })
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	marks := rec.Marks()
	if len(marks) != ticks+1 {
		t.Fatalf("recorder holds %d marks, want %d", len(marks), ticks+1)
	}
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
	w.vsync(t)
	time.Sleep(50 * time.Millisecond)
	if got := w.count(); got != 5 {
		t.Fatalf("the loop presented %d frames after Run returned", got)
	}
}
