package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

type SelSpan struct {
	Key   layout.BoxKey
	Start int
	End   int
}

var selectionColor = frame.RGB(178, 215, 255)

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
	xa := runeX(obj.Node.DataContent, rect, s, start, b.scale, b.metrics, obj.GlyphOrder)
	xb := runeX(obj.Node.DataContent, rect, s, end, b.scale, b.metrics, obj.GlyphOrder)
	x0, x1 := xa, xb
	if x1 < x0 {
		x0, x1 = x1, x0
	}
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

func runeX(text string, rect frame.Rect, s *style.ComputedStyle, i int, scale float32, metrics layout.Metrics, order []int) int32 {
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
	if order != nil {
		return runeXBidi(runes, rect, fontSize, letterSpacing, wordSpacing, slot, i, metrics, order)
	}
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

func runeXBidi(runes []rune, rect frame.Rect, fontSize, letterSpacing, wordSpacing int32, slot frame.FontSlot, i int, metrics layout.Metrics, order []int) int32 {
	visualSlot := len(order)
	for k, log := range order {
		if log == i {
			visualSlot = k
			break
		}
	}
	if metrics != nil {
		pen := int32(0)
		for k := 0; k < visualSlot; k++ {
			r := runes[order[k]]
			pen += metrics.GlyphAdvanceFixed(fontSize, r, slot)
			if k < len(order)-1 {
				pen += letterSpacing * 64
			}
			if r == ' ' {
				pen += wordSpacing * 64
			}
		}
		return rect.X0 + (pen+32)>>6
	}
	pen := int32(0)
	for k := 0; k < visualSlot; k++ {
		r := runes[order[k]]
		pen += fontSize / 2
		if k < len(order)-1 {
			pen += letterSpacing
		}
		if r == ' ' {
			pen += wordSpacing
		}
	}
	return rect.X0 + pen
}
