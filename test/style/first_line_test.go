package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// TestFirstLineStyleResolved verifies that ::first-line styles are computed
// and stored in the pseudo-element map.
func TestFirstLineStyleResolved(t *testing.T) {
	html := `<html><body><p class="test">Hello world</p></body></html>`
	cssText := `p.test::first-line { font-weight: bold; color: red; }`

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

	key := style.PseudoKey{NodeID: pNode.ID, Pseudo: "first-line"}
	cs, ok := pseudoStyles[key]
	if !ok {
		t.Fatalf("expected ::first-line style for p, got keys: %v", pseudoKeys(pseudoStyles))
	}
	if cs.FontWeight != style.WeightBold {
		t.Errorf("expected font-weight bold (700), got %v", cs.FontWeight)
	}
}

// TestFirstLineDisallowedProperties verifies that margin, padding, and border
// properties are filtered out of ::first-line styles.
func TestFirstLineDisallowedProperties(t *testing.T) {
	html := `<html><body><p class="test">Hello</p></body></html>`
	cssText := `p.test::first-line {
		font-weight: bold;
		margin-top: 50px;
		padding-left: 20px;
		border-top-width: 5px;
	}`

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

	key := style.PseudoKey{NodeID: pNode.ID, Pseudo: "first-line"}
	cs, ok := pseudoStyles[key]
	if !ok {
		t.Fatal("expected ::first-line style")
	}

	// Allowed property should survive.
	if cs.FontWeight != style.WeightBold {
		t.Errorf("expected font-weight bold, got %v", cs.FontWeight)
	}
	// Disallowed properties should be zeroed.
	if cs.MarginTop != 0 {
		t.Errorf("expected margin-top 0 (filtered), got %v", cs.MarginTop)
	}
	if cs.PaddingLeft != 0 {
		t.Errorf("expected padding-left 0 (filtered), got %v", cs.PaddingLeft)
	}
	if cs.BorderTopWidth != 0 {
		t.Errorf("expected border-top-width 0 (filtered), got %v", cs.BorderTopWidth)
	}
}

// TestFirstLineAllowedProperties verifies that all CSS-allowed properties
// survive the filter on ::first-line.
func TestFirstLineAllowedProperties(t *testing.T) {
	html := `<html><body><p>Hello</p></body></html>`
	cssText := `p::first-line {
		font-size: 24px;
		font-style: italic;
		color: blue;
		background-color: yellow;
		text-decoration: underline;
		text-transform: uppercase;
		letter-spacing: 2px;
		word-spacing: 4px;
		line-height: 30px;
	}`

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

	key := style.PseudoKey{NodeID: pNode.ID, Pseudo: "first-line"}
	cs, ok := pseudoStyles[key]
	if !ok {
		t.Fatal("expected ::first-line style")
	}

	if cs.FontSize != 24 {
		t.Errorf("font-size: got %v, want 24", cs.FontSize)
	}
	if cs.FontStyle != style.FontStyleItalic {
		t.Errorf("font-style: got %v, want italic", cs.FontStyle)
	}
	if cs.TextTransform != "uppercase" {
		t.Errorf("text-transform: got %q, want uppercase", cs.TextTransform)
	}
	if cs.LetterSpacing != 2 {
		t.Errorf("letter-spacing: got %v, want 2", cs.LetterSpacing)
	}
	if cs.WordSpacing != 4 {
		t.Errorf("word-spacing: got %v, want 4", cs.WordSpacing)
	}
	if cs.LineHeight != 30 {
		t.Errorf("line-height: got %v, want 30", cs.LineHeight)
	}
}

// TestFirstLineNoRules verifies that elements without ::first-line rules
// produce no pseudo-style entry.
func TestFirstLineNoRules(t *testing.T) {
	html := `<html><body><p>Hello</p></body></html>`
	cssText := `p { color: red; }`

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

	key := style.PseudoKey{NodeID: pNode.ID, Pseudo: "first-line"}
	if _, ok := pseudoStyles[key]; ok {
		t.Error("expected no ::first-line style when no rules match")
	}
}

// pseudoKeys returns the keys of a pseudo-style map for error messages.
func pseudoKeys(m map[style.PseudoKey]*style.ComputedStyle) []style.PseudoKey {
	keys := make([]style.PseudoKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
