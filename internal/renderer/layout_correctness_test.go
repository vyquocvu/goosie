package renderer

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// helper to parse HTML, apply CSS, and run layout.
func runLayout(t *testing.T, htmlStr string, cssStr string) (*LayoutEngine, *LayoutBox, *RenderNode) {
	parser := css.NewParser(cssStr)
	stylesheet, err := parser.Parse()
	if err != nil {
		t.Fatalf("CSS parsing failed: %v", err)
	}

	renderTree, err := parseHTMLToRenderTree(htmlStr)
	if err != nil {
		t.Fatalf("HTML parsing failed: %v", err)
	}

	styleManager := NewStyleManager(stylesheet)
	styleManager.ApplyStyles(renderTree)

	layoutEngine := NewLayoutEngine(800, 600)
	layoutRoot := layoutEngine.ComputeLayout(renderTree)
	return layoutEngine, layoutRoot, renderTree
}

func TestBoxSizing(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
	<html>
	<body>
		<div class="content-box">Content</div>
		<div class="border-box">Border</div>
	</body>
	</html>`

	cssStr := `
		.content-box {
			width: 100px;
			height: 50px;
			padding: 10px;
			border: 5px solid black;
			box-sizing: content-box;
		}
		.border-box {
			width: 100px;
			height: 50px;
			padding: 10px;
			border: 5px solid black;
			box-sizing: border-box;
		}
	`

	le, _, renderTree := runLayout(t, htmlStr, cssStr)

	// Verify content-box
	contentBoxNode := findNodeByClass(renderTree, "content-box")
	if contentBoxNode == nil {
		t.Fatal("content-box node not found")
	}
	contentLayoutBox := le.GetLayoutBox(contentBoxNode.ID)
	if contentLayoutBox == nil {
		t.Fatal("content-box layout box not found")
	}
	// border-box width = width (100) + padding-left (10) + padding-right (10) + border-left (5) + border-right (5) = 130
	if contentLayoutBox.Box.Width != 130 {
		t.Errorf("content-box border-box width = %f; want 130", contentLayoutBox.Box.Width)
	}
	// border-box height = height (50) + padding-top (10) + padding-bottom (10) + border-top (5) + border-bottom (5) = 80
	if contentLayoutBox.Box.Height != 80 {
		t.Errorf("content-box border-box height = %f; want 80", contentLayoutBox.Box.Height)
	}

	// Verify border-box
	borderBoxNode := findNodeByClass(renderTree, "border-box")
	if borderBoxNode == nil {
		t.Fatal("border-box node not found")
	}
	borderLayoutBox := le.GetLayoutBox(borderBoxNode.ID)
	if borderLayoutBox == nil {
		t.Fatal("border-box layout box not found")
	}
	// border-box width = width = 100
	if borderLayoutBox.Box.Width != 100 {
		t.Errorf("border-box width = %f; want 100", borderLayoutBox.Box.Width)
	}
	// border-box height = height = 50
	if borderLayoutBox.Box.Height != 50 {
		t.Errorf("border-box height = %f; want 50", borderLayoutBox.Box.Height)
	}
}

func TestMarginCollapseSibling(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
	<html>
	<body>
		<div class="block1">Box1</div>
		<div class="block2">Box2</div>
	</body>
	</html>`

	cssStr := `
		.block1 {
			margin-bottom: 20px;
			height: 50px;
		}
		.block2 {
			margin-top: 15px;
			height: 50px;
		}
	`

	le, _, renderTree := runLayout(t, htmlStr, cssStr)

	node1 := findNodeByClass(renderTree, "block1")
	node2 := findNodeByClass(renderTree, "block2")

	if node1 == nil || node2 == nil {
		t.Fatal("nodes not found")
	}

	box1 := le.GetLayoutBox(node1.ID)
	box2 := le.GetLayoutBox(node2.ID)

	t.Logf("box1: Y=%f, Height=%f, MarginBottom=%f, Float=%q, Position=%q", box1.Box.Y, box1.Box.Height, box1.MarginBottom, box1.Float, box1.Position)
	t.Logf("box2: Y=%f, Height=%f, MarginTop=%f, Float=%q, Position=%q", box2.Box.Y, box2.Box.Height, box2.MarginTop, box2.Float, box2.Position)

	// Collapsed margin = max(20, 15) = 20
	expectedY2 := box1.Box.Y + box1.Box.Height + 20
	if box2.Box.Y != expectedY2 {
		t.Errorf("block2 Y = %f; want %f (margin-collapsed)", box2.Box.Y, expectedY2)
	}
}

