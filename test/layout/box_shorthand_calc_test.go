package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func layoutAtViewport(t *testing.T, html, sheet string, w, h float32) *layout.Arena {
	t.Helper()
	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	if sheet != "" {
		sheets = append(sheets, css.Parse(sheet))
	}
	styles := style.ResolveViewport(doc, sheets, style.Viewport{W: w, H: h}, nil)
	arena := layout.Build(doc, styles, nil)
	layout.Block(arena, layout.ObjectID(1), w, h)
	return arena
}

func TestCalcPaddingInAShorthandMovesTheFlow(t *testing.T) {
	const doc = `<html><body style="margin:0"><div><p>a</p></div><section>b</section></body></html>`
	arena := layoutAtViewport(t, doc, "div { height: 20px; padding: calc(10vh - 4px) 3px; }", 1440, 900)
	section := findByTag(arena, "section")
	if section == nil {
		t.Fatal("section not found")
	}
	if !almostEqual(section.Y, 192) {
		t.Errorf("section.Y = %v, want 192 (86 top padding + 20 box + 86 bottom padding)", section.Y)
	}
	p := findByTag(arena, "p")
	if p == nil {
		t.Fatal("p not found")
	}
	if !almostEqual(p.X, 3) {
		t.Errorf("p.X = %v, want 3 (the shorthand's second argument)", p.X)
	}
}
