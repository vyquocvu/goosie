package frame_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
)

func tilePool() *frame.BitmapPool {
	return frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, 4096)
}

func TestCoordForAtBoundariesAndNegatives(t *testing.T) {
	tests := []struct {
		p    frame.Point
		want frame.TileCoord
	}{
		{frame.Point{X: 0, Y: 0}, frame.TileCoord{Col: 0, Row: 0}},
		{frame.Point{X: 255, Y: 255}, frame.TileCoord{Col: 0, Row: 0}},
		{frame.Point{X: 256, Y: 0}, frame.TileCoord{Col: 1, Row: 0}},
		{frame.Point{X: 0, Y: 256}, frame.TileCoord{Col: 0, Row: 1}},
		// Go's / truncates toward zero, so without floor division x = -1 would
		// land in tile 0 alongside x = -256, and negative-origin content would
		// render into the wrong tile.
		{frame.Point{X: -1, Y: -1}, frame.TileCoord{Col: -1, Row: -1}},
		{frame.Point{X: -256, Y: -256}, frame.TileCoord{Col: -1, Row: -1}},
		{frame.Point{X: -257, Y: 512}, frame.TileCoord{Col: -2, Row: 2}},
	}
	for _, tc := range tests {
		if got := frame.CoordFor(tc.p, frame.TileSize); got != tc.want {
			t.Errorf("CoordFor(%v) = %v, want %v", tc.p, got, tc.want)
		}
	}
}

func TestTileCoordRectRoundTrips(t *testing.T) {
	c := frame.TileCoord{Col: 3, Row: -2}
	r := c.Rect(frame.TileSize)
	if want := frame.Rect4(768, -512, 1024, -256); r != want {
		t.Fatalf("Rect = %v, want %v", r, want)
	}
	for _, p := range []frame.Point{{X: 768, Y: -512}, {X: 1023, Y: -257}} {
		if got := frame.CoordFor(p, frame.TileSize); got != c {
			t.Fatalf("CoordFor(%v) = %v, want the tile that covers it (%v)", p, got, c)
		}
	}
}

func TestVisibleCoordsAtRetinaViewport(t *testing.T) {
	// 3024x1900 device px is a 16-inch MacBook Pro in fullscreen. 12x8 = 96
	// tiles is the number the design document budgets against, so it is asserted
	// rather than measured at runtime.
	g := frame.NewGrid(frame.Rect4(0, 0, 4096, 8192), frame.TileSize, 1<<30, tilePool())
	vp := frame.Viewport{Size: frame.Size{W: 3024, H: 1900}}
	coords := g.VisibleCoords(vp, nil)
	if len(coords) != 96 {
		t.Fatalf("visible tiles = %d, want 12x8 = 96", len(coords))
	}
	// A viewport one device px wider than a whole number of tile columns costs an
	// extra column: the partial column is still visible and must be rasterized.
	vp.Size.W = 3073
	if got := len(g.VisibleCoords(vp, nil)); got != 104 {
		t.Fatalf("visible tiles at 3073x1900 = %d, want 13x8 = 104", got)
	}
}

func TestVisibleCoordsClampedToLayerExtent(t *testing.T) {
	// A short document must not produce coordinates that no content can fill;
	// those tiles would be rasterized, billed to the budget, and present forever
	// as blank.
	g := frame.NewGrid(frame.Rect4(0, 0, 512, 300), frame.TileSize, 1<<30, tilePool())
	vp := frame.Viewport{Size: frame.Size{W: 2048, H: 2048}}
	coords := g.VisibleCoords(vp, nil)
	if len(coords) != 4 {
		t.Fatalf("visible tiles = %d (%v), want 2 cols x 2 rows over a 512x300 extent", len(coords), coords)
	}
	for _, c := range coords {
		if !g.Inside(c) {
			t.Fatalf("coordinate %v lies outside the layer extent", c)
		}
	}
	if g.Needs(frame.TileCoord{Col: 40, Row: 40}, 1) {
		t.Fatal("Needs reported true for a coordinate outside the layer")
	}
}

