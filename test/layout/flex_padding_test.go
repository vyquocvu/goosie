package layout_test

import "testing"

func TestFlexItemBlockContentKeepsPaddingOnce(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div style="display:flex; width:400px">
  <div style="box-sizing:border-box; padding:0 5px 0 4px">
    <div style="width:100px; height:20px"></div>
  </div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (row, item, child)", len(divs))
	}
	item, child := divs[1], divs[2]
	if !almostEqual(item.W, 100) {
		t.Errorf("item.W = %v, want 100 (border-box 109 minus 9 padding)", item.W)
	}
	if !almostEqual(child.W, 100) {
		t.Errorf("child.W = %v, want 100", child.W)
	}
}
