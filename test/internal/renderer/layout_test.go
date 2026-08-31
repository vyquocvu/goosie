package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"testing"
)

func TestNewLayoutEngine(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)
	if le == nil {
		t.Fatal("NewLayoutEngine returned nil")
	}
	if le.CanvasWidth() != 800 {
		t.Errorf("Expected width 800, got %f", le.CanvasWidth())
	}
	if le.CanvasHeight() != 600 {
		t.Errorf("Expected height 600, got %f", le.CanvasHeight())
	}
}

func TestLayoutTextNode(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)
	node := renderer.NewRenderNode(renderer.NodeTypeText)
	node.Text = "Hello World"

	le.Layout(node)

	if node.Box.X != 0 {
		t.Errorf("Expected X=0, got %f", node.Box.X)
	}
	if node.Box.Y != 0 {
		t.Errorf("Expected Y=0, got %f", node.Box.Y)
	}
	if node.Box.Width != 800 {
		t.Errorf("Expected Width=800, got %f", node.Box.Width)
	}
	if node.Box.Height <= 0 {
		t.Error("Expected Height > 0")
	}
}

func TestLayoutElementNode(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)
	node := renderer.NewRenderNode(renderer.NodeTypeElement)
	node.TagName = "div"

	child := renderer.NewRenderNode(renderer.NodeTypeText)
	child.Text = "Content"
	node.AddChild(child)

	le.Layout(node)

	if node.Box.X != 0 {
		t.Errorf("Expected X=0, got %f", node.Box.X)
	}
	if node.Box.Width != 800 {
		t.Errorf("Expected Width=800, got %f", node.Box.Width)
	}
	if node.Box.Height <= 0 {
		t.Error("Expected Height > 0")
	}

	// Check child was laid out
	if child.Box.Width <= 0 {
		t.Error("Child box width not set")
	}
}

func TestLayoutHeading(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)

	tests := []struct {
		tagName      string
		expectedSize float32
	}{
		{"h1", le.DefaultFontSize() * 2.0},
		{"h2", le.DefaultFontSize() * 1.5},
		{"h3", le.DefaultFontSize() * 1.17},
		{"p", le.DefaultFontSize()},
	}

	for _, tt := range tests {
		t.Run(tt.tagName, func(t *testing.T) {
			size := le.GetFontSize(tt.tagName)
			if size != tt.expectedSize {
				t.Errorf("Expected font size %f for %s, got %f", tt.expectedSize, tt.tagName, size)
			}
		})
	}
}

func TestLayoutMultipleChildren(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)
	parent := renderer.NewRenderNode(renderer.NodeTypeElement)
	parent.TagName = "div"

	// Add multiple children
	for i := 0; i < 3; i++ {
		child := renderer.NewRenderNode(renderer.NodeTypeElement)
		child.TagName = "p"
		text := renderer.NewRenderNode(renderer.NodeTypeText)
		text.Text = "Paragraph text"
		child.AddChild(text)
		parent.AddChild(child)
	}

	le.Layout(parent)

	// Verify children are stacked vertically (non-overlapping)
	prevY := float32(0)
	for i, child := range parent.Children {
		if i > 0 && child.Box.Y < prevY {
			t.Errorf("Child %d overlaps previous child: Box.Y=%f < prevY=%f", i, child.Box.Y, prevY)
		}
		prevY = child.Box.Y + child.Box.Height
	}
}

func TestLayoutNestedElements(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)

	// Create nested structure: div > p > text
	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.TagName = "div"

	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "Nested text"

	p.AddChild(text)
	div.AddChild(p)

	le.Layout(div)

	// Verify all nodes have been laid out
	if div.Box.Width == 0 {
		t.Error("Div box width not set")
	}
	if p.Box.Width == 0 {
		t.Error("P box width not set")
	}
	if text.Box.Width == 0 {
		t.Error("Text box width not set")
	}
}

func TestComputeLayout(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)

	// Create a simple render tree
	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.TagName = "div"

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "Hello"
	div.AddChild(text)

	// Compute layout tree
	layoutRoot := le.ComputeLayout(div)

	if layoutRoot == nil {
		t.Fatal("ComputeLayout returned nil")
	}

	if layoutRoot.NodeID != div.ID {
		t.Errorf("Expected NodeID %d, got %d", div.ID, layoutRoot.NodeID)
	}

	if layoutRoot.Box.Width != 800 {
		t.Errorf("Expected width 800, got %f", layoutRoot.Box.Width)
	}

	// With the inline layout fix, text nodes are not created as child LayoutBoxes
	// Instead, they are stored in LineBoxes
	if len(layoutRoot.LineBoxes) == 0 {
		t.Error("Expected LineBoxes for inline content, got none")
	}
}

