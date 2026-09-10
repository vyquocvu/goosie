package renderer_test

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	imgpkg "github.com/vyquocvu/goosie/internal/image"
	"github.com/vyquocvu/goosie/internal/memory"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestRendererWithImages(t *testing.T) {
	// Create a temporary directory for test images
	tmpDir := t.TempDir()
	testImagePath := filepath.Join(tmpDir, "test.png")

	// Create a simple test image
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	// Save the test image
	f, err := os.Create(testImagePath)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatalf("Failed to encode test image: %v", err)
	}
	f.Close()

	// Create a renderer
	r := renderer.NewRenderer(800, 600)

	// Test HTML with image using file path
	html := `<html><body>
		<h1>Test Image</h1>
		<img src="` + testImagePath + `" alt="Test Image">
		<p>This is a test paragraph.</p>
	</body></html>`

	// Render the HTML
	obj, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}
	if obj == nil {
		t.Fatal("RenderHTML returned nil object")
	}

	// Give async image loading time to complete
	time.Sleep(200 * time.Millisecond)

	// Check that the image was cached
	if r.ImageLoader().GetCache().Len() == 0 {
		t.Error("Expected image to be cached")
	}

	// Check that the cached image has the correct dimensions
	cached := r.ImageLoader().GetCache().Get(testImagePath)
	if cached == nil {
		t.Fatal("Image not found in cache")
	}
	if cached.Width != 50 {
		t.Errorf("Expected width 50, got %d", cached.Width)
	}
	if cached.Height != 50 {
		t.Errorf("Expected height 50, got %d", cached.Height)
	}

	// Check if the image data is attached to the render node
	r.TreeMu().RLock()
	imgNode := findNodeByTag(r.CurrentRenderTree(), "img")
	r.TreeMu().RUnlock()
	if imgNode == nil {
		t.Fatal("img node not found in render tree")
	}

	r.TreeMu().RLock()
	hasImageData := imgNode.ImageData != nil
	r.TreeMu().RUnlock()
	if !hasImageData {
		t.Error("Expected ImageData to be attached to the render node")
	}
}

func TestRendererWithMissingImage(t *testing.T) {
	r := renderer.NewRenderer(800, 600)

	// Test HTML with non-existent image
	html := `<html><body>
		<h1>Missing Image Test</h1>
		<img src="/nonexistent/image.png" alt="Missing Image">
		<p>This image doesn't exist.</p>
	</body></html>`

	// Render the HTML - should not crash
	obj, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}
	if obj == nil {
		t.Fatal("RenderHTML returned nil object")
	}

	// Give async loading time to fail
	time.Sleep(200 * time.Millisecond)

	// Image should be in cache with error state
	cached := r.ImageLoader().GetCache().Get("/nonexistent/image.png")
	if cached != nil {
		// It should have an error state if it's cached
		if cached.State != 2 { // StateError = 2
			t.Errorf("Expected error state, got state %v", cached.State)
		}
	}
}

func TestRendererWithImageNoSrc(t *testing.T) {
	r := renderer.NewRenderer(800, 600)

	// Test HTML with image without src attribute
	html := `<html><body>
		<h1>Image Without Source</h1>
		<img alt="No Source">
		<p>This image has no source.</p>
	</body></html>`

	// Render the HTML - should not crash
	obj, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}
	if obj == nil {
		t.Fatal("RenderHTML returned nil object")
	}

	// No image should be loaded
	if r.ImageLoader().GetCache().Len() > 0 {
		t.Error("Expected no images in cache")
	}
}

func TestImageCacheEviction(t *testing.T) {
	// Create a renderer with small cache
	r := renderer.NewRenderer(800, 600)
	r.ImageLoader().GetCache().SetCapacity(2)

	tmpDir := t.TempDir()

	// Create 3 test images
	for i := 1; i <= 3; i++ {
		imgPath := filepath.Join(tmpDir, strings.Join([]string{"test", string(rune('0' + i)), ".png"}, ""))
		img := image.NewRGBA(image.Rect(0, 0, 10, 10))
		f, _ := os.Create(imgPath)
		png.Encode(f, img)
		f.Close()

		// Load the image
		html := `<html><body><img src="` + imgPath + `"></body></html>`
		r.RenderHTML(context.Background(), html)
	}

	// Wait for async loading
	time.Sleep(300 * time.Millisecond)

	// Cache should only have 2 items (capacity limit)
	if r.ImageLoader().GetCache().Len() > 2 {
		t.Errorf("Expected cache length <= 2, got %d", r.ImageLoader().GetCache().Len())
	}
}

func TestRendererMemoryManagerIntegration(t *testing.T) {
	memMgr := memory.NewManager(memory.Config{
		GlobalLimit: 100 * 1024,
		Limits: map[memory.Component]uint64{
			memory.ComponentImage: 50 * 1024, // 50 KB limit
		},
	})

	r := renderer.NewRenderer(800, 600)
	r.SetMemoryManager(memMgr)

	if r.MemoryManager() != memMgr {
		t.Error("Expected MemoryManager to return registered manager")
	}

	cache := r.ImageLoader().GetCache()
	// Insert 100x100 RGBA image (40 KB)
	cache.Put("img1", &imgpkg.ImageData{Width: 100, Height: 100, Format: "png", State: imgpkg.StateLoaded})

	// Check that memMgr reported usage matches cache bytes (40,000 bytes)
	if memMgr.Usage(memory.ComponentImage) != 40000 {
		t.Errorf("Expected ComponentImage usage 40000, got %d", memMgr.Usage(memory.ComponentImage))
	}

	// Insert second image (40 KB) -> total 80 KB exceeds 50 KB limit
	// memMgr will trigger evictor (calling cache.Evict) to evict down within budget
	cache.Put("img2", &imgpkg.ImageData{Width: 100, Height: 100, Format: "png", State: imgpkg.StateLoaded})

	if memMgr.Usage(memory.ComponentImage) > 50*1024 {
		t.Errorf("Expected ComponentImage usage <= 50KB after eviction, got %d", memMgr.Usage(memory.ComponentImage))
	}
	if cache.Bytes() > 50*1024 {
		t.Errorf("Expected cache bytes <= 50KB after eviction, got %d", cache.Bytes())
	}
	if cache.Get("img1") != nil {
		t.Error("Expected img1 to be evicted by memory manager")
	}
	if cache.Get("img2") == nil {
		t.Error("Expected img2 to be retained")
	}
}

func TestRendererNavigationClearsImageCache(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	cache := r.ImageLoader().GetCache()

	cache.Put("img1", &imgpkg.ImageData{Width: 100, Height: 100, Format: "png", State: imgpkg.StateLoaded})
	if cache.Len() != 1 {
		t.Fatalf("Expected 1 cached image, got %d", cache.Len())
	}

	// Set initial URL
	r.SetCurrentURL("https://example.com/page1")
	if cache.Len() != 1 {
		t.Error("Initial SetCurrentURL should not clear cache")
	}

	// Navigate to new URL -> should clear image cache
	r.SetCurrentURL("https://example.com/page2")
	if cache.Len() != 0 {
		t.Errorf("Expected cache cleared on URL change, got %d items", cache.Len())
	}

	// Test explicit ClearImageCache
	cache.Put("img2", &imgpkg.ImageData{Width: 50, Height: 50, Format: "png", State: imgpkg.StateLoaded})
	r.ClearImageCache()
	if cache.Len() != 0 {
		t.Errorf("Expected cache cleared on ClearImageCache, got %d items", cache.Len())
	}
}

