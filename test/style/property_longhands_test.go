package style_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// These longhands reach the layout and paint layers but no test fed them, so the code that
// parses them had never run in CI: `clear`, the overflow trio, `flex-wrap`,
// `background-repeat`, the border width/style/color and shorthand family, the elliptical
// corner radii, `flex`, and the grid line forms. Each case below resolves a real document
// through style.Resolve and asserts the computed field, so a refactor of the parser cannot
// pass by returning the same wrong thing from a different function.
//
// The last test in the file is the one that matters most: every property is fed values a
// hand-written page can actually contain, and none of them is allowed to panic.

func TestClearAndOverflowLonghands(t *testing.T) {
	for _, tc := range []struct{ decl, want string }{
		{"clear: left", "ClearLeft"},
		{"clear: right", "ClearRight"},
		{"clear: both", "ClearBoth"},
		{"clear: none", "ClearNone"},
		{"clear: all", "ClearNone"},
		{"overflow: hidden", "OverflowHidden"},
		{"overflow: scroll", "OverflowScroll"},
		{"overflow: auto", "OverflowAuto"},
		{"overflow: visible", "OverflowVisible"},
		{"overflow: clip", "OverflowVisible"},
	} {
		t.Run(tc.decl, func(t *testing.T) {
			s := styleFor(t, `<html><body><div class="x">x</div></body></html>`, `.x { `+tc.decl+` }`, "div")
			got := ""
			switch {
			case strings.HasPrefix(tc.decl, "clear"):
				switch s.Clear {
				case style.ClearLeft:
					got = "ClearLeft"
				case style.ClearRight:
					got = "ClearRight"
				case style.ClearBoth:
					got = "ClearBoth"
				case style.ClearNone:
					got = "ClearNone"
				}
			default:
				switch s.Overflow {
				case style.OverflowHidden:
					got = "OverflowHidden"
				case style.OverflowScroll:
					got = "OverflowScroll"
				case style.OverflowAuto:
					got = "OverflowAuto"
				case style.OverflowVisible:
					got = "OverflowVisible"
				}
			}
			if got != tc.want {
				t.Errorf("%s -> %q, want %s", tc.decl, got, tc.want)
			}
		})
	}
}

func TestOverflowAxesSetIndependently(t *testing.T) {
	s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
		`.x { overflow-x: auto }`, "div")
	if s.OverflowX != style.OverflowAuto {
		t.Errorf("OverflowX = %v, want auto", s.OverflowX)
	}
	if s.OverflowY != style.OverflowVisible {
		t.Errorf("OverflowY = %v, want the default visible; overflow-x must not set both axes", s.OverflowY)
	}
}

func TestFlexWrapAndFlexShorthand(t *testing.T) {
	for _, tc := range []struct {
		decl                string
		wrap                style.FlexWrap
		grow, shrink, basis float32
	}{
		{".x { flex-wrap: wrap }", style.FlexWrapValue, 0, 1, -1},
		{".x { flex-wrap: wrap-reverse }", style.FlexWrapReverse, 0, 1, -1},
		{".x { flex-wrap: nowrap }", style.FlexNowrap, 0, 1, -1},
		{".x { flex-wrap: reverse }", style.FlexNowrap, 0, 1, -1},
		{".x { flex: 1 }", style.FlexNowrap, 1, 1, 0},
		{".x { flex: none }", style.FlexNowrap, 0, 0, -1},
		{".x { flex: auto }", style.FlexNowrap, 1, 1, -1},
		{".x { flex: 1 2 10px }", style.FlexNowrap, 1, 2, 10},
		{".x { flex: 2 3 }", style.FlexNowrap, 2, 3, 0},
	} {
		t.Run(tc.decl, func(t *testing.T) {
			s := styleFor(t, `<html><body><div class="x">x</div></body></html>`, tc.decl, "div")
			if s.FlexWrap != tc.wrap {
				t.Errorf("FlexWrap = %v, want %v", s.FlexWrap, tc.wrap)
			}
			if s.FlexGrow != tc.grow || s.FlexShrink != tc.shrink || s.FlexBasis != tc.basis {
				t.Errorf("flex = (%v,%v,%v), want (%v,%v,%v)",
					s.FlexGrow, s.FlexShrink, s.FlexBasis, tc.grow, tc.shrink, tc.basis)
			}
		})
	}
}

