package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

// textRunAlignHTML is GOOSIE-VALIGNTEXT-001: the same classes BR-272 measured on
// atomic inline-blocks, measured on a run of *text*. Chromium answers them all,
// and the numbers below are its, taken with Playwright on 2026-11-12 at an
// 800x600 viewport in standards mode - the doctype is part of the oracle for the
// same reason BR-272 states it there.
//
// The rows are built so the classes are actually distinguishable. On a line
// whose only font is 16px every edge value lands on the same edge the strut
// already reaches, so a row of that shape cannot tell `top` from `baseline`:
// either the run's box dominates the line (and its top is the line's top
// whatever the class says) or it fits inside the strut (and both edges are the
// strut's). Each group below therefore moves one of the three numbers - the
// block's font, the run's line height, or the strut - away from the others.
//
// The strut of a `.st` row is 30 above / 20 below a 50px line (a 16px face,
// 14/4/18, with 16px of half-leading), so a run whose own line height is 40px
// asks 25 above and 15 below and neither it nor the line moves on its own.
const textRunAlignHTML = `<!DOCTYPE html><html><head><style>
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.row { width: 500px; }
.st { line-height: 50px; }
.run40 { line-height: 40px; }
.big74 { font-size: 64px; line-height: 74px; }
.tiny10 { font-size: 64px; line-height: 10px; }
.chip { display: inline-block; width: 40px; height: 74px; }
.short { display: inline-block; width: 40px; height: 40px; }
</style></head><body>
<div class="row st" id=g0>words<span class=run40 style="vertical-align:baseline">tail</span> more</div>
<div class="row st" id=g1>words<span class=run40 style="vertical-align:top">tail</span> more</div>
<div class="row st" id=g2>words<span class=run40 style="vertical-align:bottom">tail</span> more</div>
<div class="row st" id=g3>words<span class=run40 style="vertical-align:text-top">tail</span> more</div>
<div class="row st" id=g4>words<span class=run40 style="vertical-align:text-bottom">tail</span> more</div>
<div class="row st" id=g5>words<span class=run40 style="vertical-align:5px">tail</span> more</div>
<div class="row st" id=g6>words<span class=run40 style="vertical-align:-5px">tail</span> more</div>
<div class=row id=g7>words<span class=run40 style="vertical-align:top">tail</span> more</div>
<div class="row st" id=m0>words<span class=big74 style="vertical-align:top">Big</span> more</div>
<div class="row st" id=m1>words<span class=big74 style="vertical-align:bottom">Big</span> more</div>
<div class="row st" id=m2>words<span class=big74 style="vertical-align:text-top">Big</span> more</div>
<div class="row st" id=m3>words<span class=big74 style="vertical-align:text-bottom">Big</span> more</div>
<div class="row st" id=m4>words<span class=big74 style="vertical-align:5px">Big</span> more</div>
<div class="row st" id=m5>words<span class=big74 style="vertical-align:-15px">Big</span> more</div>
<div class=row id=d0>words<span style="font-size:64px;vertical-align:baseline">Big</span> more</div>
<div class=row id=d1>words<span style="font-size:64px;vertical-align:top">Big</span> more</div>
<div class=row id=d2>words<span style="font-size:64px;vertical-align:5px">Big</span> more</div>
<div class="row st" id=u0>words<span class=chip style="vertical-align:bottom"></span> more</div>
<div class="row st" id=u1>words<span class=chip style="vertical-align:top"></span> more</div>
<div class="row st" id=u2>words<span class=chip style="vertical-align:baseline"></span> more</div>
<div class=row id=u3>words<span class=short style="vertical-align:bottom"></span> more</div>
<div class=row id=n0>words<span class=tiny10 style="vertical-align:top">Big</span> more</div>
<div class=row id=n1>words<span class=tiny10 style="vertical-align:bottom">Big</span> more</div>
</body></html>`

