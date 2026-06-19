package renderer

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestParseLengthValues tests the parseLength function
func TestParseLengthValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fontSize float32
		expected float32
	}{
		{"empty string", "", 16.0, 0},
		{"zero", "0", 16.0, 0},
		{"pixels", "10px", 16.0, 10.0},
		{"em units", "2em", 16.0, 32.0},
		{"rem units", "1.5rem", 16.0, 24.0},
		{"thin keyword", "thin", 16.0, 1.0},
		{"medium keyword", "medium", 16.0, 3.0},
		{"thick keyword", "thick", 16.0, 5.0},
		{"plain number", "5", 16.0, 5.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLength(tt.input, tt.fontSize)
			if result != tt.expected {
				t.Errorf("parseLength(%q, %f) = %f; want %f", tt.input, tt.fontSize, result, tt.expected)
			}
		})
	}
}

// TestParseBoxShorthand tests the parseBoxShorthand function
func TestParseBoxShorthand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected [4]string
	}{
		{
			name:     "one value",
			input:    "10px",
			expected: [4]string{"10px", "10px", "10px", "10px"},
		},
		{
			name:     "two values",
			input:    "10px 20px",
			expected: [4]string{"10px", "20px", "10px", "20px"},
		},
		{
			name:     "three values",
			input:    "10px 20px 30px",
			expected: [4]string{"10px", "20px", "30px", "20px"},
		},
		{
			name:     "four values",
			input:    "10px 20px 30px 40px",
			expected: [4]string{"10px", "20px", "30px", "40px"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBoxShorthand(tt.input)
			if result != tt.expected {
				t.Errorf("parseBoxShorthand(%q) = %v; want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestMarginParsing tests that CSS margin properties are correctly parsed
func TestMarginParsing(t *testing.T) {
	cssText := `
		.margin-all { margin: 10px; }
		.margin-two { margin: 10px 20px; }
		.margin-four { margin: 5px 10px 15px 20px; }
		.margin-individual { 
			margin-top: 5px;
			margin-right: 10px;
			margin-bottom: 15px;
			margin-left: 20px;
		}
	`

	parser := css.NewParser(cssText)
	stylesheet, err := parser.Parse()
	if err != nil {
		t.Fatalf("Failed to parse CSS: %v", err)
	}

	htmlStr := `<!DOCTYPE html>
<html>
<body>
	<div class="margin-all">All margins</div>
	<div class="margin-two">Two margins</div>
	<div class="margin-four">Four margins</div>
	<div class="margin-individual">Individual margins</div>
</body>
</html>`

	renderTree, err := parseHTMLToRenderTree(htmlStr)
	if err != nil {
		t.Fatalf("Failed to parse HTML to render tree: %v", err)
	}
	if renderTree == nil {
		t.Fatal("renderTree is nil")
	}
	
	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	// Test margin-all
	marginAll := findNodeByClass(renderTree, "margin-all")
	if marginAll == nil {
		t.Fatal("margin-all node not found")
	}
	if marginAll.ComputedStyle.MarginTop != "10px" ||
		marginAll.ComputedStyle.MarginRight != "10px" ||
		marginAll.ComputedStyle.MarginBottom != "10px" ||
		marginAll.ComputedStyle.MarginLeft != "10px" {
		t.Errorf("margin-all: got %v %v %v %v, want all 10px",
			marginAll.ComputedStyle.MarginTop,
			marginAll.ComputedStyle.MarginRight,
			marginAll.ComputedStyle.MarginBottom,
			marginAll.ComputedStyle.MarginLeft)
	}

	// Test margin-two
	marginTwo := findNodeByClass(renderTree, "margin-two")
	if marginTwo == nil {
		t.Fatal("margin-two node not found")
	}
	if marginTwo.ComputedStyle.MarginTop != "10px" ||
		marginTwo.ComputedStyle.MarginRight != "20px" ||
		marginTwo.ComputedStyle.MarginBottom != "10px" ||
		marginTwo.ComputedStyle.MarginLeft != "20px" {
		t.Errorf("margin-two: got %v %v %v %v, want 10px 20px 10px 20px",
			marginTwo.ComputedStyle.MarginTop,
			marginTwo.ComputedStyle.MarginRight,
			marginTwo.ComputedStyle.MarginBottom,
			marginTwo.ComputedStyle.MarginLeft)
	}

	// Test margin-four
	marginFour := findNodeByClass(renderTree, "margin-four")
	if marginFour == nil {
		t.Fatal("margin-four node not found")
	}
	if marginFour.ComputedStyle.MarginTop != "5px" ||
		marginFour.ComputedStyle.MarginRight != "10px" ||
		marginFour.ComputedStyle.MarginBottom != "15px" ||
		marginFour.ComputedStyle.MarginLeft != "20px" {
		t.Errorf("margin-four: got %v %v %v %v, want 5px 10px 15px 20px",
			marginFour.ComputedStyle.MarginTop,
			marginFour.ComputedStyle.MarginRight,
			marginFour.ComputedStyle.MarginBottom,
			marginFour.ComputedStyle.MarginLeft)
	}

	// Test margin-individual
	marginIndividual := findNodeByClass(renderTree, "margin-individual")
	if marginIndividual == nil {
		t.Fatal("margin-individual node not found")
	}
	if marginIndividual.ComputedStyle.MarginTop != "5px" ||
		marginIndividual.ComputedStyle.MarginRight != "10px" ||
		marginIndividual.ComputedStyle.MarginBottom != "15px" ||
		marginIndividual.ComputedStyle.MarginLeft != "20px" {
		t.Errorf("margin-individual: got %v %v %v %v, want 5px 10px 15px 20px",
			marginIndividual.ComputedStyle.MarginTop,
			marginIndividual.ComputedStyle.MarginRight,
			marginIndividual.ComputedStyle.MarginBottom,
			marginIndividual.ComputedStyle.MarginLeft)
	}
}

// TestPaddingParsing tests that CSS padding properties are correctly parsed
func TestPaddingParsing(t *testing.T) {
	cssText := `
		.padding-all { padding: 10px; }
		.padding-two { padding: 10px 20px; }
		.padding-four { padding: 5px 10px 15px 20px; }
	`

	parser := css.NewParser(cssText)
	stylesheet, err := parser.Parse()
	if err != nil {
		t.Fatalf("Failed to parse CSS: %v", err)
	}

	htmlStr := `<!DOCTYPE html>
<html>
<body>
	<div class="padding-all">All padding</div>
	<div class="padding-two">Two padding</div>
	<div class="padding-four">Four padding</div>
</body>
</html>`

	renderTree, err := parseHTMLToRenderTree(htmlStr)
	if err != nil {
		t.Fatalf("Failed to parse HTML to render tree: %v", err)
	}

	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	// Test padding-all
	paddingAll := findNodeByClass(renderTree, "padding-all")
	if paddingAll == nil {
		t.Fatal("padding-all node not found")
	}
	if paddingAll.ComputedStyle.PaddingTop != "10px" ||
		paddingAll.ComputedStyle.PaddingRight != "10px" ||
		paddingAll.ComputedStyle.PaddingBottom != "10px" ||
		paddingAll.ComputedStyle.PaddingLeft != "10px" {
		t.Errorf("padding-all: got %v %v %v %v, want all 10px",
			paddingAll.ComputedStyle.PaddingTop,
			paddingAll.ComputedStyle.PaddingRight,
			paddingAll.ComputedStyle.PaddingBottom,
			paddingAll.ComputedStyle.PaddingLeft)
	}
}

// TestBorderParsing tests that CSS border properties are correctly parsed
func TestBorderParsing(t *testing.T) {
	cssText := `
		.border-all { border: 2px solid red; }
		.border-width { border-width: 1px 2px 3px 4px; }
		.border-style { border-style: solid dashed dotted double; }
		.border-color { border-color: red blue green yellow; }
		.border-sides {
			border-top: 1px solid red;
			border-right: 2px dashed blue;
			border-bottom: 3px dotted green;
			border-left: 4px double yellow;
		}
	`

	parser := css.NewParser(cssText)
	stylesheet, err := parser.Parse()
	if err != nil {
		t.Fatalf("Failed to parse CSS: %v", err)
	}

	htmlStr := `<!DOCTYPE html>
<html>
<body>
	<div class="border-all">All borders</div>
	<div class="border-width">Border width</div>
	<div class="border-style">Border style</div>
	<div class="border-color">Border color</div>
	<div class="border-sides">Border sides</div>
</body>
</html>`

	renderTree, err := parseHTMLToRenderTree(htmlStr)
	if err != nil {
		t.Fatalf("Failed to parse HTML to render tree: %v", err)
	}

	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	// Test border-all
	borderAll := findNodeByClass(renderTree, "border-all")
	if borderAll == nil {
		t.Fatal("border-all node not found")
	}
	if borderAll.ComputedStyle.BorderTopWidth != "2px" ||
		borderAll.ComputedStyle.BorderTopStyle != "solid" {
		t.Errorf("border-all: got width=%v style=%v, want 2px solid",
			borderAll.ComputedStyle.BorderTopWidth,
			borderAll.ComputedStyle.BorderTopStyle)
	}
	expectedRed := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	if borderAll.ComputedStyle.BorderTopColor != expectedRed {
		t.Errorf("border-all: got color=%v, want red", borderAll.ComputedStyle.BorderTopColor)
	}

	// Test border-width
	borderWidth := findNodeByClass(renderTree, "border-width")
	if borderWidth == nil {
		t.Fatal("border-width node not found")
	}
	if borderWidth.ComputedStyle.BorderTopWidth != "1px" ||
		borderWidth.ComputedStyle.BorderRightWidth != "2px" ||
		borderWidth.ComputedStyle.BorderBottomWidth != "3px" ||
		borderWidth.ComputedStyle.BorderLeftWidth != "4px" {
		t.Errorf("border-width: got %v %v %v %v, want 1px 2px 3px 4px",
			borderWidth.ComputedStyle.BorderTopWidth,
			borderWidth.ComputedStyle.BorderRightWidth,
			borderWidth.ComputedStyle.BorderBottomWidth,
			borderWidth.ComputedStyle.BorderLeftWidth)
	}

	// Test border-style
	borderStyle := findNodeByClass(renderTree, "border-style")
	if borderStyle == nil {
		t.Fatal("border-style node not found")
	}
	if borderStyle.ComputedStyle.BorderTopStyle != "solid" ||
		borderStyle.ComputedStyle.BorderRightStyle != "dashed" ||
		borderStyle.ComputedStyle.BorderBottomStyle != "dotted" ||
		borderStyle.ComputedStyle.BorderLeftStyle != "double" {
		t.Errorf("border-style: got %v %v %v %v, want solid dashed dotted double",
			borderStyle.ComputedStyle.BorderTopStyle,
			borderStyle.ComputedStyle.BorderRightStyle,
			borderStyle.ComputedStyle.BorderBottomStyle,
			borderStyle.ComputedStyle.BorderLeftStyle)
	}
}

// TestBoxModelLayout tests that box model properties affect layout
func TestBoxModelLayout(t *testing.T) {
	cssText := `
		.box {
			margin: 10px;
			padding: 20px;
			border: 5px solid black;
		}
	`

	parser := css.NewParser(cssText)
	stylesheet, err := parser.Parse()
	if err != nil {
		t.Fatalf("Failed to parse CSS: %v", err)
	}

	htmlStr := `<!DOCTYPE html>
<html>
<body>
	<div class="box">Content</div>
</body>
</html>`

	renderTree, err := parseHTMLToRenderTree(htmlStr)
	if err != nil {
		t.Fatalf("Failed to parse HTML to render tree: %v", err)
	}

	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	// Create layout engine and compute layout
	layoutEngine := NewLayoutEngine(800, 600)
	layoutRoot := layoutEngine.ComputeLayout(renderTree)

	// Find the box element in the layout tree
	boxNode := findNodeByClass(renderTree, "box")
	if boxNode == nil {
		t.Fatal("box node not found in render tree")
	}

	layoutBox := layoutEngine.GetLayoutBox(boxNode.ID)
	if layoutBox == nil {
		t.Fatal("layout box not found")
	}

	// Check that margins are applied
	if layoutBox.MarginTop != 10.0 {
		t.Errorf("MarginTop = %f; want 10.0", layoutBox.MarginTop)
	}
	if layoutBox.MarginLeft != 10.0 {
		t.Errorf("MarginLeft = %f; want 10.0", layoutBox.MarginLeft)
	}

	// Check that padding is applied
	if layoutBox.PaddingTop != 20.0 {
		t.Errorf("PaddingTop = %f; want 20.0", layoutBox.PaddingTop)
	}
	if layoutBox.PaddingLeft != 20.0 {
		t.Errorf("PaddingLeft = %f; want 20.0", layoutBox.PaddingLeft)
	}

	// Check that border is applied
	if layoutBox.BorderTopWidth != 5.0 {
		t.Errorf("BorderTopWidth = %f; want 5.0", layoutBox.BorderTopWidth)
	}
	if layoutBox.BorderTopStyle != "solid" {
		t.Errorf("BorderTopStyle = %s; want solid", layoutBox.BorderTopStyle)
	}

	// The box should be positioned with margin offset.
	// body has 8px margin (UA default), div has 10px margin: Box.X = 8 + 10 = 18
	if layoutBox.Box.X != 18.0 {
		t.Errorf("Box.X = %f; want 18.0 (body 8px margin + div 10px margin)", layoutBox.Box.X)
	}
	
	// Verify layout root was created
	if layoutRoot == nil {
		t.Error("Layout root should not be nil")
	}
}

// findNodeByClass is a helper to find a node by class attribute
func findNodeByClass(node *RenderNode, className string) *RenderNode {
	if node == nil {
		return nil
	}

	if class, ok := node.GetAttribute("class"); ok && class == className {
		return node
	}

	for _, child := range node.Children {
		if found := findNodeByClass(child, className); found != nil {
			return found
		}
	}

	return nil
}

// parseHTMLToRenderTree is a helper that parses HTML and builds a render tree
func parseHTMLToRenderTree(htmlStr string) (*RenderNode, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, err
	}

	// html.Parse returns a DocumentNode, we need to find the html element
	var htmlNode *html.Node
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "html" {
			htmlNode = c
			break
		}
	}
	if htmlNode == nil {
		return nil, fmt.Errorf("html element not found")
	}

	return BuildRenderTree(htmlNode), nil
}
