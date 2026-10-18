package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestOverscrollBehavior(t *testing.T) {
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
		#d1 { overscroll-behavior: contain; }
		#d2 { overscroll-behavior: none auto; }
		#d3 { overscroll-behavior-x: none; overscroll-behavior-y: contain; }
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s1 := styles[d1.ID]
	if s1.OverscrollBehaviorX != "contain" || s1.OverscrollBehaviorY != "contain" {
		t.Errorf("d1 overscroll-behavior = %q / %q, want contain / contain", s1.OverscrollBehaviorX, s1.OverscrollBehaviorY)
	}

	s2 := styles[d2.ID]
	if s2.OverscrollBehaviorX != "none" || s2.OverscrollBehaviorY != "auto" {
		t.Errorf("d2 overscroll-behavior = %q / %q, want none / auto", s2.OverscrollBehaviorX, s2.OverscrollBehaviorY)
	}

	s3 := styles[d3.ID]
	if s3.OverscrollBehaviorX != "none" || s3.OverscrollBehaviorY != "contain" {
		t.Errorf("d3 overscroll-behavior = %q / %q, want none / contain", s3.OverscrollBehaviorX, s3.OverscrollBehaviorY)
	}
}
