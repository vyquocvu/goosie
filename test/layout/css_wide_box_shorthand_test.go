package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

const cssWideBoxLayoutHTML = `<!doctype html><html><body style="margin:0;padding:0">
<div id="marginInheritParent" style="margin:20px;padding:1px;width:200px"><div id="marginInherit" style="margin:inherit;width:50px;height:10px"></div></div>
<div id="marginInitialParent" style="margin:20px;padding:1px;width:200px"><div id="marginInitial" style="margin:9px;margin:initial;width:50px;height:10px"></div></div>
<div id="marginUnsetParent" style="margin:20px;padding:1px;width:200px"><div id="marginUnset" style="margin:9px;margin:unset;width:50px;height:10px"></div></div>
<div id="paddingParent" style="padding:1px 2px 3px 4px;width:200px"><div id="paddingInherit" style="padding:inherit;width:50px;height:10px"></div><div id="paddingInitial" style="padding:9px;padding:initial;width:50px;height:10px"></div><div id="paddingUnset" style="padding:9px;padding:unset;width:50px;height:10px"></div></div>
<div id="insetContainer" style="position:relative;width:400px;height:300px"><div id="insetParent" style="inset:10px 20px 30px 40px"><div id="insetInherit" style="position:absolute;inset:inherit;width:50px;height:20px"></div><div id="insetInitial" style="position:absolute;inset:9px;inset:initial;width:50px;height:20px"></div><div id="insetUnset" style="position:absolute;inset:9px;inset:unset;width:50px;height:20px"></div></div></div>
</body></html>`

func cssWideLayoutObject(t *testing.T, arena *layout.Arena, id string) *layout.Object {
	t.Helper()
	obj := layoutObjectWithID(arena, id)
	if obj == nil {
		t.Fatalf("missing layout box #%s", id)
	}
	return obj
}

