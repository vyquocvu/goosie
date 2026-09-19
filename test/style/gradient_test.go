package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

// styleFor returns the computed style of the first element named tag.
func styleFor(t *testing.T, html, sheet, tag string) *style.ComputedStyle {
	t.Helper()
	doc := dom.Parse(html)
	var sheets []*css.Stylesheet
	if sheet != "" {
		sheets = append(sheets, css.Parse(sheet))
	}
	styles := style.Resolve(doc, sheets)
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
	if found == nil {
		t.Fatalf("no <%s> resolved in %s", tag, html)
	}
	return found
}

func TestUnitlessLineHeightRecomputesPerFontSize(t *testing.T) {
	// A unitless line-height inherits as the number, not as the length its parent
	// computed, so a 1.6 on the body gives a 32px heading 51px rather than the
	// 26px its 16px parent worked out. Getting this wrong moves every line below
	// the first heading out of place.
	const html = `<html><body><h1>title</h1><p>text</p></body></html>`
	const sheet = `body { line-height: 1.6 }`
	p := styleFor(t, html, sheet, "p")
	if want := p.FontSize * 1.6; p.LineHeight != want {
		t.Errorf("p line-height = %v, want %v", p.LineHeight, want)
	}
	h1 := styleFor(t, html, sheet, "h1")
	if h1.FontSize <= p.FontSize {
		t.Fatalf("h1 font size %v should exceed p's %v", h1.FontSize, p.FontSize)
	}
	if want := h1.FontSize * 1.6; h1.LineHeight != want {
		t.Errorf("h1 line-height = %v, want %v", h1.LineHeight, want)
	}
}

func TestLengthLineHeightStillInheritsAsAValue(t *testing.T) {
	// The contrast case: a declared length is a length, so the child inherits the
	// resolved number and does not re-derive it.
	p := styleFor(t, `<html><body><h1>t</h1></body></html>`, `body { line-height: 20px }`, "h1")
	if p.LineHeight != 20 {
		t.Errorf("h1 line-height = %v, want 20", p.LineHeight)
	}
}

func TestEmLineHeightResolvesAgainstOwnFontSize(t *testing.T) {
	// `line-height: 1em` is a length whose em unit belongs to the element's own
	// final font size, whichever order the size arrives in. yesterweb's nav links
	// declare the line-height before the 22px size in the same rule and its
	// headings declare them in two separate rules; resolving either against the
	// default 16px shrinks every line box and drags the rest of the page up.
	t.Run("declared before the size in the same rule", func(t *testing.T) {
		a := styleFor(t, `<html><body><a href="#">x</a></body></html>`,
			`a { line-height: 1em; font-size: 22px }`, "a")
		if a.LineHeight != 22 {
			t.Errorf("a line-height = %v, want 22", a.LineHeight)
		}
	})
	t.Run("declared in a later rule", func(t *testing.T) {
		h2 := styleFor(t, `<html><body><h2>x</h2></body></html>`,
			`h2 { font-size: 25px } h2 { line-height: 1em }`, "h2")
		if h2.LineHeight != 25 {
			t.Errorf("h2 line-height = %v, want 25", h2.LineHeight)
		}
	})
	t.Run("inherits as the resolved length", func(t *testing.T) {
		// The computed value of an em line-height is a length, so a child takes
		// the 20px the parent worked out rather than re-deriving 2em itself.
		h1 := styleFor(t, `<html><body><h1>x</h1></body></html>`,
			`body { font-size: 10px; line-height: 2em } h1 { font-size: 32px }`, "h1")
		if h1.LineHeight != 20 {
			t.Errorf("h1 line-height = %v, want 20 (the parent's resolved length)", h1.LineHeight)
		}
	})
}

func TestBackgroundGradientRamp(t *testing.T) {
	s := styleFor(t, `<html><body><div class="box">x</div></body></html>`,
		`.box { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%) }`, "div")
	g := s.BackgroundGradient
	if g.Angle != 135 {
		t.Errorf("angle = %v, want 135", g.Angle)
	}
	if len(g.Stops) != 2 {
		t.Fatalf("stops = %+v, want two", g.Stops)
	}
	if g.Stops[0].At != 0 || g.Stops[1].At != 1 {
		t.Errorf("stop positions = %v, %v, want 0 and 1", g.Stops[0].At, g.Stops[1].At)
	}
	for i, want := range []css.Color{{0x66, 0x7e, 0xea, 255}, {0x76, 0x4b, 0xa2, 255}} {
		if g.Stops[i].Color != want {
			t.Errorf("stop %d colour = %+v, want %+v", i, g.Stops[i].Color, want)
		}
	}
	// The shorthand declares no colour, and a gradient is not one.
	if s.BackgroundColor.A != 0 {
		t.Errorf("background colour = %+v, want transparent", s.BackgroundColor)
	}
}

func TestGradientStopsWithoutPositionsSpreadEvenly(t *testing.T) {
	s := styleFor(t, `<html><body><div class="box">x</div></body></html>`,
		`.box { background-image: linear-gradient(to right, red, green, blue) }`, "div")
	g := s.BackgroundGradient
	if g.Angle != 90 {
		t.Errorf("angle = %v, want 90", g.Angle)
	}
	if len(g.Stops) != 3 {
		t.Fatalf("stops = %+v, want three", g.Stops)
	}
	for i, want := range []float32{0, 0.5, 1} {
		if g.Stops[i].At != want {
			t.Errorf("stop %d position = %v, want %v", i, g.Stops[i].At, want)
		}
	}
}

func TestGradientWithAlphaFunctionStops(t *testing.T) {
	s := styleFor(t, `<html><body><div class="box">x</div></body></html>`,
		`.box { background-image: linear-gradient(rgba(0, 0, 0, 0.5), rgba(255, 255, 255, 0.25)) }`, "div")
	if len(s.BackgroundGradient.Stops) != 2 {
		t.Fatalf("stops = %+v, want two", s.BackgroundGradient.Stops)
	}
	if got := s.BackgroundGradient.Stops[0].Color.A; got != 128 {
		t.Errorf("first stop alpha = %v, want 128", got)
	}
}

func TestFlatBackgroundIsNotAGradient(t *testing.T) {
	s := styleFor(t, `<html><body><div class="box">x</div></body></html>`,
		`.box { background: #0a0b0c }`, "div")
	if !s.BackgroundGradient.Empty() {
		t.Errorf("gradient = %+v, want none", s.BackgroundGradient)
	}
	if want := (css.Color{0x0a, 0x0b, 0x0c, 255}); s.BackgroundColor != want {
		t.Errorf("colour = %+v, want %+v", s.BackgroundColor, want)
	}
}
