package raster

import (
	"errors"
	"fmt"
	"image"
	"image/draw"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

var ErrNilDestination = errors.New("raster: nil tile destination")

func RasterizeTile(dl *paint.LayerDL, bounds frame.Rect, out *frame.Bitmap, f *Fonts, g *GlyphAtlas, bg frame.Color) error {
	return rasterizeTile(dl, bounds, out, f, g, bg, nil)
}

func rasterizeTile(dl *paint.LayerDL, bounds frame.Rect, out *frame.Bitmap, f *Fonts, g *GlyphAtlas, bg frame.Color, scratch *frame.BitmapPool) error {
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
	for i := 0; i < dl.Len(); i++ {
		if grp, ok := dl.GroupAt(i); ok {
			if err := compositeGroup(dl, grp, b, out, dx, dy, clip, f, g, scratch, nil); err != nil {
				return err
			}
			i = grp.End - 1
			continue
		}
		if !dl.CachedIntersects(i, b) {
			continue
		}
		if err := drawCommand(out, dl, i, dx, dy, clip, f, g, nil); err != nil {
			return err
		}
	}
	return nil
}

func compositeGroup(dl *paint.LayerDL, grp paint.Group, tile frame.Rect, dst *frame.Bitmap, dx, dy int32, clip frame.Rect, f *Fonts, g *GlyphAtlas, pool *frame.BitmapPool, ink *frame.Rect) error {
	if !grp.Bounds.Intersects(tile) {
		return nil
	}
	sc, pooled := acquireScratch(dst, pool)
	if pooled {
		defer pool.Release(sc)
	}
	sc.Reset()
	scClip := sc.Bounds()
	var wrote frame.Rect
	for i := grp.Start; i < grp.End; i++ {
		if i > grp.Start {
			if child, ok := dl.GroupAt(i); ok {
				if err := compositeGroup(dl, child, tile, sc, dx, dy, scClip, f, g, pool, &wrote); err != nil {
					return err
				}
				i = child.End - 1
				continue
			}
		}
		if !dl.CachedIntersects(i, tile) {
			continue
		}
		if err := drawCommand(sc, dl, i, dx, dy, scClip, f, g, &wrote); err != nil {
			return err
		}
	}
	if wrote.Empty() {
		return nil
	}
	r := wrote.Intersection(scClip)
	if r.Empty() {
		return nil
	}
	fadeRect(sc, grp.OwnOpacity, r)
	dst.BlitOver(sc, frame.Point{X: r.X0, Y: r.Y0}, r, clip, nil)
	if ink != nil {
		*ink = ink.Union(r)
	}
	return nil
}

func acquireScratch(dst *frame.Bitmap, pool *frame.BitmapPool) (*frame.Bitmap, bool) {
	if pool != nil && pool.Size() == dst.Size() {
		return pool.Acquire(), true
	}
	return frame.NewBitmap(dst.W, dst.H), false
}

type fadeScale [256]byte

func newFadeScale(o float32) fadeScale {
	var t fadeScale
	for i := range t {
		t[i] = byte(float32(i) * o)
	}
	return t
}

func fadeRect(b *frame.Bitmap, o float32, r frame.Rect) {
	if o >= 1 {
		return
	}
	if o <= 0 {
		b.FillRect(r, frame.TransparentBlack, nil)
		return
	}
	t := newFadeScale(o)
	x0, x1 := int(r.X0), int(r.X1)
	for y := int(r.Y0); y < int(r.Y1); y++ {
		base := y * b.Stride
		row := b.RGBA[base+x0*4 : base+x1*4]
		for i, v := range row {
			row[i] = t[v]
		}
	}
}

func drawCommand(dst *frame.Bitmap, dl *paint.LayerDL, i int, dx, dy int32, tileClip frame.Rect, f *Fonts, g *GlyphAtlas, ink *frame.Rect) error {
	c := dl.At(i)
	cmdClip := tileClip
	if !c.Clip.Empty() {
		cmdClip = tileClip.Intersection(c.Clip.Translate(dx, dy))
		if cmdClip.Empty() {
			return nil
		}
	}
	if ink != nil && c.Kind != paint.CmdText {
		*ink = ink.Union(c.Rect.Translate(dx, dy).Canon())
	}
	switch c.Kind {
	case paint.CmdFill:
		if c.Mask != nil {
			drawMaskedFill(dst, c.Rect.Translate(dx, dy), applyOpacity(c.Color, c.Opacity), c.Mask, cmdClip)
		} else if c.Radius.Empty() {
			dst.FillRect(c.Rect.Translate(dx, dy), applyOpacity(c.Color, c.Opacity), nil)
		} else {
			dst.FillRounded(c.Rect.Translate(dx, dy), c.Radius, applyOpacity(c.Color, c.Opacity), nil)
		}
	case paint.CmdBorder:
		drawBorder(dst, &c, dx, dy, cmdClip)
	case paint.CmdText:
		return drawText(dst, &c, dx, dy, cmdClip, f, g, ink)
	case paint.CmdImage:
		drawImage(dst, &c, dx, dy, cmdClip)
	case paint.CmdGradient:
		dst.FillLinearGradient(c.Rect.Translate(dx, dy), c.Radius, c.Gradient, nil)
	case paint.CmdRadialGradient:
		dst.FillRadialGradient(c.Rect.Translate(dx, dy), c.Radius, c.RadialGradient, nil)
	}
	return nil
}

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

func drawBorder(out *frame.Bitmap, c *paint.DisplayCmd, dx, dy int32, clip frame.Rect) {
	type side struct {
		spec paint.SideSpec
		box  frame.Rect
	}
	r := c.Rect.Translate(dx, dy).Canon()
	w, h := r.W(), r.H()
	if !c.Radius.Empty() {
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

func clampW(width, extent int32) int32 {
	if width > extent {
		return extent
	}
	return width
}

func drawText(out *frame.Bitmap, c *paint.DisplayCmd, dx, dy int32, clip frame.Rect, f *Fonts, g *GlyphAtlas, ink *frame.Rect) error {
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
		pen := frame.Point{X: gl.X + dx, Y: gl.Y + dy}
		origin := pen.Translate(glyph.Bounds.X0, glyph.Bounds.Y0)
		if ink != nil {
			mr := glyph.Mask.Bounds()
			*ink = ink.Union(frame.RectAt(origin, frame.Size{W: int32(mr.Dx()), H: int32(mr.Dy())}))
		}
		out.BlitMask(glyph.Mask, origin, col, clip, nil)
	}
	return nil
}

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

func rectOf(r image.Rectangle) frame.Rect {
	return frame.Rect4(int32(r.Min.X), int32(r.Min.Y), int32(r.Max.X), int32(r.Max.Y))
}

func opacityCoverage(opacity float32) uint8 {
	if opacity == 0 || opacity >= 1 {
		return 255
	}
	if opacity < 0 {
		return 0
	}
	return uint8(float32(opacity) * 255)
}
