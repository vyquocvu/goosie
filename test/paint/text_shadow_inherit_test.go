package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
)

// Paint-side oracles for GOOSIE-TEXTSHADOW-001: the cascade now hands
// text-shadow to descendants (css-text-decor-3 §3), and these tests prove the
// inherited layer reaches the glyphs and resolves its colour per using element.
//
// Both documents below are the probe rasters, measured against Chromium
// 131.0.6778.33 through Playwright at deviceScaleFactor 1 over a 340x120
// viewport (same CSS shell as apRaster: `font:16px serif`, `html,body{margin:0}`),
// with the declaration as the only difference between the two renders.
// "Changed" is a pixel whose three channels move by more than 24 levels in
// total, which is the harness's own effect metric.
//
//	probe10: .box{width:300px;height:60px;margin:20px;text-shadow:2px 2px 0 rgb(0,0,255)}
//	         "one two<span>three four five</span>six seven eight"
//	  Chromium: 1,182 changed px, all of them inside the block, 1,093 blue-tinted.
//	  Inside the span's own rect (71,20)-(162,38), which is where an INHERITED
//	  shadow is the only thing that can land: 468 changed px. Re-measured on the
//	  2f973603 engine this same document read 10 px there - not 0, because the
//	  preceding raw-text runs' offset copies bleed into the rect - and 605 blue
//	  px in the frame.
//	  getComputedStyle on the span: `rgb(0, 0, 255) 2px 2px 0px`.
//
//	probe11: .cc{width:300px;margin:20px;color:rgb(0,150,0);text-shadow:4px 4px}
//	         .rr{color:rgb(200,0,0)} on the span
//	  The omitted colour is currentcolor, so the layers must travel unresolved:
//	  Chromium computes `rgb(0, 150, 0) 4px 4px 0px` on the declaring element and
//	  `rgb(200, 0, 0) 4px 4px 0px` on the child. Split by ink colour, over the
//	  shifted span rect (117,24)-(233,42): red 544, green 0, other 45; over the
//	  parent's own first run (24,24)-(117,42): green 419, red 1; over the tail
//	  after the span (233,24)-(320,42): green 303, red 0. Whole frame: 722 green,
//	  545 red. On the 2f973603 engine the shifted span rect read 0 changed px of
//	  any colour, while the parent's own first run still read 386 green px: the
//	  child inherited no layers, so it had no ink to colour at all.

// tsCount classifies the pixels a declaration changed inside a rect by which
// channel dominates the resulting colour.
func tsCount(with, without *frame.Bitmap, x0, y0, x1, y1 int) (green, red, blue, other int) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			a, b := with.At(x, y), without.At(x, y)
			d := tsAbs(int(a.R())-int(b.R())) + tsAbs(int(a.G())-int(b.G())) + tsAbs(int(a.B())-int(b.B()))
			if d <= 24 {
				continue
			}
			switch {
			case int(a.G())-max(int(a.R()), int(a.B())) > 20:
				green++
			case int(a.R())-max(int(a.G()), int(a.B())) > 20:
				red++
			case int(a.B())-max(int(a.R()), int(a.G())) > 20:
				blue++
			default:
				other++
			}
		}
	}
	return
}

func tsAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// TestInheritedTextShadowPaintsChildGlyphs is the headline pixel claim: a
// declaration on an ancestor must paint on a child's glyphs. Chromium changed
// 468 px inside the span's rect; the child owns no declaration of its own, so
// nothing but inheritance can put ink there.
// qc:BR-278
func TestInheritedTextShadowPaintsChildGlyphs(t *testing.T) {
	const withTS = `.box{width:300px;height:60px;margin:20px;text-shadow:2px 2px 0 rgb(0,0,255)}`
	const without = `.box{width:300px;height:60px;margin:20px}`
	const body = `<div class="box">one two<span>three four five</span>six seven eight</div>`

	w, o := apRaster(t, withTS, body), apRaster(t, without, body)

	// The span's rect, from Chromium's and Goosie's own layout of this document
	// (cmd/dump-layout: ID=9 span X=71 Y=20 W=92 H=17).
	_, _, blueInSpan, _ := tsCount(w, o, 71, 20, 162, 38)
	totalBlue := 0
	for y := 0; y < 120; y++ {
		for x := 0; x < 340; x++ {
			c := w.At(x, y)
			if int(c.B())-int(c.R()) > 20 {
				totalBlue++
			}
		}
	}
	t.Logf("goosie: %d changed-blue px inside the span rect, %d blue px in the frame (Chromium 468 and 1,093)", blueInSpan, totalBlue)

	if blueInSpan < 300 {
		t.Errorf("%d changed blue-tinted px inside the span's rect (71,20)-(162,38), want >=300: Chromium measured 468 there and Goosie measured 0 before text-shadow inherited, so the child's glyphs are still losing the ancestor's layer", blueInSpan)
	}
	// Control: the same rect in the document without the declaration has no blue.
	for y := 20; y < 38; y++ {
		for x := 71; x < 162; x++ {
			if c := o.At(x, y); int(c.B())-int(c.R()) > 20 {
				t.Fatalf("pixel (%d,%d) of the no-declaration control is blue-tinted %v: the count above would not be measuring the inherited declaration", x, y, ibColorName(c))
			}
		}
	}
	// And the ancestor's own raw text keeps its shadow, which TASK-053's BR-233
	// already pinned: an inheritance fix must not move it.
	if totalBlue < 900 {
		t.Errorf("%d blue px in the frame, want >=900 (Chromium 1,093): the declaring element's own runs must keep painting", totalBlue)
	}
}

// TestInheritedTextShadowResolvesCurrentColorPerUsingElement is the colour half:
// a `text-shadow:4px 4px` layer carries currentcolor, and the child that
// restyles `color` must shadow in ITS colour while the parent's runs shadow in
// theirs. Chromium put 544 red px and 0 green px inside the shifted span rect,
// and 419 green px with 1 red px on the parent's first run.
// qc:BR-278
func TestInheritedTextShadowResolvesCurrentColorPerUsingElement(t *testing.T) {
	const withTS = `.cc{width:300px;margin:20px;color:rgb(0,150,0);text-shadow:4px 4px}.rr{color:rgb(200,0,0)}`
	const without = `.cc{width:300px;margin:20px;color:rgb(0,150,0)}.rr{color:rgb(200,0,0)}`
	const body = `<div class="cc">green run text <span class="rr">red child text here</span> tail green</div>`

	w, o := apRaster(t, withTS, body), apRaster(t, without, body)

	spanGreen, spanRed, _, spanOther := tsCount(w, o, 117, 24, 233, 42)
	headGreen, headRed, _, _ := tsCount(w, o, 24, 24, 117, 42)
	t.Logf("goosie: span rect green=%d red=%d other=%d; parent run green=%d red=%d (Chromium 0/544/45 and 419/1)", spanGreen, spanRed, spanOther, headGreen, headRed)

	if spanRed < 300 {
		t.Errorf("%d changed red-tinted px inside the shifted span rect (117,24)-(233,42), want >=300: Chromium measured 544, and an inherited layer that never reached the child measures 0 there", spanRed)
	}
	if spanRed <= spanGreen {
		t.Errorf("span rect ink is green %d vs red %d: the inherited currentcolor layer must resolve against the USING element's colour (rgb(200,0,0) in Chromium), not the declaring element's rgb(0,150,0)", spanGreen, spanRed)
	}
	if headGreen < 200 {
		t.Errorf("%d changed green-tinted px on the parent's own first run, want >=200 (Chromium 419): the declaring element keeps its own colour", headGreen)
	}
	if headRed > headGreen/4 {
		t.Errorf("parent run ink reads green %d / red %d: the parent has no red declaration, so its shadow must not pick the child's colour up", headGreen, headRed)
	}
}
