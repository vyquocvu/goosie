package layout_test

import "testing"

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