func warm(t *testing.T, g *frame.Grid, vp frame.Viewport, version uint64) []frame.TileCoord {
	t.Helper()
	coords := g.VisibleCoords(vp, nil)
	for _, c := range coords {
		if !g.Needs(c, version) {
			t.Fatalf("tile %v reported valid before being rasterized", c)
		}
		bmp, ok := g.Acquire(c)
		if !ok {
			t.Fatalf("Acquire(%v) refused a visible coordinate", c)
		}
		bmp.FillRect(bmp.Bounds(), frame.RGB(1, 2, 3), nil)
		if !g.MarkValid(c, version, bmp) {
			t.Fatalf("MarkValid(%v) rejected a tile that was just acquired", c)
		}
	}
	return coords
}

func TestWarmScrollRasterizesNoTiles(t *testing.T) {
	// This is the raster half of invariant 1 and the reason the whole design
	// exists: once the cache holds the content a scroll will reveal, scrolling
	// must reuse those tiles and do zero rasterization.
	g := frame.NewGrid(frame.Rect4(0, 0, 1024, 8192), frame.TileSize, 1<<30, tilePool())
	// Warm a band taller than the viewport used to measure, so the scrolled-to
	// rows are content the cache has already paid for. This is the prefetch case
	// the design budgets for, not an artificial one.
	warm(t, g, frame.Viewport{Size: frame.Size{W: 1024, H: 4096}}, 1)
	if got := g.Stats().Rasterized; got != 64 {
		t.Fatalf("initial warm-up rasterized %d tiles, want 4 cols x 16 rows = 64", got)
	}

	vp := frame.Viewport{Size: frame.Size{W: 1024, H: 2560}}
	for _, offset := range []int32{0, 512, 1024, 1536} {
		vp.Offset.Y = offset
		before := g.Stats().Rasterized
		coords := g.VisibleCoords(vp, nil)
		if len(coords) != 40 {
			t.Fatalf("offset %d: visible tiles = %d, want 10 rows x 4 cols = 40", offset, len(coords))
		}
		for _, c := range coords {
			if g.Needs(c, 1) {
				t.Fatalf("offset %d: tile %v needs rasterization over warm content", offset, c)
			}
			g.Touch(c)
		}
		if got := g.Stats().Rasterized; got != before {
			t.Fatalf("offset %d: warm scroll rasterized %d tiles", offset, got-before)
		}
	}
	if got := g.Stats().Reused; got < 40 {
		t.Fatalf("Reused = %d, want at least one touch per visible tile", got)
	}

	// A scroll that straddles the edge of the warmed band asks for exactly the
	// rows it never warmed: visible rows 8..17, of which 8..15 are warm, so the
	// two cold rows are 8 tiles rather than the whole viewport's 40.
	vp.Offset.Y = 2048
	need := 0
	for _, c := range g.VisibleCoords(vp, nil) {
		if g.Needs(c, 1) {
			need++
		}
	}
	if need != 8 {
		t.Fatalf("partially cold scroll requested %d tiles, want the 8 newly revealed", need)
	}
}

func TestInvalidateMarksOnlyIntersectingTiles(t *testing.T) {
	g := frame.NewGrid(frame.Rect4(0, 0, 1024, 4096), frame.TileSize, 1<<30, tilePool())
	vp := frame.Viewport{Size: frame.Size{W: 1024, H: 1024}}
	warm(t, g, vp, 1)

	// One dirty band of content one row tall, spanning all four columns.
	n := g.Invalidate(frame.Rect4(0, 256, 1000, 300))
	if n != 4 {
		t.Fatalf("Invalidate marked %d tiles, want the 4 in row 1", n)
	}
	for col := int32(0); col < 4; col++ {
		c := frame.TileCoord{Col: col, Row: 1}
		if !g.Needs(c, 1) {
			t.Fatalf("tile %v in the dirty row still reports valid", c)
		}
		// Stale pixels survive: presenting a slightly old tile is the mechanism
		// behind "present now, fix next vsync".
		if !g.Peek(c).Blittable() {
			t.Fatalf("tile %v dropped its pixels when invalidated", c)
		}
	}
	for col := int32(0); col < 4; col++ {
		if c := (frame.TileCoord{Col: col, Row: 2}); g.Needs(c, 1) {
			t.Fatalf("tile %v outside the dirty rect was invalidated", c)
		}
	}
	// An empty dirty rect stales nothing.
	if got := g.Invalidate(frame.Rect{}); got != 0 {
		t.Fatalf("Invalidate(empty) marked %d tiles", got)
	}
}

