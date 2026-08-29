package renderer

import (
	"image/color"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestStyleSpecificitySorting(t *testing.T) {
	// Rule 1 has higher specificity (tag + class = [0, 1, 1]) and is declared earlier.
	// Rule 2 has lower specificity (tag = [0, 0, 1]) and is declared later.
	// Rule 1 should win for font-size.
	content := `<html><head><style>
		p.special { font-size: 24px; color: red; }
		p { font-size: 14px; color: blue; }
	</style></head><body>
		<p class="special" id="target">Hello</p>
	</body></html>`

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	target := findNodeByTag(renderTree, "p")
	if target == nil {
		t.Fatal("target node not found")
	}

	// Higher specificity p.special (24px, red) should beat later p (14px, blue)
	if target.ComputedStyle.FontSize != 24.0 {
		t.Errorf("Expected font-size 24.0, got %f", target.ComputedStyle.FontSize)
	}
	expectedColor := color.RGBA{R: 0xff, A: 0xff}
	if target.ComputedStyle.Color != expectedColor {
		t.Errorf("Expected color %v, got %v", expectedColor, target.ComputedStyle.Color)
	}
}

func TestStyleImportantDeclarationPrecedence(t *testing.T) {
	// Rule 1 has lower specificity (tag) but has !important.
	// Rule 2 has higher specificity (id + class) but normal declaration.
	// Rule 1 should win for color.
	content := `<html><head><style>
		p { color: green !important; }
		#target.special { color: red; }
	</style></head><body>
		<p class="special" id="target">Hello</p>
	</body></html>`

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	target := findNodeByTag(renderTree, "p")
	if target == nil {
		t.Fatal("target node not found")
	}

	expectedColor := color.RGBA{G: 0x80, A: 0xff} // green
	if target.ComputedStyle.Color != expectedColor {
		t.Errorf("Expected !important green color %v, got %v", expectedColor, target.ComputedStyle.Color)
	}
}

func TestFontShorthandParsing(t *testing.T) {
	content := `<html><head><style>
		body { font: 18px/1.5 sans-serif; }
		h1 { font: italic bold 24px/1.2 Arial, sans-serif; }
		p.small { font: 400 12px monospace; }
	</style></head><body>
		<h1>Heading</h1>
		<p class="small">Code</p>
	</body></html>`

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	body := findNodeByTag(renderTree, "body")
	if body == nil {
		t.Fatal("body not found")
	}
	if body.ComputedStyle.FontSize != 18.0 {
		t.Errorf("Expected body font-size 18.0, got %f", body.ComputedStyle.FontSize)
	}
	if body.ComputedStyle.LineHeight != 27.0 {
		t.Errorf("Expected body line-height 27.0 (18 * 1.5), got %f", body.ComputedStyle.LineHeight)
	}
	if body.ComputedStyle.FontFamily != "sans-serif" {
		t.Errorf("Expected body font-family 'sans-serif', got '%s'", body.ComputedStyle.FontFamily)
	}

	h1 := findNodeByTag(renderTree, "h1")
	if h1 == nil {
		t.Fatal("h1 not found")
	}
	if h1.ComputedStyle.FontStyle != "italic" {
		t.Errorf("Expected h1 font-style 'italic', got '%s'", h1.ComputedStyle.FontStyle)
	}
	if h1.ComputedStyle.FontWeight != "bold" {
		t.Errorf("Expected h1 font-weight 'bold', got '%s'", h1.ComputedStyle.FontWeight)
	}
	if h1.ComputedStyle.FontSize != 24.0 {
		t.Errorf("Expected h1 font-size 24.0, got %f", h1.ComputedStyle.FontSize)
	}
	if h1.ComputedStyle.FontFamily != "Arial, sans-serif" {
		t.Errorf("Expected h1 font-family 'Arial, sans-serif', got '%s'", h1.ComputedStyle.FontFamily)
	}

	p := findNodeByTag(renderTree, "p")
	if p == nil {
		t.Fatal("p not found")
	}
	if p.ComputedStyle.FontWeight != "400" {
		t.Errorf("Expected p font-weight '400', got '%s'", p.ComputedStyle.FontWeight)
	}
	if p.ComputedStyle.FontSize != 12.0 {
		t.Errorf("Expected p font-size 12.0, got %f", p.ComputedStyle.FontSize)
	}
	if p.ComputedStyle.FontFamily != "monospace" {
		t.Errorf("Expected p font-family 'monospace', got '%s'", p.ComputedStyle.FontFamily)
	}
}

func TestListStyleShorthandParsing(t *testing.T) {
	content := `<html><head><style>
		ul.none { list-style: none; }
		ul.square { list-style: square inside; }
	</style></head><body>
		<ul class="none"><li id="li1">Item 1</li></ul>
		<ul class="square"><li id="li2">Item 2</li></ul>
	</body></html>`

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	li1 := findNodeByID(renderTree, "li1")
	if li1 == nil {
		t.Fatal("li1 not found")
	}
	if li1.ComputedStyle.ListStyleType != "none" {
		t.Errorf("Expected li1 list-style-type 'none', got '%s'", li1.ComputedStyle.ListStyleType)
	}

	li2 := findNodeByID(renderTree, "li2")
	if li2 == nil {
		t.Fatal("li2 not found")
	}
	if li2.ComputedStyle.ListStyleType != "square" {
		t.Errorf("Expected li2 list-style-type 'square', got '%s'", li2.ComputedStyle.ListStyleType)
	}
	if li2.ComputedStyle.ListStylePosition != "inside" {
		t.Errorf("Expected li2 list-style-position 'inside', got '%s'", li2.ComputedStyle.ListStylePosition)
	}
}

func TestDefaultUALinkStyle(t *testing.T) {
	content := `<html><body>
		<a href="https://example.com" id="link">Default Link</a>
	</body></html>`

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManager(nil) // Only UA stylesheet
	sm.ApplyStyles(renderTree)

	link := findNodeByID(renderTree, "link")
	if link == nil {
		t.Fatal("link not found")
	}

	expectedColor := color.RGBA{R: 0, G: 0, B: 0xee, A: 0xff}
	if link.ComputedStyle.Color != expectedColor {
		t.Errorf("Expected default UA link color %v, got %v", expectedColor, link.ComputedStyle.Color)
	}
	if !strings.Contains(link.ComputedStyle.TextDecoration, "underline") {
		t.Errorf("Expected default UA link underline decoration, got '%s'", link.ComputedStyle.TextDecoration)
	}
}
