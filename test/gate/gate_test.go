package gate

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

const (
	gateCSSW, gateCSSH = 1440, 900
	gateDPR            = 2
	gateDevW           = gateCSSW * gateDPR
	gateDevH           = gateCSSH * gateDPR

	gateCols      = 12
	gateRows      = 42
	gateDocH      = int32(gateRows) * frame.TileSize
	gateMaxOffset = gateDocH - gateDevH

	gateBudgetTiles = 512

	gateWarmFrames = 600
	gateWarmStep   = 100
	gateColdFrames = 30
	gateColdStep   = 200

	gateQueue = 4096

	gateComposerBitmaps = 2

	gateTextRuns = 1200
	gateSeed     = 7
)

var gateSize = frame.Size{W: gateDevW, H: gateDevH}

var gateBackground = frame.RGB(9, 9, 9)

type harness struct {
	tb   testing.TB
	spec paint.SceneSpec
	dl   *paint.LayerDL
	l    *frame.Layer
	pool *raster.Pool
	s    *raster.Scheduler
	c    *surface.Composer
	w    *headless.Window
	clk  *headless.ManualClock

	dir    int32
	frames int
}

func newHarness(tb testing.TB, spec paint.SceneSpec, fn raster.RasterFunc) *harness {
	tb.Helper()
	spec.Cols, spec.Rows, spec.BudgetTiles = gateCols, gateRows, gateBudgetTiles
	if fn == nil {
		fonts, err := raster.NewFonts()
		if err != nil {
			tb.Fatalf("fonts: %v", err)
		}
		fn = raster.DefaultRaster(fonts, raster.NewGlyphAtlas(4<<20, fonts))
	}

	dl, l := paint.BuildLayer(spec)
	wp := raster.New(raster.DefaultWorkers(), gateQueue, fn)
	wp.Start(context.Background())
	c := surface.NewComposer(gateSize, frame.NewBitmapPool(gateSize, gateComposerBitmaps))
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

func (h *harness) close() {
	_ = h.pool.Close()
	_ = h.w.Close()
}

func sceneSpec(prealloc int32) paint.SceneSpec {
	return paint.SceneSpec{
		DocHeight:     gateDocH,
		TextRuns:      gateTextRuns,
		Seed:          gateSeed,
		Checkerboard:  true,
		PreallocTiles: prealloc,
	}
}

func (h *harness) frame(ev surface.Event) (surface.FrameWork, []frame.TileCoord) {
	h.tb.Helper()
	plan, needed := h.s.BeginFrame(ev)
	if plan == nil {
		h.tb.Fatal("BeginFrame returned no plan after SetPlan")
	}
	h.c.Resize(plan.Viewport.Size)
	work := h.s.Submit(needed)
	damage := h.s.ComposeInto(h.c.Backing, h.c, plan, needed)
	if err := h.w.Present(h.c.Backing, damage); err != nil {
		h.tb.Fatalf("present: %v", err)
	}
	h.frames++
	return work, needed
}

func (h *harness) vsync(dy int32) surface.Event {
	return surface.Event{Kind: surface.EvVsync, Delta: frame.Point{Y: dy}, At: h.clk.Now()}
}

func (h *harness) scroll(dy int32) (surface.FrameWork, []frame.TileCoord) {
	return h.frame(h.vsync(dy))
}

func (h *harness) travel(dy int32) (surface.FrameWork, []frame.TileCoord) {
	h.tb.Helper()
	step := dy * h.dir
	at := h.s.Viewport().Offset.Y
	if at+step > gateMaxOffset || at+step < 0 {
		h.dir = -h.dir
		step = dy * h.dir
	}
	return h.scroll(step)
}

func (h *harness) pump() surface.FrameWork {
	_, needed := h.s.BeginFrame(h.vsync(0))
	return h.s.Submit(needed)
}

func (h *harness) untilQuiet() {
	h.tb.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		w := h.pump()
		if w.Needed == 0 && h.l.Grid.Stats().PaintingBytes == 0 {
			return
		}
		if time.Now().After(deadline) {
			h.tb.Fatalf("raster never settled: work %+v grid %+v", w, h.l.Stats())
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *harness) scrollTo(y int32) {
	h.tb.Helper()
	h.mustScroll(y - h.s.Viewport().Offset.Y)
	if got := h.s.Viewport().Offset.Y; got != y {
		h.tb.Fatalf("scrollTo(%d) landed at %d", y, got)
	}
}

func (h *harness) mustScroll(dy int32) (surface.FrameWork, []frame.TileCoord) {
	h.tb.Helper()
	at := h.s.Viewport().Offset.Y
	work, needed := h.scroll(dy)
	if dy != 0 && h.s.Viewport().Offset.Y == at {
		h.tb.Fatalf("a scroll of %d device pixels did not move the viewport", dy)
	}
	return work, needed
}

func (h *harness) warmUp() {
	h.tb.Helper()
	for y := int32(0); y <= gateMaxOffset; y += frame.TileSize {
		h.scrollTo(y)
		h.untilQuiet()
	}
	h.scrollTo(0)
	h.untilQuiet()
}

func newWarmScene(tb testing.TB) *harness {
	tb.Helper()
	h := newHarness(tb, sceneSpec(0), nil)
	h.warmUp()
	return h
}

func (h *harness) stats() raster.Stats { return h.s.Stats() }

func allocTotals(runs int, f func()) (avg float64, mallocs, bytes uint64) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	runtime.GC()
	f()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < runs; i++ {
		f()
	}
	runtime.ReadMemStats(&after)
	mallocs = after.Mallocs - before.Mallocs
	bytes = after.TotalAlloc - before.TotalAlloc
	return float64(mallocs / uint64(runs)), mallocs, bytes
}

type allocWindow struct {
	avg     float64
	mallocs uint64
	bytes   uint64
}

func zeroAllocVerdict(what string, frames int, first, second allocWindow) (string, bool) {
	if first.mallocs == 0 {
		return "", false
	}
	if second.mallocs == 0 {
		return fmt.Sprintf("%s: %d allocations (%d bytes) in the first %d-frame window and none in a second on the same warm path, so those %d were the runtime's bookkeeping rather than the frame path's work",
			what, first.mallocs, first.bytes, frames, first.mallocs), false
	}
	return fmt.Sprintf("%s: %d allocations (%d bytes) in the first %d-frame window and %d (%d bytes) in a second on the same warm path, which averaged %g and %g allocs/op: a warm frame path that allocates in both windows allocates whenever it can draw",
		what, first.mallocs, first.bytes, frames, second.mallocs, second.bytes, first.avg, second.avg), true
}

func assertZeroAllocs(t *testing.T, what string, frames int, f func()) {
	t.Helper()
	avg, mallocs, bytes := allocTotals(frames, f)
	first := allocWindow{avg, mallocs, bytes}
	if first.mallocs == 0 {
		if first.avg != 0 {
			t.Errorf("%s: allocs/op = %g with a zero total, which cannot both be true", what, avg)
		}
		return
	}
	avg, mallocs, bytes = allocTotals(frames, f)
	second := allocWindow{avg, mallocs, bytes}
	msg, fail := zeroAllocVerdict(what, frames, first, second)
	if fail {
		t.Errorf("%s", msg)
		return
	}
	t.Logf("%s", msg)
}

func assertBurstAllocatesNothing(t *testing.T, what string, frames int, build func() (run func(), done func())) {
	t.Helper()
	type stray struct {
		mallocs, bytes uint64
		avg            float64
	}
	var first stray
	for try := 0; try < 2; try++ {
		run, closeHarness := build()
		avg, mallocs, bytes := allocTotals(frames, run)
		closeHarness()
		if mallocs == 0 {
			if try > 0 {
				t.Logf("%s: the first burst reported %d allocations (%d bytes) and a freshly built one reported none, so those %d were not the frame path's",
					what, first.mallocs, first.bytes, first.mallocs)
			}
			if avg != 0 {
				t.Errorf("%s: allocs/op = %g with a zero total, which cannot both be true", what, avg)
			}
			return
		}
		if try == 0 {
			first = stray{mallocs, bytes, avg}
		}
	}
	t.Errorf("%s: %d allocations (%d bytes) across %d frames on a second freshly built frame path, which averaged %g allocs/op: %g bytes per allocation, so the frame path makes these rather than the runtime making them for somebody else",
		what, first.mallocs, first.bytes, frames, first.avg, float64(first.bytes)/float64(first.mallocs))
}
