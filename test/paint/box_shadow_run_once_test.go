package paint_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

var srShadowColor = frame.RGBA(200, 0, 0, 128)

func srShadowFills(t *testing.T, css, body string) []string {
	t.Helper()
	var out []string
	for _, c := range ibDisplayList(t, css, body, 800) {
		if c.Kind == paint.CmdFill && c.Color == srShadowColor {
			out = append(out, fmt.Sprintf("(%d,%d,%d,%d)", c.Rect.X0, c.Rect.Y0, c.Rect.X1, c.Rect.Y1))
		}
	}
	return out
}

const (
	srLayer = `.box{width:300px;height:76px;margin:20px;background:#ffffff;` +
		`box-shadow:0 0 10px 6px rgba(200,0,0,0.5)}`
	srBare = `.box{width:300px;height:76px;margin:20px;background:#ffffff}`
)

var srShapes = []struct {
	name, body string
}{
	{"empty block", `<div class="box"></div>`},
	{"raw text", `<div class="box">one two three four</div>`},
	{"words in spans with block-owned whitespace",
		`<div class="box"><span>one</span> <span>two</span> <span>three</span> <span>four</span></div>`},
	{"raw text split by a block sibling",
		`<div class="box">one two<div style="height:20px"></div>three four</div>`},
}

// qc:BR-280
func TestShadowDeclarationIsEmittedOnceWhateverTheTextShape(t *testing.T) {
	base := srShadowFills(t, srLayer, srShapes[0].body)
	if len(base) != 16 {
		t.Fatalf("the empty block emitted %d shadow fills, want the 16 strips of one declaration", len(base))
	}
	for _, shape := range srShapes[1:] {
		got := srShadowFills(t, srLayer, shape.body)
		if strings.Join(got, " ") != strings.Join(base, " ") {
			t.Errorf("%s: shadow fills are %v, want exactly the element's own %v", shape.name, got, base)
		}
	}
}

// qc:BR-280
func TestRunShadowSpillInsideTheElementIsGone(t *testing.T) {
	for _, shape := range srShapes {
		bm := apRaster(t, srLayer, shape.body)
		tint := srInteriorRed(bm)
		if tint != 0 {
			t.Errorf("%s: %d pixels inside the block's interior read red-dominant, want 0 as Chromium measures; "+
				"a text run is painting the element's shadow again inside the box", shape.name, tint)
		}
		if dark := srInteriorDark(bm); dark == 0 && shape.name != "empty block" {
			t.Errorf("%s: no glyph ink left inside the block", shape.name)
		}
	}
}

// qc:BR-280
func TestShadowPaintsTheSamePixelsWhateverTheTextShape(t *testing.T) {
	base := apChanged(apRaster(t, srBare, srShapes[0].body), apRaster(t, srLayer, srShapes[0].body))
	if len(base) != 15332 {
		t.Fatalf("the declaration changed %d pixels on the empty block, want the measured 15332 "+
			"(Chromium changes 16636 on this document; the old ring model changed 13056)", len(base))
	}
	for _, shape := range srShapes[1:] {
		got := apChanged(apRaster(t, srBare, shape.body), apRaster(t, srLayer, shape.body))
		if d := srSymDiff(base, got); d != 0 {
			t.Errorf("%s: the declaration changed %d pixels, %d of them differ from the element-only shape's set",
				shape.name, len(got), d)
		}
	}
}

// qc:BR-280
func TestInlineBlockDoesNotDuplicateItsOwnShadow(t *testing.T) {
	css := `.page{width:300px;margin:20px}` +
		`.ib{display:inline-block;width:60px;height:20px;background:rgb(0,200,0);` +
		`box-shadow:0 0 0 6px rgba(200,0,0,0.5)}`
	body := `<div class="page"><span class="ib">x</span> tail</div>`
	got := srShadowFills(t, css, body)
	want := []string{"(14,14,86,20)", "(14,40,86,46)", "(14,20,20,40)", "(80,20,86,40)"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("inline-block shadow fills = %v, want the four strips of its own ring %v "+
			"(the pre-fix engine emitted eight: the run inside it painted the ring again)", got, want)
	}
}

// qc:BR-280
func TestPseudoElementKeepsItsOwnShadow(t *testing.T) {
	css := `.page{width:300px;margin:20px;box-shadow:0 0 0 6px rgba(200,0,0,0.5)}` +
		`.page::before{content:"pre";box-shadow:0 0 0 4px rgba(200,0,0,0.5)}`
	got := srShadowFills(t, css, `<div class="page">one two</div>`)
	want := []string{
		"(14,14,326,20)", "(14,38,326,44)", "(14,20,20,38)", "(320,20,326,38)",
		"(16,16,45,20)", "(16,38,45,42)", "(16,20,20,38)", "(41,20,45,38)",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("shadow fills = %v, want the block's ring then the ::before's own ring %v", got, want)
	}
}

// qc:BR-280
func TestInlineGeneratorFragmentsStillEmitTheShadow(t *testing.T) {
	css := `.page{width:300px;margin:20px}` +
		`.s{box-shadow:0 0 0 6px rgba(200,0,0,0.5)}`
	got := srShadowFills(t, css, `<div class="page"><span class="s">one two three four</span></div>`)
	if len(got) == 0 {
		t.Fatalf("an inline element's shadow reached no fragment at all: the union box paints none, " +
			"so this would drop the declaration entirely (measured 28 fills, one strip group per run)")
	}
	if len(got)%4 != 0 {
		t.Errorf("inline shadow fills = %d, want whole strip groups of 4: %v", len(got), got)
	}
}

// qc:BR-280
func TestRunGatedShadowStaysBehindItsElementAndText(t *testing.T) {
	cmds := ibDisplayList(t, srLayer, srShapes[1].body, 800)
	firstShadow, bg, firstText := -1, -1, -1
	for i, c := range cmds {
		if c.Kind == paint.CmdFill && c.Color == srShadowColor && firstShadow < 0 {
			firstShadow = i
		}
		if c.Kind == paint.CmdFill && c.Color == frame.RGB(255, 255, 255) && bg < 0 {
			bg = i
		}
		if c.Kind == paint.CmdText && firstText < 0 {
			firstText = i
		}
	}
	if firstShadow < 0 || bg < 0 || firstText < 0 {
		t.Fatalf("missing commands: shadow=%d background=%d text=%d", firstShadow, bg, firstText)
	}
	if !(firstShadow < bg && bg < firstText) {
		t.Errorf("paint order shadow@%d background@%d text@%d, want shadow behind background behind text",
			firstShadow, bg, firstText)
	}
}

func srInteriorRed(bm *frame.Bitmap) int {
	n := 0
	for y := 20; y < 96; y++ {
		for x := 20; x < 320; x++ {
			c := bm.At(x, y)
			if int(c.R())-int(c.G()) > 25 && int(c.G()) > 40 {
				n++
			}
		}
	}
	return n
}

func srInteriorDark(bm *frame.Bitmap) int {
	n := 0
	for y := 20; y < 96; y++ {
		for x := 20; x < 320; x++ {
			c := bm.At(x, y)
			if int(c.R())+int(c.G())+int(c.B()) < 270 {
				n++
			}
		}
	}
	return n
}

func srSymDiff(a, b map[[2]int]struct{}) int {
	n := 0
	for k := range a {
		if _, ok := b[k]; !ok {
			n++
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			n++
		}
	}
	return n
}
