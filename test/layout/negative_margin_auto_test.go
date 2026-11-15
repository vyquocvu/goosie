package layout_test

import (
	"testing"
)

// qc:BR-319
func TestNegativeOnePxMarginIsNotAuto(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 400px; height: 40px;">
  <div style="width: 200px; margin: -1px;">LEFT</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	if !almostEqual(divs[1].X, -1) {
		t.Errorf("box X = %v, want -1 (Chromium keeps a real -1px margin, not auto-centred)", divs[1].X)
	}
}

// qc:BR-319
func TestNegativeOnePxLeftWithAutoRightAnchorsLeft(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 800px; height: 40px;">
  <div style="width: 200px; margin-left: -1px; margin-right: auto;">LEFT</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	if !almostEqual(divs[1].X, -1) {
		t.Errorf("box X = %v, want -1 (Chromium honours margin-left:-1px and distributes only the literal auto on the right)", divs[1].X)
	}
}

// qc:BR-319
func TestWikiHeaderNegativeMarginShorthandAnchorsLeft(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 400px; height: 40px;">
  <div style="width: 200px; margin: -1px -1px -1px 0px;">HEADER</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	if !almostEqual(divs[1].X, 0) {
		t.Errorf("box X = %v, want 0 (Chromium places the box at margin-left:0px, not auto-centred)", divs[1].X)
	}
}

// qc:BR-319
func TestLiteralAutoMarginsStillCenter(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 400px; height: 40px;">
  <div style="width: 200px; margin: 0 auto;">CENTER</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	if !almostEqual(divs[1].X, 100) {
		t.Errorf("box X = %v, want 100 (literal margin:0 auto still centres in a 400px container)", divs[1].X)
	}
}
