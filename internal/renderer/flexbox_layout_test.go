package renderer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestFlexLayoutBasic(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.flex-container {
						display: flex;
						width: 300px;
					}
					.item {
						flex-shrink: 0;
					}
				</style>
			</head>
			<body>
				<div class="flex-container">
					<div class="item">1</div>
					<div class="item">2</div>
					<div class="item">3</div>
				</div>
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

	layoutEngine := NewLayoutEngine(800, 600)
	layoutTree := layoutEngine.ComputeLayout(renderTree)

	if layoutTree == nil {
		t.Fatal("ComputeLayout returned nil")
	}

	// Find the flex container
	flexContainer := findLayoutByClass(t, layoutEngine, renderTree, "flex-container")
	if flexContainer == nil {
		t.Fatal("flex-container not found in layout tree")
	}

	// Verify it has 3 children laid out horizontally
	if len(flexContainer.Children) != 3 {
		t.Errorf("expected 3 flex items, got %d", len(flexContainer.Children))
	}

	// Items should be laid out in order with X increasing
	if len(flexContainer.Children) >= 2 {
		if flexContainer.Children[1].Box.X <= flexContainer.Children[0].Box.X {
			t.Error("flex items should be laid out left to right in row direction")
		}
	}
}

func TestFlexLayoutJustifyContent(t *testing.T) {
	tests := []struct {
		name           string
		justifyContent string
	}{
		{"flex-start", "flex-start"},
		{"flex-end", "flex-end"},
		{"center", "center"},
		{"space-between", "space-between"},
		{"space-around", "space-around"},
		{"space-evenly", "space-evenly"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			htmlContent := `
				<html>
					<head>
						<style>
							.flex-container {
								display: flex;
								justify-content: ` + tt.justifyContent + `;
								width: 300px;
							}
							.item {
								flex: 0 0 50px;
							}
						</style>
					</head>
					<body>
						<div class="flex-container">
							<div class="item">1</div>
							<div class="item">2</div>
						</div>
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

			layoutEngine := NewLayoutEngine(800, 600)
			layoutEngine.ComputeLayout(renderTree)

			flexContainer := findLayoutByClass(t, layoutEngine, renderTree, "flex-container")
			if flexContainer == nil {
				t.Fatal("flex-container not found")
			}

			if len(flexContainer.Children) != 2 {
				t.Fatalf("expected 2 items, got %d", len(flexContainer.Children))
			}

			item1 := flexContainer.Children[0]
			item2 := flexContainer.Children[1]

			// Basic sanity checks
			if item1.Box.X < 0 || item2.Box.X < 0 {
				t.Error("items should have non-negative X positions")
			}

			if item2.Box.X < item1.Box.X {
				t.Error("second item should be to the right of first item")
			}
		})
	}
}

func TestFlexLayoutFlexGrow(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.flex-container {
						display: flex;
						width: 300px;
					}
					.item1 {
						flex-grow: 1;
					}
					.item2 {
						flex-grow: 2;
					}
				</style>
			</head>
			<body>
				<div class="flex-container">
					<div class="item1">1</div>
					<div class="item2">2</div>
				</div>
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

	layoutEngine := NewLayoutEngine(800, 600)
	layoutEngine.ComputeLayout(renderTree)

	flexContainer := findLayoutByClass(t, layoutEngine, renderTree, "flex-container")
	if flexContainer == nil {
		t.Fatal("flex-container not found")
	}

	if len(flexContainer.Children) != 2 {
		t.Fatalf("expected 2 items, got %d", len(flexContainer.Children))
	}

	item1 := flexContainer.Children[0]
	item2 := flexContainer.Children[1]

	// item2 has flex-grow: 2, item1 has flex-grow: 1
	// item2 should get 2x the extra space compared to item1
	// Their final widths should reflect this ratio when starting from 0 base size
	if item1.Box.Width <= 0 || item2.Box.Width <= 0 {
		t.Errorf("both items should have positive widths (item1: %f, item2: %f)",
			item1.Box.Width, item2.Box.Width)
	}

	// item2 should be wider than item1
	if item2.Box.Width < item1.Box.Width {
		t.Errorf("item2 (flex-grow: 2) should be wider than item1 (flex-grow: 1), got item1: %f, item2: %f",
			item1.Box.Width, item2.Box.Width)
	}
}

func TestFlexLayoutDirection(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.flex-container {
						display: flex;
						flex-direction: column;
						width: 300px;
						height: 200px;
					}
					.item {
						flex: 1;
					}
				</style>
			</head>
			<body>
				<div class="flex-container">
					<div class="item">1</div>
					<div class="item">2</div>
				</div>
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

	layoutEngine := NewLayoutEngine(800, 600)
	layoutEngine.ComputeLayout(renderTree)

	flexContainer := findLayoutByClass(t, layoutEngine, renderTree, "flex-container")
	if flexContainer == nil {
		t.Fatal("flex-container not found")
	}

	if len(flexContainer.Children) != 2 {
		t.Fatalf("expected 2 items, got %d", len(flexContainer.Children))
	}

	item1 := flexContainer.Children[0]
	item2 := flexContainer.Children[1]

	// In column direction, items should be stacked vertically
	// item2 should be below item1 (higher Y value)
	if item2.Box.Y <= item1.Box.Y {
		t.Errorf("column direction: second item should be below first (item1.Y=%f, item2.Y=%f)",
			item1.Box.Y, item2.Box.Y)
	}

	// Both items should have same X position (stacked vertically, not horizontally)
	if item1.Box.X != item2.Box.X {
		t.Errorf("column direction: items should have same X position (item1.X=%f, item2.X=%f)",
			item1.Box.X, item2.Box.X)
	}
}

func TestFlexShorthandParsing(t *testing.T) {
	tests := []struct {
		name           string
		flexValue      string
		expectedGrow   float32
		expectedShrink float32
		expectedBasis  string
	}{
		{"single number", "1", 1, 0, ""},
		{"two numbers", "1 2", 1, 2, ""},
		{"full shorthand", "1 1 auto", 1, 1, "auto"},
		{"none", "none", 0, 0, "auto"},
		{"auto", "auto", 1, 1, "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style := &Style{}
			parseFlexShorthand(tt.flexValue, style)

			if style.FlexGrow != tt.expectedGrow {
				t.Errorf("flex-grow: expected %f, got %f", tt.expectedGrow, style.FlexGrow)
			}
			if style.FlexShrink != tt.expectedShrink {
				t.Errorf("flex-shrink: expected %f, got %f", tt.expectedShrink, style.FlexShrink)
			}
			if style.FlexBasis != tt.expectedBasis {
				t.Errorf("flex-basis: expected %s, got %s", tt.expectedBasis, style.FlexBasis)
			}
		})
	}
}

func TestFlexPropertyParsing(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.container {
						display: flex;
						flex-direction: column;
						justify-content: space-between;
						align-items: center;
						gap: 10px;
					}
					.item {
						flex-grow: 2;
						flex-shrink: 0;
						order: 1;
					}
				</style>
			</head>
			<body>
				<div class="container">
					<div class="item">Test</div>
				</div>
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

	// Find container and verify styles
	container := findNodeByClass(renderTree, "container")
	if container == nil {
		t.Fatal("container not found")
	}

	if container.ComputedStyle.Display != "flex" {
		t.Errorf("expected display: flex, got %s", container.ComputedStyle.Display)
	}
	if container.ComputedStyle.FlexDirection != "column" {
		t.Errorf("expected flex-direction: column, got %s", container.ComputedStyle.FlexDirection)
	}
	if container.ComputedStyle.JustifyContent != "space-between" {
		t.Errorf("expected justify-content: space-between, got %s", container.ComputedStyle.JustifyContent)
	}
	if container.ComputedStyle.AlignItems != "center" {
		t.Errorf("expected align-items: center, got %s", container.ComputedStyle.AlignItems)
	}
	if container.ComputedStyle.Gap != "10px" {
		t.Errorf("expected gap: 10px, got %s", container.ComputedStyle.Gap)
	}

	// Find item and verify styles
	item := findNodeByClass(renderTree, "item")
	if item == nil {
		t.Fatal("item not found")
	}

	if item.ComputedStyle.FlexGrow != 2 {
		t.Errorf("expected flex-grow: 2, got %f", item.ComputedStyle.FlexGrow)
	}
	if item.ComputedStyle.FlexShrink != 0 {
		t.Errorf("expected flex-shrink: 0, got %f", item.ComputedStyle.FlexShrink)
	}
	if item.ComputedStyle.Order != 1 {
		t.Errorf("expected order: 1, got %d", item.ComputedStyle.Order)
	}
}

// Helper function to find layout box by class
func findLayoutByClass(t *testing.T, le *LayoutEngine, renderTree *RenderNode, className string) *LayoutBox {
	node := findNodeByClass(renderTree, className)
	if node == nil {
		return nil
	}
	return le.GetLayoutBox(node.ID)
}

func TestComprehensiveGeneratedFlexboxFiles(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd failed: %v", err)
	}

	flexboxFiles, err := filepath.Glob(filepath.Join(cwd, "..", "..", "testdata", "test_*_flexbox.html"))
	if err != nil {
		t.Fatalf("filepath.Glob failed: %v", err)
	}
	if len(flexboxFiles) == 0 {
		t.Fatal("no generated flexbox files found in testdata")
	}

	for _, file := range flexboxFiles {
		t.Run(filepath.Base(file), func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("os.ReadFile failed: %v", err)
			}

			r := NewRenderer(800, 600)
			obj, err := r.RenderHTML(context.Background(), string(content))
			if err != nil {
				t.Fatalf("RenderHTML failed for %s: %v", file, err)
			}
			if obj == nil {
				t.Fatalf("RenderHTML returned nil object for %s", file)
			}
			if r.currentLayoutTree == nil {
				t.Fatalf("layout tree is nil for %s", file)
			}
			if r.GetContentHeight() <= 0 {
				t.Fatalf("expected positive content height for %s, got %f", file, r.GetContentHeight())
			}

			if countFlexNodes(r.currentRenderTree) == 0 {
				t.Fatalf("expected at least one flex node in render tree for %s", file)
			}
		})
	}
}

func countFlexNodes(node *RenderNode) int {
	if node == nil {
		return 0
	}

	count := 0
	if node.ComputedStyle.Display == "flex" {
		count++
	}

	for _, child := range node.Children {
		count += countFlexNodes(child)
	}

	return count
}
