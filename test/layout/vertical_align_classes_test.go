package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

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
		{"b0", 10, "baseline"}, {"b1", 10, "baseline beside text"},
		{"t0", 0, "top"}, {"t1", 0, "top beside text"},
		{"m0", 14, "bottom"}, {"m1", 14, "bottom beside text"},
		{"tt0", 0, "text-top"}, {"tt1", 0, "text-top beside text"},
		{"tb0", 14, "text-bottom"}, {"tb1", 14, "text-bottom beside text"},
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
		{"b0", 18, "baseline"}, {"t0", 18, "top"}, {"m0", 18, "bottom"},
		{"tt0", 18, "text-top"}, {"tb0", 18, "text-bottom"}, {"l0", 18, "5px"},
		{"n0", 19, "-5px"},
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
	if !almostEqual(stretch.BorderH(), 67) {
		t.Errorf("the 50px line carrying a 64px run is %g tall, want 67 (Chromium)", stretch.BorderH())
	}
	word := arena.Get(big.FirstKid)
	if !almostEqual(word.H, 74) {
		t.Errorf("the run's content area is %g tall, want its own 74 (Chromium)", word.H)
	}
	if !almostEqual(word.Y-stretch.Y, -12) {
		t.Errorf("the run's content area starts %g above the line top, want -12 (Chromium)", word.Y-stretch.Y)
	}
}
