package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func styleAtViewport(t *testing.T, html, sheet, tag string, vp style.Viewport) *style.ComputedStyle {
	t.Helper()
	doc := dom.Parse(html)
	var sheets []*css.Stylesheet
	if sheet != "" {
		sheets = append(sheets, css.Parse(sheet))
	}
	styles := style.ResolveViewport(doc, sheets, vp)
	var found *style.ComputedStyle
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n; c != nil && found == nil; c = c.NextSibling {
			if c.Element() && c.Data == tag {
				if s, ok := styles[c.ID]; ok {
					found = s
					return
				}
			}
			walk(c.FirstChild)
		}
	}
	walk(&doc.Node)
	return found
}

// A `clamp()` font size is how CSS-Tricks states its body type, and the bounds
// have to survive the cascade: the middle term gives 32px here, so the 30px upper
// bound is what lands.
func TestClampFontSizeReachesTheCascade(t *testing.T) {
	s := styleFor(t, `<html><body><main class="p"></main></body></html>`,
		`.p{font-size:clamp(10px,calc(1rem + 1em),30px)}`, "main")
	if s == nil {
		t.Fatal("main style not found")
	}
	if d := s.FontSize - 30; d > 0.01 || d < -0.01 {
		t.Errorf("FontSize = %v, want 30", s.FontSize)
	}
}

// Viewport units inside a function have to read the frame the document is laid
// out in, not the parser's fallback: MDN's side padding is
// `max(1rem,calc(50vw - 720px + 1rem))`, and at a 1280px frame that is 16px,
// while at 1600px the calc term gives 96px and wins.
func TestViewportUnitsInsideFunctionsReadTheFrame(t *testing.T) {
	sheet := `.p{padding-left:max(1rem,calc(50vw - 720px + 1rem))}`
	narrow := styleAtViewport(t, `<html><body><main class="p"></main></body></html>`, sheet, "main", style.Viewport{W: 1280, H: 800})
	if narrow == nil {
		t.Fatal("narrow main style not found")
	}
	if d := narrow.PaddingLeft - 16; d > 0.01 || d < -0.01 {
		t.Errorf("PaddingLeft at 1280px = %v, want 16", narrow.PaddingLeft)
	}
	wide := styleAtViewport(t, `<html><body><main class="p"></main></body></html>`, sheet, "main", style.Viewport{W: 1600, H: 900})
	if wide == nil {
		t.Fatal("wide main style not found")
	}
	if d := wide.PaddingLeft - 96; d > 0.01 || d < -0.01 {
		t.Errorf("PaddingLeft at 1600px = %v, want 96", wide.PaddingLeft)
	}
}
