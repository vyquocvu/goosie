package style_test

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
	"testing"
)

const viewportCascadeDoc = `<html><body><div><p>a <span>b</span> c</p></div></body></html>`

const viewportCascadeSheet = `
	p { margin: 1vh 2vw; width: 50vw; font: bold 12px/1.4 Helvetica, sans-serif; }
	span { color: maroon; background: rgba(0, 0, 0, 0.5); }
	div { padding: 3vh 3px; }
`

// The cascade rewrites viewport units for every declaration it resolves, so the
// rewrite is allowed to cost nothing only when it can prove there is nothing to
// rewrite. That proof is the thing this pins: the values that carry a viewport unit
// still resolve against the frame, and the ones that merely look like one - the "ma"
// of a colour name, the "va" of a keyword - still come through untouched. A fast path
// that skipped too much would show up here as a zero width or an unset colour.
func TestViewportUnitsAndTheirLookalikesBothSurviveTheCascade(t *testing.T) {
	vp := style.Viewport{W: 1440, H: 900}

	p := styleAtViewport(t, viewportCascadeDoc, viewportCascadeSheet, "p", vp)
	if p == nil {
		t.Fatal("p style not found")
	}
	if d := p.MarginTop - 9; d > 0.01 || d < -0.01 {
		t.Errorf("MarginTop = %v, want 1vh of 900 = 9", p.MarginTop)
	}
	if d := p.MarginLeft - 28.8; d > 0.01 || d < -0.01 {
		t.Errorf("MarginLeft = %v, want 2vw of 1440 = 28.8", p.MarginLeft)
	}
	if d := p.Width - 720; d > 0.01 || d < -0.01 {
		t.Errorf("Width = %v, want 50vw of 1440 = 720", p.Width)
	}
	if d := p.FontSize - 12; d > 0.01 || d < -0.01 {
		t.Errorf("FontSize = %v, want 12", p.FontSize)
	}

	div := styleAtViewport(t, viewportCascadeDoc, viewportCascadeSheet, "div", vp)
	if div == nil {
		t.Fatal("div style not found")
	}
	if d := div.PaddingTop - 27; d > 0.01 || d < -0.01 {
		t.Errorf("PaddingTop = %v, want 3vh of 900 = 27", div.PaddingTop)
	}
	if d := div.PaddingRight - 3; d > 0.01 || d < -0.01 {
		t.Errorf("PaddingRight = %v, want 3", div.PaddingRight)
	}

	span := styleAtViewport(t, viewportCascadeDoc, viewportCascadeSheet, "span", vp)
	if span == nil {
		t.Fatal("span style not found")
	}
	if span.Color != (css.Color{R: 128, G: 0, B: 0, A: 255}) {
		t.Errorf("Color = %+v, want maroon (#800000)", span.Color)
	}
	if span.BackgroundColor != (css.Color{R: 0, G: 0, B: 0, A: 128}) {
		t.Errorf("BackgroundColor = %+v, want rgba(0, 0, 0, 0.5)", span.BackgroundColor)
	}
}
