// Coverage-blending primitives. The pixel paths a display-list command needs
// live here rather than in raster so that the tile buffer's coordinate and
// stride rules have exactly one owner, and so rasterizer behaviour can be tested
// as a property of the buffer it writes.

package frame

import "image"

// ScaleAlpha returns c with every channel, alpha included, multiplied by
// cov/255. Because c is premultiplied, scaling all four terms together is the
// correct way to reduce a color's coverage: scaling alpha alone would leave the
// color channels promising more ink than the pixel can carry.
func ScaleAlpha(c Color, cov uint8) Color {
	if cov == 255 {
		return c
	}
	if cov == 0 {
		return TransparentBlack
	}
	a := uint32(cov)
	r := uint32(c.R()) * a / 255
	g := uint32(c.G()) * a / 255
	b := uint32(c.B()) * a / 255
	return Color(r<<24 | g<<16 | b<<8 | a)
}

// BlitMask blends c over b wherever mask carries coverage.
//
// origin is where the mask's own top-left pixel lands, and clip bounds the
// write. This is the primitive behind antialiased text: a glyph is one
// single-channel coverage map, and drawing it is scaling a color by that
// coverage and blending.
//
// It deliberately does not go through image/draw. DrawMask is the right general
// tool, but it boxes a writer per call, and text is the one command present on
// nearly every tile of a real page - invariant 6 leaves no room for an
// allocation per glyph.
func (b *Bitmap) BlitMask(mask *image.Alpha, origin Point, c Color, clip Rect, writes *int64) {
	if mask == nil || c.A() == 0 {
		return
	}
	mr := mask.Bounds()
	if mr.Empty() {
		return
	}
	dr := RectAt(origin, Size{W: int32(mr.Dx()), H: int32(mr.Dy())}).
		Intersection(clip).Intersection(b.Bounds())
	if dr.Empty() {
		return
	}
	if writes != nil {
		*writes += int64(dr.W()) * int64(dr.H())
	}
	// The first source pixel of the destination row, in mask coordinates. Moving
	// one destination pixel right advances one mask pixel, so both loops are
	// straight index arithmetic.
	mx0 := int(dr.X0) - int(origin.X) + mr.Min.X
	my0 := int(dr.Y0) - int(origin.Y) + mr.Min.Y
	for y, my := int(dr.Y0), my0; y < int(dr.Y1); y, my = y+1, my+1 {
		row := my*mask.Stride + mx0
		do := y * b.Stride
		for x := int(dr.X0); x < int(dr.X1); x, row = x+1, row+1 {
			cov := mask.Pix[row]
			if cov == 0 {
				continue
			}
			o := do + x*4
			dst := Color(uint32(b.RGBA[o])<<24 | uint32(b.RGBA[o+1])<<16 | uint32(b.RGBA[o+2])<<8 | uint32(b.RGBA[o+3]))
			out := BlendOver(dst, ScaleAlpha(c, cov))
			b.RGBA[o] = byte(out >> 24)
			b.RGBA[o+1] = byte(out >> 16)
			b.RGBA[o+2] = byte(out >> 8)
			b.RGBA[o+3] = byte(out)
		}
	}
}

