package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func textCenter(t *testing.T, s *engine.Session, content string) (float32, float32) {
	t.Helper()
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.DataContent == content {
			x0, y0, x1, y1 := obj.BorderRect()
			if x0 >= x1 || y0 >= y1 {
				t.Fatalf("word %q has an empty box", content)
			}
			return (x0 + x1) / 2, (y0 + y1) / 2
		}
	}
	t.Fatalf("word %q not found in arena", content)
	return 0, 0
}

func TestHasControlAtHover(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	if !s.HasControlAt(x, y) {
		t.Fatalf("HasControlAt(%v, %v) = false, want true over input#a", x, y)
	}
	if s.HasControlAt(0, 5000) {
		t.Fatal("HasControlAt reports a control past the end of the document")
	}
}

func TestHasTextAtHover(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;"><p id="t">Hello world</p></body></html>`)
	x, y := textCenter(t, s, "Hello")
	if !s.HasTextAt(x, y) {
		t.Fatalf("HasTextAt(%v, %v) = false, want true over the word Hello", x, y)
	}
	if s.HasTextAt(0, 5000) {
		t.Fatal("HasTextAt reports text past the end of the document")
	}
}

func TestHasTextAtIncludesLineLeading(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin:0">
		<p style="font-size:16px;line-height:1.6;max-width:8em;margin:0">Hello world this text wraps onto another line</p>
		<p style="margin-top:48px">Next</p>
	</body></html>`)
	x0, y0 := textCenter(t, s, "Hello")
	_, y1 := textCenter(t, s, "line")
	if y1 <= y0 {
		t.Fatalf("fixture did not wrap: Hello y=%v line y=%v", y0, y1)
	}
	mid := (y0 + y1) / 2
	if !s.HasTextAt(x0, mid) {
		t.Fatalf("HasTextAt(%v, %v) = false in the leading between wrapped lines", x0, mid)
	}
	_, nextY := textCenter(t, s, "Next")
	gap := y1 + (nextY-y1)/2
	if s.HasTextAt(x0, gap) {
		t.Fatalf("HasTextAt(%v, %v) = true in the margin between paragraphs", x0, gap)
	}
}

func TestHasTextAtExcludesColumnGutter(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin:0">
		<p style="position:absolute;left:0;top:0;margin:0">Left</p>
		<p style="position:absolute;left:200px;top:0;margin:0">Right</p>
	</body></html>`)
	lx, ly := textCenter(t, s, "Left")
	rx, ry := textCenter(t, s, "Right")
	if ly != ry {
		t.Fatalf("fixture columns are not side by side: left y=%v right y=%v", ly, ry)
	}
	gutter := (lx + rx) / 2
	if s.HasTextAt(gutter, ly) {
		t.Fatalf("HasTextAt(%v, %v) = true in the gutter between columns", gutter, ly)
	}
	if !s.HasTextAt(lx, ly) || !s.HasTextAt(rx, ry) {
		t.Fatal("HasTextAt lost the column text itself")
	}
}

func TestHasTextAtOutOfDocumentOrder(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin:0">
		<p style="margin:0">Top</p>
		<p style="position:absolute;left:0;top:100px;margin:0">Bottom</p>
		<p style="position:absolute;left:0;top:50px;margin:0">Middle</p>
	</body></html>`)
	x, topY := textCenter(t, s, "Top")
	_, midY := textCenter(t, s, "Middle")
	_, botY := textCenter(t, s, "Bottom")
	if !(topY < midY && midY < botY) {
		t.Fatalf("fixture not stacked: top=%v mid=%v bot=%v", topY, midY, botY)
	}
	gap := (topY + midY) / 2
	if s.HasTextAt(x, gap) {
		t.Fatalf("HasTextAt(%v, %v) = true in the empty gap between lines", x, gap)
	}
	if !s.HasTextAt(x, topY) || !s.HasTextAt(x, midY) || !s.HasTextAt(x, botY) {
		t.Fatal("HasTextAt lost one of the three lines")
	}
}
