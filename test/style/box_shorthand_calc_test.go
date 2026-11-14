package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

const shorthandDoc = "<html><body><div>x</div></body></html>"

type boxSides struct{ top, right, bottom, left float32 }

func paddingOf(cs *style.ComputedStyle) boxSides {
	return boxSites(cs.PaddingTop, cs.PaddingRight, cs.PaddingBottom, cs.PaddingLeft)
}

func marginOf(cs *style.ComputedStyle) boxSides {
	return boxSites(cs.MarginTop, cs.MarginRight, cs.MarginBottom, cs.MarginLeft)
}

func borderWidthOf(cs *style.ComputedStyle) boxSides {
	return boxSites(cs.BorderTopWidth, cs.BorderRightWidth, cs.BorderBottomWidth, cs.BorderLeftWidth)
}

func boxSites(t, r, b, l float32) boxSides { return boxSides{t, r, b, l} }

func TestBoxShorthandKeepsAFunctionArgumentTogether(t *testing.T) {
	cases := []struct {
		decl  string
		sides func(*style.ComputedStyle) boxSides
		want  boxSides
	}{
		{"padding: 5px 3px", paddingOf, boxSites(5, 3, 5, 3)},
		{"padding: 1em 2rem", paddingOf, boxSites(16, 32, 16, 32)},
		{"margin: 0 auto", marginOf, boxSites(0, style.MarginAuto, 0, style.MarginAuto)},
		{"padding-inline: 4px 8px", paddingOf, boxSites(0, 8, 0, 4)},
		{"padding: calc(10vh - 4px)", paddingOf, boxSites(86, 86, 86, 86)},
		{"padding: calc(10vh - 4px) 3px", paddingOf, boxSites(86, 3, 86, 3)},
		{"padding: 0 0 0 calc(10vh - 4px)", paddingOf, boxSites(0, 0, 0, 86)},
		{"padding: max(1rem, calc(10vh - 4px)) 3px", paddingOf, boxSites(86, 3, 86, 3)},
		{"margin: calc(10vh - 4px) 3px", marginOf, boxSites(86, 3, 86, 3)},
		{"margin: calc(1em + 2px)", marginOf, boxSites(18, 18, 18, 18)},
		{"border-width: calc(1vh + 1px) 2px", borderWidthOf, boxSites(10, 2, 10, 2)},
		{"padding-inline: calc(10vw - 4px) 2vw", paddingOf, boxSites(0, 28.8, 0, 140)},
	}
	vp := style.Viewport{W: 1440, H: 900}
	for _, c := range cases {
		cs := styleAtViewport(t, shorthandDoc, "div { "+c.decl+" }", "div", vp)
		if cs == nil {
			t.Fatalf("%q: no style computed for the div", c.decl)
		}
		if cs.FontSize != 16 {
			t.Fatalf("%q: the fixture's font-size is %v, not the 16px the em and rem expectations assume", c.decl, cs.FontSize)
		}
		got := c.sides(cs)
		if got != c.want {
			t.Errorf("%q = top %v right %v bottom %v left %v, want %v", c.decl, got.top, got.right, got.bottom, got.left, c.want)
		}
	}
}

func TestBoxShorthandWithAnUnclosedFunctionSetsNoSide(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}
	for _, decl := range []string{
		"padding: calc(10vh -",
		"padding: calc(10vh - 4px 3px",
		"margin: max(1rem",
	} {
		cs := styleAtViewport(t, shorthandDoc, "div { "+decl+" }", "div", vp)
		if cs == nil {
			t.Fatalf("%q: no style computed for the div", decl)
		}
		if cs.PaddingTop != 0 || cs.PaddingRight != 0 || cs.PaddingBottom != 0 || cs.PaddingLeft != 0 {
			t.Errorf("%q = padding %v/%v/%v/%v, want an unparseable value to leave the box alone", decl,
				cs.PaddingTop, cs.PaddingRight, cs.PaddingBottom, cs.PaddingLeft)
		}
		if cs.MarginTop != 0 || cs.MarginBottom != 0 {
			t.Errorf("%q = margin %v/%v, want an unparseable value to leave the box alone", decl, cs.MarginTop, cs.MarginBottom)
		}
	}
}

func TestFlexAndRadiusWithAnUnclosedFunctionChangeNothing(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}
	baseline := styleAtViewport(t, shorthandDoc, "", "div", vp)
	if baseline == nil {
		t.Fatal("no style computed for the undeclared div")
	}
	for _, c := range []struct {
		decl      string
		grow, shr float32
	}{
		{"flex: 0 0 calc(10vh -", 0, 0},
		{"flex: 1 1 calc(10vh - 4px", 1, 1},
	} {
		cs := styleAtViewport(t, shorthandDoc, "div { "+c.decl+" }", "div", vp)
		if cs == nil {
			t.Fatalf("%q: no style computed for the div", c.decl)
		}
		if cs.FlexGrow != c.grow || cs.FlexShrink != c.shr {
			t.Errorf("%q = grow %v shrink %v, want %v/%v", c.decl, cs.FlexGrow, cs.FlexShrink, c.grow, c.shr)
		}
		if cs.FlexBasis != baseline.FlexBasis {
			t.Errorf("%q = basis %v, want the undeclared %v", c.decl, cs.FlexBasis, baseline.FlexBasis)
		}
	}
	for _, decl := range []string{
		"border-radius: calc(10vh - 4px",
		"border-top-left-radius: calc(1em +",
	} {
		cs := styleAtViewport(t, shorthandDoc, "div { "+decl+" }", "div", vp)
		if cs == nil {
			t.Fatalf("%q: no style computed for the div", decl)
		}
		if cs.BorderRadius != baseline.BorderRadius {
			t.Errorf("%q = border-radius %v, want the undeclared %v", decl, cs.BorderRadius, baseline.BorderRadius)
		}
	}
}

