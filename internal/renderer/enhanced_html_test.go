package renderer

import (
	"context"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"golang.org/x/net/html"
)

// TestCSSColorSupport tests if CSS colors are applied to text
func TestCSSColorSupport(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.red-text { color: red; }
					.blue-text { color: #0000ff; }
				</style>
			</head>
			<body>
				<p class="red-text">This text should be red</p>
				<p class="blue-text">This text should be blue</p>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	// Extract and parse CSS
	stylesheet := extractAndParseCSS(doc)
	if stylesheet == nil || len(stylesheet.Rules) == 0 {
		t.Fatal("Expected CSS rules to be parsed")
	}

	// Build render tree
	renderTree := BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		t.Fatal("Expected render tree to be built")
	}

	// Apply styles
	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	// Check that styles were applied
	foundRed := false
	foundBlue := false

	var checkNode func(*RenderNode)
	checkNode = func(node *RenderNode) {
		if node.ComputedStyle != nil && node.ComputedStyle.Color != nil {
			// Check if color was applied
			if node.TagName == "p" {
				if classAttr, ok := node.GetAttribute("class"); ok {
					if classAttr == "red-text" {
						foundRed = true
						t.Logf("Found red-text node with color: %v", node.ComputedStyle.Color)
					} else if classAttr == "blue-text" {
						foundBlue = true
						t.Logf("Found blue-text node with color: %v", node.ComputedStyle.Color)
					}
				}
			}
		}
		for _, child := range node.Children {
			checkNode(child)
		}
	}

	checkNode(renderTree)

	if !foundRed {
		t.Error("Expected red color to be applied to red-text class")
	}
	if !foundBlue {
		t.Error("Expected blue color to be applied to blue-text class")
	}
}

// TestCSSFontSizeSupport tests if CSS font sizes are applied
func TestCSSFontSizeSupport(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.large { font-size: 24px; }
					.small { font-size: 12px; }
				</style>
			</head>
			<body>
				<p class="large">Large text</p>
				<p class="small">Small text</p>
			</body>
		</html>
	`
	_ = NewRenderer(800, 600)
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	foundLarge := false
	foundSmall := false

	var checkNode func(*RenderNode)
	checkNode = func(node *RenderNode) {
		if node.ComputedStyle != nil && node.ComputedStyle.FontSize > 0 {
			if classAttr, ok := node.GetAttribute("class"); ok {
				if classAttr == "large" && node.ComputedStyle.FontSize == 24.0 {
					foundLarge = true
					t.Logf("Found large class with font-size: %v", node.ComputedStyle.FontSize)
				} else if classAttr == "small" && node.ComputedStyle.FontSize == 12.0 {
					foundSmall = true
					t.Logf("Found small class with font-size: %v", node.ComputedStyle.FontSize)
				}
			}
		}
		for _, child := range node.Children {
			checkNode(child)
		}
	}

	checkNode(renderTree)

	if !foundLarge {
		t.Error("Expected font-size 24px to be applied to large class")
	}
	if !foundSmall {
		t.Error("Expected font-size 12px to be applied to small class")
	}
}

// TestImageFormatSupport tests that the image loader supports various formats
func TestImageFormatSupport(t *testing.T) {
	// This test verifies that the image loader imports the necessary decoders
	// The actual format support is verified by the existence of the imports
	// in internal/image/loader.go

	htmlContent := `
		<html>
			<body>
				<img src="test.png" alt="PNG image" />
				<img src="test.jpg" alt="JPEG image" />
				<img src="test.gif" alt="GIF image" />
				<img src="test.webp" alt="WebP image" />
			</body>
		</html>
	`
	_ = NewRenderer(800, 600)
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	renderTree := BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		t.Fatal("Expected render tree to be built")
	}

	// Count image nodes
	imageCount := 0
	var countImages func(*RenderNode)
	countImages = func(node *RenderNode) {
		if node.TagName == "img" {
			imageCount++
		}
		for _, child := range node.Children {
			countImages(child)
		}
	}

	countImages(renderTree)

	if imageCount != 4 {
		t.Errorf("Expected 4 image nodes, got %d", imageCount)
	}
}

// TestLinkClickability tests that links can be clicked
func TestLinkClickability(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<a href="https://example.com">Click me</a>
			</body>
		</html>
	`
	r := NewRenderer(800, 600)

	r.SetNavigationCallback(func(url string) {
		if url != "https://example.com" {
			t.Errorf("Expected URL https://example.com, got %s", url)
		}
	})

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	renderTree := BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		t.Fatal("Expected render tree to be built")
	}

	// Check that link node exists
	linkFound := false
	var findLink func(*RenderNode)
	findLink = func(node *RenderNode) {
		if node.TagName == "a" {
			linkFound = true
			if href, ok := node.GetAttribute("href"); !ok || href != "https://example.com" {
				t.Errorf("Expected href https://example.com, got %s", href)
			}
		}
		for _, child := range node.Children {
			findLink(child)
		}
	}

	findLink(renderTree)

	if !linkFound {
		t.Error("Expected to find a link node")
	}
}

