package style_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

var (
	bccBlue    = css.Color{R: 0, G: 0, B: 255, A: 255}
	bccGreen   = css.Color{R: 0, G: 128, B: 0, A: 255}
	bccRed     = css.Color{R: 255, G: 0, B: 0, A: 255}
	bccClear   = css.Color{R: 0, G: 0, B: 0, A: 0}
	bccTeal    = css.Color{R: 0, G: 200, B: 200, A: 255}
	bccLime    = css.Color{R: 0, G: 200, B: 0, A: 255}
	bccYellow  = css.Color{R: 200, G: 200, B: 0, A: 255}
	bccMagenta = css.Color{R: 255, G: 0, B: 255, A: 255}
	bccNight   = css.Color{R: 10, G: 20, B: 30, A: 255}
	bccGrey    = css.Color{R: 117, G: 117, B: 117, A: 255}
)

const bccHTML = `<html><body>
<div id="solidNoColor">TEXT</div>
<div id="shorthandNoColor">TEXT</div>
<div id="topNoColor">TEXT</div>
<div id="explicitBlue">TEXT</div>
<div id="transparentColor">TEXT</div>
<div id="keywordCurrent">TEXT</div>
<div id="twoComp">TEXT</div>
<div id="threeComp">TEXT</div>
<div id="fourComp">TEXT</div>
<div id="blockTwo">TEXT</div>
<div id="blockOne">TEXT</div>
<div id="inlineOne">TEXT</div>
<div id="inlineTwo">TEXT</div>
<div id="parentOf_topInherit"><div id="topInherit">TEXT</div></div>
<div id="shorthandMixed">TEXT</div>
<div id="initialKeyword">TEXT</div>
<div id="unsetKeyword">TEXT</div>
<div id="styleNone">TEXT</div>
<div id="parentOf_inheritFromParent"><div id="inheritFromParent">TEXT</div></div>
<div id="parentOf_inheritUnresolvedParent"><div id="inheritUnresolvedParent">TEXT</div></div>
<div id="invalidAfterValid">TEXT</div>
<div id="invalidOnly">TEXT</div>
<div id="outlineNoColor">TEXT</div>
<div id="colorTwice">TEXT</div>
<div id="colorThenTop">TEXT</div>
<div id="topThenColor">TEXT</div>
<div id="shorthandThenColor">TEXT</div>
<input id="placeholderNoColor" placeholder="HELLOWORLD">
</body></html>`

