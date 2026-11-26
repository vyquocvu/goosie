package paint

import (
	"math"
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
	b.appendRunGlyphs(text, rect, s, color, opacity, order, levels, true)
}

func (b *Builder) appendRunGlyphs(text string, rect frame.Rect, s *style.ComputedStyle, color frame.Color, opacity float32, order []int, levels []byte, decorate bool) {
	fontSize := int32(s.FontSize * b.scale)
	if fontSize <= 0 {
		return
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return
	}
	slot := s.FontSlot()
	baseline := b.runBaseline(rect, s)
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
	if decorate {
		b.appendTextDecoration(text, rect, s, baseline, color, opacity, nil)
	}
}

func (b *Builder) runBaseline(rect frame.Rect, s *style.ComputedStyle) int32 {
	fontSize := int32(s.FontSize * b.scale)
	if b.metrics != nil {
		ascent, _, _ := b.metrics.LineMetrics(fontSize, s.FontSlot())
		return rect.Y0 + ascent
	}
	return rect.Y0 + int32(s.FontSize*b.scale*0.8)
}

func (b *Builder) appendTextDecoration(text string, rect frame.Rect, s *style.ComputedStyle, baseline int32, color frame.Color, opacity float32, clip *frame.Rect) {
	if s == nil || s.TextDecoration == style.TextDecorationNone || isBlankText(text) || b.scale <= 0 {
		return
	}
	fontSize := int32(s.FontSize * b.scale)
	if fontSize <= 0 {
		return
	}
	explicitThickness := s.TextDecorationThickness > 0
	thicknessCSS := s.FontSize / 16
	if explicitThickness {
		thicknessCSS = s.TextDecorationThickness
	}
	thickness := int32(thicknessCSS * b.scale)
	if thickness < 1 {
		thickness = 1
	}
	height := thickness
	barThickness := thickness
	gap := int32(0)
	switch s.TextDecorationStyle {
	case style.TextDecorationStyleDouble:
		if !explicitThickness {
			barThickness = int32(b.scale)
			if barThickness < 1 {
				barThickness = 1
			}
			gap = barThickness
			height = barThickness*2 + gap
		} else if thickness >= 3 {
			barThickness = thickness / 3
			if barThickness < 1 {
				barThickness = 1
			}
			gap = thickness - barThickness*2
			if gap < 1 {
				barThickness = (thickness - 1) / 2
				gap = 1
			}
			height = thickness
		}
	case style.TextDecorationStyleDotted, style.TextDecorationStyleDashed:
		if !explicitThickness {
			thickness = int32(2 * b.scale)
			if thickness < 1 {
				thickness = 1
			}
			height = thickness
		}
	case style.TextDecorationStyleWavy:
		waveHeight := int32(4 * b.scale)
		if waveHeight < 4 {
			waveHeight = 4
		}
		if height < waveHeight {
			height = waveHeight
		}
	}
	deco := color
	if s.TextDecorationColor.A > 0 {
		deco = convertColor(s.TextDecorationColor)
	}
	appendLine := func(top, doubleHeight, doubleGap int32) {
		switch s.TextDecorationStyle {
		case style.TextDecorationStyleDouble:
			if doubleGap == 0 {
				b.appendDecorationBar(frame.Rect4(rect.X0, top, rect.X1, top+doubleHeight), deco, opacity, clip)
				return
			}
			b.appendDecorationBar(frame.Rect4(rect.X0, top, rect.X1, top+barThickness), deco, opacity, clip)
			b.appendDecorationBar(frame.Rect4(rect.X0, top+barThickness+doubleGap, rect.X1, top+doubleHeight), deco, opacity, clip)
		case style.TextDecorationStyleDotted:
			b.appendDecorationPattern(rect.X0, rect.X1, top, height, thickness, thickness, deco, opacity, clip)
		case style.TextDecorationStyleDashed:
			b.appendDecorationPattern(rect.X0, rect.X1, top, height, int32(6*b.scale), int32(4*b.scale), deco, opacity, clip)
		case style.TextDecorationStyleWavy:
			b.appendDecorationWave(rect.X0, rect.X1, top, height, deco, opacity, clip)
		default:
			b.appendDecorationBar(frame.Rect4(rect.X0, top, rect.X1, top+height), deco, opacity, clip)
		}
	}
	doubleHeight, doubleGap := height, gap
	if !explicitThickness && s.TextDecorationStyle == style.TextDecorationStyleDouble {
		doubleHeight = barThickness * 4
		doubleGap = barThickness * 2
	}
	if s.TextDecoration&style.TextDecorationUnderline != 0 {
		offset := int32(0)
		if s.TextUnderlineOffset != 0 {
			offset = int32(s.TextUnderlineOffset * b.scale)
		} else if explicitThickness {
			offset = int32(math.Ceil(float64(thicknessCSS)/2) * float64(b.scale))
		} else {
			switch s.TextDecorationStyle {
			case style.TextDecorationStyleDotted, style.TextDecorationStyleDashed:
				offset = 0
			case style.TextDecorationStyleDouble:
				offset = int32(b.scale)
			case style.TextDecorationStyleWavy:
				offset = int32(2 * b.scale)
			default:
				offset = int32(math.Ceil(float64(thicknessCSS)/2) * float64(b.scale))
			}
		}
		appendLine(baseline+offset, doubleHeight, doubleGap)
	}
	if s.TextDecoration&style.TextDecorationOverline != 0 {
		appendLine(rect.Y0-doubleHeight, doubleHeight, doubleGap)
	}
	if s.TextDecoration&style.TextDecorationLineThrough != 0 {
		center := baseline - int32(s.FontSize*b.scale*5/16)
		top := center - height/2
		if s.TextDecorationStyle == style.TextDecorationStyleDouble {
			top = center
		}
		appendLine(top, height, gap)
	}
}

