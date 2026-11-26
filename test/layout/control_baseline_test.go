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

type controlLineMetrics struct{}

func (controlLineMetrics) GlyphAdvance(sizePx int32, r rune, slot frame.FontSlot) int32 {
	return controlLineMetrics{}.GlyphAdvanceFixed(sizePx, r, slot) / 64
}

func (controlLineMetrics) GlyphAdvanceFixed(sizePx int32, r rune, slot frame.FontSlot) int32 {
	if slot.Family == frame.FontCourier {
		return (sizePx*384 + 5) / 10
	}
	if slot.Family == frame.FontTimes && r == 'W' {
		return sizePx * 32
	}
	switch r {
	case 'G':
		switch sizePx {
		case 13:
			return 647
		case 16:
			return 796
		case 20:
			return 996
		case 24:
			return 1194
		case 32:
			return 1592
		}
	case 'o':
		switch sizePx {
		case 13:
			return 463
		case 16:
			return 570
		case 20:
			return 712
		case 24:
			return 855
		case 32:
			return 1140
		}
	case 'x':
		return sizePx * 32
	case '0':
		switch sizePx {
		case 13:
			return 437
		case 16:
			return 512
		case 20:
			return 640
		case 24:
			return 768
		case 32:
			return 1024
		}
	case 'W':
		switch sizePx {
		case 13:
			return 749
		case 16:
			return 896
		case 20:
			return 1152
		case 24:
			return 1408
		case 32:
			return 1856
		}
	}
	base := int32(463)
	switch r {
	case 'S', 'A':
		base = 555
	case 'G':
		base = 648
	case 'H', 'R':
		base = 601
	case 'm':
		base = 693
	case 'i', 'l':
		base = 185
	case 't':
		base = 231
	case ' ':
		base = 230
	case 'c':
		base = 416
	case 's':
		base = 415
	case 'r':
		base = 277
	}
	return (base*sizePx + 6) / 13
}

func (controlLineMetrics) LineMetrics(sizePx int32, slot frame.FontSlot) (int32, int32, int32) {
	switch sizePx {
	case 12:
		return 11, 3, 14
	case 13:
		return 12, 3, 15
	case 24:
		return 22, 6, 28
	case 32:
		return 30, 7, 37
	case 64:
		return 59, 15, 74
	}
	return 14, 4, 18
}

func (controlLineMetrics) XHeightFixed(sizePx int32, slot frame.FontSlot) int32 {
	return sizePx * 67 / 2
}

func controlMetricsSession(t *testing.T, html string, viewportW float32) *layout.Arena {
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
	arena.Metrics = controlLineMetrics{}
	layout.Block(arena, layout.ObjectID(1), viewportW, 0)
	return arena
}

func controlOffsets(t *testing.T, arena *layout.Arena, row string) (lineH, top, left, text float32) {
	t.Helper()
	r := findByID(arena, row)
	if r == nil {
		t.Fatalf("#%s has no layout object", row)
	}
	text = -1
	found := false
	for k := r.FirstKid; k != 0; k = arena.Get(k).NextSibling {
		obj := arena.Get(k)
		if obj.Node == nil || !obj.Node.Element() {
			if text < 0 {
				text = obj.Y - r.Y
			}
			continue
		}
		lineH, top, left, found = r.BorderH(), obj.Y-r.Y, obj.X-r.X, true
		break
	}
	if !found {
		t.Fatalf("#%s has no element on its line", row)
	}
	return
}

