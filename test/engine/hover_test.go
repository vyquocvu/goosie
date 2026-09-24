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
