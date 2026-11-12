package layout_test

import (
	"testing"
)

// The geometry GOOSIE-LHENORMAL-001 was opened for. `line-height: normal`
// frozen into a length hands the ancestor's number down the tree, so a 64px
// block laid out at the 16px body's height and its own glyphs were drawn
// outside the box, over whatever block came next.
//
// Every expected y and height is Chromium's, measured with Playwright on
// 2026-11-12 against this markup at an 800x600 viewport, and the fake metrics
// the session installs are the face Chromium reports at these two sizes
// (16px: 14/4/18, 64px: 59/15/74), so the line heights below are the rule
// rather than a host's font.
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
		// The control: nothing declared but the body's own normal.
		{"a", 0, 18, "the 16px face height"},
		// `normal` from the class, 64px from the class, and the same pair split
		// across a class and an inline style all have to reach the same height.
		// Frozen as 1.2 x 16px this was 19.
		{"b", 18, 74, "the 64px face height, not the 16px body's"},
		{"c", 92, 74, "an undeclared normal at 64px, for #b to be measured against"},
		// A 64px run on a 16px block that declares normal: the run's own height
		// grows the line, so the parent's frozen number cannot hold it down.
		{"d", 166, 74, "the taller run deciding the line"},
		// A number is a share of this element's font size and keeps working:
		// 1.5 x 64px = 96, measured on both engines.
		{"e", 240, 96, "1.5 x the element's own 64px"},
		{"f", 336, 18, "one wrapped line at the 16px normal height"},
		// The block the bug used to paint over. It sits at Chromium's y because
		// every block above it took its real height.
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
	// The change is about the keyword, so a declared length has to keep
	// behaving like a length: inherited resolved, and taller than the face.
	// Without this, "everything got taller" and "normal stopped being a number"
	// are the same test result.
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
	// 50px is a length the element asked for and it survives the block's own
	// 64px font; the sibling at the same size takes the face's 74.
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
	// BR-270's rule reads the strut's two halves, so the keyword has to reach
	// the face there too. Frozen as 1.2 x 16px the height was 19.2, which splits
	// into 14.6 above the baseline and 4.6 below it: #a came out 28.6 where
	// Chromium reports 28. The 64px rows are the controls that keep the split
	// honest on the other side of the baseline.
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
	// #b's chip hangs 35px below the line top: the line's baseline is 59 down
	// and the box puts its bottom margin edge there.
	if chip := arena.Get(findByID(arena, "b").FirstKid); !almostEqual(chip.Y-findByID(arena, "b").Y, 35) {
		t.Errorf("#b's chip sits %g below the line top, want 35 (Chromium)", chip.Y-findByID(arena, "b").Y)
	}
}