const bccCSS = `
#solidNoColor { border-width:6px; border-style:solid; color:rgb(0, 0, 255); }
#shorthandNoColor { border:6px solid; color:rgb(255, 0, 0); }
#topNoColor { border-top:6px solid; color:rgb(0, 128, 0); }
#explicitBlue { border:6px solid rgb(0, 0, 255); color:rgb(255, 0, 0); }
#transparentColor { border-width:6px; border-style:solid; border-color:transparent; color:rgb(0, 0, 255); }
#keywordCurrent { border-width:6px; border-style:solid; border-color:currentcolor; color:rgb(0, 200, 200); }
#twoComp { border-width:6px; border-style:solid; border-color:rgb(255, 0, 0) rgb(0, 0, 255); color:rgb(0, 128, 0); }
#threeComp { border-width:6px; border-style:solid; border-color:rgb(255, 0, 0) rgb(0, 200, 0) rgb(0, 0, 255); color:rgb(9, 9, 9); }
#fourComp { border-width:6px; border-style:solid; border-color:rgb(255, 0, 0) rgb(0, 200, 0) rgb(0, 0, 255) rgb(200, 200, 0); color:rgb(9, 9, 9); }
#blockTwo { border-width:6px; border-style:solid; border-block-color:rgb(255, 0, 0) rgb(0, 0, 255); color:rgb(0, 128, 0); }
#blockOne { border-width:6px; border-style:solid; border-block-color:rgb(255, 0, 0); color:rgb(0, 128, 0); }
#inlineOne { border-width:6px; border-style:solid; border-inline-color:rgb(0, 0, 255); color:rgb(0, 128, 0); }
#inlineTwo { border-width:6px; border-style:solid; border-inline-color:rgb(255, 0, 0) rgb(0, 0, 255); color:rgb(0, 128, 0); }
#parentOf_topInherit { color:rgb(0, 0, 255); border-color:rgb(0, 0, 255); }
#topInherit { border-width:6px; border-style:solid; border-top-color:inherit; color:rgb(0, 128, 0); }
#shorthandMixed { border:6px solid; border-left-color:rgb(255, 0, 255); color:rgb(0, 0, 255); }
#initialKeyword { border-width:6px; border-style:solid; border-color:initial; color:rgb(10, 20, 30); }
#unsetKeyword { border-width:6px; border-style:solid; border-color:unset; color:rgb(10, 20, 30); }
#styleNone { border-width:6px; border-style:none; color:rgb(0, 0, 255); }
#parentOf_inheritFromParent { color:rgb(0, 0, 255); border-color:rgb(0, 0, 255); }
#inheritFromParent { border-width:6px; border-style:solid; border-color:inherit; color:rgb(0, 128, 0); }
#parentOf_inheritUnresolvedParent { color:rgb(255, 0, 0); border-width:6px; border-style:solid; }
#inheritUnresolvedParent { border-width:6px; border-style:solid; border-color:inherit; color:rgb(0, 128, 0); }
#invalidAfterValid { border-width:6px; border-style:solid; border-color:rgb(255, 0, 0); border-color:notacolor; color:rgb(0, 128, 0); }
#invalidOnly { border-width:6px; border-style:solid; border-color:notacolor; color:rgb(0, 0, 255); }
#outlineNoColor { border-width:6px; border-style:solid; border-color:transparent; outline:3px solid; color:rgb(0, 0, 255); }
#colorTwice { border-width:6px; border-style:solid; border-color:rgb(255, 0, 0); border-color:rgb(0, 0, 255); color:rgb(0, 128, 0); }
#colorThenTop { border-width:6px; border-style:solid; border-color:rgb(255, 0, 0); border-top-color:rgb(0, 0, 255); color:rgb(0, 128, 0); }
#topThenColor { border-width:6px; border-style:solid; border-top-color:rgb(0, 0, 255); border-color:rgb(255, 0, 0); color:rgb(0, 128, 0); }
#shorthandThenColor { border:6px solid rgb(255, 0, 0); border-color:rgb(0, 0, 255); color:rgb(0, 128, 0); }
#placeholderNoColor::placeholder { border-width:2px; border-style:solid; }
`

func bccStyle(t *testing.T, id string) *style.ComputedStyle {
	t.Helper()
	doc := domtest.Parse(bccHTML)
	sheets := []*css.Stylesheet{css.Parse(bccCSS)}
	styles := style.Resolve(doc, sheets, nil)
	n := findByID(doc, id)
	if n == nil {
		t.Fatalf("no element #%s in the fixture document", id)
	}
	cs, ok := styles[n.ID]
	if !ok {
		t.Fatalf("#%s reached no computed style", id)
	}
	return cs
}

type bccSides struct {
	top, right, bottom, left css.Color
}

func bccAll(c css.Color) bccSides { return bccSides{top: c, right: c, bottom: c, left: c} }

func (w bccSides) color(side string) css.Color {
	switch side {
	case "top":
		return w.top
	case "right":
		return w.right
	case "bottom":
		return w.bottom
	}
	return w.left
}

func bccShow(c css.Color) string { return fmt.Sprintf("rgba(%d,%d,%d,%d)", c.R, c.G, c.B, c.A) }

func bccCheck(t *testing.T, id string, want bccSides) {
	t.Helper()
	cs := bccStyle(t, id)
	got := bccSides{top: cs.BorderTopColor, right: cs.BorderRightColor, bottom: cs.BorderBottomColor, left: cs.BorderLeftColor}
	for _, side := range []string{"top", "right", "bottom", "left"} {
		if got.color(side) != want.color(side) {
			t.Errorf("#%s border-%s-color = %s, want %s (measured on Chromium 153.0.8010.12 by scripts/verify-border-currentcolor.js)",
				id, side, bccShow(got.color(side)), bccShow(want.color(side)))
		}
	}
}

// qc:BR-327
func TestBorderColourDefaultsToElementColour(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want bccSides
	}{
		{"solidNoColor", bccAll(bccBlue)},
		{"shorthandNoColor", bccAll(bccRed)},
		{"topNoColor", bccAll(bccGreen)},
		{"keywordCurrent", bccAll(bccTeal)},
		{"initialKeyword", bccAll(bccNight)},
		{"unsetKeyword", bccAll(bccNight)},
		{"invalidOnly", bccAll(bccBlue)},
		{"styleNone", bccAll(bccBlue)},
	} {
		bccCheck(t, tc.id, tc.want)
	}
}

