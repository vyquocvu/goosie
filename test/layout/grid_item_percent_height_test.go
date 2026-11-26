package layout_test

import "testing"

// qc:BR-330
func TestGridItemPercentHeightUsesGridContentBox(t *testing.T) {
	arena := sessionH(t, `<!DOCTYPE html><html style="height:100%"><body style="margin:0;height:100%"><div id="grid" style="display:grid;grid-template-rows:minmax(0,1fr);justify-items:center;align-items:center;height:calc(100% - 560px);min-height:150px;max-height:290px;width:1000px"><div id="wrap" style="grid-area:1/1;height:100%;max-height:92px"><svg id="mark" width="272" height="92" viewBox="0 0 272 92"><path d="M0 0H272V92H0Z" fill="red"/></svg></div></div></body></html>`, 1280, 1024)
	grid := findByID(arena, "grid")
	wrap := findByID(arena, "wrap")
	mark := findByID(arena, "mark")
	if grid == nil || wrap == nil || mark == nil {
		t.Fatalf("missing objects: grid=%v wrap=%v mark=%v", grid, wrap, mark)
	}
	if !almostEqual(grid.H, 290) {
		t.Errorf("grid.H = %v, want 290", grid.H)
	}
	if !almostEqual(wrap.H, 92) {
		t.Errorf("wrap.H = %v, want 92", wrap.H)
	}
	if !almostEqual(wrap.Y, 99) {
		t.Errorf("wrap.Y = %v, want 99", wrap.Y)
	}
	if !almostEqual(mark.X, 364) {
		t.Errorf("mark.X = %v, want 364", mark.X)
	}
	if !almostEqual(mark.Y, 99) {
		t.Errorf("mark.Y = %v, want 99", mark.Y)
	}
}
