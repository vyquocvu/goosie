package layout_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

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

func TestFlexItemLabelStaysInsideTheItem(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div style="height:200px"></div>
<div style="display:flex">
  <button style="padding:8px 10px;font-size:16px;line-height:16px">Accept additional cookies</button>
</div>
</body></html>`, 800)

	btn := findByTag(arena, "button")
	if btn == nil {
		t.Fatal("button not found")
	}
	var label *layout.Object
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node != nil && obj.Node.Text() && strings.Contains(obj.Node.TextContent(), "Accept") {
			label = obj
			break
		}
	}
	if label == nil {
		t.Fatal("button label not found")
	}
	top := btn.Y
	bottom := btn.Y + btn.H + btn.PaddingTop + btn.PaddingBottom + btn.BorderTop + btn.BorderBottom
	if label.Y < top || label.Y > bottom {
		t.Fatalf("label Y = %v, button covers %v–%v", label.Y, top, bottom)
	}
}
