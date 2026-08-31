package dom_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom/atom"
	"github.com/vyquocvu/goosie/internal/engine/testpages"
	"github.com/vyquocvu/goosie/internal/dom"
)

// BenchmarkStoreAllocate measures node allocation performance.
func BenchmarkStoreAllocate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := dom.NewStore(1024)
		for j := 0; j < 100; j++ {
			id, _ := s.Allocate()
			s.SetKind(id, dom.NodeKindElement)
			s.SetName(id, atom.AtomDiv)
		}
	}
}

// BenchmarkStoreAppendChild measures appending children.
func BenchmarkStoreAppendChild(b *testing.B) {
	s := dom.NewStore(1024)
	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		div, _ := s.Allocate()
		s.SetKind(div, dom.NodeKindElement)
		s.SetName(div, atom.AtomDiv)
		s.AppendChild(doc, div)
	}
}

// BenchmarkStoreSetAttrs measures setting attributes.
func BenchmarkStoreSetAttrs(b *testing.B) {
	s := dom.NewStore(1024)
	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	s.SetName(div, atom.AtomDiv)

	attrs := []dom.Attr{
		{Name: atom.AttrId, Value: atom.Intern("test")},
		{Name: atom.AttrClass, Value: atom.Intern("container")},
		{Name: atom.AttrHref, Value: atom.Intern("https://example.com")},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.SetAttrs(div, attrs)
	}
}

// BenchmarkStoreSetText measures setting text content.
func BenchmarkStoreSetText(b *testing.B) {
	s := dom.NewStore(1024)
	text, _ := s.Allocate()
	s.SetKind(text, dom.NodeKindText)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.SetText(text, "Hello, world!")
	}
}

// BenchmarkStoreChildIterator measures child iteration.
func BenchmarkStoreChildIterator(b *testing.B) {
	s := dom.NewStore(1024)
	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	for i := 0; i < 100; i++ {
		div, _ := s.Allocate()
		s.SetKind(div, dom.NodeKindElement)
		s.SetName(div, atom.AtomDiv)
		s.AppendChild(doc, div)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		for it := s.Children(doc); it.Next(); {
			_ = it.ID()
			count++
		}
		if count != 100 {
			b.Fatalf("count = %d, want 100", count)
		}
	}
}

// BenchmarkStoreSubtreeIterator measures subtree traversal.
func BenchmarkStoreSubtreeIterator(b *testing.B) {
	s := dom.NewStore(1024)
	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	// Build a tree: doc -> 10 divs -> 10 ps each = 101 nodes.
	for i := 0; i < 10; i++ {
		div, _ := s.Allocate()
		s.SetKind(div, dom.NodeKindElement)
		s.SetName(div, atom.AtomDiv)
		s.AppendChild(doc, div)
		for j := 0; j < 10; j++ {
			p, _ := s.Allocate()
			s.SetKind(p, dom.NodeKindElement)
			s.SetName(p, atom.AtomP)
			s.AppendChild(div, p)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		for it := s.Subtree(doc); it.Next(); {
			_ = it.ID()
			count++
		}
		if count != 111 {
			b.Fatalf("count = %d, want 111", count)
		}
	}
}

// BenchmarkStoreRemoveChild measures removing children.
func BenchmarkStoreRemoveChild(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		s := dom.NewStore(64)
		doc, _ := s.Allocate()
		s.SetKind(doc, dom.NodeKindDocument)
		div, _ := s.Allocate()
		s.SetKind(div, dom.NodeKindElement)
		s.SetName(div, atom.AtomDiv)
		s.AppendChild(doc, div)
		b.StartTimer()

		s.RemoveChild(doc, div)
	}
}

// BenchmarkStoreRemoveSubtree measures removing subtrees.
func BenchmarkStoreRemoveSubtree(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		s := dom.NewStore(1024)
		doc, _ := s.Allocate()
		s.SetKind(doc, dom.NodeKindDocument)

		div, _ := s.Allocate()
		s.SetKind(div, dom.NodeKindElement)
		s.SetName(div, atom.AtomDiv)
		s.AppendChild(doc, div)

		for j := 0; j < 100; j++ {
			p, _ := s.Allocate()
			s.SetKind(p, dom.NodeKindElement)
			s.SetName(p, atom.AtomP)
			s.AppendChild(div, p)
		}
		b.StartTimer()

		s.Remove(div)
	}
}

