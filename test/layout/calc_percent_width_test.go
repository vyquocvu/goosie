package layout_test

import (
	"testing"
)

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
		900,
		400,
		500,
		500,
		480,
	} {
		if !almostEqual(divs[i].W, want) {
			t.Errorf("div[%d].W = %v, want %v", i, divs[i].W, want)
		}
	}
}

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
