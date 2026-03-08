package renderer

import (
	"testing"
)

func TestNewInlineLayoutEngine(t *testing.T) {
	fontMetrics := NewFontMetrics(16.0)
	ile := NewInlineLayoutEngine(fontMetrics, 16.0)
	
	if ile == nil {
		t.Fatal("NewInlineLayoutEngine returned nil")
	}
	if ile.fontMetrics == nil {
		t.Error("fontMetrics not initialized")
	}
	if ile.defaultFontSize != 16.0 {
		t.Errorf("Expected defaultFontSize 16.0, got %f", ile.defaultFontSize)
	}
}

func TestCollapseWhiteSpace(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single space", "hello world", "hello world"},
		{"multiple spaces", "hello    world", "hello world"},
		{"tabs", "hello\t\tworld", "hello world"},
		{"newlines", "hello\n\nworld", "hello world"},
		{"mixed whitespace", "hello \t\n  world", "hello world"},
		{"leading whitespace", "  hello world", "hello world"},
		{"trailing whitespace", "hello world  ", "hello world"},
		{"only whitespace", "   \t\n  ", ""},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ile.collapseWhiteSpace(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCollapseWhiteSpacePreserveNewlines(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single line", "hello world", "hello world"},
		{"two lines", "hello\nworld", "hello\nworld"},
		{"spaces and newlines", "hello  world\nfoo  bar", "hello world\nfoo bar"},
		{"multiple newlines", "hello\n\nworld", "hello\n\nworld"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ile.collapseWhiteSpacePreserveNewlines(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSplitTextForWrapping(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"single word", "hello", []string{"hello"}},
		{"two words", "hello world", []string{"hello", "world"}},
		{"multiple words", "the quick brown fox", []string{"the", "quick", "brown", "fox"}},
		{"empty string", "", []string{}},
		{"only spaces", "   ", []string{}},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ile.splitTextForWrapping(tt.input, WhiteSpaceNormal)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d words, got %d", len(tt.expected), len(result))
				return
			}
			for i, word := range result {
				if word != tt.expected[i] {
					t.Errorf("Word %d: expected %q, got %q", i, tt.expected[i], word)
				}
			}
		})
	}
}

func TestProcessWhiteSpace(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	input := "hello   \n  world"
	
	tests := []struct {
		mode     WhiteSpaceMode
		expected string
	}{
		{WhiteSpaceNormal, "hello world"},
		{WhiteSpaceNoWrap, "hello world"},
		{WhiteSpacePre, "hello   \n  world"},
		{WhiteSpacePreWrap, "hello   \n  world"},
		{WhiteSpacePreLine, "hello\nworld"},
	}
	
	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			result := ile.processWhiteSpace(input, tt.mode)
			if result != tt.expected {
				t.Errorf("Mode %v: expected %q, got %q", tt.mode, tt.expected, result)
			}
		})
	}
}

func TestLayoutInlineContentSimple(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	// Create a simple paragraph with text
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"
	
	text := NewRenderNode(NodeTypeText)
	text.Text = "Hello World"
	text.Parent = p
	p.AddChild(text)
	
	// Layout inline content
	lines, totalHeight := ile.LayoutInlineContent(p, 0, 0, 400, WhiteSpaceNormal)
	
	if len(lines) == 0 {
		t.Fatal("Expected at least one line")
	}
	if totalHeight <= 0 {
		t.Error("Expected totalHeight > 0")
	}
	
	// Check first line
	line := lines[0]
	if len(line.InlineBoxes) == 0 {
		t.Error("Expected inline boxes in first line")
	}
	if line.AvailableWidth != 400 {
		t.Errorf("Expected available width 400, got %f", line.AvailableWidth)
	}
}

func TestLayoutInlineContentWithWrapping(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	// Create a paragraph with text that should wrap
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"
	
	text := NewRenderNode(NodeTypeText)
	text.Text = "This is a very long piece of text that should definitely wrap onto multiple lines when the available width is small"
	text.Parent = p
	p.AddChild(text)
	
	// Layout with narrow width to force wrapping
	lines, totalHeight := ile.LayoutInlineContent(p, 0, 0, 100, WhiteSpaceNormal)
	
	if len(lines) <= 1 {
		t.Errorf("Expected multiple lines due to wrapping, got %d", len(lines))
	}
	if totalHeight <= 20 { // Should be taller than a single line
		t.Errorf("Expected totalHeight > 20, got %f", totalHeight)
	}
	
	// Check that each line has content
	for i, line := range lines {
		if len(line.InlineBoxes) == 0 {
			t.Errorf("Line %d has no inline boxes", i)
		}
	}
}

