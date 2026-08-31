package image_test

import (
	"testing"

	img "github.com/vyquocvu/goosie/internal/image"
)

func TestNewCache(t *testing.T) {
	cache := img.NewCache(10)
	if cache == nil {
		t.Fatal("NewCache returned nil")
	}
	if cache.Len() != 0 {
		t.Errorf("Expected empty cache, got length %d", cache.Len())
	}
}

func TestCachePutAndGet(t *testing.T) {
	cache := img.NewCache(3)

	// Create test image data
	img1 := &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded}
	img2 := &img.ImageData{Width: 200, Height: 200, Format: "jpeg", State: img.StateLoaded}
	img3 := &img.ImageData{Width: 300, Height: 300, Format: "gif", State: img.StateLoaded}

	// Test Put and Get
	cache.Put("img1", img1)
	cache.Put("img2", img2)
	cache.Put("img3", img3)

	if cache.Len() != 3 {
		t.Errorf("Expected cache length 3, got %d", cache.Len())
	}

	// Get existing items
	result := cache.Get("img1")
	if result == nil {
		t.Error("Expected to find img1, got nil")
	} else if result.Width != 100 {
		t.Errorf("Expected width 100, got %d", result.Width)
	}

	result = cache.Get("img2")
	if result == nil {
		t.Error("Expected to find img2, got nil")
	} else if result.Width != 200 {
		t.Errorf("Expected width 200, got %d", result.Width)
	}

	// Get non-existent item
	result = cache.Get("img4")
	if result != nil {
		t.Error("Expected nil for non-existent key, got value")
	}
}

func TestCacheLRUEviction(t *testing.T) {
	cache := img.NewCache(2)

	img1 := &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded}
	img2 := &img.ImageData{Width: 200, Height: 200, Format: "jpeg", State: img.StateLoaded}
	img3 := &img.ImageData{Width: 300, Height: 300, Format: "gif", State: img.StateLoaded}

	// Add two items (fills capacity)
	cache.Put("img1", img1)
	cache.Put("img2", img2)

	if cache.Len() != 2 {
		t.Errorf("Expected cache length 2, got %d", cache.Len())
	}

	// Add third item - should evict img1 (least recently used)
	cache.Put("img3", img3)

	if cache.Len() != 2 {
		t.Errorf("Expected cache length 2 after eviction, got %d", cache.Len())
	}

	// img1 should be evicted
	if cache.Get("img1") != nil {
		t.Error("Expected img1 to be evicted")
	}

	// img2 and img3 should still be present
	if cache.Get("img2") == nil {
		t.Error("Expected img2 to still be in cache")
	}
	if cache.Get("img3") == nil {
		t.Error("Expected img3 to still be in cache")
	}
}

func TestCacheLRUOrdering(t *testing.T) {
	cache := img.NewCache(2)

	img1 := &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded}
	img2 := &img.ImageData{Width: 200, Height: 200, Format: "jpeg", State: img.StateLoaded}
	img3 := &img.ImageData{Width: 300, Height: 300, Format: "gif", State: img.StateLoaded}

	// Add two items
	cache.Put("img1", img1)
	cache.Put("img2", img2)

	// Access img1 to make it more recently used
	cache.Get("img1")

	// Add img3 - should evict img2 (now least recently used)
	cache.Put("img3", img3)

	// img2 should be evicted
	if cache.Get("img2") != nil {
		t.Error("Expected img2 to be evicted")
	}

	// img1 and img3 should still be present
	if cache.Get("img1") == nil {
		t.Error("Expected img1 to still be in cache")
	}
	if cache.Get("img3") == nil {
		t.Error("Expected img3 to still be in cache")
	}
}

func TestCacheClear(t *testing.T) {
	cache := img.NewCache(3)

	img1 := &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded}
	img2 := &img.ImageData{Width: 200, Height: 200, Format: "jpeg", State: img.StateLoaded}

	cache.Put("img1", img1)
	cache.Put("img2", img2)

	if cache.Len() != 2 {
		t.Errorf("Expected cache length 2, got %d", cache.Len())
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("Expected empty cache after Clear, got length %d", cache.Len())
	}

	if cache.Get("img1") != nil || cache.Get("img2") != nil {
		t.Error("Expected all items to be removed after Clear")
	}
}

func TestCacheSetCapacity(t *testing.T) {
	cache := img.NewCache(5)

	// Add 5 items
	for i := 0; i < 5; i++ {
		id := &img.ImageData{Width: i * 100, Height: i * 100, Format: "png", State: img.StateLoaded}
		cache.Put(string(rune('a'+i)), id)
	}

	if cache.Len() != 5 {
		t.Errorf("Expected cache length 5, got %d", cache.Len())
	}

	// Reduce capacity to 3 - should evict 2 items
	cache.SetCapacity(3)

	if cache.Len() != 3 {
		t.Errorf("Expected cache length 3 after reducing capacity, got %d", cache.Len())
	}
}

func TestCacheUpdate(t *testing.T) {
	cache := img.NewCache(2)

	img1 := &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded}
	img1Updated := &img.ImageData{Width: 150, Height: 150, Format: "png", State: img.StateLoaded}

	cache.Put("img1", img1)

	// Update the same key
	cache.Put("img1", img1Updated)

	if cache.Len() != 1 {
		t.Errorf("Expected cache length 1 after update, got %d", cache.Len())
	}

	result := cache.Get("img1")
	if result == nil {
		t.Fatal("Expected to find img1")
	}
	if result.Width != 150 {
		t.Errorf("Expected updated width 150, got %d", result.Width)
	}
}