func TestBackgroundRepeatLonghand(t *testing.T) {
	for _, tc := range []struct {
		decl string
		want style.BgRepeat
	}{
		{"background-repeat: repeat", style.BgRepeatRepeat},
		{"background-repeat: repeat-x", style.BgRepeatRepeatX},
		{"background-repeat: REPEAT-Y", style.BgRepeatRepeatY},
		{"background-repeat: no-repeat", style.BgRepeatNoRepeat},
		{"background-repeat:  repeat-x ", style.BgRepeatRepeatX},
		{"background-repeat: space", style.BgRepeatSpace},
		{"background-repeat: round", style.BgRepeatRound},
	} {
		t.Run(tc.decl, func(t *testing.T) {
			s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
				`.x { `+tc.decl+` }`, "div")
			if s.BackgroundRepeat != tc.want {
				t.Errorf("%s -> BackgroundRepeat = %v, want %v", tc.decl, s.BackgroundRepeat, tc.want)
			}
		})
	}
}

func TestBorderShorthandPartsInAnyOrder(t *testing.T) {
	for _, tc := range []struct {
		name, decl string
		width      float32
		styleStr   string
		r, g, b, a uint8
	}{
		{"canonical", "border: 2px solid red", 2, "solid", 255, 0, 0, 255},
		{"style last", "border: 2px red solid", 2, "solid", 255, 0, 0, 255},
		{"width last", "border: solid red 2px", 2, "solid", 255, 0, 0, 255},
		// `medium` is the initial border-width, and the engine spells it 3px.
		{"style only", "border: solid", 3, "solid", 0, 0, 0, 0},
		{"none stays zero", "border: none", 0, "none", 0, 0, 0, 0},
		{"width only", "border: 4px", 4, "", 0, 0, 0, 0},
		{"hex colour", "border: 1px dashed #0088ee", 1, "dashed", 0, 136, 238, 255},
		// Authoring style puts a space after every comma, so the colour arrives as
		// several whitespace-separated tokens. Splitting the value on spaces alone
		// loses it.
		{"rgb with spaces", "border: 1px solid rgb(0, 136, 238)", 1, "solid", 0, 136, 238, 255},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
				`.x { `+tc.decl+` }`, "div")
			if s.BorderTopWidth != tc.width || s.BorderRightWidth != tc.width ||
				s.BorderBottomWidth != tc.width || s.BorderLeftWidth != tc.width {
				t.Errorf("%s widths = (%v,%v,%v,%v), want all %v", tc.decl,
					s.BorderTopWidth, s.BorderRightWidth, s.BorderBottomWidth, s.BorderLeftWidth, tc.width)
			}
			if s.BorderTopStyle != tc.styleStr {
				t.Errorf("%s BorderTopStyle = %q, want %q", tc.decl, s.BorderTopStyle, tc.styleStr)
			}
			if c := s.BorderTopColor; c.R != tc.r || c.G != tc.g || c.B != tc.b || c.A != tc.a {
				t.Errorf("%s BorderTopColor = %+v, want {%v %v %v %v}", tc.decl, c, tc.r, tc.g, tc.b, tc.a)
			}
		})
	}
}

func TestPerSideBorderShorthand(t *testing.T) {
	s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
		`.x { border-top: 3px dotted blue; border-left: 5px }`, "div")
	if s.BorderTopWidth != 3 || s.BorderTopStyle != "dotted" {
		t.Errorf("border-top = (%v,%q), want (3,dotted)", s.BorderTopWidth, s.BorderTopStyle)
	}
	if s.BorderTopColor.B != 255 {
		t.Errorf("BorderTopColor = %+v, want blue", s.BorderTopColor)
	}
	if s.BorderLeftWidth != 5 {
		t.Errorf("BorderLeftWidth = %v, want 5", s.BorderLeftWidth)
	}
	if s.BorderRightWidth != 0 || s.BorderBottomWidth != 0 {
		t.Errorf("untouched sides = (%v,%v), want both 0", s.BorderRightWidth, s.BorderBottomWidth)
	}
}

