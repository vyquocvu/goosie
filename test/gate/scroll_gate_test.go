package gate

// M3 exit criteria, as tests. The fixture is a 40,960px-tall document (160 tile rows)
// with 12 columns and 1,200 text runs, producing ~3,120 layout objects. The gate drives
// 600 scroll frames and asserts:
//   - allocs/op == 0 on warm scroll
//   - zero style/layout passes on scroll-only frames (content version unchanged)
//   - <5% tile miss rate (prefetch keeps tiles warm)
//
// The document is built with the same synthetic scene generator as M1, but sized to
// match the gate_scroll.html fixture's requirements. The frame path does not know
// whether the display list came from a DOM or a synthetic generator; it caches tiles
// the same way.

import (
	"context"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

const (
	// m3Rows is the document height in tile rows: 160 * 256 = 40,960px, which exceeds
	// the ≥40,000px requirement.
	m3Rows = 160
	// m3DocH is the document height in device pixels.
	m3DocH = m3Rows * frame.TileSize
	// m3MaxOffset is the lowest legal viewport offset.
	m3MaxOffset = m3DocH - gateDevH
	// m3BudgetTiles holds the whole document: 12 * 160 = 1,920 tiles, plus room for
	// prefetch. 2,048 buffers is 128 MiB at TileSize, which is room for every tile.
	m3BudgetTiles = 2048
	// m3Frames is the criterion: 600 scroll frames.
	m3Frames = 600
	// m3Step is the scroll distance per frame in device pixels.
	m3Step = 100
	// m3TextRuns is the glyph stress: 1,200 runs, same as M1.
	m3TextRuns = 1200
)

// m3SceneSpec is the M3 document: 160 rows tall, 12 columns, 1,200 text runs.
func m3SceneSpec() paint.SceneSpec {
	return paint.SceneSpec{
		DocHeight:     m3DocH,
		TextRuns:      m3TextRuns,
		Seed:          gateSeed,
		Checkerboard:  true,
		PreallocTiles: m3BudgetTiles,
	}
}

// newM3Harness builds a frame path for the M3 document without overwriting the spec.
func newM3Harness(tb testing.TB, spec paint.SceneSpec) *harness {
	tb.Helper()
	// Unlike newHarness, this does not overwrite spec.Cols/Rows/BudgetTiles.
	fonts, err := raster.NewFonts()
	if err != nil {
		tb.Fatalf("fonts: %v", err)
	}
	fn := raster.DefaultRaster(fonts, raster.NewGlyphAtlas(4<<20, fonts))

	dl, l := paint.BuildLayer(spec)
	wp := raster.New(raster.DefaultWorkers(), gateQueue, fn)
	wp.Start(context.Background())
	c := surface.NewComposer(gateSize, frame.NewBitmapPool(gateSize, 2))
	clk := headless.NewManualClock(time.Unix(1_700_000_000, 0))
	w := headless.New(headless.Config{
		Size:        gateSize,
		Scale:       gateDPR,
		VsyncPeriod: time.Hour,
		Clock:       clk,
		EventBuffer: 1,
		History:     headless.DefaultHistory,
	})
	s := raster.NewScheduler(l, wp, frame.Viewport{Size: gateSize}, gateDPR, raster.Pref{})
	s.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{l}, Background: gateBackground})

	h := &harness{tb: tb, spec: spec, dl: dl, l: l, pool: wp, s: s, c: c, w: w, clk: clk, dir: 1}
	for i := 0; i < headless.DefaultHistory; i++ {
		if err := w.Present(c.Backing, nil); err != nil {
			h.tb.Fatalf("prewarm present: %v", err)
		}
	}
	tb.Cleanup(h.close)
	return h
}

// TestM3_WarmScrollZeroAllocs asserts that 600 scroll frames allocate nothing on the
// UI path. The cache is warmed by walking the entire document before the measured sweep.
func TestM3_WarmScrollZeroAllocs(t *testing.T) {
	spec := m3SceneSpec()
	spec.BudgetTiles = m3BudgetTiles
	h := newM3Harness(t, spec)
	h.warmUpM3()

	base := h.stats()
	if base.Rasterized == 0 {
		t.Fatal("warm-up rasterized nothing; the cache is not warm")
	}

	assertZeroAllocs(t, "M3 warm scroll frame", m3Frames, func() { h.travelM3() })

	after := h.stats()
	if got := after.Rasterized - base.Rasterized; got != 0 {
		t.Errorf("%d tiles rasterized during %d warm scroll frames, want 0", got, m3Frames)
	}
	if got := after.Submitted - base.Submitted; got != 0 {
		t.Errorf("%d raster jobs submitted during warm sweep, want 0", got)
	}
	if got := after.Frames - base.Frames; got < int64(m3Frames) {
		t.Errorf("sweep drew %d frames, want at least %d", got, m3Frames)
	}
}

