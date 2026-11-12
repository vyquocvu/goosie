package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

// verticalAlignHTML is GOOSIE-INLINEBASELINE-001's second half: the same 4px chip
// on the same 16px strut, once per `vertical-align` class, and each row written
// twice - alone on its line (which the block pass's row path resolves) and after
// a word of text (which the inline pass resolves). Chromium gives the two the
// same numbers, measured on 2026-11-12 against this markup at an 800x600
// viewport, and that is what makes the shared model testable rather than one
// path's accident.
//
// The chip is shorter than the strut on both sides on purpose: a box that fills
// its line cannot show which half of the line it was aligned against, which is
// exactly the question `text-top` and `-5px` answer.
//
// The doctype is part of the oracle. Measured on Chromium 2026-11-12, the same
// markup without it answers 4px for a line whose only content is the 4px chip -
// quirks mode drops the strut - where with it the line is the 18px the strut
// makes. Standards mode is the mode every number below is stated in.
const verticalAlignHTML = `<!DOCTYPE html><html><head><style>
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.row { width: 380px; }
.chip { display: inline-block; width: 40px; height: 4px; }
.tall { display: inline-block; width: 90px; height: 26px; margin: 0 10px 10px 0; }
.stretch { width: 380px; line-height: 50px; }
.big { font-size: 64px; }
</style></head><body>
<div class=row id=b0><span class=chip style="vertical-align:baseline"></span></div>
<div class=row id=b1>words<span class=chip style="vertical-align:baseline"></span></div>
<div class=row id=t0><span class=chip style="vertical-align:top"></span></div>
<div class=row id=t1>words<span class=chip style="vertical-align:top"></span></div>
<div class=row id=m0><span class=chip style="vertical-align:bottom"></span></div>
<div class=row id=m1>words<span class=chip style="vertical-align:bottom"></span></div>
<div class=row id=tt0><span class=chip style="vertical-align:text-top"></span></div>
<div class=row id=tt1>words<span class=chip style="vertical-align:text-top"></span></div>
<div class=row id=tb0><span class=chip style="vertical-align:text-bottom"></span></div>
<div class=row id=tb1>words<span class=chip style="vertical-align:text-bottom"></span></div>
<div class=row id=l0><span class=chip style="vertical-align:5px"></span></div>
<div class=row id=l1>words<span class=chip style="vertical-align:5px"></span></div>
<div class=row id=n0><span class=chip style="vertical-align:-5px"></span></div>
<div class=row id=n1>words<span class=chip style="vertical-align:-5px"></span></div>
<div class=row id=x0>words <span class=tall></span> more</div>
<div class=stretch id=s0>Fifty of <span class=big id=big>Big</span> here too</div>
</body></html>`

// chipIn is how far one row's chip sits below the top of its line box, which is
// the number `vertical-align` decides and the number the reference reports as the
// chip's offset within its row.
func chipIn(t *testing.T, arena *layout.Arena, row string) (float32, float32) {
	t.Helper()
	r := findByID(arena, row)
	if r == nil {
		t.Fatalf("#%s has no layout object", row)
	}
	for k := r.FirstKid; k != 0; k = arena.Get(k).NextSibling {
		obj := arena.Get(k)
		if obj.Node != nil && obj.Node.Data == "span" {
			return obj.Y - r.Y, r.BorderH()
		}
	}
	t.Fatalf("#%s has no span on its line", row)
	return 0, 0
}

// qc:BR-272
func TestVerticalAlignClassesPlaceTheChipWhereChromiumPutsIt(t *testing.T) {
	arena := metricsSession(t, verticalAlignHTML, 800)
	for _, c := range []struct {
		id   string
		want float32
		why  string
	}{
		// An empty inline-block puts its bottom margin edge on the baseline, so
		// the strut's ascent less its own height is all that stands above it.
		{"b0", 10, "baseline"}, {"b1", 10, "baseline beside text"},
		{"t0", 0, "top"}, {"t1", 0, "top beside text"},
		// `bottom` rides the line box's own bottom edge, which the strut reaches.
		{"m0", 14, "bottom"}, {"m1", 14, "bottom beside text"},
		// The two text-edge values measure against the parent's em box, not the
		// line's: here that happens to put them on the same edges as `top` and
		// `bottom`, and on a taller line it would not.
		{"tt0", 0, "text-top"}, {"tt1", 0, "text-top beside text"},
		{"tb0", 14, "text-bottom"}, {"tb1", 14, "text-bottom beside text"},
		// A length shifts the box's own baseline, so its top moves by the same
		// amount and nothing else on the line has to follow it.
		{"l0", 5, "5px"}, {"l1", 5, "5px beside text"},
		{"n0", 15, "-5px"}, {"n1", 15, "-5px beside text"},
	} {
		got, h := chipIn(t, arena, c.id)
		if !almostEqual(got, c.want) {
			t.Errorf("#%s (%s): chip sits %g below the line top, want %g (Chromium); line %g tall",
				c.id, c.why, got, c.want, h)
		}
	}
}

// qc:BR-272
func TestVerticalAlignClassesGiveTheLineBoxItsOwnHeight(t *testing.T) {
	arena := metricsSession(t, verticalAlignHTML, 800)
	for _, c := range []struct {
		id   string
		want float32
		why  string
	}{
		// Every class that keeps the box inside the strut answers the strut's own
		// 18px: the line neither grows for a chip it already contains nor shrinks
		// to the chip.
		{"b0", 18, "baseline"}, {"t0", 18, "top"}, {"m0", 18, "bottom"},
		{"tt0", 18, "text-top"}, {"tb0", 18, "text-bottom"}, {"l0", 18, "5px"},
		// A lowered box hangs past the strut's descent and the line has to contain
		// it: 4px of box, 5px down, 4px of strut descent is a 19px line.
		{"n0", 19, "-5px"},
		// The seam the defect was filed on: a text run and a 36px margin box on one
		// line grow it above the baseline by the box and below by the strut.
		{"x0", 40, "text beside a taller inline-block"},
	} {
		row := findByID(arena, c.id)
		if row == nil {
			t.Fatalf("#%s has no layout object", c.id)
		}
		if !almostEqual(row.BorderH(), c.want) {
			t.Errorf("#%s (%s): line box is %g tall, want %g (Chromium)", c.id, c.why, row.BorderH(), c.want)
		}
	}
}

// qc:BR-272
func TestDeclaredLineHeightShorterThanTheRunItCarriesOverflowsIt(t *testing.T) {
	arena := metricsSession(t, verticalAlignHTML, 800)
	stretch, big := findByID(arena, "s0"), findByID(arena, "big")
	if stretch == nil || big == nil {
		t.Fatalf("missing #s0 or #big")
	}
	// A 64px run under an inherited 50px line height asks 47px above the baseline
	// and 3 below, while the 16px strut the block itself builds asks 30 and 20.
	// Both sides answer separately, so the line is 67 - and the run's own 74px
	// content area starts 12px above it, overflowing rather than being squeezed.
	if !almostEqual(stretch.BorderH(), 67) {
		t.Errorf("the 50px line carrying a 64px run is %g tall, want 67 (Chromium)", stretch.BorderH())
	}
	// The run's content area is the word box the arena places; the inline element
	// above it only aggregates its fragments at paint time.
	word := arena.Get(big.FirstKid)
	if !almostEqual(word.H, 74) {
		t.Errorf("the run's content area is %g tall, want its own 74 (Chromium)", word.H)
	}
	if !almostEqual(word.Y-stretch.Y, -12) {
		t.Errorf("the run's content area starts %g above the line top, want -12 (Chromium)", word.Y-stretch.Y)
	}
}
