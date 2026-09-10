package gate

import (
	"context"
	"runtime"
	"runtime/debug"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/v2/internal/frame"
	"github.com/vyquocvu/goosie/v2/internal/paint"
	"github.com/vyquocvu/goosie/v2/internal/platform/headless"
	"github.com/vyquocvu/goosie/v2/internal/raster"
	"github.com/vyquocvu/goosie/v2/internal/surface"
)

// The harness is the M1 frame path wired the way a real browser wires it - a
// synthetic scene, a layer with its grid, a worker pool, a composer, and a headless
// window that receives the presented frame - driven from one goroutine the way
// surface.Loop drives its scheduler: BeginFrame, Submit, ComposeInto, Present.
//
// It is deliberately not surface.Loop.Run. The loop needs a vsync from its window,
// and the only vsync source that is deterministic enough to gate on is a clock a test
// advances, which allocates a fresh channel per tick. That is the platform's cost, not
// the frame path's, and the criterion these tests assert is about the frame path. The
// loop's own contract - one frame per vsync, input coalesced, present after compose -
// is what surface's tests prove; here the sequence is written out so the counters and
// the allocation totals belong to the frame and nothing else.

// The geometry is the spec's, in device pixels: a 1440x900 CSS viewport at DPR 2 is
// 2880x1800, which on a 256px tile grid is 12 columns by 8 rows - 96 tiles on screen.
const (
	gateCSSW, gateCSSH = 1440, 900
	gateDPR            = 2
	gateDevW           = gateCSSW * gateDPR
	gateDevH           = gateCSSH * gateDPR

	// gateCols is the document's tile-column count, and the narrowest document that
	// covers the viewport: 11 tiles is 2816px, which is 64px short of 2880.
	gateCols = 12
	// gateRows is the document's tile-row count. The number is not arbitrary: it is
	// the smallest height that keeps every tile the warm sweep can reach inside one
	// budget, which is what lets that sweep assert that nothing rasterized. See
	// gateBudgetTiles.
	gateRows      = 42
	gateDocH      = int32(gateRows) * frame.TileSize
	gateMaxOffset = gateDocH - gateDevH // 8952: the bottom of the document

	// gateBudgetTiles has to hold the whole document, because a cache that cannot hold
	// what the measured sweep reaches is a cache that rasterizes during it. The
	// document is 12x42 tiles, so 512 buffers is room for every tile plus eight over.
	gateBudgetTiles = 512

	// gateWarmFrames and gateWarmStep are the criterion: 600 scroll frames of 100px.
	gateWarmFrames = 600
	gateWarmStep   = 100
	// gateColdFrames and gateColdStep are the other criterion: 30 frames of 200px into
	// content that has never been drawn.
	gateColdFrames = 30
	gateColdStep   = 200

	// gateQueue is deep enough that no gate frame is turned away for want of a worker.
	// Refusals are legal behaviour and are asserted to be zero rather than designed
	// around: a gate that counted refused work as redundant work would pass by being
	// unable to tell the two apart.
	gateQueue = 4096

	gateTextRuns = 1200
	gateSeed     = 7
)

// gateSize is the surface the whole gate draws into, in device pixels.
var gateSize = frame.Size{W: gateDevW, H: gateDevH}

// gateBackground is unlike any colour the scene produces, so "this pixel is still the
// page background" is an unambiguous claim about a tile that never arrived.
var gateBackground = frame.RGB(9, 9, 9)

// harness owns one frame path.
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

	// dir is the travel direction of the last measured frame, so a long sweep can be
	// reversed at the ends of the document instead of pressing against a clamp.
	dir int32
	// frames counts the frames this harness has drawn, which is how the cold benchmark
	// knows when its document has stopped being cold.
	frames int
}

