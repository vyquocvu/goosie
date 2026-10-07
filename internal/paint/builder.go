package paint

import (
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// svgNamespace is the NSSVG constant value (defined in internal/dom).
// We duplicate it here to avoid importing dom from paint, which would
// violate the import boundary rules.
const svgNamespace = 2

// Builder walks a layout arena and emits display commands into a List.
//
// The builder is the seam between layout and paint: layout produces positioned
// boxes with styles, and the builder translates each box into the drawing
// operations that paint understands. The builder runs after layout is complete;
// it does not mutate the arena.
type Builder struct {
	list        *List
	arena       *layout.Arena
	scale       float32
	metrics     layout.Metrics
	focus       *layout.Object
	focusCaret  int
	focusMarked string
	selection   map[*layout.Object]SelSpan
}

// NewBuilder returns a builder that will emit commands into list. metrics is
// the same glyph-advance source layout measured with; nil falls back to the
// half-em estimate layout used, which spaces glyphs approximately.
func NewBuilder(list *List, arena *layout.Arena, scale float32, metrics layout.Metrics) *Builder {
	return &Builder{list: list, arena: arena, scale: scale, metrics: metrics}
}

// SetFocus marks the control to paint with a caret and focus ring. caret is a
// rune index into the control's value; marked is the IME composition preview
// shown spliced into the value at the caret.
func (b *Builder) SetFocus(obj *layout.Object, caret int, marked string) {
	b.focus = obj
	b.focusCaret = caret
	b.focusMarked = marked
}

// SetSelection marks word boxes to paint with the selection highlight. The
// spans reference the same layout objects the arena walk visits, so identity
// in this map is what paintText matches on.
func (b *Builder) SetSelection(spans []SelSpan) {
	b.selection = make(map[*layout.Object]SelSpan, len(spans))
	for _, sp := range spans {
		b.selection[sp.Obj] = sp
	}
}

// Build walks the arena starting at root and appends display commands to the
// list. The root object's position is taken as the origin; a page with a
// scrolled viewport passes the scroll offset as root.X/Y before calling Build.
func (b *Builder) Build(root layout.ObjectID) {
	b.paintBox(root, nil, 0)
}

func (b *Builder) paintBox(id layout.ObjectID, clip *frame.Rect, ordinal int) {
	obj := b.arena.Get(id)
	if obj.Style != nil && obj.Style.Display == style.DisplayNone {
		return
	}

	// Determine the clip rect to pass to children. Start with the inherited clip.
	kidClip := clip
	if obj.Style != nil {
		// CSS clip:rect() with zero area hides the element's own painting
		// (backgrounds, borders, text, images) but children still walk.
		clipHidden := obj.Style.HasClip && (obj.Style.ClipTop >= obj.Style.ClipBottom || obj.Style.ClipLeft >= obj.Style.ClipRight)

		x0, y0, x1, y1 := obj.BorderRect()
		rect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
		radius := corners(obj.Style, rect, b.scale)
		opacity := obj.Style.Opacity
		// Backgrounds paint inside the clip box, not the border box it
		// defaults to: content-box (or padding-box) clip shrinks every
		// background layer - colour, gradients, and the image below.
		cx0, cy0, cx1, cy1 := obj.BgAreaRect(obj.Style.BackgroundClip)
		bgClip := frame.RectF4(cx0, cy0, cx1, cy1).ToDevice(b.scale)
		clipRect := rect.Intersection(bgClip)
		// A clipped fill follows the inner curve: each corner loses the
		// border+padding thickness meeting it, per the corner-shaping rule
		// the fuzz-tolerant radius tests pin down.
		clipRadius := radius.Shrunk(
			clipRect.Y0-rect.Y0, rect.X1-clipRect.X1,
			rect.Y1-clipRect.Y1, clipRect.X0-rect.X0)

		// Record the list length before painting this element's own content,
		// so we can apply the CSS transform to just these commands.
		cmdStart := b.list.Len()

		if !clipHidden {
			// An inline element's own box carries the union of its fragment
			// rects, so a background painted there would fill the whole bounding
			// box across line breaks. The inline pass's word copies are the
			// fragments that own the visible per-line backgrounds; they share the
			// element's computed style but are text nodes, so they still paint.
			if obj.Style.Display != style.DisplayInline || (obj.Node != nil && obj.Node.Text()) {
				// Box shadows paint behind the element's background and border.
				if len(obj.Style.BoxShadow) > 0 {
					b.paintBoxShadow(obj, rect, radius, opacity)
				}
				if obj.Style.BackgroundColor.A > 0 && !clipRect.Empty() {
					b.list.Append(DisplayCmd{
						Kind:    CmdFill,
						Rect:    clipRect,
						Color:   convertColor(obj.Style.BackgroundColor),
						Radius:  clipRadius,
						Opacity: opacity,
					})
				}
				if g := gradient(obj.Style.BackgroundGradient, opacity); !g.Empty() {
					b.paintGradientTiles(obj, clipRect, clipRadius, g, false, frame.RadialGradient{})
				}
				if rg := radialGradient(obj.Style.BackgroundRadialGradient, opacity); !rg.Empty() {
					b.paintGradientTiles(obj, clipRect, clipRadius, frame.LinearGradient{}, true, rg)
				}
				b.paintBackground(obj, opacity)
			}
			if obj.Style.Display != style.DisplayInline && obj.Style.Display != style.DisplayNone {
				b.paintBorders(obj, rect, radius, opacity)
			}
			if obj.Style.Display == style.DisplayListItem {
				b.paintMarker(obj, ordinal)
			}
			if obj.Node != nil && obj.Node.Text() &&
				(obj.Node.DataContent != "" || b.composingTextareaText(obj)) {
				b.paintText(obj, rect, opacity, clip)
			}
			if obj.Node != nil && obj.Node.Data == "input" {
				typ := strings.ToLower(obj.Node.GetAttribute("type"))
				switch typ {
				case "checkbox":
					cbRect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
					b.paintCheckbox(obj, cbRect, opacity)
				case "radio":
					rdRect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
					b.paintRadio(obj, rdRect, opacity)
				case "submit", "reset", "button":
					b.paintButtonLabel(obj, opacity, clip)
				default:
					b.paintControlValue(obj, opacity, clip)
					b.paintPlaceholder(obj, opacity, clip)
				}
			}
			if obj.Node != nil && obj.Node.Data == "select" {
				b.paintSelect(obj, opacity, clip)
			}
			if obj.Node != nil && obj.Node.Data == "img" {
				b.paintImage(obj, opacity, clip)
			}
			// SVG shape elements paint through their own path.
			if obj.Node != nil && obj.Node.Namespace == svgNamespace && isSVGShape(obj.Node.Data) {
				b.paintSVGShape(obj, opacity)
			}
			if b.focus != nil && obj == b.focus {
				b.paintFocus(obj, clip)
			}
		}
		// Apply CSS transform to this element's own commands. The transform
		// is resolved around the transform-origin and applied as a matrix to
		// each command's rect, replacing it with the axis-aligned bounding
		// box of the transformed corners.
		if len(obj.Style.Transform) > 0 {
			b.applyTransform(obj, cmdStart, x0, y0, x1, y1)
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
	item := 0
	for kid := obj.FirstKid; kid != 0; kid = b.arena.Get(kid).NextSibling {
		ord := 0
		if k := b.arena.Get(kid); k.Style != nil && k.Style.Display == style.DisplayListItem {
			// The counter advances for every item, even ones styled list-style:none.
			item++
			ord = item
		}
		b.paintBox(kid, kidClip, ord)
	}
}

func (b *Builder) paintBorders(obj *layout.Object, rect frame.Rect, radius frame.Corners, opacity float32) {
	s := obj.Style
	if s == nil {
		return
	}
	// The widths come off the box, not the style: layout is what decides which
	// edges a box actually draws, and a collapsed table border is exactly the
	// case where the two disagree. A nonzero width always paints at least one
	// device pixel: truncating 0.1px to zero would erase the hairlines the
	// small-values tests require to be visible.
	toDevice := func(cssPx float32) int32 {
		if cssPx <= 0 {
			return 0
		}
		// Floor, with a one-pixel floor for nonzero widths: 1.9px snaps
		// to 1px (never rounds up to 2px), while 0.1px still paints its
		// hairline instead of vanishing.
		if w := int32(cssPx * b.scale); w > 0 {
			return w
		}
		return 1
	}
	border := BorderSpec{}
	if obj.BorderTop > 0 {
		border.Top = SideSpec{Width: toDevice(obj.BorderTop), Color: convertColor(s.BorderTopColor)}
	}
	if obj.BorderRight > 0 {
		border.Right = SideSpec{Width: toDevice(obj.BorderRight), Color: convertColor(s.BorderRightColor)}
	}
	if obj.BorderBottom > 0 {
		border.Bottom = SideSpec{Width: toDevice(obj.BorderBottom), Color: convertColor(s.BorderBottomColor)}
	}
	if obj.BorderLeft > 0 {
		border.Left = SideSpec{Width: toDevice(obj.BorderLeft), Color: convertColor(s.BorderLeftColor)}
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
	text := obj.Node.DataContent
	if b.composingTextareaText(obj) {
		text = b.composeControlText(obj, text)
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
	if sp, ok := b.selection[obj]; ok {
		b.paintSelectionHighlight(obj, rect, s, sp, opacity)
	}
	// Text shadows paint behind the main text as offset copies.
	if len(s.TextShadow) > 0 {
		b.paintTextShadow(text, rect, s, opacity)
	}
	b.appendRun(text, rect, s, convertColor(s.Color), opacity)
}

// composingTextareaText reports whether obj paints the first text child of the
// focused textarea, the one text node a composition preview can splice into.
// The inline pass paints word copies of the node (same NodeID, new pointer), so
// match by ID rather than pointer identity.
func (b *Builder) composingTextareaText(obj *layout.Object) bool {
	if b.focus == nil || b.focusMarked == "" || obj.Node == nil || b.focus.Node == nil {
		return false
	}
	if obj.Node.Parent != b.focus.Node || !obj.Node.Text() {
		return false
	}
	for c := b.focus.Node.FirstChild; c != nil; c = c.NextSibling {
		if c.Text() {
			return obj.Node.ID == c.ID
		}
	}
	return false
}

// paintImage draws a decoded replaced image into its content box. A box whose
// image never decoded (or was not fetched) carries no pixels, so it contributes
// no command and simply leaves the gap the layout reserved for it.
func (b *Builder) paintImage(obj *layout.Object, opacity float32, clip *frame.Rect) {
	src, ok := obj.Image.(image.Image)
	if !ok || src == nil {
		return
	}
	box := src.Bounds()
	if box.Dx() <= 0 || box.Dy() <= 0 {
		return
	}
	x0, y0, x1, y1 := obj.ContentRect()
	if x1 <= x0 || y1 <= y0 {
		return
	}
	dest := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	srcBox := box
	// object-fit decides how the intrinsic pixels sit in the content box. The
	// default (fill) and any unrecognised keyword stretch to the whole box, which
	// is what the two rects already say. contain shrinks the destination so the
	// whole image shows, centered; cover keeps the destination on the box and
	// crops the source to the centre so the box fills edge to edge.
	if s := obj.Style; s != nil {
		switch s.ObjectFit {
		case "contain", "scale-down":
			if r, ok := fitContain(box, x1-x0, y1-y0); ok {
				// fitContain centres inside `w x h`, so its edges are relative to
				// the content box origin. Treating them as page coordinates left
				// the picture letterboxed near the top-left of the document while
				// its own box sat empty further down the page.
				dest = frame.RectF4(r.X0+x0, r.Y0+y0, r.X1+x0, r.Y1+y0).ToDevice(b.scale)
			}
		case "cover":
			if sb, ok := fitCoverSrcBox(box, x1-x0, y1-y0); ok {
				srcBox = sb
			}
		}
	}
	b.list.Append(DisplayCmd{
		Kind:    CmdImage,
		Rect:    dest,
		Image:   ImageSpec{Src: src, SrcBox: srcBox},
		Opacity: opacity,
	})
}

// paintSVGShape draws an SVG shape element (rect, circle, ellipse, line,
// polygon, polyline, path) using the element's geometry attributes and
// presentation style. Shapes are rasterized into fill/stroke commands.
func (b *Builder) paintSVGShape(obj *layout.Object, opacity float32) {
	if obj.Node == nil || obj.Node.Namespace != 2 { // NSSVG
		return
	}
	x0, y0, x1, y1 := obj.ContentRect()
	if x1 <= x0 || y1 <= y0 {
		return
	}
	scale := b.scale
	tag := obj.Node.Data

	// Parse fill color from style or attribute.
	fillColor := b.parseSVGPaint(obj, "fill", "black")
	strokeColor := b.parseSVGPaint(obj, "stroke", "")
	strokeWidth := b.parseSVGLength(obj, "stroke-width", 1) * scale

	switch tag {
	case "rect":
		b.paintSVGRect(obj, x0, y0, x1, y1, fillColor, strokeColor, strokeWidth, opacity)
	case "circle":
		b.paintSVGCircle(obj, x0, y0, x1, y1, fillColor, strokeColor, strokeWidth, opacity)
	case "ellipse":
		b.paintSVGEllipse(obj, x0, y0, x1, y1, fillColor, strokeColor, strokeWidth, opacity)
	case "line":
		b.paintSVGLine(obj, x0, y0, x1, y1, strokeColor, strokeWidth, opacity)
	case "polygon", "polyline":
		b.paintSVGPolygon(obj, x0, y0, x1, y1, fillColor, strokeColor, strokeWidth, tag == "polyline", opacity)
	case "path":
		b.paintSVGPath(obj, x0, y0, x1, y1, fillColor, strokeColor, strokeWidth, opacity)
	}
}

// parseSVGPaint returns the fill/stroke color for an SVG element, checking
// the computed style first, then the presentation attribute.
func (b *Builder) parseSVGPaint(obj *layout.Object, attr, fallback string) frame.Color {
	if obj.Style != nil && obj.Style.Color.A > 0 {
		return convertColor(obj.Style.Color)
	}
	v := obj.Node.GetAttribute(attr)
	if v == "" {
		v = fallback
	}
	if v == "none" {
		return 0
	}
	c := parseSVGColor(v)
	return c
}

// parseSVGLength parses an SVG length attribute, returning the value in
// user units (pixels).
func (b *Builder) parseSVGLength(obj *layout.Object, attr string, fallback float32) float32 {
	v := obj.Node.GetAttribute(attr)
	if v == "" {
		return fallback
	}
	// Strip units if present (px, em, etc.) - we treat everything as px.
	v = strings.TrimSuffix(v, "px")
	v = strings.TrimSuffix(v, "em")
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return fallback
	}
	return float32(f)
}

// parseSVGColor parses a simple CSS color value.
func parseSVGColor(s string) frame.Color {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Handle hex colors.
	if len(s) == 4 && s[0] == '#' {
		r, _ := strconv.ParseUint(s[1:2]+s[1:2], 16, 8)
		g, _ := strconv.ParseUint(s[2:3]+s[2:3], 16, 8)
		b, _ := strconv.ParseUint(s[3:4]+s[3:4], 16, 8)
		return frame.RGBA(uint8(r), uint8(g), uint8(b), 255)
	}
	if len(s) == 7 && s[0] == '#' {
		r, _ := strconv.ParseUint(s[1:3], 16, 8)
		g, _ := strconv.ParseUint(s[3:5], 16, 8)
		b, _ := strconv.ParseUint(s[5:7], 16, 8)
		return frame.RGBA(uint8(r), uint8(g), uint8(b), 255)
	}
	// Named colors.
	switch s {
	case "black":
		return frame.RGB(0, 0, 0)
	case "white":
		return frame.RGB(255, 255, 255)
	case "red":
		return frame.RGB(255, 0, 0)
	case "green":
		return frame.RGB(0, 128, 0)
	case "blue":
		return frame.RGB(0, 0, 255)
	case "yellow":
		return frame.RGB(255, 255, 0)
	case "cyan":
		return frame.RGB(0, 255, 255)
	case "magenta":
		return frame.RGB(255, 0, 255)
	case "gray", "grey":
		return frame.RGB(128, 128, 128)
	case "orange":
		return frame.RGB(255, 165, 0)
	case "purple":
		return frame.RGB(128, 0, 128)
	case "pink":
		return frame.RGB(255, 192, 203)
	case "brown":
		return frame.RGB(165, 42, 42)
	}
	return 0
}

func (b *Builder) paintSVGRect(obj *layout.Object, bx0, by0, bx1, by1 float32, fill, stroke frame.Color, strokeW float32, opacity float32) {
	x := b.parseSVGLengthAttr(obj, "x", 0)
	y := b.parseSVGLengthAttr(obj, "y", 0)
	w := b.parseSVGLengthAttr(obj, "width", bx1-bx0)
	h := b.parseSVGLengthAttr(obj, "height", by1-by0)
	rx := b.parseSVGLengthAttr(obj, "rx", 0)
	ry := b.parseSVGLengthAttr(obj, "ry", rx)

	scale := b.scale
	rect := frame.RectF4(bx0+x*scale, by0+y*scale, bx0+(x+w)*scale, by0+(y+h)*scale).ToDevice(scale)
	if rect.Empty() {
		return
	}

	if fill.A() > 0 {
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: rect, Color: fill, Opacity: opacity})
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokeRect(rect, stroke, strokeW, rx*scale, ry*scale, opacity)
	}
}

func (b *Builder) paintSVGCircle(obj *layout.Object, bx0, by0, bx1, by1 float32, fill, stroke frame.Color, strokeW float32, opacity float32) {
	cx := b.parseSVGLengthAttr(obj, "cx", (bx1-bx0)/2)
	cy := b.parseSVGLengthAttr(obj, "cy", (by1-by0)/2)
	r := b.parseSVGLengthAttr(obj, "r", 0)
	if r <= 0 {
		return
	}
	scale := b.scale
	cxp := bx0 + cx*scale
	cyp := by0 + cy*scale
	rp := r * scale

	if fill.A() > 0 {
		b.paintSVGFilledCircle(cxp, cyp, rp, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedCircle(cxp, cyp, rp, stroke, strokeW, opacity)
	}
}

func (b *Builder) paintSVGEllipse(obj *layout.Object, bx0, by0, bx1, by1 float32, fill, stroke frame.Color, strokeW float32, opacity float32) {
	cx := b.parseSVGLengthAttr(obj, "cx", (bx1-bx0)/2)
	cy := b.parseSVGLengthAttr(obj, "cy", (by1-by0)/2)
	rx := b.parseSVGLengthAttr(obj, "rx", 0)
	ry := b.parseSVGLengthAttr(obj, "ry", 0)
	if rx <= 0 || ry <= 0 {
		return
	}
	scale := b.scale
	cxp := bx0 + cx*scale
	cyp := by0 + cy*scale
	rxp := rx * scale
	ryp := ry * scale

	if fill.A() > 0 {
		b.paintSVGFilledEllipse(cxp, cyp, rxp, ryp, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedEllipse(cxp, cyp, rxp, ryp, stroke, strokeW, opacity)
	}
}

func (b *Builder) paintSVGLine(obj *layout.Object, bx0, by0, bx1, by1 float32, stroke frame.Color, strokeW float32, opacity float32) {
	x1 := b.parseSVGLengthAttr(obj, "x1", 0)
	y1 := b.parseSVGLengthAttr(obj, "y1", 0)
	x2 := b.parseSVGLengthAttr(obj, "x2", bx1-bx0)
	y2 := b.parseSVGLengthAttr(obj, "y2", 0)
	if stroke.A() <= 0 || strokeW <= 0 {
		return
	}
	scale := b.scale
	b.paintSVGLineSegment(bx0+x1*scale, by0+y1*scale, bx0+x2*scale, by0+y2*scale, stroke, strokeW, opacity)
}

func (b *Builder) paintSVGPolygon(obj *layout.Object, bx0, by0, bx1, by1 float32, fill, stroke frame.Color, strokeW float32, open bool, opacity float32) {
	pointsStr := obj.Node.GetAttribute("points")
	if pointsStr == "" {
		return
	}
	points := parseSVGPoints(pointsStr)
	if len(points) < 2 {
		return
	}
	scale := b.scale
	// Offset points to the box origin.
	for i := range points {
		points[i][0] = bx0 + points[i][0]*scale
		points[i][1] = by0 + points[i][1]*scale
	}

	if !open && fill.A() > 0 {
		b.paintSVGFilledPolygon(points, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedPolygon(points, stroke, strokeW, open, opacity)
	}
}

func (b *Builder) paintSVGPath(obj *layout.Object, bx0, by0, bx1, by1 float32, fill, stroke frame.Color, strokeW float32, opacity float32) {
	d := obj.Node.GetAttribute("d")
	if d == "" {
		return
	}
	segments := parseSVGPath(d)
	if len(segments) == 0 {
		return
	}
	scale := b.scale
	// Offset to box origin.
	for i := range segments {
		for j := range segments[i] {
			if j%2 == 0 {
				segments[i][j] = bx0 + segments[i][j]*scale
			} else {
				segments[i][j] = by0 + segments[i][j]*scale
			}
		}
	}

	if fill.A() > 0 {
		b.paintSVGFilledPath(segments, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedPath(segments, stroke, strokeW, opacity)
	}
}

func (b *Builder) parseSVGLengthAttr(obj *layout.Object, attr string, fallback float32) float32 {
	if obj.Node == nil {
		return fallback
	}
	v := obj.Node.GetAttribute(attr)
	if v == "" {
		return fallback
	}
	v = strings.TrimSuffix(v, "px")
	v = strings.TrimSuffix(v, "em")
	v = strings.TrimSuffix(v, "%")
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return fallback
	}
	return float32(f)
}

func (b *Builder) paintSVGStrokeRect(rect frame.Rect, color frame.Color, width, rx, ry float32, opacity float32) {
	hw := width / 2
	// Top
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X0, rect.Y0, rect.X1, rect.Y0+int32(width)), Color: color, Opacity: opacity})
	// Bottom
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X0, rect.Y1-int32(width), rect.X1, rect.Y1), Color: color, Opacity: opacity})
	// Left
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X0, rect.Y0, rect.X0+int32(width), rect.Y1), Color: color, Opacity: opacity})
	// Right
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X1-int32(width), rect.Y0, rect.X1, rect.Y1), Color: color, Opacity: opacity})
	_ = hw
	_ = rx
	_ = ry
}