func TestLayoutInlineContentMultipleTextNodes(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	// Create a paragraph with multiple text nodes
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"
	
	text1 := NewRenderNode(NodeTypeText)
	text1.Text = "Hello"
	text1.Parent = p
	p.AddChild(text1)
	
	text2 := NewRenderNode(NodeTypeText)
	text2.Text = " "
	text2.Parent = p
	p.AddChild(text2)
	
	text3 := NewRenderNode(NodeTypeText)
	text3.Text = "World"
	text3.Parent = p
	p.AddChild(text3)
	
	// Layout inline content
	lines, _ := ile.LayoutInlineContent(p, 0, 0, 400, WhiteSpaceNormal)
	
	if len(lines) == 0 {
		t.Fatal("Expected at least one line")
	}
	
	// Should have inline boxes for the text content
	totalBoxes := 0
	for _, line := range lines {
		totalBoxes += len(line.InlineBoxes)
	}
	
	if totalBoxes == 0 {
		t.Error("Expected inline boxes for text nodes")
	}
}

func TestLayoutInlineContentWithInlineElements(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	// Create a paragraph with inline elements
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"
	
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
	text2.Text = " text"
	text2.Parent = p
	p.AddChild(text2)
	
	// Layout inline content
	lines, _ := ile.LayoutInlineContent(p, 0, 0, 400, WhiteSpaceNormal)
	
	if len(lines) == 0 {
		t.Fatal("Expected at least one line")
	}
	
	// Should have inline boxes for all text pieces
	totalBoxes := 0
	for _, line := range lines {
		totalBoxes += len(line.InlineBoxes)
	}
	
	if totalBoxes < 3 {
		t.Errorf("Expected at least 3 inline boxes, got %d", totalBoxes)
	}
}

func TestFinalizeLine(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	line := ile.newLineBox(0, 0, 400, "left")
	
	// Add some inline boxes with different metrics
	box1 := &InlineBox{
		X:             0,
		Y:             0,
		Width:         50,
		Height:        20,
		Ascent:        15,
		Descent:       5,
		VerticalAlign: VerticalAlignBaseline,
	}
	
	box2 := &InlineBox{
		X:             60,
		Y:             0,
		Width:         60,
		Height:        24,
		Ascent:        18,
		Descent:       6,
		VerticalAlign: VerticalAlignBaseline,
	}
	
	line.InlineBoxes = append(line.InlineBoxes, box1, box2)
	line.Ascent = 18  // Max ascent
	line.Descent = 6  // Max descent
	
	ile.finalizeLine(line)
	
	// Check line height
	expectedHeight := float32(24) // 18 + 6
	if line.Height != expectedHeight {
		t.Errorf("Expected line height %f, got %f", expectedHeight, line.Height)
	}
	
	// Check that Y positions were adjusted
	if box1.Y == 0 && box2.Y == 0 {
		t.Error("Expected Y positions to be adjusted during finalization")
	}
}

func TestVerticalAlignment(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	tests := []struct {
		name  string
		align VerticalAlign
		checkY func(y float32) bool
	}{
		{"baseline", VerticalAlignBaseline, func(y float32) bool { return y >= 0 }},
		{"top", VerticalAlignTop, func(y float32) bool { return y >= 0 }},
		{"bottom", VerticalAlignBottom, func(y float32) bool { return y >= 0 }},
		{"middle", VerticalAlignMiddle, func(y float32) bool { return y >= 0 }},
		{"sub", VerticalAlignSub, func(y float32) bool { return y >= 0 }},
		{"super", VerticalAlignSuper, func(y float32) bool { return true }}, // Can be negative
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := ile.newLineBox(0, 0, 400, "left")
			line.Ascent = 15
			line.Descent = 5
			
			box := &InlineBox{
				X:             0,
				Width:         50,
				Height:        20,
				Ascent:        15,
				Descent:       5,
				VerticalAlign: tt.align,
			}
			
			line.InlineBoxes = append(line.InlineBoxes, box)
			ile.finalizeLine(line)
			
			// Verify Y position is valid according to test expectation
			if !tt.checkY(box.Y) {
				t.Errorf("Y position %f is invalid for alignment %s", box.Y, tt.name)
			}
		})
	}
}

