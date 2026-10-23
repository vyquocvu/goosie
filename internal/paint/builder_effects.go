package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
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
