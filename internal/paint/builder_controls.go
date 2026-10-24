package paint

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// paintPlaceholder draws the hint a control shows while it holds no value. The
// text is not in the DOM, so it never went through layout: it starts at the
// control's content origin and is bounded by that box.
func (b *Builder) paintPlaceholder(obj *layout.Object, opacity float32, clip *frame.Rect) {
	s := obj.Style
	if s == nil || obj.Node == nil || !obj.Node.Element() {
		return
	}
	text := obj.Node.GetAttribute("placeholder")
	if text == "" || obj.Node.GetAttribute("value") != "" {
		return
	}
	// A composition preview takes the placeholder's place on an empty control.
	if b.focus.object != nil && b.focus.object == obj && b.focus.marked != "" {
		return
	}
	x0, y0, x1, y1 := obj.ContentRect()
	rect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	if rect.Empty() {
		return
	}
	if clip != nil {
		rect = rect.Intersection(*clip)
		if rect.Empty() {
			return
		}
	}
	b.appendRun(text, rect, s, frame.RGB(117, 117, 117), opacity)
}

// composeControlText returns text with the composition preview spliced at the
// caret when obj shows the focused control's text — the input itself or a
// textarea's first text child. The caret bar stays before the marked text, so
// the preview moves with the caret without moving the caret.
func (b *Builder) composeControlText(obj *layout.Object, text string) string {
	if b.focus.object == nil || b.focus.marked == "" || obj.Node == nil {
		return text
	}
	isInput := obj == b.focus.object
	isTextareaText := obj.Node.Parent == b.focus.object.Node && obj.Node.Text()
	if !isInput && !isTextareaText {
		return text
	}
	runes := []rune(text)
	caret := b.focus.caret
	if caret > len(runes) {
		caret = len(runes)
	}
	if caret < 0 {
		caret = 0
	}
	marked := []rune(b.focus.marked)
	out := make([]rune, 0, len(runes)+len(marked))
	out = append(out, runes[:caret]...)
	out = append(out, marked...)
	out = append(out, runes[caret:]...)
	return string(out)
}

// paintControlValue draws the text an input holds. The value is not in the DOM
// text flow, so like the placeholder it starts at the content origin bounded
// by the content box. Textareas paint through the normal text path instead.
func (b *Builder) paintControlValue(obj *layout.Object, opacity float32, clip *frame.Rect) {
	s := obj.Style
	if s == nil || obj.Node == nil || !obj.Node.Element() {
		return
	}
	text := b.composeControlText(obj, obj.Node.GetAttribute("value"))
	if text == "" {
		return
	}
	typ := strings.ToLower(obj.Node.GetAttribute("type"))
	// Checkbox, radio, submit, reset, and button inputs paint through their
	// own specialised paths; skip them here.
	switch typ {
	case "checkbox", "radio", "submit", "reset", "button":
		return
	}
	if obj.Node.GetAttribute("type") == "password" {
		text = strings.Repeat("•", len([]rune(text)))
	}
	x0, y0, x1, y1 := obj.ContentRect()
	rect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	if rect.Empty() {
		return
	}
	if clip != nil {
		rect = rect.Intersection(*clip)
		if rect.Empty() {
			return
		}
	}
	b.appendRun(text, rect, s, frame.RGB(0, 0, 0), opacity)
}

// paintCheckbox draws a checkbox widget: a small square box with a 1px border.
// When checked, a checkmark is drawn inside.
func (b *Builder) paintCheckbox(obj *layout.Object, rect frame.Rect, opacity float32) {
	if rect.Empty() {
		return
	}
	// White background fill.
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    rect,
		Color:   frame.RGB(255, 255, 255),
		Opacity: opacity,
	})
	// 1px border.
	borderColor := frame.RGB(117, 117, 117)
	// Top
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X0, rect.Y0, rect.X1, rect.Y0+1), Color: borderColor, Opacity: opacity})
	// Bottom
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X0, rect.Y1-1, rect.X1, rect.Y1), Color: borderColor, Opacity: opacity})
	// Left
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X0, rect.Y0, rect.X0+1, rect.Y1), Color: borderColor, Opacity: opacity})
	// Right
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X1-1, rect.Y0, rect.X1, rect.Y1), Color: borderColor, Opacity: opacity})

	// If checked, draw a checkmark.
	if obj.Node != nil && obj.Node.HasAttribute("checked") {
		checkColor := frame.RGB(0, 0, 0)
		// Apply accent-color if set and has alpha > 0
		if obj.Style != nil && obj.Style.AccentColor.A > 0 {
			checkColor = frame.RGBA(obj.Style.AccentColor.R, obj.Style.AccentColor.G, obj.Style.AccentColor.B, 255)
		}
		// Simple checkmark: a small V shape drawn as filled pixels.
		cx := (rect.X0 + rect.X1) / 2
		cy := (rect.Y0 + rect.Y1) / 2
		// Draw a simple checkmark using a few filled rects.
		// Left stroke (going down-right):
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(cx-3, cy-1, cx-2, cy+1), Color: checkColor, Opacity: opacity})
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(cx-2, cy, cx-1, cy+2), Color: checkColor, Opacity: opacity})
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(cx-1, cy+1, cx, cy+3), Color: checkColor, Opacity: opacity})
		// Right stroke (going up-right):
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(cx, cy, cx+1, cy+2), Color: checkColor, Opacity: opacity})
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(cx+1, cy-1, cx+2, cy+1), Color: checkColor, Opacity: opacity})
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(cx+2, cy-2, cx+3, cy), Color: checkColor, Opacity: opacity})
	}
}

