package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

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

	if cs.FontWeight != style.WeightBold {
		t.Errorf("expected font-weight bold, got %v", cs.FontWeight)
	}
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

func TestFirstLineLineHeightResolvesAgainstFinalFontSize(t *testing.T) {
	cases := []struct {
		name        string
		parent      string
		declaration string
		want        float32
	}{
		{"ratio-before-size", "", "line-height: 1.5; font-size: 32px;", 48},
		{"em-before-size", "", "line-height: 1.5em; font-size: 32px;", 48},
		{"inherited-ratio", "line-height: 1.5;", "font-size: 32px;", 48},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := domtest.Parse(`<html><body><p id="target">Hello</p></body></html>`)
			sheets := []*css.Stylesheet{css.Parse(`p { font-size: 16px; ` + tc.parent + ` } p::first-line { ` + tc.declaration + ` }`)}
			styles := style.Resolve(doc, sheets, nil)
			pseudoStyles := style.ResolvePseudoElements(doc, sheets, styles, nil)
			p := doc.ElementByID("target")
			if p == nil {
				t.Fatal("p not found")
			}
			cs := pseudoStyles[style.PseudoKey{NodeID: p.ID, Pseudo: "first-line"}]
			if cs == nil {
				t.Fatal("first-line style not found")
			}
			if cs.FontSize != 32 || cs.LineHeight != tc.want {
				t.Fatalf("font-size = %v, line-height = %v, want 32, %v", cs.FontSize, cs.LineHeight, tc.want)
			}
		})
	}
}

func TestFirstLineMetricOverlayPreservesNestedOwnDeclarations(t *testing.T) {
	doc := domtest.Parse(`<html><body><p id="target"><span id="nested">Alpha</span> beta</p></body></html>`)
	sheets := []*css.Stylesheet{css.Parse(`
		p { font: 16px/20px Arial, sans-serif; }
		p::first-line {
			font-family: Arial, sans-serif;
			font-size: 32px;
			font-weight: bold;
			font-style: italic;
			line-height: 1.5;
			letter-spacing: 2px;
			word-spacing: 3px;
		}
		#nested { font-size: 20px; font-weight: 700; }
	`)}
	styles := style.Resolve(doc, sheets, nil)
	pseudoStyles := style.ResolvePseudoElements(doc, sheets, styles, nil)
	target := doc.ElementByID("target")
	nested := doc.ElementByID("nested")
	firstLine := pseudoStyles[style.PseudoKey{NodeID: target.ID, Pseudo: "first-line"}]
	got := style.FirstLineMetrics(styles[nested.ID], firstLine)
	if got == styles[nested.ID] {
		t.Fatal("expected an overlaid style")
	}
	if got.FontSize != 20 || got.FontWeight != style.WeightBold {
		t.Fatalf("nested font = %v/%v, want 20/700", got.FontSize, got.FontWeight)
	}
	if got.FontFamily != firstLine.FontFamily || got.FontStyle != firstLine.FontStyle {
		t.Fatalf("pseudo font family/style = %v/%v, want %v/%v", got.FontFamily, got.FontStyle, firstLine.FontFamily, firstLine.FontStyle)
	}
	if got.LineHeight != 30 || got.LetterSpacing != 2 || got.WordSpacing != 3 {
		t.Fatalf("pseudo metrics = line-height %v, letter-spacing %v, word-spacing %v, want 30/2/3", got.LineHeight, got.LetterSpacing, got.WordSpacing)
	}
}

func TestFirstLineMetricOverlayReturnsOriginalWhenUnchanged(t *testing.T) {
	cases := []struct {
		name  string
		rules string
	}{
		{"unspecified", `p::first-line { color: red; }`},
		{"same-value", `p::first-line { font-size: 16px; }`},
		{"inherit", `p::first-line { font-family: inherit; font-size: inherit; font-weight: inherit; font-style: inherit; line-height: inherit; letter-spacing: inherit; word-spacing: inherit; }`},
		{"unset", `p::first-line { font-family: unset; font-size: unset; font-weight: unset; font-style: unset; line-height: unset; letter-spacing: unset; word-spacing: unset; }`},
		{"font-inherit", `p::first-line { font: inherit; }`},
		{"font-unset", `p::first-line { font: unset; }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := domtest.Parse(`<html><body><p id="target">Alpha</p></body></html>`)
			sheets := []*css.Stylesheet{css.Parse(`p { font-size: 16px; } ` + tc.rules)}
			styles := style.Resolve(doc, sheets, nil)
			pseudoStyles := style.ResolvePseudoElements(doc, sheets, styles, nil)
			target := doc.ElementByID("target")
			firstLine := pseudoStyles[style.PseudoKey{NodeID: target.ID, Pseudo: "first-line"}]
			base := styles[target.FirstChild.ID]
			if tc.name != "same-value" && firstLine.FirstLineSpecified() != 0 {
				t.Fatalf("unspecified mask = %d, want 0", firstLine.FirstLineSpecified())
			}
			if got := style.FirstLineMetrics(base, firstLine); got != base {
				t.Fatal("unchanged metrics returned a new style pointer")
			}
		})
	}
}

func TestFirstLineInitialMetricsSpecifyAndReset(t *testing.T) {
	doc := domtest.Parse(`<html><body><p id="target" style="font: italic bold 32px/48px Arial; letter-spacing: 2px; word-spacing: 3px">Alpha</p></body></html>`)
	sheets := []*css.Stylesheet{css.Parse(`p::first-line { font-family: initial; font-size: initial; font-weight: initial; font-style: initial; line-height: initial; letter-spacing: initial; word-spacing: initial; }`)}
	styles := style.Resolve(doc, sheets, nil)
	pseudoStyles := style.ResolvePseudoElements(doc, sheets, styles, nil)
	target := doc.ElementByID("target")
	firstLine := pseudoStyles[style.PseudoKey{NodeID: target.ID, Pseudo: "first-line"}]
	if firstLine.FirstLineSpecified() == 0 {
		t.Fatal("initial declarations did not specify first-line metrics")
	}
	if firstLine.FontSize != 16 || firstLine.FontWeight != style.WeightNormal || firstLine.FontStyle != style.FontStyleNormal ||
		firstLine.LineHeight != -1 || firstLine.LetterSpacing != 0 || firstLine.WordSpacing != 0 {
		t.Fatalf("initial metrics were not reset: %+v", firstLine)
	}
}

func pseudoKeys(m map[style.PseudoKey]*style.ComputedStyle) []style.PseudoKey {
	keys := make([]style.PseudoKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
