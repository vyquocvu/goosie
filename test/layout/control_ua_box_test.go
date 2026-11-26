package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

const controlUAHTML = `<!DOCTYPE html><html><head><style>
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.row { width: 600px; }
</style></head><body>
<div class="row" id=g0><input id=i0 type=text></div>
<div class="row" id=g1><input id=i1 type=number></div>
<div class="row" id=g2><input id=i2 type=search></div>
<div class="row" id=g3><input id=i3 type=password></div>
<div class="row" id=g4><input id=i4 type=tel></div>
<div class="row" id=g5><input id=i5 type=url></div>
<div class="row" id=g6><input id=i6 type=email></div>
<div class="row" id=g7><select id=s7><option>a</option></select></div>
<div class="row" id=g8><textarea id=t8 rows=2 cols=20></textarea></div>
<div class="row" id=g9><button id=b9>Go</button></div>
<div class="row" id=g10><input id=i10 type=submit value=Submit></div>
<div class="row" id=g11><input id=i11 type=button value=Go></div>
<div class="row" id=g12><input id=i12 type=reset></div>
<div class="row" id=g13><input id=i13 type=checkbox></div>
<div class="row" id=g14><input id=i14 type=radio></div>
<div class="row" id=g15><input id=i15 type=range></div>
<div class="row" id=g16><input id=i16 type=color></div>
<div class="row" id=g17><input id=i17 type=file></div>
<div class="row" id=g18><input id=i18 type=date></div>
<div class="row" id=g19><input id=i19 type=time></div>
<div class="row" id=g20><input id=i20 type=month></div>
<div class="row" id=g21><input id=i21 type=week></div>
<div class="row" id=g22><input id=i22 type=datetime-local></div>
<div class="row" id=g23><input id=i23 type=image src=blank.png width=13 height=13></div>
<div class="row" id=g24><img id=m24 src=blank.png width=13 height=13></div>
<div class="row" id=g25><input id=i25 type=text style="padding:8px;border:1px solid #ccc"></div>
<div class="row" id=g26><input id=i26 type=text style="font-size:24px"></div>
<div class="row" id=g27><select id=s27 style="font-size:24px"><option>a</option></select></div>
<div class="row" id=g28><button id=b28 style="font-size:24px">Go</button></div>
</body></html>`

func controlUAStyle(t *testing.T, arena *layout.Arena, id string) *style.ComputedStyle {
	t.Helper()
	obj := findByID(arena, id)
	if obj == nil || obj.Style == nil {
		t.Fatalf("#%s has no layout box or computed style", id)
	}
	return obj.Style
}

func controlUABorderBox(t *testing.T, arena *layout.Arena, id string) (w, h float32) {
	t.Helper()
	obj := findByID(arena, id)
	if obj == nil {
		t.Fatalf("#%s has no layout box", id)
	}
	_, _, x1, y1 := obj.BorderRect()
	return x1 - obj.X, y1 - obj.Y
}

