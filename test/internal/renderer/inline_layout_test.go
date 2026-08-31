package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"testing"
)

func TestNewInlineLayoutEngine(t *testing.T) {
	fontMetrics := renderer.NewFontMetrics(16.0)
	ile := renderer.NewInlineLayoutEngine(fontMetrics, 16.0)

	if ile == nil {
		t.Fatal("NewInlineLayoutEngine returned nil")
	}
	if ile.FontMetrics() == nil {
		t.Error("fontMetrics not initialized")
	}
	if ile.DefaultFontSize() != 16.0 {
		t.Errorf("Expected defaultFontSize 16.0, got %f", ile.DefaultFontSize())
	}
}

func TestCollapseWhiteSpace(t *testing.T) {
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

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
			result := ile.CollapseWhiteSpace(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCollapseWhiteSpacePreserveNewlines(t *testing.T) {
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

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
			result := ile.CollapseWhiteSpacePreserveNewlines(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSplitTextForWrapping(t *testing.T) {
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

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
			result := ile.SplitTextForWrapping(tt.input, renderer.WhiteSpaceNormal)
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
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

	input := "hello   \n  world"

	tests := []struct {
		mode     renderer.WhiteSpaceMode
		expected string
	}{
		{renderer.WhiteSpaceNormal, "hello world"},
		{renderer.WhiteSpaceNoWrap, "hello world"},
		{renderer.WhiteSpacePre, "hello   \n  world"},
		{renderer.WhiteSpacePreWrap, "hello   \n  world"},
		{renderer.WhiteSpacePreLine, "hello\nworld"},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			result := ile.ProcessWhiteSpace(input, tt.mode)
			if result != tt.expected {
				t.Errorf("Mode %v: expected %q, got %q", tt.mode, tt.expected, result)
			}
		})
	}
}

func TestLayoutInlineContentSimple(t *testing.T) {
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

	// Create a simple paragraph with text
	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "Hello World"
	text.Parent = p
	p.AddChild(text)

	// Layout inline content
	lines, totalHeight := ile.LayoutInlineContent(p, 0, 0, 400, renderer.WhiteSpaceNormal, nil)

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
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

	// Create a paragraph with text that should wrap
	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "This is a very long piece of text that should definitely wrap onto multiple lines when the available width is small"
	text.Parent = p
	p.AddChild(text)

	// Layout with narrow width to force wrapping
	lines, totalHeight := ile.LayoutInlineContent(p, 0, 0, 100, renderer.WhiteSpaceNormal, nil)

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
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

	// Create a paragraph with multiple text nodes
	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"

	text1 := renderer.NewRenderNode(renderer.NodeTypeText)
	text1.Text = "Hello"
	text1.Parent = p
	p.AddChild(text1)

	text2 := renderer.NewRenderNode(renderer.NodeTypeText)
	text2.Text = " "
	text2.Parent = p
	p.AddChild(text2)

	text3 := renderer.NewRenderNode(renderer.NodeTypeText)
	text3.Text = "World"
	text3.Parent = p
	p.AddChild(text3)

	// Layout inline content
	lines, _ := ile.LayoutInlineContent(p, 0, 0, 400, renderer.WhiteSpaceNormal, nil)

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
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

	// Create a paragraph with inline elements
	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"

	text1 := renderer.NewRenderNode(renderer.NodeTypeText)
	text1.Text = "This is "
	text1.Parent = p
	p.AddChild(text1)

	strong := renderer.NewRenderNode(renderer.NodeTypeElement)
	strong.TagName = "strong"
	strong.Parent = p
	p.AddChild(strong)

	strongText := renderer.NewRenderNode(renderer.NodeTypeText)
	strongText.Text = "bold"
	strongText.Parent = strong
	strong.AddChild(strongText)

	text2 := renderer.NewRenderNode(renderer.NodeTypeText)
	text2.Text = " text"
	text2.Parent = p
	p.AddChild(text2)

	// Layout inline content
	lines, _ := ile.LayoutInlineContent(p, 0, 0, 400, renderer.WhiteSpaceNormal, nil)

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
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

	line := ile.NewLineBox(0, 0, 400, "left", 0)

	// Add some inline boxes with different metrics
	box1 := &renderer.InlineBox{
		X:             0,
		Y:             0,
		Width:         50,
		Height:        20,
		Ascent:        15,
		Descent:       5,
		VerticalAlign: renderer.VerticalAlignBaseline,
	}

	box2 := &renderer.InlineBox{
		X:             60,
		Y:             0,
		Width:         60,
		Height:        24,
		Ascent:        18,
		Descent:       6,
		VerticalAlign: renderer.VerticalAlignBaseline,
	}

	line.InlineBoxes = append(line.InlineBoxes, box1, box2)
	line.Ascent = 18 // Max ascent
	line.Descent = 6 // Max descent

	ile.FinalizeLine(line)

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
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

	tests := []struct {
		name   string
		align  renderer.VerticalAlign
		checkY func(y float32) bool
	}{
		{"baseline", renderer.VerticalAlignBaseline, func(y float32) bool { return y >= 0 }},
		{"top", renderer.VerticalAlignTop, func(y float32) bool { return y >= 0 }},
		{"bottom", renderer.VerticalAlignBottom, func(y float32) bool { return y >= 0 }},
		{"middle", renderer.VerticalAlignMiddle, func(y float32) bool { return y >= 0 }},
		{"sub", renderer.VerticalAlignSub, func(y float32) bool { return y >= 0 }},
		{"super", renderer.VerticalAlignSuper, func(y float32) bool { return true }}, // Can be negative
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := ile.NewLineBox(0, 0, 400, "left", 0)
			line.Ascent = 15
			line.Descent = 5

			box := &renderer.InlineBox{
				X:             0,
				Width:         50,
				Height:        20,
				Ascent:        15,
				Descent:       5,
				VerticalAlign: tt.align,
			}

			line.InlineBoxes = append(line.InlineBoxes, box)
			ile.FinalizeLine(line)

			// Verify Y position is valid according to test expectation
			if !tt.checkY(box.Y) {
				t.Errorf("Y position %f is invalid for alignment %s", box.Y, tt.name)
			}
		})
	}
}

