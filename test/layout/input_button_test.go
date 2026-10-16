package layout_test

import (
	"testing"
)

// Submit/reset/button inputs shrink-wrap their value label like Chrome's
// inline-block buttons. The UA sheet typed them display:inline, which sent
// them down the generic inline descent with no children to measure: every
// submit button on the web laid out at 0x0 (Google's "Tìm trên Google"
// buttons were all zero-sized).
func TestSubmitButtonHasSize(t *testing.T) {
	arena := session(t, `<html><body><form><input type="submit" value="Go"></form></body></html>`, 800)
	inp := findByTag(arena, "input")
	if inp == nil {
		t.Fatal("input not found")
	}
	if inp.W < 80 {
		t.Errorf("submit W = %v, want >= 80 (label shrink-wrap floor)", inp.W)
	}
	if inp.H <= 0 {
		t.Errorf("submit H = %v, want > 0", inp.H)
	}
}

func TestSubmitButtonDefaultLabel(t *testing.T) {
	arena := session(t, `<html><body><form><input type="submit"></form></body></html>`, 800)
	inp := findByTag(arena, "input")
	if inp == nil {
		t.Fatal("input not found")
	}
	if inp.W < 80 || inp.H <= 0 {
		t.Errorf("submit W,H = %v,%v, want >=80,>0", inp.W, inp.H)
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
