package dom_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

// TestHeadLevelElementInBodyDoesNotSwallowContent pins HTML5's insertion point
// for style/link/meta/title. Those tags belong in a head only while the head is
// the live insertion point; after the body has begun they are inserted where
// they appear. Synthesizing a head there leaves it open on the stack, so every
// later element becomes a child of a display:none subtree and the whole page
// renders blank - not a wrong colour or a shifted box, but nothing at all.
func TestHeadLevelElementInBodyDoesNotSwallowContent(t *testing.T) {
	cases := []struct{ name, html string }{
		{"style in body, no head tag", `<html><body><style>.x{color:red}</style><div class="content">A</div></body></html>`},
		{"link in body, no head tag", `<html><body><link rel="stylesheet" href="a.css"><div class="content">A</div></body></html>`},
		{"meta in body, no head tag", `<html><body><meta charset="utf-8"><div class="content">A</div></body></html>`},
		{"body tag with an unclosed head", `<html><head><style>.x{color:red}</style><body><div class="content">A</div></body></html>`},
		{"head closed, content after it", `<html><head></head><link rel="stylesheet" href="a.css"><div class="content">A</div></body></html>`},
		{"title in body", `<html><body><title>t</title><div class="content">A</div></body></html>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := dom.Parse(c.html)
			div := find(&doc.Node, "content")
			if div == nil {
				t.Fatalf("content div is missing; tree = %s", strings.Join(tags(&doc.Node), " "))
			}
			for a := div; a != nil; a = a.Parent {
				if a == doc.Head {
					t.Fatalf("content div is nested inside <head> (display:none); tree = %s", strings.Join(tags(&doc.Node), " "))
				}
			}
			if doc.Body == nil || div.Parent != doc.Body {
				got := "<nil>"
				if div.Parent != nil {
					got = div.Parent.Data
				}
				t.Errorf("content div's parent = %s, want body", got)
			}
			if doc.HTML != nil && doc.Body != nil && doc.Body.Parent != doc.HTML {
				got := "<nil>"
				if doc.Body.Parent != nil {
					got = doc.Body.Parent.Data
				}
				t.Errorf("<body>'s parent = %s, want html - the whole stack was emptied by an unconditional pop", got)
			}
		})
	}
}

// TestHeadKeepsContentBeforeTheBody is the other half of the pin: a head-level
// element before any body content must still land in the head, so a document's
// own <style> does not become a rendered child of the body.
func TestHeadKeepsContentBeforeTheBody(t *testing.T) {
	doc := dom.Parse(`<html><head><style>.x{color:red}</style><link rel="stylesheet" href="a.css"></head><body><div class="content">A</div></body></html>`)
	if doc.Head == nil {
		t.Fatal("no head element")
	}
	for _, tag := range []string{"style", "link"} {
		found := false
		for _, k := range doc.Head.ChildElements() {
			if k.Data == tag {
				found = true
			}
		}
		if !found {
			t.Errorf("<%s> is not a child of head; head children = %s", tag, childTags(doc.Head))
		}
	}
	div := find(&doc.Node, "content")
	if div == nil || div.Parent != doc.Body {
		t.Errorf("content div is not a child of body")
	}
}

func tags(n *dom.Node) []string {
	var out []string
	var walk func(*dom.Node, int)
	walk = func(x *dom.Node, d int) {
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() {
				indent := strings.Repeat(".", d+1)
				out = append(out, indent+c.Data)
				walk(c, d+1)
			}
		}
	}
	walk(n, 0)
	return out
}

func childTags(n *dom.Node) string {
	var out []string
	for _, c := range n.ChildElements() {
		out = append(out, c.Data)
	}
	return strings.Join(out, " ")
}
