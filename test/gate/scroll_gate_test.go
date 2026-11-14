package gate

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
	m3Rows        = 160
	m3DocH        = m3Rows * frame.TileSize
	m3MaxOffset   = m3DocH - gateDevH
	m3BudgetTiles = 2048
	m3Frames      = 600
	m3Step        = 100
	m3TextRuns    = 1200
)

func m3SceneSpec() paint.SceneSpec {
	return paint.SceneSpec{
		DocHeight:     m3DocH,
		TextRuns:      m3TextRuns,
		Seed:          gateSeed,
		Checkerboard:  true,
		PreallocTiles: m3BudgetTiles,
	}
}

func newM3Harness(tb testing.TB, spec paint.SceneSpec) *harness {
	tb.Helper()
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

// qc:BR-174
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

	if totalNeeded > 0 {
		missRate := float64(rasterizedDuringSweep) / float64(totalNeeded)
		if missRate > 0.05 {
			t.Errorf("tile miss rate %.2f%% (%d/%d) exceeds 5%% threshold",
				missRate*100, rasterizedDuringSweep, totalNeeded)
		}
	}

	if rasterizedDuringSweep > 0 {
		missRate := float64(rasterizedDuringSweep) / float64(totalNeeded) * 100
		t.Logf("warning: %d tiles rasterized during warm sweep (miss rate %.2f%%)",
			rasterizedDuringSweep, missRate)
	}
}

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

func (h *harness) warmUpM3() {
	h.tb.Helper()
	for y := int32(0); y <= m3MaxOffset; y += frame.TileSize {
		h.scrollTo(y)
		h.untilQuiet()
	}
	h.scrollTo(0)
	h.untilQuiet()
}

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
