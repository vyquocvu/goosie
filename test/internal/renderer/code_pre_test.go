package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// findElementByTag is a helper function to find an HTML element by tag name
func findElementByTag(doc *html.Node, tagName string) *html.Node {
	var result *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tagName {
			result = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	return result
}

func TestRenderCodeElement(t *testing.T) {
	htmlContent := `<code>const x = 42;</code>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	codeHTMLNode := findElementByTag(doc, "code")
	if codeHTMLNode == nil {
		t.Fatal("Code HTML node not found")
	}

	tree := renderer.BuildRenderTree(codeHTMLNode)
	if tree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	if tree.TagName != "code" {
		t.Errorf("Expected tag name 'code', got '%s'", tree.TagName)
	}
}

func TestRenderPreElement(t *testing.T) {
	htmlContent := `<pre>  Line 1
  Line 2
    Indented</pre>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	preHTMLNode := findElementByTag(doc, "pre")
	if preHTMLNode == nil {
		t.Fatal("Pre HTML node not found")
	}

	tree := renderer.BuildRenderTree(preHTMLNode)
	if tree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	if tree.TagName != "pre" {
		t.Errorf("Expected tag name 'pre', got '%s'", tree.TagName)
	}

	// Pre should be a block element
	if !tree.IsBlock() {
		t.Error("Pre element should be a block element")
	}
}

func TestRenderBlockquoteElement(t *testing.T) {
	htmlContent := `<blockquote>This is a quote</blockquote>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	blockquoteHTMLNode := findElementByTag(doc, "blockquote")
	if blockquoteHTMLNode == nil {
		t.Fatal("Blockquote HTML node not found")
	}

	tree := renderer.BuildRenderTree(blockquoteHTMLNode)
	if tree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	if tree.TagName != "blockquote" {
		t.Errorf("Expected tag name 'blockquote', got '%s'", tree.TagName)
	}

	// Blockquote should be a block element
	if !tree.IsBlock() {
		t.Error("Blockquote element should be a block element")
	}
}

func TestFontMetricsCodeStyle(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	// Test code element style
	codeStyle := fm.GetTextStyle("code")
	if !codeStyle.Monospace {
		t.Error("Code element should have monospace style")
	}

	// Test pre element style
	preStyle := fm.GetTextStyle("pre")
	if !preStyle.Monospace {
		t.Error("Pre element should have monospace style")
	}
}

func TestCanvasRendererCodeElements(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)

	// Test rendering code element
	htmlContent := `<code>const x = 42;</code>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	codeHTMLNode := findElementByTag(doc, "code")
	if codeHTMLNode == nil {
		t.Fatal("Code HTML node not found")
	}

	tree := renderer.BuildRenderTree(codeHTMLNode)
	if tree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	// Render should not panic
	obj := cr.Render(tree)
	if obj == nil {
		t.Error("Render returned nil for code element")
	}
}

func TestCanvasRendererPreElements(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)

	// Test rendering pre element
	htmlContent := `<pre>Line 1
Line 2</pre>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	preHTMLNode := findElementByTag(doc, "pre")
	if preHTMLNode == nil {
		t.Fatal("Pre HTML node not found")
	}

	tree := renderer.BuildRenderTree(preHTMLNode)
	if tree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	// Render should not panic
	obj := cr.Render(tree)
	if obj == nil {
		t.Error("Render returned nil for pre element")
	}
}

func TestCanvasRendererBlockquoteElements(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)

	// Test rendering blockquote element
	htmlContent := `<blockquote>This is a quote</blockquote>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	blockquoteHTMLNode := findElementByTag(doc, "blockquote")
	if blockquoteHTMLNode == nil {
		t.Fatal("Blockquote HTML node not found")
	}

	tree := renderer.BuildRenderTree(blockquoteHTMLNode)
	if tree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	// Render should not panic
	obj := cr.Render(tree)
	if obj == nil {
		t.Error("Render returned nil for blockquote element")
	}
}