func (b *Builder) paintSVGFilledCircle(cx, cy, r float32, color frame.Color, opacity float32) {
	ri := int32(r)
	for dy := -ri; dy <= ri; dy++ {
		for dx := -ri; dx <= ri; dx++ {
			if float32(dx*dx)+float32(dy*dy) <= r*r {
				b.list.Append(DisplayCmd{
					Kind:    CmdFill,
					Rect:    frame.Rect4(int32(cx)+dx, int32(cy)+dy, int32(cx)+dx+1, int32(cy)+dy+1),
					Color:   color,
					Opacity: opacity,
				})
			}
		}
	}
}

func (b *Builder) paintSVGStrokedCircle(cx, cy, r float32, color frame.Color, width float32, opacity float32) {
	ri := int32(r)
	hw := width / 2
	for dy := -ri; dy <= ri; dy++ {
		for dx := -ri; dx <= ri; dx++ {
			dist := float32(dx*dx) + float32(dy*dy)
			outerR := r + hw
			innerR := r - hw
			if dist <= outerR*outerR && dist >= innerR*innerR {
				b.list.Append(DisplayCmd{
					Kind:    CmdFill,
					Rect:    frame.Rect4(int32(cx)+dx, int32(cy)+dy, int32(cx)+dx+1, int32(cy)+dy+1),
					Color:   color,
					Opacity: opacity,
				})
			}
		}
	}
}

