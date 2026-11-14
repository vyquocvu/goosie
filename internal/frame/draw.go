package frame

import "image"

func ScaleAlpha(c Color, cov uint8) Color {
	if cov == 255 {
		return c
	}
	if cov == 0 {
		return TransparentBlack
	}
	ca := uint32(cov)
	r := uint32(c.R()) * ca / 255
	g := uint32(c.G()) * ca / 255
	b := uint32(c.B()) * ca / 255
	a := uint32(c.A()) * ca / 255
	return Color(r<<24 | g<<16 | b<<8 | a)
}

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
	mx0 := int(dr.X0) - int(origin.X) + mr.Min.X
	my0 := int(dr.Y0) - int(origin.Y) + mr.Min.Y
	opaque := c.Opaque()
	for y, my := int(dr.Y0), my0; y < int(dr.Y1); y, my = y+1, my+1 {
		row := my*mask.Stride + mx0
		drowBase := y*b.Stride + int(dr.X0)*4
		drow := b.RGBA[drowBase : drowBase+int(dr.X1-dr.X0)*4]
		for x := int(dr.X0); x < int(dr.X1); x, row = x+1, row+1 {
			cov := mask.Pix[row]
			if cov == 0 {
				continue
			}
			ox := (x - int(dr.X0)) * 4
			p := drow[ox : ox+4 : ox+4]
			if cov == 255 && opaque {
				p[0] = byte(c >> 24)
				p[1] = byte(c >> 16)
				p[2] = byte(c >> 8)
				p[3] = byte(c)
				continue
			}
			dst := Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
			var out Color
			if opaque {
				out = BlendOverCoverage(dst, c, cov)
			} else {
				out = BlendOver(dst, ScaleAlpha(c, cov))
			}
			p[0] = byte(out >> 24)
			p[1] = byte(out >> 16)
			p[2] = byte(out >> 8)
			p[3] = byte(out)
		}
	}
}

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
	full := dst.Canon()
	fw, fh := int32(full.W()), int32(full.H())
	sxStep := int32(int64(sw) << 16 / int64(fw))
	syStep := int32(int64(sh) << 16 / int64(fh))
	offX, offY := dr.X0-full.X0, dr.Y0-full.Y0
	sb := src.Bounds()
	xLo, xHi := int32(sb.Min.X), int32(sb.Max.X)-1
	yLo, yHi := int32(sb.Min.Y), int32(sb.Max.Y)-1
	var (
		pix    []byte
		stride int
		direct bool
		base   int
	)
	if rgba, ok := src.(*image.RGBA); ok {
		pix, stride, direct = rgba.Pix, rgba.Stride, true
		base = -int(sb.Min.Y)*stride - int(sb.Min.X)*4
	}
	if cov == 255 {
		for dy := int32(0); dy < dr.H(); dy++ {
			fy := (dy+offY)*syStep + syStep/2
			sy0 := clamp32(srcRect.Y0+(fy>>16), yLo, yHi)
			sy1 := clamp32(srcRect.Y0+(fy>>16)+1, yLo, yHi)
			wy := uint32((fy & 0xFFFF) >> 10)
			for dx := int32(0); dx < dr.W(); dx++ {
				fx := (dx+offX)*sxStep + sxStep/2
				sx0 := clamp32(srcRect.X0+(fx>>16), xLo, xHi)
				sx1 := clamp32(srcRect.X0+(fx>>16)+1, xLo, xHi)
				wx := uint32((fx & 0xFFFF) >> 10)
				c := blendBilinear(
					sample(src, pix, stride, direct, base, sx0, sy0), sample(src, pix, stride, direct, base, sx1, sy0),
					sample(src, pix, stride, direct, base, sx0, sy1), sample(src, pix, stride, direct, base, sx1, sy1), wx, wy)
				o := int(dy+dr.Y0)*b.Stride + int(dx+dr.X0)*4
				p := b.RGBA[o : o+4 : o+4]
				dstC := Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
				out := BlendOver(dstC, c)
				p[0] = byte(out >> 24)
				p[1] = byte(out >> 16)
				p[2] = byte(out >> 8)
				p[3] = byte(out)
			}
		}
		return
	}
	for dy := int32(0); dy < dr.H(); dy++ {
		fy := (dy+offY)*syStep + syStep/2
		sy0 := clamp32(srcRect.Y0+(fy>>16), yLo, yHi)
		sy1 := clamp32(srcRect.Y0+(fy>>16)+1, yLo, yHi)
		wy := uint32((fy & 0xFFFF) >> 10)
		for dx := int32(0); dx < dr.W(); dx++ {
			fx := (dx+offX)*sxStep + sxStep/2
			sx0 := clamp32(srcRect.X0+(fx>>16), xLo, xHi)
			sx1 := clamp32(srcRect.X0+(fx>>16)+1, xLo, xHi)
			wx := uint32((fx & 0xFFFF) >> 10)
			c := blendBilinear(
				sample(src, pix, stride, direct, base, sx0, sy0), sample(src, pix, stride, direct, base, sx1, sy0),
				sample(src, pix, stride, direct, base, sx0, sy1), sample(src, pix, stride, direct, base, sx1, sy1), wx, wy)
			c = ScaleAlpha(c, cov)
			o := int(dy+dr.Y0)*b.Stride + int(dx+dr.X0)*4
			p := b.RGBA[o : o+4 : o+4]
			dstC := Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
			out := BlendOver(dstC, c)
			p[0] = byte(out >> 24)
			p[1] = byte(out >> 16)
			p[2] = byte(out >> 8)
			p[3] = byte(out)
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

func sample(src image.Image, pix []byte, stride int, direct bool, base int, x, y int32) Color {
	if direct {
		o := int(y)*stride + int(x)*4 + base
		return Color(uint32(pix[o])<<24 | uint32(pix[o+1])<<16 | uint32(pix[o+2])<<8 | uint32(pix[o+3]))
	}
	r, g, bb, a := src.At(int(x), int(y)).RGBA()
	return Color(r>>8<<24 | g>>8<<16 | bb>>8<<8 | a>>8)
}

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
