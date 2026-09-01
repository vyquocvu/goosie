package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"testing"

	"fyne.io/fyne/v2"
)

func TestNewFontMetrics(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)
	if fm == nil {
		t.Fatal("NewFontMetrics returned nil")
	}
	if fm.DefaultFontSize() != 16.0 {
		t.Errorf("Expected default font size 16.0, got %f", fm.DefaultFontSize())
	}
}

func TestMeasureText(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	tests := []struct {
		name     string
		text     string
		fontSize float32
		style    fyne.TextStyle
	}{
		{"simple text", "Hello World", 16.0, fyne.TextStyle{}},
		{"bold text", "Bold Text", 16.0, fyne.TextStyle{Bold: true}},
		{"italic text", "Italic Text", 16.0, fyne.TextStyle{Italic: true}},
		{"larger font", "Large", 24.0, fyne.TextStyle{}},
		{"smaller font", "Small", 12.0, fyne.TextStyle{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := fm.MeasureText(tt.text, tt.fontSize, tt.style, 0)

			// Width should be greater than 0 for non-empty text
			if metrics.Width <= 0 {
				t.Errorf("Expected width > 0, got %f", metrics.Width)
			}

			// Height should be greater than 0
			if metrics.Height <= 0 {
				t.Errorf("Expected height > 0, got %f", metrics.Height)
			}

			// Ascent should be positive
			if metrics.Ascent <= 0 {
				t.Errorf("Expected ascent > 0, got %f", metrics.Ascent)
			}

			// Descent should be positive
			if metrics.Descent <= 0 {
				t.Errorf("Expected descent > 0, got %f", metrics.Descent)
			}

			// Ascent + Descent should approximately equal font size
			expectedHeight := tt.fontSize
			actualHeight := metrics.Ascent + metrics.Descent
			if actualHeight < expectedHeight*0.8 || actualHeight > expectedHeight*1.2 {
				t.Errorf("Ascent+Descent (%f) should be close to font size (%f)", actualHeight, expectedHeight)
			}
		})
	}
}

func TestMeasureTextEmpty(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	metrics := fm.MeasureText("", 16.0, fyne.TextStyle{}, 0)

	if metrics.Width != 0 {
		t.Errorf("Expected width 0 for empty text, got %f", metrics.Width)
	}
	if metrics.Height != 0 {
		t.Errorf("Expected height 0 for empty text, got %f", metrics.Height)
	}
}

func TestMeasureTextWithWrapping(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	// Test with text that should wrap
	text := "This is a long text that should wrap to multiple lines when constrained by width"
	maxWidth := float32(200.0)

	metrics := fm.MeasureTextWithWrapping(text, 16.0, fyne.TextStyle{}, 0, maxWidth)

	// Width should not exceed maxWidth
	if metrics.Width > maxWidth {
		t.Errorf("Wrapped width (%f) should not exceed maxWidth (%f)", metrics.Width, maxWidth)
	}

	// Height should be greater than single line height (indicating wrapping occurred)
	singleLine := fm.MeasureText(text, 16.0, fyne.TextStyle{}, 0)
	if singleLine.Width > maxWidth && metrics.Height <= singleLine.Height {
		t.Error("Text should wrap to multiple lines and have greater height")
	}
}

func TestMeasureTextWithWrappingShortText(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	// Test with text that fits on one line
	text := "Short"
	maxWidth := float32(500.0)

	metrics := fm.MeasureTextWithWrapping(text, 16.0, fyne.TextStyle{}, 0, maxWidth)
	singleLine := fm.MeasureText(text, 16.0, fyne.TextStyle{}, 0)

	// Should be the same as single line measurement
	if metrics.Width != singleLine.Width {
		t.Errorf("Expected width %f, got %f", singleLine.Width, metrics.Width)
	}
	if metrics.Height != singleLine.Height {
		t.Errorf("Expected height %f, got %f", singleLine.Height, metrics.Height)
	}
}

func TestMeasureTextWithWrappingEmpty(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	metrics := fm.MeasureTextWithWrapping("", 16.0, fyne.TextStyle{}, 0, 200.0)

	if metrics.Width != 0 {
		t.Errorf("Expected width 0 for empty text, got %f", metrics.Width)
	}
	if metrics.Height != 0 {
		t.Errorf("Expected height 0 for empty text, got %f", metrics.Height)
	}
}

func TestGetFontSize(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	tests := []struct {
		tagName      string
		expectedSize float32
	}{
		{"h1", 32.0},
		{"h2", 24.0},
		{"h3", 18.72},
		{"h4", 16.0},
		{"h5", 13.28},
		{"h6", 10.72},
		{"p", 16.0},
		{"div", 16.0},
		{"span", 16.0},
	}

	for _, tt := range tests {
		t.Run(tt.tagName, func(t *testing.T) {
			size := fm.GetFontSize(tt.tagName)
			if size != tt.expectedSize {
				t.Errorf("Expected font size %f for %s, got %f", tt.expectedSize, tt.tagName, size)
			}
		})
	}
}