func TestComputeLayoutWithMultipleChildren(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)

	// Create render tree with multiple children
	parent := renderer.NewRenderNode(renderer.NodeTypeElement)
	parent.TagName = "div"

	for i := 0; i < 3; i++ {
		child := renderer.NewRenderNode(renderer.NodeTypeElement)
		child.TagName = "p"
		text := renderer.NewRenderNode(renderer.NodeTypeText)
		text.Text = "Paragraph"
		child.AddChild(text)
		parent.AddChild(child)
	}

	// Compute layout tree
	layoutRoot := le.ComputeLayout(parent)

	if len(layoutRoot.Children) != 3 {
		t.Errorf("Expected 3 children, got %d", len(layoutRoot.Children))
	}

	// Verify children are stacked vertically
	for i := 0; i < len(layoutRoot.Children)-1; i++ {
		child1 := layoutRoot.Children[i]
		child2 := layoutRoot.Children[i+1]

		if child2.Box.Y <= child1.Box.Y {
			t.Errorf("Child %d should be positioned below child %d", i+1, i)
		}
	}
}

func TestGetLayoutBox(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)

	// Create render tree
	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.TagName = "div"

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "Test"
	div.AddChild(text)

	// Compute layout
	layoutRoot := le.ComputeLayout(div)

	// Test GetLayoutBox
	divBox := le.GetLayoutBox(div.ID)
	if divBox == nil {
		t.Error("GetLayoutBox returned nil for div")
	}
	if divBox != layoutRoot {
		t.Error("GetLayoutBox returned wrong box for div")
	}

	textBox := le.GetLayoutBox(text.ID)
	if textBox == nil {
		t.Error("GetLayoutBox returned nil for text")
	}
	// For inline content (text nodes), GetLayoutBox returns the parent's layout box
	// since inline content doesn't have its own dedicated layout box
	if textBox.NodeID != div.ID {
		t.Errorf("Expected NodeID %d (parent div), got %d", div.ID, textBox.NodeID)
	}
}

func TestHitTest(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)

	// Create render tree with block children instead of inline
	// to test hit testing properly
	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.TagName = "div"

	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "Test"
	p.AddChild(text)
	div.AddChild(p)

	// Compute layout
	layoutRoot := le.ComputeLayout(div)

	// Manually set layout positions for testing
	layoutRoot.Box = renderer.Rect{X: 0, Y: 0, Width: 800, Height: 100}
	if len(layoutRoot.Children) > 0 {
		layoutRoot.Children[0].Box = renderer.Rect{X: 10, Y: 10, Width: 200, Height: 30}
	}

	tests := []struct {
		name       string
		x, y       float32
		expectedID int64
	}{
		{"hit child", 50, 20, p.ID},
		{"hit parent", 500, 50, div.ID},
		{"miss", 900, 200, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := le.HitTest(layoutRoot, tt.x, tt.y)
			if result != tt.expectedID {
				t.Errorf("HitTest(%f, %f) = %d, expected %d", tt.x, tt.y, result, tt.expectedID)
			}
		})
	}
}

func TestHitTestNested(t *testing.T) {
	le := renderer.NewLayoutEngine(800, 600)

	// Create nested render tree: div > p > text
	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.TagName = "div"

	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"
	div.AddChild(p)

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "Nested"
	p.AddChild(text)

	// Compute layout
	layoutRoot := le.ComputeLayout(div)

	// Manually set positions
	layoutRoot.Box = renderer.Rect{X: 0, Y: 0, Width: 800, Height: 200}
	if len(layoutRoot.Children) > 0 {
		layoutRoot.Children[0].Box = renderer.Rect{X: 50, Y: 50, Width: 300, Height: 100}
	}

	// Hit test on p
	result := le.HitTest(layoutRoot, 200, 80)
	if result != p.ID {
		t.Errorf("Expected p node ID %d, got %d", p.ID, result)
	}

	// Hit test on div but not on p
	result = le.HitTest(layoutRoot, 10, 10)
	if result != div.ID {
		t.Errorf("Expected div node ID %d, got %d", div.ID, result)
	}
}
