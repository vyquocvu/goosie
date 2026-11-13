package paint

import (
	"image"
	"math"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

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
			// Inset layers belong over the background and under the border, so
			// they are emitted by paintInsetBoxShadow at that point in the order
			// rather than behind the element here.
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

// insetBlurSamples is how many points an inset layer's fitted falloff is
// sampled at. Five samples keep every pixel of the swept Chromium profiles
// within 0.016 of the measured alpha - 4 levels of an 8-bit ramp.
const insetBlurSamples = 5

// insetMaskCap bounds one masked inset cell's resolution. Stretching is safe
// because a cell only reaches the cap when the blur is wide, and a wide blur is
// a flat curve: the coverage slope is 1/(sigma*sqrt(2*pi)), so the error one
// block introduces stays within the levels the ramp fit already costs.
const insetMaskCap = 64

// insetMaskBudget bounds the area that may be masked pixel by pixel. A cell's
// mask never holds more entries than the cell has pixels, so the same number
// caps the mask memory and the per-pixel work, and one tile's worth is what the
// rasterizer already agrees to cost for a damaged tile. Past it an inset layer
// whose bands cross falls back to stacked ramps.
const insetMaskBudget = 65536

// paintInsetBoxShadow draws every `inset` box-shadow layer inside the box.
//
// CSS Backgrounds §4.4 places an inset shadow over the background and under the
// border, and Chromium clips it to the padding box: measured with a 4px
// translucent red border over `inset 0 0 8px rgba(0,0,255,.5)`, the border band
// reads border-over-background exactly (248,120,120) - the blue stops at the
// padding edge, where the blurred profile peaks at the same 0.221 it peaks at on
// a borderless box.
//
// The tile rasterizer has no blur, so each edge gets the Gaussian a real blur
// integrates: with sigma = blur/2, the alpha surviving at distance x from the
// edge is Phi((depth - x)/sigma) of the declared alpha, where the sharp depth is
// spread + the offset component pointing at that edge. Both halves are measured:
// `inset 4px 4px 0 2px` inks 6px along the left and top and nothing along the
// right and bottom, `inset 0 0 0 6px` inks 6px on all four edges, and `inset 0 0
// 0 0` inks nothing at all. A blurred edge keeps that depth as the centre of its
// falloff and is sampled into one linear ramp.
//
// The ramps cannot simply cover the box's four edges, because a corner belongs to
// two of them. Blurring an inset layer convolves a rectangular hole with a
// separable kernel, so the corner's coverage is 1-(1-cx)(1-cy) of the declared
// alpha - measured on `inset 3px 3px 6px rgba(0,0,0,.4)` as 0.367 against the
// model's 0.384 - while compositing two ramps source-over multiplies their
// (1-alpha) terms and lands at 0.521. Corners therefore get cells of their own,
// each one masked fill carrying that product per pixel. A blur-free layer needs
// none of that: it paints the union of its four bands once each, because its
// hard-edged corner measures the declared alpha rather than a doubled composite.
func (b *Builder) paintInsetBoxShadow(obj *layout.Object, paint boxPaintContext) {
	s := obj.Style
	if s == nil {
		return
	}
	px0, py0, px1, py1 := obj.BgAreaRect(style.BgBoxPadding)
	pad := frame.RectF4(px0, py0, px1, py1).ToDevice(b.scale)
	if pad.Empty() {
		return
	}
	// The band follows the box's corner arcs, so it needs the padding box's
	// radius: the border box's radius shrunk by each border width, exactly as the
	// background clip's radius is shrunk above it.
	radius := paint.radius.Shrunk(
		pad.Y0-paint.rect.Y0, paint.rect.X1-pad.X1,
		paint.rect.Y1-pad.Y1, pad.X0-paint.rect.X0)
	// Layers are emitted back to front: CSS stacks the first declared shadow on
	// top of the later ones.
	for i := len(s.BoxShadow) - 1; i >= 0; i-- {
		sh := s.BoxShadow[i]
		if !sh.Inset {
			continue
		}
		color := convertColor(sh.Color)
		if sh.ColorIsCurrent {
			color = convertColor(s.Color)
		}
		if paint.opacity > 0 && paint.opacity < 1 {
			color = frame.ScaleColor(color, paint.opacity)
		}
		if color.A() == 0 {
			continue
		}
		dx := int32(sh.OffsetX * b.scale)
		dy := int32(sh.OffsetY * b.scale)
		spread := int32(sh.Spread * b.scale)
		sigma := float32(sh.Blur) * b.scale / 2
		left, right := spread+dx, spread-dx
		top, bottom := spread+dy, spread-dy
		if sigma <= 0 {
			b.appendInsetSharpBands(pad, left, right, top, bottom, color, radius)
			continue
		}
		b.appendInsetBlurredField(pad, left, right, top, bottom, sigma, color, radius)
	}
}

// appendInsetSharpBands paints a blur-free inset layer as one union of the four
// edge bands. A hard-edged layer inks every pixel outside the hole rect, so a
// corner belongs to the band that reaches it and must not be composited twice:
// measured on `inset 4px 4px 0 2px rgba(0,0,255,.5)`, the 6x6 corner is
// (120,120,248) - the same 0.5 the mid-edge carries - where stacking two fills
// would have measured 0.75.
func (b *Builder) appendInsetSharpBands(rect frame.Rect, left, right, top, bottom int32, color frame.Color, radius frame.Corners) {
	clamp := func(v, dim int32) int32 {
		if v < 0 {
			return 0
		}
		if v > dim {
			return dim
		}
		return v
	}
	left, right = clamp(left, rect.W()), clamp(right, rect.W())
	top, bottom = clamp(top, rect.H()), clamp(bottom, rect.H())
	if left+right >= rect.W() || top+bottom >= rect.H() {
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: rect, Color: color, Radius: radius})
		return
	}
	appends := []frame.Rect{
		frame.Rect4(rect.X0, rect.Y0, rect.X0+left, rect.Y1),
		frame.Rect4(rect.X1-right, rect.Y0, rect.X1, rect.Y1),
		// The horizontal bands run between the vertical ones, so the four cover
		// the union exactly once.
		frame.Rect4(rect.X0+left, rect.Y0, rect.X1-right, rect.Y0+top),
		frame.Rect4(rect.X0+left, rect.Y1-bottom, rect.X1-right, rect.Y1),
	}
	for _, r := range appends {
		if r.Empty() {
			continue
		}
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: r, Color: color, Radius: radius})
	}
}