// textRunOffsets reads the three numbers a vertical-align class decides on a
// line: how tall the line box is, where the block's own text starts inside it
// (which is where its baseline sits, minus the block's ascent) and where the
// aligned run's content area starts.
func textRunOffsets(t *testing.T, arena *layout.Arena, row string) (lineH, blockText, run float32) {
	t.Helper()
	r := findByID(arena, row)
	if r == nil {
		t.Fatalf("#%s has no layout object", row)
	}
	first := arena.Get(r.FirstKid)
	if first == nil || first.Node == nil {
		t.Fatalf("#%s has no text of its own on its line", row)
	}
	blockText = first.Y - r.Y
	for k := r.FirstKid; k != 0; k = arena.Get(k).NextSibling {
		obj := arena.Get(k)
		if obj.Node == nil || obj.Node.Data != "span" {
			continue
		}
		// An atomic box travels to its line position as a whole box; a text run's
		// box is the word the inline pass put under its span element.
		if obj.FirstKid == 0 {
			run = obj.Y - r.Y
			continue
		}
		word := arena.Get(obj.FirstKid)
		if word == nil || word.Node == nil {
			t.Fatalf("#%s has no word inside its span", row)
		}
		run = word.Y - r.Y
	}
	lineH = r.BorderH()
	return lineH, blockText, run
}

// qc:BR-273
func TestTextRunVerticalAlignClassesPlaceTheRunWhereChromiumPutsIt(t *testing.T) {
	arena := metricsSession(t, textRunAlignHTML, 800)
	for _, c := range []struct {
		id        string
		lineH     float32
		blockText float32
		run       float32
		why       string
	}{
		// The run's box is 40px tall with 11px of half-leading above its content
		// area, and it fits inside the 50px strut: the line and its baseline stay
		// the strut's, and each class moves only the run.
		{"g0", 50, 16, 16, "baseline"},
		{"g1", 50, 16, 11, "top: the box's own top edge rides the line's"},
		{"g2", 50, 16, 21, "bottom: its bottom edge rides the line's"},
		// The text-edge values measure against the parent font's content area,
		// which the strut's half-leading pushes 16px into the line: `text-top`
		// therefore starts the box at 16 and `text-bottom` ends it at 34.
		{"g3", 56, 16, 27, "text-top: the box hangs below the strut"},
		{"g4", 56, 22, 11, "text-bottom: the box hangs above it"},
		// A length shifts the run's own baseline, so its content area moves by
		// exactly that and the line does not move at all: 25+5 and 15-5 of demand
		// both stay inside what the strut already provides.
		{"g5", 50, 16, 11, "5px raises it"},
		{"g6", 50, 16, 21, "-5px lowers it"},
		// On a 16px strut the same 40px box is the tallest thing on the line, so
		// `top` stretches it from 18 to 40 while the baseline stays the strut's:
		// the block's own text does not move and the run's ink starts 11px down,
		// at its own half-leading rather than at the line's top edge.
		{"g7", 40, 0, 11, "top beside a 14/4 strut"},
	} {
		lineH, blockText, run := textRunOffsets(t, arena, c.id)
		if !almostEqual(run, c.run) {
			t.Errorf("#%s (%s): run starts %g below the line top, want %g (Chromium)",
				c.id, c.why, run, c.run)
		}
		if !almostEqual(lineH, c.lineH) {
			t.Errorf("#%s (%s): line box is %g tall, want %g (Chromium)", c.id, c.why, lineH, c.lineH)
		}
		if !almostEqual(blockText, c.blockText) {
			t.Errorf("#%s (%s): the block's own text starts %g below the line top, want %g (Chromium)",
				c.id, c.why, blockText, c.blockText)
		}
	}
}

