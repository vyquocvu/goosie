package paint_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// These are the oracles for GOOSIE-SHADOWRUN-001: layout splices a box per word
// for every text node and fills it with the generating element's computed style
// verbatim (style.inheritStyle: `cs := *parent`), box-shadow included, and
// paintBoxShadow had no owner test - so a block declaring one shadow emitted it
// once per text run as well as once for itself. A ring is the box minus itself,
// so each run's ring landed inside the element's own, already-filled interior.
//
// The background half of this duplication closed as GOOSIE-INLINEBG-001 and the
// anonymous-box half as BR-232/BR-233; TASK-053 §6 named the run path as the
// remaining multiplier and BR-232's fixture deliberately wrapped its text in
// spans so that the anonymous contribution was the only one on the list. These
// tests put raw text back in.
//
// Every number was measured before it was encoded - Playwright with Chromium
// 131.0.6778.33 and cmd/goosie-headless / cmd/dump-paint on this host, 800x160
// DPR 1, on /tmp/b280/sr_plain.html, sr_inline.html, sr_anon.html,
// sr_span_only.html, sr_span_only2.html (a 300x76 block at margin 30 with
// `background:#ffffff` and `box-shadow:0 0 10px 6px rgba(200,0,0,.5)`):
//
//	red-dominant pixels inside the block's own interior
//	  Chromium              0 on every shape (only glyph ink: 289 / 289 / 293)
//	  Goosie pre-fix        1,197 raw text | 2,573 words in spans | 2,019 split bands
//	  Goosie post-fix           0        |      0           |      0
//	shadow-coloured fills on the display list, one declared layer
//	  empty block 16 | raw text 128 pre -> 16 post | spans 64 pre -> 16 post
//	  split bands 112 pre -> 16 post
//	changed pixels (with-shadow minus without-shadow), same 340x120 tile
//	  Goosie post-fix: 15,332 on all four shapes, symmetric difference 0 between
//	  them (13,056 under the concentric-ring blur this slice replaced; pre-fix the
//	  raw-text shapes differed from the empty block by 7,158 / 4,740 / 7,154 px)
//	  Chromium: 16,636 on the same two documents, and its own changed sets differ
//	  from Goosie's on 1,304 px - every one of them a 1-level tail pixel at the
//	  edge of the field, none of them past 6 levels
//
// The shapes that must NOT change are pinned too, because a gate that silences
// every fragment is the wrong rule: an inline generator's fragments own the
// shadow (the union box paints none), an inline-block's own box owns it, and a
// ::before box owns its own.
//
// The pixel half lives in testdata/render/css-features/23-box-shadow-run-once.html
// (row 118 of the corpus): scored on one corpus against Chromium 131.0.6778.33, the
// fixed binary reads 96.358% PASS and the pre-fix binary 88.807% FAIL, with 19,401
// red-tinted interior px against Chromium's 1,071 - every one of them the
// inline-block's own ring, a row the fixed engine keeps at 1,114. Seven of its eight
// rows use a hard spread ring because the outer blur's Gaussian tail was still
// unmeasured then; it is the separable field BR-281 measures against Chromium.
// Blurring every row scored that defect instead of this one (87.44% fixed /
// 78.16% pre-fix, both under the bar).

// srShadowColor is rgba(200,0,0,0.5) as the display list stores it: premultiplied,
// so the red byte is 100 and the alpha byte 128.
var srShadowColor = frame.RGBA(200, 0, 0, 128)

// srShadowFills returns the rects of every shadow-coloured fill, in display-list
// order, so an emission shows up as its strip group.
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

// srShapes are the text shapes that must all end at the element's own emission.
// The spans shape is the one that measured worst (2,573 interior pixels): the
// words belong to the spans, but the whitespace between them is still the block's
// own text, so its runs kept emitting.
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

// The declaration is the element's, so its text must not add one command to the
// list. Comparing the whole ordered rect list against the shape that has no text
// at all is stronger than counting: it also pins which rects the emission covers.
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

// The visible half, and the number the defect was filed on: Chromium keeps its
// shadow strictly outside its own box, so nothing red-dominant survives inside a
// block that filled its background.
// qc:BR-280
func TestRunShadowSpillInsideTheElementIsGone(t *testing.T) {
	for _, shape := range srShapes {
		bm := apRaster(t, srLayer, shape.body)
		tint := srInteriorRed(bm)
		if tint != 0 {
			t.Errorf("%s: %d pixels inside the block's interior read red-dominant, want 0 as Chromium measures; "+
				"a text run is painting the element's shadow again inside the box", shape.name, tint)
		}
		// The gate must not have taken the text with it: glyph ink is still there
		// (208 dark pixels on this host's face, Chromium 289 on its own - the
		// difference is GOOSIE-FONTMETRIC-001, not this slice).
		if dark := srInteriorDark(bm); dark == 0 && shape.name != "empty block" {
			t.Errorf("%s: no glyph ink left inside the block", shape.name)
		}
	}
}

// Shape invariance at the rasterizer, which is what a page actually gets: the set
// of pixels the declaration changes must not depend on how the element's text is
// shaped. 15,332 pixels on all four shapes here, symmetric difference 0 (Chromium
// changes 16,636 on the same two documents, and 1,304 px of the difference between
// the two engines' sets are 1-level tail pixels) - the pre-fix raw-text shapes
// differed from the empty block by 7,158 / 4,740 / 7,154 pixels past 30 levels.
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

// The shapes the gate must leave alone. An inline-block declares the shadow on
// itself and its own box emits it once: measured on /tmp/b280/sr_span_only2.html,
// Chromium reads 508 red-dominant pixels around a 60x20 inline-block with
// `box-shadow:0 0 0 6px rgba(200,0,0,.5)` and Goosie 509 (502 pixels past 30
// levels disagree before the fix, 182 after), while the pre-fix engine also let
// the inline-block's own text run emit the same ring.
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

// Generated content is the documented exception in fragmentOwnsDecorations: a
// ::before box owns its own decorations, so silencing it would be a new defect.
// Measured on the fourth probe: the block's ring plus the ::before's own ring,
// eight fills, unchanged by this slice.
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

// The legitimate half, pinned so a future gate cannot collapse it: an inline
// element's own union box paints no decoration, so its fragments are the only
// thing that does. This is the shape registered as GOOSIE-INLINEBOXSHADOW-001 -
// Goosie emits one ring per word, Chromium one per line fragment (a span with
// `box-shadow:0 0 0 6px rgba(200,0,0,.5)` over `one two three four` puts 36
// red-dominant pixels inside the fragment's own band in Chromium against Goosie's
// 839) - and it is deliberately left as it is here, because fixing it means
// emitting per line fragment rather than per run. What this test insists on is
// that the fragments keep emitting something.
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

// Paint order survives the gate: a shadow goes behind its element, and the
// element's background behind its text.
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

// srInteriorRed counts red-dominant pixels inside the 300x76 block's own interior.
// The threshold is the one that separates a shadow tint from glyph ink: glyph
// pixels are dark in every channel, a `rgba(200,0,0,.5)` tint over white is
// (227,127,127) and over the 240-gray page (220,120,120).
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
