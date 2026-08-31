package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
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
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}

	styleManager := renderer.NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	layoutEngine := renderer.NewLayoutEngine(800, 600)
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
			renderTree := renderer.BuildRenderTree(findBodyNode(doc))
			styleManager := renderer.NewStyleManager(stylesheet)
			styleManager.ApplyStyles(renderTree)

			layoutEngine := renderer.NewLayoutEngine(800, 600)
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
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	styleManager := renderer.NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	layoutEngine := renderer.NewLayoutEngine(800, 600)
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

func TestFlexLayoutFlexBasisZero(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.flex-container {
						display: flex;
						width: 300px;
					}
					.grow {
						flex-grow: 1;
						flex-basis: 0;
						width: 250px;
					}
					.nogrow {
						flex-grow: 0;
						flex-basis: 0;
						width: 250px;
					}
				</style>
			</head>
			<body>
				<div class="flex-container">
					<div class="grow">grow</div>
					<div class="nogrow">nogrow</div>
				</div>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	styleManager := renderer.NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	layoutEngine := renderer.NewLayoutEngine(800, 600)
	layoutEngine.ComputeLayout(renderTree)

	grow := findLayoutByClass(t, layoutEngine, renderTree, "grow")
	if grow == nil {
		t.Fatal("grow item not found")
	}
	nogrow := findLayoutByClass(t, layoutEngine, renderTree, "nogrow")
	if nogrow == nil {
		t.Fatal("nogrow item not found")
	}

	// flex-basis: 0 with flex-grow: 1 should fill the remaining space after the
	// nogrow item's automatic minimum (min-width:auto), ignoring width: 250px.
	// flex-basis: 0 with flex-grow: 0 should shrink to its min-content size
	// (automatic minimum), NOT to 0 and NOT to width: 250px. This matches the
	// browser: flex items won't shrink below their content by default.
	if grow.Box.Width <= 0 || nogrow.Box.Width <= 0 {
		t.Errorf("items should have positive widths (grow: %f, nogrow: %f)",
			grow.Box.Width, nogrow.Box.Width)
	}
	if grow.Box.Width+nogrow.Box.Width < 298 || grow.Box.Width+nogrow.Box.Width > 302 {
		t.Errorf("grow (%f) + nogrow (%f) should fill the 300px container; got sum %f",
			grow.Box.Width, nogrow.Box.Width, grow.Box.Width+nogrow.Box.Width)
	}
	// nogrow stays at its min-content width (a single word), well below 250px.
	if nogrow.Box.Width > 100 {
		t.Errorf("nogrow item (flex-basis: 0, flex-grow: 0) width = %f; want min-content (ignoring width: 250px)", nogrow.Box.Width)
	}
}

