package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

const middleScriptHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
  body { margin: 8px; font: 16px sans-serif; }
  /* 600px because the fake face advances half the em per character and a 64px
     word in a 400px row would wrap, which is a harness artifact rather than a
     line box. */
  .row { width: 600px; }
  .st { line-height: 50px; }
  .f12 { font-size: 12px; } .f24 { font-size: 24px; }
  .f32 { font-size: 32px; } .f64 { font-size: 64px; }
  .mid { vertical-align: middle; }
  .sup { vertical-align: super; } .sub { vertical-align: sub; }
  .c4 { display: inline-block; width: 30px; height: 4px; }
  .c20 { display: inline-block; width: 30px; height: 20px; }
  .c40 { display: inline-block; width: 30px; height: 40px; }
  .run40 { line-height: 40px; }
  .big74 { font-size: 64px; line-height: 74px; }
  .r24 { font-size: 24px; line-height: 28px; }
</style></head><body>
<div class=row id=m16a>words <span class="c4 mid"></span> more</div>
<div class=row id=m16b>words <span class="c20 mid"></span> more</div>
<div class=row id=m16c>words <span class="c40 mid"></span> more</div>
<div class="row st" id=m50a>words <span class="c4 mid"></span> more</div>
<div class="row st" id=m50b>words <span class="c20 mid"></span> more</div>
<div class="row st" id=m50c>words <span class="c40 mid"></span> more</div>
<div class="row f24" id=m24a>words <span class="c4 mid"></span> more</div>
<div class="row f32" id=m32a>words <span class="c4 mid"></span> more</div>
<div class="row f64" id=m64a>words <span class="c4 mid"></span> more</div>
<div class="row f64" id=m64c>words <span class="c40 mid"></span> more</div>
<div class="row st" id=t40>words <span class="run40 mid">tail</span> more</div>
<div class="row st" id=t74>words <span class="big74 mid">Big</span> more</div>
<div class=row id=aS4>words <span class="c4 sup"></span> more</div>
<div class=row id=aU4>words <span class="c4 sub"></span> more</div>
<div class=row id=aS40>words <span class="c40 sup"></span> more</div>
<div class=row id=aU40>words <span class="c40 sub"></span> more</div>
<div class="row f64" id=aS4b>words <span class="c4 sup"></span> more</div>
<div class="row f64" id=aU4b>words <span class="c4 sub"></span> more</div>
<div class=row id=rS64>words <span class="big74 sup">Big</span> more</div>
<div class=row id=rU64>words <span class="big74 sub">Big</span> more</div>
<div class="row f64" id=rS24>words <span class="r24 sup">tail</span> more</div>
<div class="row f64" id=rU24>words <span class="r24 sub">tail</span> more</div>
<div class="row f12" id=x12s>aa <span class=sup>bb</span> cc</div>
<div class="row f12" id=x12u>aa <span class=sub>bb</span> cc</div>
<div class=row id=x16s>aa <span class=sup>bb</span> cc</div>
<div class=row id=x16u>aa <span class=sub>bb</span> cc</div>
<div class="row f24" id=x24s>aa <span class=sup>bb</span> cc</div>
<div class="row f24" id=x24u>aa <span class=sub>bb</span> cc</div>
<div class="row f32" id=x32s>aa <span class=sup>bb</span> cc</div>
<div class="row f32" id=x32u>aa <span class=sub>bb</span> cc</div>
<div class="row f64" id=x64s>aa <span class=sup>bb</span> cc</div>
<div class="row f64" id=x64u>aa <span class=sub>bb</span> cc</div>
</body></html>`

type middleScriptCase struct {
	id    string
	lineH float32
	top   float32
	text  float32
	why   string
}

func checkMiddleScript(t *testing.T, cases []middleScriptCase) {
	t.Helper()
	arena := controlMetricsSession(t, middleScriptHTML, 800)
	for _, c := range cases {
		lineH, top, text := alignOffsets(t, arena, c.id)
		if !almostEqual(lineH, c.lineH) || !almostEqual(top, c.top) ||
			(c.text >= 0 && !almostEqual(text, c.text)) {
			t.Errorf("#%s = line %.4f, offset %.4f, text %.4f, want %.4f, %.4f, %.4f (%s)",
				c.id, lineH, top, text, c.lineH, c.top, c.text, c.why)
		}
	}
}

func alignOffsets(t *testing.T, arena *layout.Arena, row string) (lineH, top, text float32) {
	t.Helper()
	r := findByID(arena, row)
	if r == nil {
		t.Fatalf("#%s has no layout object", row)
	}
	text = -1
	for k := r.FirstKid; k != 0; k = arena.Get(k).NextSibling {
		obj := arena.Get(k)
		if obj.Node == nil || !obj.Node.Element() {
			if text < 0 {
				text = obj.Y - r.Y
			}
			continue
		}
		box := obj
		for box.W == 0 && box.H == 0 {
			kid := box.FirstKid
			for kid != 0 && arena.Get(kid).W <= 0 && arena.Get(kid).H <= 0 {
				kid = arena.Get(kid).NextSibling
			}
			if kid == 0 {
				break
			}
			box = arena.Get(kid)
		}
		return r.BorderH(), box.Y - r.Y, text
	}
	t.Fatalf("#%s has no element on its line", row)
	return 0, 0, 0
}

// qc:BR-275
func TestMiddleCentersAnAtomicBoxOnTheParentsXHeight(t *testing.T) {
	checkMiddleScript(t, []middleScriptCase{
		{"m16a", 18, 7.8125, 0, "base 14 - xh/2 4.1875 - extent/2 2 on an 18px line that the box does not reach past; the bottom-edge fallback answered 14"},
		{"m16b", 20, 0, 0.1875, "extent 20 asks 14.1875 above the baseline, so the line grows by 0.1875 and the text rides it"},
		{"m16c", 40, 0, 10.1875, "extent 40 asks 24.1875 above and 15.8125 below; the text of the block starts 10.1875 down the line"},
		{"m50a", 50, 23.8125, 16, "the 50px strut keeps its baseline at 30: 30 - 4.1875 - 2"},
		{"m50b", 50, 15.8125, 16, "30 - 4.1875 - 10"},
		{"m50c", 50, 5.8125, 16, "30 - 4.1875 - 20, still inside the strut on both sides"},
		{"m24a", 28, 13.71875, 0, "the parent is the 24px face: base 22 - xh/2 6.28125 - 2"},
		{"m32a", 37, 19.625, 0, "base 30 - xh/2 8.375 - extent/2 2; the 32px face's x-height is 16.75, exactly twice the 16px one, so this row and m16a answer with the same half"},
		{"m64a", 74, 40.25, 0, "base 59 - xh/2 16.75 - extent/2 2; the reference answers 40.265625. It centres on the face's own x-height - 0.523em, bounded to [0.52294922, 0.52304688] by a sweep of 16 sizes - and rounds the resulting position to 1/64 px, which reproduces all 16 of them, while a metric that has to fit the 1/64 grid can only answer 33.5px here. 15 sizes agree either way; 64px is the one where half the difference is already a whole 1/64, so this is metric currency (GOOSIE-FONTMETRIC-001), not a second rule"},
		{"m64c", 74, 22.25, 0, "base 59 - 16.75 - 20, where the reference answers 22.265625 for the same reason"},
	})
}

// qc:BR-275
func TestMiddleCentersARunOfTextTheSameWay(t *testing.T) {
	checkMiddleScript(t, []middleScriptCase{
		{"t40", 50, 16.8125, 16, "box 40 tall centred on 30 - 4.1875 puts its top at 5.8125, and the content area starts 11px of half-leading inside it"},
		{"t74", 74, 0, 27.1875, "the 64px run is centred on the 16px parent's x-height, so it asks 41.1875 above the baseline and the line grows to 74 while the block's own text drops to 27.1875"},
	})
}

// qc:BR-275
func TestScriptShiftMovesTheBaselineOfEveryKindOfBox(t *testing.T) {
	checkMiddleScript(t, []middleScriptCase{
		{"aS4", 18, 3.671875, 0, "10 where its bottom edge sits on the baseline, minus the 16px shift 6.328125; the line does not grow because 4+6.328125 stays under the strut's 14"},
		{"aU4", 18.1875, 14.1875, 0, "10 plus 4.1875, and the lowered box hangs below the strut so the line grows by exactly the shift"},
		{"aS40", 50.328125, 0, 32.328125, "extent 40 raised by 6.328125 asks 46.328125 above the baseline, so the text of the block starts 32.328125 down"},
		{"aU40", 40, 0, 21.8125, "extent 40 lowered: above 35.8125, below 4.1875"},
		{"aS4b", 74, 32.671875, 0, "in the 64px block the shift is 22.328125: 59 - 4 - 22.328125"},
		{"aU4b", 74, 68.796875, 0, "59 - 4 + 13.796875, still inside a line the strut already fills"},
		{"rS64", 74, 0, 51.328125, "a 64px run in a 16px block is raised by the 16px number, so its box asks 59+6.328125 above the baseline and the text of the block rides 51.328125 down the line"},
		{"rU64", 74, 0, 40.8125, "lowered by 4.1875: above 54.8125, below 19.1875"},
		{"rS24", 74, 14.671875, 0, "a 24px run in a 64px block: 59 - 22 - 22.328125, the 64px shift rather than the 24px one"},
		{"rU24", 78.796875, 50.796875, 0, "and lowered by 13.796875, which the strut does not cover, so the line grows past 74"},
	})
}

// qc:BR-275
func TestScriptShiftScalesWithTheParentSize(t *testing.T) {
	checkMiddleScript(t, []middleScriptCase{
		{"x12s", 19, 0, 5, "12/3 + 1 = 5 exactly; the line is the 14px face plus the shift"},
		{"x12u", 17.390625, 3.390625, 0, "12/5 = 2.4 truncates to 2.390625, +1"},
		{"x16s", 24.328125, 0, 6.328125, "16/3 = 5.3333 truncates to 5.328125, +1"},
		{"x16u", 22.1875, 4.1875, 0, "16/5 = 3.2 truncates to 3.1875, +1"},
		{"x24s", 37, 0, 9, "24/3 is exact: 8+1"},
		{"x24u", 33.796875, 5.796875, 0, "24/5 = 4.8 truncates to 4.796875, +1"},
		{"x32s", 48.65625, 0, 11.65625, "32/3 truncates to 10.65625, +1"},
		{"x32u", 44.390625, 7.390625, 0, "32/5 = 6.4 truncates to 6.390625, +1"},
		{"x64s", 96.328125, 0, 22.328125, "64/3 truncates to 21.328125, +1"},
		{"x64u", 87.796875, 13.796875, 0, "64/5 = 12.8 truncates to 12.796875, +1"},
	})
}
