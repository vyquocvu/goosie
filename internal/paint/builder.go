package paint

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// Builder walks a layout arena and emits display commands into a List.
//
// The builder is the seam between layout and paint: layout produces positioned
// boxes with styles, and the builder translates each box into the drawing
// operations that paint understands. The builder runs after layout is complete;
// it does not mutate the arena.
type Builder struct {
	list    *List
	arena   *layout.Arena
	scale   float32
	metrics layout.Metrics
}

// NewBuilder returns a builder that will emit commands into list. metrics is
// the same glyph-advance source layout measured with; nil falls back to the
// half-em estimate layout used, which spaces glyphs approximately.
func NewBuilder(list *List, arena *layout.Arena, scale float32, metrics layout.Metrics) *Builder {
	return &Builder{list: list, arena: arena, scale: scale, metrics: metrics}
}

// Build walks the arena starting at root and appends display commands to the
// list. The root object's position is taken as the origin; a page with a
// scrolled viewport passes the scroll offset as root.X/Y before calling Build.
func (b *Builder) Build(root layout.ObjectID) {
	b.paintBox(root, nil)
}

func (b *Builder) paintBox(id layout.ObjectID, clip *frame.Rect) {
	obj := b.arena.Get(id)
	if obj.Style != nil && obj.Style.Display == style.DisplayNone {
		return
	}

	// Determine the clip rect to pass to children. Start with the inherited clip.
	kidClip := clip
	if obj.Style != nil {
		x0, y0, x1, y1 := obj.BorderRect()
		rect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
		radius := corners(obj.Style, rect, b.scale)
		opacity := obj.Style.Opacity
		if obj.Style.BackgroundColor.A > 0 {
			b.list.Append(DisplayCmd{
				Kind:    CmdFill,
				Rect:    rect,
				Color:   convertColor(obj.Style.BackgroundColor),
				Radius:  radius,
				Opacity: opacity,
			})
		}
		if g := gradient(obj.Style.BackgroundGradient, opacity); !g.Empty() {
			b.list.Append(DisplayCmd{
				Kind:     CmdGradient,
				Rect:     rect,
				Gradient: g,
				Radius:   radius,
			})
		}
		if obj.Style.Display != style.DisplayInline && obj.Style.Display != style.DisplayNone {
			b.paintBorders(obj, rect, radius, opacity)
		}
		if obj.Node != nil && obj.Node.Type == 2 && obj.Node.DataContent != "" {
			b.paintText(obj, rect, opacity, clip)
		}
		if obj.Node != nil && obj.Node.Data == "input" {
			b.paintPlaceholder(obj, opacity, clip)
		}
		// If this box has overflow:hidden, compute a content-space clip rect
		// that children must respect.
		if obj.Style.Overflow == style.OverflowHidden || obj.Style.OverflowX == style.OverflowHidden || obj.Style.OverflowY == style.OverflowHidden {
			cx0, cy0, cx1, cy1 := obj.ContentRect()
			cr := frame.RectF4(cx0, cy0, cx1, cy1).ToDevice(b.scale)
			if kidClip != nil {
				intersected := kidClip.Intersection(cr)
				kidClip = &intersected
			} else {
				kidClip = &cr
			}
		}
	}
	for kid := obj.FirstKid; kid != 0; kid = b.arena.Get(kid).NextSibling {
		b.paintBox(kid, kidClip)
	}
}

