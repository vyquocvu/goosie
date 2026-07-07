package renderer

import (
	"testing"
)

func TestNewLayoutBox(t *testing.T) {
	nodeID := int64(123)
	lb := NewLayoutBox(nodeID)

	if lb == nil {
		t.Fatal("NewLayoutBox returned nil")
	}
	if lb.NodeID != nodeID {
		t.Errorf("Expected NodeID %d, got %d", nodeID, lb.NodeID)
	}
	if lb.Display != DisplayBlock {
		t.Errorf("Expected Display to be DisplayBlock, got %s", lb.Display)
	}
	if lb.Children == nil {
		t.Error("Children slice not initialized")
	}
}

func TestLayoutBoxAddChild(t *testing.T) {
	parent := NewLayoutBox(1)
	child := NewLayoutBox(2)

	parent.AddChild(child)

	if len(parent.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(parent.Children))
	}
	if parent.Children[0] != child {
		t.Error("Child not added correctly")
	}
}

func TestLayoutBoxIsBlock(t *testing.T) {
	lb := NewLayoutBox(1)
	lb.Display = DisplayBlock

	if !lb.IsBlock() {
		t.Error("Expected IsBlock to return true")
	}
	if lb.IsInline() {
		t.Error("Expected IsInline to return false")
	}
}

func TestLayoutBoxIsInline(t *testing.T) {
	lb := NewLayoutBox(1)
	lb.Display = DisplayInline

	if lb.IsBlock() {
		t.Error("Expected IsBlock to return false")
	}
	if !lb.IsInline() {
		t.Error("Expected IsInline to return true")
	}
}

func TestLayoutBoxGetContentBox(t *testing.T) {
	lb := NewLayoutBox(1)
	lb.Box = Rect{X: 10, Y: 20, Width: 100, Height: 50}
	lb.PaddingTop = 5
	lb.PaddingRight = 10
	lb.PaddingBottom = 5
	lb.PaddingLeft = 10

	contentBox := lb.GetContentBox()

	expectedX := float32(20)      // 10 + 10 (left padding)
	expectedY := float32(25)      // 20 + 5 (top padding)
	expectedWidth := float32(80)  // 100 - 10 - 10 (left + right padding)
	expectedHeight := float32(40) // 50 - 5 - 5 (top + bottom padding)

	if contentBox.X != expectedX {
		t.Errorf("Expected X=%f, got %f", expectedX, contentBox.X)
	}
	if contentBox.Y != expectedY {
		t.Errorf("Expected Y=%f, got %f", expectedY, contentBox.Y)
	}
	if contentBox.Width != expectedWidth {
		t.Errorf("Expected Width=%f, got %f", expectedWidth, contentBox.Width)
	}
	if contentBox.Height != expectedHeight {
		t.Errorf("Expected Height=%f, got %f", expectedHeight, contentBox.Height)
	}
}

func TestLayoutBoxContains(t *testing.T) {
	lb := NewLayoutBox(1)
	lb.Box = Rect{X: 10, Y: 20, Width: 100, Height: 50}

	tests := []struct {
		name     string
		x, y     float32
		expected bool
	}{
		{"inside", 50, 40, true},
		{"top-left corner", 10, 20, true},
		{"bottom-right corner", 110, 70, true},
		{"outside left", 5, 40, false},
		{"outside right", 115, 40, false},
		{"outside top", 50, 15, false},
		{"outside bottom", 50, 75, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lb.Contains(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("Contains(%f, %f) = %v, expected %v", tt.x, tt.y, result, tt.expected)
			}
		})
	}
}

func TestLayoutBoxMultipleChildren(t *testing.T) {
	parent := NewLayoutBox(1)

	for i := 0; i < 5; i++ {
		child := NewLayoutBox(int64(i + 2))
		parent.AddChild(child)
	}

	if len(parent.Children) != 5 {
		t.Errorf("Expected 5 children, got %d", len(parent.Children))
	}

	for i, child := range parent.Children {
		expectedID := int64(i + 2)
		if child.NodeID != expectedID {
			t.Errorf("Child %d: expected NodeID %d, got %d", i, expectedID, child.NodeID)
		}
	}
}
