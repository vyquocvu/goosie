package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestOutlineShorthandResolves(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	input := doc.NewElement("input")
	input.SetAttribute("type", "checkbox")
	input.SetAttribute("checked", "")
	body.AppendChild(input)

	sheet := css.Parse(`
		body { color: #123456; }
		input:checked { outline: 2px solid #2ecc71; }
		input#off { outline: none; }
	`)
	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s := styles[input.ID]
	if s == nil {
		t.Fatal("no computed style for the checked input")
	}
	if s.OutlineWidth != 2 {
		t.Errorf("OutlineWidth = %v, want 2", s.OutlineWidth)
	}
	if s.OutlineStyle != "solid" {
		t.Errorf("OutlineStyle = %q, want solid", s.OutlineStyle)
	}
	if want := (css.Color{R: 46, G: 204, B: 113, A: 255}); s.OutlineColor != want {
		t.Errorf("OutlineColor = %v, want %v", s.OutlineColor, want)
	}

	plain := doc.NewElement("input")
	plain.SetAttribute("type", "text")
	body.AppendChild(plain)
	plainStyles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)
	if ps := plainStyles[plain.ID]; ps.OutlineStyle != "" || ps.OutlineWidth != 0 {
		t.Errorf("undeclared outline = (%q, %v), want the initial no-ring state", ps.OutlineStyle, ps.OutlineWidth)
	}
}

func TestOutlineLonghandsAndOffset(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	div := doc.NewElement("div")
	body.AppendChild(div)

	sheet := css.Parse(`div { outline-width: 3px; outline-style: dashed; outline-color: #ff0000; outline-offset: 4px; }`)
	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)
	s := styles[div.ID]
	if s == nil {
		t.Fatal("no computed style for the div")
	}
	if s.OutlineWidth != 3 || s.OutlineStyle != "dashed" {
		t.Errorf("outline = (%v, %q), want (3, dashed)", s.OutlineWidth, s.OutlineStyle)
	}
	if want := (css.Color{R: 255, G: 0, B: 0, A: 255}); s.OutlineColor != want {
		t.Errorf("OutlineColor = %v, want %v", s.OutlineColor, want)
	}
	if s.OutlineOffset != 4 {
		t.Errorf("OutlineOffset = %v, want 4", s.OutlineOffset)
	}
}