// paintRadio draws a radio button widget: a small circle with a 1px border.
// When checked, a filled dot is drawn inside.
func (b *Builder) paintRadio(obj *layout.Object, rect frame.Rect, opacity float32) {
	if rect.Empty() {
		return
	}
	// White background fill.
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    rect,
		Color:   frame.RGB(255, 255, 255),
		Opacity: opacity,
	})
	// Draw a circular border using small filled rects approximating a circle.
	borderColor := frame.RGB(117, 117, 117)
	cx := (rect.X0 + rect.X1) / 2
	cy := (rect.Y0 + rect.Y1) / 2
	rx := (rect.X1 - rect.X0) / 2
	ry := (rect.Y1 - rect.Y0) / 2
	// Draw the circle outline by filling pixels near the ellipse boundary.
	for dy := -ry; dy <= ry; dy++ {
		for dx := -rx; dx <= rx; dx++ {
			// Check if this pixel is on the border (near the ellipse edge).
			dist := float32(dx*dx)/float32(rx*rx) + float32(dy*dy)/float32(ry*ry)
			if dist >= 0.7 && dist <= 1.1 {
				b.list.Append(DisplayCmd{
					Kind:    CmdFill,
					Rect:    frame.Rect4(cx+dx, cy+dy, cx+dx+1, cy+dy+1),
					Color:   borderColor,
					Opacity: opacity,
				})
			}
		}
	}

	// If checked, draw a filled dot in the center.
	if obj.Node != nil && obj.Node.HasAttribute("checked") {
		dotColor := frame.RGB(0, 0, 0)
		// Apply accent-color if set and has alpha > 0
		if obj.Style != nil && obj.Style.AccentColor.A > 0 {
			dotColor = frame.RGBA(obj.Style.AccentColor.R, obj.Style.AccentColor.G, obj.Style.AccentColor.B, 255)
		}
		dotR := rx / 3
		if dotR < 1 {
			dotR = 1
		}
		for dy := -dotR; dy <= dotR; dy++ {
			for dx := -dotR; dx <= dotR; dx++ {
				if float32(dx*dx)+float32(dy*dy) <= float32(dotR*dotR)+1 {
					b.list.Append(DisplayCmd{
						Kind:    CmdFill,
						Rect:    frame.Rect4(cx+dx, cy+dy, cx+dx+1, cy+dy+1),
						Color:   dotColor,
						Opacity: opacity,
					})
				}
			}
		}
	}
}

// paintButtonLabel draws the value text of a button-like input (submit, reset,
// button) centered in its box.
func (b *Builder) paintButtonLabel(obj *layout.Object, opacity float32, clip *frame.Rect) {
	s := obj.Style
	if s == nil || obj.Node == nil {
		return
	}
	var text string
	if obj.Node.HasAttribute("value") {
		text = obj.Node.GetAttribute("value")
	} else {
		typ := strings.ToLower(obj.Node.GetAttribute("type"))
		switch typ {
		case "submit":
			text = "Submit"
		case "reset":
			text = "Reset"
		}
	}
	if text == "" {
		return
	}
	x0, y0, x1, y1 := obj.ContentRect()
	rect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	if rect.Empty() {
		return
	}
	if clip != nil {
		rect = rect.Intersection(*clip)
		if rect.Empty() {
			return
		}
	}
	// Center the text horizontally.
	fontSize := int32(s.FontSize * b.scale)
	if fontSize <= 0 {
		return
	}
	runes := []rune(text)
	textW := int32(0)
	slot := s.FontSlot()
	if b.metrics != nil {
		for _, r := range runes {
			textW += b.metrics.GlyphAdvance(fontSize, r, slot)
		}
	} else {
		textW = int32(len(runes)) * fontSize / 2
	}
	rectW := rect.X1 - rect.X0
	if textW < rectW {
		rect.X0 += (rectW - textW) / 2
	}
	b.appendRun(text, rect, s, frame.RGB(0, 0, 0), opacity)
}

