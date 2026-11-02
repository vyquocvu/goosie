package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

// ValidationBubble is the message a blocked form submission shows beside the
// offending control. Obj is the control's layout box in document CSS pixels.
type ValidationBubble struct {
	Obj     *layout.Object
	Message string
}

// SetValidationBubble marks the constraint-validation message to paint next to
// the control. It paints after the document walk, so an ancestor's overflow
// never clips it.
func (b *Builder) SetValidationBubble(bubble *ValidationBubble) {
	b.validation = bubble
}

const (
	validationFontSize = 13
	validationPadX     = 8
	validationPadY     = 6
	validationGap      = 6
	validationRadius   = 4
	validationArrowW   = 10
	validationArrowH   = 5
	validationMargin   = 2
)

// paintValidationBubble draws the bubble above the control, or below it when
// the control sits too close to the top of the page. It is appended after the
// whole document so it always lands on top.
func (b *Builder) paintValidationBubble() {
	v := b.validation
	if v == nil || v.Obj == nil || v.Message == "" || b.scale <= 0 {
		return
	}
	root := b.arena.Get(layout.ObjectID(1))
	if root == nil {
		return
	}
	cx0, cy0, cx1, cy1 := v.Obj.BorderRect()
	if cx1 <= cx0 || cy1 <= cy0 {
		return
	}
	fontSize := int32(validationFontSize * b.scale)
	if fontSize <= 0 {
		return
	}
	slot := frame.FontSlot{}
	if v.Obj.Style != nil {
		slot = v.Obj.Style.FontSlot()
	}
	padX := int32(validationPadX * b.scale)
	padY := int32(validationPadY * b.scale)
	gap := int32(validationGap * b.scale)
	arrowW := int32(validationArrowW * b.scale)
	arrowH := int32(validationArrowH * b.scale)
	if arrowW < 2 {
		arrowW = 2
	}
	if arrowH < 1 {
		arrowH = 1
	}

	textW := b.validationTextWidth(v.Message, fontSize, slot)
	if textW <= 0 {
		return
	}
	ascent, descent := b.validationLineMetrics(fontSize, slot)
	textH := ascent + descent
	bubbleW := textW + 2*padX
	bubbleH := textH + 2*padY

	control := frame.RectF4(cx0, cy0, cx1, cy1).ToDevice(b.scale)
	pageX0, pageY0, pageX1, _ := root.BorderRect()
	pageLeft := int32(pageX0 * b.scale)
	pageRight := int32(pageX1 * b.scale)
	pageTop := int32(pageY0 * b.scale)

	x0 := (control.X0+control.X1)/2 - bubbleW/2
	minX := pageLeft + int32(validationMargin*b.scale)
	maxX := pageRight - int32(validationMargin*b.scale) - bubbleW
	if x0 > maxX {
		x0 = maxX
	}
	if x0 < minX {
		x0 = minX
	}

	above := true
	y1 := control.Y0 - gap
	y0 := y1 - bubbleH
	if y0 < pageTop {
		above = false
		y0 = control.Y1 + gap
		y1 = y0 + bubbleH
	}

	arrowX := (control.X0 + control.X1) / 2
	if arrowX < x0+arrowW {
		arrowX = x0 + arrowW
	}
	if arrowX > x0+bubbleW-arrowW {
		arrowX = x0 + bubbleW - arrowW
	}
	b.paintValidationArrow(arrowX, x0, x0+bubbleW, y0, y1, arrowW, arrowH, above)

	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    frame.Rect4(x0, y0, x0+bubbleW, y1),
		Color:   frame.RGB(60, 60, 60),
		Radius:  uniformCorners(float32(validationRadius * b.scale)),
		Opacity: 1,
	})

	glyphs := make([]GlyphRun, 0, len(v.Message))
	pen := int32(0)
	for _, r := range v.Message {
		x := x0 + padX + (pen+32)>>6
		glyphs = append(glyphs, GlyphRun{Rune: r, X: x, Y: y0 + padY + ascent, Size: fontSize, Slot: slot})
		pen += b.validationAdvance(fontSize, r, slot)
	}
	b.list.Append(DisplayCmd{
		Kind: CmdText,
		Rect: frame.Rect4(x0, y0, x0+bubbleW, y1),
		Text: TextRun{Glyphs: glyphs, Color: frame.RGB(255, 255, 255)},
	})
}

// paintValidationArrow joins the bubble to the control with a small triangle:
// pointing down when the bubble sits above, up when it sits below.
func (b *Builder) paintValidationArrow(cx, x0, x1, y0, y1, w, h int32, above bool) {
	if cx-w/2 < x0 {
		cx = x0 + w/2
	}
	if cx+w/2 > x1 {
		cx = x1 - w/2
	}
	for i := int32(0); i < h; i++ {
		var half int32
		if above {
			half = int32(float32(w) * float32(h-i) / float32(2*h))
			halfY := y1 + i
			b.paintValidationStrip(cx-half, halfY, cx+half, halfY+1)
			continue
		}
		half = int32(float32(w) * float32(i+1) / float32(2*h))
		halfY := y0 - h + i
		b.paintValidationStrip(cx-half, halfY, cx+half, halfY+1)
	}
}

func (b *Builder) paintValidationStrip(x0, y0, x1, y1 int32) {
	if x1 <= x0 || y1 <= y0 {
		return
	}
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    frame.Rect4(x0, y0, x1, y1),
		Color:   frame.RGB(60, 60, 60),
		Opacity: 1,
	})
}

func (b *Builder) validationTextWidth(text string, fontSize int32, slot frame.FontSlot) int32 {
	pen := int32(0)
	for _, r := range text {
		pen += b.validationAdvance(fontSize, r, slot)
	}
	return (pen + 32) >> 6
}

func (b *Builder) validationAdvance(fontSize int32, r rune, slot frame.FontSlot) int32 {
	if b.metrics != nil {
		return b.metrics.GlyphAdvanceFixed(fontSize, r, slot)
	}
	return fontSize / 2 * 64
}

func (b *Builder) validationLineMetrics(fontSize int32, slot frame.FontSlot) (ascent, descent int32) {
	if b.metrics != nil {
		if a, d, _ := b.metrics.LineMetrics(fontSize, slot); a > 0 {
			return a, d
		}
	}
	ascent = int32(float32(fontSize) * 0.8)
	return ascent, fontSize - ascent
}
