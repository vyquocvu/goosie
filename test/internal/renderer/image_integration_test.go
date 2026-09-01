package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
