package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

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
		{"g0", 50, 16, 16, "baseline"},
		{"g1", 50, 16, 11, "top: the box's own top edge rides the line's"},
		{"g2", 50, 16, 21, "bottom: its bottom edge rides the line's"},
		{"g3", 56, 16, 27, "text-top: the box hangs below the strut"},
		{"g4", 56, 22, 11, "text-bottom: the box hangs above it"},
		{"g5", 50, 16, 11, "5px raises it"},
		{"g6", 50, 16, 21, "-5px lowers it"},
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
		{"m0", 74, 16, 0, "top"},
		{"m1", 74, 40, 0, "bottom"},
		{"m2", 90, 16, 16, "text-top"},
		{"m3", 90, 56, 0, "text-bottom"},
		{"m4", 84, 50, 0, "5px"},
		{"m5", 74, 30, 0, "-15px"},
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
		{"u0", 74, 40, 0, "inline-block bottom"},
		{"u1", 74, 16, 0, "inline-block top"},
		{"u2", 94, 60, 0, "inline-block baseline"},
		{"u3", 40, 22, 0, "40px bottom box on a 16px strut"},
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
