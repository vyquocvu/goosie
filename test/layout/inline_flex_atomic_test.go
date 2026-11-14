package layout_test

import (
	"testing"
)

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
