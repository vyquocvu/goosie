package paint

import (
	"unicode/utf16"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func (b *Builder) paintText(obj *layout.Object, rect frame.Rect, opacity float32, clip *frame.Rect) {
	text := obj.Node.DataContent
	if b.composingTextareaText(obj) {
		text = b.composeControlText(obj, text)
	} else if b.composingEditableText(obj) {
		text = b.composeTextAtUTF16Offset(text)
	}
	if text == "" {
		return
	}
	if rect.Empty() {
		return
	}
	// Apply overflow:hidden clip from an ancestor.
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
	if marks := b.find[obj]; len(marks) > 0 {
		b.paintFindHighlights(marks, opacity, clip)
	}
	if sp, ok := b.selection[obj]; ok {
		b.paintSelectionHighlight(obj, rect, s, sp, opacity)
	}
	// Text shadows paint behind the main text as offset copies.
	if len(s.TextShadow) > 0 {
		b.paintTextShadow(text, rect, s, opacity)
	}
	b.appendRun(text, rect, s, convertColor(s.Color), opacity)
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

// composingTextareaText reports whether obj paints the first text child of the
// focused textarea, the one text node a composition preview can splice into.
// The inline pass paints word copies of the node (same NodeID, new pointer), so
// match by ID rather than pointer identity.
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

// appendRun emits one line of glyphs across the box's content origin, spacing
// them with the same advances layout measured the line with.
func (b *Builder) appendRun(text string, rect frame.Rect, s *style.ComputedStyle, color frame.Color, opacity float32) {
	fontSize := int32(s.FontSize * b.scale)
	if fontSize <= 0 {
		return
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return
	}
	slot := s.FontSlot()
	// Layout positioned this box's content area, whose top is the line box top
	// plus half the leading. The baseline is one ascent further down, read from
	// the same face metrics layout used, so the glyph sits where the line box
	// says it does instead of a guessed fraction of the em.
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
		// The pen stays fractional across the run and only the glyph position
		// rounds, the way a browser places text: rounding each advance instead
		// loses a fraction of a pixel per character and drifts the line tail.
		pen := int32(0)
		for i, r := range runes {
			x := rect.X0 + (pen+32)>>6
			glyphs = append(glyphs, GlyphRun{Rune: r, X: x, Y: baseline, Size: fontSize, Slot: slot})
			pen += b.metrics.GlyphAdvanceFixed(fontSize, r, slot)
			if i < len(runes)-1 {
				pen += letterSpacing * 64
			}
			if r == ' ' {
				pen += wordSpacing * 64
			}
		}
	} else {
		x := rect.X0
		advance := fontSize / 2
		for i, r := range runes {
			glyphs = append(glyphs, GlyphRun{Rune: r, X: x, Y: baseline, Size: fontSize, Slot: slot})
			x += advance
			if i < len(runes)-1 {
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
	// Chromium puts the Times underline 2px below the baseline at 16px, one
	// pixel thick, and scales both with the font.
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

// paintTextShadow draws each text-shadow layer as an offset copy of the text
// in the shadow colour, painted before the main text so it appears behind.
func (b *Builder) paintTextShadow(text string, rect frame.Rect, s *style.ComputedStyle, opacity float32) {
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
		b.appendRun(text, shadowRect, s, color, opacity)
	}
}