func TestFlexLayoutAutomaticMinSize(t *testing.T) {
	// Mirrors the IANA site layout: a row-reverse flex container with a fixed
	// 250px nav (flex-basis: 0, width: 250px) whose content is a 225px-wide
	// block with 25px margin-right. The nav must stay 250px wide via the
	// automatic minimum (min(specified size suggestion, content size
	// suggestion)); the main content (flex-grow: 1, flex-basis: 0) takes the
	// rest.
	htmlContent := `
		<html>
			<head>
				<style>
					.article {
						display: flex;
						flex-direction: row-reverse;
						width: 1100px;
					}
					.main {
						flex-grow: 1;
						flex-basis: 0;
					}
					.sidenav {
						flex-basis: 0;
						width: 250px;
					}
					.navigation_box {
						width: 225px;
						margin-right: 25px;
					}
				</style>
			</head>
			<body>
				<div class="article">
					<div class="main">main content text</div>
					<div class="sidenav">
						<div class="navigation_box">nav content</div>
					</div>
				</div>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	styleManager := renderer.NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	layoutEngine := renderer.NewLayoutEngine(1200, 800)
	layoutEngine.ComputeLayout(renderTree)

	main := findLayoutByClass(t, layoutEngine, renderTree, "main")
	if main == nil {
		t.Fatal("main item not found")
	}
	sidenav := findLayoutByClass(t, layoutEngine, renderTree, "sidenav")
	if sidenav == nil {
		t.Fatal("sidenav item not found")
	}

	// The nav must not collapse below 250px (its width + child content).
	if sidenav.Box.Width < 249 || sidenav.Box.Width > 251 {
		t.Errorf("sidenav width = %f; want ~250px (automatic minimum, flex-basis: 0)", sidenav.Box.Width)
	}
	// main takes the remaining 1100 - 250 = 850px.
	if main.Box.Width < 848 || main.Box.Width > 852 {
		t.Errorf("main width = %f; want ~850px (1100 - 250px sidenav)", main.Box.Width)
	}
	if main.Box.Width+sidenav.Box.Width < 1098 || main.Box.Width+sidenav.Box.Width > 1102 {
		t.Errorf("main (%f) + sidenav (%f) should fill 1100px", main.Box.Width, sidenav.Box.Width)
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
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	styleManager := renderer.NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	layoutEngine := renderer.NewLayoutEngine(800, 600)
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
			style := &renderer.Style{}
			renderer.ParseFlexShorthand(tt.flexValue, style)

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
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	styleManager := renderer.NewStyleManager(stylesheet)
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
func findLayoutByClass(t *testing.T, le *renderer.LayoutEngine, renderTree *renderer.RenderNode, className string) *renderer.LayoutBox {
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

	flexboxFiles, err := filepath.Glob(filepath.Join(cwd, "..", "..", "..", "testdata", "test_*_flexbox.html"))
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

			r := renderer.NewRenderer(800, 600)
			obj, err := r.RenderHTML(context.Background(), string(content))
			if err != nil {
				t.Fatalf("RenderHTML failed for %s: %v", file, err)
			}
			if obj == nil {
				t.Fatalf("RenderHTML returned nil object for %s", file)
			}
			if r.CurrentLayoutTree() == nil {
				t.Fatalf("layout tree is nil for %s", file)
			}
			if r.GetContentHeight() <= 0 {
				t.Fatalf("expected positive content height for %s, got %f", file, r.GetContentHeight())
			}

			if countFlexNodes(r.CurrentRenderTree()) == 0 {
				t.Fatalf("expected at least one flex node in render tree for %s", file)
			}
		})
	}
}

// TestFlexColumnIndefiniteHeightJustifyContentNoop guards against the
// indefinite-column-height 10000px fallback: when a column flex container has
// no definite height, it shrink-wraps its content, so justify-content has no
// free space to distribute. flex-end/center must not push items ~10000px down
// (this broke Google's doodle homepage, whose logo span was laid out at y≈10066).
func TestFlexColumnIndefiniteHeightJustifyContentNoop(t *testing.T) {
	tests := []struct {
		name           string
		justifyContent string
	}{
		{"flex-end", "flex-end"},
		{"center", "center"},
		{"space-between", "space-between"},
		{"space-around", "space-around"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			htmlContent := `
				<html>
					<head>
						<style>
							.flex-container {
								display: flex;
								flex-direction: column;
								justify-content: ` + tt.justifyContent + `;
								width: 300px;
							}
							.item {
								height: 40px;
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
			renderTree := renderer.BuildRenderTree(findBodyNode(doc))
			styleManager := renderer.NewStyleManager(stylesheet)
			styleManager.ApplyStyles(renderTree)

			layoutEngine := renderer.NewLayoutEngine(800, 600)
			layoutEngine.ComputeLayout(renderTree)

			flexContainer := findLayoutByClass(t, layoutEngine, renderTree, "flex-container")
			if flexContainer == nil {
				t.Fatal("flex-container not found")
			}

			// The container has no explicit height: its height equals its
			// content. Flex items must start at the container's top.
			expectedTop := flexContainer.Box.Y
			for i, child := range flexContainer.Children {
				if child.Box.Y > expectedTop+60 {
					t.Errorf("%s: item %d pushed to y=%.0f (container top %.0f, height %.0f); should be near the top",
						tt.name, i, child.Box.Y, expectedTop, flexContainer.Box.Height)
				}
			}
			if flexContainer.Box.Height > 200 {
				t.Errorf("%s: container height %.0f is inflated; expected content height (~80-120px)",
					tt.name, flexContainer.Box.Height)
			}
		})
	}
}

func countFlexNodes(node *renderer.RenderNode) int {
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
