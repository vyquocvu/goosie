package dom_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

// TestLessThanFollowedByNonLetterIsText pins the tag-open state: only a
// letter after "<" starts a tag. "a <= b", "2 < 3" and a trailing "<" are
// literal text. Routing "<=" into the tag reader swallowed the real close tag
// that followed and nested every later sibling inside the box - which is how
// one "<=" in a media-queries parity fixture collapsed the whole lower page
// into a single div.
func TestLessThanFollowedByNonLetterIsText(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body>
<div class="a">viewport <= 500px</div>
<div class="b">after</div>
</body></html>`)

	a := find(&doc.Node, "a")
	if a == nil {
		t.Fatal("no .a element found")
	}
	if txt := a.TextContent(); !strings.Contains(txt, "viewport <= 500px") {
		t.Errorf(".a text = %q, want it to keep the literal \"<=\"", txt)
	}
	b := find(&doc.Node, "b")
	if b == nil {
		t.Fatal("no .b element found")
	}
	if b.Parent != a.Parent {
		t.Error(".b is not a sibling of .a (it nested inside after \"<=\")")
	}
	if txt := b.TextContent(); txt != "after" {
		t.Errorf(".b text = %q, want %q", txt, "after")
	}
}

// TestBareLessThanAtEndIsText pins the end-of-input half of the same state:
// a "<" with nothing after it is text, not a tag that eats the document.
func TestBareLessThanAtEndIsText(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body><div class="a">trailing <</div></body></html>`)
	a := find(&doc.Node, "a")
	if a == nil {
		t.Fatal("no .a element found")
	}
	if txt := a.TextContent(); !strings.Contains(txt, "trailing <") {
		t.Errorf(".a text = %q, want the trailing \"<\" kept as text", txt)
	}
}

// TestImpliedHtmlElementExists pins the always-there html element: a document
// that omits <html> still grows one, so `html { background: ... }` matches and
// canvas resolution sees it. Before the seed, Doc.HTML stayed nil and the
// body's background wrongly won the canvas.
func TestImpliedHtmlElementExists(t *testing.T) {
	doc := domtest.Parse(`<!DOCTYPE html><title>t</title><body><p>x</p></body>`)
	if doc.HTML == nil || doc.HTML.Data != "html" {
		t.Fatalf("Doc.HTML = %v, want the implied html element", doc.HTML)
	}
	if doc.Body == nil || doc.Head == nil {
		t.Errorf("Doc.Head = %v Doc.Body = %v, want both implied", doc.Head, doc.Body)
	}
}

// TestExplicitHtmlAttrsMerge pins attribute merging: an explicit <html> tag
// keeps contributing lang/class onto the seeded element instead of being
// dropped (or duplicating the element).
func TestExplicitHtmlAttrsMerge(t *testing.T) {
	doc := domtest.Parse(`<!DOCTYPE html><html lang="en" class="a"><body><p>x</p></body></html>`)
	if doc.HTML == nil {
		t.Fatal("no html element")
	}
	if doc.HTML.GetAttribute("lang") != "en" {
		t.Errorf("lang = %q, want en", doc.HTML.GetAttribute("lang"))
	}
	count := 0
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() && c.Data == "html" {
				count++
			}
			walk(c)
		}
	}
	walk(&doc.Node)
	if count != 1 {
		t.Errorf("found %d html elements, want exactly 1", count)
	}
}