// qc:BR-276
func TestUASheetResolvesTheMeasuredControlCascade(t *testing.T) {
	arena := controlMetricsSession(t, controlUAHTML, 800)
	white := css.Color{R: 255, G: 255, B: 255, A: 255}
	face := css.Color{R: 239, G: 239, B: 239, A: 255}
	mid := css.Color{R: 118, G: 118, B: 118, A: 255}
	clear := css.Color{}
	for _, c := range []struct {
		id                     string
		fam                    frame.FontFamily
		bs                     style.BoxSizing
		bw                     float32
		bstyle                 string
		padT, padR, padB, padL float32
		bg                     css.Color
		mgn                    [4]float32
		why                    string
	}{
		{"i0", frame.FontArial, style.BoxSizingContentBox, 2, "inset", 1, 2, 1, 2, white, [4]float32{}, "a text field: a 2px inset frame in the reference's own grey around 1px/2px of padding"},
		{"i2", frame.FontArial, style.BoxSizingBorderBox, 2, "inset", 1, 2, 1, 2, white, [4]float32{}, "search differs from text only in box-sizing"},
		{"i13", frame.FontArial, style.BoxSizingBorderBox, 0, "", 0, 0, 0, 0, clear, [4]float32{3, 3, 3, 4}, "a checkbox carries no chrome and 3/3/3/4 of UA margin"},
		{"i14", frame.FontArial, style.BoxSizingBorderBox, 0, "", 0, 0, 0, 0, clear, [4]float32{3, 3, 0, 5}, "radio: the same box on 3/3/0/5"},
		{"i15", frame.FontArial, style.BoxSizingContentBox, 0, "", 0, 0, 0, 0, white, [4]float32{2, 2, 2, 2}, "range: no chrome, 2px of margin on every side"},
		{"i16", frame.FontArial, style.BoxSizingBorderBox, 1, "solid", 1, 2, 1, 2, face, [4]float32{}, "the colour swatch: a 1px frame on the button face"},
		{"i17", frame.FontArial, style.BoxSizingContentBox, 0, "", 0, 0, 0, 0, clear, [4]float32{}, "file: transparent and chromeless"},
		{"i18", frame.FontCourier, style.BoxSizingContentBox, 2, "inset", 0, 0, 0, 1, white, [4]float32{}, "date: the field frame with 1px of leading padding only"},
		{"s7", frame.FontArial, style.BoxSizingBorderBox, 1, "solid", 0, 0, 0, 0, white, [4]float32{}, "select: a 1px solid frame and no padding"},
		{"t8", frame.FontCourier, style.BoxSizingContentBox, 1, "solid", 2, 2, 2, 2, white, [4]float32{}, "textarea: monospace, 2px all round"},
		{"b9", frame.FontArial, style.BoxSizingBorderBox, 2, "outset", 1, 6, 1, 6, face, [4]float32{}, "button: the button face and chrome"},
		{"i10", frame.FontArial, style.BoxSizingBorderBox, 2, "outset", 1, 6, 1, 6, face, [4]float32{}, "submit: the same face as the real button"},
		{"i23", frame.FontArial, style.BoxSizingContentBox, 0, "", 0, 0, 0, 0, clear, [4]float32{}, "image input: no chrome, and inline-block like the reference"},
	} {
		s := controlUAStyle(t, arena, c.id)
		if !almostEqual(s.FontSize, 13.3333) {
			t.Errorf("#%s font-size = %v, want 13.3333 (%s)", c.id, s.FontSize, c.why)
		}
		if s.FontFamily != c.fam {
			t.Errorf("#%s family = %v, want %v (%s)", c.id, s.FontFamily, c.fam, c.why)
		}
		if s.BoxSizing != c.bs {
			t.Errorf("#%s box-sizing = %v, want %v (%s)", c.id, s.BoxSizing, c.bs, c.why)
		}
		if !almostEqual(s.BorderTopWidth, c.bw) || s.BorderTopStyle != c.bstyle {
			t.Errorf("#%s border-top = %v %v, want %v %v (%s)", c.id, s.BorderTopStyle, s.BorderTopWidth, c.bstyle, c.bw, c.why)
		}
		if !almostEqual(s.BorderTopWidth, s.BorderBottomWidth) || !almostEqual(s.BorderLeftWidth, s.BorderRightWidth) {
			t.Errorf("#%s borders differ by side: %v/%v/%v/%v (%s)", c.id,
				s.BorderTopWidth, s.BorderRightWidth, s.BorderBottomWidth, s.BorderLeftWidth, c.why)
		}
		if !almostEqual(s.PaddingTop, c.padT) || !almostEqual(s.PaddingRight, c.padR) ||
			!almostEqual(s.PaddingBottom, c.padB) || !almostEqual(s.PaddingLeft, c.padL) {
			t.Errorf("#%s padding = %v/%v/%v/%v, want %v/%v/%v/%v (%s)", c.id,
				s.PaddingTop, s.PaddingRight, s.PaddingBottom, s.PaddingLeft,
				c.padT, c.padR, c.padB, c.padL, c.why)
		}
		if s.BackgroundColor != c.bg {
			t.Errorf("#%s background = %v, want %v (%s)", c.id, s.BackgroundColor, c.bg, c.why)
		}
		m := [4]float32{s.MarginTop, s.MarginRight, s.MarginBottom, s.MarginLeft}
		if m != c.mgn {
			t.Errorf("#%s margins = %v, want %v (%s)", c.id, m, c.mgn, c.why)
		}
	}

	if s := controlUAStyle(t, arena, "i25"); !almostEqual(s.BorderTopWidth, 1) || s.BorderTopStyle != "solid" ||
		!almostEqual(s.PaddingTop, 8) || !almostEqual(s.PaddingLeft, 8) {
		t.Errorf("#i25 author chrome did not reach the box: border %v %v, padding %v/%v",
			s.BorderTopStyle, s.BorderTopWidth, s.PaddingTop, s.PaddingLeft)
	}
	if s := controlUAStyle(t, arena, "i26"); !almostEqual(s.FontSize, 24) {
		t.Errorf("#i26 author font-size = %v, want 24 to beat the UA control font", s.FontSize)
	}

	if s := controlUAStyle(t, arena, "i23"); s.Display != style.DisplayInlineBlock {
		t.Errorf("#i23 display = %v, want inline-block", s.Display)
	}

	for _, id := range []string{"i0", "i2", "i18", "s7", "t8"} {
		if s := controlUAStyle(t, arena, id); s.BorderTopColor != mid {
			t.Errorf("#%s border colour = %v, want %v", id, s.BorderTopColor, mid)
		}
	}
}

