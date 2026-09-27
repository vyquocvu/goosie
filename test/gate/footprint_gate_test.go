package gate

import (
	"runtime"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform/headless"
)

// The footprint criteria. M1's other gates say the frame path allocates nothing per
// frame; these say what the path holds when it is not allocating, which is the number a
// "lightweight browser" claim is actually about.
//
// They are bounds against capacity rather than absolute megabytes on purpose. Retained
// memory here is the tile cache plus the two whole-surface bitmap rings, and all three
// are configured: gateBudgetTiles is the cache ceiling and each ring slot is one device
// surface. A flat "under 50MB" assertion would be false at this geometry (1440x900 at
// DPR 2 with a document-wide cache is ~210MB, and correctly so) and true at a phone-sized
// one whether or not the path leaked, so it would gate the wrong thing either way. What
// these do gate is memory the configuration did not ask for - a frame path that starts
// retaining a fourth surface bitmap, or a sweep that keeps every tile buffer it touches
// alive past the budget - and that is the regression class an absolute number only
// reports after it has already shipped.
//
// The absolute figures are recorded in docs/superpowers/plans/ along with the process
// resident-set numbers, so a change in either is visible to a reader without being
// inferred from a passing test.

const miB = 1 << 20

// gateFootprintSlack is what retained heap may exceed the configured caches by. The
// measured residual is ~5.5MB, and it is the glyph atlas the rasterizer is handed
// (4MB) plus the scene and the layer records; 12MB leaves room for that to move and no
// room for a surface bitmap (19.78MB) to appear.
const gateFootprintSlack = 12.0

// gateGrowthSlack is what retained heap may grow by across the warm sweep. Measured
// growth is 0.00MB over gateWarmFrames. 4MB still catches a path retaining ~7KB per
// frame, which is the size of a small per-frame record, and TestFootprintInstrumentDetectsRetention
// below proves it catches that rather than passing because it cannot see.
const gateGrowthSlack = 4.0

// heapMiB returns live heap after a full collection: what the process retains, not what
// the allocator has asked the OS for. Sys is deliberately not used - it counts arena
// reservation, which follows GC timing rather than anything the frame path does.
func heapMiB() float64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / miB
}

// configuredCapacityMiB is the memory the harness above asked for by construction: the
// tile grid at its budget, and one device surface per composer backing and per window
// present slot.
func configuredCapacityMiB(g *frame.Layer) float64 {
	surface := float64(gateDevW*gateDevH*4) / miB
	return float64(g.Grid.Stats().Budget)/miB +
		float64(gateComposerBitmaps+headless.DefaultHistory)*surface
}

func TestFootprintAtRestIsBoundedByConfiguredCaches(t *testing.T) {
	h := newWarmScene(t)
	rest := heapMiB()
	capacity := configuredCapacityMiB(h.l)
	gridMB := float64(h.l.Grid.Stats().Bytes) / miB
	t.Logf("retained=%.2f MiB, configured=%.2f MiB (tile grid %.2f of it)", rest, capacity, gridMB)

	// The lower bound keeps this from passing on a path that holds nothing: an empty
	// pipeline is under every ceiling above and measures nothing.
	if h.l.Grid.Stats().Valid != gateCols*gateRows {
		t.Fatalf("the warm scene holds %d of %d tiles, so the upper bound below is not being tested",
			h.l.Grid.Stats().Valid, gateCols*gateRows)
	}
	if rest > capacity+gateFootprintSlack {
		t.Errorf("retained %.2f MiB exceeds configured caches %.2f MiB plus %.2f MiB slack: the path holds memory the configuration did not ask for",
			rest, capacity, gateFootprintSlack)
	}
}

func TestFootprintDoesNotGrowAcrossWarmSweep(t *testing.T) {
	h := newWarmScene(t)
	before := heapMiB()
	for i := 0; i < gateWarmFrames; i++ {
		h.travel(gateWarmStep)
	}
	h.untilQuiet()
	after := heapMiB()
	t.Logf("retained %.2f -> %.2f MiB across %d scroll frames", before, after, gateWarmFrames)
	if after-before > gateGrowthSlack {
		t.Errorf("%d warm scroll frames grew retained heap by %.2f MiB (budget %.2f): roughly %.1f KiB per frame is being held after the frame that made it",
			gateWarmFrames, after-before, gateGrowthSlack,
			(after-before)*miB/float64(gateWarmFrames)/1024)
	}
}

// TestFootprintInstrumentDetectsRetention is the same guard
// TestGate_AllocationInstrumentIsNotVacuous gives the allocation criterion: the growth
// measurement is shown to see memory held across frames. Without it, a GC that happened
// to run differently, or a harness whose sweep stopped drawing, would report zero growth
// forever.
func TestFootprintInstrumentDetectsRetention(t *testing.T) {
	h := newWarmScene(t)
	before := heapMiB()
	// Hold one device surface's worth per frame for 32 frames - more than the budget
	// allows, far less than a real leak would need to be noticed.
	var held [][]byte
	for i := 0; i < 32; i++ {
		h.travel(gateWarmStep)
		held = append(held, make([]byte, 1<<20))
	}
	after := heapMiB()
	if len(held) == 0 {
		t.Fatal("nothing was retained, so this proves nothing")
	}
	t.Logf("deliberate retention: %.2f -> %.2f MiB", before, after)
	if after-before <= gateGrowthSlack {
		t.Errorf("retaining 32 MiB across the sweep moved the measurement by %.2f MiB, under the %.2f MiB budget: the growth criterion cannot see what it claims to",
			after-before, gateGrowthSlack)
	}
	// Keep the slices alive past the measurement so the compiler cannot drop them early.
	runtime.KeepAlive(held)
}
