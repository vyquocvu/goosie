package renderer

import (
	"context"
	"testing"
)

// TestM54_RefreshSkipsStyleAndLayoutWhenClean verifies that calling Refresh()
// on a renderer with no changes does NOT recompute style or layout.
func TestM54_RefreshSkipsStyleAndLayoutWhenClean(t *testing.T) {
	r := NewRenderer(800, 600)
	r.testingMode = true

	_, err := r.RenderHTML(context.Background(), `<html><body><p>Hello</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	// After initial render, renderer should be clean.
	if r.IsDirty() {
		t.Fatal("renderer should be clean after initial render")
	}

	// Capture the current layout tree pointer.
	origLayout := r.currentLayoutTree

	// Refresh when clean should NOT recompute layout.
	r.Refresh()

	if r.currentLayoutTree != origLayout {
		t.Error("Refresh() on clean renderer should not recompute layout tree")
	}
}

// TestM54_RefreshRecomputesWhenDirty verifies that calling Refresh() after
// MarkDirty() DOES recompute style and layout.
func TestM54_RefreshRecomputesWhenDirty(t *testing.T) {
	r := NewRenderer(800, 600)
	r.testingMode = true

	_, err := r.RenderHTML(context.Background(), `<html><body><p>Hello</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	// Mark dirty.
	r.MarkDirty()
	if !r.IsDirty() {
		t.Fatal("renderer should be dirty after MarkDirty()")
	}

	// Refresh when dirty SHOULD recompute.
	r.Refresh()

	// After refresh, should be clean again.
	if r.IsDirty() {
		t.Error("renderer should be clean after Refresh()")
	}
}

// TestM54_SetSizeMarksDirty verifies that SetSize() marks the renderer dirty.
func TestM54_SetSizeMarksDirty(t *testing.T) {
	r := NewRenderer(800, 600)
	r.testingMode = true

	_, err := r.RenderHTML(context.Background(), `<html><body><p>Hello</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	if r.IsDirty() {
		t.Fatal("renderer should be clean after render")
	}

	r.SetSize(1024, 768)

	if !r.IsDirty() {
		t.Error("renderer should be dirty after SetSize()")
	}
}

// TestM54_MarkDirtyAndRefresh verifies the dirty-clean cycle.
func TestM54_MarkDirtyAndRefresh(t *testing.T) {
	r := NewRenderer(800, 600)
	r.testingMode = true

	_, err := r.RenderHTML(context.Background(), `<html><body><div><p>Test</p></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	// Cycle 1: clean -> dirty -> refresh -> clean.
	if r.IsDirty() {
		t.Fatal("should start clean")
	}
	r.MarkDirty()
	if !r.IsDirty() {
		t.Fatal("should be dirty after MarkDirty")
	}
	r.Refresh()
	if r.IsDirty() {
		t.Fatal("should be clean after Refresh")
	}

	// Cycle 2: multiple MarkDirty calls coalesce.
	r.MarkDirty()
	r.MarkDirty()
	r.MarkDirty()
	if !r.IsDirty() {
		t.Fatal("should be dirty")
	}
	r.Refresh()
	if r.IsDirty() {
		t.Fatal("should be clean after Refresh")
	}
}

// TestM54_InitialRenderSetsClean verifies that the initial render does not
// leave the renderer in a dirty state.
func TestM54_InitialRenderSetsClean(t *testing.T) {
	r := NewRenderer(800, 600)
	r.testingMode = true

	if r.IsDirty() {
		t.Fatal("new renderer should not be dirty")
	}

	_, err := r.RenderHTML(context.Background(), `<html><body><h1>Title</h1></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	if r.IsDirty() {
		t.Fatal("renderer should be clean after initial render")
	}
}

// TestM54_RefreshWhenCleanStillTriggersCallback verifies that Refresh() when
// clean still fires the onRefresh callback (for UI update) but skips
// expensive recomputation.
func TestM54_RefreshWhenCleanStillTriggersCallback(t *testing.T) {
	r := NewRenderer(800, 600)
	r.testingMode = true

	_, err := r.RenderHTML(context.Background(), `<html><body><p>Hello</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	callbackCalled := false
	r.SetRefreshCallback(func() {
		callbackCalled = true
	})

	// Refresh when clean should still trigger the callback.
	r.Refresh()

	if !callbackCalled {
		t.Error("Refresh() should trigger onRefresh callback even when clean")
	}
}
