package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// qc:BR-257
func TestFloatContainerShrinkToFitIncludesAllFloatedChildren(t *testing.T) {
	session, err := engine.NewSession(`<html><body style="margin: 0;">
<div id="wrap" style="width: 800px;">
  <nav id="nav" style="float: left;">
    <a id="a" style="float: left; display: block; width: 100px; height: 40px;"></a>
    <a id="b" style="float: left; display: block; width: 120px; height: 40px;"></a>
    <a id="c" style="float: left; display: block; width: 80px; height: 40px;"></a>
  </nav>
  <div id="after" style="clear: both; height: 20px;"></div>
</div>
</body></html>`, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	nav := findByID(session.Arena, "nav")
	a := findByID(session.Arena, "a")
	b := findByID(session.Arena, "b")
	c := findByID(session.Arena, "c")
	after := findByID(session.Arena, "after")
	if nav == nil || a == nil || b == nil || c == nil || after == nil {
		t.Fatal("missing layout objects")
	}
	if !almostEqual(nav.W, 300) {
		t.Errorf("nav.W = %v, want 300 from all floated children", nav.W)
	}
	if !almostEqual(a.X, 0) || !almostEqual(b.X, 100) || !almostEqual(c.X, 220) {
		t.Errorf("float X = %v, %v, %v, want 0, 100, 220", a.X, b.X, c.X)
	}
	if !almostEqual(a.Y, 0) || !almostEqual(b.Y, 0) || !almostEqual(c.Y, 0) {
		t.Errorf("float Y = %v, %v, %v, want one horizontal row", a.Y, b.Y, c.Y)
	}
	if !almostEqual(nav.H, 40) {
		t.Errorf("nav.H = %v, want 40 from its floated children", nav.H)
	}
	if !almostEqual(after.Y, 40) {
		t.Errorf("cleared sibling Y = %v, want 40 below the floated subtree", after.Y)
	}
}
