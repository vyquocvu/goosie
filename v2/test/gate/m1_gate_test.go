package gate

// The M1 exit criteria, as tests. Everything here asserts counters rather than
// durations, because CI runners vary enough in cores and memory bandwidth that a
// timing gate there would flake, and a flaky gate is ignored within a month. The two
// benchmarks in this file record the timings for the macOS report and assert nothing
// about them; the nightly macos-latest job is where the millisecond budget is gated.

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vyquocvu/goosie/v2/internal/archtest"
	"github.com/vyquocvu/goosie/v2/internal/frame"
	"github.com/vyquocvu/goosie/v2/internal/paint"
	"github.com/vyquocvu/goosie/v2/internal/raster"
)

// gateRaster stands in for a tile rasterizer without doing glyph work: it writes real
// pixels - a colour that identifies the tile, so a composition can be checked for the
// tile it is holding - and allocates nothing.
//
// It exists because testing.AllocsPerRun counts the whole process, not one goroutine,
// so a cold burst measured with the real rasterizer would report the workers' glyph
// masks as part of "the UI path allocates nothing". The worker's own allocation cost is
// Task 7's claim; the UI thread's is this file's, and this is the only way to tell the
// two apart without a per-thread allocation counter the runtime does not have.
func gateRaster(j raster.Job) error {
	if j.Out == nil {
		return errors.New("gate: job carried no buffer")
	}
	j.Out.Reset()
	j.Out.FillRect(j.Out.Bounds(), gateTileColor(j.Coord), nil)
	return nil
}

// gateTileColor stamps a coordinate into three bytes. A tile whose colour decodes back
// to its own coordinate arrived through the pool un-swapped.
func gateTileColor(c frame.TileCoord) frame.Color {
	return frame.RGB(byte(c.Row*7+11), byte(c.Col*17+5), byte(c.Row*3+c.Col+3))
}

// Criterion 4: "BenchmarkWarmScroll (1440x900 @ DPR 2, checkerboard + glyph stress,
// tile cache warm): allocs/op == 0 and tiles rasterized per frame after warmup == 0".
//
// The scene is drawn with the real rasterizer, so the cache being warm means 504 tiles
// of real checkerboard and real glyphs are resident, not that nothing was ever asked
// for. That is also why this one does not need the allocating raster func above: during
// a warm scroll no worker runs at all, so every allocation in the window is the frame
// path's.
func TestGate_WarmScrollZeroAllocsZeroTiles(t *testing.T) {
	h := newWarmScene(t)
	if got := h.dl.Len(); got < 1000 {
		t.Fatalf("the gate's display list holds %d commands; the glyph stress is not in it", got)
	}

	base := h.stats()
	if base.Rasterized == 0 {
		t.Fatal("a warmed cache rasterized nothing, so \"zero after warm-up\" would be a claim about an empty page")
	}
	if got := h.l.Stats().Tiles; got != gateCols*gateRows {
		t.Fatalf("warm-up left %d tiles in the grid, want the whole document's %d", got, gateCols*gateRows)
	}

	assertZeroAllocs(t, "warm scroll frame", gateWarmFrames, func() { h.travel(gateWarmStep) })

	after := h.stats()
	if got := after.Rasterized - base.Rasterized; got != 0 {
		t.Errorf("%d tiles rasterized during %d warm scroll frames, want 0", got, gateWarmFrames)
	}
	// The same claim from the other direction: a frame that asked for tiles and got
	// none reused them, and Needed rather than Rasterized is what a hitch is caused by.
	if got := after.Submitted - base.Submitted; got != 0 {
		t.Errorf("%d raster jobs submitted during the warm sweep, want 0", got)
	}
	if got := after.Frames - base.Frames; got < int64(gateWarmFrames) {
		t.Errorf("the sweep drew %d frames, want at least %d", got, gateWarmFrames)
	}
	if got := h.s.ScratchAllocs(); got != 0 {
		t.Errorf("a scratch buffer grew %d times across the whole run; a warm frame must not resize a list", got)
	}
	// Invariant 1's other half: scrolling is a position, not a content change. A frame
	// path that bumped the version to scroll would re-rasterize the page on every flick,
	// and the counter above would then be non-zero for a reason the reader could not see.
	if got := h.l.ContentVersion; got != paint.SceneVersion {
		t.Errorf("content version after %d scroll frames = %d, want the scene's %d: a scroll must never bump it",
			gateWarmFrames, got, paint.SceneVersion)
	}
}

