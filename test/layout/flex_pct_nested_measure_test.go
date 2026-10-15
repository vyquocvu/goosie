package layout_test

import "testing"

// A flex item holding a percentage-width nested flex container - pkg.go.dev's
// header pattern (`li.inline-flex > a.inline-flex[width:100%]`) - must measure
// from its content, not from the probe width. The max-content probe lays the
// subtree out at 10000px, where `width:100%` resolves to ~10000px; trusting
// that probe-resolved width as the item's max-content stranded one 10000px
// item per menu entry, blew the document to 55kpx wide, and failed
// PaintChecked's tile budget so the whole page refused to render.
const pctWidthNestedFlexHTML = `<html><body style="margin:0">
<ul style="display:flex;margin:0;padding:0;list-style:none">
<li id="c1" style="display:inline-flex;flex:none"><a href="#" style="display:inline-flex;width:100%;padding:0 24px">Why Go</a><ul style="display:none"><li>sub</li></ul></li>
<li id="c2" style="display:inline-flex;flex:none"><a href="#" style="display:inline-flex;width:100%;padding:0 24px">Docs</a></li>
</ul>
</body></html>`

func TestFlexItemWithPctWidthNestedFlexMeasuresContent(t *testing.T) {
	arena := session(t, pctWidthNestedFlexHTML, 1280)
	for _, id := range []string{"c1", "c2"} {
		it := findByID(arena, id)
		if it == nil {
			t.Fatalf("li#%s not found", id)
		}
		if it.W > 500 {
			t.Errorf("li#%s W = %v, want content-sized (<500); probe width leaked", id, it.W)
		}
		if it.X < 0 || it.X > 1280 {
			t.Errorf("li#%s X = %v, want inside the 1280 viewport", id, it.X)
		}
	}
}
