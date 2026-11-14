package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

const centredButtonHTML = `<html><body style="margin:0">
	<h5 style="text-align:center;font-size:40px">X<button style="display:inline-block;padding:10px;background:deeppink;color:#fff;margin:10px">PLEASE</button>Y</h5>
</body></html>`

func TestCentredInlineBlockShrinkWrapsItsLabel(t *testing.T) {
	arena := session(t, centredButtonHTML, 800)
	layout.Inline(arena, layout.ObjectID(1))
	btn := findByTag(arena, "button")
	label := findWordObject(arena, "PLEASE")
	right := findWordObject(arena, "Y")
	if btn == nil || label == nil || right == nil {
		t.Fatalf("missing boxes: button=%v label=%v Y=%v", btn != nil, label != nil, right != nil)
	}
	if want := label.W + btn.PaddingLeft + btn.PaddingRight; btn.W > want+1 {
		t.Errorf("button W=%g, want at most %g (its label plus padding, not the line)", btn.W, want)
	}
	if x0 := btn.X + btn.PaddingLeft; label.X < x0-1 || label.X > btn.X+btn.W {
		t.Errorf("label X=%g W=%g sits outside the button at X=%g W=%g", label.X, label.W, btn.X, btn.W)
	}
	if right.X <= btn.X+btn.W {
		t.Errorf("the glyph after the button starts at %g, inside the button ending at %g", right.X, btn.X+btn.W)
	}
	if right.Y >= label.Y+label.H || label.Y >= right.Y+right.H {
		t.Errorf("the glyph after the button wrapped: Y=%g H=%g against the label at Y=%g H=%g", right.Y, right.H, label.Y, label.H)
	}
}