func TestBudgetEvictsLeastRecentlyUsedAndPlateaus(t *testing.T) {
	one := frame.TileSizeBytes()
	g := frame.NewGrid(frame.Rect4(0, 0, 1024, 8192), frame.TileSize, 4*one, tilePool())

	// Four tiles fill the budget exactly.
	for row := int32(0); row < 4; row++ {
		c := frame.TileCoord{Col: 0, Row: row}
		bmp, _ := g.Acquire(c)
		g.MarkValid(c, 1, bmp)
	}
	if got := g.Stats().Bytes; got != 4*one {
		t.Fatalf("Bytes = %d, want the budget %d", got, 4*one)
	}

	// Touch row 3 so row 0 becomes the oldest, then demand a fifth tile.
	g.Touch(frame.TileCoord{Col: 0, Row: 3})
	c := frame.TileCoord{Col: 0, Row: 4}
	bmp, _ := g.Acquire(c)
	g.MarkValid(c, 1, bmp)

	st := g.Stats()
	if st.Evictions != 1 {
		t.Fatalf("Evictions = %d, want 1", st.Evictions)
	}
	if g.Peek(frame.TileCoord{Col: 0, Row: 0}).Pixels != nil {
		t.Fatal("budget eviction took the wrong victim; the least-recently-used tile must go")
	}
	if g.Peek(frame.TileCoord{Col: 0, Row: 3}).Pixels == nil {
		t.Fatal("budget eviction took a recently touched tile")
	}
	if st.Bytes > st.Budget {
		t.Fatalf("Bytes %d exceeded Budget %d", st.Bytes, st.Budget)
	}

	// A long scroll must not grow the footprint past the ceiling.
	for row := int32(5); row < 40; row++ {
		cc := frame.TileCoord{Col: 0, Row: row}
		b, _ := g.Acquire(cc)
		g.MarkValid(cc, 1, b)
		if got := g.Stats(); got.Bytes > got.Budget {
			t.Fatalf("after %d tiles, Bytes = %d > Budget = %d", row, got.Bytes, got.Budget)
		}
	}
}

func TestPrefetchCoordsAheadOfScroll(t *testing.T) {
	g := frame.NewGrid(frame.Rect4(0, 0, 1024, 8192), frame.TileSize, 1<<30, tilePool())
	vp := frame.Viewport{Size: frame.Size{W: 1024, H: 1024}}
	ahead := g.PrefetchCoords(vp, frame.Point{Y: 1}, 2, nil)
	if len(ahead) != 8 {
		t.Fatalf("prefetch ring = %d tiles, want 2 rows x 4 cols", len(ahead))
	}
	for _, c := range ahead {
		if c.Row < 4 {
			t.Fatalf("prefetch %v is inside the viewport, not ahead of it", c)
		}
	}
	if got := g.PrefetchCoords(vp, frame.Point{Y: 1}, 0, nil); got != nil {
		t.Fatalf("zero prefetch rows produced %d coordinates", len(got))
	}
	if got := g.PrefetchCoords(vp, frame.Point{}, 2, nil); got != nil {
		t.Fatalf("no scroll direction produced %d coordinates", len(got))
	}
	// Scrolling up must prefetch above, and never off the top of the document.
	above := g.PrefetchCoords(vp, frame.Point{Y: -1}, 4, nil)
	if len(above) != 0 {
		t.Fatalf("prefetch above the document origin produced %d coordinates", len(above))
	}
}

func TestLateWorkerResultIsDropped(t *testing.T) {
	// A worker that finishes after its tile was evicted must not write into a
	// buffer the UI thread has handed to someone else.
	g := frame.NewGrid(frame.Rect4(0, 0, 512, 512), frame.TileSize, 1<<30, tilePool())
	stale, _ := g.Acquire(frame.TileCoord{Col: 0, Row: 0})
	g.MarkValid(frame.TileCoord{Col: 0, Row: 0}, 1, stale)
	g.SetBudget(frame.TileSizeBytes()) // evicts everything but one holder
	fresh, _ := g.Acquire(frame.TileCoord{Col: 1, Row: 1})
	g.MarkValid(frame.TileCoord{Col: 1, Row: 1}, 1, fresh)

	if g.MarkValid(frame.TileCoord{Col: 0, Row: 0}, 1, stale) {
		t.Fatal("a result for an evicted tile was accepted")
	}
	if got := g.Stats().DroppedLate; got == 0 {
		t.Fatal("dropped-late results are not counted; the gate needs this number")
	}
}

