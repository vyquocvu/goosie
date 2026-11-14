package gate

import (
	"runtime"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/raster"
)

const miB = 1 << 20

const gateFootprintSlack = 12.0

const gateGrowthSlack = 4.0

func heapMiB() float64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / miB
}

const gateFallbackFontBudgetMiB = float64(raster.MaxParsedFallbackFonts*raster.MaxFallbackFontBytes) / miB

func configuredCapacityMiB(g *frame.Layer) float64 {
	surface := float64(gateDevW*gateDevH*4) / miB
	return float64(g.Grid.Stats().Budget)/miB +
		float64(gateComposerBitmaps+headless.DefaultHistory)*surface +
		gateFallbackFontBudgetMiB
}

func TestFootprintAtRestIsBoundedByConfiguredCaches(t *testing.T) {
	h := newWarmScene(t)
	rest := heapMiB()
	capacity := configuredCapacityMiB(h.l)
	gridMB := float64(h.l.Grid.Stats().Bytes) / miB
	t.Logf("retained=%.2f MiB, configured=%.2f MiB (tile grid %.2f of it)", rest, capacity, gridMB)

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

func TestFootprintInstrumentDetectsRetention(t *testing.T) {
	h := newWarmScene(t)
	before := heapMiB()
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
	runtime.KeepAlive(held)
}

func TestFootprintAtRestInstrumentDetectsRetention(t *testing.T) {
	h := newWarmScene(t)
	capacity := configuredCapacityMiB(h.l)
	ceiling := capacity + gateFootprintSlack

	withinBytes := int(gateFallbackFontBudgetMiB * miB)
	heldWithin := make([]byte, withinBytes)
	for i := range heldWithin {
		heldWithin[i] = byte(i)
	}
	restWithin := heapMiB()
	if restWithin > ceiling {
		t.Errorf("within-budget retention (%.2f MiB) gave rest=%.2f MiB, want <= ceiling %.2f MiB",
			gateFallbackFontBudgetMiB, restWithin, ceiling)
	}
	runtime.KeepAlive(heldWithin)

	excessBytes := 39 * miB
	heldExcess := make([]byte, excessBytes)
	for i := range heldExcess {
		heldExcess[i] = byte(i)
	}
	restExcess := heapMiB()
	t.Logf("at-rest instrument: baseline configured=%.2f MiB, ceiling=%.2f MiB, within(%.2f MiB)=%.2f MiB, excess(39 MiB)=%.2f MiB",
		capacity, ceiling, gateFallbackFontBudgetMiB, restWithin, restExcess)
	if restExcess <= ceiling {
		t.Errorf("deliberate retention of 39 MiB font gave rest=%.2f MiB <= ceiling %.2f MiB: at-rest criterion cannot see font retention",
			restExcess, ceiling)
	}
	runtime.KeepAlive(heldExcess)
}
