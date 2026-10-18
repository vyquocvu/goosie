package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestTextUnderlineOffsetAndThickness(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	p1 := doc.NewElement("p")
	p1.SetAttribute("id", "p1")
	body.AppendChild(p1)

	span1 := doc.NewElement("span")
	span1.SetAttribute("id", "span1")
	p1.AppendChild(span1)

	sheet := css.Parse(`
		#p1 {
			font-size: 20px;
			text-decoration: underline;
			text-underline-offset: 4px;
			text-decoration-thickness: 2px;
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s1 := styles[p1.ID]
	if s1.TextUnderlineOffset != 4 {
		t.Errorf("p1 TextUnderlineOffset: want 4, got %v", s1.TextUnderlineOffset)
	}
	if s1.TextDecorationThickness != 2 {
		t.Errorf("p1 TextDecorationThickness: want 2, got %v", s1.TextDecorationThickness)
	}

	// Verify child inherits TextUnderlineOffset and TextDecorationThickness
	sSpan := styles[span1.ID]
	if sSpan.TextUnderlineOffset != 4 {
		t.Errorf("span1 inherited TextUnderlineOffset: want 4, got %v", sSpan.TextUnderlineOffset)
	}
	if sSpan.TextDecorationThickness != 2 {
		t.Errorf("span1 inherited TextDecorationThickness: want 2, got %v", sSpan.TextDecorationThickness)
	}
}