// paintSelect draws the selected option text and a dropdown arrow indicator
// for a <select> element.
func (b *Builder) paintSelect(obj *layout.Object, opacity float32, clip *frame.Rect) {
	s := obj.Style
	if s == nil || obj.Node == nil {
		return
	}
	// Find the selected option.
	var text string
	var firstOption string
	for c := obj.Node.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() && c.Data == "option" {
			optText := c.TextContent()
			if firstOption == "" {
				firstOption = optText
			}
			if c.HasAttribute("selected") {
				text = optText
			}
		}
	}
	if text == "" {
		text = firstOption
	}
	if text == "" {
		return
	}
	x0, y0, x1, y1 := obj.ContentRect()
	rect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	if rect.Empty() {
		return
	}
	if clip != nil {
		rect = rect.Intersection(*clip)
		if rect.Empty() {
			return
		}
	}
	// Leave room for the dropdown arrow on the right.
	arrowW := int32(12 * b.scale)
	textRect := rect
	if rect.X1-rect.X0 > arrowW {
		textRect.X1 -= arrowW
	}
	b.appendRun(text, textRect, s, frame.RGB(0, 0, 0), opacity)

	// Draw a dropdown arrow (small triangle) on the right side.
	arrowCX := rect.X1 - arrowW/2
	arrowCY := (rect.Y0 + rect.Y1) / 2
	arrowColor := frame.RGB(80, 80, 80)
	// Simple downward triangle: 3 rows narrowing from top to bottom.
	for i := int32(0); i < 3; i++ {
		halfW := 3 - i
		b.list.Append(DisplayCmd{
			Kind:    CmdFill,
			Rect:    frame.Rect4(arrowCX-halfW, arrowCY-1+i, arrowCX+halfW, arrowCY+i),
			Color:   arrowColor,
			Opacity: opacity,
		})
	}
}

// paintFocus draws the focused control's caret and focus ring.
func (b *Builder) paintFocus(obj *layout.Object, clip *frame.Rect) {
	if obj.Node == nil || obj.Style == nil ||
		(obj.Node.Data != "input" && obj.Node.Data != "textarea") {
		return
	}
	b.paintCaret(obj, clip)
	b.paintFocusRing(obj)
}

// controlText returns the editable text of a form control: the value attribute
// for input, the first text child for textarea.
func controlText(obj *layout.Object) string {
	if obj.Node == nil {
		return ""
	}
	if obj.Node.Data == "input" {
		return obj.Node.GetAttribute("value")
	}
	for c := obj.Node.FirstChild; c != nil; c = c.NextSibling {
		if c.Text() {
			return c.DataContent
		}
	}
	return ""
}

// paintCaret draws the 1px bar where the next typed rune goes, one glyph
// advance past the value's first caret runes.
func (b *Builder) paintCaret(obj *layout.Object, clip *frame.Rect) {
	s := obj.Style
	if s == nil {
		return
	}
	runes := []rune(controlText(obj))
	caret := b.focus.caret
	if caret > len(runes) {
		caret = len(runes)
	}
	if caret < 0 {
		caret = 0
	}
	fontSize := int32(s.FontSize * b.scale)
	if fontSize <= 0 {
		return
	}
	slot := s.FontSlot()
	pen := int32(0)
	for _, r := range runes[:caret] {
		if b.metrics != nil {
			pen += b.metrics.GlyphAdvanceFixed(fontSize, r, slot)
		} else {
			pen += fontSize / 2 * 64
		}
	}
	x0, y0, _, y1 := obj.ContentRect()
	x := x0 + float32((pen+32)>>6)
	rect := frame.RectF4(x, y0, x+1, y1).ToDevice(b.scale)
	if clip != nil {
		rect = rect.Intersection(*clip)
	}
	if rect.Empty() {
		return
	}
	caretColor := s.Color
	if s.CaretColor.A > 0 {
		caretColor = s.CaretColor
	}
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    rect,
		Color:   frame.RGB(caretColor.R, caretColor.G, caretColor.B),
		Opacity: float32(caretColor.A) / 255.0,
	})
}

