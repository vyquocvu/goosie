package layout_test

import "testing"

func TestMinHeightHoldsChildMarginBeforeBlockifiedInline(t *testing.T) {
	arena := session(t, `<html><body style="margin:0;line-height:15px">
<div id="parent" style="min-height:46px"><br><div id="child" style="height:28px;margin-bottom:24px"></div></div>
<span id="footer"><div id="inner"><div id="next" style="margin-top:19px;height:15px"></div></div></span>
</body></html>`, 800)

	parent := layoutObjectWithID(arena, "parent")
	child := layoutObjectWithID(arena, "child")
	footer := layoutObjectWithID(arena, "footer")
	inner := layoutObjectWithID(arena, "inner")
	next := layoutObjectWithID(arena, "next")
	if parent == nil || child == nil || footer == nil || inner == nil || next == nil {
		t.Fatalf("missing boxes: parent=%v child=%v footer=%v inner=%v next=%v",
			parent != nil, child != nil, footer != nil, inner != nil, next != nil)
	}
	if !almostEqual(parent.H, 46) {
		t.Errorf("parent.H = %v, want min-height 46", parent.H)
	}
	if !almostEqual(footer.Y, 65) {
		t.Errorf("footer.Y = %v, want 65 (parent bottom 46 plus next top margin 19)", footer.Y)
	}
	if !almostEqual(next.Y, 65) {
		t.Errorf("next.Y = %v, want 65 (the top margin collapses through the blockified span)", next.Y)
	}
	if !almostEqual(inner.Y, 65) {
		t.Errorf("inner.Y = %v, want 65", inner.Y)
	}
}