// qc:BR-273
func TestTextRunVerticalAlignClassesGrowTheLineAroundABiggerFont(t *testing.T) {
	arena := metricsSession(t, textRunAlignHTML, 800)
	for _, c := range []struct {
		id        string
		lineH     float32
		blockText float32
		run       float32
		why       string
	}{
		// A 64px run whose own line height is 74px (59 above its baseline, 15
		// below) on a 50px strut. `top` pins its box to the line's top edge and
		// the line grows downward past the strut while the baseline stays the
		// strut's - the block's own text does not move.
		{"m0", 74, 16, 0, "top"},
		// `bottom` pins the same box to the line's bottom edge, so the 24px the
		// box adds appear ABOVE the baseline and every other box on the line, the
		// block's own text included, rides 24px lower.
		{"m1", 74, 40, 0, "bottom"},
		// The text-edge values measure against the parent font: 14px above the
		// baseline for the top edge, 4px below it for the bottom one, and either
		// way the 74px box reaches far past the strut, giving a 90px line.
		{"m2", 90, 16, 16, "text-top"},
		{"m3", 90, 56, 0, "text-bottom"},
		// A length moves the run's own baseline, so here it moves the line's too:
		// +5px lifts the box's baseline 5px above the strut's and the whole line
		// follows it up, and -15px pushes it back down past 59 to 44.
		{"m4", 84, 50, 0, "5px"},
		{"m5", 74, 30, 0, "-15px"},
		// The degenerate case, and the reason BR-272's fixture could not see any
		// of this: on a line whose 16px strut is smaller than the run, every
		// class answers the same 74px line with the run's box at 0, because the
		// run is what defines both edges. Only the block's own text tells the
		// three apart - `baseline` puts the line's baseline 59px down at the run's
		// own, `top` leaves it at the strut's 14px, and `5px` lifts the run's
		// baseline so the line's sits at 64px.
		{"d0", 74, 45, 0, "baseline, run dominant"},
		{"d1", 74, 0, 0, "top, run dominant"},
		{"d2", 74, 50, 0, "5px, run dominant"},
	} {
		lineH, blockText, run := textRunOffsets(t, arena, c.id)
		if !almostEqual(lineH, c.lineH) {
			t.Errorf("#%s (%s): line box is %g tall, want %g (Chromium)", c.id, c.why, lineH, c.lineH)
		}
		if !almostEqual(blockText, c.blockText) {
			t.Errorf("#%s (%s): the block's own text starts %g below the line top, want %g (Chromium)",
				c.id, c.why, blockText, c.blockText)
		}
		if !almostEqual(run, c.run) {
			t.Errorf("#%s (%s): run starts %g below the line top, want %g (Chromium)",
				c.id, c.why, run, c.run)
		}
	}
}

// qc:BR-273
func TestEdgeAlignedBoxesMoveTheBaselineOnlyWhenTheyStretchTheLine(t *testing.T) {
	arena := metricsSession(t, textRunAlignHTML, 800)
	for _, c := range []struct {
		id        string
		lineH     float32
		blockText float32
		run       float32
		why       string
	}{
		// The same three classes on a 74px inline-block, which is what BR-272
		// shipped: `bottom` measured against the line's bottom edge has to move
		// the baseline down by the stretch (24px), `top` must not move it at all,
		// and a baseline box puts its own bottom edge on the baseline and gains
		// the strut's 20px below it - the 94px line BR-270 registers.
		{"u0", 74, 40, 0, "inline-block bottom"},
		{"u1", 74, 16, 0, "inline-block top"},
		{"u2", 94, 60, 0, "inline-block baseline"},
		// In an 18px strut a 40px `bottom` box stretches the line by 22px, all of
		// it above the baseline, so the block's text starts at 22 rather than the
		// 0 the centring heuristic gave it.
		{"u3", 40, 22, 0, "40px bottom box on a 16px strut"},
		// Negative leading is the case that separates a text run's box from its
		// content area: a 64px run under `line-height: 10px` has 32px of leading
		// pulled OUT of its content area, so pinning the 10px box to an edge
		// starts the ink 32px above or 24px above the line, and the 18px line
		// never grows because the box itself is only 10px tall.
		{"n0", 18, 0, -32, "top with -32px of half-leading"},
		{"n1", 18, 0, -24, "bottom with -32px of half-leading"},
	} {
		lineH, blockText, run := textRunOffsets(t, arena, c.id)
		if !almostEqual(lineH, c.lineH) {
			t.Errorf("#%s (%s): line box is %g tall, want %g (Chromium)", c.id, c.why, lineH, c.lineH)
		}
		if !almostEqual(blockText, c.blockText) {
			t.Errorf("#%s (%s): the block's own text starts %g below the line top, want %g (Chromium)",
				c.id, c.why, blockText, c.blockText)
		}
		if !almostEqual(run, c.run) {
			t.Errorf("#%s (%s): box starts %g below the line top, want %g (Chromium)",
				c.id, c.why, run, c.run)
		}
	}
}