// insetReach is how far one blurred edge of an inset layer carries ink: its
// sharp depth plus the two standard deviations of Gaussian tail that still hold
// 2.3% of it, rounded up and stopped at the box's far edge. The depth stays
// signed. A band pushed off an edge by an offset leaves a negative depth there
// and only its tail leaks in: measured on `inset 4px 0 8px 2px`, the edge
// opposite the offset carries 0.133 of the declared alpha at its border and dies
// two pixels in, while the sharp `inset 4px 4px 0 2px` paints nothing there.
func insetReach(depth, dim int32, sigma float32) int32 {
	reach := float32(depth) + 2*sigma
	if reach > float32(dim) {
		reach = float32(dim)
	}
	if reach <= 0 {
		return 0
	}
	return int32(math.Ceil(float64(reach)))
}

// appendInsetBlurredField paints one blurred inset layer inside rect: an edge
// ramp per side that stops short of the corners, then one square per corner
// carrying what the two ramps cannot. `left`, `right`, `top` and `bottom` are
// the signed sharp depths, sigma the blur's standard deviation.
//
// The partition is the whole box's field cut into cells that each depend on one
// axis alone, which is the only shape the rasterizer can ink exactly. Opposite
// bands that cross - a box narrower than two blurs - leave no such middle cell,
// so the field goes out as cells at every pixel instead, each one carrying both
// axes' product. What remains for the stacked full-length ramps is the crossing
// box too large to mask and the rounded corner, which a masked fill drops: both
// stay exact at a declared alpha of 1 and short of it by a(1-a)cx*cy elsewhere,
// which the corner-square doc above measures.
func (b *Builder) appendInsetBlurredField(rect frame.Rect, left, right, top, bottom int32, sigma float32, color frame.Color, radius frame.Corners) {
	tl, tr := insetReach(left, rect.W(), sigma), insetReach(right, rect.W(), sigma)
	tt, tb := insetReach(top, rect.H(), sigma), insetReach(bottom, rect.H(), sigma)
	crossing := tl+tr >= rect.W() || tt+tb >= rect.H()
	switch {
	case crossing && int(rect.W())*int(rect.H()) <= insetMaskBudget:
		b.appendInsetMaskedCells(rect, left, right, top, bottom, sigma, color)
	case crossing || !radius.Empty():
		b.appendInsetRamp(frame.Rect4(rect.X0, rect.Y0, rect.X0+tl, rect.Y1), 90, left, tl, sigma, color, radius)
		b.appendInsetRamp(frame.Rect4(rect.X1-tr, rect.Y0, rect.X1, rect.Y1), 270, right, tr, sigma, color, radius)
		b.appendInsetRamp(frame.Rect4(rect.X0, rect.Y0, rect.X1, rect.Y0+tt), 180, top, tt, sigma, color, radius)
		b.appendInsetRamp(frame.Rect4(rect.X0, rect.Y1-tb, rect.X1, rect.Y1), 0, bottom, tb, sigma, color, radius)
	default:
		b.appendInsetRamp(frame.Rect4(rect.X0, rect.Y0+tt, rect.X0+tl, rect.Y1-tb), 90, left, tl, sigma, color, radius)
		b.appendInsetRamp(frame.Rect4(rect.X1-tr, rect.Y0+tt, rect.X1, rect.Y1-tb), 270, right, tr, sigma, color, radius)
		b.appendInsetRamp(frame.Rect4(rect.X0+tl, rect.Y0, rect.X1-tr, rect.Y0+tt), 180, top, tt, sigma, color, radius)
		b.appendInsetRamp(frame.Rect4(rect.X0+tl, rect.Y1-tb, rect.X1-tr, rect.Y1), 0, bottom, tb, sigma, color, radius)
		b.appendInsetMaskedCell(rect, frame.Rect4(rect.X0, rect.Y0, rect.X0+tl, rect.Y0+tt), left, right, top, bottom, sigma, color)
		b.appendInsetMaskedCell(rect, frame.Rect4(rect.X1-tr, rect.Y0, rect.X1, rect.Y0+tt), left, right, top, bottom, sigma, color)
		b.appendInsetMaskedCell(rect, frame.Rect4(rect.X0, rect.Y1-tb, rect.X0+tl, rect.Y1), left, right, top, bottom, sigma, color)
		b.appendInsetMaskedCell(rect, frame.Rect4(rect.X1-tr, rect.Y1-tb, rect.X1, rect.Y1), left, right, top, bottom, sigma, color)
	}
}