func TestIsInlineBlock(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	tests := []struct {
		tagName  string
		expected bool
	}{
		{"img", true},
		{"button", true},
		{"input", true},
		{"select", true},
		{"span", false},
		{"strong", false},
		{"em", false},
		{"div", false},
		{"p", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.tagName, func(t *testing.T) {
			node := NewRenderNode(NodeTypeElement)
			node.TagName = tt.tagName
			
			result := ile.isInlineBlock(node)
			if result != tt.expected {
				t.Errorf("isInlineBlock(%s) = %v, expected %v", tt.tagName, result, tt.expected)
			}
		})
	}
}

func TestLayoutInlineContentEmptyText(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	// Create a paragraph with empty/whitespace-only text
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"
	
	text := NewRenderNode(NodeTypeText)
	text.Text = "   \n\t  "
	text.Parent = p
	p.AddChild(text)
	
	// Layout inline content
	lines, totalHeight := ile.LayoutInlineContent(p, 0, 0, 400, WhiteSpaceNormal)
	
	// Should collapse to nothing
	if totalHeight > 0 {
		t.Errorf("Expected totalHeight = 0 for whitespace-only text, got %f", totalHeight)
	}
	
	// Lines might exist but should be empty
	for i, line := range lines {
		if len(line.InlineBoxes) > 0 {
			t.Errorf("Line %d should have no inline boxes for whitespace-only text", i)
		}
	}
}

func TestGetFontSizeForNode(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	// Create nodes with different parent tags
	tests := []struct {
		parentTag    string
		expectedSize float32
	}{
		{"h1", 32.0},   // 16 * 2.0
		{"h2", 24.0},   // 16 * 1.5
		{"h3", 18.72},  // 16 * 1.17
		{"p", 16.0},    // 16 * 1.0
		{"div", 16.0},  // default
	}
	
	for _, tt := range tests {
		t.Run(tt.parentTag, func(t *testing.T) {
			parent := NewRenderNode(NodeTypeElement)
			parent.TagName = tt.parentTag
			
			child := NewRenderNode(NodeTypeText)
			child.Text = "test"
			child.Parent = parent
			
			fontSize := ile.getFontSizeForNode(child)
			if fontSize != tt.expectedSize {
				t.Errorf("Expected font size %f for parent %s, got %f", tt.expectedSize, tt.parentTag, fontSize)
			}
		})
	}
}

func TestCharacterBreaking(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)
	
	// Create a paragraph with a very long word
	p := NewRenderNode(NodeTypeElement)
	p.TagName = "p"
	
	text := NewRenderNode(NodeTypeText)
	text.Text = "verylongwordthatcannotfitonasinglelineshouldbebrokenatcharacterboundaries"
	text.Parent = p
	p.AddChild(text)
	
	// Layout with narrow width to force character breaking
	lines, totalHeight := ile.LayoutInlineContent(p, 0, 0, 50, WhiteSpaceNormal)
	
	if len(lines) <= 1 {
		t.Errorf("Expected multiple lines due to character breaking, got %d", len(lines))
	}
	if totalHeight <= 20 {
		t.Errorf("Expected totalHeight > 20 for broken text, got %f", totalHeight)
	}
	
	// Verify each line has content
	for i, line := range lines {
		if len(line.InlineBoxes) == 0 {
			t.Errorf("Line %d has no inline boxes", i)
		}
		// Verify line doesn't exceed available width significantly
		if line.Width > line.AvailableWidth*1.1 { // Allow 10% tolerance
			t.Errorf("Line %d width %f exceeds available width %f", i, line.Width, line.AvailableWidth)
		}
	}
}

// Helper method to provide a string representation for WhiteSpaceMode
func (mode WhiteSpaceMode) String() string {
	switch mode {
	case WhiteSpaceNormal:
		return "normal"
	case WhiteSpaceNoWrap:
		return "nowrap"
	case WhiteSpacePre:
		return "pre"
	case WhiteSpacePreWrap:
		return "pre-wrap"
	case WhiteSpacePreLine:
		return "pre-line"
	default:
		return "unknown"
	}
}
