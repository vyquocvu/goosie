package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestSpacingWithCalcAndEm(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	p1 := doc.NewElement("p")
	p1.SetAttribute("id", "p1")
	body.AppendChild(p1)

	p2 := doc.NewElement("p")
	p2.SetAttribute("id", "p2")
	body.AppendChild(p2)

	sheet := css.Parse(`
		#p1 {
			font-size: 20px;
			letter-spacing: 0.1em;
			word-spacing: 0.5em;
		}
		#p2 {
			font-size: 16px;
			letter-spacing: calc(2px + 3px);
			word-spacing: calc(10px - 2px);
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s1 := styles[p1.ID]
	// 0.1em at 20px is 2px
	if s1.LetterSpacing != 2 {
		t.Errorf("p1 LetterSpacing: want 2, got %v", s1.LetterSpacing)
	}
	// 0.5em at 20px is 10px
	if s1.WordSpacing != 10 {
		t.Errorf("p1 WordSpacing: want 10, got %v", s1.WordSpacing)
	}

	s2 := styles[p2.ID]
	// calc(2px + 3px) is 5px
	if s2.LetterSpacing != 5 {
		t.Errorf("p2 LetterSpacing: want 5, got %v", s2.LetterSpacing)
	}
	// calc(10px - 2px) is 8px
	if s2.WordSpacing != 8 {
		t.Errorf("p2 WordSpacing: want 8, got %v", s2.WordSpacing)
	}
}
