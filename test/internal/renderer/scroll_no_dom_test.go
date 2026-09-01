package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"testing"
)

// TestM54_ScrollDoesNotTraverseDOM verifies that the scroll path
// (UpdateViewport → RenderWithViewport) uses the cached display list
// and does not rebuild it from the DOM tree.
func TestM54_ScrollDoesNotTraverseDOM(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)

	_, err := r.RenderHTML(context.Background(), `<html><body><p>Line 1</p><p>Line 2</p><p>Line 3</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	// Capture the cached display list reference.
	r.CanvasRenderer().CanvasRendererMu().RLock()
	origDL := r.CanvasRenderer().CachedDisplayList()
	r.CanvasRenderer().CanvasRendererMu().RUnlock()

	if origDL == nil {
		t.Fatal("display list should be cached after initial render")
	}

	// Simulate scrolling by calling UpdateViewport multiple times.
	for i := 0; i < 10; i++ {
		r.SetViewport(float32(i*100), 600)
		r.UpdateViewport()
	}

	// Verify the display list was reused (same pointer).
	r.CanvasRenderer().CanvasRendererMu().RLock()
	currentDL := r.CanvasRenderer().CachedDisplayList()
	r.CanvasRenderer().CanvasRendererMu().RUnlock()

	if currentDL != origDL {
		t.Error("scroll should reuse cached display list, not rebuild from DOM")
	}
}

// TestM54_ScrollDoesNotRecomputeLayout verifies that scrolling does not
// trigger layout recomputation.
func TestM54_ScrollDoesNotRecomputeLayout(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)

	_, err := r.RenderHTML(context.Background(), `<html><body><div><p>Content</p></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	// Capture the layout tree reference.
	origLayout := r.CurrentLayoutTree()

	// Scroll.
	for i := 0; i < 10; i++ {
		r.SetViewport(float32(i*50), 600)
		r.UpdateViewport()
	}

	// Layout tree should be unchanged.
	if r.CurrentLayoutTree() != origLayout {
		t.Error("scroll should not recompute layout tree")
	}
}

// TestM54_RenderWithViewportUsesDisplayCommands verifies that
// RenderWithViewport consumes display commands from the cached list
// rather than walking the render tree.
func TestM54_RenderWithViewportUsesDisplayCommands(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)

	_, err := r.RenderHTML(context.Background(), `<html><body><h1>Title</h1><p>Paragraph</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify display list has commands.
	r.CanvasRenderer().CanvasRendererMu().RLock()
	dl := r.CanvasRenderer().CachedDisplayList()
	cmdCount := len(dl.Commands)
	r.CanvasRenderer().CanvasRendererMu().RUnlock()

	if cmdCount == 0 {
		t.Fatal("display list should have commands after render")
	}

	// Call RenderWithViewport directly with the same trees.
	r.TreeMu().RLock()
	renderTree := r.CurrentRenderTree()
	layoutTree := r.CurrentLayoutTree()
	r.TreeMu().RUnlock()

	result := r.CanvasRenderer().RenderWithViewport(renderTree, layoutTree)
	if result == nil {
		t.Error("RenderWithViewport should return non-nil result")
	}

	// Display list should still be the same (reused).
	r.CanvasRenderer().CanvasRendererMu().RLock()
	dl2 := r.CanvasRenderer().CachedDisplayList()
	r.CanvasRenderer().CanvasRendererMu().RUnlock()

	if dl != dl2 {
		t.Error("RenderWithViewport should reuse cached display list when trees unchanged")
	}
}