func TestFailedTileRendersBlankAndIsNotRetriedForever(t *testing.T) {
	g := frame.NewGrid(frame.Rect4(0, 0, 512, 512), frame.TileSize, 1<<30, tilePool())
	c := frame.TileCoord{Col: 0, Row: 0}
	for attempt := 0; attempt < frame.MaxTileAttempts; attempt++ {
		// Acquire and abandon: the worker gave up without publishing.
		if _, ok := g.Acquire(c); !ok {
			t.Fatalf("Acquire(%v) refused a coordinate inside the layer", c)
		}
		g.MarkFailed(c)
		if !g.Failed(c) {
			t.Fatalf("tile %v not marked failed after attempt %d", c, attempt)
		}
	}
	if g.Peek(c).Blittable() {
		t.Fatal("a failed tile reported pixels worth presenting")
	}
	if got := g.Stats().Failed; got != 1 {
		t.Fatalf("Failed = %d, want the tile counted once", got)
	}
}

// A tile that failed under an old version is only blank for that version. Once
// content changes under it, the pixels it could not produce no longer exist, so
// the retry cap that gave up on it has nothing left to protect and the tile has
// to be willing to try again - otherwise one transient rasteriser panic costs a
// blank rectangle for the rest of the document's life.
func TestVersionBumpRetriesATileThatFailedUnderTheOldOne(t *testing.T) {
	g := frame.NewGrid(frame.Rect4(0, 0, 512, 512), frame.TileSize, 1<<30, tilePool())
	dead, live := frame.TileCoord{Col: 0, Row: 0}, frame.TileCoord{Col: 1, Row: 1}
	for _, c := range []frame.TileCoord{dead, live} {
		bmp, _ := g.Acquire(c)
		g.MarkValid(c, 1, bmp)
	}
	for attempt := 0; attempt < frame.MaxTileAttempts; attempt++ {
		g.Acquire(dead)
		g.MarkFailed(dead)
	}
	if !g.Failed(dead) {
		t.Fatal("setup: the tile was not given up on")
	}

	// The dirty rect covers the failed tile; the other one is outside it.
	if got := g.Advance(2, dead.Rect(frame.TileSize)); got != 0 {
		t.Fatalf("Advance staled %d tiles, want 0 - a failed tile holds no pixels to keep showing", got)
	}
	if g.Failed(dead) {
		t.Fatal("a version bump left a tile Failed inside its own dirty rect; it will never render again")
	}
	if got := g.Peek(dead).Attempts; got != 0 {
		t.Fatalf("Attempts = %d after a version bump, want the retry budget reset", got)
	}
	if !g.Needs(dead, 2) {
		t.Fatal("the retried tile reports itself current at the new version")
	}
	if g.Failed(live) || g.Needs(live, 2) {
		t.Fatal("a tile outside the dirty rect changed state; an image load would cost a full page repaint")
	}
}

func TestLayerBumpStalesOnlyDirtyTiles(t *testing.T) {
	pool := tilePool()
	l := frame.NewLayer(1, frame.Rect4(0, 0, 1024, 2048), 1<<30, pool)
	vp := frame.Viewport{Size: frame.Size{W: 1024, H: 512}}
	// Establish version 1 without staling anything, then warm the visible band at
	// that version, which is what the first frame does.
	if got := l.Bump(1, frame.Rect{}); got != 0 {
		t.Fatalf("first Bump with an empty dirty rect staled %d tiles", got)
	}
	for _, c := range l.VisibleCoords(vp, nil) {
		bmp, _ := l.Grid.Acquire(c)
		l.Grid.MarkValid(c, 1, bmp)
	}
	if got := l.Bump(1, frame.Rect4(0, 0, 10, 10)); got != 0 {
		t.Fatalf("Bump to the current version staled %d tiles, want 0", got)
	}
	// A real version bump with a one-row dirty rect.
	if got := l.Bump(2, frame.Rect4(0, 300, 1000, 320)); got != 4 {
		t.Fatalf("Bump staled %d tiles, want the 4 in row 1", got)
	}
	if l.ContentVersion != 2 {
		t.Fatalf("ContentVersion = %d, want 2", l.ContentVersion)
	}
	if !l.Needs(frame.TileCoord{Col: 0, Row: 1}) {
		t.Fatal("dirty tile still reports current after a version bump")
	}
	if l.Needs(frame.TileCoord{Col: 0, Row: 0}) {
		t.Fatal("a tile outside the dirty rect was re-rasterized; an image load would cost a full page repaint")
	}
	// An empty dirty rect on a version bump means "nothing changed visibly", which
	// the caller uses when a mutation resolved to no visual difference.
	l2 := frame.NewLayer(2, frame.Rect4(0, 0, 1024, 1024), 1<<30, pool)
	if got := l2.Bump(3, frame.Rect{}); got != 0 {
		t.Fatalf("Bump with empty dirty staled %d tiles", got)
	}
}