// qc:BR-327
func TestDeclaredBorderColoursWinOverTheElementColour(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want bccSides
	}{
		{"explicitBlue", bccAll(bccBlue)},
		{"transparentColor", bccAll(bccClear)},
		{"outlineNoColor", bccAll(bccClear)},
		{"invalidAfterValid", bccAll(bccRed)},
		{"twoComp", bccSides{top: bccRed, right: bccBlue, bottom: bccRed, left: bccBlue}},
		{"threeComp", bccSides{top: bccRed, right: bccLime, bottom: bccBlue, left: bccLime}},
		{"fourComp", bccSides{top: bccRed, right: bccLime, bottom: bccBlue, left: bccYellow}},
		{"shorthandMixed", bccSides{top: bccBlue, right: bccBlue, bottom: bccBlue, left: bccMagenta}},
	} {
		bccCheck(t, tc.id, tc.want)
	}
}

// qc:BR-327
func TestLogicalBorderColourShorthandsMapOneAxisEach(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want bccSides
	}{
		{"blockOne", bccSides{top: bccRed, right: bccGreen, bottom: bccRed, left: bccGreen}},
		{"blockTwo", bccSides{top: bccRed, right: bccGreen, bottom: bccBlue, left: bccGreen}},
		{"inlineOne", bccSides{top: bccGreen, right: bccBlue, bottom: bccGreen, left: bccBlue}},
		{"inlineTwo", bccSides{top: bccGreen, right: bccBlue, bottom: bccGreen, left: bccRed}},
	} {
		bccCheck(t, tc.id, tc.want)
	}
}

// qc:BR-327
func TestBorderColorInheritTakesTheParentsColour(t *testing.T) {
	bccCheck(t, "inheritFromParent", bccAll(bccBlue))
	bccCheck(t, "topInherit", bccSides{top: bccBlue, right: bccGreen, bottom: bccGreen, left: bccGreen})
}

// qc:BR-327
func TestBorderColorInheritOfAnUnresolvedParentColourKeepsTheParentsPaint(t *testing.T) {
	cs := bccStyle(t, "inheritUnresolvedParent")
	got := bccSides{top: cs.BorderTopColor, right: cs.BorderRightColor, bottom: cs.BorderBottomColor, left: cs.BorderLeftColor}
	if got != bccAll(bccRed) {
		t.Errorf("#inheritUnresolvedParent border colours = %s/%s/%s/%s, want the parent's resolved rgb(255,0,0) on all four sides: the reference computes the child's OWN colour rgb(0,128,0) here because it inherits the currentcolor keyword rather than its resolution, and that divergence is registered as GOOSIE-SHORTHANDWIDE-001",
			bccShow(got.top), bccShow(got.right), bccShow(got.bottom), bccShow(got.left))
	}
}

// qc:BR-327
func TestBorderColourSourceOrderLastDeclarationWins(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want bccSides
	}{
		{"colorTwice", bccAll(bccBlue)},
		{"colorThenTop", bccSides{top: bccBlue, right: bccRed, bottom: bccRed, left: bccRed}},
		{"topThenColor", bccAll(bccRed)},
		{"shorthandThenColor", bccAll(bccBlue)},
	} {
		bccCheck(t, tc.id, tc.want)
	}
}

// qc:BR-327
func TestPlaceholderBorderColourFollowsThePseudoOwnColour(t *testing.T) {
	doc := domtest.Parse(bccHTML)
	sheets := []*css.Stylesheet{css.Parse(bccCSS)}
	styles := style.Resolve(doc, sheets, nil)
	pseudoStyles := style.ResolvePseudoElements(doc, sheets, styles, nil)
	n := findByID(doc, "placeholderNoColor")
	if n == nil {
		t.Fatal("no #placeholderNoColor in the fixture document")
	}
	cs, ok := pseudoStyles[style.PseudoKey{NodeID: n.ID, Pseudo: "placeholder"}]
	if !ok {
		t.Fatal("#placeholderNoColor::placeholder reached no computed style")
	}
	got := bccSides{top: cs.BorderTopColor, right: cs.BorderRightColor, bottom: cs.BorderBottomColor, left: cs.BorderLeftColor}
	if got != bccAll(bccGrey) {
		t.Errorf("::placeholder border colours = %s/%s/%s/%s, want rgb(117,117,117) on all four sides — the pseudo's own UA colour, measured by scripts/verify-border-currentcolor.js",
			bccShow(got.top), bccShow(got.right), bccShow(got.bottom), bccShow(got.left))
	}
}
