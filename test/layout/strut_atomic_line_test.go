package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// chromiumLineMetrics is the face the oracle below is scored against: the
// ascent, descent and `line-height: normal` height Chromium reports for the
// document's font, measured with Playwright's TextMetrics on 2026-11-12 (16px
// sans-serif: 14 / 4 / 18, 64px: 59 / 15 / 74). A layout oracle cannot read
// them from the host - test/layout has no raster font - so it supplies them,
// which is what makes the expected line heights below arithmetic rather than
// guesses: every one is strutAbove, strutBelow and the boxes' own demands
// combined, exactly as Chromium combines them.
type chromiumLineMetrics struct{}

func (chromiumLineMetrics) GlyphAdvance(sizePx int32, r rune, slot frame.FontSlot) int32 {
	return sizePx / 2
}

func (chromiumLineMetrics) GlyphAdvanceFixed(sizePx int32, r rune, slot frame.FontSlot) int32 {
	return sizePx * 32
}

func (chromiumLineMetrics) LineMetrics(sizePx int32, slot frame.FontSlot) (int32, int32, int32) {
	if sizePx == 64 {
		return 59, 15, 74
	}
	return 14, 4, 18
}

// XHeightFixed reports the same face's x-height, which Chromium measures at
// 0.5234375em (8.375px at 16px, 33.5px at 64px - the OS/2 value, not the ink
// box, which reads 8.3672px). `vertical-align: middle` halves it, so the
// fraction is what the oracle expects.
func (chromiumLineMetrics) XHeightFixed(sizePx int32, slot frame.FontSlot) int32 {
	return sizePx * 67 / 2
}

