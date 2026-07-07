package renderer

import (
	"testing"
)

func TestNewDisplayList(t *testing.T) {
	dl := NewDisplayList()

	if dl == nil {
		t.Fatal("NewDisplayList returned nil")
	}
	if dl.Commands == nil {
		t.Error("Commands slice not initialized")
	}
	if len(dl.Commands) != 0 {
		t.Errorf("Expected empty command list, got %d commands", len(dl.Commands))
	}
}

func TestDisplayListAddCommand(t *testing.T) {
	dl := NewDisplayList()

	cmd := &PaintCommand{
		Type:   PaintText,
		NodeID: 1,
		Text:   "Hello",
	}

	dl.AddCommand(cmd)

	if len(dl.Commands) != 1 {
		t.Errorf("Expected 1 command, got %d", len(dl.Commands))
	}
	if dl.Commands[0] != cmd {
		t.Error("Command not added correctly")
	}
}

func TestDisplayListClear(t *testing.T) {
	dl := NewDisplayList()

	// Add some commands
	dl.AddCommand(&PaintCommand{Type: PaintText, NodeID: 1})
	dl.AddCommand(&PaintCommand{Type: PaintRect, NodeID: 2})

	if len(dl.Commands) != 2 {
		t.Errorf("Expected 2 commands before clear, got %d", len(dl.Commands))
	}

	dl.Clear()

	if len(dl.Commands) != 0 {
		t.Errorf("Expected 0 commands after clear, got %d", len(dl.Commands))
	}
}

func TestDisplayListBuilderBuildEmpty(t *testing.T) {
	dlb := NewDisplayListBuilder()

	// Test with nil inputs
	dl := dlb.Build(nil, nil)
	if dl == nil {
		t.Fatal("Build returned nil")
	}
	if len(dl.Commands) != 0 {
		t.Errorf("Expected 0 commands for nil inputs, got %d", len(dl.Commands))
	}
}

func TestDisplayListBuilderBuildSimple(t *testing.T) {
	dlb := NewDisplayListBuilder()

	// Create a simple render tree
	renderNode := NewRenderNode(NodeTypeText)
	renderNode.Text = "Hello World"

	// Create a layout box
	layoutBox := NewLayoutBox(renderNode.ID)
	layoutBox.Box = Rect{X: 10, Y: 20, Width: 100, Height: 30}

	// Build display list
	dl := dlb.Build(layoutBox, renderNode)

	if len(dl.Commands) != 1 {
		t.Fatalf("Expected 1 command, got %d", len(dl.Commands))
	}

	cmd := dl.Commands[0]
	if cmd.Type != PaintText {
		t.Errorf("Expected PaintText command, got %v", cmd.Type)
	}
	if cmd.Text != "Hello World" {
		t.Errorf("Expected text 'Hello World', got '%s'", cmd.Text)
	}
	if cmd.NodeID != renderNode.ID {
		t.Errorf("Expected NodeID %d, got %d", renderNode.ID, cmd.NodeID)
	}
}

func TestDisplayListBuilderBuildWithChildren(t *testing.T) {
	dlb := NewDisplayListBuilder()

	// Create a render tree with parent and children
	parent := NewRenderNode(NodeTypeElement)
	parent.TagName = "div"

	child1 := NewRenderNode(NodeTypeText)
	child1.Text = "First"
	parent.AddChild(child1)

	child2 := NewRenderNode(NodeTypeText)
	child2.Text = "Second"
	parent.AddChild(child2)

	// Create layout tree
	parentBox := NewLayoutBox(parent.ID)
	child1Box := NewLayoutBox(child1.ID)
	child1Box.Box = Rect{X: 0, Y: 0, Width: 100, Height: 20}
	child2Box := NewLayoutBox(child2.ID)
	child2Box.Box = Rect{X: 0, Y: 20, Width: 100, Height: 20}

	parentBox.AddChild(child1Box)
	parentBox.AddChild(child2Box)

	// Build display list
	dl := dlb.Build(parentBox, parent)

	// Should have 2 commands (one for each text node)
	if len(dl.Commands) != 2 {
		t.Fatalf("Expected 2 commands, got %d", len(dl.Commands))
	}

	// Verify first command
	if dl.Commands[0].Text != "First" {
		t.Errorf("Expected first command text 'First', got '%s'", dl.Commands[0].Text)
	}

	// Verify second command
	if dl.Commands[1].Text != "Second" {
		t.Errorf("Expected second command text 'Second', got '%s'", dl.Commands[1].Text)
	}
}

func TestDisplayListBuilderTextStyling(t *testing.T) {
	dlb := NewDisplayListBuilder()

	tests := []struct {
		name         string
		parentTag    string
		expectBold   bool
		expectItalic bool
	}{
		{"strong", "strong", true, false},
		{"bold", "b", true, false},
		{"emphasis", "em", false, true},
		{"italic", "i", false, true},
		{"heading", "h1", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create render tree
			parent := NewRenderNode(NodeTypeElement)
			parent.TagName = tt.parentTag

			child := NewRenderNode(NodeTypeText)
			child.Text = "Styled text"
			parent.AddChild(child)

			// Create layout tree
			parentBox := NewLayoutBox(parent.ID)
			childBox := NewLayoutBox(child.ID)
			parentBox.AddChild(childBox)

			// Build display list
			dl := dlb.Build(parentBox, parent)

			// Find text command
			var textCmd *PaintCommand
			for _, cmd := range dl.Commands {
				if cmd.Type == PaintText {
					textCmd = cmd
					break
				}
			}

			if textCmd == nil {
				t.Fatal("No text command found")
			}

			if textCmd.Bold != tt.expectBold {
				t.Errorf("Expected Bold=%v, got %v", tt.expectBold, textCmd.Bold)
			}
			if textCmd.Italic != tt.expectItalic {
				t.Errorf("Expected Italic=%v, got %v", tt.expectItalic, textCmd.Italic)
			}
		})
	}
}
