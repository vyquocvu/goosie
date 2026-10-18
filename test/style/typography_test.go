package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestTypographyProperties(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	d1 := doc.NewElement("div")
	d1.SetAttribute("id", "d1")
	body.AppendChild(d1)

	d2 := doc.NewElement("div")
	d2.SetAttribute("id", "d2")
	body.AppendChild(d2)

	d3 := doc.NewElement("div")
	d3.SetAttribute("id", "d3")
	body.AppendChild(d3)

	sheet := css.Parse(`
		#d1 {
			word-break: break-all;
			hyphens: auto;
			tab-size: 4;
		}
		#d2 {
			word-break: keep-all;
			hyphens: none;
			tab-size: 16px;
		}
		#d3 {
			-webkit-hyphens: manual;
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s1 := styles[d1.ID]
	if s1.WordBreak != "break-all" {
		t.Errorf("d1 WordBreak = %q, want break-all", s1.WordBreak)
	}
	if s1.Hyphens != "auto" {
		t.Errorf("d1 Hyphens = %q, want auto", s1.Hyphens)
	}
	if s1.TabSize != 4 {
		t.Errorf("d1 TabSize = %v, want 4", s1.TabSize)
	}

	s2 := styles[d2.ID]
	if s2.WordBreak != "keep-all" {
		t.Errorf("d2 WordBreak = %q, want keep-all", s2.WordBreak)
	}
	if s2.Hyphens != "none" {
		t.Errorf("d2 Hyphens = %q, want none", s2.Hyphens)
	}
	if s2.TabSize != 16 {
		t.Errorf("d2 TabSize = %v, want 16", s2.TabSize)
	}

	s3 := styles[d3.ID]
	if s3.Hyphens != "manual" {
		t.Errorf("d3 Hyphens = %q, want manual", s3.Hyphens)
	}
}
