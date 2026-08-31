package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"reflect"
	"testing"
)

func TestTableColumnCacheBasic(t *testing.T) {
	cache := renderer.NewTableColumnCache(2)

	// Test Miss
	if _, ok := cache.Get(1, 400.0); ok {
		t.Error("expected cache miss for non-existent table ID")
	}

	// Test Set and Get
	widths1 := []float32{100.0, 200.0}
	cache.Set(1, 400.0, widths1)

	gotWidths, ok := cache.Get(1, 400.0)
	if !ok {
		t.Fatal("expected cache hit for table ID 1")
	}
	if !reflect.DeepEqual(gotWidths, widths1) {
		t.Errorf("got widths %v, want %v", gotWidths, widths1)
	}

	// Test Get with different width (miss)
	if _, ok := cache.Get(1, 500.0); ok {
		t.Error("expected cache miss for different available width")
	}
}

func TestTableColumnCacheEviction(t *testing.T) {
	cache := renderer.NewTableColumnCache(2) // capacity 2

	cache.Set(1, 400.0, []float32{100.0, 100.0})
	cache.Set(2, 400.0, []float32{150.0, 150.0})

	// Table ID 1 and 2 are in cache
	if _, ok := cache.Get(1, 400.0); !ok {
		t.Error("expected table 1 to be in cache")
	}

	// Add 3rd table, should evict 1 (FIFO)
	cache.Set(3, 400.0, []float32{200.0, 200.0})

	if _, ok := cache.Get(1, 400.0); ok {
		t.Error("expected table 1 to be evicted")
	}
	if _, ok := cache.Get(2, 400.0); !ok {
		t.Error("expected table 2 to remain in cache")
	}
	if _, ok := cache.Get(3, 400.0); !ok {
		t.Error("expected table 3 to be in cache")
	}
}

func TestTableColumnCacheInvalidation(t *testing.T) {
	cache := renderer.NewTableColumnCache(5)

	cache.Set(1, 400.0, []float32{100.0})
	cache.Set(2, 400.0, []float32{200.0})

	cache.Invalidate(1)

	if _, ok := cache.Get(1, 400.0); ok {
		t.Error("expected table 1 to be invalid/deleted")
	}
	if _, ok := cache.Get(2, 400.0); !ok {
		t.Error("expected table 2 to remain in cache")
	}
}

func TestTableColumnCacheClear(t *testing.T) {
	cache := renderer.NewTableColumnCache(5)

	cache.Set(1, 400.0, []float32{100.0})
	cache.Set(2, 400.0, []float32{200.0})

	cache.Clear()

	if _, ok := cache.Get(1, 400.0); ok {
		t.Error("expected cache to be empty after Clear")
	}
	if _, ok := cache.Get(2, 400.0); ok {
		t.Error("expected cache to be empty after Clear")
	}
}

func TestTableColumnCacheInvalidationIntegration(t *testing.T) {
	renderer.GlobalTableColumnCache.Clear()

	// Create table tree
	table := renderer.NewRenderNode(renderer.NodeTypeElement)
	table.TagName = "table"

	row := renderer.NewRenderNode(renderer.NodeTypeElement)
	row.TagName = "tr"
	table.AddChild(row)

	cell := renderer.NewRenderNode(renderer.NodeTypeElement)
	cell.TagName = "td"
	row.AddChild(cell)

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "Hello"
	cell.AddChild(text)

	// Perform layout
	le := renderer.NewLayoutEngine(800, 600)
	layoutRoot := le.ComputeLayout(table)
	if layoutRoot == nil {
		t.Fatal("layout failed")
	}

	// Verify that table ID is in cache
	if _, ok := renderer.GlobalTableColumnCache.Get(table.ID, 800.0); !ok {
		t.Error("expected table column widths to be cached")
	}

	// Create incremental layout engine
	ile := renderer.NewIncrementalLayoutEngine(800, 600)

	// Invalidate the child text node
	ile.InvalidateNode(text, renderer.DirtyLayout)

	// Verify that table ID is now invalidated in cache
	if _, ok := renderer.GlobalTableColumnCache.Get(table.ID, 800.0); ok {
		t.Error("expected table column widths to be invalidated after child node invalidation")
	}
}
