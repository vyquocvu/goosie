package layout_test

import "testing"

// qc:BR-259
// clear:left on a float drops its top below the bottom of every left float
// already placed in the same block formatting context. Chromium 1280x1024 on
// this fixture puts "two" at (0,100): beside the first float would be X=100.
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
// A float's clearance reaches floats held by an earlier sibling's subtree: the
// two wrappers are plain blocks in the same formatting context, so the second
// wrapper's float must sit below both floats of the first.
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
// A cleared float also advances the line later floats sit on: CSS 2.1 §9.5.1
// forbids a float's outer top from being higher than an earlier float's, so the
// uncleared float after one must not jump back to Y=0. Chromium puts it at
// (100,100), beside the cleared float rather than under the first one, and the
// constraint crosses the sibling wrappers.
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
	// X is deliberately not asserted: the width a float reserves on its line is
	// not merged across sibling subtrees, so this box starts at the left edge
	// rather than beside the cleared float. Chromium puts it at (100,100).
}

// qc:BR-259
// An inline element holding block content is split into anonymous block boxes
// that stay in the parent's formatting context, so a cleared float inside it
// sees a float held by an earlier sibling. The span is display:inline until the
// block child forces the split.
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
