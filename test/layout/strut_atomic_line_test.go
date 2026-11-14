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

func (chromiumLineMetrics) XHeightFixed(sizePx int32, slot frame.FontSlot) int32 {
	return sizePx * 67 / 2
}

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

func marginTopIn(row, kid *layout.Object) float32 {
	return kid.Y - kid.MarginTop - row.Y
}

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
		{"a", want{0, 40}},
		{"b", want{40, 40}},
		{"c", want{96, 18}},
		{"d", want{130, 18}},
		{"e", want{148, 36}},
		{"f", want{184, 36}},
		{"g", want{220, 28}},
		{"h", want{248, 45}},
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
		{"a", []float32{0, 0}},
		{"d", []float32{10}},
		{"e", []float32{0}},
		{"f", []float32{0}},
		{"g", []float32{0}},
		{"h", []float32{17, 0}},
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
