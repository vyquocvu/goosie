package layout_test

import (
	"testing"
)

// A float is thrown to one side of its container and stops taking a vertical
// slot: the content that follows it starts at the same height and rides past its
// side. Stacking it in the flow instead pushed every row of lobste.rs's story
// list down by the height of its vote column.
func TestFloatTakesNoVerticalSlot(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="width: 400px;">
<aside style="float: left; width: 30px; height: 50px;"></aside>
<nav style="margin-left: 36px; height: 20px;"></nav>
</section>
</body></html>`, 800)

	voters := findByTag(arena, "aside")
	text := findByTag(arena, "nav")
	if voters == nil || text == nil {
		t.Fatal("float and sibling not found")
	}
	if !almostEqual(voters.X, 0) {
		t.Errorf("float X = %v, want 0", voters.X)
	}
	if !almostEqual(text.Y, 0) {
		t.Errorf("sibling Y = %v, want 0: it starts where the float does, not below it", text.Y)
	}
	if !almostEqual(text.X, 36) {
		t.Errorf("sibling X = %v, want 36 (its own margin clears the float)", text.X)
	}
	if !almostEqual(text.W, 364) {
		t.Errorf("sibling W = %v, want 364", text.W)
	}
}

// A right float hangs from the container's right content edge, measured from the
// outside of its own box.
func TestFloatRightHangsAtTheRightEdge(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="width: 400px;">
<aside style="float: right; width: 50px; height: 10px;"></aside>
</section>
</body></html>`, 800)

	box := findByTag(arena, "aside")
	if box == nil {
		t.Fatal("float not found")
	}
	if !almostEqual(box.X, 350) {
		t.Errorf("right float X = %v, want 350", box.X)
	}
}

// Two floats on the same side stack along the edge rather than landing on top of
// each other, which is how a row of floated columns works.
func TestSameSideFloatsSitSideBySide(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="width: 400px;">
<aside style="float: left; width: 100px; height: 10px;"></aside>
<nav style="float: left; width: 100px; height: 10px;"></nav>
</section>
</body></html>`, 800)

	first := findByTag(arena, "aside")
	second := findByTag(arena, "nav")
	if first == nil || second == nil {
		t.Fatal("floats not found")
	}
	if !almostEqual(first.X, 0) || !almostEqual(second.X, 100) {
		t.Errorf("floats at X=%v and %v, want 0 and 100", first.X, second.X)
	}
	if !almostEqual(first.Y, second.Y) {
		t.Errorf("floats at Y=%v and %v, want the same line", first.Y, second.Y)
	}
}

// A container that establishes a formatting context - here by clipping - grows
// around the floats it holds. A plain block box collapses past them instead,
// which is the behaviour a clearfix works around.
func TestFormattingContextGrowsAroundFloats(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="width: 400px; overflow: hidden;">
<aside style="float: left; width: 30px; height: 50px;"></aside>
</section>
<article style="width: 400px;">
<nav style="float: left; width: 30px; height: 50px;"></nav>
</article>
</body></html>`, 800)

	clip := findByTag(arena, "section")
	plain := findByTag(arena, "article")
	if clip == nil || plain == nil {
		t.Fatal("containers not found")
	}
	if !almostEqual(clip.H, 50) {
		t.Errorf("clipping container H = %v, want 50 (it contains its float)", clip.H)
	}
	if !almostEqual(plain.H, 0) {
		t.Errorf("plain container H = %v, want 0 (a float takes no slot in it)", plain.H)
	}
}

// An inline-block holding block content shrinks to its widest child too. The
// measure that reads the placed words cannot see past a block child, so the box
// kept the whole row: lobste.rs's tag pills, which are list items inside an
// inline-block list, filled the line and wrapped over each other.
func TestInlineBlockOfBlockContentShrinksToItsChildren(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 400px;">
<ul style="display: inline-block; margin: 0;"><li>word</li></ul>
<ol style="display: inline-block; margin: 0;"><li>next</li></ol>
</div>
</body></html>`, 800)

	ul := findByTag(arena, "ul")
	ol := findByTag(arena, "ol")
	if ul == nil || ol == nil {
		t.Fatal("inline-block lists not found")
	}
	if ul.W <= 0 || ul.W >= 400 {
		t.Errorf("ul.W = %v, want it to shrink to its widest item", ul.W)
	}
	if !almostEqual(ol.Y, ul.Y) {
		t.Errorf("ol.Y = %v, ul.Y = %v: both belong on the same row", ol.Y, ul.Y)
	}
	if ol.X <= ul.X+ul.W {
		t.Errorf("ol.X = %v, want it after the ul that ends at %v", ol.X, ul.X+ul.W)
	}
}
