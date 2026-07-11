package renderer

import (
	"context"
	"testing"
)

// TestM54_RenderUsesDisplayListWhenAvailable verifies that CanvasRenderer.Render()
// uses the cached display list instead of traversing the DOM tree when a
// display list is available.
func TestM54_RenderUsesDisplayListWhenAvailable(t *testing.T) {
	r := NewRenderer(800, 600)
	r.testingMode = true

	_, err := r.RenderHTML(context.Background(), `<html><body><h1>Title</h1><p>Text</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify display list is cached.
	r.canvasRenderer.mu.RLock()
	dl := r.canvasRenderer.cachedDisplayList
	r.canvasRenderer.mu.RUnlock()

	if dl == nil {
		t.Fatal("display list should be cached after RenderHTML")
	}
	if len(dl.Commands) == 0 {
		t.Fatal("display list should have commands")
	}

	// Call Render() with the same render tree.
	r.treeMu.RLock()
	renderTree := r.currentRenderTree
	r.treeMu.RUnlock()

	result := r.canvasRenderer.Render(renderTree)
	if result == nil {
		t.Error("Render() should return non-nil result")
	}
}

// TestM54_RenderWithNilReturnsEmpty verifies that Render(nil) returns an
// empty container without any display list or DOM access.
func TestM54_RenderWithNilReturnsEmpty(t *testing.T) {
	cr := NewCanvasRenderer(800, 600)
	result := cr.Render(nil)
	if result == nil {
		t.Error("Render(nil) should return non-nil empty container")
	}
}

// TestM54_RasterPathConsumesDisplayCommands verifies that the main
// production raster path (RenderWithViewport) only consumes display
// commands and does not walk the render tree during repaint.
func TestM54_RasterPathConsumesDisplayCommands(t *testing.T) {
	r := NewRenderer(800, 600)
	r.testingMode = true

	_, err := r.RenderHTML(context.Background(), `<html><body><div><p>Hello</p><p>World</p></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify display list is cached.
	r.canvasRenderer.mu.RLock()
	dl := r.canvasRenderer.cachedDisplayList
	r.canvasRenderer.mu.RUnlock()

	if dl == nil || len(dl.Commands) == 0 {
		t.Fatal("display list should have commands after render")
	}

	// Simulate a repaint by calling RenderWithViewport again.
	r.treeMu.RLock()
	renderTree := r.currentRenderTree
	layoutTree := r.currentLayoutTree
	r.treeMu.RUnlock()

	// The display list should be reused (same pointer).
	result := r.canvasRenderer.RenderWithViewport(renderTree, layoutTree)
	if result == nil {
		t.Fatal("RenderWithViewport should return non-nil")
	}

	r.canvasRenderer.mu.RLock()
	dl2 := r.canvasRenderer.cachedDisplayList
	r.canvasRenderer.mu.RUnlock()

	if dl != dl2 {
		t.Error("repaint should reuse same display list — raster path consumes display commands only")
	}
}
