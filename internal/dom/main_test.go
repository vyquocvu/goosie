package dom

import (
	"testing"
)

func TestMainElementInBody(t *testing.T) {
	html := "<html><body><main>content</main></body></html>"
	doc, err := ParseBounded(html, ParseLimits{Nodes: 100, Depth: 10, Attributes: 10, AttributeBytes: 100})
	if err != nil {
		t.Fatal(err)
	}

	var main *Node
	var visit func(*Node)
	visit = func(n *Node) {
		if n.Type == NodeElement && n.Data == "main" {
			main = n
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(&doc.Node)

	if main == nil {
		t.Fatal("main element not found")
	}

	if main.Parent != doc.Body {
		t.Errorf("main parent = %v, want document.body", main.Parent)
	}
}
