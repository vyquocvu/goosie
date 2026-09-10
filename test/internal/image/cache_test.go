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

func TestCacheByteBoundsEviction(t *testing.T) {
	// 100 KB limit, capacity 10
	cache := img.NewCacheWithByteLimit(100*1024, 10)
	if cache.MaxBytes() != 100*1024 {
		t.Errorf("Expected maxBytes 102400, got %d", cache.MaxBytes())
	}

	// 100x100 RGBA image is 40,000 bytes (~39 KB)
	img1 := &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded}
	img2 := &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded}
	img3 := &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded}

	cache.Put("img1", img1) // 40 KB
	cache.Put("img2", img2) // 80 KB
	if cache.Len() != 2 {
		t.Fatalf("Expected 2 items, got %d", cache.Len())
	}
	if cache.Bytes() != 80000 {
		t.Errorf("Expected 80000 bytes, got %d", cache.Bytes())
	}

	// Adding img3 would bring total to 120 KB (> 100 KB budget) -> evicts img1
	cache.Put("img3", img3)

	if cache.Len() != 2 {
		t.Fatalf("Expected 2 items after byte-bound eviction, got %d", cache.Len())
	}
	if cache.Bytes() > 100*1024 {
		t.Errorf("Cache bytes %d exceeds maxBytes %d", cache.Bytes(), cache.MaxBytes())
	}
	if cache.Get("img1") != nil {
		t.Error("Expected img1 to be evicted due to byte limit")
	}
	if cache.Get("img2") == nil || cache.Get("img3") == nil {
		t.Error("Expected img2 and img3 to be retained")
	}
}

func TestCacheEvictMethod(t *testing.T) {
	cache := img.NewCacheWithByteLimit(200*1024, 10)

	// 3 images of 40 KB each = 120 KB total
	cache.Put("img1", &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded})
	cache.Put("img2", &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded})
	cache.Put("img3", &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded})

	if cache.Bytes() != 120000 {
		t.Fatalf("Expected 120000 bytes, got %d", cache.Bytes())
	}

	// Evict 50 KB -> should evict img1 (40 KB) and img2 (40 KB) = 80 KB freed
	freed := cache.Evict(50 * 1024)
	if freed < 50*1024 {
		t.Errorf("Expected freed >= 50KB, got %d", freed)
	}
	if cache.Bytes() > 70*1024 {
		t.Errorf("Expected remaining bytes <= 70KB, got %d", cache.Bytes())
	}
	if cache.Get("img1") != nil {
		t.Error("Expected img1 to be evicted")
	}
	if cache.Get("img3") == nil {
		t.Error("Expected newest img3 to be retained")
	}
}

func TestCacheUsageCallback(t *testing.T) {
	cache := img.NewCacheWithByteLimit(100*1024, 10)

	var lastReported uint64
	callbackCount := 0
	cache.SetUsageCallback(func(bytes uint64) {
		lastReported = bytes
		callbackCount++
	})

	// Initial call on SetUsageCallback (0 bytes)
	if callbackCount != 1 || lastReported != 0 {
		t.Fatalf("Expected initial callback with 0 bytes, got count=%d, bytes=%d", callbackCount, lastReported)
	}

	// Put an image of 40 KB
	cache.Put("img1", &img.ImageData{Width: 100, Height: 100, Format: "png", State: img.StateLoaded})
	if lastReported != 40000 {
		t.Errorf("Expected reported bytes 40000, got %d", lastReported)
	}

	// Clear cache
	cache.Clear()
	if lastReported != 0 {
		t.Errorf("Expected reported bytes 0 after clear, got %d", lastReported)
	}
}

func TestCacheOversizedItemNotCached(t *testing.T) {
	// Cache max 50 KB
	cache := img.NewCacheWithByteLimit(50*1024, 10)

	// Image of 200x200 = 160 KB (> 50 KB)
	oversized := &img.ImageData{Width: 200, Height: 200, Format: "png", State: img.StateLoaded}
	cache.Put("large", oversized)

	if cache.Len() != 0 {
		t.Errorf("Expected oversized item not to be cached, got len %d", cache.Len())
	}
	if cache.Get("large") != nil {
		t.Error("Expected nil for oversized item")
	}
}

