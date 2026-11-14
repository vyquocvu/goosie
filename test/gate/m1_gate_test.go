package gate

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

func gateRaster(j raster.Job) error {
	if j.Out == nil {
		return errors.New("gate: job carried no buffer")
	}
	j.Out.Reset()
	j.Out.FillRect(j.Out.Bounds(), gateTileColor(j.Coord), nil)
	return nil
}

func gateTileColor(c frame.TileCoord) frame.Color {
	return frame.RGB(byte(c.Row*7+11), byte(c.Col*17+5), byte(c.Row*3+c.Col+3))
}

// qc:BR-174
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
	if got := after.Submitted - base.Submitted; got != 0 {
		t.Errorf("%d raster jobs submitted during the warm sweep, want 0", got)
	}
	if got := after.Frames - base.Frames; got < int64(gateWarmFrames) {
		t.Errorf("the sweep drew %d frames, want at least %d", got, gateWarmFrames)
	}
	if got := h.s.ScratchAllocs(); got != 0 {
		t.Errorf("a scratch buffer grew %d times across the whole run; a warm frame must not resize a list", got)
	}
	if got := h.l.ContentVersion; got != paint.SceneVersion {
		t.Errorf("content version after %d scroll frames = %d, want the scene's %d: a scroll must never bump it",
			gateWarmFrames, got, paint.SceneVersion)
	}
}

func TestGate_ColdScrollBurstNoRedundantJobs(t *testing.T) {
	t.Run("allocs/op==0_on_the_ui_path", func(t *testing.T) {
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
		if got := after.Rasterized - base.Rasterized; got < 100 {
			t.Fatalf("the burst rasterized %d tiles; it was not a burst into un-rasterized content", got)
		}
	})
}

func distinct(coords []frame.TileCoord) map[frame.TileCoord]bool {
	out := make(map[frame.TileCoord]bool, len(coords))
	for _, c := range coords {
		out[c] = true
	}
	return out
}

var bannedSubstrings = []string{"metal", "opengl", "vulkan", "glfw", "fyne", "webgpu"}

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

func gateTestBinary(t *testing.T, root string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "gate.testbin")
	cmd := exec.Command("go", "test", "-c", "-o", bin, "./test/gate")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test -c: %v\n%s", err, out)
	}
	return bin
}

func appBinary(t *testing.T, root string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "goosie")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/goosie")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/goosie: %v\n%s", err, out)
	}
	return bin
}

func scanBinaryForGPU(t *testing.T, bin string, floor int) {
	t.Helper()
	syms, err := archtest.Symbols(bin)
	if err != nil {
		t.Fatalf("go tool nm %s: %v", bin, err)
	}
	if len(syms) < floor {
		t.Fatalf("%s has %d symbols; the listing is not real", bin, len(syms))
	}
	for _, v := range archtest.CheckBanned("", syms, bannedSubstrings...) {
		t.Errorf("%s links %s: %s", filepath.Base(bin), v.Import, v.Why)
	}
}

func TestGate_NoGPULinkage(t *testing.T) {
	root := moduleRoot(t)

	deps, err := archtest.Deps(root, "./...")
	if err != nil {
		t.Fatalf("go list -deps ./...: %v", err)
	}
	if len(deps) < 20 {
		t.Fatalf("the dependency closure of ./... is %d paths; that is not a real list", len(deps))
	}
	for _, v := range archtest.CheckBannedDeps(deps, bannedSubstrings...) {
		t.Errorf("v2 depends on %s: %s", v.Import, v.Why)
	}

	scanBinaryForGPU(t, gateTestBinary(t, root), 1000)
	scanBinaryForGPU(t, appBinary(t, root), 1000)
}

func TestGate_ArchtestBoundaries(t *testing.T) {
	root := moduleRoot(t)
	pkgs, err := archtest.List(root, "./...")
	if err != nil {
		t.Fatalf("go list ./...: %v", err)
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
	found, err := archtest.CheckCgo(root, filepath.Join(root, "internal", "platform", "darwin"))
	if err != nil {
		t.Fatalf("cgo walk: %v", err)
	}
	for _, path := range found {
		t.Errorf("%s uses cgo; only internal/platform/darwin may", path)
	}
	unformatted, err := archtest.UnformattedGoFiles(root)
	if err != nil {
		t.Fatalf("gofmt walk: %v", err)
	}
	for _, path := range unformatted {
		t.Errorf("%s: not gofmt-formatted", path)
	}
	globals, err := archtest.MutableGlobals(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatalf("global-state walk: %v", err)
	}
	for _, g := range globals {
		t.Errorf("%s: the engine may not mutate a package-level var after init()", g)
	}
}

var allocSink []byte

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

func reportMsPerOp(b *testing.B) {
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1e6, "ms/op")
}

func BenchmarkWarmScroll(b *testing.B) {
	b.ReportAllocs()
	defer reportMsPerOp(b)
	b.StopTimer()
	h := newWarmScene(b)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		h.travel(gateWarmStep)
	}
}

func BenchmarkColdScrollBurst(b *testing.B) {
	b.ReportAllocs()
	defer reportMsPerOp(b)
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