func TestGetTextStyle(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	tests := []struct {
		tagName        string
		expectedBold   bool
		expectedItalic bool
		expectedMono   bool
	}{
		{"h1", true, false, false},
		{"h2", true, false, false},
		{"h3", true, false, false},
		{"strong", true, false, false},
		{"b", true, false, false},
		{"em", false, true, false},
		{"i", false, true, false},
		{"code", false, false, true},
		{"pre", false, false, true},
		{"p", false, false, false},
		{"div", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.tagName, func(t *testing.T) {
			style := fm.GetTextStyle(tt.tagName)
			if style.Bold != tt.expectedBold {
				t.Errorf("Expected Bold=%v for %s, got %v", tt.expectedBold, tt.tagName, style.Bold)
			}
			if style.Italic != tt.expectedItalic {
				t.Errorf("Expected Italic=%v for %s, got %v", tt.expectedItalic, tt.tagName, style.Italic)
			}
			if style.Monospace != tt.expectedMono {
				t.Errorf("Expected Monospace=%v for %s, got %v", tt.expectedMono, tt.tagName, style.Monospace)
			}
		})
	}
}

func TestGetTextStyleFromNode(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	// Test simple node
	t.Run("simple bold", func(t *testing.T) {
		node := renderer.NewRenderNode(renderer.NodeTypeElement)
		node.TagName = "strong"

		style := fm.GetTextStyleFromNode(node)
		if !style.Bold {
			t.Error("Expected bold style for strong element")
		}
	})

	// Test nested nodes
	t.Run("nested bold and italic", func(t *testing.T) {
		parent := renderer.NewRenderNode(renderer.NodeTypeElement)
		parent.TagName = "strong"

		child := renderer.NewRenderNode(renderer.NodeTypeElement)
		child.TagName = "em"
		parent.AddChild(child)

		style := fm.GetTextStyleFromNode(child)
		if !style.Bold {
			t.Error("Expected bold style from parent strong element")
		}
		if !style.Italic {
			t.Error("Expected italic style from em element")
		}
	})

	// Test text node with styled parent
	t.Run("text in bold parent", func(t *testing.T) {
		parent := renderer.NewRenderNode(renderer.NodeTypeElement)
		parent.TagName = "b"

		textNode := renderer.NewRenderNode(renderer.NodeTypeText)
		textNode.Text = "Bold text"
		parent.AddChild(textNode)

		style := fm.GetTextStyleFromNode(textNode)
		if !style.Bold {
			t.Error("Expected bold style inherited from parent")
		}
	})
}

func TestSplitIntoWords(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"simple", "hello world", []string{"hello", "world"}},
		{"multiple spaces", "hello  world", []string{"hello", "world"}},
		{"with newline", "hello\nworld", []string{"hello", "world"}},
		{"with tab", "hello\tworld", []string{"hello", "world"}},
		{"single word", "hello", []string{"hello"}},
		{"empty", "", []string{}},
		{"only spaces", "   ", []string{}},
		{"trailing space", "hello ", []string{"hello"}},
		{"leading space", " hello", []string{"hello"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderer.SplitIntoWords(tt.text)

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

func TestFontMetricsConsistency(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	// Measure the same text multiple times - should get consistent results
	text := "Test Text"
	fontSize := float32(16.0)
	style := fyne.TextStyle{}

	m1 := fm.MeasureText(text, fontSize, style, 0)
	m2 := fm.MeasureText(text, fontSize, style, 0)

	if m1.Width != m2.Width {
		t.Errorf("Inconsistent width measurements: %f vs %f", m1.Width, m2.Width)
	}
	if m1.Height != m2.Height {
		t.Errorf("Inconsistent height measurements: %f vs %f", m1.Height, m2.Height)
	}
}

func TestFontSizeScaling(t *testing.T) {
	fm := renderer.NewFontMetrics(16.0)

	text := "Test"
	style := fyne.TextStyle{}

	// Measure at different font sizes
	m1 := fm.MeasureText(text, 16.0, style, 0)
	m2 := fm.MeasureText(text, 32.0, style, 0)

	// Larger font should have larger dimensions
	if m2.Width <= m1.Width {
		t.Errorf("32pt text width (%f) should be > 16pt text width (%f)", m2.Width, m1.Width)
	}
	if m2.Height <= m1.Height {
		t.Errorf("32pt text height (%f) should be > 16pt text height (%f)", m2.Height, m1.Height)
	}

	// Ascent and descent should scale with font size
	if m2.Ascent <= m1.Ascent {
		t.Errorf("32pt ascent (%f) should be > 16pt ascent (%f)", m2.Ascent, m1.Ascent)
	}
	if m2.Descent <= m1.Descent {
		t.Errorf("32pt descent (%f) should be > 16pt descent (%f)", m2.Descent, m1.Descent)
	}
}
