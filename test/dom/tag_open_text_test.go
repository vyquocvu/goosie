package dom_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

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

func TestImpliedHtmlElementExists(t *testing.T) {
	doc := domtest.Parse(`<!DOCTYPE html><title>t</title><body><p>x</p></body>`)
	if doc.HTML == nil || doc.HTML.Data != "html" {
		t.Fatalf("Doc.HTML = %v, want the implied html element", doc.HTML)
	}
	if doc.Body == nil || doc.Head == nil {
		t.Errorf("Doc.Head = %v Doc.Body = %v, want both implied", doc.Head, doc.Body)
	}
}

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

func TestVoidEndTagsIgnored(t *testing.T) {
	doc := domtest.Parse(`<table><col></col><col></col><td></td></table>`)
	var table, colCount int
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() {
				if c.Data == "table" {
					table++
				}
				if c.Data == "col" {
					colCount++
				}
			}
			walk(c)
		}
	}
	walk(&doc.Node)
	if table != 1 || colCount != 2 {
		t.Errorf("table=%d cols=%d, want 1 table holding both cols", table, colCount)
	}
	var findTdParent func(n *dom.Node) string
	findTdParent = func(n *dom.Node) string {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() && c.Data == "td" {
				if p := c.Parent; p != nil {
					return p.Data
				}
			}
			if got := findTdParent(c); got != "" {
				return got
			}
		}
		return ""
	}
	if got := findTdParent(&doc.Node); got != "tr" {
		t.Errorf("td parent = %q, want tr (inside implied tbody)", got)
	}
}