func TestBorderWidthAndStyleAndColorShorthands(t *testing.T) {
	t.Run("border-width", func(t *testing.T) {
		s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
			`.x { border-width: 1px 2px 3px 4px }`, "div")
		if s.BorderTopWidth != 1 || s.BorderRightWidth != 2 || s.BorderBottomWidth != 3 || s.BorderLeftWidth != 4 {
			t.Errorf("border-width 4 = (%v,%v,%v,%v), want (1,2,3,4)",
				s.BorderTopWidth, s.BorderRightWidth, s.BorderBottomWidth, s.BorderLeftWidth)
		}
		three := styleFor(t, `<html><body><div class="y">y</div></body></html>`,
			`.y { border-width: 1px 2px 3px }`, "div")
		if three.BorderTopWidth != 1 || three.BorderRightWidth != 2 || three.BorderBottomWidth != 3 || three.BorderLeftWidth != 2 {
			t.Errorf("border-width 3 = (%v,%v,%v,%v), want (1,2,3,2)",
				three.BorderTopWidth, three.BorderRightWidth, three.BorderBottomWidth, three.BorderLeftWidth)
		}
		two := styleFor(t, `<html><body><div class="z">z</div></body></html>`,
			`.z { border-width: 1px 2px }`, "div")
		if two.BorderTopWidth != 1 || two.BorderRightWidth != 2 || two.BorderBottomWidth != 1 || two.BorderLeftWidth != 2 {
			t.Errorf("border-width 2 = (%v,%v,%v,%v), want (1,2,1,2)",
				two.BorderTopWidth, two.BorderRightWidth, two.BorderBottomWidth, two.BorderLeftWidth)
		}
		one := styleFor(t, `<html><body><div class="w">w</div></body></html>`,
			`.w { border-width: 6px }`, "div")
		if one.BorderTopWidth != 6 || one.BorderLeftWidth != 6 {
			t.Errorf("border-width 1 = (%v,%v), want both 6", one.BorderTopWidth, one.BorderLeftWidth)
		}
	})

	t.Run("border-style", func(t *testing.T) {
		s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
			`.x { border-style: solid dashed dotted double }`, "div")
		if s.BorderTopStyle != "solid" || s.BorderRightStyle != "dashed" ||
			s.BorderBottomStyle != "dotted" || s.BorderLeftStyle != "double" {
			t.Errorf("border-style 4 = (%q,%q,%q,%q), want (solid,dashed,dotted,double)",
				s.BorderTopStyle, s.BorderRightStyle, s.BorderBottomStyle, s.BorderLeftStyle)
		}
		two := styleFor(t, `<html><body><div class="y">y</div></body></html>`,
			`.y { border-style: solid dashed }`, "div")
		if two.BorderTopStyle != "solid" || two.BorderRightStyle != "dashed" ||
			two.BorderBottomStyle != "solid" || two.BorderLeftStyle != "dashed" {
			t.Errorf("border-style 2 = (%q,%q,%q,%q), want (solid,dashed,solid,dashed)",
				two.BorderTopStyle, two.BorderRightStyle, two.BorderBottomStyle, two.BorderLeftStyle)
		}
	})

	t.Run("border-color", func(t *testing.T) {
		s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
			`.x { border-color: rgb(255, 0, 0) blue }`, "div")
		if s.BorderTopColor.R != 255 || s.BorderTopColor.B != 0 {
			t.Errorf("BorderTopColor = %+v, want red with its spaces kept together", s.BorderTopColor)
		}
		if s.BorderRightColor.B != 255 {
			t.Errorf("BorderRightColor = %+v, want blue", s.BorderRightColor)
		}
		if s.BorderBottomColor.R != 255 {
			t.Errorf("BorderBottomColor = %+v, want the first value repeated for top/bottom", s.BorderBottomColor)
		}
		three := styleFor(t, `<html><body><div class="y">y</div></body></html>`,
			`.y { border-color: red green blue }`, "div")
		if three.BorderTopColor.R != 255 || three.BorderRightColor.G != 128 ||
			three.BorderBottomColor.B != 255 || three.BorderLeftColor.G != 128 {
			t.Errorf("border-color 3 = (%+v,%+v,%+v,%+v), want left to mirror right",
				three.BorderTopColor, three.BorderRightColor, three.BorderBottomColor, three.BorderLeftColor)
		}
	})
}

