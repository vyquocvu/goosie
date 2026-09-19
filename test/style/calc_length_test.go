package style_test

import (
	"testing"
)

// lobste.rs hangs its story list into the left gutter with a `calc()` that adds
// up the header's own parts: logo width, its paddings, and the anchor margins
// around it. Left unparsed the whole article column slid 36px to the left of
// where Chromium puts it, which is a mismatch on every text line of the page.
func TestCalcLengthsReachTheCascade(t *testing.T) {
	s := styleFor(t, `<html><body style="margin:0"><main>stories</main></body></html>`,
		`main{margin-left:calc(10px + 18px + 0.25rem + 0.25rem);margin-right:calc(2em - 6px);width:calc((100px + 20px) * 2)}`,
		"main")
	if s == nil {
		t.Fatal("main style not found")
	}
	if s.MarginLeft != 36 {
		t.Errorf("MarginLeft = %v, want 36", s.MarginLeft)
	}
	if s.MarginRight != 26 {
		t.Errorf("MarginRight = %v, want 26", s.MarginRight)
	}
	if s.Width != 240 {
		t.Errorf("Width = %v, want 240", s.Width)
	}
}
