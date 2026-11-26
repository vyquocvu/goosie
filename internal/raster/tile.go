package raster

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"

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
		idx  int
	}
	r := c.Rect.Translate(dx, dy).Canon()
	w, h := r.W(), r.H()
	widths := [4]int32{
		clampW(c.Border.Top.Width, h),
		clampW(c.Border.Right.Width, w),
		clampW(c.Border.Bottom.Width, h),
		clampW(c.Border.Left.Width, w),
	}
	if !c.Radius.Empty() {
		drawRoundedBorder(out, c, r, widths)
		return
	}
	sides := [4]side{
		{c.Border.Top, frame.Rect4(r.X0, r.Y0, r.X1, r.Y0+widths[0]), 0},
		{c.Border.Bottom, frame.Rect4(r.X0, r.Y1-widths[2], r.X1, r.Y1), 2},
		{c.Border.Left, frame.Rect4(r.X0, r.Y0, r.X0+widths[3], r.Y1), 3},
		{c.Border.Right, frame.Rect4(r.X1-widths[1], r.Y0, r.X1, r.Y1), 1},
	}
	for _, s := range sides {
		if s.spec.Width <= 0 || s.box.Empty() || s.spec.Color.A() == 0 {
			continue
		}
		if s.spec.Style == paint.BorderStyleRidge || s.spec.Style == paint.BorderStyleGroove {
			outerWidth := (widths[s.idx] + 1) / 2
			outerColor, innerColor := splitBorderColors(s.spec, s.idx, c.Opacity)
			outerBox, innerBox := splitSideRect(s.box, int32(s.idx), outerWidth)
			if !outerBox.Empty() && outerColor.A() != 0 {
				out.FillRect(outerBox, outerColor, nil)
			}
			if !innerBox.Empty() && innerColor.A() != 0 {
				out.FillRect(innerBox, innerColor, nil)
			}
			continue
		}
		colour := borderEdgeColor(s.spec, s.idx, c.Opacity)
		if colour.A() != 0 {
			out.FillRect(s.box, colour, nil)
		}
	}
}

func drawRoundedBorder(out *frame.Bitmap, c *paint.DisplayCmd, r frame.Rect, widths [4]int32) {
	colours := [4]frame.Color{}
	hasSplit := false
	for i, spec := range [4]paint.SideSpec{c.Border.Top, c.Border.Right, c.Border.Bottom, c.Border.Left} {
		colours[i] = borderEdgeColor(spec, i, c.Opacity)
		hasSplit = hasSplit || spec.Style == paint.BorderStyleRidge || spec.Style == paint.BorderStyleGroove
	}
	if !hasSplit {
		out.StrokeRounded(r, c.Radius, widths[0], widths[1], widths[2], widths[3],
			colours[0], colours[1], colours[2], colours[3], nil)
		return
	}
	outerWidths := [4]int32{}
	innerWidths := [4]int32{}
	outerColours := [4]frame.Color{}
	innerColours := [4]frame.Color{}
	hasInner := false
	for i, spec := range [4]paint.SideSpec{c.Border.Top, c.Border.Right, c.Border.Bottom, c.Border.Left} {
		if spec.Style == paint.BorderStyleRidge || spec.Style == paint.BorderStyleGroove {
			outerWidths[i] = (widths[i] + 1) / 2
			innerWidths[i] = widths[i] - outerWidths[i]
			outerColours[i], innerColours[i] = splitBorderColors(spec, i, c.Opacity)
			hasInner = hasInner || innerWidths[i] > 0
		} else {
			outerWidths[i] = widths[i]
			outerColours[i] = colours[i]
		}
	}
	out.StrokeRounded(r, c.Radius, outerWidths[0], outerWidths[1], outerWidths[2], outerWidths[3],
		outerColours[0], outerColours[1], outerColours[2], outerColours[3], nil)
	if !hasInner {
		return
	}
	mid := frame.Rect4(r.X0+outerWidths[3], r.Y0+outerWidths[0], r.X1-outerWidths[1], r.Y1-outerWidths[2])
	midRadius := c.Radius.Shrunk(outerWidths[0], outerWidths[1], outerWidths[2], outerWidths[3])
	if midRadius.Empty() {
		inner := paint.DisplayCmd{
			Kind: paint.CmdBorder,
			Rect: mid,
			Border: paint.BorderSpec{
				Top:    paint.SideSpec{Width: innerWidths[0], Color: innerColours[0]},
				Right:  paint.SideSpec{Width: innerWidths[1], Color: innerColours[1]},
				Bottom: paint.SideSpec{Width: innerWidths[2], Color: innerColours[2]},
				Left:   paint.SideSpec{Width: innerWidths[3], Color: innerColours[3]},
			},
		}
		drawBorder(out, &inner, 0, 0, frame.Rect{})
		return
	}
	out.StrokeRounded(mid, midRadius, innerWidths[0], innerWidths[1], innerWidths[2], innerWidths[3],
		innerColours[0], innerColours[1], innerColours[2], innerColours[3], nil)
}