func (b *Builder) appendDecorationBar(rect frame.Rect, color frame.Color, opacity float32, clip *frame.Rect) {
	if clip != nil {
		rect = rect.Intersection(*clip)
	}
	if rect.Empty() {
		return
	}
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: rect, Color: color, Opacity: opacity})
}

func (b *Builder) appendDecorationPattern(x0, x1, y0, height, on, off int32, color frame.Color, opacity float32, clip *frame.Rect) {
	if on < 1 {
		on = 1
	}
	if off < 1 {
		off = 1
	}
	for x := x0; x < x1; x += on + off {
		end := x + on
		if end > x1 {
			end = x1
		}
		b.appendDecorationBar(frame.Rect4(x, y0, end, y0+height), color, opacity, clip)
	}
}

func (b *Builder) appendDecorationWave(x0, x1, y0, height int32, color frame.Color, opacity float32, clip *frame.Rect) {
	if height <= 1 || x1 <= x0 {
		b.appendDecorationBar(frame.Rect4(x0, y0, x1, y0+height), color, opacity, clip)
		return
	}
	period := int32(10 * b.scale)
	if period < 10 {
		period = 10
	}
	profile := [...]int32{3, 3, 3, 2, 1, 0, 0, 1, 2, 3}
	starts := make([]int32, height)
	for i := range starts {
		starts[i] = -1
	}
	for x := x0; x < x1; x++ {
		phase := (x - x0) % period
		row := profile[phase*10/period] * (height - 1) / 3
		for other, start := range starts {
			if start >= 0 && int32(other) != row {
				b.appendDecorationBar(frame.Rect4(start, y0+int32(other), x, y0+int32(other)+1), color, opacity, clip)
				starts[other] = -1
			}
		}
		if starts[row] < 0 {
			starts[row] = x
		}
	}
	for row, start := range starts {
		if start >= 0 {
			b.appendDecorationBar(frame.Rect4(start, y0+int32(row), x1, y0+int32(row)+1), color, opacity, clip)
		}
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
		b.appendRunGlyphs(text, shadowRect, s, color, opacity, order, levels, false)
	}
}