// paintFocusRing draws a 2px Chromium-blue ring 2px outside the control's
// border box, as four strips so it works at any size.
func (b *Builder) paintFocusRing(obj *layout.Object) {
	x0, y0, x1, y1 := obj.BorderRect()
	if x1 <= x0 || y1 <= y0 {
		return
	}
	const gap = 2
	const thick = 2
	color := frame.RGB(0, 103, 244)
	outer := frame.RectF4(x0-gap, y0-gap, x1+gap, y1+gap).ToDevice(b.scale)
	inner := frame.RectF4(x0+thick-gap, y0+thick-gap, x1-thick+gap, y1-thick+gap).ToDevice(b.scale)
	strip := func(x0, y0, x1, y1 int32) {
		r := frame.Rect4(x0, y0, x1, y1)
		if r.Empty() {
			return
		}
		b.list.Append(DisplayCmd{
			Kind:    CmdFill,
			Rect:    r,
			Color:   color,
			Opacity: 1,
		})
	}
	strip(outer.X0, outer.Y0, outer.X1, inner.Y0)
	strip(outer.X0, inner.Y1, outer.X1, outer.Y1)
	strip(outer.X0, inner.Y0, inner.X0, inner.Y1)
	strip(inner.X1, inner.Y0, outer.X1, inner.Y1)
}

// listCounterStyle maps a marker type to the counter-style name FormatCounter
// understands. Ordinals come from sibling order (custom ol start/reversed and
// scoped counters do not feed markers yet); formatting is still per-type.
func listCounterStyle(t style.ListStyleType) string {
	switch t {
	case style.ListStyleDecimalLeadingZero:
		return "decimal-leading-zero"
	case style.ListStyleUpperRoman:
		return "upper-roman"
	case style.ListStyleLowerRoman:
		return "lower-roman"
	case style.ListStyleUpperAlpha:
		return "upper-alpha"
	case style.ListStyleLowerAlpha:
		return "lower-alpha"
	case style.ListStyleGeorgian:
		return "georgian"
	case style.ListStyleArmenian:
		return "armenian"
	default:
		return "decimal"
	}
}

// paintMarker draws the outside marker of a list-item. The marker takes the
// item's own font and color, is right-aligned to the item's content edge, and
// sits on the baseline of the first text line inside it.
func (b *Builder) paintMarker(obj *layout.Object, ordinal int) {
	s := obj.Style
	if s == nil || s.ListStyleType == style.ListStyleNone || s.Visibility != "visible" {
		return
	}
	var text string
	switch s.ListStyleType {
	case style.ListStyleDisc:
		text = "•"
	case style.ListStyleCircle:
		text = "◦"
	case style.ListStyleSquare:
		text = "▪"
	case style.ListStyleDecimal:
		text = strconv.Itoa(ordinal) + "."
	case style.ListStyleDecimalLeadingZero,
		style.ListStyleUpperRoman, style.ListStyleLowerRoman,
		style.ListStyleUpperAlpha, style.ListStyleLowerAlpha,
		style.ListStyleGeorgian, style.ListStyleArmenian:
		text = style.FormatCounter(ordinal, listCounterStyle(s.ListStyleType)) + "."
	default:
		return
	}
	fontSize := int32(s.FontSize * b.scale)
	if fontSize <= 0 {
		return
	}
	slot := s.FontSlot()
	width := int32(0)
	for _, r := range text {
		if b.metrics != nil {
			width += b.metrics.GlyphAdvance(fontSize, r, slot)
		} else {
			width += fontSize / 2
		}
	}
	if width <= 0 {
		return
	}
	// The marker is followed by a space, so the glyph - not the trailing
	// whitespace - is what ends at the item's content edge. Right-aligning the
	// text alone butts `1.` against `Mission`, which reads as one word.
	gap := int32(0)
	if b.metrics != nil {
		gap = b.metrics.GlyphAdvance(fontSize, ' ', slot)
	}
	x0, y0, _, _ := obj.ContentRect()
	if line := firstTextObject(b.arena, obj); line != nil {
		_, ly, _, _ := line.ContentRect()
		y0 = ly
	}
	mx := int32(x0*b.scale) - width - gap
	my := int32(y0 * b.scale)
	rect := frame.Rect{X0: mx, Y0: my, X1: mx + width, Y1: my + fontSize}
	b.appendRun(text, rect, s, convertColor(s.Color), s.Opacity)
}

