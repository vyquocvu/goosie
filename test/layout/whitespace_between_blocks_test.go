package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func TestWhitespaceTextBetweenBlockChildrenDoesNotCreateLineBox(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div id="wrap">
  <div id="first" style="height: 10px;"></div>
  <div id="second" style="height: 20px;"></div>
</div>
</body></html>`, 800)

	wrap := findByID(arena, "wrap")
	first := findByID(arena, "first")
	second := findByID(arena, "second")
	if wrap == nil || first == nil || second == nil {
		t.Fatal("missing layout objects")
	}
	if !almostEqual(first.Y, 0) || !almostEqual(second.Y, 10) {
		t.Errorf("children Y = %v, %v, want 0, 10 without a whitespace line box", first.Y, second.Y)
	}
	if !almostEqual(wrap.H, 30) {
		t.Errorf("wrap.H = %v, want 30", wrap.H)
	}
}

func TestWhitespaceTextBetweenInlineChildrenStillRendersSpace(t *testing.T) {
	session, err := engine.NewSession(`<html><body style="margin: 0;">
<div id="wrap"><span id="a">a</span> <span id="b">b</span></div>
</body></html>`, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	b := findByID(session.Arena, "b")
	if b == nil {
		t.Fatal("missing second span")
	}
	if !almostEqual(b.X, 16) {
		t.Errorf("second span X = %v, want 16 (a glyph plus one whitespace)", b.X)
	}
}

func TestEmptyTablePseudoDoesNotAddLineBox(t *testing.T) {
	session, err := engine.NewSession(`<html><head><style>
#wrap::before { content: ""; display: table; }
#wrap::after { content: ""; display: table; clear: both; }
#child { float: left; width: 10px; height: 20px; }
</style></head><body style="margin: 0;">
<div id="wrap"><div id="child"></div></div>
</body></html>`, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	wrap := findByID(session.Arena, "wrap")
	child := findByID(session.Arena, "child")
	if wrap == nil || child == nil {
		t.Fatal("missing layout objects")
	}
	if !almostEqual(child.Y, 0) {
		t.Errorf("child.Y = %v, want 0 without a clearfix line box", child.Y)
	}
	if !almostEqual(wrap.H, 20) {
		t.Errorf("wrap.H = %v, want 20", wrap.H)
	}
}
