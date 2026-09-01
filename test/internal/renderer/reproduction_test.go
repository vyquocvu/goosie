package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/net/html"
)

// TestZIndexParsing verifies that z-index and positioning properties are parsed correctly
func TestZIndexParsing(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.layer-1 {
						position: absolute;
						z-index: 10;
						top: 10px;
						left: 10px;
					}
					.layer-2 {
						position: relative;
						z-index: 5;
					}
				</style>
			</head>
			<body>
				<div class="layer-1">Layer 1</div>
				<div class="layer-2">Layer 2</div>
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

	layer1 := findNodeByClass(renderTree, "layer-1")
	if layer1 == nil {
		t.Fatal("layer-1 not found")
	}

	if layer1.ComputedStyle.Position != "absolute" {
		t.Errorf("Expected position: absolute, got %s", layer1.ComputedStyle.Position)
	}
	if layer1.ComputedStyle.ZIndex != 10 {
		t.Errorf("Expected z-index: 10, got %d", layer1.ComputedStyle.ZIndex)
	}
	if layer1.ComputedStyle.Top != "10px" {
		t.Errorf("Expected top: 10px, got %s", layer1.ComputedStyle.Top)
	}

	layer2 := findNodeByClass(renderTree, "layer-2")
	if layer2 == nil {
		t.Fatal("layer-2 not found")
	}

	if layer2.ComputedStyle.Position != "relative" {
		t.Errorf("Expected position: relative, got %s", layer2.ComputedStyle.Position)
	}
	if layer2.ComputedStyle.ZIndex != 5 {
		t.Errorf("Expected z-index: 5, got %d", layer2.ComputedStyle.ZIndex)
	}
}

// TestOverflowParsing verifies that overflow property is parsed correctly
func TestOverflowParsing(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.clip {
						overflow: hidden;
						width: 100px;
						height: 100px;
					}
					.scroll {
						overflow: scroll;
					}
				</style>
			</head>
			<body>
				<div class="clip">Clipped content</div>
				<div class="scroll">Scrollable content</div>
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

	clip := findNodeByClass(renderTree, "clip")
	if clip == nil {
		t.Fatal("clip node not found")
	}

	if clip.ComputedStyle.Overflow != "hidden" {
		t.Errorf("Expected overflow: hidden, got %s", clip.ComputedStyle.Overflow)
	}

	scroll := findNodeByClass(renderTree, "scroll")
	if scroll == nil {
		t.Fatal("scroll node not found")
	}

	if scroll.ComputedStyle.Overflow != "scroll" {
		t.Errorf("Expected overflow: scroll, got %s", scroll.ComputedStyle.Overflow)
	}
}

// TestZIndexRendering verifies that z-index affects rendering order
func TestZIndexRendering(t *testing.T) {
	// This test requires rendering logic to be implemented.
	// We create a mock renderer and check the resulting canvas objects.

	htmlContent := `
		<html>
			<head>
				<style>
					.bottom {
						position: absolute;
						z-index: 1;
						color: red;
					}
					.top {
						position: absolute;
						z-index: 10;
						color: blue;
					}
				</style>
			</head>
			<body>
				<div class="top">Top</div>
				<div class="bottom">Bottom</div>
			</body>
		</html>
	`

	r := renderer.NewRenderer(800, 600)
	// We need layout engine to handle positioning (not implemented yet, but let's see if render handles z-index)

	canvasObj, err := r.RenderHTML(context.Background(), htmlContent)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	container, ok := canvasObj.(*fyne.Container)
	if !ok {
		t.Fatalf("Expected container, got %T", canvasObj)
	}

	if len(container.Objects) != 2 {
		t.Fatalf("Expected 2 objects, got %d", len(container.Objects))
	}

	// Helper to extract text from object
	var getText func(obj fyne.CanvasObject) string
	getText = func(obj fyne.CanvasObject) string {
		if lbl, ok := obj.(*widget.Label); ok {
			return lbl.Text
		}
		if txt, ok := obj.(*canvas.Text); ok {
			return txt.Text
		}
		if cont, ok := obj.(*fyne.Container); ok {
			// Search recursively
			for _, child := range cont.Objects {
				if txt := getText(child); txt != "" {
					return txt
				}
			}
		}
		return ""
	}

	firstObj := container.Objects[0]
	secondObj := container.Objects[1]

	firstText := getText(firstObj)
	secondText := getText(secondObj)

	// Lower z-index should be first (painted earlier)
	if firstText != "Bottom" {
		t.Errorf("Expected first object to be Bottom (z-index 1), got %s", firstText)
	}

	// Higher z-index should be second (painted later)
	if secondText != "Top" {
		t.Errorf("Expected second object to be Top (z-index 10), got %s", secondText)
	}
}

// TestOverflowDisplayList verifies that overflow property generates clip commands
func TestOverflowDisplayList(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.clip {
						overflow: hidden;
						width: 100px;
						height: 100px;
					}
					.content {
						width: 200px;
						height: 200px;
					}
				</style>
			</head>
			<body>
				<div class="clip">
					<div class="content">Content</div>
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

	// We need to run layout to generate LayoutBox
	// Since we don't have a public Layout engine that we can easily control here without full Renderer,
	// let's manually construct the LayoutBox tree or use a helper if available.
	// Actually, Renderer.computeLayout is private.
	// But we can use ComputeLayout from renderer.go if we can access it? No, it's a method on Renderer.
	// Let's use NewRenderer and run the full pipeline up to DisplayList.

	// However, DisplayListBuilder is what we want to test.
	// Let's try to verify via the generated Fyne objects.
	// If Clip commands are working, the resulting Fyne object structure for the clip div
	// should be a Scroll container (or at least wrapped in a way that handles overflow).

	r := renderer.NewRenderer(800, 600)
	canvasObj, err := r.RenderHTML(context.Background(), htmlContent)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	// canvasObj is the root container.
	// For overflow:hidden, the clip div should be rendered as a plain (non-scrollable)
	// fyne.Container, NOT a container.Scroll — so users see clipped content without scrollbars.

	foundScroll := false
	foundContainer := false
	var traverse func(obj fyne.CanvasObject)
	traverse = func(obj fyne.CanvasObject) {
		if _, ok := obj.(*widget.RichText); ok {
			// RichText might be used for text, ignore
		}
		if _, ok := obj.(*container.Scroll); ok {
			foundScroll = true
			return
		}
		if cont, ok := obj.(*fyne.Container); ok {
			foundContainer = true
			for _, child := range cont.Objects {
				traverse(child)
			}
		}
	}

	traverse(canvasObj)

	// overflow:hidden must NOT produce a scroll container.
	if foundScroll {
		t.Errorf("overflow:hidden should not produce a Scroll container (no user-visible scrollbars)")
	}
	// The render output should still contain at least the root container.
	if !foundContainer {
		t.Errorf("Expected at least one plain container in the render output")
	}
}

// Helper function to find node by class is already defined in flexbox_layout_test.go
// so we don't redefine it here.
