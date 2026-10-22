package raster

import (
	"errors"
	"fmt"
	"image"
	"image/draw"

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
// The list is read through its frozen bounds sidecar, so a tile over
// whitespace walks one cheap pass instead of re-walking glyphs per command.
// Nothing here takes a lock: the display list is frozen, and the glyph caches
// synchronize themselves.
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
	// One index pass with frozen bounds: Intersecting would scan the list for
	// the span and then re-test every command inside it (a glyph walk per text
	// command per tile). CachedIntersects answers the same predicate in O(1),
	// so a single pass both selects and filters with no second walk.
	for i := 0; i < dl.Len(); i++ {
		if !dl.CachedIntersects(i, b) {
			continue
		}
		c := dl.At(i)
		// A command's own clip narrows the tile clip; the tile clip alone is what
		// every other command draws against.
		cmdClip := clip
		if !c.Clip.Empty() {
			cmdClip = clip.Intersection(c.Clip.Translate(dx, dy))
			if cmdClip.Empty() {
				continue
			}
		}
		switch c.Kind {
		case paint.CmdFill:
			if c.Mask != nil {
				drawMaskedFill(out, c.Rect.Translate(dx, dy), applyOpacity(c.Color, c.Opacity), c.Mask, cmdClip)
			} else if c.Radius.Empty() {
				out.FillRect(c.Rect.Translate(dx, dy), applyOpacity(c.Color, c.Opacity), nil)
			} else {
				out.FillRounded(c.Rect.Translate(dx, dy), c.Radius, applyOpacity(c.Color, c.Opacity), nil)
			}
		case paint.CmdBorder:
			drawBorder(out, &c, dx, dy, cmdClip)
		case paint.CmdText:
			if err := drawText(out, &c, dx, dy, cmdClip, f, g); err != nil {
				return err
			}
		case paint.CmdImage:
			drawImage(out, &c, dx, dy, cmdClip)
		case paint.CmdGradient:
			out.FillLinearGradient(c.Rect.Translate(dx, dy), c.Radius, c.Gradient, nil)
		case paint.CmdRadialGradient:
			out.FillRadialGradient(c.Rect.Translate(dx, dy), c.Radius, c.RadialGradient, nil)
		}
	}
	return nil
}

// drawMaskedFill paints color over dst, multiplied by a mask stretched over
// the fill rect. The mask resolves per the match-source rule: images with
// real translucency gate on alpha, alpha-less images on luminance. Rounded
// corners do not apply under a mask (icons, the motivating case, are square);
// mask-size/position/repeat are deferred, so the single layer always covers.
func drawMaskedFill(out *frame.Bitmap, dst frame.Rect, color frame.Color, mask image.Image, clip frame.Rect) {
	mb := mask.Bounds()
	mw, mh := mb.Dx(), mb.Dy()
	if mw <= 0 || mh <= 0 || dst.Empty() {
		return
	}
	rgba, ok := mask.(*image.RGBA)
	if !ok {
		tmp := image.NewRGBA(image.Rect(0, 0, mw, mh))
		draw.Draw(tmp, tmp.Bounds(), mask, mb.Min, draw.Src)
		rgba = tmp
	}
	useAlpha := false
	for i, n := 0, len(rgba.Pix)/4; i < n && i < 2048; i++ {
		if rgba.Pix[i*4+3] < 250 {
			useAlpha = true
			break
		}
	}
	cl := dst.Intersection(clip).Intersection(out.Bounds())
	if cl.Empty() {
		return
	}
	dw, dh := dst.W(), dst.H()
	if dw <= 0 || dh <= 0 {
		return
	}
	cr, cg, cb, ca := uint32(color.R()), uint32(color.G()), uint32(color.B()), uint32(color.A())
	for y := int(cl.Y0); y < int(cl.Y1); y++ {
		my := (y - int(dst.Y0)) * mh / int(dh)
		if my < 0 {
			my = 0
		} else if my >= mh {
			my = mh - 1
		}
		rowBase := y*out.Stride + int(cl.X0)*4
		row := out.RGBA[rowBase : rowBase+int(cl.X1-cl.X0)*4]
		o := 0
		for x := int(cl.X0); x < int(cl.X1); x++ {
			mx := (x - int(dst.X0)) * mw / int(dw)
			if mx < 0 {
				mx = 0
			} else if mx >= mw {
				mx = mw - 1
			}
			off := (my+mb.Min.Y)*rgba.Stride + (mx+mb.Min.X)*4
			var ma uint32
			if useAlpha {
				ma = uint32(rgba.Pix[off+3])
			} else {
				ma = (uint32(rgba.Pix[off])*2126 + uint32(rgba.Pix[off+1])*7152 + uint32(rgba.Pix[off+2])*722) / 10000
			}
			if ma == 0 {
				o += 4
				continue
			}
			p := row[o : o+4 : o+4]
			dstc := frame.Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
			src := frame.Color(cr*ma/255<<24 | cg*ma/255<<16 | cb*ma/255<<8 | ca*ma/255)
			outc := frame.BlendOver(dstc, src)
			p[0] = byte(outc >> 24)
			p[1] = byte(outc >> 16)
			p[2] = byte(outc >> 8)
			p[3] = byte(outc)
			o += 4
		}
	}
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
