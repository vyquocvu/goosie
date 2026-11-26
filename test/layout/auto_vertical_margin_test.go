package layout_test

import "testing"

func TestVerticalAutoMarginDoesNotLeakSentinelIntoBlockFlow(t *testing.T) {
	doc := `<html><body style="margin:0"><div id="box" style="margin-top:auto;margin-block-end:auto;height:20px"></div><div id="next" style="height:10px"></div></body></html>`
	arena := layoutAtViewport(t, doc, "", 800, 600)
	box := findByID(arena, "box")
	next := findByID(arena, "next")
	if box == nil || next == nil {
		t.Fatalf("box=%v next=%v", box, next)
	}
	if box.MarginTop != 0 || box.MarginBottom != 0 {
		t.Errorf("block margins = (%v, %v), want (0, 0)", box.MarginTop, box.MarginBottom)
	}
	if !almostEqual(next.Y, 20) {
		t.Errorf("next.Y = %v, want 20 (auto vertical margins resolve to zero outside flex)", next.Y)
	}
}

func TestVerticalAutoMarginDoesNotShrinkAtomicInlineLine(t *testing.T) {
	doc := `<html><body style="margin:0"><div id="line"><img id="img" src="x" style="width:10px;height:10px;margin-top:auto;margin-bottom:auto"></div></body></html>`
	arena := layoutAtViewport(t, doc, "", 800, 600)
	img := findByID(arena, "img")
	if img == nil {
		t.Fatal("img not found")
	}
	if img.MarginTop != 0 || img.MarginBottom != 0 {
		t.Errorf("atomic inline margins = (%v, %v), want (0, 0)", img.MarginTop, img.MarginBottom)
	}
}
