package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"testing"
	"time"

	imageloader "github.com/vyquocvu/goosie/internal/image"
)

func TestRendererWithDataURI(t *testing.T) {
	r := renderer.NewRenderer(800, 600)

	// Test HTML with data URI image
	// 1x1 red pixel GIF
	dataURI := "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"
	html := `<html><body>
		<h1>Data URI Test</h1>
		<img src="` + dataURI + `" alt="Data URI Image">
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
	cached := r.ImageLoader().GetCache().Get(dataURI)
	if cached == nil {
		t.Fatal("Image not found in cache")
	}
	if cached.Width != 1 {
		t.Errorf("Expected width 1, got %d", cached.Width)
	}
	if cached.Height != 1 {
		t.Errorf("Expected height 1, got %d", cached.Height)
	}
	if cached.State != imageloader.StateLoaded {
		t.Errorf("Expected StateLoaded, got %v", cached.State)
	}
}

func TestRendererWithImageError(t *testing.T) {
	r := renderer.NewRenderer(800, 600)

	// Test HTML with invalid image
	// Invalid base64
	invalidDataURI := "data:image/gif;base64,INVALID"
	html := `<html><body>
		<h1>Invalid Image Test</h1>
		<img src="` + invalidDataURI + `" alt="Invalid Image">
	</body></html>`

	// Render the HTML
	_, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	// Give async image loading time to complete
	time.Sleep(200 * time.Millisecond)

	// Check that the image was cached with error state
	cached := r.ImageLoader().GetCache().Get(invalidDataURI)
	if cached == nil {
		t.Fatal("Image not found in cache")
	}
	if cached.State != imageloader.StateError {
		t.Errorf("Expected StateError, got %v", cached.State)
	}
}