func (b *Builder) paintBorders(obj *layout.Object, rect frame.Rect, radius frame.Corners, opacity float32) {
	s := obj.Style
	if s == nil {
		return
	}
	// The widths come off the box, not the style: layout is what decides which
	// edges a box actually draws, and a collapsed table border is exactly the
	// case where the two disagree.
	border := BorderSpec{}
	if obj.BorderTop > 0 {
		border.Top = SideSpec{Width: int32(obj.BorderTop), Color: convertColor(s.BorderTopColor)}
	}
	if obj.BorderRight > 0 {
		border.Right = SideSpec{Width: int32(obj.BorderRight), Color: convertColor(s.BorderRightColor)}
	}
	if obj.BorderBottom > 0 {
		border.Bottom = SideSpec{Width: int32(obj.BorderBottom), Color: convertColor(s.BorderBottomColor)}
	}
	if obj.BorderLeft > 0 {
		border.Left = SideSpec{Width: int32(obj.BorderLeft), Color: convertColor(s.BorderLeftColor)}
	}
	if border.Top.Width > 0 || border.Right.Width > 0 || border.Bottom.Width > 0 || border.Left.Width > 0 {
		b.list.Append(DisplayCmd{
			Kind:    CmdBorder,
			Rect:    rect,
			Border:  border,
			Radius:  radius,
			Opacity: opacity,
		})
	}
}

// corners resolves a box's declared radii against the device rect they round. A
// percentage is the share of the side it runs along that the cascade could not
// compute, and radii that overflow their side scale down together, which is what
// turns an oversized value like 999px into a pill.
func corners(s *style.ComputedStyle, r frame.Rect, scale float32) frame.Corners {
	if s == nil {
		return frame.Corners{}
	}
	declared := s.BorderRadius
	// A declared percentage is a negative sentinel, so the check for "nothing was
	// declared" has to be against zero rather than against a positive length.
	if declared == ([4][2]float32{}) {
		return frame.Corners{}
	}
	w, h := float32(r.W()), float32(r.H())
	resolve := func(v, side float32) float32 {
		if v < -2 {
			return (-2 - v) * side / 100
		}
		if v <= 0 {
			return 0
		}
		return v * scale
	}
	var c [4][2]float32
	for i := 0; i < 4; i++ {
		c[i][0] = resolve(declared[i][0], w)
		c[i][1] = resolve(declared[i][1], h)
	}
	fit := func(sum, side float32) float32 {
		if sum <= side || sum <= 0 {
			return 1
		}
		return side / sum
	}
	k := fit(c[0][0]+c[1][0], w)
	if v := fit(c[3][0]+c[2][0], w); v < k {
		k = v
	}
	if v := fit(c[0][1]+c[3][1], h); v < k {
		k = v
	}
	if v := fit(c[1][1]+c[2][1], h); v < k {
		k = v
	}
	rad := func(i int) frame.Radius {
		return frame.Radius{X: c[i][0] * k, Y: c[i][1] * k}
	}
	return frame.Corners{TL: rad(0), TR: rad(1), BR: rad(2), BL: rad(3)}
}

func (b *Builder) paintText(obj *layout.Object, rect frame.Rect, opacity float32, clip *frame.Rect) {
	if obj.Node == nil || obj.Node.DataContent == "" {
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
	b.appendRun(obj.Node.DataContent, rect, s, convertColor(s.Color), opacity)
}

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
		x := rect.X0
		for i, r := range runes {
			glyphs = append(glyphs, GlyphRun{Rune: r, X: x, Y: baseline, Size: fontSize, Slot: slot})
			x += b.metrics.GlyphAdvance(fontSize, r, slot)
			if i < len(runes)-1 {
				x += letterSpacing
			}
			if r == ' ' {
				x += wordSpacing
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
}

func convertColor(c css.Color) frame.Color {
	return frame.RGBA(c.R, c.G, c.B, c.A)
}

// gradient turns a declared ramp into premultiplied stops and folds the box's
// opacity into each of them, so the command carries one multiplier rather than
// two and the rasterizer never revisits the style.
func gradient(g style.Gradient, opacity float32) frame.LinearGradient {
	if g.Empty() {
		return frame.LinearGradient{}
	}
	stops := make([]frame.GradientStop, 0, len(g.Stops))
	for _, s := range g.Stops {
		c := convertColor(s.Color)
		if opacity > 0 && opacity < 1 {
			c = frame.ScaleColor(c, opacity)
		}
		stops = append(stops, frame.GradientStop{At: s.At, Color: c})
	}
	return frame.LinearGradient{Angle: g.Angle, Stops: stops}
}
