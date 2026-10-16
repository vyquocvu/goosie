package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// SelSpan marks the rune range [Start, End) of one word box as selected.
// Obj points at the same layout object the builder's arena walk visits, so the
// engine can key spans by pointer identity without a second lookup.
type SelSpan struct {
	Obj   *layout.Object
	Start int
	End   int
}

// selectionColor is the unfocused-selection blue macOS text fields use. The
// text keeps its own color on top, matching how a real browser paints
// selection rather than inverting the glyph pixels.
var selectionColor = frame.RGB(178, 215, 255)

// paintSelectionHighlight fills the selected rune range of a word box with the
// selection color, under the text appendRun is about to emit.
func (b *Builder) paintSelectionHighlight(obj *layout.Object, rect frame.Rect, s *style.ComputedStyle, sp SelSpan, opacity float32) {
	runes := []rune(obj.Node.DataContent)
	start, end := sp.Start, sp.End
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start >= end {
		return
	}
	x0 := runeX(obj.Node.DataContent, rect, s, start, b.scale, b.metrics)
	x1 := runeX(obj.Node.DataContent, rect, s, end, b.scale, b.metrics)
	if x1 <= x0 {
		return
	}
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    frame.Rect{X0: x0, Y0: rect.Y0, X1: x1, Y1: rect.Y1},
		Color:   selectionColor,
		Opacity: opacity,
	})
}

// runeX returns the device-pixel x of the left edge of rune i, walking the
// same pen appendRun places glyphs with, so highlight edges line up with the
// glyph boundaries instead of a proportional guess.
func runeX(text string, rect frame.Rect, s *style.ComputedStyle, i int, scale float32, metrics layout.Metrics) int32 {
	runes := []rune(text)
	if i <= 0 {
		return rect.X0
	}
	if i > len(runes) {
		i = len(runes)
	}
	fontSize := int32(s.FontSize * scale)
	letterSpacing := int32(s.LetterSpacing * scale)
	wordSpacing := int32(s.WordSpacing * scale)
	slot := s.FontSlot()
	if metrics != nil {
		pen := int32(0)
		for k := 0; k < i; k++ {
			pen += metrics.GlyphAdvanceFixed(fontSize, runes[k], slot)
			if k < len(runes)-1 {
				pen += letterSpacing * 64
			}
			if runes[k] == ' ' {
				pen += wordSpacing * 64
			}
		}
		return rect.X0 + (pen+32)>>6
	}
	pen := int32(0)
	for k := 0; k < i; k++ {
		pen += fontSize / 2
		if k < len(runes)-1 {
			pen += letterSpacing
		}
		if runes[k] == ' ' {
			pen += wordSpacing
		}
	}
	return rect.X0 + pen
}