func (b *Builder) paintSVGFilledEllipse(cx, cy, rx, ry float32, color frame.Color, opacity float32) {
	rxi := int32(rx)
	ryi := int32(ry)
	for dy := -ryi; dy <= ryi; dy++ {
		for dx := -rxi; dx <= rxi; dx++ {
			if float32(dx*dx)/(rx*rx)+float32(dy*dy)/(ry*ry) <= 1 {
				b.list.Append(DisplayCmd{
					Kind:    CmdFill,
					Rect:    frame.Rect4(int32(cx)+dx, int32(cy)+dy, int32(cx)+dx+1, int32(cy)+dy+1),
					Color:   color,
					Opacity: opacity,
				})
			}
		}
	}
}

func (b *Builder) paintSVGStrokedEllipse(cx, cy, rx, ry float32, color frame.Color, width float32, opacity float32) {
	rxi := int32(rx)
	ryi := int32(ry)
	hw := width / 2
	for dy := -ryi; dy <= ryi; dy++ {
		for dx := -rxi; dx <= rxi; dx++ {
			dist := float32(dx*dx)/(rx*rx) + float32(dy*dy)/(ry*ry)
			outerScale := 1 + hw/rx
			innerScale := 1 - hw/rx
			if dist <= outerScale*outerScale && dist >= innerScale*innerScale {
				b.list.Append(DisplayCmd{
					Kind:    CmdFill,
					Rect:    frame.Rect4(int32(cx)+dx, int32(cy)+dy, int32(cx)+dx+1, int32(cy)+dy+1),
					Color:   color,
					Opacity: opacity,
				})
			}
		}
	}
}