// firstTextObject returns the item's first text descendant in paint order,
// skipping hidden subtrees: the marker aligns with that line, not the box top.
func firstTextObject(a *layout.Arena, obj *layout.Object) *layout.Object {
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style != nil && k.Style.Display == style.DisplayNone {
			continue
		}
		if k.Node != nil && k.Node.Text() && k.Node.DataContent != "" {
			return k
		}
		if d := firstTextObject(a, k); d != nil {
			return d
		}
	}
	return nil
}

func (b *Builder) paintProgress(obj *layout.Object, rect frame.Rect, opacity float32) {
	if rect.Empty() || obj.Node == nil {
		return
	}
	trackBg := frame.RGB(230, 230, 230)
	borderColor := frame.RGB(204, 204, 204)
	b.list.Append(DisplayCmd{
		Kind:  CmdFill,
		Rect:  rect,
		Color: trackBg,
	})
	b.list.Append(DisplayCmd{
		Kind: CmdBorder,
		Rect: rect,
		Border: BorderSpec{
			Top:    SideSpec{Width: 1, Color: borderColor},
			Right:  SideSpec{Width: 1, Color: borderColor},
			Bottom: SideSpec{Width: 1, Color: borderColor},
			Left:   SideSpec{Width: 1, Color: borderColor},
		},
	})
	valStr := obj.Node.GetAttribute("value")
	if valStr == "" {
		return
	}
	val, errVal := strconv.ParseFloat(valStr, 64)
	maxVal := 1.0
	if maxStr := obj.Node.GetAttribute("max"); maxStr != "" {
		if m, err := strconv.ParseFloat(maxStr, 64); err == nil && m > 0 {
			maxVal = m
		}
	}
	if errVal != nil || val < 0 {
		val = 0
	}
	if val > maxVal {
		val = maxVal
	}
	frac := float32(val / maxVal)
	if frac > 0 {
		fillW := int32(float32(rect.W()) * frac)
		if fillW > rect.W() {
			fillW = rect.W()
		}
		fillRect := frame.Rect4(rect.X0, rect.Y0, rect.X0+fillW, rect.Y1)
		barColor := frame.RGB(0, 117, 255)
		b.list.Append(DisplayCmd{
			Kind:  CmdFill,
			Rect:  fillRect,
			Color: barColor,
		})
	}
}

func (b *Builder) paintMeter(obj *layout.Object, rect frame.Rect, opacity float32) {
	if rect.Empty() || obj.Node == nil {
		return
	}
	trackBg := frame.RGB(230, 230, 230)
	borderColor := frame.RGB(204, 204, 204)
	b.list.Append(DisplayCmd{
		Kind:  CmdFill,
		Rect:  rect,
		Color: trackBg,
	})
	b.list.Append(DisplayCmd{
		Kind: CmdBorder,
		Rect: rect,
		Border: BorderSpec{
			Top:    SideSpec{Width: 1, Color: borderColor},
			Right:  SideSpec{Width: 1, Color: borderColor},
			Bottom: SideSpec{Width: 1, Color: borderColor},
			Left:   SideSpec{Width: 1, Color: borderColor},
		},
	})

	parseF := func(name string, def float64) float64 {
		s := obj.Node.GetAttribute(name)
		if s == "" {
			return def
		}
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return v
		}
		return def
	}
	minVal := parseF("min", 0)
	maxVal := parseF("max", 1)
	if maxVal <= minVal {
		maxVal = minVal + 1
	}
	val := parseF("value", minVal)
	if val < minVal {
		val = minVal
	}
	if val > maxVal {
		val = maxVal
	}
	low := parseF("low", minVal)
	high := parseF("high", maxVal)
	opt := parseF("optimum", (minVal+maxVal)/2)

	meterColor := frame.RGB(40, 167, 69) // Green
	if opt >= high {
		if val < low {
			meterColor = frame.RGB(220, 53, 69) // Red
		} else if val < high {
			meterColor = frame.RGB(255, 193, 7) // Yellow
		}
	} else if opt <= low {
		if val > high {
			meterColor = frame.RGB(220, 53, 69)
		} else if val > low {
			meterColor = frame.RGB(255, 193, 7)
		}
	} else {
		if val < low || val > high {
			meterColor = frame.RGB(255, 193, 7)
		}
	}

	frac := float32((val - minVal) / (maxVal - minVal))
	if frac > 0 {
		fillW := int32(float32(rect.W()) * frac)
		if fillW > rect.W() {
			fillW = rect.W()
		}
		fillRect := frame.Rect4(rect.X0, rect.Y0, rect.X0+fillW, rect.Y1)
		b.list.Append(DisplayCmd{
			Kind:  CmdFill,
			Rect:  fillRect,
			Color: meterColor,
		})
	}
}
