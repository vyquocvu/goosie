package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func positionedSession(t *testing.T, html string, viewportW float32) *layout.Arena {
	t.Helper()
	doc := domtest.Parse(html)
	styles := style.Resolve(doc, nil, nil)
	arena := layout.Build(doc, styles, nil)
	layout.Block(arena, layout.ObjectID(1), viewportW, 0)
	layout.Positioning(arena, layout.ObjectID(1), viewportW, 0)
	return arena
}

func findAbspos(arena *layout.Arena) []*layout.Object {
	var out []*layout.Object
	for i := range arena.Objects {
		o := &arena.Objects[i]
		if o.Style != nil && o.Style.Position == style.PositionAbsolute {
			out = append(out, o)
		}
	}
	return out
}

// An absolutely positioned box with auto width shrink-wraps its content
// (CSS 2.1 §10.3.7). When its children are blocks, there is no inline extent
// to read back, but the measure width must still collapse to the children's
// preferred widths: leaving the 10000px probe width in place paints
// viewport-wide strips (Google's dropdowns, carousels) and breaks every
// overflow clip and document extent around them.
func TestAbsposShrinkWrapsBlockChildren(t *testing.T) {
	arena := positionedSession(t, `<html><body><div style="position: relative;"><div style="position: absolute; top: 0; left: 0;"><p>menu</p></div></div></body></html>`, 800)
	boxes := findAbspos(arena)
	if len(boxes) != 1 {
		t.Fatalf("found %d abspos boxes, want 1", len(boxes))
	}
	if boxes[0].W > 500 {
		t.Errorf("abspos W = %v, want shrink-wrapped (probe width 10000 leaked)", boxes[0].W)
	}
}

func TestAbsposShrinkKeepsShortContentNarrow(t *testing.T) {
	arena := positionedSession(t, `<html><body><div style="position: relative;"><div style="position: absolute; top: 0; left: 0;"><div><span>hi there</span></div></div></div></body></html>`, 800)
	boxes := findAbspos(arena)
	if len(boxes) != 1 {
		t.Fatalf("found %d abspos boxes, want 1", len(boxes))
	}
	if boxes[0].W > 500 {
		t.Errorf("abspos W = %v, want shrink-wrapped", boxes[0].W)
	}
}