// Criterion 5: "BenchmarkColdScrollBurst (30 frames of 200px into un-rasterized
// content): allocs/op == 0, and raster job count equals stale-tile count (no redundant
// work)". Two claims, so two scenes: the allocation half needs a raster func that keeps
// the workers quiet, and the job-count half is about the scheduler's bookkeeping.
func TestGate_ColdScrollBurstNoRedundantJobs(t *testing.T) {
	t.Run("allocs/op==0_on_the_ui_path", func(t *testing.T) {
		// PreallocTiles hides the pool's first-touch cost, and NewGrid pre-builds the
		// tile records, so this measures the frame path itself: the plan, the coordinate
		// walks, the submissions, the drains, the composition, and the present.
		assertBurstAllocatesNothing(t, "cold scroll frame", gateColdFrames, func() (func(), func()) {
			h := newHarness(t, sceneSpec(gateBudgetTiles), gateRaster)
			return func() { h.travel(gateColdStep) }, h.close
		})
	})

	t.Run("jobs==stale_tiles", func(t *testing.T) {
		h := newHarness(t, sceneSpec(gateBudgetTiles), gateRaster)
		base := h.stats()
		seen := map[frame.TileCoord]int{}
		var sumNeeded, sumSubmitted, sumRefused, dupFrames, dupOffers int

		for i := 0; i < gateColdFrames; i++ {
			work, needed := h.travel(gateColdStep)
			// Within one frame a coordinate appears at most once: the visible walk and
			// the prefetch ring each visit every coordinate they name once, and a tile
			// already on order is filtered rather than queued twice.
			for _, c := range needed {
				seen[c]++
				dupOffers += seen[c] - 1
			}
			if len(needed) != len(distinct(needed)) {
				dupFrames++
				t.Errorf("frame %d named %d coordinates and %d distinct ones: the same tile was asked for twice in one frame",
					i, len(needed), len(distinct(needed)))
			}
			sumNeeded += work.Needed
			sumSubmitted += work.Submitted
			sumRefused += work.Refused
		}
		h.untilQuiet()
		after := h.stats()

		// Each frame's submissions are its needed set, and nothing was turned away: a
		// refused job would be re-asked by a later frame, which is the redundancy this
		// criterion is about, so the equality is only meaningful with zero refusals.
		if sumRefused != 0 {
			t.Errorf("%d submissions refused during the burst; the re-asks that follow are not redundant work, but the count below cannot mean anything until they are", sumRefused)
		}
		if got := after.Deferred - base.Deferred; got != 0 {
			t.Errorf("%d tiles deferred for want of budget; the burst needs a bigger budget to test anything", got)
		}
		if got := after.Submitted - base.Submitted; int(got) != sumSubmitted {
			t.Errorf("the scheduler counted %d submissions and the frames counted %d", got, sumSubmitted)
		}
		if got := after.Rasterized - base.Rasterized; int(got) != len(seen) {
			t.Errorf("%d tiles rasterized for %d distinct coordinates offered; a coordinate drawn twice is redundant work, and once refused is 0 the two counts must be equal",
				got, len(seen))
		}
		if sumSubmitted != len(seen) {
			t.Errorf("%d jobs submitted for %d distinct coordinates: %d of them were queued more than once",
				sumSubmitted, len(seen), sumSubmitted-len(seen))
		}
		if sumNeeded != sumSubmitted {
			t.Errorf("%d tiles were named as needed and %d were submitted; the gap is work the frame wanted and did not get", sumNeeded, sumSubmitted)
		}
		if dupFrames != 0 {
			t.Errorf("%d of %d frames named a coordinate twice", dupFrames, gateColdFrames)
		}
		if dupOffers != 0 {
			t.Errorf("%d coordinates were offered by more than one frame, so a job was still on order when the same tile was asked for again",
				dupOffers)
		}
		// The burst has to actually be cold, or every count above proves nothing.
		if got := after.Rasterized - base.Rasterized; got < 100 {
			t.Fatalf("the burst rasterized %d tiles; it was not a burst into un-rasterized content", got)
		}
	})
}

