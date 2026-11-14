package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

const lengthSplitDoc = "<html><body><div>x</div></body></html>"

func TestBorderSpacingKeepsAFunctionArgumentTogether(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}
	for _, c := range []struct {
		decl string
		h, v float32
	}{
		{"border-spacing: 4px", 4, 4},
		{"border-spacing: 2px 5px", 2, 5},
		{"border-spacing: calc(10vh - 4px)", 86, 86},
		{"border-spacing: calc(10vh - 4px) 3px", 86, 3},
		{"border-spacing: 3px calc(10vh - 4px)", 3, 86},
		{"border-spacing: calc(10vw + 1px) calc(1vh + 1px)", 145, 10},
		{"border-spacing: calc(10vh - 4px)\n3px", 86, 3},
	} {
		cs := styleAtViewport(t, lengthSplitDoc, "div { "+c.decl+" }", "div", vp)
		if cs.BorderSpacingH != c.h || cs.BorderSpacingV != c.v {
			t.Errorf("%q = h %v v %v, want %v/%v", c.decl, cs.BorderSpacingH, cs.BorderSpacingV, c.h, c.v)
		}
	}
}

func TestBackgroundSizeKeepsAFunctionArgumentTogether(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}
	for _, c := range []struct {
		decl       string
		size       style.BgSize
		w, h       float32
		wPct, hPct bool
	}{
		{"background-size: cover", style.BgSizeCover, 0, 0, false, false},
		{"background-size: 100px 20px", style.BgSizeLength, 100, 20, false, false},
		{"background-size: 50% 100px", style.BgSizeLength, 0.5, 100, true, false},
		{"background-size: calc(10vh - 4px)", style.BgSizeLength, 86, -1, false, false},
		{"background-size: calc(10vh - 4px) 10px", style.BgSizeLength, 86, 10, false, false},
		{"background-size: 10px calc(10vh - 4px)", style.BgSizeLength, 10, 86, false, false},
	} {
		cs := styleAtViewport(t, lengthSplitDoc, "div { "+c.decl+" }", "div", vp)
		if cs.BackgroundSize != c.size {
			t.Errorf("%q size = %v, want %v", c.decl, cs.BackgroundSize, c.size)
			continue
		}
		if c.size != style.BgSizeLength {
			continue
		}
		if cs.BgSizeW != c.w || cs.BgSizeWPct != c.wPct {
			t.Errorf("%q width = %v (pct %v), want %v (pct %v)", c.decl, cs.BgSizeW, cs.BgSizeWPct, c.w, c.wPct)
		}
		if cs.BgSizeH != c.h || cs.BgSizeHPct != c.hPct {
			t.Errorf("%q height = %v (pct %v), want %v (pct %v)", c.decl, cs.BgSizeH, cs.BgSizeHPct, c.h, c.hPct)
		}
	}
}

func TestBackgroundPositionKeepsAFunctionArgumentTogether(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}
	for _, c := range []struct {
		decl         string
		xMode, yMode style.BgPosMode
		x, y         float32
		xPct, yPct   bool
	}{
		{"background-position: center 20px", style.BgPosCenter, style.BgPosLength, 0, 20, false, false},
		{"background-position: 10px 20px", style.BgPosLength, style.BgPosLength, 10, 20, false, false},
		{"background-position: calc(10vh - 4px) 20px", style.BgPosLength, style.BgPosLength, 86, 20, false, false},
		{"background-position: 20px calc(10vh - 4px)", style.BgPosLength, style.BgPosLength, 20, 86, false, false},
		{"background-position: calc(10vh - 4px)", style.BgPosLength, style.BgPosCenter, 86, 0, false, false},
	} {
		cs := styleAtViewport(t, lengthSplitDoc, "div { "+c.decl+" }", "div", vp)
		if cs.BackgroundPosXMode != c.xMode || cs.BackgroundPosYMode != c.yMode {
			t.Errorf("%q modes = %v/%v, want %v/%v", c.decl, cs.BackgroundPosXMode, cs.BackgroundPosYMode, c.xMode, c.yMode)
			continue
		}
		if cs.BackgroundPosX != c.x || cs.BgPosXPct != c.xPct {
			t.Errorf("%q x = %v (pct %v), want %v (pct %v)", c.decl, cs.BackgroundPosX, cs.BgPosXPct, c.x, c.xPct)
		}
		if cs.BackgroundPosY != c.y || cs.BgPosYPct != c.yPct {
			t.Errorf("%q y = %v (pct %v), want %v (pct %v)", c.decl, cs.BackgroundPosY, cs.BgPosYPct, c.y, c.yPct)
		}
	}
}

