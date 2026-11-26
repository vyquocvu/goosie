package layout_test

import "testing"

// qc:BR-330
func TestCalcHeightInFlexColumnHonorsMaxHeight(t *testing.T) {
	arena := sessionH(t, `<!DOCTYPE html><html style="height:100%"><head></head><body style="height:100%;margin:0"><div id="col" style="display:flex;flex-direction:column;height:100%"><div id="logo" style="min-height:150px;height:calc(100% - 560px);max-height:290px"><div style="height:92px"></div></div><div id="search" style="height:140px"></div></div></body></html>`, 1024, 1280)
	col := findByID(arena, "col")
	logo := findByID(arena, "logo")
	search := findByID(arena, "search")
	if col == nil || logo == nil || search == nil {
		t.Fatalf("missing objects: col=%v logo=%v search=%v", col, logo, search)
	}
	if !almostEqual(col.H, 1280) {
		t.Errorf("col.H = %v, want 1280", col.H)
	}
	if !almostEqual(logo.H, 290) {
		t.Errorf("logo.H = %v, want 290", logo.H)
	}
	if !almostEqual(logo.Y, 0) {
		t.Errorf("logo.Y = %v, want 0", logo.Y)
	}
	if !almostEqual(search.Y, 290) {
		t.Errorf("search.Y = %v, want 290", search.Y)
	}
}
