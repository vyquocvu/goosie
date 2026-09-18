package raster

import (
	"errors"
	"fmt"
	"image"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// ErrNilDestination is returned when there is nowhere to put the pixels. It is
// an error rather than a panic because RasterizeTile runs on a worker, and a
// panic there would take the pool down with it (pool.go has the guard that
// catches the ones that still happen).
var ErrNilDestination = errors.New("raster: nil tile destination")

// RasterizeTile paints the tile covering bounds into out.
//
// bounds is in layer content space; out is a TileSize-square buffer whose own
// coordinate space starts at zero. Translating once at the top is what makes the
// result position-independent: the same tile buffer is valid no matter where the
// viewport happens to be, which is the property that lets a scrolled-back tile
// be reused instead of re-rendered.
//
// The list is read through Intersecting, so a tile over whitespace walks one
// contiguous span instead of the whole page. Nothing here takes a lock: the
// display list is frozen, and the glyph caches synchronize themselves.
//
// g is the glyph cache and is what a frame path passes. It may be nil, in which
// case glyphs come straight from f unshared - a slower rendering that exists so a
// one-off tool can rasterize a tile without building an atlas.
func RasterizeTile(dl *paint.LayerDL, bounds frame.Rect, out *frame.Bitmap, f *Fonts, g *GlyphAtlas, bg frame.Color) error {
	if out == nil {
		return ErrNilDestination
	}
	if out.Empty() {
		return fmt.Errorf("raster: tile buffer has no pixels for %v", bounds)
	}
	if bg.Opaque() {
		out.FillRect(out.Bounds(), bg, nil)
	} else {
		out.Reset()
	}
	if dl == nil || dl.Len() == 0 {
		return nil
	}
	b := bounds.Canon()
	dx, dy := -b.X0, -b.Y0
	clip := out.Bounds()
	for _, c := range dl.Intersecting(b) {
		if !c.Intersects(b) {
			continue
		}
		switch c.Kind {
		case paint.CmdFill:
			if c.Radius.Empty() {
				out.FillRect(c.Rect.Translate(dx, dy), applyOpacity(c.Color, c.Opacity), nil)
			} else {
				out.FillRounded(c.Rect.Translate(dx, dy), c.Radius, applyOpacity(c.Color, c.Opacity), nil)
			}
		case paint.CmdBorder:
			drawBorder(out, &c, dx, dy, clip)
		case paint.CmdText:
			if err := drawText(out, &c, dx, dy, clip, f, g); err != nil {
				return err
			}
		case paint.CmdImage:
			drawImage(out, &c, dx, dy, clip)
		case paint.CmdGradient:
			out.FillLinearGradient(c.Rect.Translate(dx, dy), c.Radius, c.Gradient, nil)
		}
	}
	return nil
}

// applyOpacity folds a command's opacity into its premultiplied color by scaling
// all four channels together, which is the only operation that keeps a
// premultiplied color valid: un-premultiplying, scaling alpha, and re-multiplying
// would round differently at every opacity step.
//
// Zero means unset rather than invisible, so a producer that emits only opaque
// commands never touches the field. That is consistent with the rest of
// DisplayCmd, where visibility is decided by Color and Rect: an element that is
// genuinely transparent is dropped by the style pass before it becomes a command.
func applyOpacity(c frame.Color, opacity float32) frame.Color {
	if opacity == 0 || opacity >= 1 {
		return c
	}
	if opacity < 0 {
		return frame.TransparentBlack
	}
	scale := func(v uint8) uint32 { return uint32(float32(v) * opacity) }
	return frame.Color(scale(c.R())<<24 | scale(c.G())<<16 | scale(c.B())<<8 | scale(c.A()))
}

// drawBorder paints up to four edge strips. Each strip is a fill, so a border
// costs at most four clipped rectangles and no extra geometry.
//
// The strips go into a fixed array rather than a slice literal: this runs once
// per bordered box per tile, and a heap slice here would put an allocation inside
// the steady-state frame path.
func drawBorder(out *frame.Bitmap, c *paint.DisplayCmd, dx, dy int32, clip frame.Rect) {
	type side struct {
		spec paint.SideSpec
		box  frame.Rect
	}
	r := c.Rect.Translate(dx, dy).Canon()
	w, h := r.W(), r.H()
	if !c.Radius.Empty() {
		// A rounded box's ring is one shape, not four strips: the corner bands
		// belong to the horizontal edges, so the arc stays continuous.
		out.StrokeRounded(r, c.Radius,
			clampW(c.Border.Top.Width, h), clampW(c.Border.Right.Width, w),
			clampW(c.Border.Bottom.Width, h), clampW(c.Border.Left.Width, w),
			applyOpacity(c.Border.Top.Color, c.Opacity),
			applyOpacity(c.Border.Right.Color, c.Opacity),
			applyOpacity(c.Border.Bottom.Color, c.Opacity),
			applyOpacity(c.Border.Left.Color, c.Opacity),
			nil)
		return
	}
	var sides [4]side
	sides[0] = side{c.Border.Top, frame.Rect4(r.X0, r.Y0, r.X1, r.Y0+clampW(c.Border.Top.Width, h))}
	sides[1] = side{c.Border.Bottom, frame.Rect4(r.X0, r.Y1-clampW(c.Border.Bottom.Width, h), r.X1, r.Y1)}
	sides[2] = side{c.Border.Left, frame.Rect4(r.X0, r.Y0, r.X0+clampW(c.Border.Left.Width, w), r.Y1)}
	sides[3] = side{c.Border.Right, frame.Rect4(r.X1-clampW(c.Border.Right.Width, w), r.Y0, r.X1, r.Y1)}
	for _, s := range sides {
		if s.spec.Width <= 0 || s.box.Empty() || s.spec.Color.A() == 0 {
			continue
		}
		out.FillRect(s.box, applyOpacity(s.spec.Color, c.Opacity), nil)
	}
}

// clampW stops a border whose two opposite edges together exceed the box from
// wrapping past the middle and painting the far edge.
func clampW(width, extent int32) int32 {
	if width > extent {
		return extent
	}
	return width
}

func drawText(out *frame.Bitmap, c *paint.DisplayCmd, dx, dy int32, clip frame.Rect, f *Fonts, g *GlyphAtlas) error {
	if g == nil && f == nil {
		return errors.New("raster: text command with neither a glyph atlas nor a font")
	}
	col := applyOpacity(c.Text.Color, c.Opacity)
	if col.A() == 0 {
		return nil
	}
	for _, gl := range c.Text.Glyphs {
		if gl.Size <= 0 {
			continue
		}
		var glyph Glyph
		if g != nil {
			glyph = g.Get(gl.Rune, gl.Size, gl.Slot)
		} else {
			glyph = f.Glyph(gl.Size, gl.Rune, gl.Slot)
		}
		if !glyph.Ok || glyph.Mask == nil || glyph.Mask.Bounds().Empty() {
			continue
		}
		// The pen sits on the baseline; the glyph's own bounds say where its ink
		// falls relative to the pen.
		pen := frame.Point{X: gl.X + dx, Y: gl.Y + dy}
		out.BlitMask(glyph.Mask, pen.Translate(glyph.Bounds.X0, glyph.Bounds.Y0), col, clip, nil)
	}
	return nil
}

// drawImage scales a decoded image into the command's box. A missing source is
// not an error: an image that has not decoded yet is a normal state for a page,
// and the affected tiles are re-rasterized when it arrives.
func drawImage(out *frame.Bitmap, c *paint.DisplayCmd, dx, dy int32, clip frame.Rect) {
	src := c.Image.Src
	if src == nil {
		return
	}
	box := src.Bounds()
	if !c.Image.SrcBox.Empty() {
		box = c.Image.SrcBox
	}
	if box.Empty() {
		return
	}
	dst := c.Rect.Translate(dx, dy)
	out.ScaleOver(src, rectOf(box), dst, clip, opacityCoverage(c.Opacity), nil)
}

// rectOf re-exports an image.Rectangle as the content-space rect the frame
// primitives speak in. The two types are deliberately not merged: one is a
// standard-library image's own coordinate space and the other is a layer's.
func rectOf(r image.Rectangle) frame.Rect {
	return frame.Rect4(int32(r.Min.X), int32(r.Min.Y), int32(r.Max.X), int32(r.Max.Y))
}

// opacityCoverage turns a command's opacity into the 8-bit coverage the scaler
// multiplies each sampled pixel by, with the same unset-is-opaque convention as
// applyOpacity. 255 lets the scaler skip the multiplication for the common case.
func opacityCoverage(opacity float32) uint8 {
	if opacity == 0 || opacity >= 1 {
		return 255
	}
	if opacity < 0 {
		return 0
	}
	return uint8(float32(opacity) * 255)
}