// newHarness builds a frame path over one synthetic scene. fn may be nil, which means
// the real rasterizer.
func newHarness(tb testing.TB, spec paint.SceneSpec, fn raster.RasterFunc) *harness {
	tb.Helper()
	spec.Cols, spec.Rows, spec.BudgetTiles = gateCols, gateRows, gateBudgetTiles
	if fn == nil {
		fonts, err := raster.NewFonts()
		if err != nil {
			tb.Fatalf("fonts: %v", err)
		}
		// The atlas budget is per layer and is not one of M1's criteria; a tile is
		// 256px on a side, so 4 MiB holds more glyph masks than one screen of text
		// needs and evicts none of them.
		fn = raster.DefaultRaster(fonts, raster.NewGlyphAtlas(4<<20, fonts))
	}

	dl, l := paint.BuildLayer(spec)
	wp := raster.New(raster.DefaultWorkers(), gateQueue, fn)
	wp.Start(context.Background())
	c := surface.NewComposer(gateSize, frame.NewBitmapPool(gateSize, 2))
	clk := headless.NewManualClock(time.Unix(1_700_000_000, 0))
	w := headless.New(headless.Config{
		Size:        gateSize,
		Scale:       gateDPR,
		VsyncPeriod: time.Hour, // the gate supplies its own vsyncs; see the note above
		Clock:       clk,
		EventBuffer: 1,
		History:     headless.DefaultHistory,
	})
	s := raster.NewScheduler(l, wp, frame.Viewport{Size: gateSize}, gateDPR, raster.Pref{})
	s.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{l}, Background: gateBackground})

	h := &harness{tb: tb, spec: spec, dl: dl, l: l, pool: wp, s: s, c: c, w: w, clk: clk, dir: 1}
	// A window's frame buffers belong to its startup, not to a scroll frame, so the
	// ring is filled here: Present allocates a slot the first time it sees each
	// position in it, and a cold burst that started at the top of a document would
	// otherwise be charged for the backend's own setup.
	for i := 0; i < headless.DefaultHistory; i++ {
		if err := w.Present(c.Backing, nil); err != nil {
			h.tb.Fatalf("prewarm present: %v", err)
		}
	}
	tb.Cleanup(h.close)
	return h
}

// close stops the workers and the window. Called from Cleanup, and by the cold
// benchmark between bursts, where an accumulating set of worker goroutines would be
// measuring a machine with more cores than the one it runs on.
func (h *harness) close() {
	_ = h.pool.Close()
	_ = h.w.Close()
}

// sceneSpec is the gate's document: the checkerboard and glyph stress the criterion
// names, sized to the constants above.
func sceneSpec(prealloc int32) paint.SceneSpec {
	return paint.SceneSpec{
		DocHeight:     gateDocH,
		TextRuns:      gateTextRuns,
		Seed:          gateSeed,
		Checkerboard:  true,
		PreallocTiles: prealloc,
	}
}

// frame runs one vsync the way the loop's draw does, present included. It returns the
// frame's work counters and the coordinates it named, which stay valid until the next
// BeginFrame - the scheduler appends into a retained buffer rather than a fresh one.
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

// vsync is a pacing tick carrying a vertical scroll delta, in device pixels.
func (h *harness) vsync(dy int32) surface.Event {
	return surface.Event{Kind: surface.EvVsync, Delta: frame.Point{Y: dy}, At: h.clk.Now()}
}

// scroll draws one scroll frame of dy device pixels.
func (h *harness) scroll(dy int32) (surface.FrameWork, []frame.TileCoord) {
	return h.frame(h.vsync(dy))
}

// travel scrolls dy device pixels per frame and reverses at the ends of the document,
// so a 600-frame sweep walks the page rather than spending 590 frames pinned against a
// clamp. A frame that cannot move damages nothing, and a gate that measured those would
// be measuring an idle vsync and calling it a scroll.
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

// pump resolves a frame and submits it without compositing or presenting, for waiting
// on asynchronous work without charging the wait to the measured path.
func (h *harness) pump() surface.FrameWork {
	_, needed := h.s.BeginFrame(h.vsync(0))
	return h.s.Submit(needed)
}

// untilQuiet draws until the frame path asks for nothing and no buffer is on order at a
// worker. It is the only way to be sure a warm cache is warm rather than merely idle at
// this instant.
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

// scrollTo lands exactly on a document offset and draws the frame that gets there. A
// setup move, not a measured one: it fails rather than clamping, because a warm-up that
// silently stopped short leaves a cache that is not warm.
func (h *harness) scrollTo(y int32) {
	h.tb.Helper()
	h.mustScroll(y - h.s.Viewport().Offset.Y)
	if got := h.s.Viewport().Offset.Y; got != y {
		h.tb.Fatalf("scrollTo(%d) landed at %d", y, got)
	}
}

// mustScroll draws one scroll frame of dy device pixels and fails if the frame path
// refused to move. A scroll that was clamped damages nothing, so measuring it would be
// measuring an idle vsync and calling it a scroll; the long sweeps above reverse at the
// ends of the document rather than leaning on a clamp.
func (h *harness) mustScroll(dy int32) (surface.FrameWork, []frame.TileCoord) {
	h.tb.Helper()
	at := h.s.Viewport().Offset.Y
	work, needed := h.scroll(dy)
	if dy != 0 && h.s.Viewport().Offset.Y == at {
		h.tb.Fatalf("a scroll of %d device pixels did not move the viewport", dy)
	}
	return work, needed
}

