package layout_test

import "testing"

// A flex item with no measurable content must not keep the probe width it
// was measured at: the measure fallback handed the 10000px probe width to
// contentless items, stranding viewport-wide strips across pages. The
// fallback exists for replaced elements (whose size comes from outside the
// box model), so it stays limited to those tags.
func TestFlexItemWithoutContentDoesNotKeepProbeWidth(t *testing.T) {
	arena := session(t, `<html><body><div style="display: flex;"><div id="empty"></div><div id="text">hi</div></div></body></html>`, 800)
	empty := findByID(arena, "empty")
	if empty == nil {
		t.Fatal("empty div not found")
	}
	if empty.W > 500 {
		t.Errorf("contentless flex item W = %v, want narrow (probe width leaked)", empty.W)
	}
	text := findByID(arena, "text")
	if text == nil {
		t.Fatal("text div not found")
	}
	if text.W <= 0 || text.W > 500 {
		t.Errorf("text flex item W = %v, want small positive", text.W)
	}
}

func TestFlexReplacedItemKeepsOwnWidth(t *testing.T) {
	arena := session(t, `<html><body><div style="display: flex;"><input type="text" value="" id="inp"><div id="text">hi</div></div></body></html>`, 800)
	inp := findByID(arena, "inp")
	if inp == nil {
		t.Fatal("input not found")
	}
	if inp.W < 100 {
		t.Errorf("empty text input W = %v, want replaced size ~154", inp.W)
	}
}
