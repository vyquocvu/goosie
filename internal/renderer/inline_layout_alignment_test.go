package renderer

import (
	"testing"
)

func TestTextAlignment(t *testing.T) {
	ile := NewInlineLayoutEngine(NewFontMetrics(16.0), 16.0)

	tests := []struct {
		name      string
		textAlign string
		checkX    func(x float32) bool
	}{
		{"left", "left", func(x float32) bool { return x == 0 }},
		{"center", "center", func(x float32) bool { return x > 0 && x < 200 }}, // Should be centered in 400px width
		{"right", "right", func(x float32) bool { return x > 300 }},            // Should be right-aligned
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := ile.newLineBox(0, 0, 400, tt.textAlign, 0)

			box := &InlineBox{
				X:             0,
				Width:         50,
				Height:        20,
				Ascent:        15,
				Descent:       5,
				VerticalAlign: VerticalAlignBaseline,
			}

			line.InlineBoxes = append(line.InlineBoxes, box)
			line.Width = 50
			ile.finalizeLine(line)

			if !tt.checkX(box.X) {
				t.Errorf("X position %f is invalid for alignment %s (available: 400, width: 50)", box.X, tt.name)
			}
		})
	}
}
