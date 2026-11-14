package layout_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

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