func TestCSSWideBoxKeywordsUseChromiumGeometry(t *testing.T) {
	arena := session(t, cssWideBoxLayoutHTML, 800)
	layout.Positioning(arena, layout.ObjectID(1), 800, 600)

	marginInheritParent := cssWideLayoutObject(t, arena, "marginInheritParent")
	marginInherit := cssWideLayoutObject(t, arena, "marginInherit")
	if !almostEqual(marginInherit.X, marginInheritParent.X+marginInheritParent.PaddingLeft+20) ||
		!almostEqual(marginInherit.Y, marginInheritParent.Y+marginInheritParent.PaddingTop+20) {
		t.Errorf("margin:inherit origin = %v,%v, want Chromium's %v,%v", marginInherit.X, marginInherit.Y,
			marginInheritParent.X+marginInheritParent.PaddingLeft+20, marginInheritParent.Y+marginInheritParent.PaddingTop+20)
	}
	if !almostEqual(marginInherit.W, 50) || !almostEqual(marginInherit.H, 10) {
		t.Errorf("margin:inherit size = %vx%v, want Chromium's 50x10", marginInherit.W, marginInherit.H)
	}

	marginInitialParent := cssWideLayoutObject(t, arena, "marginInitialParent")
	marginInitial := cssWideLayoutObject(t, arena, "marginInitial")
	marginUnsetParent := cssWideLayoutObject(t, arena, "marginUnsetParent")
	marginUnset := cssWideLayoutObject(t, arena, "marginUnset")
	for _, c := range []struct {
		id     string
		child  *layout.Object
		parent *layout.Object
	}{
		{"marginInitial", marginInitial, marginInitialParent},
		{"marginUnset", marginUnset, marginUnsetParent},
	} {
		if !almostEqual(c.child.X, c.parent.X+c.parent.PaddingLeft) || !almostEqual(c.child.Y, c.parent.Y+c.parent.PaddingTop) {
			t.Errorf("%s origin = %v,%v, want Chromium's %v,%v", c.id, c.child.X, c.child.Y,
				c.parent.X+c.parent.PaddingLeft, c.parent.Y+c.parent.PaddingTop)
		}
		if !almostEqual(c.child.W, 50) || !almostEqual(c.child.H, 10) {
			t.Errorf("%s size = %vx%v, want Chromium's 50x10", c.id, c.child.W, c.child.H)
		}
	}

	paddingParent := cssWideLayoutObject(t, arena, "paddingParent")
	paddingInherit := cssWideLayoutObject(t, arena, "paddingInherit")
	if !almostEqual(paddingInherit.X, paddingParent.X+paddingParent.PaddingLeft) ||
		!almostEqual(paddingInherit.Y, paddingParent.Y+paddingParent.PaddingTop) {
		t.Errorf("padding:inherit origin = %v,%v, want Chromium's %v,%v", paddingInherit.X, paddingInherit.Y,
			paddingParent.X+paddingParent.PaddingLeft, paddingParent.Y+paddingParent.PaddingTop)
	}
	if !almostEqual(paddingInherit.PaddingTop, 1) || !almostEqual(paddingInherit.PaddingRight, 2) ||
		!almostEqual(paddingInherit.PaddingBottom, 3) || !almostEqual(paddingInherit.PaddingLeft, 4) {
		t.Errorf("padding:inherit used padding = %v/%v/%v/%v, want Chromium's 1/2/3/4",
			paddingInherit.PaddingTop, paddingInherit.PaddingRight, paddingInherit.PaddingBottom, paddingInherit.PaddingLeft)
	}
	if !almostEqual(paddingInherit.W, 50) || !almostEqual(paddingInherit.H, 10) {
		t.Errorf("padding:inherit content size = %vx%v, want Chromium's 50x10", paddingInherit.W, paddingInherit.H)
	}
	outerW := paddingInherit.W + paddingInherit.PaddingLeft + paddingInherit.PaddingRight
	outerH := paddingInherit.H + paddingInherit.PaddingTop + paddingInherit.PaddingBottom
	if !almostEqual(outerW, 56) || !almostEqual(outerH, 14) {
		t.Errorf("padding:inherit outer size = %vx%v, want Chromium's 56x14", outerW, outerH)
	}
	for _, id := range []string{"paddingInitial", "paddingUnset"} {
		padding := cssWideLayoutObject(t, arena, id)
		if !almostEqual(padding.PaddingTop, 0) || !almostEqual(padding.PaddingRight, 0) ||
			!almostEqual(padding.PaddingBottom, 0) || !almostEqual(padding.PaddingLeft, 0) {
			t.Errorf("%s used padding = %v/%v/%v/%v, want Chromium's 0/0/0/0", id,
				padding.PaddingTop, padding.PaddingRight, padding.PaddingBottom, padding.PaddingLeft)
		}
		if !almostEqual(padding.W, 50) || !almostEqual(padding.H, 10) {
			t.Errorf("%s content size = %vx%v, want Chromium's 50x10", id, padding.W, padding.H)
		}
	}

	insetContainer := cssWideLayoutObject(t, arena, "insetContainer")
	insetInherit := cssWideLayoutObject(t, arena, "insetInherit")
	if !almostEqual(insetInherit.X, insetContainer.X+40) || !almostEqual(insetInherit.Y, insetContainer.Y+10) {
		t.Errorf("inset:inherit origin = %v,%v, want Chromium's %v,%v", insetInherit.X, insetInherit.Y,
			insetContainer.X+40, insetContainer.Y+10)
	}
	for _, id := range []string{"insetInitial", "insetUnset"} {
		inset := cssWideLayoutObject(t, arena, id)
		if !almostEqual(inset.X, insetContainer.X) || !almostEqual(inset.Y, insetContainer.Y) {
			t.Errorf("%s origin = %v,%v, want Chromium's static %v,%v", id, inset.X, inset.Y, insetContainer.X, insetContainer.Y)
		}
		if !almostEqual(inset.W, 50) || !almostEqual(inset.H, 20) {
			t.Errorf("%s size = %vx%v, want Chromium's 50x20", id, inset.W, inset.H)
		}
	}
}

func TestCSSWidePaddingInheritUsesChromiumPercentageGeometry(t *testing.T) {
	arena := session(t, `<!doctype html><html><body style="margin:0;padding:0">
<div style="width:400px;padding:10% 20%"><div id="percentageInherit" style="padding:inherit;width:50px;height:10px"></div></div>
</body></html>`, 800)
	parent := findByTag(arena, "div")
	child := cssWideLayoutObject(t, arena, "percentageInherit")
	if parent == nil {
		t.Fatal("missing percentage padding parent")
	}
	if !almostEqual(child.PaddingTop, 40) || !almostEqual(child.PaddingRight, 80) ||
		!almostEqual(child.PaddingBottom, 40) || !almostEqual(child.PaddingLeft, 80) {
		t.Errorf("inherited percentage padding = %v/%v/%v/%v, want Chromium's 40/80/40/80",
			child.PaddingTop, child.PaddingRight, child.PaddingBottom, child.PaddingLeft)
	}
	if !almostEqual(child.X, parent.X+parent.PaddingLeft) || !almostEqual(child.Y, parent.Y+parent.PaddingTop) {
		t.Errorf("inherited percentage padding origin = %v,%v, want Chromium's %v,%v", child.X, child.Y,
			parent.X+parent.PaddingLeft, parent.Y+parent.PaddingTop)
	}
	outerW := child.W + child.PaddingLeft + child.PaddingRight
	outerH := child.H + child.PaddingTop + child.PaddingBottom
	if !almostEqual(outerW, 210) || !almostEqual(outerH, 90) {
		t.Errorf("inherited percentage padding outer size = %vx%v, want Chromium's 210x90", outerW, outerH)
	}
}