// distinct is the set of coordinates one frame named. It is a test-side helper and
// allocates freely, which is why the burst that is measured for allocations does not
// use it.
func distinct(coords []frame.TileCoord) map[frame.TileCoord]bool {
	out := make(map[frame.TileCoord]bool, len(coords))
	for _, c := range coords {
		out[c] = true
	}
	return out
}

// bannedSubstrings are the graphics APIs whose presence in the link would mean v2 had
// stopped being a CPU compositor. They are the five the criterion names plus WebGPU, and
// they are matched case-insensitively as substrings, in import paths and in symbol names
// alike: a linkage rule that misses "Metal" because it was written "metal" is not a rule.
// Header names are deliberately absent - `nm` reports symbols, and "gl.h" would never
// match one.
var bannedSubstrings = []string{"metal", "opengl", "vulkan", "glfw", "fyne", "webgpu"}

// moduleRoot is where `go list ./v2/...` means what it says.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this file; cannot find the module root")
	}
	root, err := archtest.RepoRoot(here)
	if err != nil {
		t.Fatalf("%s: %v", here, err)
	}
	return root
}

// gateTestBinary builds the gate's own test binary unstripped and returns its path. It
// is the same link as the one running this test - the scene, the scheduler, the
// composer and a platform backend - built into t.TempDir().
//
// os.Executable() is not usable here: `go test` links the binary it runs with -s -w, and
// a stripped Mach-O keeps only its undefined dynamic imports (84 of them for this
// package, all libSystem), which says nothing about what Go or a cgo shim pulled in. The
// check has to read a symbol table that still has one.
func gateTestBinary(t *testing.T, root string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "gate.testbin")
	cmd := exec.Command("go", "test", "-c", "-o", bin, "./v2/test/gate")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test -c: %v\n%s", err, out)
	}
	return bin
}

// Criterion 6: "v2 is GPU-free: no Metal/GL/Vulkan symbol in the binary". Both halves
// of the claim are checked, because they fail differently. The import closure catches
// a dependency somebody added; the symbol table catches what a cgo shim links, which
// never appears as a Go import at all.
func TestGate_NoGPULinkage(t *testing.T) {
	root := moduleRoot(t)

	deps, err := archtest.Deps(root, "./v2/...")
	if err != nil {
		t.Fatalf("go list -deps ./v2/...: %v", err)
	}
	if len(deps) < 20 {
		t.Fatalf("the dependency closure of ./v2/... is %d paths; that is not a real list", len(deps))
	}
	for _, v := range archtest.CheckBannedDeps(deps, bannedSubstrings...) {
		t.Errorf("v2 depends on %s: %s", v.Import, v.Why)
	}

	bin := gateTestBinary(t, root)
	syms, err := archtest.Symbols(bin)
	if err != nil {
		t.Fatalf("go tool nm %s: %v", bin, err)
	}
	// A whole test binary, its dependencies and its runtime is thousands of symbols.
	// Under this many the listing is not one, and an empty one would pass the loop below
	// by listing nothing rather than by linking nothing.
	if len(syms) < 1000 {
		t.Fatalf("%s has %d symbols; the listing is not real", bin, len(syms))
	}
	for _, v := range archtest.CheckBanned("", syms, bannedSubstrings...) {
		t.Errorf("the frame path links %s: %s", v.Import, v.Why)
	}
}

