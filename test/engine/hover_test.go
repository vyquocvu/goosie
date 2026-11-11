package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// textCenter returns the centre of the laid-out word box whose text is
// exactly content, which is where a hover of that word must register.
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

// TestHasTextAtIncludesLineLeading is the example.com hover case: line-height
// 1.6 leaves a gap between wrapped glyph boxes. The I-beam has to stay down
// across that gap, and it has to come back up in the margin before the next
// paragraph.
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
