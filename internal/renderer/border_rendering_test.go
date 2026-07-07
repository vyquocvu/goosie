package renderer

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestBorderRendering tests border rendering with display list
func TestBorderRendering(t *testing.T) {
	cssText := `
		.border-solid {
			border: 2px solid red;
			padding: 10px;
			margin: 5px;
		}
		.border-mixed {
			border-top: 3px solid blue;
			border-right: 2px dashed green;
			border-bottom: 4px dotted yellow;
			border-left: 1px solid black;
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
	<div class="border-solid">Solid border</div>
	<div class="border-mixed">Mixed borders</div>
</body>
</html>`

	renderTree, err := parseHTMLToRenderTree(htmlStr)
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	// Create layout
	layoutEngine := NewLayoutEngine(800, 600)
	layoutRoot := layoutEngine.ComputeLayout(renderTree)

	// Build display list
	builder := NewDisplayListBuilder()
	displayList := builder.Build(layoutRoot, renderTree)

	// Check that border commands were generated
	borderCommands := 0
	for _, cmd := range displayList.Commands {
		if cmd.Type == PaintBorder {
			borderCommands++
			t.Logf("Border command: Top=%f, Right=%f, Bottom=%f, Left=%f",
				cmd.BorderTopWidth, cmd.BorderRightWidth,
				cmd.BorderBottomWidth, cmd.BorderLeftWidth)
		}
	}

	if borderCommands == 0 {
		t.Error("No border commands generated")
	}

	// Verify solid border element
	solidNode := findNodeByClass(renderTree, "border-solid")
	if solidNode == nil {
		t.Fatal("border-solid node not found")
	}

	solidLayout := layoutEngine.GetLayoutBox(solidNode.ID)
	if solidLayout == nil {
		t.Fatal("border-solid layout not found")
	}

	// Check border properties
	if solidLayout.BorderTopWidth != 2.0 {
		t.Errorf("border-solid top width = %f; want 2.0", solidLayout.BorderTopWidth)
	}
	if solidLayout.BorderTopStyle != "solid" {
		t.Errorf("border-solid top style = %s; want solid", solidLayout.BorderTopStyle)
	}

	// Verify mixed border element
	mixedNode := findNodeByClass(renderTree, "border-mixed")
	if mixedNode == nil {
		t.Fatal("border-mixed node not found")
	}

	mixedLayout := layoutEngine.GetLayoutBox(mixedNode.ID)
	if mixedLayout == nil {
		t.Fatal("border-mixed layout not found")
	}

	// Check different border widths
	if mixedLayout.BorderTopWidth != 3.0 {
		t.Errorf("border-mixed top width = %f; want 3.0", mixedLayout.BorderTopWidth)
	}
	if mixedLayout.BorderRightWidth != 2.0 {
		t.Errorf("border-mixed right width = %f; want 2.0", mixedLayout.BorderRightWidth)
	}
	if mixedLayout.BorderBottomWidth != 4.0 {
		t.Errorf("border-mixed bottom width = %f; want 4.0", mixedLayout.BorderBottomWidth)
	}
	if mixedLayout.BorderLeftWidth != 1.0 {
		t.Errorf("border-mixed left width = %f; want 1.0", mixedLayout.BorderLeftWidth)
	}

	// Check different border styles
	if mixedLayout.BorderTopStyle != "solid" {
		t.Errorf("border-mixed top style = %s; want solid", mixedLayout.BorderTopStyle)
	}
	if mixedLayout.BorderRightStyle != "dashed" {
		t.Errorf("border-mixed right style = %s; want dashed", mixedLayout.BorderRightStyle)
	}
	if mixedLayout.BorderBottomStyle != "dotted" {
		t.Errorf("border-mixed bottom style = %s; want dotted", mixedLayout.BorderBottomStyle)
	}
	if mixedLayout.BorderLeftStyle != "solid" {
		t.Errorf("border-mixed left style = %s; want solid", mixedLayout.BorderLeftStyle)
	}
}

// TestBoxModelWithBackgroundAndBorder tests combining backgrounds and borders
func TestBoxModelWithBackgroundAndBorder(t *testing.T) {
	cssText := `
		.styled-box {
			background-color: #eeeeee;
			border: 3px solid #333333;
			padding: 15px;
			margin: 10px;
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
	<div class="styled-box">Content with background and border</div>
</body>
</html>`

	renderTree, err := parseHTMLToRenderTree(htmlStr)
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	// Find the styled box
	boxNode := findNodeByClass(renderTree, "styled-box")
	if boxNode == nil {
		t.Fatal("styled-box node not found")
	}

	// Check that both background and border are set
	if boxNode.ComputedStyle.BackgroundColor == nil {
		t.Error("Background color not set")
	}
	if boxNode.ComputedStyle.BorderTopWidth == "" {
		t.Error("Border width not set")
	}

	// Create layout and verify box model application
	layoutEngine := NewLayoutEngine(800, 600)
	layoutRoot := layoutEngine.ComputeLayout(renderTree)

	boxLayout := layoutEngine.GetLayoutBox(boxNode.ID)
	if boxLayout == nil {
		t.Fatal("styled-box layout not found")
	}

	// Verify padding
	if boxLayout.PaddingTop != 15.0 {
		t.Errorf("Padding = %f; want 15.0", boxLayout.PaddingTop)
	}

	// Verify margin
	if boxLayout.MarginTop != 10.0 {
		t.Errorf("Margin = %f; want 10.0", boxLayout.MarginTop)
	}

	// Verify border
	if boxLayout.BorderTopWidth != 3.0 {
		t.Errorf("Border width = %f; want 3.0", boxLayout.BorderTopWidth)
	}

	// Verify layout root was created
	if layoutRoot == nil {
		t.Error("Layout root should not be nil")
	}
}
