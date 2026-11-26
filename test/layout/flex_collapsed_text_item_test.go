package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

func TestFlexRowPercentWidthUsesUsedSize(t *testing.T) {
	arena := sessionSheets(t, `<html><head><style>
*{box-sizing:border-box}
#inner{display:flex;flex-direction:row-reverse;width:347px;height:161px}
#media{flex-shrink:0;max-width:33.3%;height:100%;width:288px;background:#ddd}
#content{display:flex;flex:1 1 auto;flex-flow:column;padding:24px 24px 0;width:100%}
#text{width:100%}
#headline{font-size:20px;line-height:28px;margin:0}
#cta{margin-top:auto;padding:16px 0 0}
#button{width:24px;height:24px}
</style></head><body style="margin:0">
<div id="inner"><div id="media"></div><div id="content"><div id="text"><p id="headline">Product support</p></div><div id="cta"><button id="button"></button></div></div></div>
</body></html>`, 1280)
	layout.Inline(arena, layout.ObjectID(1))

	content := findByID(arena, "content")
	text := findByID(arena, "text")
	cta := findByID(arena, "cta")
	if content == nil || text == nil || cta == nil {
		t.Fatal("missing flex card boxes")
	}
	if !almostEqual(content.W+48, 231.453) {
		t.Errorf("content border width = %v, want 231.453", content.W+48)
	}
	if !almostEqual(text.W, 183.453) {
		t.Errorf("text width = %v, want 183.453 (100%% of the used flex width)", text.W)
	}
	if !almostEqual(text.H, 28) {
		t.Errorf("text height = %v, want 28", text.H)
	}
	if cta.Y < 100 {
		t.Errorf("cta.Y = %v, want a bottom-aligned position near 120", cta.Y)
	}
}

func TestFlexColumnIgnoresCollapsedTextItems(t *testing.T) {
	arena := sessionSheets(t, `<html><head><style>
#col{display:flex;flex-direction:column;height:100px;width:100px;margin-top:24px}
#a{height:20px;background:#ddd}
#b{height:20px;margin-top:auto;background:#bbb}
</style></head><body style="margin:0">
<div id="col">
  <div id="a"></div>
  <div id="b"></div>
</div>
</body></html>`, 800)

	col := findByID(arena, "col")
	a := findByID(arena, "a")
	b := findByID(arena, "b")
	if col == nil || a == nil || b == nil {
		t.Fatal("missing flex column boxes")
	}
	if !almostEqual(a.Y, col.Y) {
		t.Errorf("a.Y = %v, want %v", a.Y, col.Y)
	}
	if !almostEqual(b.Y, col.Y+80) {
		t.Errorf("b.Y = %v, want %v (auto margin must absorb all free space)", b.Y, col.Y+80)
	}
}
