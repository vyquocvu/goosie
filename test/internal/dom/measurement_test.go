package dom_test

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine/testpages"
	"golang.org/x/net/html"
)

// ---------------------------------------------------------------------------
// M2.1 — Measure the current DOM representation
//
// These tests and benchmarks establish a locked baseline for the pointer-heavy
// *html.Node DOM tree produced by golang.org/x/net/html before the compact
// DOM store migration begins in M2.2+.
// ---------------------------------------------------------------------------

// TestMeasureNodeStructSizes reports unsafe.Sizeof for the core html.Node and
// html.Attribute types.  This is a test (not a benchmark) so the values appear
// in `go test -v` output and are recorded in the PR.
func TestMeasureNodeStructSizes(t *testing.T) {
	var node html.Node
	var attr html.Attribute

	nodeSize := unsafe.Sizeof(node)
	attrSize := unsafe.Sizeof(attr)

	t.Logf("sizeof(html.Node)      = %d bytes", nodeSize)
	t.Logf("sizeof(html.Attribute) = %d bytes", attrSize)
	t.Logf("Node pointer fields: Parent, FirstChild, LastChild, PrevSibling, NextSibling (5 × 8 = 40 bytes on 64-bit)")
	t.Logf("Node interface field:  DataAtom (atom.Atom = uint32 padded)")

	// Sanity: html.Node must not shrink unexpectedly.
	if nodeSize == 0 {
		t.Fatal("sizeof(html.Node) is 0 — measurement error")
	}
}

// countNodes returns the total number of html.Node values reachable from root.
func countNodes(root *html.Node) int {
	if root == nil {
		return 0
	}
	n := 1
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		n += countNodes(c)
	}
	return n
}

// countAttributes returns the total number of html.Attribute values across
// all element nodes reachable from root.
func countAttributes(root *html.Node) int {
	if root == nil {
		return 0
	}
	n := len(root.Attr)
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		n += countAttributes(c)
	}
	return n
}

// TestMeasureCorpusNodeCounts parses each corpus page and logs the node count,
// attribute count, and estimated heap footprint per page.
func TestMeasureCorpusNodeCounts(t *testing.T) {
	p := dom.NewParser()
	summaries := testpages.List()

	for _, s := range summaries {
		page, ok := testpages.Get(s.Name)
		if !ok {
			t.Fatalf("corpus page %q not found", s.Name)
		}
		doc, err := p.ParseDocument(strings.NewReader(page.HTML))
		if err != nil {
			t.Fatalf("parse %s: %v", s.Name, err)
		}
		nodes := countNodes(doc)
		attrs := countAttributes(doc)
		nodeSize := unsafe.Sizeof(html.Node{})
		attrSize := unsafe.Sizeof(html.Attribute{})
		estimatedBytes := uintptr(nodes)*nodeSize + uintptr(attrs)*attrSize

		t.Logf("%-28s  HTML=%6d B  nodes=%5d  attrs=%5d  est heap≈%d B (%.1f KB)",
			s.Name, page.HTMLBytes, nodes, attrs, estimatedBytes, float64(estimatedBytes)/1024)
	}
}

