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
	focus      *layout.Object
	focusCaret int
	selection  map[*layout.Object]SelSpan
}

// NewBuilder returns a builder that will emit commands into list. metrics is
// the same glyph-advance source layout measured with; nil falls back to the
// half-em estimate layout used, which spaces glyphs approximately.
func NewBuilder(list *List, arena *layout.Arena, scale float32, metrics layout.Metrics) *Builder {
	return &Builder{list: list, arena: arena, scale: scale, metrics: metrics}
}

// SetFocus marks the control to paint with a caret and focus ring. caret is a
// rune index into the control's value.
func (b *Builder) SetFocus(obj *layout.Object, caret int) {
	b.focus = obj
	b.focusCaret = caret
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
		if !clipHidden {
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
			b.paintBackground(obj, opacity)
			if obj.Style.Display != style.DisplayInline && obj.Style.Display != style.DisplayNone {
				b.paintBorders(obj, rect, radius, opacity)
			}
			if obj.Style.Display == style.DisplayListItem {
				b.paintMarker(obj, ordinal)
			}
			if obj.Node != nil && obj.Node.Type == 2 && obj.Node.DataContent != "" {
				b.paintText(obj, rect, opacity, clip)
			}
			if obj.Node != nil && obj.Node.Data == "input" {
				b.paintControlValue(obj, opacity, clip)
				b.paintPlaceholder(obj, opacity, clip)
			}
			if obj.Node != nil && obj.Node.Data == "img" {
				b.paintImage(obj, opacity, clip)
			}
			if b.focus != nil && obj == b.focus {
				b.paintFocus(obj, clip)
			}
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
	if sp, ok := b.selection[obj]; ok {
		b.paintSelectionHighlight(obj, rect, s, sp, opacity)
	}
	b.appendRun(obj.Node.DataContent, rect, s, convertColor(s.Color), opacity)
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
	x0, y0, x1, y1 := obj.BorderRect()
	if x1 <= x0 || y1 <= y0 {
		return
	}
	ax0, ay0, ax1, ay1 := x0, y0, x1, y1
	if ax1 < ax0 {
		ax0, ax1 = ax1, ax0
	}
	if ay1 < ay0 {
		ay0, ay1 = ay1, ay0
	}
	areaW, areaH := ax1-ax0, ay1-ay0

	tileW, tileH := natW, natH
	switch s.BackgroundSize {
	case style.BgSizeContain:
		k := areaW / natW
		if h := areaH / natH; h < k {
			k = h
		}
		tileW, tileH = natW*k, natH*k
	case style.BgSizeCover:
		k := areaW / natW
		if h := areaH / natH; h > k {
			k = h
		}
		tileW, tileH = natW*k, natH*k
	case style.BgSizeLength:
		if s.BgSizeWPct {
			tileW = areaW * s.BgSizeW
		} else {
			tileW = s.BgSizeW
		}
		switch {
		case s.BgSizeH < 0: // height auto: preserve the image aspect ratio
			if tileW > 0 {
				tileH = natH * (tileW / natW)
			}
		case s.BgSizeHPct:
			tileH = areaH * s.BgSizeH
		default:
			tileH = s.BgSizeH
		}
	}
	if tileW <= 0 || tileH <= 0 {
		return
	}

	repeatX := s.BackgroundRepeat == style.BgRepeatRepeat || s.BackgroundRepeat == style.BgRepeatRepeatX
	repeatY := s.BackgroundRepeat == style.BgRepeatRepeat || s.BackgroundRepeat == style.BgRepeatRepeatY

	ox := bgOrigin(s.BackgroundPosXMode, s.BackgroundPosX, s.BgPosXPct, ax0, areaW, tileW)
	oy := bgOrigin(s.BackgroundPosYMode, s.BackgroundPosY, s.BgPosYPct, ay0, areaH, tileH)

	xs := bgSpans(ox, tileW, ax0, ax1, repeatX)
	ys := bgSpans(oy, tileH, ay0, ay1, repeatY)
	if len(xs)*len(ys) == 0 || len(xs)*len(ys) > maxBgTiles {
		return
	}
	for _, cx := range xs {
		for _, cy := range ys {
			vx0, vy0 := math.Max(float64(cx), float64(ax0)), math.Max(float64(cy), float64(ay0))
			vx1, vy1 := math.Min(float64(cx+tileW), float64(ax1)), math.Min(float64(cy+tileH), float64(ay1))
			if vx1 <= vx0 || vy1 <= vy0 {
				continue
			}
			// Map the visible destination slice back to the image's source
			// sub-rectangle, at the tile's scale.
			sx0 := int32(float64(sb.Min.X) + (vx0-float64(cx))/float64(tileW)*float64(sb.Dx()))
			sx1 := int32(float64(sb.Min.X) + (vx1-float64(cx))/float64(tileW)*float64(sb.Dx()))
			sy0 := int32(float64(sb.Min.Y) + (vy0-float64(cy))/float64(tileH)*float64(sb.Dy()))
			sy1 := int32(float64(sb.Min.Y) + (vy1-float64(cy))/float64(tileH)*float64(sb.Dy()))
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
}

// bgOrigin resolves one background-position axis into a CSS-px coordinate where
// a tile of the given size begins within a span of `areaLen` starting at `base`.
// A percentage aligns `length` of the image with `length` of the free space; a
// length is a plain offset from the start edge.
func bgOrigin(mode style.BgPosMode, length float32, isPct bool, base, areaLen, tileSize float32) float32 {
	switch mode {
	case style.BgPosCenter:
		return base + (areaLen-tileSize)/2
	case style.BgPosEnd:
		return base + areaLen - tileSize
	case style.BgPosLength:
		if isPct {
			return base + (areaLen-tileSize)*length
		}
		return base + length
	default: // BgPosStart
		return base
	}
}

// bgSpans lists the start coordinate of every tile along one axis. A repeating
// axis phases from origin and fills the whole [min,max) span; a non-repeating
// axis contributes only the single placed tile.
func bgSpans(origin, size, lo, hi float32, repeat bool) []float32 {
	if !repeat {
		return []float32{origin}
	}
	start := origin
	// Walk the phase back to the left edge so tiling covers the whole area.
	for i := 0; start > lo && i < maxBgTiles; i++ {
		start -= size
	}
	var out []float32
	for x := start; x < hi && len(out) < maxBgTiles; x += size {
		out = append(out, x)
	}
	return out
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

// paintControlValue draws the text an input holds. The value is not in the DOM
// text flow, so like the placeholder it starts at the content origin bounded
// by the content box. Textareas paint through the normal text path instead.
func (b *Builder) paintControlValue(obj *layout.Object, opacity float32, clip *frame.Rect) {
	s := obj.Style
	if s == nil || obj.Node == nil || !obj.Node.Element() {
		return
	}
	text := obj.Node.GetAttribute("value")
	if text == "" {
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
		if c.Type == 2 {
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
		if k.Node != nil && k.Node.Type == 2 && k.Node.DataContent != "" {
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
