package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func findRowNode(doc *dom.Document) *dom.Node {
	var found *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if found != nil {
			return
		}
		if n.Element() && n.GetAttribute("class") == "row" {
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

// The four original pseudo-elements keep working single-colon spellings
// (CSS 2.1 §5.12.3): :after must match exactly what ::after matches, or
// every legacy clearfix silently stops generating its box.
func TestSingleColonPseudoElements(t *testing.T) {
	doc := domtest.Parse(`<html><body><div class="row"><p>x</p></div></body></html>`)
	node := findRowNode(doc)
	if node == nil {
		t.Fatal("row not found")
	}
	for _, selStr := range []string{".row:after", ".row::after"} {
		sel := css.ParseSelector(selStr)
		if !sel.MatchesPseudoElement(node, "after") {
			t.Errorf("%s does not match ::after", selStr)
		}
	}
	for _, selStr := range []string{".row:before", ".row::before"} {
		sel := css.ParseSelector(selStr)
		if !sel.MatchesPseudoElement(node, "before") {
			t.Errorf("%s does not match ::before", selStr)
		}
	}
	// A real pseudo-class must not be mistaken for an element.
	sel := css.ParseSelector(".row:hover")
	if sel.MatchesPseudoElement(node, "hover") {
		t.Error(".row:hover matched as pseudo-element")
	}
}