// qc:BR-276
func TestControlsSizeToTheirMeasuredBorderBoxes(t *testing.T) {
	arena := controlMetricsSession(t, controlUAHTML, 800)
	for _, c := range []struct {
		id         string
		w, h       float32
		checkWidth bool
		why        string
	}{
		{"i0", 153, 21, true, "text field: 145px of content in 8px of chrome"},
		{"i1", 153, 21, true, "number"},
		{"i2", 153, 21, true, "search"},
		{"i3", 153, 21, true, "password"},
		{"i4", 153, 21, true, "tel"},
		{"i5", 153, 21, true, "url"},
		{"i6", 153, 21, true, "email"},
		{"s7", 30, 19, true, "select: the height is one line of the control font and 4px; the 30px width is the option label plus Chromium's 20px select interior and 2px frame"},
		{"t8", 182, 36, true, "textarea rows=2: two lines and 6px of chrome; its 176px content is 20 characters plus Chromium's 16px scrollbar allowance"},
		{"b9", 33.796875, 21, true, "button: the label plus 16px"},
		{"i10", 57.5, 21, true, "submit with a value"},
		{"i11", 33.796875, 21, true, "input[type=button] answers exactly what the real button does"},
		{"i12", 50.828125, 21, true, "reset with no value takes the reference's own default label"},
		{"i13", 13, 13, true, "checkbox"},
		{"i14", 13, 13, true, "radio"},
		{"i15", 129, 16, true, "range"},
		{"i16", 50, 27, true, "colour swatch"},
		{"i17", 253, 21, true, "file: the interior button and its label, 21 tall like every one-line field"},
		{"i18", 124.328125, 21.328125, true, "date"},
		{"i19", 77, 24, true, "time"},
		{"i20", 154.328125, 21.328125, true, "month"},
		{"i21", 146.328125, 21.328125, true, "week"},
		{"i22", 184.328125, 21.328125, true, "datetime-local"},
		{"i23", 13, 13, true, "image input sized from its attributes, exactly the 13x13 the <img> beside it answers"},
		{"m24", 13, 13, true, "the plain image beside it"},
		{"i25", 163, 33, true, "the same field with the page's 8px padding and 1px border replacing the UA chrome"},
		{"i26", 258, 34, true, "the same field at an author 24px: the box is one line of it plus 6px; the reference's 258px width is the character metric"},
		{"b28", 48.015625, 34, true, "a 24px button answers 34 too"},
	} {
		w, h := controlUABorderBox(t, arena, c.id)
		if !almostEqual(h, c.h) {
			t.Errorf("#%s border box H = %v, want %v (%s)", c.id, h, c.h, c.why)
		}
		if c.checkWidth && !almostEqual(w, c.w) {
			t.Errorf("#%s border box W = %v, want %v (%s)", c.id, w, c.w, c.why)
		}
	}
	if w, h := controlUABorderBox(t, arena, "s27"); !almostEqual(h, 32) || !almostEqual(w, 36) {
		t.Errorf("#s27 border box = %vx%v, want 36x32 (a 24px select: the reference answers 36x31)", w, h)
	}
}

// qc:BR-276
func TestControlsKeepTheirMeasuredLinesAndOffsets(t *testing.T) {
	arena := controlMetricsSession(t, controlUAHTML, 800)
	for _, c := range []struct {
		id        string
		line, top float32
		left      float32
		checkLeft bool
		why       string
	}{
		{"i0", 21, 0, 0, false, "text field: the interior value line covers the strut's descent"},
		{"i2", 21, 0, 0, false, "search"},
		{"s7", 19, 0, 0, false, "select: its own 19px box decides the line"},
		{"t8", 40, 0, 0, false, "textarea: an empty control has no line boxes, so it aligns on its bottom margin edge and the strut's 4 lands under it"},
		{"b9", 21, 0, 0, false, "button: one line of the control font and its 6px of chrome"},
		{"i10", 21, 0, 0, false, "submit"},
		{"i13", 20, 3, 4, true, "checkbox: 3px of margin and 13 of box above the baseline, 3 below"},
		{"i14", 20, 3, 5, true, "radio"},
		{"i15", 22, 2, 2, true, "range: 2px of margin and 16 of box above, 2 below"},
		{"i16", 27, 0, 0, false, "colour swatch"},
		{"i17", 22, 0, 0, false, "file: Chromium's line is 21, its own box; this engine answers 22 because a chromeless control leaves the parent strut's 4px descent as the only thing under its baseline, and it lands 1px outside the box (GOOSIE-INLINEBASELINE-001)"},
		{"i18", 21.328125, 0, 0, false, "date: the line is the box, which is the shadow editor's height and not the page's"},
		{"i19", 24, 0, 0, false, "time"},
		{"i23", 18, 1, 0, false, "image input: bottom margin edge, so the strut decides both sides"},
		{"m24", 18, 1, 0, false, "and the image it answers identically to"},
		{"i25", 33, 0, 0, false, "the page's own 8px padding and 1px border move the interior baseline down with them"},
		{"i26", 34, 0, 0, false, "a 24px field is 34 tall on the line the reference gives it"},
	} {
		lineH, top, left, _ := controlOffsets(t, arena, controlUARow(c.id))
		if !almostEqual(lineH, c.line) || !almostEqual(top, c.top) {
			t.Errorf("#%s = line %v, offset %v, want %v, %v (%s)", c.id, lineH, top, c.line, c.top, c.why)
		}
		if c.checkLeft && !almostEqual(left, c.left) {
			t.Errorf("#%s left = %v, want %v (%s)", c.id, left, c.left, c.why)
		}
	}
}

func controlUARow(id string) string {
	digits := ""
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] < '0' || id[i] > '9' {
			break
		}
		digits = string(id[i]) + digits
	}
	return "g" + digits
}