func TestCornerRadiusIncludingElliptical(t *testing.T) {
	s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
		`.x { border-top-left-radius: 10px 20px }`, "div")
	if got := s.BorderRadius[0]; got[0] != 10 || got[1] != 20 {
		t.Errorf("border-top-left-radius = %v, want [10 20]", got)
	}
	if got := s.BorderRadius[2]; got[0] != 0 || got[1] != 0 {
		t.Errorf("bottom-right corner = %v, want untouched", got)
	}

	s = styleFor(t, `<html><body><div class="y">y</div></body></html>`,
		`.y { border-bottom-right-radius: 50% }`, "div")
	// A percentage radius cannot be resolved until the box is known, so css.Value carries it
	// unresolved as -2-pct and layout turns it back into a share of the corner's box.
	if got := s.BorderRadius[2][0]; got != -52 {
		t.Errorf("bottom-right radius = %v, want -52 (the 50%% sentinel encoding), not a dropped or literal 50", got)
	}
}

func TestGridLineFormsReduceToSpan(t *testing.T) {
	for _, tc := range []struct {
		decl string
		want int
	}{
		{"grid-column: span 2", 2},
		{"grid-column: SPAN 3", 3},
		{"grid-column: 1 / 4", 3},
		{"grid-column: auto", 1},
		{"grid-column: 3", 1},
		{"grid-column: header / main", 1},
		{"grid-column: span", 1},
		{"grid-column: span x", 1},
		{"grid-column: 4 / 2", 1},
	} {
		t.Run(tc.decl, func(t *testing.T) {
			s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
				`.x { `+tc.decl+` }`, "div")
			if got := s.GridColumnSpan; got != tc.want {
				t.Errorf("%s -> GridColumnSpan = %d, want %d", tc.decl, got, tc.want)
			}
		})
	}
	row := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
		`.x { grid-row: span 2 }`, "div")
	if row.GridRowSpan != 2 {
		t.Errorf("grid-row: span 2 -> GridRowSpan = %d, want 2", row.GridRowSpan)
	}
	if row.GridColumnSpan != 0 {
		t.Errorf("grid-row set GridColumnSpan = %d; the two properties are independent", row.GridColumnSpan)
	}
}

// The style attribute is the highest-priority author origin and the one route every property
// takes without a stylesheet. It is asserted here so the inline parser and the cascade entry
// point are both exercised, and so an inline border reaches the same fields as a rule's.
func TestInlineStyleAttributeReachesComputedFields(t *testing.T) {
	s := styleFor(t,
		`<html><body><div style="clear: both; border-top: 2px dashed red; background-repeat: repeat-x">x</div></body></html>`,
		"", "div")
	if s.Clear != style.ClearBoth {
		t.Errorf("Clear = %v, want both", s.Clear)
	}
	if s.BorderTopWidth != 2 || s.BorderTopStyle != "dashed" || s.BorderTopColor.R != 255 {
		t.Errorf("inline border-top = (%v,%q,%+v), want (2,dashed,red)",
			s.BorderTopWidth, s.BorderTopStyle, s.BorderTopColor)
	}
	if s.BackgroundRepeat != style.BgRepeatRepeatX {
		t.Errorf("BackgroundRepeat = %v, want repeat-x", s.BackgroundRepeat)
	}
}