func TestPlanKeepsOnlyTheNewestAndCountsSuperseded(t *testing.T) {
	var p frame.Plan
	first := frame.FramePlan{Serial: 1, Viewport: frame.Viewport{Size: frame.Size{W: 100, H: 100}}}
	second := frame.FramePlan{Serial: 2, Viewport: frame.Viewport{Size: frame.Size{W: 200, H: 100}}}

	p.Publish(first)
	p.Publish(second) // supersedes: nobody presented it
	if got := p.Stats().Dropped; got != 1 {
		t.Fatalf("Dropped = %d, want 1 superseded plan", got)
	}
	latest, ok := p.Latest()
	if !ok || latest.Serial != 2 {
		t.Fatalf("Latest = %v (%v), want serial 2", latest.Serial, ok)
	}
	// Latest must not consume, or the presenter cannot re-check the plan it is
	// about to blit.
	if again, _ := p.Latest(); again.Serial != 2 {
		t.Fatal("Latest drained the slot")
	}
	taken, ok := p.Take()
	if !ok || taken.Serial != 2 {
		t.Fatalf("Take = %v (%v), want serial 2", taken.Serial, ok)
	}
	// A present plan is not a dropped plan: republishing after presentation is
	// the normal cadence and must not inflate the counter.
	p.Publish(frame.FramePlan{Serial: 3})
	if got := p.Stats(); got.Dropped != 1 || got.Published != 3 {
		t.Fatalf("stats = %+v, want Dropped 1 Published 3", got)
	}
	// Re-taking without a publish is idempotent: holding a vsync on an
	// unchanged plan is normal and cheap.
	a, _ := p.Take()
	b, _ := p.Take()
	if a.Serial != b.Serial {
		t.Fatalf("re-Take changed the plan: %d then %d", a.Serial, b.Serial)
	}
	var empty frame.Plan
	if _, ok := empty.Latest(); ok {
		t.Fatal("zero-value Plan reported a plan")
	}
	if _, ok := empty.Take(); ok {
		t.Fatal("Take on an empty Plan reported a plan")
	}
}

func TestFramePlanLayerLookup(t *testing.T) {
	pool := tilePool()
	a := frame.NewLayer(frame.LayerID(3), frame.Rect4(0, 0, 10, 10), 1<<20, pool)
	b := frame.NewLayer(frame.LayerID(9), frame.Rect4(0, 0, 10, 10), 1<<20, pool)
	p := frame.FramePlan{Layers: []*frame.Layer{a, nil, b}}
	if got := p.Layer(9); got != b {
		t.Fatalf("Layer(9) = %v", got)
	}
	if got := p.Layer(4); got != nil {
		t.Fatalf("Layer(4) = %v, want nil", got)
	}
}

func TestGridTickAndStatsSnapshots(t *testing.T) {
	g := frame.NewGrid(frame.Rect4(0, 0, 512, 512), frame.TileSize, 1<<30, tilePool())
	g.Tick()
	g.Tick()
	if g.Clock() != 2 {
		t.Fatalf("Clock = %d, want 2", g.Clock())
	}
	if g.TileSize() != frame.TileSize {
		t.Fatalf("TileSize = %d, want %d", g.TileSize(), frame.TileSize)
	}
	st := g.Stats()
	if st.Tiles != 0 || st.Valid != 0 || st.Bytes != 0 {
		t.Fatalf("fresh grid stats = %+v, want all zero", st)
	}
}