// Criterion 3: archtest green, kept here so "M1 CI green" is one command. This is the
// same code the boundary suite runs, not a copy of its table: a rule maintained twice
// is a rule that gets updated once.
func TestGate_ArchtestBoundaries(t *testing.T) {
	root := moduleRoot(t)
	pkgs, err := archtest.List(root, "./v2/...")
	if err != nil {
		t.Fatalf("go list ./v2/...: %v", err)
	}
	if len(pkgs) < 8 {
		t.Fatalf("go listed %d v2 packages; the pattern is wrong", len(pkgs))
	}
	for _, v := range archtest.CheckLayers(pkgs) {
		t.Errorf("import graph: %v", v)
	}
	for _, v := range archtest.CheckBannedImports(pkgs, "fyne.io/") {
		t.Errorf("gui toolkit: %s imports %s", v.Importer, v.Import)
	}
	found, err := archtest.CheckCgo(filepath.Join(root, "v2"), filepath.Join(root, "v2", "internal", "platform", "darwin"))
	if err != nil {
		t.Fatalf("cgo walk: %v", err)
	}
	for _, path := range found {
		t.Errorf("%s uses cgo; only v2/internal/platform/darwin may", path)
	}
}

// allocSink keeps an allocation the instrument is meant to count from being optimised
// away. Nothing reads it. The type is a slice rather than an interface because storing
// one into an interface copies its header, which would be a second allocation this test
// has no use for.
var allocSink []byte

// The two zero-allocation criteria are worth as much as this check: an instrument that
// could not see a leak would report none. Both shapes of leak have to be visible - one
// per frame, and one that happens once in the window, which is what a buffer growing for
// the first time looks like and is exactly the case an average rounds away.
func TestGate_AllocationInstrumentIsNotVacuous(t *testing.T) {
	if _, m, _ := allocTotals(30, func() { allocSink = make([]byte, 9) }); m != 30 {
		t.Errorf("30 frames that each allocated once were counted as %d allocations", m)
	}

	once := 0
	if _, m, _ := allocTotals(30, func() {
		once++
		if once == 9 {
			allocSink = make([]byte, 8192)
		}
	}); m != 1 {
		t.Errorf("a window containing exactly one allocation was counted as %d", m)
	}
}

// The benchmarks are the macOS report's numbers. They measure what the design cares
// about - the UI thread's work per frame - and assert nothing about it. Each reports its
// mean in ms/op next to the ns/op Go prints, because that is the unit M1's budgets are
// stated in and the nightly job's report is read by a person.
func reportMsPerOp(b *testing.B) {
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1e6, "ms/op")
}

func BenchmarkWarmScroll(b *testing.B) {
	b.ReportAllocs()
	defer reportMsPerOp(b)
	// The warm-up rasterizes the whole document, which is over a hundred megabytes of
	// tile bitmaps and thousands of allocations - all of it setup, none of it a frame.
	// -benchmem counts everything between a benchmark's start and its end, so without
	// this the report blames the frame path for warming the cache and says it allocated
	// 8 times a frame where the test above proves it allocated nothing.
	b.StopTimer()
	h := newWarmScene(b)
	b.StartTimer()
	// One op is one frame, which is the unit the 16.6ms vsync budget is stated in.
	for i := 0; i < b.N; i++ {
		h.travel(gateWarmStep)
	}
}

func BenchmarkColdScrollBurst(b *testing.B) {
	b.ReportAllocs()
	defer reportMsPerOp(b)
	// One op is one cold scroll frame, taken from a burst of gateColdFrames of them,
	// because "30 frames of 200px into un-rasterized content" is a shape rather than a
	// unit. When a harness runs out of document it is replaced outside the timer: a
	// second pass over the same content is warm, and a cold benchmark that quietly
	// became a warm one would report a third of the time. The harness constructor is
	// outside the timer for the same reason it is outside the warm one's - a window's
	// buffers and a grid's pre-built records are startup, not frames - which is why the
	// cold case reports the workers' glyph allocations and not the scene's.
	b.StopTimer()
	h := newHarness(b, sceneSpec(gateBudgetTiles), nil)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		if i%gateColdFrames == 0 && i > 0 {
			b.StopTimer()
			h.untilQuiet()
			h.close()
			h = newHarness(b, sceneSpec(gateBudgetTiles), nil)
			b.StartTimer()
		}
		h.travel(gateColdStep)
	}
	b.StopTimer()
	h.untilQuiet()
}
