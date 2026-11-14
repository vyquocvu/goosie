package paint

import (
	"unicode/utf16"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func (b *Builder) paintText(obj *layout.Object, rect frame.Rect, opacity float32, clip *frame.Rect) {
	text := obj.Node.DataContent
	order := obj.GlyphOrder
	levels := obj.BidiLevels
	if b.composingTextareaText(obj) {
		text, order, levels = b.composeControlText(obj, text), nil, nil
	} else if b.composingEditableText(obj) {
		text, order, levels = b.composeTextAtUTF16Offset(text), nil, nil
	}
	if text == "" {
		return
	}
	if rect.Empty() {
		return
	}
	if clip != nil {
		rect = rect.Intersection(*clip)
		if rect.Empty() {
			return
		}
	}
	s := obj.Style
	if s == nil {
		return
	}
	if marks := b.find[obj.Key]; len(marks) > 0 {
		b.paintFindHighlights(marks, opacity, clip)
	}
	if sp, ok := b.selection[obj.Key]; ok {
		b.paintSelectionHighlight(obj, rect, s, sp, opacity)
	}
	if len(s.TextShadow) > 0 {
		b.paintTextShadow(text, rect, s, opacity, order, levels)
	}
	b.appendRunOrdered(text, rect, s, convertColor(s.Color), opacity, order, levels)
}

func (b *Builder) composingEditableText(obj *layout.Object) bool {
	return b.focus.object != nil && b.focus.marked != "" && b.focus.compositionNode != 0 && obj != nil && obj.Node != nil && uint32(obj.Node.ID) == b.focus.compositionNode
}

func (b *Builder) composeTextAtUTF16Offset(text string) string {
	runes := []rune(text)
	index, units := 0, 0
	for index < len(runes) && units < b.focus.compositionOffset {
		length := utf16.RuneLen(runes[index])
		if length < 0 {
			length = 1
		}
		units += length
		index++
	}
	marked := []rune(b.focus.marked)
	out := make([]rune, 0, len(runes)+len(marked))
	out = append(out, runes[:index]...)
	out = append(out, marked...)
	out = append(out, runes[index:]...)
	return string(out)
}

func (b *Builder) composingTextareaText(obj *layout.Object) bool {
	if b.focus.object == nil || b.focus.marked == "" || obj.Node == nil || b.focus.object.Node == nil {
		return false
	}
	if b.focus.object.Node.Data != "textarea" {
		return false
	}
	if obj.Node.Parent != b.focus.object.Node || !obj.Node.Text() {
		return false
	}
	for c := b.focus.object.Node.FirstChild; c != nil; c = c.NextSibling {
		if c.Text() {
			return obj.Node.ID == c.ID
		}
	}
	return false
}

func (b *Builder) appendRun(text string, rect frame.Rect, s *style.ComputedStyle, color frame.Color, opacity float32) {
	b.appendRunOrdered(text, rect, s, color, opacity, nil, nil)
}

func (b *Builder) appendRunOrdered(text string, rect frame.Rect, s *style.ComputedStyle, color frame.Color, opacity float32, order []int, levels []byte) {
	fontSize := int32(s.FontSize * b.scale)
	if fontSize <= 0 {
		return
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return
	}
	slot := s.FontSlot()
	baseline := rect.Y0
	if b.metrics != nil {
		ascent, _, _ := b.metrics.LineMetrics(fontSize, slot)
		baseline += ascent
	} else {
		baseline += int32(s.FontSize * b.scale * 0.8)
	}
	letterSpacing := int32(s.LetterSpacing * b.scale)
	wordSpacing := int32(s.WordSpacing * b.scale)
	glyphs := make([]GlyphRun, 0, len(runes))
	if b.metrics != nil {
		pen := int32(0)
		for k := range runes {
			logical := logicalRune(order, k)
			r := runes[logical]
			drawRune := mirrorRune(r, bidiLevel(levels, k))
			x := rect.X0 + (pen+32)>>6
			glyphs = append(glyphs, GlyphRun{Rune: drawRune, X: x, Y: baseline, Size: fontSize, Slot: slot})
			pen += b.metrics.GlyphAdvanceFixed(fontSize, r, slot)
			if k < len(runes)-1 {
				pen += letterSpacing * 64
			}
			if r == ' ' {
				pen += wordSpacing * 64
			}
		}
	} else {
		x := rect.X0
		advance := fontSize / 2
		for k := range runes {
			logical := logicalRune(order, k)
			r := runes[logical]
			drawRune := mirrorRune(r, bidiLevel(levels, k))
			glyphs = append(glyphs, GlyphRun{Rune: drawRune, X: x, Y: baseline, Size: fontSize, Slot: slot})
			x += advance
			if k < len(runes)-1 {
				x += letterSpacing
			}
			if r == ' ' {
				x += wordSpacing
			}
		}
	}
	b.list.Append(DisplayCmd{
		Kind: CmdText,
		Rect: rect,
		Text: TextRun{
			Glyphs: glyphs,
			Color:  color,
		},
		Opacity: opacity,
	})
	if s.TextDecoration&style.TextDecorationUnderline != 0 && !isBlankText(text) {
		thick := (s.FontSize * b.scale) / 16
		if s.TextDecorationThickness > 0 {
			thick = s.TextDecorationThickness * b.scale
		}
		if thick < 1 {
			thick = 1
		}
		offset := float32(thick)
		if s.TextUnderlineOffset != 0 {
			offset = s.TextUnderlineOffset * b.scale
		}
		y0 := baseline + int32(offset)
		b.list.Append(DisplayCmd{
			Kind:    CmdFill,
			Rect:    frame.Rect4(rect.X0, y0, rect.X1, y0+int32(thick)),
			Color:   color,
			Opacity: opacity,
		})
	}
}

func isBlankText(text string) bool {
	for _, r := range text {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

func logicalRune(order []int, k int) int {
	if k >= len(order) {
		return k
	}
	return order[k]
}

func bidiLevel(levels []byte, k int) byte {
	if k >= len(levels) {
		return 0
	}
	return levels[k]
}

func (b *Builder) paintTextShadow(text string, rect frame.Rect, s *style.ComputedStyle, opacity float32, order []int, levels []byte) {
	for _, sh := range s.TextShadow {
		dx := int32(sh.OffsetX * b.scale)
		dy := int32(sh.OffsetY * b.scale)
		shadowRect := frame.Rect4(
			rect.X0+dx,
			rect.Y0+dy,
			rect.X1+dx,
			rect.Y1+dy,
		)
		if shadowRect.Empty() {
			continue
		}
		color := convertColor(sh.Color)
		if sh.ColorIsCurrent {
			color = convertColor(s.Color)
		}
		b.appendRunOrdered(text, shadowRect, s, color, opacity, order, levels)
	}
}
