package layout_test

import (
	"testing"
)

const lineHeightNormalLayoutHTML = `<html><head><style>
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.norm { line-height: normal; }
.b64 { font-size: 64px; }
.marker { width: 400px; height: 10px; }
</style></head><body>
<div id=a class=norm>x</div>
<div id=b class="norm b64">x</div>
<div id=c class=b64>x</div>
<div id=d class=norm><span class=b64>x</span></div>
<div id=e class="b64" style="line-height:1.5">x</div>
<div id=f class=norm>text more text more text more text more text more text more text more text more</div>
<div id=g class=marker></div>
</body></html>`

// qc:BR-271
func TestNormalLineHeightFollowsTheElementsOwnFont(t *testing.T) {
	arena := metricsSession(t, lineHeightNormalLayoutHTML, 800)
	for _, c := range []struct {
		id     string
		y, h   float32
		expect string
	}{
		{"a", 0, 18, "the 16px face height"},
		{"b", 18, 74, "the 64px face height, not the 16px body's"},
		{"c", 92, 74, "an undeclared normal at 64px, for #b to be measured against"},
		{"d", 166, 74, "the taller run deciding the line"},
		{"e", 240, 96, "1.5 x the element's own 64px"},
		{"f", 336, 18, "one wrapped line at the 16px normal height"},
		{"g", 354, 10, "the sibling #f ends at, which #b's height decides"},
	} {
		obj := findByID(arena, c.id)
		if obj == nil {
			t.Fatalf("#%s has no layout object", c.id)
		}
		if !almostEqual(obj.Y, c.y) || !almostEqual(obj.BorderH(), c.h) {
			t.Errorf("#%s Y=%g BorderH=%g, want Y=%g H=%g - %s (Chromium)", c.id, obj.Y, obj.BorderH(), c.y, c.h, c.expect)
		}
	}
}

// qc:BR-271
func TestNormalLineHeightDoesNotReachIntoAnExplicitLength(t *testing.T) {
	const html = `<html><head><style>
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.fixed { line-height: 50px; }
.b64 { font-size: 64px; }
.marker { width: 400px; height: 10px; }
</style></head><body>
<div id=a class="fixed b64">x</div>
<div id=b class=b64>x</div>
<div id=c class=marker></div>
</body></html>`
	arena := metricsSession(t, html, 800)
	fixed, normal, marker := findByID(arena, "a"), findByID(arena, "b"), findByID(arena, "c")
	if fixed == nil || normal == nil || marker == nil {
		t.Fatalf("missing #a, #b or #c")
	}
	if !almostEqual(fixed.BorderH(), 50) {
		t.Errorf("#a BorderH=%g, want the declared 50px (Chromium)", fixed.BorderH())
	}
	if !almostEqual(normal.BorderH(), 74) {
		t.Errorf("#b BorderH=%g, want the 64px face height of 74 (Chromium)", normal.BorderH())
	}
	if !almostEqual(marker.Y, fixed.Y+fixed.BorderH()+normal.BorderH()) {
		t.Errorf("#c Y=%g, want it below a 50px block and a 74px one at %g", marker.Y, fixed.Y+fixed.BorderH()+normal.BorderH())
	}
}

// qc:BR-271
func TestStrutUnderANormalLineHeightStillHoldsAnAtomicRow(t *testing.T) {
	const html = `<html><head><style>
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.norm { line-height: normal; }
.chip { display: inline-block; width: 40px; height: 24px; }
.b64 { font-size: 64px; }
.marker { width: 400px; height: 10px; }
</style></head><body>
<div id=a class=norm><span class=chip></span></div>
<div id=b class="norm b64"><span class=chip></span></div>
<div id=c style="font-size:64px" class=norm><span class=chip></span></div>
<div id=d class=marker></div>
</body></html>`
	arena := metricsSession(t, html, 800)
	for _, c := range []struct {
		id   string
		y, h float32
		why  string
	}{
		{"a", 0, 28, "the 26px margin box over the 16px strut: 24 above the baseline, the strut's 4 below"},
		{"b", 28, 74, "the 64px face reaches 59 above its baseline and 15 below it, so the strut is what holds the line"},
		{"c", 102, 74, "the same row reached through an inline font-size rather than a class"},
		{"d", 176, 10, "the marker under all three rows, which compounds any row that came out short"},
	} {
		obj := findByID(arena, c.id)
		if obj == nil {
			t.Fatalf("#%s has no layout object", c.id)
		}
		if !almostEqual(obj.Y, c.y) || !almostEqual(obj.BorderH(), c.h) {
			t.Errorf("#%s Y=%g BorderH=%g, want Y=%g H=%g - %s (Chromium)", c.id, obj.Y, obj.BorderH(), c.y, c.h, c.why)
		}
	}
	if chip := arena.Get(findByID(arena, "b").FirstKid); !almostEqual(chip.Y-findByID(arena, "b").Y, 35) {
		t.Errorf("#b's chip sits %g below the line top, want 35 (Chromium)", chip.Y-findByID(arena, "b").Y)
	}
}