func splitSideRect(box frame.Rect, side, outerWidth int32) (frame.Rect, frame.Rect) {
	switch side {
	case 0:
		return frame.Rect4(box.X0, box.Y0, box.X1, box.Y0+outerWidth), frame.Rect4(box.X0, box.Y0+outerWidth, box.X1, box.Y1)
	case 2:
		return frame.Rect4(box.X0, box.Y1-outerWidth, box.X1, box.Y1), frame.Rect4(box.X0, box.Y0, box.X1, box.Y1-outerWidth)
	case 3:
		return frame.Rect4(box.X0, box.Y0, box.X0+outerWidth, box.Y1), frame.Rect4(box.X0+outerWidth, box.Y0, box.X1, box.Y1)
	default:
		return frame.Rect4(box.X1-outerWidth, box.Y0, box.X1, box.Y1), frame.Rect4(box.X0, box.Y0, box.X1-outerWidth, box.Y1)
	}
}

func borderEdgeColor(spec paint.SideSpec, side int, opacity float32) frame.Color {
	colour := spec.Color
	switch spec.Style {
	case paint.BorderStyleInset:
		colour = adjustBevelColor(colour, side == 0 || side == 3)
	case paint.BorderStyleOutset:
		colour = adjustBevelColor(colour, side == 1 || side == 2)
	}
	return applyOpacity(colour, opacity)
}

func splitBorderColors(spec paint.SideSpec, side int, opacity float32) (frame.Color, frame.Color) {
	darken := side == 1 || side == 2
	if spec.Style == paint.BorderStyleGroove {
		darken = !darken
	}
	return applyOpacity(adjustBevelColor(spec.Color, darken), opacity),
		applyOpacity(adjustBevelColor(spec.Color, !darken), opacity)
}

func adjustBevelColor(colour frame.Color, darken bool) frame.Color {
	alpha := colour.A()
	if alpha == 0 {
		return colour
	}
	rgb := borderStraightRGB(colour)
	luminance := borderRelativeLuminance(rgb)
	if luminance <= 0.014443844 {
		if darken {
			return borderColor(borderShadeRGB(rgb, false), alpha)
		}
		rgb = borderShadeRGB(rgb, false)
		return borderColor(borderShadeRGB(rgb, false), alpha)
	}
	if darken {
		return borderColor(borderShadeRGB(rgb, true), alpha)
	}
	if luminance > 0.83077 {
		return colour
	}
	return borderColor(borderShadeRGB(rgb, false), alpha)
}

func borderStraightRGB(colour frame.Color) [3]uint8 {
	alpha := uint32(colour.A())
	if alpha == 255 {
		return [3]uint8{colour.R(), colour.G(), colour.B()}
	}
	straight := func(channel uint8) uint8 {
		return uint8((uint32(channel)*255 + alpha/2) / alpha)
	}
	return [3]uint8{straight(colour.R()), straight(colour.G()), straight(colour.B())}
}

func borderColor(rgb [3]uint8, alpha uint8) frame.Color {
	return frame.RGBA(rgb[0], rgb[1], rgb[2], alpha)
}

func borderShadeRGB(rgb [3]uint8, darken bool) [3]uint8 {
	maxChannel := rgb[0]
	if rgb[1] > maxChannel {
		maxChannel = rgb[1]
	}
	if rgb[2] > maxChannel {
		maxChannel = rgb[2]
	}
	if maxChannel == 0 {
		if darken {
			return [3]uint8{}
		}
		return [3]uint8{84, 84, 84}
	}
	value := float32(maxChannel) / 255
	multiplier := float32(0)
	if darken {
		multiplier = max(0, (value-0.33)/value)
	} else {
		multiplier = min(1, value+0.33) / value
	}
	quantize := func(channel uint8) uint8 {
		n := int(float32(channel) * multiplier * 255.9999847 / 255)
		if n < 0 {
			return 0
		}
		if n > 255 {
			return 255
		}
		return uint8(n)
	}
	return [3]uint8{quantize(rgb[0]), quantize(rgb[1]), quantize(rgb[2])}
}

func borderRelativeLuminance(rgb [3]uint8) float32 {
	linear := func(channel uint8) float32 {
		value := float32(channel) / 255
		if value <= 0.04045 {
			return value / 12.92
		}
		return float32(math.Pow(float64((value+0.055)/1.055), 2.4))
	}
	return 0.2126*linear(rgb[0]) + 0.7152*linear(rgb[1]) + 0.0722*linear(rgb[2])
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