// appendInsetRamp paints one blurred edge strip of one inset layer: a band
// `thickness` device pixels deep, measured inward from the edge the ramp runs
// away from. Angle 90 leaves the band's left edge, 270 its right, 180 its top
// and 0 its bottom, which is the only direction convention the gradient
// rasterizer reads.
func (b *Builder) appendInsetRamp(band frame.Rect, angle float32, depth, thickness int32, sigma float32, color frame.Color, radius frame.Corners) {
	if band.Empty() || thickness <= 0 {
		return
	}
	stops := make([]frame.GradientStop, insetBlurSamples)
	for i := range stops {
		t := float32(i) / float32(insetBlurSamples-1)
		stops[i] = frame.GradientStop{
			At:    t,
			Color: frame.ScaleColor(color, insetCoverage(float32(thickness)*t, float32(depth), sigma)),
		}
	}
	b.list.Append(DisplayCmd{
		Kind:     CmdGradient,
		Rect:     band,
		Gradient: frame.LinearGradient{Angle: angle, Stops: stops},
		Radius:   radius,
	})
}

// insetFieldCoverage is the fraction of an inset layer's declared alpha that
// survives at (x, y) device pixels inside a w x h box. Blurring an inset layer
// convolves a rectangular hole with a separable Gaussian, so each axis keeps the
// sum of the two tails that reach the point and the point is inked by what either
// axis owes it: 1-(1-cx)(1-cy). Measured on `inset 3px 3px 6px rgba(0,0,0,.4)`,
// the corner reads 0.367, which this model gives as 0.384 and stacking two ramps
// source-over gives as 0.521.
func insetFieldCoverage(x, y float32, w, h int32, left, right, top, bottom int32, sigma float32) float32 {
	cx := insetCoverage(x, float32(left), sigma) + insetCoverage(float32(w)-x, float32(right), sigma)
	cy := insetCoverage(y, float32(top), sigma) + insetCoverage(float32(h)-y, float32(bottom), sigma)
	if cx > 1 {
		cx = 1
	}
	if cy > 1 {
		cy = 1
	}
	return 1 - (1-cx)*(1-cy)
}