func TestUnclosedFunctionLeavesTheseLengthListsAlone(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}
	baseline := styleAtViewport(t, lengthSplitDoc, "", "div", vp)
	for _, decl := range []string{
		"border-spacing: calc(10vh -",
		"background-size: calc(10vh -",
		"background-position: calc(10vh -",
	} {
		cs := styleAtViewport(t, lengthSplitDoc, "div { "+decl+" }", "div", vp)
		if cs.BorderSpacingH != baseline.BorderSpacingH || cs.BorderSpacingV != baseline.BorderSpacingV {
			t.Errorf("%q moved border-spacing to %v/%v, baseline %v/%v", decl,
				cs.BorderSpacingH, cs.BorderSpacingV, baseline.BorderSpacingH, baseline.BorderSpacingV)
		}
		if cs.BackgroundSize != baseline.BackgroundSize || cs.BgSizeW != baseline.BgSizeW || cs.BgSizeH != baseline.BgSizeH {
			t.Errorf("%q moved background-size to %v (%v/%v)", decl,
				cs.BackgroundSize, cs.BgSizeW, cs.BgSizeH)
		}
		if cs.BackgroundPosXMode != baseline.BackgroundPosXMode || cs.BackgroundPosX != baseline.BackgroundPosX {
			t.Errorf("%q moved background-position x to %v/%v", decl,
				cs.BackgroundPosXMode, cs.BackgroundPosX)
		}
	}
}

func TestBackgroundShorthandAcceptsALengthFunction(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}
	for _, c := range []struct{ decl string }{
		{"background: url(a.png) calc(10vh - 4px) no-repeat"},
	} {
		cs := styleAtViewport(t, lengthSplitDoc, "div { "+c.decl+" }", "div", vp)
		if cs.BackgroundPosXMode != style.BgPosLength || cs.BackgroundPosX != 86 {
			t.Errorf("%q x = %v/%v, want length 86", c.decl, cs.BackgroundPosXMode, cs.BackgroundPosX)
		}
		if cs.BackgroundRepeat != style.BgRepeatNoRepeat {
			t.Errorf("%q lost the repeat keyword: %v", c.decl, cs.BackgroundRepeat)
		}
	}
	for _, c := range []struct{ decl string }{
		{"background: url(a.png) center / calc(10vh - 4px)"},
	} {
		cs := styleAtViewport(t, lengthSplitDoc, "div { "+c.decl+" }", "div", vp)
		if cs.BackgroundSize != style.BgSizeLength || cs.BgSizeW != 86 {
			t.Errorf("%q size = %v/%v, want length 86", c.decl, cs.BackgroundSize, cs.BgSizeW)
		}
	}
	baseline := styleAtViewport(t, lengthSplitDoc, "", "div", vp)
	for _, decl := range []string{
		"background: linear-gradient(red, blue)",
		"background: rgba(0, 0, 0, 0.5)",
		"background: url(a.png) rgba(1 2 3)",
	} {
		cs := styleAtViewport(t, lengthSplitDoc, "div { "+decl+" }", "div", vp)
		if cs.BackgroundPosXMode != baseline.BackgroundPosXMode || cs.BackgroundPosX != baseline.BackgroundPosX {
			t.Errorf("%q moved background-position x to %v/%v, baseline %v/%v", decl,
				cs.BackgroundPosXMode, cs.BackgroundPosX, baseline.BackgroundPosXMode, baseline.BackgroundPosX)
		}
	}
}

func TestUnbalancedCloseParenDoesNotSwallowTheNextArgument(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}
	cs := styleAtViewport(t, lengthSplitDoc, "div { padding: 7px) 3px }", "div", vp)
	if cs.PaddingRight != 3 || cs.PaddingLeft != 3 {
		t.Errorf("`padding: 7px) 3px` = right %v left %v, want 3/3 (the parseable argument survives)",
			cs.PaddingRight, cs.PaddingLeft)
	}
	cs = styleAtViewport(t, lengthSplitDoc, "div { border-spacing: 2px) 5px }", "div", vp)
	if cs.BorderSpacingV != 5 {
		t.Errorf("`border-spacing: 2px) 5px` = v %v, want 5", cs.BorderSpacingV)
	}
	cs = styleAtViewport(t, lengthSplitDoc, "div { padding: calc(max(1em, 2px) + 4px) 3px }", "div", vp)
	if cs.PaddingTop != 20 || cs.PaddingRight != 3 {
		t.Errorf("`padding: calc(max(1em, 2px) + 4px) 3px` = %v/%v, want 20/3", cs.PaddingTop, cs.PaddingRight)
	}
}
