package layout_test

import "testing"

// qc:BR-330
func TestFlexBorderBoxMinHeightAddsPadding(t *testing.T) {
	arena := sessionH(t, `<html><body style="margin:0">
<header id="h" style="display:flex;align-items:center;box-sizing:border-box;min-height:50px;padding:8px 44px">
<div id="item" style="width:10px;height:10px"></div>
</header>
</body></html>`, 1280, 600)
	h, item := findByID(arena, "h"), findByID(arena, "item")
	if h == nil || item == nil {
		t.Fatalf("h=%v item=%v", h, item)
	}
	if !almostEqual(h.BorderH(), 50) {
		t.Errorf("h.BorderH() = %v, want 50 (min-height wins when content is shorter)", h.BorderH())
	}
	if !almostEqual(item.Y, 20) {
		t.Errorf("item.Y = %v, want 20 (centered in the 34px content box)", item.Y)
	}
}

// qc:BR-330
func TestFlexBorderBoxMinHeightDoesNotShrinkTallerContent(t *testing.T) {
	arena := sessionH(t, `<html><body style="margin:0">
<header id="h" style="display:flex;align-items:center;box-sizing:border-box;min-height:50px;padding:8px 44px">
<div id="item" style="width:10px;height:50px"></div>
</header>
</body></html>`, 1280, 600)
	h, item := findByID(arena, "h"), findByID(arena, "item")
	if h == nil || item == nil {
		t.Fatalf("h=%v item=%v", h, item)
	}
	if !almostEqual(h.BorderH(), 66) {
		t.Errorf("h.BorderH() = %v, want 66 (50px of content plus 8px padding on each side)", h.BorderH())
	}
	if !almostEqual(item.Y, 8) {
		t.Errorf("item.Y = %v, want 8", item.Y)
	}
}

// qc:BR-330
func TestEmptyBlockCollapsesItsOwnMargins(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<section id="c" style="padding:1px">
<div id="empty" style="margin:24px 0"></div>
<div id="next" style="height:10px"></div>
</section>
</body></html>`, 800)
	next := findByID(arena, "next")
	if next == nil {
		t.Fatal("next not found")
	}
	if !almostEqual(next.Y, 25) {
		t.Errorf("next.Y = %v, want 25 (one collapsed 24px gap after the padding)", next.Y)
	}
}

// qc:BR-330
func TestEmptyBlockMarginCollapsesThroughItsParent(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div id="wrap"><div id="empty" style="margin:24px 0"></div></div>
<div id="next" style="height:10px"></div>
</body></html>`, 800)
	wrap, next := findByID(arena, "wrap"), findByID(arena, "next")
	if wrap == nil || next == nil {
		t.Fatalf("wrap=%v next=%v", wrap, next)
	}
	if !almostEqual(wrap.Y, 24) {
		t.Errorf("wrap.Y = %v, want 24", wrap.Y)
	}
	if !almostEqual(next.Y, 24) {
		t.Errorf("next.Y = %v, want 24 (the empty box's margins collapse to one gap before the parent)", next.Y)
	}
}

// qc:BR-330
func TestEmptyBlockMarginCollapsesThroughNestedEmptyParents(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div id="wrap"><div id="outer"><div id="empty" style="margin:24px 0"><div id="inner"></div></div></div></div>
<div id="next" style="height:10px"></div>
</body></html>`, 800)
	wrap, next := findByID(arena, "wrap"), findByID(arena, "next")
	if wrap == nil || next == nil {
		t.Fatalf("wrap=%v next=%v", wrap, next)
	}
	if !almostEqual(wrap.Y, 24) {
		t.Errorf("wrap.Y = %v, want 24", wrap.Y)
	}
	if !almostEqual(next.Y, 24) {
		t.Errorf("next.Y = %v, want 24 (nested empty parents share the same collapsed margin)", next.Y)
	}
}

// qc:BR-330
func TestRelativeEmptyBlockMarginCollapsesThroughParent(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div id="wrap"><div id="empty" style="position:relative;margin:24px 0"></div></div>
<div id="next" style="height:10px"></div>
</body></html>`, 800)
	wrap, next := findByID(arena, "wrap"), findByID(arena, "next")
	if wrap == nil || next == nil {
		t.Fatalf("wrap=%v next=%v", wrap, next)
	}
	if !almostEqual(wrap.Y, 24) {
		t.Errorf("wrap.Y = %v, want 24", wrap.Y)
	}
	if !almostEqual(next.Y, 24) {
		t.Errorf("next.Y = %v, want 24 (a relative empty box still collapses its margins)", next.Y)
	}
}

// qc:BR-330
func TestFlexPreservesNegativeMargins(t *testing.T) {
	arena := sessionH(t, `<html><body style="margin:0">
<div id="row" style="display:flex;height:40px">
<div id="lead" style="margin-left:-8px;width:20px;height:20px"></div>
</div>
</body></html>`, 400, 200)
	lead := findByID(arena, "lead")
	if lead == nil {
		t.Fatal("lead not found")
	}
	if !almostEqual(lead.X, -8) {
		t.Errorf("lead.X = %v, want -8 (a literal negative margin is not auto)", lead.X)
	}
}

// qc:BR-330
func TestFlexAutoMarginIsNotTreatedAsNegative(t *testing.T) {
	arena := sessionH(t, `<html><body style="margin:0">
<div id="row" style="display:flex;justify-content:center;width:200px;height:40px">
<div id="lead" style="margin-left:auto;margin-right:auto;width:20px;height:20px"></div>
</div>
</body></html>`, 400, 200)
	lead := findByID(arena, "lead")
	if lead == nil {
		t.Fatal("lead not found")
	}
	if !almostEqual(lead.X, 90) {
		t.Errorf("lead.X = %v, want 90 (auto margins still absorb free space)", lead.X)
	}
}
