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

// A float that does not fit beside the floats already on its line starts a new
// line below them. Before this, two width:100% floats were placed side by side
// at X=0 and X=container width, and every later block was pushed one viewport
// further right: vnexpress.net builds its whole body from width:100% floats, so
// all of it rendered off-screen and the page came out blank.
// qc:BR-257
func TestFullWidthFloatsStackInsteadOfWalkingOffTheEdge(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="width: 400px;">
<aside style="float: left; width: 100%; height: 50px;"></aside>
<nav style="float: left; width: 100%; height: 70px;"></nav>
<div style="float: right; width: 100%; height: 30px;"></div>
</section>
</body></html>`, 800)

	first := findByTag(arena, "aside")
	second := findByTag(arena, "nav")
	third := findByTag(arena, "div")
	if first == nil || second == nil || third == nil {
		t.Fatal("floats not found")
	}
	if !almostEqual(first.X, 0) || !almostEqual(first.Y, 0) {
		t.Errorf("first float at (%v,%v), want (0,0)", first.X, first.Y)
	}
	if !almostEqual(second.X, 0) || !almostEqual(second.Y, 50) {
		t.Errorf("second float at (%v,%v), want (0,50): it cannot fit beside the first", second.X, second.Y)
	}
	if !almostEqual(third.X, 0) || !almostEqual(third.Y, 120) {
		t.Errorf("right float at (%v,%v), want (0,120): it starts below both", third.X, third.Y)
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

// TestFloatPercentWidthResolvesAgainstContainer guards percentage widths on
// floats: the share speaks the containing block, but the content pass used
// to re-resolve it against the settled box (`33%` of 267 instead of 800,
// yielding 89px columns).
func TestFloatPercentWidthResolvesAgainstContainer(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="float: left; width: 33.3333%; height: 100px;">a</div>
<div style="float: left; width: 33.3333%; height: 100px;">b</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	// 33.3333% of 800 = 266.67.
	for i, d := range divs {
		if d.W < 266 || d.W > 267 {
			t.Errorf("div[%d].W = %v, want ~266.67", i, d.W)
		}
	}
	if !almostEqual(divs[1].X, divs[0].W) {
		t.Errorf("second.X = %v, want %v (beside the first)", divs[1].X, divs[0].W)
	}
}

// A float belongs to the nearest formatting context, not to the block that
// happens to contain it in the DOM. When an earlier row's float sits inside a
// nested plain block, a later row's clear:both must still move below it.
// Google's legacy advanced-search rows are exactly this shape: each row is a
// clear:both block whose label and input are floats, and a flat float context
// per block stacked every row on top of the first.
func TestClearSeesFloatsNestedInPreviousSibling(t *testing.T) {
	arena := sessionSheets(t, `<html><head><style>
.group{width:600px}
.row{clear:both}
.label{float:left;width:160px;height:40px}
</style></head><body>
<div class="group">
  <div class="row" id="r1"><div class="label">L1</div></div>
  <div class="row" id="r2"><div class="label">L2</div></div>
</div>
</body></html>`, 800)
	r1 := findByID(arena, "r1")
	r2 := findByID(arena, "r2")
	if r1 == nil || r2 == nil {
		t.Fatal("rows not found")
	}
	if r2.Y < r1.Y+40 {
		t.Errorf("r2.Y = %v, want at least %v (below the previous row's float)", r2.Y, r1.Y+40)
	}
}

// Cross-axis alignment moves a flex item as a box, not just the words inside
// it. Shifting only inline content left a centred item's block children at the
// item's pre-alignment origin; inside an overflow:hidden ancestor that put the
// Google logo 12px above its clip and made it disappear.
func TestFlexCentredItemMovesItsBlockChildren(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div style="display:flex;align-items:center;height:48px;overflow:hidden">
  <div id="cell" style="display:flex;align-items:center;height:24px;overflow:hidden">
    <div id="inner" style="width:74px;height:24px"></div>
  </div>
</div>
</body></html>`, 800)
	cell := findByID(arena, "cell")
	inner := findByID(arena, "inner")
	if cell == nil || inner == nil {
		t.Fatal("cell or inner not found")
	}
	if !almostEqual(cell.Y, 12) {
		t.Errorf("cell.Y = %v, want 12 (centred in the 48px line)", cell.Y)
	}
	if !almostEqual(inner.Y, cell.Y) {
		t.Errorf("inner.Y = %v, want %v (it moves with its flex item)", inner.Y, cell.Y)
	}
}

// An inline-block whose width is auto still contributes its shrink-to-fit size
// to a flex item's max-content. Dropping it measured Google's account area as
// the sign-in button alone, so the header wrapped the button under the apps
// icon and pushed it out of the bar.
func TestMaxContentCountsAutoWidthInlineBlockInsideInline(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div style="display:flex;width:200px">
  <div id="right" style="display:flex;flex:0 0 auto">
    <div id="outer">
      <div style="display:inline"><div style="display:inline-block"><div style="width:40px;height:40px"></div></div></div>
      <a id="signin" style="display:inline-block;padding:10px 12px;margin:12px 8px 12px 10px">Sign in</a>
    </div>
  </div>
</div>
</body></html>`, 800)
	right := findByID(arena, "right")
	signin := findByID(arena, "signin")
	if right == nil || signin == nil {
		t.Fatal("right or signin not found")
	}
	if right.W < 120 {
		t.Errorf("right.W = %v, want the 40px icon and the sign-in button to share the measure", right.W)
	}
	if signin.Y > 40 {
		t.Errorf("signin.Y = %v, want it on the icon's line, not wrapped below it", signin.Y)
	}
}