// TestEvictingAnInFlightTileWaitsForItsWorker is the buffer-ownership case the
// budget exists to create: a tile is re-rasterized, loses its presentable pixels
// to eviction while a worker is painting the replacement, and only then reports.
func TestEvictingAnInFlightTileWaitsForItsWorker(t *testing.T) {
	pool := tilePool()
	g := frame.NewGrid(frame.Rect4(0, 0, 512, 512), frame.TileSize, 1<<30, pool)
	c := frame.TileCoord{Col: 0, Row: 0}
	near := frame.TileCoord{Col: 1, Row: 0}

	first, _ := g.Acquire(c)
	if !g.MarkValid(c, 1, first) {
		t.Fatal("the first raster of a tile was refused")
	}
	// A scroll brings the tile back at an older version, so it is re-rasterized.
	// The replacement must not be the buffer still being presented, or the worker
	// and the compositor would share bytes.
	onOrder, _ := g.Acquire(c)
	if onOrder == first {
		t.Fatal("Acquire handed back the tile's presentable pixels as its paint target")
	}
	if !g.InFlight(c) {
		t.Fatal("Acquire left the tile looking idle")
	}
	second, _ := g.Acquire(near)
	g.MarkValid(near, 1, second)

	g.SetBudget(frame.TileSizeBytes())
	if g.Pixels(c) != nil {
		t.Fatal("the in-flight tile kept its pixels; eviction skipped the LRU tail")
	}
	if got := g.Stats().PaintingBytes; got != frame.TileSizeBytes() {
		t.Fatalf("PaintingBytes = %d, want one tile still held for its worker", got)
	}
	if got := g.Stats().Bytes; got != frame.TileSizeBytes() {
		t.Fatalf("Bytes = %d, want only the in-flight tile counted against the budget", got)
	}
	// The point of the whole exercise: a buffer a worker owns is not up for reuse.
	for i := 0; i < 2; i++ {
		if got := pool.Acquire(); got == onOrder {
			t.Fatal("an in-flight buffer was recycled to another tile")
		}
	}

	// The worker reports late. The tile lost its pixels in the meantime, but the
	// result is still this grid's own paint target, so it is installed rather than
	// thrown away.
	if !g.MarkValid(c, 1, onOrder) {
		t.Fatal("a result for a tile evicted mid-flight was refused")
	}
	if got := g.Stats().PaintingBytes; got != 0 {
		t.Fatalf("PaintingBytes = %d after the worker reported, want 0", got)
	}
	if g.Pixels(c) != onOrder {
		t.Fatal("the late result was not installed")
	}
	// A buffer the tile no longer has on order is stale news, whoever holds it.
	if g.MarkValid(c, 1, first) {
		t.Fatal("a foreign buffer was accepted as a raster result")
	}
	if got := g.Stats().DroppedLate; got == 0 {
		t.Fatal("the refused result was not counted")
	}
}

func TestReleaseHandsBackOnlyWhatTheGridDoesNotOwn(t *testing.T) {
	pool := tilePool()
	g := frame.NewGrid(frame.Rect4(0, 0, 512, 512), frame.TileSize, 1<<30, pool)
	c := frame.TileCoord{Col: 0, Row: 0}
	pixels, _ := g.Acquire(c)
	g.MarkValid(c, 1, pixels)

	// The tile still points at this buffer, so a release would put the pool and the
	// grid on the same memory.
	if g.Release(c, pixels) {
		t.Fatal("Release took back a buffer the tile still owns")
	}
	if g.Pixels(c) != pixels {
		t.Fatal("Release disturbed the tile's pixels")
	}
	// A buffer on order is the grid's to take back: a caller whose submit was
	// refused has nowhere else to put it, and once Release says true no worker owns
	// it and the tile is free to be re-acquired.
	onOrder, _ := g.Acquire(c)
	if !g.Release(c, onOrder) {
		t.Fatal("Release kept an in-flight buffer claimed")
	}
	if g.InFlight(c) {
		t.Fatal("Release left the tile marked in flight")
	}
	if got := pool.Acquire(); got != onOrder {
		t.Fatal("the released paint buffer did not go back to the pool")
	}
	// A buffer the grid has no claim on is not the grid's to keep.
	far := frame.TileCoord{Col: 9, Row: 9}
	if !g.Release(far, pixels) {
		t.Fatal("Release refused a coordinate outside the grid")
	}
	if g.Release(c, nil) {
		t.Fatal("Release of a nil buffer reported a return")
	}
}
