package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// qc:BR-257
func TestAutoWidthFloatWithTextAndIconUsesInlineExtent(t *testing.T) {
	session, err := engine.NewSession(`<html><body style="margin: 0;">
<div id="wrap" style="width: 800px;">
  <nav id="nav" style="float: left;">
    <a id="a" style="float: left; display: block; padding: 8px 16px;">Tutorials<i id="icon" style="display: inline-block; width: 8px; height: 8px;"></i></a>
    <a id="b" style="float: left; display: block; padding: 8px 16px;">References<i style="display: inline-block; width: 8px; height: 8px;"></i></a>
  </nav>
</div>
</body></html>`, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	nav := findByID(session.Arena, "nav")
	a := findByID(session.Arena, "a")
	b := findByID(session.Arena, "b")
	if nav == nil || a == nil || b == nil {
		t.Fatal("missing layout objects")
	}
	if a.W <= 40 || b.W <= 40 {
		t.Errorf("float widths = %v, %v; text and icon must contribute to auto width", a.W, b.W)
	}
	if b.X < a.X+a.W-1 {
		t.Errorf("second float X = %v, want after first float ending at %v", b.X, a.X+a.W)
	}
	if nav.W < a.W+b.W-1 {
		t.Errorf("nav.W = %v, want at least %v", nav.W, a.W+b.W)
	}
	if nav.H < a.H {
		t.Errorf("nav.H = %v, want at least the height of its floated children %v", nav.H, a.H)
	}
}