// metricsSession lays a document out the way the line-box oracles need it: the
// page's own <style> blocks reach the cascade (`session` passes no sheets at
// all) and the arena gets a Metrics implementation, because a line box is built
// from the face's ascent, descent and normal height and a test host has no
// raster font to read them from.
func metricsSession(t *testing.T, html string, viewportW float32) *layout.Arena {
	t.Helper()
	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n.Element() && n.Data == "style" {
			sheets = append(sheets, css.ParseForViewport(n.TextContent(), viewportW))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	styles := style.Resolve(doc, sheets, nil)
	arena := layout.Build(doc, styles, nil)
	arena.Metrics = chromiumLineMetrics{}
	layout.Block(arena, layout.ObjectID(1), viewportW, 0)
	return arena
}

// marginTopIn is how far a child's margin box sits below the top of the row it
// belongs to - what a line box actually decides, and what the Chromium oracle
// records as each kid's offset.
func marginTopIn(row, kid *layout.Object) float32 {
	return kid.Y - kid.MarginTop - row.Y
}

// atomicStrutHTML is GOOSIE-INLINESTRUT-001's shape plus the sibling shapes that
// decide the rule. Every expected number below is Chromium's, measured on
// 2026-11-12 with Playwright against this markup at an 800x600 viewport.
//
// CSS 2.1 10.6.1 makes a line box contain the containing block's strut even when
// the line carries no text at all, and the strut reaches 4px below the baseline.
// Before this rule a row of inline-blocks was only the tallest margin box, so
// each such row came out 36px where Chromium says 40, and everything under a
// stack of them drifted up by 4px per row.
const atomicStrutHTML = `<html><head><style>
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.row { width: 400px; }
.ib { display: inline-block; width: 100px; height: 24px; border: 1px solid #333; margin: 0 10px 10px 0; }
.chip { display: inline-block; width: 120px; border: 1px solid #000; padding: 4px; }
.sm { display: inline-block; width: 100px; height: 4px; margin: 0; }
</style></head><body>
<div class=row id=a><span class=ib></span><span class=ib></span></div>
<div class=row id=b><span class=ib></span><span class=ib></span></div>
<p id=c>after</p>
<div class=row id=d><span class=sm></span></div>
<div class=row id=e><span class=ib style="vertical-align:top"></span></div>
<div class=row id=f><span class=ib style="vertical-align:bottom"></span></div>
<div class=row id=g><span class=chip>hello</span></div>
<div class=row id=h><span class=chip>hello</span><span class=ib></span></div>
<div class=row id=k><img id=img width=40 height=20 src="data:image/svg+xml,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20width%3D%2240%22%20height%3D%2220%22%3E%3C%2Fsvg%3E"></div>
<div class=row id=i><input id=in type=text style="width:120px;height:20px"></div>
</body></html>`

// qc:BR-270
func TestAtomicOnlyLinePaysTheStrutBelowTheBaseline(t *testing.T) {
	arena := metricsSession(t, atomicStrutHTML, 800)
	type want struct{ y, h float32 }
	for _, c := range []struct {
		id string
		want
	}{
		// Two 26px chips with 10px bottom margins: 36 above the baseline, the
		// strut's 4 below it. The second row starts where the first ended, and
		// the paragraph after them clears its own 16px margin off 80.
		{"a", want{0, 40}},
		{"b", want{40, 40}},
		{"c", want{96, 18}},
		// A box shorter than the strut keeps the strut's height and hangs under
		// its baseline: the 4px box sits 10px down a line that stays 18px.
		{"d", want{130, 18}},
		// `top` and `bottom` measure against the line's own edges, so the strut
		// adds nothing below: both rows are exactly the 36px margin box.
		{"e", want{148, 36}},
		{"f", want{184, 36}},
		// A chip carrying its own text line brings a baseline of its own, and
		// its descent covers the strut's: 28px, not 32.
		{"g", want{220, 28}},
		// Beside a taller baseline chip that still has descent below its
		// baseline, the line grows on the far side of the baseline only.
		{"h", want{248, 45}},
		// The contrast case: an image has no text of its own, so its bottom
		// margin edge is its baseline and the strut's 4px lands below it.
		{"k", want{293, 24}},
	} {
		obj := findByID(arena, c.id)
		if obj == nil {
			t.Fatalf("#%s has no layout object", c.id)
		}
		if !almostEqual(obj.Y, c.y) || !almostEqual(obj.BorderH(), c.h) {
			t.Errorf("#%s Y=%g BorderH=%g, want Y=%g H=%g (Chromium)", c.id, obj.Y, obj.BorderH(), c.y, c.h)
		}
	}
}

// A control's text is not a DOM child, so the arena cannot see it by walking -
// but the control still aligns on that text's baseline, so its own descent covers
// the strut's and the line stays exactly the control's box. Chromium measured a
// 26px input on a 26px line on 2026-11-12 where a 20px image on the same strut
// took 24; the box heights the two engines give an input differ (that is a
// control-sizing gap, not this one), so this asserts the rule, not the number.
// qc:BR-270
func TestLoneControlDoesNotGainTheStrutAgain(t *testing.T) {
	arena := metricsSession(t, atomicStrutHTML, 800)
	row, in := findByID(arena, "i"), findByID(arena, "in")
	if row == nil || in == nil {
		t.Fatalf("missing #i or #in")
	}
	if !almostEqual(row.BorderH(), in.BorderH()) {
		t.Errorf("the input's line is %g tall for a %g tall control, want the line to be the control's own height (Chromium)", row.BorderH(), in.BorderH())
	}
	if !almostEqual(marginTopIn(row, in), 0) {
		t.Errorf("the input sits %g below the line top, want 0 (Chromium)", marginTopIn(row, in))
	}
}

// qc:BR-270
func TestAtomicOnlyLinePlacesEachBoxAgainstTheBaseline(t *testing.T) {
	arena := metricsSession(t, atomicStrutHTML, 800)
	kidsOf := func(id string) []*layout.Object {
		row := findByID(arena, id)
		if row == nil {
			t.Fatalf("#%s has no layout object", id)
		}
		var kids []*layout.Object
		for k := row.FirstKid; k != 0; k = arena.Get(k).NextSibling {
			kids = append(kids, arena.Get(k))
		}
		return kids
	}
	for _, c := range []struct {
		id   string
		want []float32
	}{
		{"a", []float32{0, 0}},  // both margin boxes ride the line's top edge
		{"d", []float32{10}},    // strut's ascent minus the 4px box
		{"e", []float32{0}},     // vertical-align: top
		{"f", []float32{0}},     // vertical-align: bottom, and the line is its own height
		{"g", []float32{0}},     // the chip spans the whole line
		{"h", []float32{17, 0}}, // the text chip drops 9px below the baseline
	} {
		kids := kidsOf(c.id)
		if len(kids) != len(c.want) {
			t.Fatalf("#%s has %d boxes on its line, want %d", c.id, len(kids), len(c.want))
		}
		for i, kid := range kids {
			if got := marginTopIn(findByID(arena, c.id), kid); !almostEqual(got, c.want[i]) {
				t.Errorf("#%s kid %d sits %g below the line top, want %g (Chromium)", c.id, i, got, c.want[i])
			}
		}
	}
}