// warmUp rasterizes every tile in the document by stepping the viewport down it one
// tile row at a time and waiting for the pool at each stop, then returns to the top.
// Warming the visible set at one offset would leave the claim "a warm scroll rasterizes
// nothing" true only about the tiles the setup happened to visit.
func (h *harness) warmUp() {
	h.tb.Helper()
	for y := int32(0); y <= gateMaxOffset; y += frame.TileSize {
		h.scrollTo(y)
		h.untilQuiet()
	}
	h.scrollTo(0)
	h.untilQuiet()
}

// newWarmScene builds the gate's scene with the real rasterizer and every tile in the
// document already rasterized, which is what the warm criterion means by a warm cache: 504
// tiles of real checkerboard and real glyphs resident, not a page that was never asked
// for. The setup is slow (it rasterizes the document) and allocating, so it is never part
// of a measured window.
func newWarmScene(tb testing.TB) *harness {
	tb.Helper()
	h := newHarness(tb, sceneSpec(0), nil)
	h.warmUp()
	return h
}

// stats is the frame path's counters, read as one snapshot.
func (h *harness) stats() raster.Stats { return h.s.Stats() }

// allocTotals measures f over runs calls and reports both what the criterion names -
// allocs/op, averaged with the integer division testing.AllocsPerRun uses - and the
// totals behind it. One uncounted warm-up call runs first, as AllocsPerRun's does.
//
// The totals are not decoration. AllocsPerRun divides integers, so 599 allocations
// across a 600-frame sweep report 0 allocs/op, and a criterion written as
// "allocs/op == 0" would then pass on a frame path that allocated once a frame. Here
// the total is what is asserted and the average is what is printed.
//
// This is AllocsPerRun's measurement re-done rather than wrapped, and wrapping is the
// reason it has to be: AllocsPerRun pins GOMAXPROCS to 1 and defers restoring the old
// count, so the restore runs after it returns. A delta taken around the call therefore
// counts procresize parking and unparking the Ps - a runtime housekeeping allocation,
// of the same size whatever the frame path does, which reads exactly like a leak. The
// pin here is taken before the first sample and released after the last one.
//
// The measurement is process-wide: Mallocs counts every goroutine's allocations, so a
// frame path measured alongside workers that rasterize for real cannot be measured at
// all. That is why the cold burst gates on a raster func that draws a tile without
// touching a font - the worker's own allocations are Task 7's claim, and the UI
// thread's are this one's.
//
// For the same reason the collector is off during the window. A GC cycle that a previous
// test's 126 MB of tiles started allocates mark workers' and span bookkeeping on
// goroutines this counter cannot exclude, and it lands in the window as a stray 32-byte
// allocation roughly once in a run - noise that would be indistinguishable from a leak to
// whoever read the failure, and a flake on the frame path's own rule. Disabling it
// changes what is counted, not what is: every mallocgc call the frame path makes is still
// in the total, and one that a real leak would cause is caught by this same window on
// every runner.
func allocTotals(runs int, f func()) (avg float64, mallocs, bytes uint64) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	runtime.GC() // finish any cycle already in flight before the first sample
	f()          // warm-up, deliberately outside the counted window
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

// assertZeroAllocs runs f once per frame for frames, and fails unless the frame path
// allocated nothing at all.
func assertZeroAllocs(t *testing.T, what string, frames int, f func()) {
	t.Helper()
	avg, mallocs, bytes := allocTotals(frames, f)
	if mallocs != 0 {
		t.Errorf("%s: %d allocations (%d bytes) across %d frames, which averaged %g allocs/op: %g bytes per allocation, so this is per-frame work rather than a one-off",
			what, mallocs, bytes, frames, avg, float64(bytes)/float64(mallocs))
		return
	}
	if avg != 0 {
		t.Errorf("%s: allocs/op = %g with a zero total, which cannot both be true", what, avg)
	}
}

// assertBurstAllocatesNothing is the cold criterion, and a cold burst is the one
// measurement here that cannot be taken at face value: Mallocs counts the whole process,
// and the burst is precisely the case where other goroutines are awake. The runtime's own
// bookkeeping for waking them - a ticket for a goroutine that was preempted, a sync.Pool
// pinned for the first time on a processor - can land inside the window and read exactly
// like a frame-path allocation. It is not one, and it is not repeatable either.
//
// So the burst is measured on freshly built frame paths, twice, and only the second one
// reproducing the first's allocations fails. That tells the two apart without tolerating
// anything the frame path itself does: whatever it allocates, an identical path allocates
// again, once a frame or on the frame that first reaches a coordinate. Composer's damage
// list is the example - it grew mid-burst on the first wide frame, and both windows here
// reported it, which is how the bug got fixed in the composer rather than in this file.
//
// What a clean second window lets go is a single unrepeatable allocation by another
// goroutine, and it is reported, not hidden. The warm criterion above needs none of this:
// a warm sweep submits no jobs, so no worker runs and the process-wide counter is the
// frame path's own.
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
