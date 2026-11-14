package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/raster"
)

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

const (
	loadAllocsFloor   = 250_000
	loadAllocsCeiling = 400_000
)

func TestDocumentLoadAllocationFence(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	html := loadFixture(t)

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
