package paint

import (
	"image"
	"math"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func (b *Builder) paintBoxShadow(obj *layout.Object, paint boxPaintContext) {
	s := obj.Style
	floored := frame.Rect{
		X0: int32(math.Floor(float64(paint.x0 * b.scale))),
		Y0: int32(math.Floor(float64(paint.y0 * b.scale))),
		X1: int32(math.Floor(float64(paint.x1 * b.scale))),
		Y1: int32(math.Floor(float64(paint.y1 * b.scale))),
	}
	radius := paint.radius
	opacity := paint.opacity
	for i := len(s.BoxShadow) - 1; i >= 0; i-- {
		sh := s.BoxShadow[i]
		if sh.Inset {
			continue
		}
		rect := paint.rect
		if sh.OffsetX != 0 || sh.OffsetY != 0 {
			rect = floored
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
		blurLayers := int32(sh.Blur * b.scale)
		if shadowRect.Empty() && blurLayers <= 0 {
			continue
		}
		color := convertColor(sh.Color)
		if sh.ColorIsCurrent {
			color = convertColor(s.Color)
		}
		sigma := float32(blurLayers) / 2
		if sigma <= 0 {
			b.appendShadowRing(shadowRect, rect, radius, color, opacity)
			continue
		}
		if !radius.Empty() || outerFieldArea(shadowRect, rect, sigma) > int64(fieldMaskBudget) {
			b.appendBlurredRings(shadowRect, rect, radius, color, opacity, blurLayers)
			continue
		}
		b.appendOuterBlurredField(shadowRect, rect, sigma, color, opacity)
	}
}

func (b *Builder) appendBlurredRings(shadowRect, rect frame.Rect, radius frame.Corners, color frame.Color, opacity float32, blurLayers int32) {
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
		frac := float32(i) / float32(steps)
		layerOpacity := opacity * frac
		b.appendShadowRing(layerRect, rect, radius, color, layerOpacity)
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
		frame.Rect4(layerRect.X0, layerRect.Y0, layerRect.X1, borderBox.Y0),
		frame.Rect4(layerRect.X0, borderBox.Y1, layerRect.X1, layerRect.Y1),
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

func outerBlurReach(sigma float32) int32 {
	return int32(math.Ceil(3 * float64(sigma)))
}

func outerFieldArea(shadowRect, borderBox frame.Rect, sigma float32) int64 {
	reach := outerBlurReach(sigma)
	field := frame.Rect4(
		shadowRect.X0-reach,
		shadowRect.Y0-reach,
		shadowRect.X1+reach,
		shadowRect.Y1+reach,
	)
	inside := field.Intersection(borderBox)
	return int64(field.W())*int64(field.H()) - int64(inside.W())*int64(inside.H())
}

func (b *Builder) appendOuterBlurredField(shadowRect, borderBox frame.Rect, sigma float32, color frame.Color, opacity float32) {
	reach := outerBlurReach(sigma)
	field := frame.Rect4(
		shadowRect.X0-reach,
		shadowRect.Y0-reach,
		shadowRect.X1+reach,
		shadowRect.Y1+reach,
	)
	strips := []frame.Rect{
		frame.Rect4(field.X0, field.Y0, field.X1, borderBox.Y0),
		frame.Rect4(field.X0, borderBox.Y1, field.X1, field.Y1),
		frame.Rect4(field.X0, borderBox.Y0, borderBox.X0, borderBox.Y1),
		frame.Rect4(borderBox.X1, borderBox.Y0, field.X1, borderBox.Y1),
	}
	for _, strip := range strips {
		b.appendOuterFieldStrip(strip, shadowRect, sigma, color, opacity)
	}
}

func (b *Builder) appendOuterFieldStrip(strip, shadowRect frame.Rect, sigma float32, color frame.Color, opacity float32) {
	if strip.Empty() {
		return
	}
	cols := outerAxisProfile(strip.X0, int(strip.W()), shadowRect.X0, shadowRect.X1, sigma)
	rows := outerAxisProfile(strip.Y0, int(strip.H()), shadowRect.Y0, shadowRect.Y1, sigma)
	step := int32(fieldMaskCap)
	for cy, ry := strip.Y0, 0; cy < strip.Y1; cy, ry = cy+step, ry+int(step) {
		rh := min(int(step), int(strip.H())-ry)
		for cx, rx := strip.X0, 0; cx < strip.X1; cx, rx = cx+step, rx+int(step) {
			cw := min(int(step), int(strip.W())-rx)
			cell := frame.Rect4(cx, cy, cx+int32(cw), cy+int32(rh))
			b.appendOuterFieldCell(cell, cols[rx:rx+cw], rows[ry:ry+rh], color, opacity)
		}
	}
}

func outerAxisProfile(start int32, n int, lo, hi int32, sigma float32) []float32 {
	out := make([]float32, n)
	if hi <= lo {
		return out
	}
	for i := range out {
		p := float32(start) + float32(i) + 0.5
		c := blurCoverage(p, float32(hi), sigma) - blurCoverage(p, float32(lo), sigma)
		if c < 0 {
			c = 0
		}
		out[i] = c
	}
	return out
}

func (b *Builder) appendOuterFieldCell(cell frame.Rect, cols, rows []float32, color frame.Color, opacity float32) {
	m := image.NewAlpha(image.Rect(0, 0, len(cols), len(rows)))
	inked := false
	for j, ry := range rows {
		row := j * m.Stride
		for i, cx := range cols {
			v := uint8(255*ry*cx + 0.5)
			m.Pix[row+i] = v
			if v > 0 {
				inked = true
			}
		}
	}
	if !inked {
		return
	}
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: cell, Color: color, Mask: m, Opacity: opacity})
}

const insetBlurSamples = 5

const fieldMaskCap = 64

const fieldMaskBudget = 65536

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
	radius := paint.radius.Shrunk(
		pad.Y0-paint.rect.Y0, paint.rect.X1-pad.X1,
		paint.rect.Y1-pad.Y1, pad.X0-paint.rect.X0)
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

func (b *Builder) appendInsetBlurredField(rect frame.Rect, left, right, top, bottom int32, sigma float32, color frame.Color, radius frame.Corners) {
	tl, tr := insetReach(left, rect.W(), sigma), insetReach(right, rect.W(), sigma)
	tt, tb := insetReach(top, rect.H(), sigma), insetReach(bottom, rect.H(), sigma)
	crossing := tl+tr >= rect.W() || tt+tb >= rect.H()
	switch {
	case crossing && int(rect.W())*int(rect.H()) <= fieldMaskBudget:
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

func (b *Builder) appendInsetRamp(band frame.Rect, angle float32, depth, thickness int32, sigma float32, color frame.Color, radius frame.Corners) {
	if band.Empty() || thickness <= 0 {
		return
	}
	stops := make([]frame.GradientStop, insetBlurSamples)
	for i := range stops {
		t := float32(i) / float32(insetBlurSamples-1)
		stops[i] = frame.GradientStop{
			At:    t,
			Color: frame.ScaleColor(color, blurCoverage(float32(thickness)*t, float32(depth), sigma)),
		}
	}
	b.list.Append(DisplayCmd{
		Kind:     CmdGradient,
		Rect:     band,
		Gradient: frame.LinearGradient{Angle: angle, Stops: stops},
		Radius:   radius,
	})
}

func insetFieldCoverage(x, y float32, w, h int32, left, right, top, bottom int32, sigma float32) float32 {
	cx := blurCoverage(x, float32(left), sigma) + blurCoverage(float32(w)-x, float32(right), sigma)
	cy := blurCoverage(y, float32(top), sigma) + blurCoverage(float32(h)-y, float32(bottom), sigma)
	if cx > 1 {
		cx = 1
	}
	if cy > 1 {
		cy = 1
	}
	return 1 - (1-cx)*(1-cy)
}

func (b *Builder) appendInsetMaskedCell(layer, cell frame.Rect, left, right, top, bottom int32, sigma float32, color frame.Color) {
	w, h := int(cell.W()), int(cell.H())
	if w <= 0 || h <= 0 {
		return
	}
	mw, mh := min(w, fieldMaskCap), min(h, fieldMaskCap)
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

func (b *Builder) appendInsetMaskedCells(rect frame.Rect, left, right, top, bottom int32, sigma float32, color frame.Color) {
	step := int32(fieldMaskCap)
	for y := rect.Y0; y < rect.Y1; y += step {
		for x := rect.X0; x < rect.X1; x += step {
			b.appendInsetMaskedCell(rect, frame.Rect4(x, y, min(x+step, rect.X1), min(y+step, rect.Y1)), left, right, top, bottom, sigma, color)
		}
	}
}

func blurCoverage(x, depth, sigma float32) float32 {
	z := float64(depth-x) / (float64(sigma) * math.Sqrt2)
	return float32(0.5 * (1 + math.Erf(z)))
}

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