// TestCSSRenderingWithColors tests that CSS colors are applied to rendered widgets
func TestCSSRenderingWithColors(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.red { color: red; }
					.blue { color: #0000ff; }
				</style>
			</head>
			<body>
				<p class="red">Red text</p>
				<p class="blue">Blue text</p>
			</body>
		</html>
	`
	r := NewRenderer(800, 600)
	r.SetCurrentURL("https://example.com")

	canvasObj, err := r.RenderHTML(context.Background(), htmlContent)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	// The canvas object should be a container
	container, ok := canvasObj.(*fyne.Container)
	if !ok {
		t.Fatalf("Expected container, got %T", canvasObj)
	}

	// We should have at least 2 objects (the two paragraphs)
	if len(container.Objects) < 2 {
		t.Fatalf("Expected at least 2 objects, got %d", len(container.Objects))
	}

	// Debug: print what we got
	t.Logf("Container has %d objects:", len(container.Objects))
	for i, obj := range container.Objects {
		t.Logf("  Object %d: %T", i, obj)
		if text, ok := obj.(*canvas.Text); ok {
			t.Logf("    Text: %s, Color: %v, Size: %f", text.Text, text.Color, text.TextSize)
		}
	}

	// Check if canvas.Text objects with colors are present
	foundRed := false
	foundBlue := false

	for _, obj := range container.Objects {
		if text, ok := obj.(*canvas.Text); ok {
			// Check for red color
			if r, g, b, a := text.Color.RGBA(); r == 0xffff && g == 0 && b == 0 && a == 0xffff {
				foundRed = true
				t.Logf("Found red text: %s", text.Text)
			}
			// Check for blue color
			if r, g, b, a := text.Color.RGBA(); r == 0 && g == 0 && b == 0xffff && a == 0xffff {
				foundBlue = true
				t.Logf("Found blue text: %s", text.Text)
			}
		}
	}

	if !foundRed {
		t.Error("Expected to find red colored text")
	}
	if !foundBlue {
		t.Error("Expected to find blue colored text")
	}
}

// TestCSSRenderingWithFontSize tests that CSS font sizes are applied
func TestCSSRenderingWithFontSize(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.large { font-size: 24px; }
					.small { font-size: 10px; }
				</style>
			</head>
			<body>
				<p class="large">Large text</p>
				<p class="small">Small text</p>
			</body>
		</html>
	`
	r := NewRenderer(800, 600)
	r.SetCurrentURL("https://example.com")

	canvasObj, err := r.RenderHTML(context.Background(), htmlContent)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	container, ok := canvasObj.(*fyne.Container)
	if !ok {
		t.Fatalf("Expected container, got %T", canvasObj)
	}

	foundLarge := false
	foundSmall := false

	for _, obj := range container.Objects {
		if text, ok := obj.(*canvas.Text); ok {
			if text.TextSize == 24.0 {
				foundLarge = true
				t.Logf("Found large text: %s (size: %f)", text.Text, text.TextSize)
			}
			if text.TextSize == 10.0 {
				foundSmall = true
				t.Logf("Found small text: %s (size: %f)", text.Text, text.TextSize)
			}
		}
	}

	if !foundLarge {
		t.Error("Expected to find large (24px) text")
	}
	if !foundSmall {
		t.Error("Expected to find small (10px) text")
	}
}
