package layout_test

import "testing"

// qc:BR-330
func TestGridAreaNumericShorthandOverlapsItems(t *testing.T) {
	arena := sessionH(t, `<html><body style="margin:0"><div id="grid" style="display:grid;grid-template-rows:minmax(0,1fr);justify-items:center;align-items:center;width:1000px;height:290px"><div id="a" style="grid-area:1/1;width:272px;height:92px"></div><div id="b" style="grid-area:1/1;width:300px;height:40px"></div><div id="c" style="grid-area:1/1;width:160px;height:20px;margin-top:auto"></div></div></body></html>`, 1024, 768)
	grid := findByID(arena, "grid")
	a := findByID(arena, "a")
	b := findByID(arena, "b")
	c := findByID(arena, "c")
	if grid == nil || a == nil || b == nil || c == nil {
		t.Fatalf("missing objects: grid=%v a=%v b=%v c=%v", grid, a, b, c)
	}
	if !almostEqual(a.Y, 99) {
		t.Errorf("a.Y = %v, want 99", a.Y)
	}
	if !almostEqual(b.Y, 125) {
		t.Errorf("b.Y = %v, want 125", b.Y)
	}
	if !almostEqual(a.X, 364) {
		t.Errorf("a.X = %v, want 364", a.X)
	}
	if !almostEqual(b.X, 350) {
		t.Errorf("b.X = %v, want 350", b.X)
	}
	if !almostEqual(c.Y, 270) {
		t.Errorf("c.Y = %v, want 270", c.Y)
	}
}