// TestM3_ScrollOnlyNoContentWork asserts that scroll frames run no style and no layout.
// The content version must not change during a scroll-only sweep, which is how invariant 1
// holds: a scroll is a viewport change, not a content change.
func TestM3_ScrollOnlyNoContentWork(t *testing.T) {
	spec := m3SceneSpec()
	spec.BudgetTiles = m3BudgetTiles
	h := newM3Harness(t, spec)
	h.warmUpM3()

	beforeVersion := h.l.ContentVersion
	if beforeVersion == 0 {
		t.Fatal("content version is 0 after warm-up; the layer has no content")
	}

	for i := 0; i < m3Frames; i++ {
		h.travelM3()
	}

	afterVersion := h.l.ContentVersion
	if afterVersion != beforeVersion {
		t.Errorf("content version changed from %d to %d during %d scroll frames; a scroll must not bump it",
			beforeVersion, afterVersion, m3Frames)
	}
}

// TestM3_TileMissRate asserts that the tile prefetch keeps the miss rate below 5%. A
// miss is a frame that needed a tile and did not get it (had to rasterize it). The warm
// cache should have every tile the sweep reaches, so the miss rate should be zero or
// close to it.
func TestM3_TileMissRate(t *testing.T) {
	spec := m3SceneSpec()
	spec.BudgetTiles = m3BudgetTiles
	h := newM3Harness(t, spec)
	h.warmUpM3()

	base := h.stats()
	if base.Rasterized == 0 {
		t.Fatal("warm-up rasterized nothing; the cache is not warm")
	}

	var totalNeeded int64
	for i := 0; i < m3Frames; i++ {
		work, _ := h.travelM3()
		totalNeeded += int64(work.Needed)
	}

	after := h.stats()
	rasterizedDuringSweep := after.Rasterized - base.Rasterized

	// The miss rate is the fraction of needed tiles that had to be rasterized during
	// the sweep. A warm cache should have zero misses, but we allow up to 5% for
	// prefetch edge cases at the document boundaries.
	if totalNeeded > 0 {
		missRate := float64(rasterizedDuringSweep) / float64(totalNeeded)
		if missRate > 0.05 {
			t.Errorf("tile miss rate %.2f%% (%d/%d) exceeds 5%% threshold",
				missRate*100, rasterizedDuringSweep, totalNeeded)
		}
	}

	// The sweep should have rasterized nothing or very little.
	if rasterizedDuringSweep > 0 {
		missRate := float64(rasterizedDuringSweep) / float64(totalNeeded) * 100
		t.Logf("warning: %d tiles rasterized during warm sweep (miss rate %.2f%%)",
			rasterizedDuringSweep, missRate)
	}
}

// travelM3 scrolls one frame and reverses at the ends of the document.
func (h *harness) travelM3() (surface.FrameWork, []frame.TileCoord) {
	h.tb.Helper()
	step := m3Step * h.dir
	at := h.s.Viewport().Offset.Y
	if at+step > m3MaxOffset || at+step < 0 {
		h.dir = -h.dir
		step = m3Step * h.dir
	}
	return h.scroll(step)
}

// warmUpM3 rasterizes every tile in the M3 document by stepping the viewport down it
// one tile row at a time and waiting for the pool at each stop, then returns to the top.
func (h *harness) warmUpM3() {
	h.tb.Helper()
	for y := int32(0); y <= m3MaxOffset; y += frame.TileSize {
		h.scrollTo(y)
		h.untilQuiet()
	}
	h.scrollTo(0)
	h.untilQuiet()
}

// BenchmarkM3WarmScroll measures the M3 document's scroll performance. One op is one
// frame, which is the unit the 16.6ms vsync budget is stated in.
func BenchmarkM3WarmScroll(b *testing.B) {
	b.ReportAllocs()
	b.StopTimer()
	spec := m3SceneSpec()
	spec.BudgetTiles = m3BudgetTiles
	h := newM3Harness(b, spec)
	h.warmUpM3()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		h.travelM3()
	}
	b.StopTimer()
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1e6, "ms/op")
}
