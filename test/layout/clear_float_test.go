package layout_test

import "testing"

// qc:BR-259
func TestClearedFloatStacksBelowEarlierFloat(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="width: 400px;">
<div id="one" style="float: left; width: 100px; height: 100px;"></div>
<div id="two" style="float: left; clear: left; width: 100px; height: 100px;"></div>
</section>
</body></html>`, 800)

	one := findByID(arena, "one")
	two := findByID(arena, "two")
	if one == nil || two == nil {
		t.Fatal("floats not found")
	}
	if !almostEqual(one.Y, 0) {
		t.Errorf("first float Y = %v, want 0", one.Y)
	}
	if !almostEqual(two.Y, 100) {
		t.Errorf("cleared float Y = %v, want 100: clear:left must drop below the first float", two.Y)
	}
	if !almostEqual(two.X, 0) {
		t.Errorf("cleared float X = %v, want 0: nothing occupies the left edge on its new line", two.X)
	}
}

// qc:BR-259
func TestClearedFloatSeesFloatsInAnEarlierSiblingSubtree(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="width: 400px;">
<div id="wrap1">
<div id="one" style="float: left; width: 100px; height: 100px;"></div>
<div id="two" style="float: left; clear: left; width: 100px; height: 100px;"></div>
</div>
<div id="wrap2">
<div id="three" style="float: left; clear: left; width: 100px; height: 100px;"></div>
</div>
</section>
</body></html>`, 800)

	three := findByID(arena, "three")
	if three == nil {
		t.Fatal("float not found")
	}
	if !almostEqual(three.Y, 200) {
		t.Errorf("cleared float Y = %v, want 200: clear:left must see both floats of the earlier sibling", three.Y)
	}
}

// qc:BR-260
func TestFloatAfterAClearedFloatStartsNoHigher(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="width: 400px;">
<div id="wrap1">
<div id="one" style="float: left; width: 100px; height: 100px;"></div>
<div id="two" style="float: left; clear: left; width: 100px; height: 100px;"></div>
</div>
<div id="three" style="float: left; width: 100px; height: 100px;"></div>
</section>
</body></html>`, 800)

	three := findByID(arena, "three")
	if three == nil {
		t.Fatal("float not found")
	}
	if !almostEqual(three.Y, 100) {
		t.Errorf("float after a cleared float Y = %v, want 100: it may not rise above the earlier float", three.Y)
	}
}

// qc:BR-259
func TestClearedFloatInsideBlockifiedInlineSeesSiblingFloat(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="width: 400px;">
<div id="wrap1">
<div id="one" style="float: left; width: 100px; height: 100px;"></div>
</div>
<span id="wrap2" style="display: inline;">
<div id="two" style="float: left; clear: left; width: 100px; height: 100px;"></div>
</span>
</section>
</body></html>`, 800)

	two := findByID(arena, "two")
	if two == nil {
		t.Fatal("float not found")
	}
	if !almostEqual(two.Y, 100) {
		t.Errorf("cleared float Y = %v, want 100: the inline box shares its parent's formatting context", two.Y)
	}
}
