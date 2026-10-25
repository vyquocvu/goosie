package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestResolveFirstLetterPseudoElement(t *testing.T) {
	html := `<html><body><p class="test">Hello</p></body></html>`
	cssText := `p.test::first-letter { font-size: 48px; }`

	doc := domtest.Parse(html)
	sheets := []*css.Stylesheet{css.Parse(cssText)}
	styles := style.Resolve(doc, sheets, nil)

	pseudoStyles := style.ResolvePseudoElements(doc, sheets, styles, nil)

	var pNode *dom.Node
	var findP func(n *dom.Node)
	findP = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() && n.Data == "p" {
			pNode = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findP(c)
		}
	}
	findP(&doc.Node)
	if pNode == nil {
		t.Fatal("p not found")
	}

	key := style.PseudoKey{NodeID: pNode.ID, Pseudo: "first-letter"}
	cs, ok := pseudoStyles[key]
	if !ok {
		t.Fatalf("expected ::first-letter style for p, got map: %v", pseudoStyles)
	}
	if cs.FontSize != 48 {
		t.Errorf("expected font-size 48, got %v", cs.FontSize)
	}
}
