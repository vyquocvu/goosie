package layout_test

import "testing"

// qc:BR-330
func TestGridFrRowHonorsAlignItemsCenter(t *testing.T) {
	cases := []struct {
		name  string
		style string
	}{
		{
			name:  "definite height",
			style: "height:290px",
		},
		{
			name:  "clamped calc height",
			style: "height:calc(100% - 560px);min-height:150px;max-height:290px",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arena := sessionH(t, `<!DOCTYPE html><html style="height:100%"><body style="margin:0;height:100%"><div id="grid" style="display:grid;align-items:center;justify-items:center;grid-template-rows:minmax(0,1fr);width:1000px;`+tc.style+`"><div id="mark" style="width:272px;height:92px"></div></div></body></html>`, 1280, 1024)
			grid := findByID(arena, "grid")
			mark := findByID(arena, "mark")
			if grid == nil || mark == nil {
				t.Fatalf("missing objects: grid=%v mark=%v", grid, mark)
			}
			if len(grid.GridRows) != 1 || !almostEqual(grid.GridRows[0], 290) {
				t.Fatalf("grid.GridRows = %v, want [290]", grid.GridRows)
			}
			if !almostEqual(grid.H, 290) {
				t.Errorf("grid.H = %v, want 290", grid.H)
			}
			if !almostEqual(mark.Y, 99) {
				t.Errorf("mark.Y = %v, want 99", mark.Y)
			}
		})
	}
}