const controlBaselineHTML = `<!DOCTYPE html><html><head><style>
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.row { width: 600px; }
.f12 { font-size: 12px; } .f13 { font-size: 13px; } .f24 { font-size: 24px; }
.f32 { font-size: 32px; } .f64 { font-size: 64px; }
.lh { line-height: 50px; }
</style></head><body>
<div class="row" id=a0><input type=checkbox></div>
<div class="row" id=a1>w <input type=checkbox></div>
<div class="row" id=a2><input type=radio></div>
<div class="row" id=a3>w <input type=radio></div>
<div class="row" id=a4><input type=range style=width:120px></div>
<div class="row" id=a5>w <input type=range style=width:120px></div>
<div class="row f12" id=b0><input type=checkbox></div>
<div class="row f13" id=b1>Label <input type=checkbox> after</div>
<div class="row f24" id=b2><input type=checkbox></div>
<div class="row f32" id=b3><input type=checkbox></div>
<div class="row f64" id=b4>Label <input type=checkbox> after</div>
<div class="row lh" id=b5><input type=checkbox></div>
<div class="row lh" id=b6>w <input type=checkbox></div>
<div class="row" id=c0><input type=checkbox style=vertical-align:top></div>
<div class="row" id=c1><input type=checkbox style=vertical-align:bottom></div>
<div class="row" id=c2><input type=checkbox style=vertical-align:5px></div>
<div class="row" id=c3><input type=checkbox style=vertical-align:-5px></div>
<div class="row" id=c4><input type=range style="width:120px;vertical-align:top"></div>
<div class="row" id=d0><input type=checkbox style=margin:0></div>
<div class="row" id=d1><input type=checkbox style="margin:10px 0"></div>
<div class="row" id=d2><input type=checkbox style="margin:0 0 12px 0"></div>
<div class="row" id=d3><input type=checkbox style="margin:12px 0 0 0"></div>
<div class="row" id=d4><input type=checkbox style="margin:5px 0 15px 0"></div>
<div class="row" id=d5>w <input type=checkbox style="margin:5px 0 15px 0"></div>
<div class="row" id=d6><input type=radio style=margin:0></div>
<div class="row" id=d7><input type=range style="margin:0;width:120px"></div>
<div class="row" id=d8><input type=range style="margin:8px 0 0 0;width:120px"></div>
<div class="row" id=d9><input type=checkbox style="margin:0;vertical-align:bottom"></div>
<div class="row" id=e0><input type=checkbox><input type=radio><input type=range style=width:120px></div>
<div class="row" id=f0><img width=13 height=13 style=margin:0></div>
<div class="row" id=f1><img width=13 height=13 style="margin:0 0 9px 0"></div>
<div class="row" id=f2><img width=13 height=13 style="margin:9px 0 0 0"></div>
<div class="row" id=f3><span style="display:inline-block;width:20px;height:20px"></span></div>
<div class="row" id=f4><span style="display:inline-block;width:20px;height:20px;margin:0 0 9px 0"></span></div>
<div class="row" id=f5><input type=text></div>
<div class="row" id=f6><button>Go</button></div>
<div class="row" id=f7><select><option>a</option></select></div>
</body></html>`

// qc:BR-274
func TestAppearanceWidgetsAlignOnTheirBottomBorderEdge(t *testing.T) {
	arena := controlMetricsSession(t, controlBaselineHTML, 800)
	for _, c := range []struct {
		id    string
		lineH float32
		top   float32
		left  float32
		why   string
	}{
		{"a0", 20, 3, 4, "checkbox alone: 3 + 13 above the baseline, 3 below, margin-left 4"},
		{"a2", 20, 3, 5, "radio alone: the same box on 3 above and 0 below, margin-left 5"},
		{"a4", 22, 2, 2, "range alone: 2 + 16 above the baseline, 2 below"},
		{"d0", 18, 1, 0, "checkbox with margin: 0 falls back on the strut: 14 above, 4 below"},
		{"d1", 33, 10, 0, "margin: 10px 0 puts all 20 of it outside the box: 23 above, 10 below"},
		{"d2", 26, 1, 0, "margin: 0 0 12px keeps the box on the baseline and hangs 12 under it"},
		{"d3", 29, 12, 0, "margin: 12px 0 0 lifts the line to 25 above and leaves the strut's 4 below"},
		{"d4", 33, 5, 0, "margin: 5px 0 15px: 18 above, 15 below - the widget's box rides the baseline, not its margin box"},
		{"d6", 18, 1, 0, "radio with margin: 0 answers like the checkbox with margin: 0"},
		{"d7", 20, 0, 0, "range with margin: 0: 16 above, so the baseline drops 2 below the strut's"},
		{"d8", 28, 8, 0, "range with margin-top: 8 keeps its bottom edge on the baseline"},
		{"e0", 22, 5, 4, "checkbox, radio and range on one line share the range's baseline"},
	} {
		lineH, top, left, _ := controlOffsets(t, arena, c.id)
		if !almostEqual(lineH, c.lineH) || !almostEqual(top, c.top) || !almostEqual(left, c.left) {
			t.Errorf("#%s = line %.4f, offset %.4f, left %.4f, want %.4f, %.4f, %.4f (%s)",
				c.id, lineH, top, left, c.lineH, c.top, c.left, c.why)
		}
	}
}