// BenchmarkMeasureParseCorpus records parse time and allocations for every
// corpus page.  Run with -benchmem to capture B/op and allocs/op.
func BenchmarkMeasureParseCorpus(b *testing.B) {
	p := dom.NewParser()
	summaries := testpages.List()

	for _, s := range summaries {
		page, ok := testpages.Get(s.Name)
		if !ok {
			b.Fatalf("corpus page %q not found", s.Name)
		}
		b.Run(s.Name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(page.HTMLBytes))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := p.ParseDocument(strings.NewReader(page.HTML))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestMeasureGCPressure reports GC behavior for repeated parse-and-discard
// cycles that simulate tab navigation.  The test performs N parse cycles on
// the largest corpus page and records heap growth and GC pause statistics.
func TestMeasureGCPressure(t *testing.T) {
	// Use the largest corpus page to amplify GC signal.
	page, ok := testpages.Get("scrolling_long")
	if !ok {
		t.Fatal("scrolling_long page not found")
	}

	const iterations = 50
	p := dom.NewParser()

	// Warm up: one parse to populate type metadata.
	if _, err := p.ParseDocument(strings.NewReader(page.HTML)); err != nil {
		t.Fatal(err)
	}

	// Force a clean baseline.
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	// Parse-and-discard loop.
	for i := 0; i < iterations; i++ {
		doc, err := p.ParseDocument(strings.NewReader(page.HTML))
		if err != nil {
			t.Fatal(err)
		}
		// Prevent the compiler from optimizing away the result.
		if doc == nil {
			t.Fatal("nil doc")
		}
	}

	// Force final collection so we can measure retained heap.
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	nodes := countNodes(mustParse(t, p, page.HTML))

	heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	gcCount := after.NumGC - before.NumGC
	var totalPauseNs uint64
	for i := uint32(0); i < gcCount && i < 256; i++ {
		idx := (after.NumGC - i - 1) % 256
		totalPauseNs += after.PauseNs[idx]
	}
	avgPauseNs := uint64(0)
	if gcCount > 0 {
		avgPauseNs = totalPauseNs / uint64(gcCount)
	}

	t.Logf("GC pressure over %d parse cycles of %q (%d nodes per doc):", iterations, page.Name, nodes)
	t.Logf("  HeapAlloc before : %d B (%.1f KB)", before.HeapAlloc, float64(before.HeapAlloc)/1024)
	t.Logf("  HeapAlloc after  : %d B (%.1f KB)", after.HeapAlloc, float64(after.HeapAlloc)/1024)
	t.Logf("  Heap growth      : %d B (%.1f KB)", heapGrowth, float64(heapGrowth)/1024)
	t.Logf("  GC cycles        : %d", gcCount)
	t.Logf("  Avg GC pause     : %d ns (%.1f µs)", avgPauseNs, float64(avgPauseNs)/1000)
	t.Logf("  Total Alloc      : %d B (%.1f MB)", after.TotalAlloc-before.TotalAlloc,
		float64(after.TotalAlloc-before.TotalAlloc)/(1024*1024))

	// After GC, retained heap growth should be modest (< 2× single-doc estimate).
	// This is a soft assertion — log a warning rather than fail, since GC timing
	// is non-deterministic.
	singleDocEstimate := uintptr(nodes) * unsafe.Sizeof(html.Node{})
	if heapGrowth > int64(singleDocEstimate)*4 {
		t.Logf("  WARNING: retained heap growth (%d B) exceeds 4× single-doc node estimate (%d B)",
			heapGrowth, singleDocEstimate)
	}
}

// BenchmarkMeasureAllocsPerNode measures allocations attributable to each
// parsed node by comparing parse output across increasing document sizes.
func BenchmarkMeasureAllocsPerNode(b *testing.B) {
	p := dom.NewParser()

	sizes := []struct {
		label string
		html  string
	}{
		{"10p", strings.Repeat("<p>Short paragraph.</p>", 10)},
		{"50p", strings.Repeat("<p>Short paragraph.</p>", 50)},
		{"100p", strings.Repeat("<p>Short paragraph.</p>", 100)},
		{"500p", strings.Repeat("<p>Short paragraph.</p>", 500)},
	}

	for _, s := range sizes {
		b.Run(s.label, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(s.html)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := p.ParseDocument(strings.NewReader(s.html))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestMeasureNodePointerDensity reports how much of each html.Node is pointer
// data vs. value data, to motivate the compact DOM store design.
func TestMeasureNodePointerDensity(t *testing.T) {
	// html.Node layout on 64-bit:
	//   Parent, FirstChild, LastChild, PrevSibling, NextSibling  — 5 pointers = 40 B
	//   Data (string header)                                      — 16 B (pointer + len)
	//   Attr ([]Attribute header)                                 — 24 B (pointer + len + cap)
	//   Namespace (string header)                                 — 16 B (pointer + len)
	//   Type (NodeType, uint)                                     — 4 B (padded)
	//   DataAtom (atom.Atom = uint32)                             — 4 B
	//
	// Pointer-rich fields: Parent, FirstChild, LastChild, PrevSibling, NextSibling,
	// Data.ptr, Attr.ptr, Namespace.ptr = 8 pointer fields.
	var node html.Node
	total := unsafe.Sizeof(node)
	pointerFields := 5*8 + 8 + 8 + 8 // 5 node ptrs + Data.ptr + Attr.ptr + Namespace.ptr
	valueFields := int(total) - pointerFields

	t.Logf("html.Node total size: %d bytes", total)
	t.Logf("  Pointer-rich fields: ~%d bytes (%d pointers)", pointerFields, pointerFields/8)
	t.Logf("  Value/other fields : ~%d bytes", valueFields)
	t.Logf("  Pointer density    : %.1f%%", float64(pointerFields)/float64(total)*100)
	t.Logf("")
	t.Logf("Per-node heap cost with attributes:")
	t.Logf("  html.Node        : %d B", unsafe.Sizeof(node))
	t.Logf("  html.Attribute   : %d B (key string + val string = 4 string headers = 64 B + data)", unsafe.Sizeof(html.Attribute{}))
	t.Logf("  Avg 2 attrs/node : %d B additional", 2*unsafe.Sizeof(html.Attribute{}))

	t.Logf("")
	t.Logf("Conclusion: each DOM node carries 8 pointer fields that the GC must trace.")
	t.Logf("A compact store with integer IDs (M2.3) would reduce pointer density")
	t.Logf("and enable contiguous slice storage for better GC and cache behavior.")
}

func mustParse(t *testing.T, p *dom.Parser, htmlStr string) *html.Node {
	t.Helper()
	doc, err := p.ParseDocument(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestMeasureCorpusSummary prints a formatted table of corpus baseline metrics.
func TestMeasureCorpusSummary(t *testing.T) {
	p := dom.NewParser()
	summaries := testpages.List()
	nodeSize := unsafe.Sizeof(html.Node{})
	attrSize := unsafe.Sizeof(html.Attribute{})

	t.Log("=== DOM Measurement Baseline (M2.1) ===")
	t.Log("")
	t.Logf("%-28s %8s %6s %6s %8s %10s", "Page", "HTML(B)", "Nodes", "Attrs", "Est(B)", "Est(KB)")
	t.Log(strings.Repeat("-", 72))

	for _, s := range summaries {
		page, ok := testpages.Get(s.Name)
		if !ok {
			t.Fatalf("corpus page %q not found", s.Name)
		}
		doc, err := p.ParseDocument(strings.NewReader(page.HTML))
		if err != nil {
			t.Fatal(err)
		}
		nodes := countNodes(doc)
		attrs := countAttributes(doc)
		est := uintptr(nodes)*nodeSize + uintptr(attrs)*attrSize
		t.Logf("%-28s %8d %6d %6d %8d %10.1f",
			s.Name, page.HTMLBytes, nodes, attrs, est, float64(est)/1024)
	}

	t.Log("")
	t.Log("APIs depending on *html.Node pointers (see api_inventory.go):")
	apis := []string{
		"internal/dom/parser.go: ParseDocument, ParseBodyText, ParseBodyHTML, GetElementByID,",
		"  GetElementByIDFull, GetElementsByClassName, GetElementsByTagName,",
		"  QuerySelector, QuerySelectorAll, getTextFromNode, nodeToElement, matchesSelector",
		"internal/renderer/renderer.go: RenderHTML, findBodyNode, loadExternalCSS,",
		"  extractExternalLinks, extractAndParseCSS, countHTMLNodes",
		"internal/js/runtime.go: populateJSNode, convertGoNodeToJS, ParseFragment usage",
		"cmd/browser/main.go: title extraction, script extraction, walk functions",
	}
	for _, line := range apis {
		t.Log("  " + line)
	}

	// Print allocation-per-run for a representative parse.
	page, _ := testpages.Get("long_article")
	allocs := testing.AllocsPerRun(10, func() {
		_, _ = p.ParseDocument(strings.NewReader(page.HTML))
	})
	t.Logf("")
	t.Logf("AllocsPerRun(long_article) = %.0f", allocs)
	t.Logf("Estimated per-node allocs  = %.2f", allocs/float64(countNodes(mustParse(t, p, page.HTML))))

}