func TestInlineStyleLosesToImportantAndBeatsRules(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="x" style="clear: both">x</div></body></html>`,
		`.x { clear: right !important }`, "div")
	if s.Clear != style.ClearRight {
		t.Errorf("Clear = %v, want right; !important must outrank the style attribute", s.Clear)
	}
	s = styleFor(t,
		`<html><body><div class="x" style="clear: both">x</div></body></html>`,
		`.x { clear: right }`, "div")
	if s.Clear != style.ClearBoth {
		t.Errorf("Clear = %v, want both; the style attribute must outrank a plain rule", s.Clear)
	}
}

// The table alignment reset distinguishes an inherited centre from one the table itself
// declares: <center><table> puts its cells at the start edge, while table{text-align:center}
// centres their contents. Both halves are asserted, because dropping the own-declaration
// check would silently un-centre the second page.
func TestInheritedTextAlignResetsAtTableButOwnDeclarationDoesNot(t *testing.T) {
	fromParent := styleFor(t, `<html><body><table><tbody><tr><td>x</td></tr></tbody></table></body></html>`,
		`body { text-align: center }`, "table")
	if fromParent.TextAlign != style.TextAlignLeft {
		t.Errorf("inherited centre: table TextAlign = %v, want left", fromParent.TextAlign)
	}
	own := styleFor(t, `<html><body><table><tbody><tr><td>x</td></tr></tbody></table></body></html>`,
		`table { text-align: center }`, "table")
	if own.TextAlign != style.TextAlignCenter {
		t.Errorf("own declaration: table TextAlign = %v, want center", own.TextAlign)
	}
	viaInline := styleFor(t, `<html><body><table style="text-align: center"><tbody><tr><td>x</td></tr></tbody></table></body></html>`,
		"", "table")
	if viaInline.TextAlign != style.TextAlignCenter {
		t.Errorf("style attribute: table TextAlign = %v, want center; an inline declaration is the table's own", viaInline.TextAlign)
	}
}

// hostile are values a real page, a truncated stylesheet, or a fuzzed response body can
// contain. A property value must never take the renderer down: the parse either lands on the
// keyword's default or is ignored, and the document keeps painting.
func TestMalformedPropertyValuesNeverPanic(t *testing.T) {
	props := []string{
		"clear", "overflow", "overflow-x", "overflow-y", "flex-wrap", "flex",
		"background-repeat", "background", "background-image", "background-size",
		"background-position", "border", "border-top", "border-width", "border-style",
		"border-color", "border-radius", "border-top-left-radius", "grid-column", "grid-row",
		"grid-template-columns", "grid-template-areas", "margin", "padding", "font",
		"font-family", "font-size", "line-height", "text-align", "text-decoration",
		"white-space", "display", "position", "float", "box-sizing", "width", "height",
		"top", "left", "opacity", "z-index", "clip", "color", "list-style-type",
	}
	values := []string{"", " ", ";", "---", "()", ")", "(", "1/", "/", "rgb(", "rgb()",
		"span", "1 /", "solid solid solid solid solid", "calc(", "calc()", "var(",
		"url(", "url()", ",", ",,,", "1px 1px 1px 1px 1px", strings.Repeat("a", 512)}
	for _, prop := range props {
		for _, value := range values {
			decl := prop + ": " + value
			t.Run(decl, func(t *testing.T) {
				// Recovered per combination rather than left to the testing package: a
				// repanic kills the binary, so the first bad value would hide every property
				// after it, and the point of this sweep is the full list of them.
				sheet := ".x { " + decl + " }"
				resolveSurvives(t, func() { styleFor(t, `<html><body><div class="x">x</div></body></html>`, sheet, "div") })
				inlineHTML := `<html><body><div style="` + decl + `">x</div></body></html>`
				resolveSurvives(t, func() { styleFor(t, inlineHTML, "", "div") })
			})
		}
	}
}

func resolveSurvives(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("resolving %q panicked: %v", t.Name(), r)
		}
	}()
	f()
}

// TestInlineDataURIBackgroundSurvivesSemicolons pins declaration splitting
// for style attributes carrying base64 data URIs: the semicolon in
// `data:image/png;base64,...` sits inside url(...) and must not end the
// background-image declaration, or the image silently vanishes while a
// later background-size still parses.
func TestInlineDataURIBackgroundSurvivesSemicolons(t *testing.T) {
	s := styleFor(t,
		`<html><body><div style="background-image:url('data:image/png;base64,iVBORw0KGgo=');background-size:50px">x</div></body></html>`,
		"", "div")
	if s.BackgroundImage != "data:image/png;base64,iVBORw0KGgo=" {
		t.Errorf("BackgroundImage = %q, want the data URI intact", s.BackgroundImage)
	}
	if s.BackgroundSize != style.BgSizeLength || s.BgSizeW != 50 || s.BgSizeH != -1 {
		t.Errorf("BackgroundSize = (%v,%v,%v), want (Length,50,auto)", s.BackgroundSize, s.BgSizeW, s.BgSizeH)
	}
}

// TestBackgroundRepeatTwoAxes pins per-axis repeat parsing: `repeat round`
// repeats horizontally and rounds vertically, and each axis falls back
// independently.
func TestBackgroundRepeatTwoAxes(t *testing.T) {
	for _, c := range []struct {
		decl string
		x, y style.BgRepeat
	}{
		{"background-repeat: repeat round", style.BgRepeatRepeat, style.BgRepeatRound},
		{"background-repeat: round repeat", style.BgRepeatRound, style.BgRepeatRepeat},
		{"background-repeat: space no-repeat", style.BgRepeatSpace, style.BgRepeatNoRepeat},
		{"background-repeat: repeat-x", style.BgRepeatRepeatX, style.BgRepeatRepeatX},
	} {
		s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
			`.x { `+c.decl+` }`, "div")
		if s.BackgroundRepeat != c.x || s.BackgroundRepeatY != c.y {
			t.Errorf("%s -> (%v,%v), want (%v,%v)", c.decl, s.BackgroundRepeat, s.BackgroundRepeatY, c.x, c.y)
		}
	}
}

// TestBackgroundOriginClipBoxes pins the box keywords and their initials:
// origins position against the padding box, clips paint through the border
// box, and clip:text parses for the cascade even though paint cannot mask it.
func TestBackgroundOriginClipBoxes(t *testing.T) {
	s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
		`.x { background-origin: content-box; background-clip: text; }`, "div")
	if s.BackgroundOrigin != style.BgBoxContent {
		t.Errorf("BackgroundOrigin = %v, want content-box", s.BackgroundOrigin)
	}
	if s.BackgroundClip != style.BgBoxText {
		t.Errorf("BackgroundClip = %v, want text", s.BackgroundClip)
	}
	d := styleFor(t, `<html><body><div>x</div></body></html>`, "", "div")
	if d.BackgroundOrigin != style.BgBoxPadding || d.BackgroundClip != style.BgBoxBorder {
		t.Errorf("initials = (%v,%v), want (padding,border)", d.BackgroundOrigin, d.BackgroundClip)
	}
}

// TestBackgroundPositionThreeFourValues pins keyword-plus-offset positions:
// `left 50px` offsets from the start edge, `right 25px` from the end edge,
// and a percentage offset resolves against the free space.
func TestBackgroundPositionThreeFourValues(t *testing.T) {
	s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
		`.x { background-position: left 50px center; }`, "div")
	if s.BackgroundPosXMode != style.BgPosStartOffset || s.BackgroundPosX != 50 {
		t.Errorf("x = (%v,%v), want (start-offset,50)", s.BackgroundPosXMode, s.BackgroundPosX)
	}
	if s.BackgroundPosYMode != style.BgPosCenter {
		t.Errorf("y = %v, want center", s.BackgroundPosYMode)
	}
	s = styleFor(t, `<html><body><div class="x">x</div></body></html>`,
		`.x { background-position: right 25px top 75%; }`, "div")
	if s.BackgroundPosXMode != style.BgPosEndOffset || s.BackgroundPosX != 25 {
		t.Errorf("x = (%v,%v), want (end-offset,25)", s.BackgroundPosXMode, s.BackgroundPosX)
	}
	if s.BackgroundPosYMode != style.BgPosStartOffset || s.BackgroundPosY != 0.75 || !s.BgPosYPct {
		t.Errorf("y = (%v,%v,%v), want (start-offset,0.75,pct)", s.BackgroundPosYMode, s.BackgroundPosY, s.BgPosYPct)
	}
}

// TestBackgroundShorthandKeepsFirstLayer pins that later comma-separated
// layers never bleed into the painted layer's fields: their positions,
// sizes and repeats belong to layers the painter does not draw.
func TestBackgroundShorthandKeepsFirstLayer(t *testing.T) {
	s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
		`.x { background: linear-gradient(to right, yellow 50%, blue 50%) 0 0 / 100% 100% no-repeat, radial-gradient(farthest-side at 0 50%, black, transparent) 0 0 / 20px 100% no-repeat; }`, "div")
	if s.BackgroundRepeat != style.BgRepeatNoRepeat || s.BackgroundRepeatY != style.BgRepeatNoRepeat {
		t.Errorf("repeat = (%v,%v), want (no-repeat,no-repeat)", s.BackgroundRepeat, s.BackgroundRepeatY)
	}
	if s.BgSizeW != 1 || !s.BgSizeWPct || s.BgSizeH != 1 || !s.BgSizeHPct {
		t.Errorf("size = (%v,%v,%v,%v), want (1,pct,1,pct)", s.BgSizeW, s.BgSizeWPct, s.BgSizeH, s.BgSizeHPct)
	}
	if s.BackgroundPosXMode != style.BgPosLength || s.BackgroundPosX != 0 || s.BackgroundPosYMode != style.BgPosLength || s.BackgroundPosY != 0 {
		t.Errorf("pos = (%v,%v,%v,%v), want lengths 0,0", s.BackgroundPosXMode, s.BackgroundPosX, s.BackgroundPosYMode, s.BackgroundPosY)
	}
}

// TestBoxShadowCurrentColorResolvesAgainstOwnColor pins used-value timing:
// an omitted shadow color stays flagged through the cascade (paint resolves
// it against the element's own final `color`), and an explicit color survives
// untouched.
func TestBoxShadowCurrentColorResolvesAgainstOwnColor(t *testing.T) {
	s := styleFor(t, `<html><body><span>x</span></body></html>`,
		`span { color: limegreen; box-shadow: -3em 0em; }`, "span")
	if len(s.BoxShadow) != 1 {
		t.Fatalf("expected 1 shadow, got %v", s.BoxShadow)
	}
	if !s.BoxShadow[0].ColorIsCurrent {
		t.Errorf("omitted shadow color should stay flagged, got %+v", s.BoxShadow[0])
	}
	s2 := styleFor(t, `<html><body><span>x</span></body></html>`,
		`span { color: limegreen; box-shadow: 10px 5px 5px red; }`, "span")
	if len(s2.BoxShadow) != 1 || s2.BoxShadow[0].ColorIsCurrent || s2.BoxShadow[0].Color.R != 255 {
		t.Errorf("explicit shadow color should survive, got %+v", s2.BoxShadow)
	}
}

// TestBoxShadowInheritCopiesParentLayers pins the `inherit` keyword: the
// child takes the parent's computed layers verbatim.
func TestBoxShadowInheritCopiesParentLayers(t *testing.T) {
	doc := domtest.Parse(`<html><body><div><p>x</p></div></body></html>`)
	sheets := []*css.Stylesheet{css.Parse(`
		div { box-shadow: 10px 5px 5px red; }
		p { box-shadow: inherit; }`)}
	styles := style.Resolve(doc, sheets, nil)
	var divS, pS *style.ComputedStyle
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n; c != nil; c = c.NextSibling {
			if c.Element() {
				if c.Data == "div" {
					divS = styles[c.ID]
				}
				if c.Data == "p" {
					pS = styles[c.ID]
				}
			}
			walk(c.FirstChild)
		}
	}
	walk(&doc.Node)
	if divS == nil || pS == nil {
		t.Fatalf("missing styles: div=%v p=%v", divS != nil, pS != nil)
	}
	if len(pS.BoxShadow) != 1 || pS.BoxShadow[0].OffsetX != 10 || pS.BoxShadow[0].OffsetY != 5 {
		t.Errorf("inherited shadow = %+v, want parent's 10px/5px layer", pS.BoxShadow)
	}
	if pS.BoxShadow[0].Color.R != 255 || pS.BoxShadow[0].ColorIsCurrent {
		t.Errorf("inherited shadow color = %+v, want resolved red", pS.BoxShadow[0])
	}
}

// TestBoxShadowInheritKeepsCurrentColorFlag pins that an inherited
// `currentcolor` layer stays flagged: paint resolves it against the child,
// not the parent whose cascade already ran.
func TestBoxShadowInheritKeepsCurrentColorFlag(t *testing.T) {
	doc := domtest.Parse(`<html><body><div><p>x</p></div></body></html>`)
	sheets := []*css.Stylesheet{css.Parse(`
		div { color: transparent; box-shadow: 10px 5px 5px currentcolor; }
		p { color: limegreen; box-shadow: inherit; }`)}
	styles := style.Resolve(doc, sheets, nil)
	var pS *style.ComputedStyle
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n; c != nil; c = c.NextSibling {
			if c.Element() && c.Data == "p" {
				pS = styles[c.ID]
			}
			walk(c.FirstChild)
		}
	}
	walk(&doc.Node)
	if pS == nil || len(pS.BoxShadow) != 1 {
		t.Fatalf("expected 1 inherited shadow, got %+v", pS)
	}
	if !pS.BoxShadow[0].ColorIsCurrent {
		t.Errorf("inherited currentcolor layer should stay flagged, got %+v", pS.BoxShadow[0])
	}
}

// TestBackgroundClipBorderAreaParses pins the border-area keyword for clip
// (and origin, where it sizes like the border box): the ring between the
// border box and the padding box is what shows a background through a
// transparent border.
func TestBackgroundClipBorderAreaParses(t *testing.T) {
	s := styleFor(t, `<html><body><div>x</div></body></html>`,
		`div { background-clip: border-area; }`, "div")
	if s.BackgroundClip != style.BgBoxBorderArea {
		t.Errorf("BackgroundClip = %v, want BgBoxBorderArea", s.BackgroundClip)
	}
}
