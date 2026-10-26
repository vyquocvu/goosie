package dom_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// buildIDDoc parses a real document, so Doc back-pointers are intact and the
// ElementByID index path (not the degenerate-tree fallback) is exercised.
func buildIDDoc(t *testing.T, body string) *dom.Document {
	t.Helper()
	doc, err := dom.ParseBounded("<html><body>"+body+"</body></html>", dom.ParseLimits{
		Nodes: 100000, Depth: 128, Attributes: 64, AttributeBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestElementByIDIndexBasic(t *testing.T) {
	doc := buildIDDoc(t, `<p id="a">1</p><p id="b">2</p>`)
	if got := doc.ElementByID("a"); got == nil || got.Data != "p" {
		t.Fatalf("ElementByID(a) = %v", got)
	}
	if doc.ElementByID("missing") != nil {
		t.Fatal("ElementByID(missing) should be nil")
	}
}

func TestElementByIDIndexDuplicateFirstWins(t *testing.T) {
	doc := buildIDDoc(t, `<p id="x">first</p><p id="x">second</p>`)
	got := doc.ElementByID("x")
	if got == nil || got.FirstChild == nil || got.FirstChild.DataContent != "first" {
		t.Fatalf("duplicate id should return first element, got %v", got)
	}
}

func TestElementByIDIndexRemoveChild(t *testing.T) {
	doc := buildIDDoc(t, `<div><p id="gone">x</p></div>`)
	p := doc.ElementByID("gone")
	if p == nil {
		t.Fatal("setup: not found")
	}
	p.Parent.RemoveChild(p)
	if doc.ElementByID("gone") != nil {
		t.Fatal("stale index: removed element still found")
	}
}

func TestElementByIDIndexRemoveIDAttr(t *testing.T) {
	doc := buildIDDoc(t, `<p id="a">x</p>`)
	p := doc.ElementByID("a")
	p.RemoveAttribute("id")
	if doc.ElementByID("a") != nil {
		t.Fatal("stale index: element found after id removal")
	}
}

func TestElementByIDIndexSetID(t *testing.T) {
	doc := buildIDDoc(t, `<p id="a">x</p><p>plain</p>`)
	plain := doc.Body.FirstChild.NextSibling
	if plain == nil || plain.Data != "p" {
		t.Fatal("setup: second p missing")
	}
	plain.SetAttribute("id", "b")
	if doc.ElementByID("b") != plain {
		t.Fatal("new id not found")
	}
	plain.SetAttribute("id", "c")
	if doc.ElementByID("b") != nil {
		t.Fatal("stale index: old id still found")
	}
	if doc.ElementByID("c") != plain {
		t.Fatal("renamed id not found")
	}
}

func TestElementByIDIndexMoveBetweenParents(t *testing.T) {
	doc := buildIDDoc(t, `<div id="d1"><p id="m">x</p></div><div id="d2"></div>`)
	m := doc.ElementByID("m")
	d2 := doc.ElementByID("d2")
	d2.AppendChild(m)
	if doc.ElementByID("m") != m {
		t.Fatal("moved element not found")
	}
	if m.Parent != d2 {
		t.Fatal("move did not relink")
	}
}

func TestElementByIDIndexAppendNew(t *testing.T) {
	doc := buildIDDoc(t, `<div id="host"></div>`)
	host := doc.ElementByID("host")
	n := doc.NewElement("span")
	n.SetAttribute("id", "late")
	host.AppendChild(n)
	if doc.ElementByID("late") != n {
		t.Fatal("appended element not found")
	}
}

func TestElementByIDIndexManyQueries(t *testing.T) {
	var body string
	for i := 0; i < 2000; i++ {
		body += fmt.Sprintf("<p id=\"p%d\">%d</p>", i, i)
	}
	doc := buildIDDoc(t, body)
	for i := 0; i < 2000; i += 7 {
		id := fmt.Sprintf("p%d", i)
		if got := doc.ElementByID(id); got == nil {
			t.Fatalf("ElementByID(%s) = nil", id)
		}
	}
	// Mutate in the middle, then verify both sides.
	mid := doc.ElementByID("p1000")
	mid.Parent.RemoveChild(mid)
	if doc.ElementByID("p1000") != nil {
		t.Fatal("stale after remove")
	}
	if doc.ElementByID("p1001") == nil || doc.ElementByID("p999") == nil {
		t.Fatal("neighbors lost after remove")
	}
}

func TestQueryIndexTagMutation(t *testing.T) {
	doc := buildIDDoc(t, `<div><p>a</p><p>b</p></div>`)
	if got := doc.ElementsByTagName("p"); len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// Append under a different parent, remove another, re-query.
	div := doc.Body.FirstChild
	n := doc.NewElement("p")
	div.AppendChild(n)
	if got := doc.ElementsByTagName("p"); len(got) != 3 {
		t.Fatalf("after append len = %d, want 3", len(got))
	}
	div.RemoveChild(div.FirstChild)
	got := doc.ElementsByTagName("p")
	if len(got) != 2 {
		t.Fatalf("after remove len = %d, want 2", len(got))
	}
}

func TestQueryIndexTagOrderAndUnknown(t *testing.T) {
	doc := buildIDDoc(t, `<div><p id="a">1</p><span>2</span><p id="b">3</p></div>`)
	ps := doc.ElementsByTagName("p")
	if len(ps) != 2 || ps[0].GetAttribute("id") != "a" || ps[1].GetAttribute("id") != "b" {
		t.Fatal("tag order not document order")
	}
	if got := doc.ElementsByTagName("nosuchtag"); len(got) != 0 {
		t.Fatalf("unknown tag len = %d, want 0", len(got))
	}
}

func TestQueryIndexClassMutation(t *testing.T) {
	doc := buildIDDoc(t, `<p class="a b">1</p><p class="b">2</p><p>3</p>`)
	if got := doc.ElementsByClassName("b"); len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got := doc.ElementsByClassName("a"); len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	// Retarget via SetAttribute.
	plain := doc.Body.FirstChild.NextSibling.NextSibling
	plain.SetAttribute("class", "a")
	if got := doc.ElementsByClassName("a"); len(got) != 2 {
		t.Fatalf("after set len = %d, want 2", len(got))
	}
	// Remove the attribute entirely.
	first := doc.Body.FirstChild
	first.RemoveAttribute("class")
	if got := doc.ElementsByClassName("a"); len(got) != 1 {
		t.Fatalf("after remove len = %d, want 1", len(got))
	}
	if got := doc.ElementsByClassName(""); len(got) != 0 {
		t.Fatalf("empty class len = %d, want 0", len(got))
	}
}

func TestQueryIndexDuplicateClassOnce(t *testing.T) {
	doc := buildIDDoc(t, `<p class="x x x">1</p>`)
	if got := doc.ElementsByClassName("x"); len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
}

func TestJSQueryContainment(t *testing.T) {
	doc := buildIDDoc(t, `<div id="outer"><p class="x">1</p><div id="inner"><p class="x">2</p><span id="s">3</span></div></div><p class="x">4</p>`)
	r, err := newJSRT(doc)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	run := func(script string) string {
		t.Helper()
		if err := r.Run(script, "inline"); err != nil {
			t.Fatal(err)
		}
		v, ok := r.Global("out")
		if !ok {
			t.Fatal("no out")
		}
		return v.Str
	}
	// Whole-document indexed queries. p.x is 3, measured in Chromium
	// 153.0.8010.12 on this fixture (2 under #inner, 1 after #outer). This
	// line used to expect 0/3 and pin the "compound unsupported" answer;
	// TASK-051 routes compounds through css.Selector, so the measured answer
	// is the right one.
	if got := run(`var out = document.querySelectorAll("p.x").length + "/" + document.querySelectorAll("p").length;`); got != "3/3" {
		t.Fatalf("qsa = %q, want 3/3 (compound p.x and tag p both 3)", got)
	}
	if got := run(`var out = document.querySelector("#s").tagName;`); got != "SPAN" {
		t.Fatalf("qs #s = %q", got)
	}
	// Element-rooted: must not escape the subtree.
	if got := run(`var out = "" + document.querySelectorAll.call(document.querySelector("#inner"), "p").length;`); got != "1" {
		t.Fatalf("rooted qsa = %q, want 1", got)
	}
	if got := run(`var out = document.querySelector.call(document.querySelector("#inner"), "span").id;`); got != "s" {
		t.Fatalf("rooted qs = %q, want s", got)
	}
	// Class singular rooted.
	if got := run(`var out = document.querySelector.call(document.querySelector("#outer"), ".x").textContent;`); got != "1" {
		t.Fatalf("rooted class qs = %q, want 1", got)
	}
	// Root itself excluded even when it matches.
	if got := run(`var out = document.querySelector.call(document.querySelector("#inner"), "#inner");`); got != "null" && got != "" {
		t.Fatalf("self qs = %q, want null", got)
	}
}

// newJSRT builds a runtime bound to doc.
func newJSRT(doc *dom.Document) (*js.Runtime, error) {
	return js.New(js.Options{URL: "https://example.test/", DOM: doc})
}