// appendInsetMaskedCell paints one cell of a blurred inset field, distances
// measured from the layer rect's edges rather than the cell's own. Source-over
// cannot produce the product of coverages that field is (it multiplies the
// un-inked fractions, not the ink the layer is owed), so the cell goes out as
// one fill of the layer's colour whose mask carries the product per pixel.
func (b *Builder) appendInsetMaskedCell(layer, cell frame.Rect, left, right, top, bottom int32, sigma float32, color frame.Color) {
	w, h := int(cell.W()), int(cell.H())
	if w <= 0 || h <= 0 {
		return
	}
	mw, mh := min(w, insetMaskCap), min(h, insetMaskCap)
	// Mask pixel i stands for the device block whose centre is (i+0.5)*w/mw: at
	// the cap the fill stretches it, and drawMaskedFill samples a mask by nearest
	// device pixel, so one value covers the whole block.
	m := image.NewAlpha(image.Rect(0, 0, mw, mh))
	ox, oy := float32(cell.X0-layer.X0), float32(cell.Y0-layer.Y0)
	for j := 0; j < mh; j++ {
		y := oy + (float32(j)+0.5)*float32(h)/float32(mh)
		row := j * m.Stride
		for i := 0; i < mw; i++ {
			x := ox + (float32(i)+0.5)*float32(w)/float32(mw)
			m.Pix[row+i] = uint8(255*insetFieldCoverage(x, y, layer.W(), layer.H(), left, right, top, bottom, sigma) + 0.5)
		}
	}
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: cell, Color: color, Mask: m})
}

// appendInsetMaskedCells tiles a whole box into masked cells of insetMaskCap
// device pixels, which is how a field whose bands cross gets its corners and its
// interior at once. The last row and column take the remainder.
func (b *Builder) appendInsetMaskedCells(rect frame.Rect, left, right, top, bottom int32, sigma float32, color frame.Color) {
	step := int32(insetMaskCap)
	for y := rect.Y0; y < rect.Y1; y += step {
		for x := rect.X0; x < rect.X1; x += step {
			b.appendInsetMaskedCell(rect, frame.Rect4(x, y, min(x+step, rect.X1), min(y+step, rect.Y1)), left, right, top, bottom, sigma, color)
		}
	}
}

// insetCoverage is the fraction of an inset layer's alpha that survives x device
// pixels inward from the edge whose blurred ink reaches depth: the tail of the
// Gaussian Skia convolves that edge with, written as the normal CDF.
func insetCoverage(x, depth, sigma float32) float32 {
	z := float64(depth-x) / (float64(sigma) * math.Sqrt2)
	return float32(0.5 * (1 + math.Erf(z)))
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