// ScaleOver draws srcRect of src into dst, resampled, clipped to clip. cov
// reduces the whole image's coverage, which is how a command's opacity reaches
// the scaler without a second pass over the pixels: 255 is opaque.
//
// Resampling is bilinear with integer weights rather than nearest-neighbour,
// because scaled images are exactly where nearest is visibly wrong: a bitmap
// icon at DPR 1.5 turns to blocks. Everything below is integer arithmetic, so
// the result is identical across platforms, runs, and goroutine interleavings,
// which is what lets a rasterized tile be compared byte for byte.
func (b *Bitmap) ScaleOver(src image.Image, srcRect, dst, clip Rect, cov uint8, writes *int64) {
	if src == nil || cov == 0 {
		return
	}
	dr := dst.Canon().Intersection(clip).Intersection(b.Bounds())
	sw, sh := srcRect.W(), srcRect.H()
	if dr.Empty() || sw <= 0 || sh <= 0 || dst.Canon().W() <= 0 || dst.Canon().H() <= 0 {
		return
	}
	if writes != nil {
		*writes += int64(dr.W()) * int64(dr.H())
	}
	// The fixed-point step is source units per destination pixel measured across
	// the WHOLE destination box, and each tile's pixels are offset by where the
	// clipped rect sits inside that box. Deriving the step from the clipped rect
	// instead would rescale the entire source into every tile, so a background
	// wider than one tile would repeat in each tile it spans rather than show its
	// matching slice.
	full := dst.Canon()
	fw, fh := int32(full.W()), int32(full.H())
	sxStep := int32(int64(sw) << 16 / int64(fw))
	syStep := int32(int64(sh) << 16 / int64(fh))
	offX, offY := dr.X0-full.X0, dr.Y0-full.Y0
	sb := src.Bounds()
	xLo, xHi := int32(sb.Min.X), int32(sb.Max.X)-1
	yLo, yHi := int32(sb.Min.Y), int32(sb.Max.Y)-1
	// *image.RGBA is read through its pixels rather than through At: At boxes a
	// color.Color interface per call, which is one allocation per destination
	// pixel and rules out image commands in a zero-allocation frame. Decoded
	// images are RGBA by the time they reach a display list, so the fast path is
	// the only path in practice; the general one stays correct.
	var (
		pix    []byte
		stride int
		direct bool
		base   int
	)
	if rgba, ok := src.(*image.RGBA); ok {
		pix, stride, direct = rgba.Pix, rgba.Stride, true
		// An image.RGBA indexes its buffer relative to its own Rect.Min, which is
		// not the origin for a sub-image. Folding the offset into one constant keeps
		// the per-pixel index a single multiply and add.
		base = -int(sb.Min.Y)*stride - int(sb.Min.X)*4
	}
	for dy := int32(0); dy < dr.H(); dy++ {
		fy := (dy+offY)*syStep + syStep/2
		sy0 := clamp32(srcRect.Y0+(fy>>16), yLo, yHi)
		sy1 := clamp32(srcRect.Y0+(fy>>16)+1, yLo, yHi)
		wy := uint32((fy & 0xFFFF) >> 10) // 0..63, the weight of the lower row
		for dx := int32(0); dx < dr.W(); dx++ {
			fx := (dx+offX)*sxStep + sxStep/2
			sx0 := clamp32(srcRect.X0+(fx>>16), xLo, xHi)
			sx1 := clamp32(srcRect.X0+(fx>>16)+1, xLo, xHi)
			wx := uint32((fx & 0xFFFF) >> 10)
			c := blendBilinear(
				sample(src, pix, stride, direct, base, sx0, sy0), sample(src, pix, stride, direct, base, sx1, sy0),
				sample(src, pix, stride, direct, base, sx0, sy1), sample(src, pix, stride, direct, base, sx1, sy1), wx, wy)
			if cov != 255 {
				c = ScaleAlpha(c, cov)
			}
			o := int(dy+dr.Y0)*b.Stride + int(dx+dr.X0)*4
			dstC := Color(uint32(b.RGBA[o])<<24 | uint32(b.RGBA[o+1])<<16 | uint32(b.RGBA[o+2])<<8 | uint32(b.RGBA[o+3]))
			out := BlendOver(dstC, c)
			b.RGBA[o] = byte(out >> 24)
			b.RGBA[o+1] = byte(out >> 16)
			b.RGBA[o+2] = byte(out >> 8)
			b.RGBA[o+3] = byte(out)
		}
	}
}

func clamp32(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// sample reads one pixel as a premultiplied 8-bit Color. Through the interface
// it uses only RGBA(), so any image.Image whose At is cheap works; the RGBA
// fast path reads the buffer directly and allocates nothing.
func sample(src image.Image, pix []byte, stride int, direct bool, base int, x, y int32) Color {
	if direct {
		o := int(y)*stride + int(x)*4 + base
		return Color(uint32(pix[o])<<24 | uint32(pix[o+1])<<16 | uint32(pix[o+2])<<8 | uint32(pix[o+3]))
	}
	r, g, bb, a := src.At(int(x), int(y)).RGBA()
	return Color(r>>8<<24 | g>>8<<16 | bb>>8<<8 | a>>8)
}

// blendBilinear interpolates four premultiplied corners. wx and wy are weights
// in 0..63 for the second index of each axis, so the four weights sum to 4096.
//
// The four channels are written out rather than looped over: the loop form needs
// a closure or a shift table per pixel, and this is the innermost loop of image
// rasterization.
func blendBilinear(nw, ne, sw, se Color, wx, wy uint32) Color {
	w0 := (64 - wx) * (64 - wy)
	w1 := wx * (64 - wy)
	w2 := (64 - wx) * wy
	w3 := wx * wy
	nwf, nef, swf, sef := uint32(nw), uint32(ne), uint32(sw), uint32(se)
	r := (nwf>>24&0xFF*w0 + nef>>24&0xFF*w1 + swf>>24&0xFF*w2 + sef>>24&0xFF*w3) / 4096
	g := (nwf>>16&0xFF*w0 + nef>>16&0xFF*w1 + swf>>16&0xFF*w2 + sef>>16&0xFF*w3) / 4096
	b := (nwf>>8&0xFF*w0 + nef>>8&0xFF*w1 + swf>>8&0xFF*w2 + sef>>8&0xFF*w3) / 4096
	a := (nwf&0xFF*w0 + nef&0xFF*w1 + swf&0xFF*w2 + sef&0xFF*w3) / 4096
	return Color(r<<24 | g<<16 | b<<8 | a)
}