// qc:BR-274
func TestWidgetBaselineSidesResolveAgainstEveryStrut(t *testing.T) {
	arena := controlMetricsSession(t, controlBaselineHTML, 800)
	for _, c := range []struct {
		id    string
		lineH float32
		top   float32
		text  float32
		why   string
	}{
		{"b0", 19, 3, -1, "12px strut (11/3) under the widget's 16/3: 16 above, 3 below"},
		{"b1", 19, 3, 4, "13px strut (12/3): the widget still decides the baseline, its text rides 4 down"},
		{"b2", 28, 9, -1, "24px strut (22/6) decides both sides: the box lands 22-16+3=9 down"},
		{"b3", 37, 17, -1, "32px strut (30/7): 30-16+3=17"},
		{"b4", 74, 46, 0, "64px strut (59/15): 59-16+3=46, and the row's text starts on the line top"},
		{"b5", 50, 17, -1, "line-height: 50px (30 above, 20 below) leaves the widget 14+3 down"},
		{"b6", 50, 17, 16, "the same row with text: the block's own text starts under its half-leading"},
		{"a1", 20, 3, 2, "16px: the widget moves the baseline 2 down from the strut's, so its text follows"},
		{"a3", 20, 3, 2, "radio with text: bottom margin 0 costs the line nothing"},
		{"a5", 22, 2, 4, "range with text: the baseline drops 4 below the strut's"},
		{"d5", 33, 5, 4, "margin: 5px 0 15px with text: 15px of bottom margin is what stretches the line"},
	} {
		lineH, top, _, text := controlOffsets(t, arena, c.id)
		if !almostEqual(lineH, c.lineH) || !almostEqual(top, c.top) ||
			(c.text >= 0 && !almostEqual(text, c.text)) {
			t.Errorf("#%s = line %.4f, offset %.4f, text %.4f, want %.4f, %.4f, %.4f (%s)",
				c.id, lineH, top, text, c.lineH, c.top, c.text, c.why)
		}
	}
}

// qc:BR-274
func TestWidgetBaselineUnderAlignClassesAndOnAtomsThatKeepTheirEdge(t *testing.T) {
	arena := controlMetricsSession(t, controlBaselineHTML, 800)
	for _, c := range []struct {
		id    string
		lineH float32
		top   float32
		why   string
	}{
		{"c0", 19, 3, "top pins the 19px margin box to the line top"},
		{"c1", 19, 3, "bottom pins it to the line bottom, and the line is exactly its height"},
		{"c2", 25, 3, "5px raises the widget's baseline: 21 above, nothing below"},
		{"c3", 22, 6, "-5px lowers it: 11 above, 8 below the strut's baseline"},
		{"c4", 20, 2, "range with top: its 20px margin box is the line, 2px of margin above the box"},
		{"d9", 18, 5, "checkbox with margin: 0 and bottom: the box hangs under a 18px strut"},
		{"f0", 18, 1, "13px image: 13 above and 0 below, so the strut decides both sides"},
		{"f1", 26, 0, "image with margin-bottom: 9: the margin box reaches 22 and the line grows under it"},
		{"f2", 26, 9, "image with margin-top: 9: the same 22px margin box, lifted"},
		{"f3", 24, 0, "empty 20px inline-block: 20 above, strut's 4 below"},
		{"f4", 33, 0, "empty inline-block with margin-bottom: 9: 29 above, strut's 4 below"},
	} {
		lineH, top, _, _ := controlOffsets(t, arena, c.id)
		if !almostEqual(lineH, c.lineH) || !almostEqual(top, c.top) {
			t.Errorf("#%s = line %.4f, offset %.4f, want %.4f, %.4f (%s)",
				c.id, lineH, top, c.lineH, c.top, c.why)
		}
	}
}

// qc:BR-274
func TestControlsThatRenderAValueKeepTheirInteriorBaseline(t *testing.T) {
	arena := controlMetricsSession(t, controlBaselineHTML, 800)
	for _, c := range []struct {
		id    string
		lineH float32
		top   float32
		why   string
	}{
		{"f5", 21, 0, "text field: one line of the 13.3333px control font plus the 2px border and 1px padding the UA sheet carries"},
		{"f6", 21, 0, "button: line 21 at offset 0, the same answer the reference gives"},
		{"f7", 19, 0, "select: the control's own 19px box, which the UA sheet now builds out of a line and a 1px frame"},
	} {
		lineH, top, _, _ := controlOffsets(t, arena, c.id)
		if !almostEqual(lineH, c.lineH) || !almostEqual(top, c.top) {
			t.Errorf("#%s = line %.4f, offset %.4f, want %.4f, %.4f (%s)",
				c.id, lineH, top, c.lineH, c.top, c.why)
		}
	}
}