func TestMinMaxWidthConstraints(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
	<html>
	<body>
		<div class="min-w">Min</div>
		<div class="max-w">Max</div>
	</body>
	</html>`

	cssStr := `
		.min-w {
			width: 50px;
			min-width: 150px;
		}
		.max-w {
			width: 500px;
			max-width: 250px;
		}
	`

	le, _, renderTree := runLayout(t, htmlStr, cssStr)

	nodeMin := findNodeByClass(renderTree, "min-w")
	nodeMax := findNodeByClass(renderTree, "max-w")

	boxMin := le.GetLayoutBox(nodeMin.ID)
	boxMax := le.GetLayoutBox(nodeMax.ID)

	if boxMin.Box.Width != 150 {
		t.Errorf("min-w width = %f; want 150 (constrained by min-width)", boxMin.Box.Width)
	}
	if boxMax.Box.Width != 250 {
		t.Errorf("max-w width = %f; want 250 (constrained by max-width)", boxMax.Box.Width)
	}
}

func TestRelativePositioning(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
	<html>
	<body>
		<div class="relative-box">Relative</div>
	</body>
	</html>`

	cssStr := `
		.relative-box {
			position: relative;
			top: 15px;
			left: 25px;
			height: 50px;
		}
	`

	le, _, renderTree := runLayout(t, htmlStr, cssStr)
	node := findNodeByClass(renderTree, "relative-box")
	box := le.GetLayoutBox(node.ID)

	// Baseline body margin is 8px. Normal X is 8, Y is 8.
	// Offset top: 15, left: 25 => X = 8+25 = 33, Y = 8+15 = 23
	if box.Box.X != 33 {
		t.Errorf("relative X = %f; want 33", box.Box.X)
	}
	if box.Box.Y != 23 {
		t.Errorf("relative Y = %f; want 23", box.Box.Y)
	}
}

func TestFloatsAndWrapping(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
	<html>
	<body>
		<div class="container">
			<div class="left-float">Float</div>
			<p class="text-p">This is wrapping text.</p>
		</div>
	</body>
	</html>`

	cssStr := `
		.container {
			width: 400px;
		}
		.left-float {
			float: left;
			width: 100px;
			height: 100px;
			margin-right: 10px;
		}
		.text-p {
			margin: 0;
		}
	`

	le, _, renderTree := runLayout(t, htmlStr, cssStr)
	floatNode := findNodeByClass(renderTree, "left-float")
	pNode := findNodeByClass(renderTree, "text-p")

	if floatNode == nil || pNode == nil {
		t.Fatal("nodes not found")
	}

	boxFloat := le.GetLayoutBox(floatNode.ID)
	boxP := le.GetLayoutBox(pNode.ID)



	// Float is placed at childX (8 UA body margin)
	if boxFloat.Box.X != 8 {
		t.Errorf("Float X = %f; want 8", boxFloat.Box.X)
	}
	if boxFloat.Box.Y != 8 {
		t.Errorf("Float Y = %f; want 8", boxFloat.Box.Y)
	}

	// The text-p box itself is in-flow, so its X starts at 8.
	if boxP.Box.X != 8 {
		t.Errorf("Paragraph X = %f; want 8", boxP.Box.X)
	}

	// But its inline content line boxes should be pushed to the right of the float:
	// left offset = 8 (body margin) + 100 (float width) + 10 (float margin-right) = 118
	if len(boxP.LineBoxes) == 0 {
		t.Fatal("Expected paragraph to have line boxes")
	}
	firstLine := boxP.LineBoxes[0]
	if firstLine.X < 118 {
		t.Errorf("First line X = %f; expected >= 118 to wrap around the float", firstLine.X)
	}
}