func TestIsInlineBlock(t *testing.T) {
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

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
			node := renderer.NewRenderNode(renderer.NodeTypeElement)
			node.TagName = tt.tagName

			result := ile.IsInlineBlock(node)
			if result != tt.expected {
				t.Errorf("isInlineBlock(%s) = %v, expected %v", tt.tagName, result, tt.expected)
			}
		})
	}
}

func TestLayoutInlineContentEmptyText(t *testing.T) {
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

	// Create a paragraph with empty/whitespace-only text
	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "   \n\t  "
	text.Parent = p
	p.AddChild(text)

	// Layout inline content
	lines, totalHeight := ile.LayoutInlineContent(p, 0, 0, 400, renderer.WhiteSpaceNormal, nil)

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
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

	// Create nodes with different parent tags
	tests := []struct {
		parentTag    string
		expectedSize float32
	}{
		{"h1", 32.0},  // 16 * 2.0
		{"h2", 24.0},  // 16 * 1.5
		{"h3", 18.72}, // 16 * 1.17
		{"p", 16.0},   // 16 * 1.0
		{"div", 16.0}, // default
	}

	for _, tt := range tests {
		t.Run(tt.parentTag, func(t *testing.T) {
			parent := renderer.NewRenderNode(renderer.NodeTypeElement)
			parent.TagName = tt.parentTag

			child := renderer.NewRenderNode(renderer.NodeTypeText)
			child.Text = "test"
			child.Parent = parent

			fontSize := ile.GetFontSizeForNode(child)
			if fontSize != tt.expectedSize {
				t.Errorf("Expected font size %f for parent %s, got %f", tt.expectedSize, tt.parentTag, fontSize)
			}
		})
	}
}

func TestCharacterBreaking(t *testing.T) {
	ile := renderer.NewInlineLayoutEngine(renderer.NewFontMetrics(16.0), 16.0)

	// Create a paragraph with a very long word
	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "verylongwordthatcannotfitonasinglelineshouldbebrokenatcharacterboundaries"
	text.Parent = p
	p.AddChild(text)

	// Layout with narrow width to force character breaking
	lines, totalHeight := ile.LayoutInlineContent(p, 0, 0, 50, renderer.WhiteSpaceNormal, nil)

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

