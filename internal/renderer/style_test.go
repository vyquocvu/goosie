package renderer

import (
	"image/color"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestStyleApplication(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					h1 {
						display: block;
						font-size: 32px;
					}
				</style>
			</head>
			<body>
				<h1>Hello, world!</h1>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}

	renderTree := BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	h1Node := findNodeByTag(renderTree, "h1")
	if h1Node == nil {
		t.Fatal("h1 node not found in render tree")
	}

	if h1Node.ComputedStyle.Display != "block" {
		t.Errorf("expected display 'block', got '%s'", h1Node.ComputedStyle.Display)
	}
	if h1Node.ComputedStyle.FontSize != 32.0 {
		t.Errorf("expected font-size 32.0, got %f", h1Node.ComputedStyle.FontSize)
	}
}

func TestAdvancedStyleApplication(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					body {
						font-size: 16px;
						background-color: #eee;
						width: 60vw;
						margin: 15vh auto;
						font-family: system-ui, sans-serif;
					}
					h1 { font-size: 1.5em; }
					div { opacity: 0.8; }
					a:link { color: #348; }
				</style>
			</head>
			<body>
				<h1>Title</h1>
				<div>A div</div>
				<a href="#">Link</a>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	bodyNode := findNodeByTag(renderTree, "body")
	if bodyNode == nil {
		t.Fatal("body node not found in render tree")
	}
	expectedBgColor := color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	if bodyNode.ComputedStyle.BackgroundColor != expectedBgColor {
		t.Errorf("expected background color %v, got %v", expectedBgColor, bodyNode.ComputedStyle.BackgroundColor)
	}
	if bodyNode.ComputedStyle.Width != "60vw" {
		t.Errorf("expected width '60vw', got '%s'", bodyNode.ComputedStyle.Width)
	}
	// Check margin shorthand was properly expanded
	if bodyNode.ComputedStyle.MarginTop != "15vh" {
		t.Errorf("expected margin-top '15vh', got '%s'", bodyNode.ComputedStyle.MarginTop)
	}
	if bodyNode.ComputedStyle.MarginRight != "auto" {
		t.Errorf("expected margin-right 'auto', got '%s'", bodyNode.ComputedStyle.MarginRight)
	}
	if bodyNode.ComputedStyle.MarginBottom != "15vh" {
		t.Errorf("expected margin-bottom '15vh', got '%s'", bodyNode.ComputedStyle.MarginBottom)
	}
	if bodyNode.ComputedStyle.MarginLeft != "auto" {
		t.Errorf("expected margin-left 'auto', got '%s'", bodyNode.ComputedStyle.MarginLeft)
	}
	if bodyNode.ComputedStyle.FontFamily != "system-ui, sans-serif" {
		t.Errorf("expected font-family 'system-ui, sans-serif', got '%s'", bodyNode.ComputedStyle.FontFamily)
	}

	h1Node := findNodeByTag(renderTree, "h1")
	if h1Node == nil {
		t.Fatal("h1 node not found in render tree")
	}
	expectedFontSize := float32(24.0)
	if h1Node.ComputedStyle.FontSize != expectedFontSize {
		t.Errorf("expected font-size %f, got %f", expectedFontSize, h1Node.ComputedStyle.FontSize)
	}

	divNode := findNodeByTag(renderTree, "div")
	if divNode == nil {
		t.Fatal("div node not found in render tree")
	}
	if divNode.ComputedStyle.Opacity != 0.8 {
		t.Errorf("expected opacity 0.8, got %f", divNode.ComputedStyle.Opacity)
	}

	aNode := findNodeByTag(renderTree, "a")
	if aNode == nil {
		t.Fatal("a node not found in render tree")
	}
	expectedLinkColor := color.RGBA{R: 0x33, G: 0x44, B: 0x88, A: 0xff}
	if aNode.ComputedStyle.Color != expectedLinkColor {
		t.Errorf("expected color %v, got %v", expectedLinkColor, aNode.ComputedStyle.Color)
	}
}

func findNodeByTag(node *RenderNode, tagName string) *RenderNode {
	if node.TagName == tagName {
		return node
	}
	for _, child := range node.Children {
		if found := findNodeByTag(child, tagName); found != nil {
			return found
		}
	}
	return nil
}

func TestNamedColorApplication(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					div {
						color: red;
						background-color: blue;
					}
				</style>
			</head>
			<body>
				<div>Red text, blue background</div>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	divNode := findNodeByTag(renderTree, "div")
	if divNode == nil {
		t.Fatal("div node not found in render tree")
	}

	expectedColor := color.RGBA{R: 0xff, A: 0xff}
	if divNode.ComputedStyle.Color != expectedColor {
		t.Errorf("expected color %v, got %v", expectedColor, divNode.ComputedStyle.Color)
	}

	expectedBgColor := color.RGBA{B: 0xff, A: 0xff}
	if divNode.ComputedStyle.BackgroundColor != expectedBgColor {
		t.Errorf("expected background color %v, got %v", expectedBgColor, divNode.ComputedStyle.BackgroundColor)
	}
}

func TestMediaQueryStyleApplication(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.box {
						background-color: red;
					}
					@media (max-width: 600px) {
						.box {
							background-color: blue;
						}
					}
				</style>
			</head>
			<body>
				<div class="box">Responsive box</div>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	// Test with large viewport (media query should NOT apply)
	largeViewportSM := NewStyleManagerWithViewport(stylesheet, 800, 600)
	largeViewportSM.ApplyStyles(renderTree)

	boxNode := findNodeByClass(renderTree, "box")
	if boxNode == nil {
		t.Fatal("box node not found in render tree")
	}

	expectedRed := color.RGBA{R: 0xff, A: 0xff}
	if boxNode.ComputedStyle.BackgroundColor != expectedRed {
		t.Errorf("large viewport: expected background color %v (red), got %v", expectedRed, boxNode.ComputedStyle.BackgroundColor)
	}

	// Rebuild render tree for second test
	renderTree2 := BuildRenderTree(findBodyNode(doc))
	if renderTree2 == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	// Test with small viewport (media query SHOULD apply)
	smallViewportSM := NewStyleManagerWithViewport(stylesheet, 500, 400)
	smallViewportSM.ApplyStyles(renderTree2)

	boxNode2 := findNodeByClass(renderTree2, "box")
	if boxNode2 == nil {
		t.Fatal("box node not found in render tree")
	}

	expectedBlue := color.RGBA{B: 0xff, A: 0xff}
	if boxNode2.ComputedStyle.BackgroundColor != expectedBlue {
		t.Errorf("small viewport: expected background color %v (blue), got %v", expectedBlue, boxNode2.ComputedStyle.BackgroundColor)
	}
}

func TestSetViewportDynamic(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.responsive {
						color: black;
					}
					@media (max-width: 768px) {
						.responsive {
							color: red;
						}
					}
				</style>
			</head>
			<body>
				<div class="responsive">Dynamic viewport</div>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	sm := NewStyleManagerWithViewport(stylesheet, 1024, 768)

	// First apply with large viewport
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm.ApplyStyles(renderTree)

	node := findNodeByClass(renderTree, "responsive")
	expectedBlack := color.RGBA{A: 0xff}
	if node.ComputedStyle.Color != expectedBlack {
		t.Errorf("before resize: expected color %v (black), got %v", expectedBlack, node.ComputedStyle.Color)
	}

	// Update viewport to smaller size
	sm.SetViewport(600, 400)

	// Reapply styles (in real browser this would happen on resize)
	renderTree2 := BuildRenderTree(findBodyNode(doc))
	sm.ApplyStyles(renderTree2)

	node2 := findNodeByClass(renderTree2, "responsive")
	expectedRed := color.RGBA{R: 0xff, A: 0xff}
	if node2.ComputedStyle.Color != expectedRed {
		t.Errorf("after resize: expected color %v (red), got %v", expectedRed, node2.ComputedStyle.Color)
	}
}

func TestParsedColorAndFontSizeExtensions(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					body { font-size: 16px; }
					.item-rem { font-size: 1.5rem; color: rgb(255, 0, 0); }
					.item-percent { font-size: 50%; color: rgba(0, 255, 0, 0.5); }
					.item-keyword { font-size: large; color: hsl(240, 100%, 50%); }
					.item-transparent { color: transparent; }
					.item-pt { font-size: 12pt; }
				</style>
			</head>
			<body>
				<div class="item-rem">Rem</div>
				<div class="item-percent">Percent</div>
				<div class="item-keyword">Keyword</div>
				<div class="item-transparent">Transparent</div>
				<div class="item-pt">Pt</div>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}
	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	// Test rem and rgb
	nodeRem := findNodeByClass(renderTree, "item-rem")
	if nodeRem == nil {
		t.Fatal("item-rem node not found")
	}
	if nodeRem.ComputedStyle.FontSize != 24.0 {
		t.Errorf("expected rem font-size 24.0, got %f", nodeRem.ComputedStyle.FontSize)
	}
	expectedRed := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	if nodeRem.ComputedStyle.Color != expectedRed {
		t.Errorf("expected rgb color %v, got %v", expectedRed, nodeRem.ComputedStyle.Color)
	}

	// Test percent and rgba
	nodePercent := findNodeByClass(renderTree, "item-percent")
	if nodePercent == nil {
		t.Fatal("item-percent node not found")
	}
	if nodePercent.ComputedStyle.FontSize != 8.0 {
		t.Errorf("expected percent font-size 8.0, got %f", nodePercent.ComputedStyle.FontSize)
	}
	expectedGreenHalf := color.RGBA{R: 0, G: 255, B: 0, A: 127}
	if nodePercent.ComputedStyle.Color != expectedGreenHalf {
		t.Errorf("expected rgba color %v, got %v", expectedGreenHalf, nodePercent.ComputedStyle.Color)
	}

	// Test keyword and hsl
	nodeKeyword := findNodeByClass(renderTree, "item-keyword")
	if nodeKeyword == nil {
		t.Fatal("item-keyword node not found")
	}
	if nodeKeyword.ComputedStyle.FontSize != 18.0 {
		t.Errorf("expected large font-size 18.0, got %f", nodeKeyword.ComputedStyle.FontSize)
	}
	expectedBlue := color.RGBA{R: 0, G: 0, B: 255, A: 255}
	if nodeKeyword.ComputedStyle.Color != expectedBlue {
		t.Errorf("expected hsl color %v, got %v", expectedBlue, nodeKeyword.ComputedStyle.Color)
	}

	// Test transparent
	nodeTrans := findNodeByClass(renderTree, "item-transparent")
	if nodeTrans == nil {
		t.Fatal("item-transparent node not found")
	}
	if nodeTrans.ComputedStyle.Color != color.Transparent {
		t.Errorf("expected transparent color, got %v", nodeTrans.ComputedStyle.Color)
	}

	// Test pt
	nodePt := findNodeByClass(renderTree, "item-pt")
	if nodePt == nil {
		t.Fatal("item-pt node not found")
	}
	if nodePt.ComputedStyle.FontSize != 16.0 {
		t.Errorf("expected pt font-size 16.0, got %f", nodePt.ComputedStyle.FontSize)
	}
}

