package atom

import (
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Static atom benchmarks
// ---------------------------------------------------------------------------

func BenchmarkLookupStaticHit(b *testing.B) {
	tags := []string{"div", "span", "p", "a", "h1", "body", "img", "table", "form", "input"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tag := range tags {
			a, ok := LookupStatic(tag)
			if !ok || a == 0 {
				b.Fatal("unexpected miss for known tag")
			}
		}
	}
}

func BenchmarkLookupStaticMiss(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := LookupStatic("custom-element")
		if ok {
			b.Fatal("unexpected hit")
		}
	}
}

func BenchmarkStaticAtomString(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = AtomDiv.String()
	}
}

// ---------------------------------------------------------------------------
// Dynamic table benchmarks
// ---------------------------------------------------------------------------

func BenchmarkTableInternNew(b *testing.B) {
	// Benchmark interning many unique strings (cold path — allocates).
	tbl := NewTable(b.N+64, b.N*32+65536)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := fmt.Sprintf("class-%d", i)
		a := tbl.Intern(s)
		if a == 0 {
			b.Fatal("Intern returned zero")
		}
	}
}

func BenchmarkTableInternHit(b *testing.B) {
	// Benchmark interning the same string repeatedly (hot path — no alloc).
	tbl := NewTable(64, 65536)
	tbl.Intern("shared-class")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := tbl.Intern("shared-class")
		if a == 0 {
			b.Fatal("Intern returned zero")
		}
	}
}

func BenchmarkTableInternStatic(b *testing.B) {
	// Interning a static string should be as fast as LookupStatic.
	tbl := NewTable(64, 65536)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := tbl.Intern("div")
		if a != AtomDiv {
			b.Fatal("expected static AtomDiv")
		}
	}
}

func BenchmarkTableLookupHit(b *testing.B) {
	tbl := NewTable(64, 65536)
	tbl.Intern("my-class")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := tbl.Lookup("my-class")
		if a == 0 {
			b.Fatal("Lookup returned zero for interned string")
		}
	}
}

func BenchmarkTableLookupMiss(b *testing.B) {
	tbl := NewTable(64, 65536)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := tbl.Lookup("nonexistent")
		if a != 0 {
			b.Fatal("Lookup should return zero for missing key")
		}
	}
}

func BenchmarkTableLookupStatic(b *testing.B) {
	// Looking up a static string via the table should short-circuit.
	tbl := NewTable(64, 65536)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := tbl.Lookup("div")
		if a != AtomDiv {
			b.Fatal("expected static AtomDiv")
		}
	}
}

// ---------------------------------------------------------------------------
// Eviction benchmark
// ---------------------------------------------------------------------------

func BenchmarkTableEviction(b *testing.B) {
	// Simulate a workload with bounded table and steady-state eviction.
	const tableSize = 256
	tbl := NewTable(tableSize, tableSize*32)

	// Pre-warm with some entries.
	for i := 0; i < tableSize; i++ {
		tbl.Intern(fmt.Sprintf("warm-%d", i))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := fmt.Sprintf("dynamic-%d", i)
		tbl.Intern(s)
	}
}

// ---------------------------------------------------------------------------
// Realistic workload benchmark
// ---------------------------------------------------------------------------

func BenchmarkTableRealisticWorkload(b *testing.B) {
	// Simulate interning tag names, attribute names, class names, and IDs
	// from a typical HTML document.
	tbl := NewTable(512, 32768)

	// Common tags (will hit static table).
	tags := []string{"div", "span", "p", "a", "h1", "h2", "ul", "li", "img", "form"}
	// Common attributes (will hit static table).
	attrs := []string{"id", "class", "href", "src", "style", "type"}
	// Dynamic class names.
	classes := make([]string, 50)
	for i := range classes {
		classes[i] = fmt.Sprintf("cls-%d", i)
	}
	// Dynamic IDs.
	ids := make([]string, 20)
	for i := range ids {
		ids[i] = fmt.Sprintf("elem-%d", i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate processing one element.
		for _, tag := range tags {
			tbl.Intern(tag)
		}
		for _, attr := range attrs {
			tbl.Intern(attr)
		}
		for _, cls := range classes {
			tbl.Intern(cls)
		}
		for _, id := range ids {
			tbl.Intern(id)
		}
	}
}

// ---------------------------------------------------------------------------
// Concurrent benchmark
// ---------------------------------------------------------------------------

func BenchmarkTableConcurrent(b *testing.B) {
	tbl := NewTable(1024, 65536)
	// Pre-populate.
	for i := 0; i < 100; i++ {
		tbl.Intern(fmt.Sprintf("key-%d", i))
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				tbl.Intern(fmt.Sprintf("key-%d", i%100))
			} else {
				tbl.Lookup(fmt.Sprintf("key-%d", i%100))
			}
			i++
		}
	})
}

// ---------------------------------------------------------------------------
// Memory comparison: interned vs raw strings
// ---------------------------------------------------------------------------

func BenchmarkStringDeduplication(b *testing.B) {
	// Simulate a document with many repeated class names.
	// Compare raw string allocation vs atom interning.
	classNames := []string{"container", "row", "col", "active", "hidden", "btn", "btn-primary", "text-center"}
	repeat := 1000

	// Build a document-like slice of raw strings.
	b.Run("RawStrings", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			raw := make([]string, 0, len(classNames)*repeat)
			for r := 0; r < repeat; r++ {
				for _, cls := range classNames {
					raw = append(raw, cls)
				}
			}
			_ = raw
		}
	})

	// With atom interning, each unique string is stored once.
	b.Run("AtomIntern", func(b *testing.B) {
		tbl := NewTable(64, 4096)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			atoms := make([]Atom, 0, len(classNames)*repeat)
			for r := 0; r < repeat; r++ {
				for _, cls := range classNames {
					atoms = append(atoms, tbl.Intern(cls))
				}
			}
			_ = atoms
		}
	})
}

// ---------------------------------------------------------------------------
// Corpus-based benchmark
// ---------------------------------------------------------------------------

func BenchmarkTableCorpusClasses(b *testing.B) {
	// Simulate extracting and interning all class names from a typical page.
	tbl := NewTable(512, 32768)

	// Simulated class names from a documentation-style page.
	pageClasses := strings.Fields(`
		container site-title main-nav post post-header post-title
		post-meta author date category post-body tags tag
		search-widget widget-title recent-posts widget
		container footer-nav
	`)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, cls := range pageClasses {
			tbl.Intern(cls)
		}
	}
}