// BenchmarkStoreCorpusParse measures building a compact store from corpus pages.
func BenchmarkStoreCorpusParse(b *testing.B) {
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
				store := dom.NewStore(512)
				// Simulate parsing by allocating nodes.
				// In M2.4, this will be replaced with actual streaming parse.
				nodeCount := estimateNodeCount(page.HTML)
				for j := 0; j < nodeCount; j++ {
					id, _ := store.Allocate()
					store.SetKind(id, dom.NodeKindElement)
					store.SetName(id, atom.AtomDiv)
				}
			}
		})
	}
}

// estimateNodeCount provides a rough estimate of node count from HTML size.
func estimateNodeCount(html string) int {
	// Rough heuristic: 1 node per 30 bytes of HTML.
	return len(html) / 30
}

// BenchmarkStoreMemoryOverhead measures the memory overhead per node.
func BenchmarkStoreMemoryOverhead(b *testing.B) {
	sizes := []int{10, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(strings.Repeat("0", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				store := dom.NewStore(size)
				for j := 0; j < size; j++ {
					id, _ := store.Allocate()
					store.SetKind(id, dom.NodeKindElement)
					store.SetName(id, atom.AtomDiv)
					store.SetAttrs(id, []dom.Attr{
						{Name: atom.AttrId, Value: atom.Intern("test")},
						{Name: atom.AttrClass, Value: atom.Intern("container")},
					})
				}
			}
		})
	}
}

// BenchmarkStoreVsPointer compares compact store allocation vs pointer-heavy approach.
func BenchmarkStoreVsPointer(b *testing.B) {
	b.Run("CompactStore", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			store := dom.NewStore(1000)
			for j := 0; j < 1000; j++ {
				id, _ := store.Allocate()
				store.SetKind(id, dom.NodeKindElement)
				store.SetName(id, atom.AtomDiv)
				store.SetAttrs(id, []dom.Attr{
					{Name: atom.AttrId, Value: atom.Intern("test")},
				})
			}
		}
	})
}

// BenchmarkStoreTraversalDepth measures traversal performance at different depths.
func BenchmarkStoreTraversalDepth(b *testing.B) {
	depths := []int{1, 10, 100}

	for _, depth := range depths {
		b.Run(strings.Repeat("D", depth), func(b *testing.B) {
			store := dom.NewStore(depth + 1)
			root, _ := store.Allocate()
			store.SetKind(root, dom.NodeKindElement)

			prev := root
			for i := 0; i < depth; i++ {
				child, _ := store.Allocate()
				store.SetKind(child, dom.NodeKindElement)
				store.AppendChild(prev, child)
				prev = child
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				count := 0
				for it := store.Subtree(root); it.Next(); {
					_ = it.ID()
					count++
				}
				if count != depth+1 {
					b.Fatalf("count = %d, want %d", count, depth+1)
				}
			}
		})
	}
}

// BenchmarkStoreTraversalWidth measures traversal performance at different widths.
func BenchmarkStoreTraversalWidth(b *testing.B) {
	widths := []int{10, 100, 1000}

	for _, width := range widths {
		b.Run(strings.Repeat("W", width), func(b *testing.B) {
			store := dom.NewStore(width + 1)
			root, _ := store.Allocate()
			store.SetKind(root, dom.NodeKindElement)

			for i := 0; i < width; i++ {
				child, _ := store.Allocate()
				store.SetKind(child, dom.NodeKindElement)
				store.AppendChild(root, child)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				count := 0
				for it := store.Children(root); it.Next(); {
					_ = it.ID()
					count++
				}
				if count != width {
					b.Fatalf("count = %d, want %d", count, width)
				}
			}
		})
	}
}
