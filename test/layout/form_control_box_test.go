package layout_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

// TestReplacedInputTypesGetAControlBox guards TASK-028 / GOOSIE-CONTROLBOX-001:
// the UA stylesheet kept date, time, datetime-local, month, week, colour and
// file inputs at display:inline, so each one collapsed to a 0x0 box and the
// border and padding a page sets on it painted nothing at all. Five bands of
// testdata/render/forms/04-advanced-inputs.html were blank for that reason.
//
// The boxes asserted here are Chromium's *content* boxes for these controls,
// read through getComputedStyle on 2026-11-12 at its own 13.3333px control font:
// date 119.328125x17.328125, time 72x20, datetime-local 179.328125x17.328125,
// month 149.328125x17.328125, week 141.328125x17.328125, colour swatch 44x23
// and the file control's "Choose File" button and label 253x21. They are the
// reference's numbers at that font and no further: the same date field answers
// 125x18 at a 14px control font, which is the shadow editor's own metric and
// not the page's (GOOSIE-DATECONTROL-001). What the page does control is on the
// other side of the content box, and that is the half of this test the original
// defect was about. An author control font scales the reference's boxes too, and
// that scaling is GOOSIE-DATECONTROL-001 rather than anything pinned here.
func TestReplacedInputTypesGetAControlBox(t *testing.T) {
	arena := controlMetricsSession(t, `<html><head><style>
input { padding: 8px; border: 1px solid #ccc; }
</style></head><body>
<input type="date"><input type="time"><input type="datetime-local">
<input type="month"><input type="week"><input type="color"><input type="file">
</body></html>`, 800)

	want := map[string][2]float32{
		"date":           {119.328125, 17.328125},
		"time":           {72, 20},
		"datetime-local": {179.328125, 17.328125},
		"month":          {149.328125, 17.328125},
		"week":           {141.328125, 17.328125},
		"color":          {44, 23},
		"file":           {253, 21},
	}
	seen := map[string]bool{}
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node == nil || obj.Node.Data != "input" {
			continue
		}
		typ := strings.ToLower(obj.Node.GetAttribute("type"))
		box, ok := want[typ]
		if !ok {
			continue
		}
		seen[typ] = true
		if !almostEqual(obj.W, box[0]) {
			t.Errorf("%s input content W = %v, want %v (Chromium's content box)", typ, obj.W, box[0])
		}
		if !almostEqual(obj.H, box[1]) {
			t.Errorf("%s input content H = %v, want %v (Chromium's content box)", typ, obj.H, box[1])
		}
		// The page's chrome belongs outside that box: 8px of padding and a 1px
		// border on each side, which is what paints at all now that the control
		// has a box to paint on.
		if _, _, x1, y1 := obj.BorderRect(); !almostEqual(x1-obj.X, box[0]+18) || !almostEqual(y1-obj.Y, box[1]+18) {
			t.Errorf("%s input border box = %vx%v, want %vx%v (the page's padding and border reach the box)",
				typ, x1-obj.X, y1-obj.Y, box[0]+18, box[1]+18)
		}
	}
	for typ := range want {
		if !seen[typ] {
			t.Errorf("no layout box for input type %q", typ)
		}
	}
}

// TestReplacedInputTypesAreInlineLevel pins the display value that gives those
// controls their box. They stay inline-level like Chromium's, but inline-block
// rather than inline: a plain inline box has no replaced size of its own.
func TestReplacedInputTypesAreInlineLevel(t *testing.T) {
	arena := session(t, `<html><body>
<input type="date"><input type="time"><input type="month"><input type="color"><input type="file">
</body></html>`, 800)

	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node == nil || obj.Node.Data != "input" || obj.Style == nil {
			continue
		}
		if obj.Style.Display != style.DisplayInlineBlock {
			t.Errorf("%s input display = %v, want inline-block so it takes a replaced box",
				obj.Node.GetAttribute("type"), obj.Style.Display)
		}
	}
}