func TestBackgroundShorthand(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.box1 { background: #123456; }
					.box2 { background: url("bg.png") no-repeat center rgb(10, 20, 30); }
					.box3 { background: transparent; }
					.box4 { background: red none; }
				</style>
			</head>
			<body>
				<div class="box1"></div>
				<div class="box2"></div>
				<div class="box3"></div>
				<div class="box4"></div>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	b1 := findNodeByClass(renderTree, "box1")
	if b1.ComputedStyle.BackgroundColor != (color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}) {
		t.Errorf("box1 expected color 123456, got %v", b1.ComputedStyle.BackgroundColor)
	}

	b2 := findNodeByClass(renderTree, "box2")
	if b2.ComputedStyle.BackgroundColor != (color.RGBA{R: 10, G: 20, B: 30, A: 255}) {
		t.Errorf("box2 expected rgb(10,20,30), got %v", b2.ComputedStyle.BackgroundColor)
	}

	b3 := findNodeByClass(renderTree, "box3")
	if b3.ComputedStyle.BackgroundColor != color.Transparent {
		t.Errorf("box3 expected transparent, got %v", b3.ComputedStyle.BackgroundColor)
	}

	b4 := findNodeByClass(renderTree, "box4")
	if b4.ComputedStyle.BackgroundColor != (color.RGBA{R: 0xff, A: 0xff}) {
		t.Errorf("box4 expected red, got %v", b4.ComputedStyle.BackgroundColor)
	}
}


