package style

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

func TestResolvePseudoElements(t *testing.T) {
	html := `<html><body><div class="test">hello</div></body></html>`
	cssText := `.test::before { content: "\25b3"; color: red; }`

	doc := dom.Parse(html)
	sheets := []*css.Stylesheet{css.Parse(cssText)}
	styles := Resolve(doc, sheets)

	pseudoStyles := ResolvePseudoElements(doc, sheets, styles)

	if len(pseudoStyles) == 0 {
		t.Fatal("expected pseudo-element styles, got none")
	}

	// Find the div element
	var findDiv func(n *dom.Node) *dom.Node
	findDiv = func(n *dom.Node) *dom.Node {
		for c := n; c != nil; c = c.NextSibling {
			if c.Element() && c.Data == "div" {
				return c
			}
			if found := findDiv(c.FirstChild); found != nil {
				return found
			}
		}
		return nil
	}
	divNode := findDiv(doc.Node.FirstChild)
	if divNode == nil {
		t.Fatal("div not found")
	}

	key := PseudoKey{NodeID: divNode.ID, Pseudo: "before"}
	cs, ok := pseudoStyles[key]
	if !ok {
		t.Fatalf("expected ::before style for div, got %v", pseudoStyles)
	}

	if cs.Content != "△" {
		t.Errorf("expected content △, got %q", cs.Content)
	}
}
