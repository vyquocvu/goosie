package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"testing"
	"unsafe"
)

func TestIntrinsicSizeCachePutGet(t *testing.T) {
	cache := renderer.NewIntrinsicSizeCache(1024)

	cache.Put(renderer.LayoutID(1), 100, 50)
	w, h, ok := cache.Get(renderer.LayoutID(1))
	if !ok || w != 100 || h != 50 {
		t.Errorf("Expected 100x50, ok=true; got %fx%f, ok=%v", w, h, ok)
	}
}

func TestIntrinsicSizeCacheEviction(t *testing.T) {
	// Calculate size of 2 entries.
	entrySize := int64(unsafe.Sizeof(renderer.IntrinsicSizeEntry{}))
	maxBytes := entrySize * 2
	cache := renderer.NewIntrinsicSizeCache(maxBytes)

	cache.Put(renderer.LayoutID(1), 10, 10)
	cache.Put(renderer.LayoutID(2), 20, 20)
	cache.Put(renderer.LayoutID(3), 30, 30) // Should evict LayoutID 1

	if _, _, ok := cache.Get(renderer.LayoutID(1)); ok {
		t.Error("Expected LayoutID(1) to be evicted")
	}

	if _, _, ok := cache.Get(renderer.LayoutID(2)); !ok {
		t.Error("Expected LayoutID(2) to still be in cache")
	}

	if _, _, ok := cache.Get(renderer.LayoutID(3)); !ok {
		t.Error("Expected LayoutID(3) to be in cache")
	}
}

func TestIntrinsicSizeCacheUpdate(t *testing.T) {
	cache := renderer.NewIntrinsicSizeCache(1024)

	cache.Put(renderer.LayoutID(1), 10, 10)
	cache.Put(renderer.LayoutID(1), 20, 20) // Update

	w, h, ok := cache.Get(renderer.LayoutID(1))
	if !ok || w != 20 || h != 20 {
		t.Errorf("Expected updated 20x20, got %fx%f", w, h)
	}
}

func TestIntrinsicSizeCache_Invalidate(t *testing.T) {
	cache := renderer.NewIntrinsicSizeCache(1024)

	cache.Put(renderer.LayoutID(1), 10, 10)
	cache.Invalidate(renderer.LayoutID(1))

	if _, _, ok := cache.Get(renderer.LayoutID(1)); ok {
		t.Error("Expected LayoutID(1) to be invalidated")
	}
}

func TestIntrinsicSizeCacheClear(t *testing.T) {
	cache := renderer.NewIntrinsicSizeCache(1024)

	cache.Put(renderer.LayoutID(1), 10, 10)
	cache.Put(renderer.LayoutID(2), 20, 20)
	cache.Clear()

	if _, _, ok := cache.Get(renderer.LayoutID(1)); ok {
		t.Error("Expected cache to be empty")
	}
}

func TestIntrinsicSizeCacheEvict(t *testing.T) {
	cache := renderer.NewIntrinsicSizeCache(1024)
	cache.Put(renderer.LayoutID(1), 10, 10)
	cache.Put(renderer.LayoutID(2), 20, 20)

	entrySize := int64(unsafe.Sizeof(renderer.IntrinsicSizeEntry{}))

	freed := cache.Evict(uint64(entrySize))
	if freed != uint64(entrySize) {
		t.Errorf("Expected freed %d, got %d", entrySize, freed)
	}

	if _, _, ok := cache.Get(renderer.LayoutID(1)); ok {
		t.Error("Expected LayoutID(1) to be evicted")
	}
	if _, _, ok := cache.Get(renderer.LayoutID(2)); !ok {
		t.Error("Expected LayoutID(2) to still be in cache")
	}
}
