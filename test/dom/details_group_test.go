package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func detailsByID(doc *dom.Document, id string) *dom.Node {
	node := doc.ElementByID(id)
	if node == nil {
		panic("missing element " + id)
	}
	return node
}

func TestCloseOtherNamedDetailsClosesSameNamePeers(t *testing.T) {
	doc := domtest.Parse(`<html><body>
<details id="d1" name="faq" open><summary>Q1</summary></details>
<details id="d2" name="faq"><summary>Q2</summary></details>
<details id="d3" name="other" open><summary>Q3</summary></details>
<details id="d4" open><summary>Q4</summary></details>
</body></html>`)

	d1, d2, d3, d4 := detailsByID(doc, "d1"), detailsByID(doc, "d2"), detailsByID(doc, "d3"), detailsByID(doc, "d4")

	d2.SetAttribute("open", "")
	second := dom.CloseOtherNamedDetails(d2)
	if len(second) != 1 || second[0] != d1 {
		t.Fatalf("closing d2 returned %v, want exactly d1", second)
	}
	if d1.HasAttribute("open") {
		t.Error("d1 stayed open after the named peer d2 opened")
	}
	if !d2.HasAttribute("open") {
		t.Error("d2 must keep its own open state")
	}
	if !d3.HasAttribute("open") || !d4.HasAttribute("open") {
		t.Error("a different name group and an unnamed details must not close")
	}

	// The "other" group has no second member, so opening d3 closes nothing.
	if closed := dom.CloseOtherNamedDetails(d3); len(closed) != 0 {
		t.Fatalf("opening d3 closed %v, want no peers", closed)
	}
	if !d2.HasAttribute("open") {
		t.Error("d2 belongs to another name group and must stay open")
	}
}

func TestCloseOtherNamedDetailsKeepsShadowTreesSeparate(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host"></div><details id="light" name="scope" open></details></body></html>`)
	host := detailsByID(doc, "host")
	shadow := &dom.ShadowRoot{Host: host, Mode: "open"}
	host.ShadowRoot = shadow
	inner := doc.NewElement("details")
	inner.SetAttribute("name", "scope")
	inner.SetAttribute("open", "")
	shadow.AppendChild(inner)

	if closed := dom.CloseOtherNamedDetails(inner); len(closed) != 0 {
		t.Fatalf("shadow member closed %v, want the shadow tree to be its own group", closed)
	}
	light := detailsByID(doc, "light")
	if !light.HasAttribute("open") {
		t.Error("a shadow-tree details must not close its same-named light-DOM peer")
	}
}

func TestCloseOtherNamedDetailsIgnoresUnnamedAndDetached(t *testing.T) {
	doc := domtest.Parse(`<html><body><details id="a" open></details><details id="b"></details><details id="c" name="faq" open></details></body></html>`)
	a, b := detailsByID(doc, "a"), detailsByID(doc, "b")
	if closed := dom.CloseOtherNamedDetails(b); len(closed) != 0 {
		t.Fatalf("unnamed details closed %v, want the name-group rule skipped", closed)
	}
	if !a.HasAttribute("open") {
		t.Error("an unnamed details must not enforce name-group exclusivity")
	}

	// A detached subtree is its own tree: its open same-named descendant is
	// closed, and nothing in the document is touched.
	detached := doc.NewElement("details")
	detached.SetAttribute("name", "faq")
	detached.SetAttribute("open", "")
	peer := doc.NewElement("details")
	peer.SetAttribute("name", "faq")
	peer.SetAttribute("open", "")
	detached.AppendChild(peer)
	if closed := dom.CloseOtherNamedDetails(detached); len(closed) != 1 || closed[0] != peer {
		t.Fatalf("detached root closed %v, want exactly its open descendant", closed)
	}
	if peer.HasAttribute("open") {
		t.Error("the detached subtree peer stayed open")
	}
	if c := detailsByID(doc, "c"); !c.HasAttribute("open") {
		t.Error("a detached mutation must not close a document member of the same name group")
	}
}
