package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

// An unsized root SVG takes its width from the containing block and its
// height from the viewBox ratio. Falling back to the 300x150 replaced-element
// default made Google's 24 px apps icon overflow its 40 px button.
func TestUnsizedSVGUsesViewBoxRatio(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div style="width: 200px"><svg viewbox="0 0 24 12"></svg></div>
</body></html>`, 400)
	layout.Inline(arena, layout.ObjectID(1))

	svg := findByTag(arena, "svg")
	if svg == nil {
		t.Fatal("svg not found")
	}
	if !almostEqual(svg.W, 200) {
		t.Errorf("svg.W = %v, want 200 (containing block width)", svg.W)
	}
	if !almostEqual(svg.H, 100) {
		t.Errorf("svg.H = %v, want 100 (200 at the 2:1 viewBox ratio)", svg.H)
	}
}

// Explicit dimensions still win over the viewBox.
func TestSizedSVGKeepsAttributes(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div style="width: 200px"><svg width="24" height="24" viewbox="0 0 24 12"></svg></div>
</body></html>`, 400)
	layout.Inline(arena, layout.ObjectID(1))

	svg := findByTag(arena, "svg")
	if svg == nil {
		t.Fatal("svg not found")
	}
	if !almostEqual(svg.W, 24) || !almostEqual(svg.H, 24) {
		t.Errorf("svg size = %vx%v, want 24x24", svg.W, svg.H)
	}
}
