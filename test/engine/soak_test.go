package engine_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/tabs"
)

// soakDocs is a representative rotation: prose, tables, flex, forms, scripts
// and styled text. Small on purpose: the gate counts cycles and resource
// growth, not page weight (heavy documents have their own budget tests).
var soakDocs = []string{
	`<html><body><p>Hello soak world, hello again.</p></body></html>`,
	`<html><body><table><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table></body></html>`,
	`<html><head><style>.r{display:flex}.c{flex:1}</style></head><body><div class="r"><div class="c">x</div><div class="c">y</div></div></body></html>`,
	`<html><body><form><input type="text" value="v"><button>Go</button></form></body></html>`,
	`<html><body><script>var x = 1 + 2;</script><p>scripted</p></body></html>`,
	`<html><head><style>p{color:green}div{margin:4px}</style></head><body><div><p>styled</p></div></body></html>`,
}

// TestLifecycleSoak runs 1000 representative document lifecycles — build,
// paint, find, reflow, teardown — plus tab-strip churn, asserting the Gate 7
// stability contract: no crash or deadlock, goroutines back to baseline (no
// loader/painter leaks), and heap growth within a predeclared budget.
//
// Budgets: goroutine delta 0 after warmup; heap growth under 64 MiB for the
// run (GC'd before measuring; generous because allocator arenas are sticky).
// No sleeps or network: ordering is by construction, so -race findings here
// are real races rather than timing artifacts.
func TestLifecycleSoak(t *testing.T) {
	// Warmup settles one-time pools/registries outside the measurement.
	for i := 0; i < 10; i++ {
		soakOne(t, soakDocs[i%len(soakDocs)], 800)
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	baseGoroutines := runtime.NumGoroutine()

	const cycles = 1000
	for i := 0; i < cycles; i++ {
		soakOne(t, soakDocs[i%len(soakDocs)], float32(800+(i%3)*112))
	}

	mgr := tabs.NewManager(nil)
	for i := 0; i < cycles; i++ {
		tab := mgr.NewTab()
		tab.SetURL(fmt.Sprintf("https://example.com/p%d", i))
		tab.SetTitle(fmt.Sprintf("p%d", i))
		mgr.SwitchTo(tab.ID())
		if i%2 == 0 && mgr.Count() > 5 {
			mgr.CloseTab(mgr.Tabs()[0].ID())
		}
	}
	for _, tab := range mgr.Tabs() {
		mgr.CloseTab(tab.ID())
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if got := runtime.NumGoroutine(); got != baseGoroutines {
		t.Fatalf("goroutines = %d, baseline %d (leaked %d per %d cycles)",
			got, baseGoroutines, got-baseGoroutines, cycles)
	}
	// Unsigned counters wrap when the second run reuses the first run's
	// arenas: saturate instead of comparing raw differences.
	var growth uint64
	if after.HeapAlloc > before.HeapAlloc {
		growth = after.HeapAlloc - before.HeapAlloc
	}
	if growth > 64<<20 {
		t.Fatalf("heap grew %d bytes over %d cycles, budget 64 MiB", growth, cycles)
	}
	t.Logf("soak: %d document cycles + %d tab churns, goroutines steady at %d",
		cycles, cycles, baseGoroutines)
}

func soakOne(t *testing.T, html string, width float32) {
	t.Helper()
	sess, err := engine.NewSession(html, nil, width,
		engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := sess.PaintChecked(1); err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	_ = sess.Find("e")
	if err := sess.Reflow(width + 64); err != nil {
		t.Fatalf("Reflow: %v", err)
	}
	sess.Close()
}