func TestFlexAndRadiusShorthandsKeepAFunctionArgumentTogether(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}
	t.Run("flex-basis", func(t *testing.T) {
		for _, c := range []struct {
			decl       string
			grow, want float32
		}{
			{"flex: 1 1 auto", 1, -1},
			{"flex: 0 0 calc(10vh - 4px)", 0, 86},
		} {
			cs := styleAtViewport(t, shorthandDoc, "div { "+c.decl+" }", "div", vp)
			if cs == nil {
				t.Fatalf("%q: no style computed for the div", c.decl)
			}
			if cs.FlexGrow != c.grow || cs.FlexBasis != c.want {
				t.Errorf("%q = grow %v basis %v, want grow %v basis %v", c.decl, cs.FlexGrow, cs.FlexBasis, c.grow, c.want)
			}
		}
	})
	t.Run("border-radius", func(t *testing.T) {
		for _, c := range []struct {
			decl       string
			horizontal [4]float32
		}{
			{"border-radius: 5px 3px", [4]float32{5, 3, 5, 3}},
			{"border-radius: calc(10vh - 4px)", [4]float32{86, 86, 86, 86}},
			{"border-radius: calc(10vh - 4px) 3px", [4]float32{86, 3, 86, 3}},
		} {
			cs := styleAtViewport(t, shorthandDoc, "div { "+c.decl+" }", "div", vp)
			if cs == nil {
				t.Fatalf("%q: no style computed for the div", c.decl)
			}
			for corner, want := range c.horizontal {
				if cs.BorderRadius[corner][0] != want || cs.BorderRadius[corner][1] != want {
					t.Errorf("%q corner %d = %v/%v, want %v/%v", c.decl, corner,
						cs.BorderRadius[corner][0], cs.BorderRadius[corner][1], want, want)
				}
			}
		}
	})
	t.Run("border-radius-longhand", func(t *testing.T) {
		for _, c := range []struct {
			decl        string
			corner      int
			horiz, vert float32
		}{
			{"border-top-left-radius: 5px 3px", 0, 5, 3},
			{"border-top-right-radius: calc(10vh - 4px)", 1, 86, 86},
			{"border-bottom-right-radius: calc(10vh - 4px) 3px", 2, 86, 3},
			{"border-bottom-left-radius: calc(1em + 2px)", 3, 18, 18},
		} {
			cs := styleAtViewport(t, shorthandDoc, "div { "+c.decl+" }", "div", vp)
			if cs == nil {
				t.Fatalf("%q: no style computed for the div", c.decl)
			}
			if cs.BorderRadius[c.corner][0] != c.horiz || cs.BorderRadius[c.corner][1] != c.vert {
				t.Errorf("%q corner %d = %v/%v, want %v/%v", c.decl, c.corner,
					cs.BorderRadius[c.corner][0], cs.BorderRadius[c.corner][1], c.horiz, c.vert)
			}
			for other := 0; other < 4; other++ {
				if other != c.corner && (cs.BorderRadius[other][0] != 0 || cs.BorderRadius[other][1] != 0) {
					t.Errorf("%q set corner %d to %v/%v", c.decl, other,
						cs.BorderRadius[other][0], cs.BorderRadius[other][1])
				}
			}
		}
	})
}

func TestBoxShorthandSplitsOnEveryCSSWhitespace(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}
	for _, c := range []struct {
		desc, decl     string
		top, rightWant float32
	}{
		{"newline", "padding: 5px\n           3px", 5, 3},
		{"carriage return", "padding: 5px\r3px", 5, 3},
		{"CRLF", "padding: 5px\r\n3px", 5, 3},
		{"form feed", "padding: 5px\f3px", 5, 3},
		{"tab", "padding: 5px\t3px", 5, 3},
	} {
		cs := styleAtViewport(t, shorthandDoc, "div { "+c.decl+"; }", "div", vp)
		if cs == nil {
			t.Fatalf("%s: no style computed for the div", c.desc)
		}
		if cs.PaddingTop != c.top || cs.PaddingRight != c.rightWant {
			t.Errorf("%s: %q = top %v right %v, want %v/%v", c.desc, c.decl,
				cs.PaddingTop, cs.PaddingRight, c.top, c.rightWant)
		}
	}
}