func (b *Builder) paintSVGLineSegment(x1, y1, x2, y2 float32, color frame.Color, width float32, opacity float32) {
	// Bresenham-like line drawing with thickness.
	dx := x2 - x1
	dy := y2 - y1
	steps := int32(math.Sqrt(float64(dx*dx + dy*dy)))
	if steps < 1 {
		steps = 1
	}
	hw := width / 2
	for i := int32(0); i <= steps; i++ {
		t := float32(i) / float32(steps)
		px := x1 + dx*t
		py := y1 + dy*t
		// Draw a small square at each step for thickness.
		size := int32(width)
		if size < 1 {
			size = 1
		}
		b.list.Append(DisplayCmd{
			Kind:    CmdFill,
			Rect:    frame.Rect4(int32(px)-int32(hw), int32(py)-int32(hw), int32(px)+int32(hw)+1, int32(py)+int32(hw)+1),
			Color:   color,
			Opacity: opacity,
		})
		_ = size
	}
}

func (b *Builder) paintSVGFilledPolygon(points [][2]float32, color frame.Color, opacity float32) {
	if len(points) < 3 {
		return
	}
	// Find bounding box.
	minX, minY := points[0][0], points[0][1]
	maxX, maxY := minX, minY
	for _, p := range points[1:] {
		if p[0] < minX {
			minX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	// Scanline fill.
	for y := int32(minY); y <= int32(maxY); y++ {
		var intersections []float32
		n := len(points)
		for i := 0; i < n; i++ {
			j := (i + 1) % n
			yi, yj := points[i][1], points[j][1]
			xi, xj := points[i][0], points[j][0]
			if (yi <= float32(y) && yj > float32(y)) || (yj <= float32(y) && yi > float32(y)) {
				t := (float32(y) - yi) / (yj - yi)
				intersections = append(intersections, xi+t*(xj-xi))
			}
		}
		// Sort intersections.
		for i := 0; i < len(intersections); i++ {
			for j := i + 1; j < len(intersections); j++ {
				if intersections[j] < intersections[i] {
					intersections[i], intersections[j] = intersections[j], intersections[i]
				}
			}
		}
		// Fill between pairs.
		for i := 0; i+1 < len(intersections); i += 2 {
			x0 := int32(intersections[i])
			x1 := int32(intersections[i+1])
			b.list.Append(DisplayCmd{
				Kind:    CmdFill,
				Rect:    frame.Rect4(x0, y, x1+1, y+1),
				Color:   color,
				Opacity: opacity,
			})
		}
	}
}

func (b *Builder) paintSVGStrokedPolygon(points [][2]float32, color frame.Color, width float32, open bool, opacity float32) {
	n := len(points)
	end := n
	if open {
		end = n - 1
	}
	for i := 0; i < end; i++ {
		j := (i + 1) % n
		b.paintSVGLineSegment(points[i][0], points[i][1], points[j][0], points[j][1], color, width, opacity)
	}
}

func (b *Builder) paintSVGFilledPath(segments [][]float32, color frame.Color, opacity float32) {
	// Flatten path to polygon points and fill.
	var points [][2]float32
	for _, seg := range segments {
		for i := 0; i+1 < len(seg); i += 2 {
			points = append(points, [2]float32{seg[i], seg[i+1]})
		}
	}
	if len(points) >= 3 {
		b.paintSVGFilledPolygon(points, color, opacity)
	}
}

func (b *Builder) paintSVGStrokedPath(segments [][]float32, color frame.Color, width float32, opacity float32) {
	for _, seg := range segments {
		for i := 0; i+3 < len(seg); i += 2 {
			b.paintSVGLineSegment(seg[i], seg[i+1], seg[i+2], seg[i+3], color, width, opacity)
		}
	}
}

// parseSVGPoints parses an SVG points attribute value into coordinate pairs.
func parseSVGPoints(s string) [][2]float32 {
	var points [][2]float32
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", " ")
	fields := strings.Fields(s)
	for i := 0; i+1 < len(fields); i += 2 {
		x, err1 := strconv.ParseFloat(fields[i], 32)
		y, err2 := strconv.ParseFloat(fields[i+1], 32)
		if err1 == nil && err2 == nil {
			points = append(points, [2]float32{float32(x), float32(y)})
		}
	}
	return points
}

// parseSVGPath parses a simplified SVG path data string into line segments.
// Supports M, L, H, V, Z commands.
func parseSVGPath(d string) [][]float32 {
	var segments [][]float32
	var current []float32
	var cx, cy float32

	d = strings.TrimSpace(d)
	i := 0
	for i < len(d) {
		cmd := d[i]
		i++
		// Skip whitespace.
		for i < len(d) && (d[i] == ' ' || d[i] == ',') {
			i++
		}

		switch cmd {
		case 'M', 'm':
			if len(current) > 0 {
				segments = append(segments, current)
			}
			current = nil
			// Parse coordinates.
			coords := parsePathCoords(d, &i)
			if cmd == 'm' && len(segments) > 0 {
				// Relative move.
				prev := segments[len(segments)-1]
				if len(prev) >= 2 {
					cx = prev[len(prev)-2]
					cy = prev[len(prev)-1]
				}
			}
			for j := 0; j+1 < len(coords); j += 2 {
				x, y := coords[j], coords[j+1]
				if cmd == 'm' {
					x += cx
					y += cy
				}
				current = append(current, x, y)
				cx, cy = x, y
			}
		case 'L', 'l':
			coords := parsePathCoords(d, &i)
			for j := 0; j+1 < len(coords); j += 2 {
				x, y := coords[j], coords[j+1]
				if cmd == 'l' {
					x += cx
					y += cy
				}
				current = append(current, x, y)
				cx, cy = x, y
			}
		case 'H', 'h':
			for i < len(d) && (d[i] == ' ' || d[i] == ',') {
				i++
			}
			start := i
			for i < len(d) && (d[i] >= '0' && d[i] <= '9' || d[i] == '.' || d[i] == '-' || d[i] == 'e' || d[i] == 'E' || d[i] == '+') {
				i++
			}
			if i > start {
				x, err := strconv.ParseFloat(d[start:i], 32)
				if err == nil {
					if cmd == 'h' {
						x += float64(cx)
					}
					current = append(current, float32(x), cy)
					cx = float32(x)
				}
			}
		case 'V', 'v':
			for i < len(d) && (d[i] == ' ' || d[i] == ',') {
				i++
			}
			start := i
			for i < len(d) && (d[i] >= '0' && d[i] <= '9' || d[i] == '.' || d[i] == '-' || d[i] == 'e' || d[i] == 'E' || d[i] == '+') {
				i++
			}
			if i > start {
				y, err := strconv.ParseFloat(d[start:i], 32)
				if err == nil {
					if cmd == 'v' {
						y += float64(cy)
					}
					current = append(current, cx, float32(y))
					cy = float32(y)
				}
			}
		case 'Z', 'z':
			if len(current) >= 2 {
				current = append(current, current[0], current[1])
			}
		}
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}
	return segments
}

func parsePathCoords(d string, i *int) []float32 {
	var coords []float32
	for *i < len(d) {
		for *i < len(d) && (d[*i] == ' ' || d[*i] == ',') {
			*i++
		}
		if *i >= len(d) {
			break
		}
		// Check if next char is a command letter.
		c := d[*i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			break
		}
		start := *i
		for *i < len(d) && (d[*i] >= '0' && d[*i] <= '9' || d[*i] == '.' || d[*i] == '-' || d[*i] == 'e' || d[*i] == 'E' || d[*i] == '+') {
			*i++
		}
		if *i > start {
			f, err := strconv.ParseFloat(d[start:*i], 32)
			if err == nil {
				coords = append(coords, float32(f))
			}
		} else {
			*i++
		}
	}
	return coords
}

// fitContain returns the centered destination rect, in content px, that shows the
// whole source box scaled up or down to fit inside w x h without cropping.
func fitContain(srcBox image.Rectangle, w, h float32) (frame.RectF, bool) {
	iw, ih := float32(srcBox.Dx()), float32(srcBox.Dy())
	if iw <= 0 || ih <= 0 || w <= 0 || h <= 0 {
		return frame.RectF{}, false
	}
	scale := w / iw
	if r := h / ih; r < scale {
		scale = r
	}
	dw, dh := iw*scale, ih*scale
	x := (w - dw) / 2
	y := (h - dh) / 2
	return frame.RectF4(x, y, x+dw, y+dh), true
}

// fitCoverSrcBox returns the centred source sub-rect that fills a w x h box when
// the image is scaled by the larger ratio, cropping the overflow off the edges.
func fitCoverSrcBox(srcBox image.Rectangle, w, h float32) (image.Rectangle, bool) {
	iw, ih := float32(srcBox.Dx()), float32(srcBox.Dy())
	if iw <= 0 || ih <= 0 || w <= 0 || h <= 0 {
		return image.Rectangle{}, false
	}
	scale := w / iw
	if r := h / ih; r > scale {
		scale = r
	}
	cropW := w / scale
	cropH := h / scale
	x0 := srcBox.Min.X + int((iw-cropW)/2)
	y0 := srcBox.Min.Y + int((ih-cropH)/2)
	r := image.Rect(x0, y0, x0+int(cropW+0.5), y0+int(cropH+0.5))
	return r, !r.Empty()
}

// maxBgTiles bounds how many background tiles one box may emit, so a small
// repeating image painted across a very tall element cannot grow the display
// list without limit.
const maxBgTiles = 4096

// BGTileSize resolves one background tile's drawn size for paint and for
// the engine's post-layout vector rasterization, which must agree exactly.
// noRatio marks a source with no intrinsic ratio (no usable dimensions and
// no viewBox): cover and contain fill the positioning area instead of ratio
// math. Otherwise negotiation is standard against the natural size.
//
// Auto always tiles at the natural size: the decoder bakes the aspect
// handling (meet centering, none stretching) into the raster, so paint
// places it 1:1. Only cover/contain scale, and only lengths resolve axes;
// an auto axis follows the ratio, falling back to the natural dimension.
func BGTileSize(s *style.ComputedStyle, areaW, areaH, natW, natH float32, noRatio bool) (float32, float32) {
	tileW, tileH := natW, natH
	switch s.BackgroundSize {
	case style.BgSizeContain:
		if noRatio {
			tileW, tileH = areaW, areaH
			break
		}
		k := areaW / natW
		if h := areaH / natH; h < k {
			k = h
		}
		tileW, tileH = natW*k, natH*k
	case style.BgSizeCover:
		if noRatio {
			tileW, tileH = areaW, areaH
			break
		}
		k := areaW / natW
		if h := areaH / natH; h > k {
			k = h
		}
		tileW, tileH = natW*k, natH*k
	case style.BgSizeLength:
		wAuto := !s.BgSizeWPct && s.BgSizeW < 0
		hAuto := !s.BgSizeHPct && s.BgSizeH < 0
		if s.BgSizeWPct {
			tileW = areaW * s.BgSizeW
		} else if !wAuto {
			tileW = s.BgSizeW
		}
		if s.BgSizeHPct {
			tileH = areaH * s.BgSizeH
		} else if !hAuto {
			tileH = s.BgSizeH
		}
		switch {
		case wAuto && hAuto:
			tileW, tileH = natW, natH
		case wAuto:
			// Width auto resolves from the height through the ratio;
			// without one it keeps the natural width: only the height
			// axis defaults to the positioning area (see hAuto).
			if !noRatio && tileH > 0 && natW > 0 && natH > 0 {
				tileW = tileH * natW / natH
			} else {
				tileW = natW
			}
		case hAuto:
			// Height auto: preserve the image aspect ratio, or fill the
			// positioning area height when there is no ratio at all.
			if !noRatio && tileW > 0 && natW > 0 && natH > 0 {
				tileH = tileW * natH / natW
			} else if noRatio {
				tileH = areaH
			} else {
				tileH = natH
			}
		}
	}
	return tileW, tileH
}

// paintBackground draws a box's CSS background-image. The repeat modes tile the
// image across the border box; no-repeat places it once. background-size selects
// the tile's drawn size and background-position selects its origin. A tile that
// overflows the box edge is clipped by shrinking both its destination rect and
// the source sub-rect that maps into it, so paint never spills onto a neighbour.
func (b *Builder) paintBackground(obj *layout.Object, opacity float32) {
	src, ok := obj.BgImage.(image.Image)
	if !ok || src == nil {
		return
	}
	s := obj.Style
	if s == nil {
		return
	}
	sb := src.Bounds()
	natW, natH := float32(sb.Dx()), float32(sb.Dy())
	if natW <= 0 || natH <= 0 {
		return
	}
	tiles := bgLayerTiles(s, obj, natW, natH, obj.BgNoRatio)
	for _, t := range tiles {
		vx0, vy0, vx1, vy1 := t.vx0, t.vy0, t.vx1, t.vy1
		// Map the visible destination slice back to the image's source
		// sub-rectangle, at the tile's scale.
		sx0 := int32(float64(sb.Min.X) + (vx0-float64(t.cx))/float64(t.tw)*float64(sb.Dx()))
		sx1 := int32(float64(sb.Min.X) + (vx1-float64(t.cx))/float64(t.tw)*float64(sb.Dx()))
		sy0 := int32(float64(sb.Min.Y) + (vy0-float64(t.cy))/float64(t.th)*float64(sb.Dy()))
		sy1 := int32(float64(sb.Min.Y) + (vy1-float64(t.cy))/float64(t.th)*float64(sb.Dy()))
		if sx1 <= sx0 {
			sx1 = sx0 + 1
		}
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		dst := frame.RectF4(float32(vx0), float32(vy0), float32(vx1), float32(vy1)).ToDevice(b.scale)
		if dst.Empty() {
			continue
		}
		b.list.Append(DisplayCmd{
			Kind:    CmdImage,
			Rect:    dst,
			Image:   ImageSpec{Src: src, SrcBox: image.Rect(int(sx0), int(sy0), int(sx1), int(sy1))},
			Opacity: opacity,
		})
	}
}

// paintGradientTiles paints one gradient layer through the background
// tile machinery: gradients carry no intrinsic size, so the positioning
// area is both the size they resolve auto axes against and the natural
// size negotiation starts from. Each tile maps the ramp to itself, which
// is what makes repeat tile a gradient instead of stretching one.
func (b *Builder) paintGradientTiles(obj *layout.Object, clipRect frame.Rect, radius frame.Corners, g frame.LinearGradient, isRadial bool, rg frame.RadialGradient) {
	s := obj.Style
	if s == nil {
		return
	}
	px0, py0, px1, py1 := obj.BgAreaRect(s.BackgroundOrigin)
	areaW, areaH := px1-px0, py1-py0
	if areaW <= 0 || areaH <= 0 {
		return
	}
	for _, t := range bgLayerTiles(s, obj, areaW, areaH, true) {
		dst := frame.RectF4(float32(t.vx0), float32(t.vy0), float32(t.vx1), float32(t.vy1)).ToDevice(b.scale)
		if dst.Empty() {
			continue
		}
		if isRadial {
			b.list.Append(DisplayCmd{
				Kind:           CmdRadialGradient,
				Rect:           dst,
				RadialGradient: rg,
				Radius:         radius,
			})
		} else {
			b.list.Append(DisplayCmd{
				Kind:     CmdGradient,
				Rect:     dst,
				Gradient: g,
				Radius:   radius,
			})
		}
	}
}

// bgTile is one painted tile: cx/cy is the tile origin and tw/th its size
// (for mapping back to source pixels); vx0..vy1 is the visible slice after
// clipping, in CSS px.
type bgTile struct {
	cx, cy, tw, th     float32
	vx0, vy0, vx1, vy1 float64
}

// bgLayerTiles lays one background layer's tiles: size from BGTileSize
// against the origin box, round rescaling, positions from background-
// position, spans covering the clip box (space stays in the origin box).
// Every returned tile is non-empty and clipped to the clip box.
func bgLayerTiles(s *style.ComputedStyle, obj *layout.Object, natW, natH float32, noRatio bool) []bgTile {
	// Tiles position within the origin box and paint inside the clip box.
	px0, py0, px1, py1 := obj.BgAreaRect(s.BackgroundOrigin)
	cx0, cy0, cx1, cy1 := obj.BgAreaRect(s.BackgroundClip)
	if px1 < px0 {
		px0, px1 = px1, px0
	}
	if py1 < py0 {
		py0, py1 = py1, py0
	}
	if cx1 < cx0 {
		cx0, cx1 = cx1, cx0
	}
	if cy1 < cy0 {
		cy0, cy1 = cy1, cy0
	}
	areaW, areaH := px1-px0, py1-py0
	if areaW <= 0 || areaH <= 0 {
		return nil
	}
	ax0, ay0 := px0, py0

	tileW, tileH := BGTileSize(s, areaW, areaH, natW, natH, noRatio)
	if tileW <= 0 || tileH <= 0 {
		return nil
	}

	rx, ry := bgRepeatAxis(s.BackgroundRepeat, true), bgRepeatAxis(s.BackgroundRepeatY, false)
	// Round rescales the tile so a whole number fits the positioning area.
	if rx == style.BgRepeatRound {
		tileW = bgRoundTile(areaW, tileW)
	}
	if ry == style.BgRepeatRound {
		tileH = bgRoundTile(areaH, tileH)
	}

	ox := bgOrigin(s.BackgroundPosXMode, s.BackgroundPosX, s.BgPosXPct, ax0, areaW, tileW)
	oy := bgOrigin(s.BackgroundPosYMode, s.BackgroundPosY, s.BgPosYPct, ay0, areaH, tileH)

	// Repeating tiles cover the whole clip box, phased from the
	// positioning origin: a padding-box origin on a bordered box still
	// repeats under the transparent border. Space distributes within the
	// positioning area itself.
	sx0, sx1 := cx0, cx1
	sy0, sy1 := cy0, cy1
	if rx == style.BgRepeatSpace {
		sx0, sx1 = ax0, px1
	}
	if ry == style.BgRepeatSpace {
		sy0, sy1 = ay0, py1
	}
	xs := bgAxisSpans(rx, ox, tileW, sx0, sx1, areaW)
	ys := bgAxisSpans(ry, oy, tileH, sy0, sy1, areaH)
	if len(xs)*len(ys) == 0 || len(xs)*len(ys) > maxBgTiles {
		return nil
	}
	var out []bgTile
	for _, cx := range xs {
		for _, cy := range ys {
			vx0, vy0 := float64(cx), float64(cy)
			vx1, vy1 := float64(cx+tileW), float64(cy+tileH)
			// Painting clips to the clip box, which may extend past the
			// positioning area the tiles phase from.
			if float32(vx0) < cx0 {
				vx0 = float64(cx0)
			}
			if float32(vy0) < cy0 {
				vy0 = float64(cy0)
			}
			if float32(vx1) > cx1 {
				vx1 = float64(cx1)
			}
			if float32(vy1) > cy1 {
				vy1 = float64(cy1)
			}
			if vx1 <= vx0 || vy1 <= vy0 {
				continue
			}
			out = append(out, bgTile{cx: cx, cy: cy, tw: tileW, th: tileH, vx0: vx0, vy0: vy0, vx1: vx1, vy1: vy1})
		}
	}
	return out
}

// bgOrigin resolves one background-position axis into a CSS-px coordinate where
// a tile of the given size begins within a span of `areaLen` starting at `base`.
// A percentage aligns `length` of the image with `length` of the free space; a
// length is a plain offset from the start edge. The three/four-value offsets
// measure from their own keyword edge instead: `right 25px` sits 25px left of
// the end, and a percentage offset resolves against the free space.
func bgOrigin(mode style.BgPosMode, length float32, isPct bool, base, areaLen, tileSize float32) float32 {
	free := areaLen - tileSize
	switch mode {
	case style.BgPosCenter:
		return base + free/2
	case style.BgPosEnd:
		return base + free
	case style.BgPosLength:
		if isPct {
			return base + free*length
		}
		return base + length
	case style.BgPosStartOffset:
		if isPct {
			return base + free*length
		}
		return base + length
	case style.BgPosEndOffset:
		if isPct {
			return base + free - free*length
		}
		return base + free - length
	default: // BgPosStart
		return base
	}
}

// bgRepeatAxis normalizes one repeat mode for the axis being laid out: the
// legacy repeat-x/repeat-y spellings repeat only on their own axis, while
// every other mode applies wherever it is stored.
func bgRepeatAxis(mode style.BgRepeat, isX bool) style.BgRepeat {
	switch mode {
	case style.BgRepeatRepeat:
		return style.BgRepeatRepeat
	case style.BgRepeatRepeatX:
		if isX {
			return style.BgRepeatRepeat
		}
		return style.BgRepeatNoRepeat
	case style.BgRepeatRepeatY:
		if !isX {
			return style.BgRepeatRepeat
		}
		return style.BgRepeatNoRepeat
	default:
		return mode
	}
}

// bgRoundTile rescales a tile so a whole number fits the positioning area:
// the count is the nearest whole number of tiles, at least one, and the
// tile stretches or squeezes to exactly fill the area with that count.
func bgRoundTile(areaLen, tileLen float32) float32 {
	if tileLen <= 0 || areaLen <= 0 {
		return tileLen
	}
	n := int(areaLen/tileLen + 0.5)
	if n < 1 {
		n = 1
	}
	return areaLen / float32(n)
}

// bgAxisSpans lists the start coordinate of every tile along one axis. A
// repeating (or round, which repeats its rescaled tile) axis phases from
// the origin and fills the whole span; space lays whole unclipped tiles
// flush with both edges and spreads the leftover evenly, centering a lone
// tile; anything else contributes only the single placed tile.
func bgAxisSpans(mode style.BgRepeat, origin, size, lo, hi, areaLen float32) []float32 {
	switch mode {
	case style.BgRepeatSpace:
		if size <= 0 {
			return nil
		}
		n := int(areaLen / size)
		if n <= 1 {
			// One tile (or none fitting): center it in the area.
			return []float32{lo + (areaLen-size)/2}
		}
		if n > maxBgTiles {
			n = maxBgTiles
		}
		gap := (areaLen - float32(n)*size) / float32(n-1)
		out := make([]float32, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, lo+float32(i)*(size+gap))
		}
		return out
	case style.BgRepeatRepeat, style.BgRepeatRound:
		start := origin
		// Walk the phase back to the edge so tiling covers the whole area.
		for i := 0; start > lo && i < maxBgTiles; i++ {
			start -= size
		}
		var out []float32
		for x := start; x < hi && len(out) < maxBgTiles; x += size {
			out = append(out, x)
		}
		return out
	default:
		return []float32{origin}
	}
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
	// A composition preview takes the placeholder's place on an empty control.
	if b.focus == obj && b.focusMarked != "" {
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
	if b.focus == nil || b.focusMarked == "" || obj.Node == nil {
		return text
	}
	isInput := obj == b.focus
	isTextareaText := obj.Node.Parent == b.focus.Node && obj.Node.Text()
	if !isInput && !isTextareaText {
		return text
	}
	runes := []rune(text)
	caret := b.focusCaret
	if caret > len(runes) {
		caret = len(runes)
	}
	if caret < 0 {
		caret = 0
	}
	marked := []rune(b.focusMarked)
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
	text := obj.Node.GetAttribute("value")
	if text == "" {
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
	// Simple downward triangle: 3 rows of increasing width.
	for i := int32(0); i < 3; i++ {
		halfW := i + 1
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
	caret := b.focusCaret
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
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    rect,
		Color:   frame.RGB(0, 0, 0),
		Opacity: 1,
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
		if thick < 1 {
			thick = 1
		}
		y0 := baseline + int32(thick)
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

func isSVGShape(tag string) bool {
	switch tag {
	case "rect", "circle", "ellipse", "line", "polygon", "polyline", "path":
		return true
	}
	return false
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

func radialGradient(g style.RadialGradient, opacity float32) frame.RadialGradient {
	if g.Empty() {
		return frame.RadialGradient{}
	}
	stops := make([]frame.GradientStop, 0, len(g.Stops))
	for _, s := range g.Stops {
		c := convertColor(s.Color)
		if opacity > 0 && opacity < 1 {
			c = frame.ScaleColor(c, opacity)
		}
		stops = append(stops, frame.GradientStop{At: s.At, Color: c})
	}
	return frame.RadialGradient{Shape: g.Shape, Cx: g.Cx, Cy: g.Cy, Stops: stops}
}

// paintBoxShadow draws each non-inset box-shadow layer as an offset coloured
// rectangle behind the element. An outer shadow is clipped to the outside of
// the border box (CSS Backgrounds §7.1.1): the element covers its own
// interior, so only the ring outside `rect` is emitted. Blur is approximated
// with up to three concentric layers at decreasing opacity; a real Gaussian
// blur is not available in the tile rasterizer.
func (b *Builder) paintBoxShadow(obj *layout.Object, rect frame.Rect, radius frame.Corners, opacity float32) {
	s := obj.Style
	for _, sh := range s.BoxShadow {
		if sh.Inset {
			// Inset shadows paint inside the box; simplified to a no-op for now
			// since they require clipping to the box interior.
			continue
		}
		dx := int32(sh.OffsetX * b.scale)
		dy := int32(sh.OffsetY * b.scale)
		spread := int32(sh.Spread * b.scale)
		shadowRect := frame.Rect4(
			rect.X0+dx-spread,
			rect.Y0+dy-spread,
			rect.X1+dx+spread,
			rect.Y1+dy+spread,
		)
		// A zero-size box still casts when blurred or spread: the layers below
		// inflate the rect. Only a truly empty shadow (no size, no blur, no
		// spread) paints nothing.
		blurLayers := int32(sh.Blur * b.scale)
		if shadowRect.Empty() && blurLayers <= 0 {
			continue
		}
		color := convertColor(sh.Color)
		if sh.ColorIsCurrent {
			color = convertColor(s.Color)
		}
		// Approximate blur with concentric layers. Each layer is slightly
		// larger and more transparent than the one before it.
		if blurLayers <= 0 {
			b.appendShadowRing(shadowRect, rect, radius, color, opacity)
			continue
		}
		// Draw from outermost (most transparent) to innermost (fully opaque).
		const maxBlurSteps = 4
		steps := blurLayers / 2
		if steps < 1 {
			steps = 1
		}
		if steps > maxBlurSteps {
			steps = maxBlurSteps
		}
		for i := steps; i >= 1; i-- {
			expand := int32(i) * blurLayers / steps
			layerRect := frame.Rect4(
				shadowRect.X0-expand,
				shadowRect.Y0-expand,
				shadowRect.X1+expand,
				shadowRect.Y1+expand,
			)
			// Alpha fades from nearly transparent (outermost) to the full
			// shadow color (innermost).
			frac := float32(i) / float32(steps)
			layerOpacity := opacity * frac
			b.appendShadowRing(layerRect, rect, radius, color, layerOpacity)
		}
	}
}

// appendShadowRing emits layerRect minus the border-box interior: an outer
// shadow never paints under its own element. Square corners decompose into
// four exact strips; rounded corners keep the full rect because the rasterizer
// has no rounded hole-punch and the element's own background covers the
// interior on every opaque test.
func (b *Builder) appendShadowRing(layerRect, borderBox frame.Rect, radius frame.Corners, color frame.Color, opacity float32) {
	if layerRect.Empty() {
		return
	}
	if !radius.Empty() {
		b.list.Append(DisplayCmd{
			Kind:    CmdFill,
			Rect:    layerRect,
			Color:   color,
			Radius:  radius,
			Opacity: opacity,
		})
		return
	}
	strips := []frame.Rect{
		// Top and bottom span the full shadow width.
		frame.Rect4(layerRect.X0, layerRect.Y0, layerRect.X1, borderBox.Y0),
		frame.Rect4(layerRect.X0, borderBox.Y1, layerRect.X1, layerRect.Y1),
		// Left and right fill between them.
		frame.Rect4(layerRect.X0, borderBox.Y0, borderBox.X0, borderBox.Y1),
		frame.Rect4(borderBox.X1, borderBox.Y0, layerRect.X1, borderBox.Y1),
	}
	for _, r := range strips {
		if r.Empty() {
			continue
		}
		b.list.Append(DisplayCmd{
			Kind:    CmdFill,
			Rect:    r,
			Color:   color,
			Opacity: opacity,
		})
	}
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

// applyTransform resolves the element's CSS transform to a matrix and applies
// it to every command emitted for this element (from cmdStart to end of list).
// Each command's device-space rect is mapped back to layout space, transformed,
// then converted to an AABB in device space.
func (b *Builder) applyTransform(obj *layout.Object, cmdStart int, bx0, by0, bx1, by1 float32) {
	s := obj.Style
	if s == nil || len(s.Transform) == 0 {
		return
	}
	elemW := bx1 - bx0
	elemH := by1 - by0
	if elemW <= 0 || elemH <= 0 {
		return
	}
	mat := ResolveTransform(s.Transform, s.TransformOrigin, elemW, elemH)
	if mat.IsIdentity() {
		return
	}
	invScale := float32(1)
	if b.scale != 0 {
		invScale = 1.0 / b.scale
	}
	cmds := b.list.All()
	for i := cmdStart; i < len(cmds); i++ {
		// Map the device-space rect back to layout space, transform, then
		// convert the AABB back to device space.
		r := cmds[i].Rect
		lr := frame.RectF4(
			float32(r.X0)*invScale,
			float32(r.Y0)*invScale,
			float32(r.X1)*invScale,
			float32(r.Y1)*invScale,
		)
		transformed := mat.TransformRect(lr)
		cmds[i].Rect = transformed.ToDevice(b.scale)
	}
}
