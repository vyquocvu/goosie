package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func svgFindByID(doc *dom.Document, id string) *dom.Node {
	var found *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if found != nil {
			return
		}
		if n.Element() && n.GetAttribute("id") == id {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	return found
}

// An explicit end tag for a void-marked SVG element (<path>...</path>, as
// served by Google's header icons) has nothing to pop: the opener never went
// on the stack. HTML5 treats it as a parse error to ignore. Popping anyway
// unwinds the whole open stack through <html>, reparenting the rest of the
// document outside it — which pushed Google's nav, logo and search a full
// viewport below the fold.
func TestSVGExplicitVoidEndTagIgnored(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="outer"><svg viewBox="0 0 24 24"><path d="M0 0h1"></path></svg><div id="after">x</div></div></body></html>`)
	after := svgFindByID(doc, "after")
	if after == nil {
		t.Fatal("after div missing")
	}
	if after.Parent == nil || after.Parent.GetAttribute("id") != "outer" {
		t.Fatalf("after div escaped its container (parent id %q)", after.Parent.GetAttribute("id"))
	}
}

func TestSVGImageExplicitEndTagIgnored(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="outer"><svg><image href="x.png"></image></svg><p id="after">x</p></div></body></html>`)
	after := svgFindByID(doc, "after")
	if after == nil {
		t.Fatal("after p missing")
	}
	if after.Parent == nil || after.Parent.GetAttribute("id") != "outer" {
		t.Fatalf("after p escaped its container (parent id %q)", after.Parent.GetAttribute("id"))
	}
}

// The document element must survive: only <html> may stand at top level
// after the void end tag is ignored.
func TestSVGEndTagKeepsDocumentRoot(t *testing.T) {
	doc := domtest.Parse(`<html><body><div><svg><path d="M0 0h1"></path></svg></div><div id="after">x</div></body></html>`)
	count := 0
	for c := doc.Node.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() {
			count++
			if c.Data != "html" {
				t.Fatalf("top-level element %q, want only html", c.Data)
			}
		}
	}
	if count != 1 {
		t.Fatalf("%d top-level elements, want 1", count)
	}
}

// Normal SVG open/close still nests: the guard must not break the paired path.
func TestSVGNestingIntact(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="outer"><svg viewBox="0 0 1 1"><g><path d="M0 0h1"/></g></svg><div id="after">x</div></div></body></html>`)
	after := svgFindByID(doc, "after")
	if after == nil || after.Parent == nil || after.Parent.GetAttribute("id") != "outer" {
		t.Fatal("normal svg nesting broke")
	}
}
