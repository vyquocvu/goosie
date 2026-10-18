package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestPointerEventsCascade(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	parent := doc.NewElement("div")
	parent.SetAttribute("id", "p")
	body.AppendChild(parent)

	c1 := doc.NewElement("div")
	c1.SetAttribute("id", "c1")
	parent.AppendChild(c1)

	c2 := doc.NewElement("div")
	c2.SetAttribute("id", "c2")
	parent.AppendChild(c2)

	sheet := css.Parse(`
		#p { pointer-events: none; }
		#c1 { }
		#c2 { pointer-events: auto; }
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	pStyle := styles[parent.ID]
	if pStyle == nil || pStyle.PointerEvents != "none" {
		t.Fatalf("parent PointerEvents = %q, want none", pStyle.PointerEvents)
	}

	c1Style := styles[c1.ID]
	if c1Style == nil || c1Style.PointerEvents != "none" {
		t.Fatalf("child1 inherited PointerEvents = %q, want none", c1Style.PointerEvents)
	}

	c2Style := styles[c2.ID]
	if c2Style == nil || c2Style.PointerEvents != "auto" {
		t.Fatalf("child2 overridden PointerEvents = %q, want auto", c2Style.PointerEvents)
	}
}
