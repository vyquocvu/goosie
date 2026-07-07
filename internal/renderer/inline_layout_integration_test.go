package renderer

import (
	"testing"
)

// TestInlineLayoutIntegration tests the complete inline layout pipeline
func TestInlineLayoutIntegration(t *testing.T) {
	// Create a paragraph with mixed inline content
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"

	// Add text with inline elements
	text1 := NewRenderNode(NodeTypeText)
	text1.Text = "This is "
	text1.Parent = p
	p.AddChild(text1)

	strong := NewRenderNode(NodeTypeElement)
	strong.TagName = "strong"
	strong.Parent = p
	p.AddChild(strong)

	strongText := NewRenderNode(NodeTypeText)
	strongText.Text = "bold"
	strongText.Parent = strong
	strong.AddChild(strongText)

	text2 := NewRenderNode(NodeTypeText)
	text2.Text = " and this is "
	text2.Parent = p
	p.AddChild(text2)

	em := NewRenderNode(NodeTypeElement)
	em.TagName = "em"
	em.Parent = p
	p.AddChild(em)

	emText := NewRenderNode(NodeTypeText)
	emText.Text = "italic"
	emText.Parent = em
	em.AddChild(emText)

	text3 := NewRenderNode(NodeTypeText)
	text3.Text = " text."
	text3.Parent = p
	p.AddChild(text3)

	// Create layout engine and compute layout
	le := NewLayoutEngine(400, 600)
	layoutRoot := le.ComputeLayout(p)

	if layoutRoot == nil {
		t.Fatal("ComputeLayout returned nil")
	}

	// Verify layout box was created
	if layoutRoot.NodeID != p.ID {
		t.Errorf("Expected NodeID %d, got %d", p.ID, layoutRoot.NodeID)
	}

	// Verify line boxes were created
	if len(layoutRoot.LineBoxes) == 0 {
		t.Error("Expected line boxes to be created")
	}

	// Verify inline boxes in line boxes
	totalInlineBoxes := 0
	for _, line := range layoutRoot.LineBoxes {
		totalInlineBoxes += len(line.InlineBoxes)
	}

	if totalInlineBoxes == 0 {
		t.Error("Expected inline boxes in line boxes")
	}

	// With the inline layout fix, inline content is NOT created as child LayoutBoxes
	// Instead, it's stored in LineBoxes
	// This is the correct behavior to avoid duplication bugs
	if len(layoutRoot.Children) != 0 {
		t.Error("Expected no child layout boxes for inline-only content")
	}
}

func TestInlineLayoutWithWrapping(t *testing.T) {
	// Create a paragraph with text that will wrap
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"

	text := NewRenderNode(NodeTypeText)
	text.Text = "This is a very long paragraph with many words that should wrap onto multiple lines when rendered with a narrow width constraint"
	text.Parent = p
	p.AddChild(text)

	// Create layout engine with narrow width
	le := NewLayoutEngine(200, 600)
	layoutRoot := le.ComputeLayout(p)

	if layoutRoot == nil {
		t.Fatal("ComputeLayout returned nil")
	}

	// Should have multiple line boxes due to wrapping
	if len(layoutRoot.LineBoxes) <= 1 {
		t.Errorf("Expected multiple line boxes due to wrapping, got %d", len(layoutRoot.LineBoxes))
	}

	// Each line should have content
	for i, line := range layoutRoot.LineBoxes {
		if len(line.InlineBoxes) == 0 {
			t.Errorf("Line %d has no content", i)
		}

		// Lines should not exceed available width (with small tolerance)
		if line.Width > line.AvailableWidth*1.05 {
			t.Errorf("Line %d width %f exceeds available width %f", i, line.Width, line.AvailableWidth)
		}
	}
}

func TestInlineLayoutWithDisplayList(t *testing.T) {
	// Create a simple paragraph
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"

	text := NewRenderNode(NodeTypeText)
	text.Text = "Hello World"
	text.Parent = p
	p.AddChild(text)

	// Create layout
	le := NewLayoutEngine(400, 600)
	layoutRoot := le.ComputeLayout(p)

	if layoutRoot == nil {
		t.Fatal("ComputeLayout returned nil")
	}

	// Build display list
	dlb := NewDisplayListBuilder()
	displayList := dlb.Build(layoutRoot, p)

	if displayList == nil {
		t.Fatal("Build returned nil display list")
	}

	// Should have paint commands for the text
	if len(displayList.Commands) == 0 {
		t.Error("Expected paint commands in display list")
	}

	// Verify text command exists
	hasTextCommand := false
	for _, cmd := range displayList.Commands {
		if cmd.Type == PaintText {
			hasTextCommand = true
			if cmd.Text == "" {
				t.Error("Text command has empty text")
			}
		}
	}

	if !hasTextCommand {
		t.Error("Expected at least one text paint command")
	}
}

