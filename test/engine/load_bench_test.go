package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/raster"
)

// loadFixture is the document the load path is measured against. It is a real
// 186 KB page rather than a synthetic scene: the synthetic generator never touches
// the HTML tokenizer, the selector matcher, or the box tree, which is where a load
// regression would land.
const loadViewportW, loadViewportH = 1440, 900

func loadFixture(tb testing.TB) string {
	tb.Helper()
	p := filepath.Join("..", "..", "testdata", "perf", "gate_scroll.html")
	data, err := os.ReadFile(p)
	if err != nil {
		tb.Fatalf("read the load fixture: %v", err)
	}
	if len(data) < 100_000 {
		tb.Fatalf("load fixture is %d bytes; the bound below is sized for the 186 KB page", len(data))
	}
	return string(data)
}

// loadOnce runs the whole pipeline the browser runs for a page: parse, style, lay
// out, and build the display list the frame path is handed. It returns the document's
// height in device pixels, which is how the tests below check that the fixture was
// really laid out rather than silently parsed into an empty document.
func loadOnce(tb testing.TB, fonts *raster.Fonts, html string) int32 {
	tb.Helper()
	sess, err := engine.NewSession(html, nil, loadViewportW,
		engine.WithMetrics(fonts), engine.WithViewportH(loadViewportH))
	if err != nil {
		tb.Fatalf("build session: %v", err)
	}
	list, err := sess.PaintChecked(2)
	if err != nil {
		tb.Fatalf("paint: %v", err)
	}
	return list.Build(1).Extent().H()
}

func BenchmarkDocumentLoad(b *testing.B) {
	fonts, err := raster.NewFonts()
	if err != nil {
		b.Fatal(err)
	}
	html := loadFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loadOnce(b, fonts, html)
	}
	b.StopTimer()
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1e6, "ms/op")
}

// The fence is a bracket, not a ceiling alone: the low end says the load path is
// still doing the work it is supposed to do, and the high end says it is not doing
// ever more of it. Both ends are set from the same measurement, so a change that
// made loading allocate nothing - by short-circuiting the pipeline, say - would fail
// here rather than being read as a win.
const (
	loadAllocsFloor   = 250_000
	loadAllocsCeiling = 400_000
)

// TestDocumentLoadAllocationFence is the regression fence §G asks for on the load
// path. The benchmark above cannot fail a build, so this asserts the same quantity in
// a plain test, which `go test ./...` runs everywhere the rest of the suite does.
func TestDocumentLoadAllocationFence(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	html := loadFixture(t)

	// The first load pays for caches - the UA rule index among them - that every
	// later load shares, and those are not a per-load regression.
	loadOnce(t, fonts, html)
	h := loadOnce(t, fonts, html)
	if h < 4000 {
		t.Fatalf("the fixture laid out to %d px tall; the fence only means something over a whole page", h)
	}

	n := testing.AllocsPerRun(3, func() { loadOnce(t, fonts, html) })
	t.Logf("allocations per document load: %.0f (document is %d px tall)", n, h)
	if n > loadAllocsCeiling {
		t.Errorf("a document load allocated %.0f objects, over the fence of %d; this is the regression §G asks to be flagged", n, loadAllocsCeiling)
	}
	if n < loadAllocsFloor {
		t.Errorf("a document load allocated %.0f objects, under the floor of %d; the load path stopped doing real work, or this instrument stopped measuring it", n, loadAllocsFloor)
	}
}
