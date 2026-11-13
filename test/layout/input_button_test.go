package layout_test

import (
	"testing"
)

// Submit/reset/button inputs shrink-wrap their value label like Chrome's
// inline-block buttons. The UA sheet typed them display:inline, which sent
// them down the generic inline descent with no children to measure: every
// submit button on the web laid out at 0x0 (Google's "Tìm trên Google"
// buttons were all zero-sized).
//
// The shrink-wrap has no floor. Chromium, measured 2026-11-12 at its own
// 13.3333px control font, answers 16px wide for an empty submit (its 12px of
// padding and 4px of border and nothing else), 33.796875px for "Go" and
// 57.5px for "Submit"; the 80px minimum this test used to pin was the engine's
// own guess at a platform rule and no page can rely on it.
func TestSubmitButtonHasSize(t *testing.T) {
	arena := session(t, `<html><body><form><input type="submit" value="Go"></form></body></html>`, 800)
	inp := findByTag(arena, "input")
	if inp == nil {
		t.Fatal("input not found")
	}
	// Two chars at the half-em advance this host measures with, and nothing else:
	// the 12px of padding and 4px of border are the UA sheet's, added by the box
	// model rather than folded in here.
	if !almostEqual(inp.W, 13.3333) {
		t.Errorf("submit W = %v, want the label alone (13.3333): a floor is not shrink-wrap", inp.W)
	}
	if inp.H <= 0 {
		t.Errorf("submit H = %v, want > 0", inp.H)
	}
	if _, _, bx1, _ := inp.BorderRect(); !almostEqual(bx1-inp.X, 29.3333) {
		t.Errorf("submit border box W = %v, want 29.3333 (label + 12px padding + 4px border)", bx1-inp.X)
	}
}

func TestSubmitButtonDefaultLabel(t *testing.T) {
	arena := session(t, `<html><body><form><input type="submit"></form></body></html>`, 800)
	inp := findByTag(arena, "input")
	if inp == nil {
		t.Fatal("input not found")
	}
	// "Submit" is six chars at the same advance: 40px of label, 56px of box.
	if !almostEqual(inp.W, 40) || inp.H <= 0 {
		t.Errorf("submit W,H = %v,%v, want 40 and > 0 (the default label, shrink-wrapped)", inp.W, inp.H)
	}
}

func TestButtonsSitSideBySide(t *testing.T) {
	arena := session(t, `<html><body><form><input type="submit" value="A"><input type="submit" value="B"></form></body></html>`, 800)
	inputs := findAllByTag(arena, "input")
	if len(inputs) != 2 {
		t.Fatalf("found %d inputs, want 2", len(inputs))
	}
	if inputs[1].X <= inputs[0].X {
		t.Errorf("buttons not side by side: x0=%v x1=%v", inputs[0].X, inputs[1].X)
	}
	if inputs[1].Y != inputs[0].Y {
		t.Errorf("buttons not on one line: y0=%v y1=%v", inputs[0].Y, inputs[1].Y)
	}
}
