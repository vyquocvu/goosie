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
// The widths asserted here are Chromium's intrinsic control widths, measured on
// that fixture at 800x600 DPR 1 as a border box minus the page's 8px padding and
// 1px border, so a control the engine has no picker for still occupies the row
// Chromium gives it.
func TestReplacedInputTypesGetAControlBox(t *testing.T) {
	arena := session(t, `<html><head><style>
input { padding: 8px; border: 1px solid #ccc; font-size: 14px; }
</style></head><body>
<input type="date"><input type="time"><input type="datetime-local">
<input type="month"><input type="week"><input type="color"><input type="file">
</body></html>`, 800)

	want := map[string]float32{
		"date":           123,
		"time":           73,
		"datetime-local": 186,
		"month":          155,
		"week":           146,
		"color":          32,
		"file":           214,
	}
	seen := map[string]bool{}
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node == nil || obj.Node.Data != "input" {
			continue
		}
		typ := strings.ToLower(obj.Node.GetAttribute("type"))
		width, ok := want[typ]
		if !ok {
			continue
		}
		seen[typ] = true
		if !almostEqual(obj.W, width) {
			t.Errorf("%s input W = %v, want %v (Chromium's intrinsic control width)", typ, obj.W, width)
		}
		if obj.H <= 0 {
			t.Errorf("%s input H = %v; a replaced control must occupy its line", typ, obj.H)
		}
		if obj.W <= 0 {
			t.Errorf("%s input W = %v; the page's border and padding on it paint nothing", typ, obj.W)
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
