package layout_test

import (
	"testing"
)

// TestCalcPercentWidthResolvesAgainstContainingBlock guards the deferred-calc
// path end to end: a math function holding a `%` cannot resolve at cascade
// time, so the cascade keeps it as text and layout evaluates it against the
// containing width. Before the deferral these widths evaluated to zero and the
// css-features/01-calc-expressions parity fixture scored 54.75% against
// Chromium; with it the boxes below measure exactly what the expression says.
func TestCalcPercentWidthResolvesAgainstContainingBlock(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: calc(100% - 100px);">a</div>
<div style="width: min(400px, 80%);">b</div>
<div style="width: max(200px, 50%);">c</div>
<div style="width: clamp(200px, 60%, 500px);">d</div>
<div style="width: calc(min(500px, 80%) - 20px);">e</div>
</body></html>`, 1000)

	divs := findAllByTag(arena, "div")
	if len(divs) != 5 {
		t.Fatalf("found %d divs, want 5", len(divs))
	}
	for i, want := range []float32{
		900, // calc(100% - 100px) on the 1000 content box
		400, // min(400, 800)
		500, // max(200, 500)
		500, // clamp(200, 600, 500)
		480, // min(500, 800) - 20
	} {
		if !almostEqual(divs[i].W, want) {
			t.Errorf("div[%d].W = %v, want %v", i, divs[i].W, want)
		}
	}
}

// TestCalcPercentMinMaxClamp guards min-width/max-width deferral: the clamps
// apply to the used width after it resolves, against the same containing box.
func TestCalcPercentMinMaxClamp(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 700px; max-width: max(200px, 50%);">wide</div>
<div style="width: 100px; min-width: min(400px, 80%);">narrow</div>
</body></html>`, 1000)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	if !almostEqual(divs[0].W, 500) {
		t.Errorf("wide.W = %v, want 500 (max(200, 500) caps 700)", divs[0].W)
	}
	if !almostEqual(divs[1].W, 400) {
		t.Errorf("narrow.W = %v, want 400 (min(400, 800) floors 100)", divs[1].W)
	}
}