func TestBlockWithInlineChildren(t *testing.T) {
	// Create a div with paragraph containing inline elements
	div := NewRenderNode(NodeTypeElement)
	div.TagName = "div"

	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"
	p.Parent = div
	div.AddChild(p)

	text := NewRenderNode(NodeTypeText)
	text.Text = "Paragraph text"
	text.Parent = p
	p.AddChild(text)

	// Create layout
	le := NewLayoutEngine(400, 600)
	layoutRoot := le.ComputeLayout(div)

	if layoutRoot == nil {
		t.Fatal("ComputeLayout returned nil")
	}

	// Div should be block
	if !layoutRoot.IsBlock() {
		t.Error("Div should be block-level")
	}

	// Should have one child (the paragraph)
	if len(layoutRoot.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(layoutRoot.Children))
	}

	// Paragraph should have line boxes
	pLayoutBox := layoutRoot.Children[0]
	if len(pLayoutBox.LineBoxes) == 0 {
		t.Error("Paragraph should have line boxes")
	}

	// With the inline layout fix, inline content is NOT created as child LayoutBoxes
	// This is the correct behavior to avoid duplication bugs
	if len(pLayoutBox.Children) != 0 {
		t.Error("Paragraph should NOT have inline child layout boxes - content is in LineBoxes")
	}
}

func TestMultipleParagraphs(t *testing.T) {
	// Create a div with multiple paragraphs
	div := NewRenderNode(NodeTypeElement)
	div.TagName = "div"

	for i := 0; i < 3; i++ {
		p := NewRenderNode(NodeTypeElement)
		p.TagName = "p"
		p.Parent = div
		div.AddChild(p)

		text := NewRenderNode(NodeTypeText)
		text.Text = "Paragraph text"
		text.Parent = p
		p.AddChild(text)
	}

	// Create layout
	le := NewLayoutEngine(400, 600)
	layoutRoot := le.ComputeLayout(div)

	if layoutRoot == nil {
		t.Fatal("ComputeLayout returned nil")
	}

	// Should have three paragraph layout boxes
	if len(layoutRoot.Children) != 3 {
		t.Errorf("Expected 3 children, got %d", len(layoutRoot.Children))
	}

	// Each paragraph should have line boxes
	for i, child := range layoutRoot.Children {
		if len(child.LineBoxes) == 0 {
			t.Errorf("Paragraph %d should have line boxes", i)
		}
	}

	// Paragraphs should be stacked vertically
	for i := 0; i < len(layoutRoot.Children)-1; i++ {
		child1 := layoutRoot.Children[i]
		child2 := layoutRoot.Children[i+1]

		if child2.Box.Y <= child1.Box.Y {
			t.Errorf("Paragraph %d should be below paragraph %d", i+1, i)
		}
	}
}

func TestEmptyParagraph(t *testing.T) {
	// Create an empty paragraph
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"

	// Create layout
	le := NewLayoutEngine(400, 600)
	layoutRoot := le.ComputeLayout(p)

	if layoutRoot == nil {
		t.Fatal("ComputeLayout returned nil")
	}

	// Empty paragraph should have minimal or zero height
	if layoutRoot.Box.Height > 50 { // Allow for some spacing
		t.Errorf("Empty paragraph has unexpected height: %f", layoutRoot.Box.Height)
	}
}

func TestWhitespaceOnlyParagraph(t *testing.T) {
	// Create a paragraph with only whitespace
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"

	text := NewRenderNode(NodeTypeText)
	text.Text = "   \n\t  "
	text.Parent = p
	p.AddChild(text)

	// Create layout
	le := NewLayoutEngine(400, 600)
	layoutRoot := le.ComputeLayout(p)

	if layoutRoot == nil {
		t.Fatal("ComputeLayout returned nil")
	}

	// Whitespace-only paragraph should collapse
	// Line boxes might exist but should be empty
	totalInlineBoxes := 0
	for _, line := range layoutRoot.LineBoxes {
		totalInlineBoxes += len(line.InlineBoxes)
	}

	if totalInlineBoxes > 0 {
		t.Error("Whitespace-only paragraph should not create inline boxes")
	}
}
