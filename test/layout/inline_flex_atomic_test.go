package layout_test

import (
	"testing"
)

// TestInlineFlexJoinsLineFlow guards atomic inline-level flex containers: an
// inline-flex box is one unbreakable word on its parent's line, so text-align
// centers it like any inline-block. Before, only inline-block took the atomic
// path and an inline-flex fell through to the blockified-children skip -
// never joining a line - which left a centered 300px figure box at X 0 instead
// of X 250 and cost the semantic-html parity fixture most of its mismatch.
func TestInlineFlexJoinsLineFlow(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<figure style="margin: 15px 0; text-align: center;">
  <div style="display: inline-flex; width: 300px; height: 200px; align-items: center; justify-content: center;">Diagram</div>
  <figcaption>Caption</figcaption>
</figure>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 1 {
		t.Fatalf("found %d divs, want 1", len(divs))
	}
	box := divs[0]
	if !almostEqual(box.X, 250) {
		t.Errorf("box.X = %v, want 250 (centered 300px box in the 800 line)", box.X)
	}
	if !almostEqual(box.W, 300) || !almostEqual(box.H, 200) {
		t.Errorf("box = {W:%v H:%v}, want {300 200}", box.W, box.H)
	}
}
