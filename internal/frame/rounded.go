package frame

import "math"

type Radius struct{ X, Y float32 }

type Corners struct{ TL, TR, BR, BL Radius }

func (c Corners) Empty() bool { return c == Corners{} }

func (c Corners) Scaled(scale float32) Corners {
	mul := func(r Radius) Radius { return Radius{X: r.X * scale, Y: r.Y * scale} }
	return Corners{TL: mul(c.TL), TR: mul(c.TR), BR: mul(c.BR), BL: mul(c.BL)}
}

func (c Corners) Shrunk(top, right, bottom, left int32) Corners {
	sub := func(r Radius, w, h int32) Radius {
		out := Radius{}
		if v := r.X - float32(w); v > 0 {
			out.X = v
		}
		if v := r.Y - float32(h); v > 0 {
			out.Y = v
		}
		return out
	}
	return Corners{
		TL: sub(c.TL, left, top),
		TR: sub(c.TR, right, top),
		BR: sub(c.BR, right, bottom),
		BL: sub(c.BL, left, bottom),
	}
}

func (c Corners) Contains(x0, y0, x1, y1, px, py float32) bool {
	if px < x0 || px >= x1 || py < y0 || py >= y1 {
		return false
	}
	if c.Empty() {
		return true
	}
	in := func(r Radius, cx, cy float32) bool {
		if r.X <= 0 || r.Y <= 0 {
			return true
		}
		dx, dy := px-cx, py-cy
		return (dx*dx)/(r.X*r.X)+(dy*dy)/(r.Y*r.Y) <= 1
	}
	switch {
	case px < x0+c.TL.X && py < y0+c.TL.Y:
		return in(c.TL, x0+c.TL.X, y0+c.TL.Y)
	case px >= x1-c.TR.X && py < y0+c.TR.Y:
		return in(c.TR, x1-c.TR.X, y0+c.TR.Y)
	case px >= x1-c.BR.X && py >= y1-c.BR.Y:
		return in(c.BR, x1-c.BR.X, y1-c.BR.Y)
	case px < x0+c.BL.X && py >= y1-c.BL.Y:
		return in(c.BL, x0+c.BL.X, y1-c.BL.Y)
	}
	return true
}

func (b *Bitmap) FillRounded(r Rect, rad Corners, c Color, writes *int64) {
	if rad.Empty() {
		b.FillRect(r, c, writes)
		return
	}
	cl := r.Intersection(b.Bounds())
	if cl.Empty() {
		return
	}
	if writes != nil {
		*writes += int64(cl.W()) * int64(cl.H())
	}
	for y := int(cl.Y0); y < int(cl.Y1); y++ {
		x0, x1 := rad.spanAt(r, y)
		if x0 < float32(cl.X0) {
			x0 = float32(cl.X0)
		}
		if x1 > float32(cl.X1) {
			x1 = float32(cl.X1)
		}
		b.fillRowCover(y, x0, x1, c)
	}
}

func (b *Bitmap) StrokeRounded(r Rect, rad Corners, top, right, bottom, left int32, ct, cr, cb, cl Color, writes *int64) {
	if rad.Empty() {
		return
	}
	bounds := r.Intersection(b.Bounds())
	if bounds.Empty() {
		return
	}
	inner := Rect{X0: r.X0 + left, Y0: r.Y0 + top, X1: r.X1 - right, Y1: r.Y1 - bottom}
	innerRad := rad.Shrunk(top, right, bottom, left)
	if writes != nil {
		*writes += int64(bounds.W()) * int64(bounds.H())
	}
	for y := int(bounds.Y0); y < int(bounds.Y1); y++ {
		x0, x1 := rad.spanAt(r, y)
		switch {
		case y < int(r.Y0)+int(top):
			b.fillRowCover(y, x0, x1, ct)
		case y >= int(r.Y1)-int(bottom):
			b.fillRowCover(y, x0, x1, cb)
		default:
			ix0, ix1 := innerRad.spanAt(inner, y)
			b.fillRowCover(y, x0, ix0, cl)
			b.fillRowCover(y, ix1, x1, cr)
		}
	}
}

func (c Corners) spanAt(r Rect, y int) (x0, x1 float32) {
	x0, x1 = float32(r.X0), float32(r.X1)
	dy := float32(y) + 0.5 - float32(r.Y0)
	db := float32(r.Y1) - float32(y) - 0.5
	if dy < 0 {
		dy = 0
	}
	if db < 0 {
		db = 0
	}
	if i := arcInset(c.TL, dy); x0 < float32(r.X0)+i {
		x0 = float32(r.X0) + i
	}
	if i := arcInset(c.TR, dy); x1 > float32(r.X1)-i {
		x1 = float32(r.X1) - i
	}
	if i := arcInset(c.BL, db); x0 < float32(r.X0)+i {
		x0 = float32(r.X0) + i
	}
	if i := arcInset(c.BR, db); x1 > float32(r.X1)-i {
		x1 = float32(r.X1) - i
	}
	if x1 < x0 {
		x1 = x0
	}
	return
}

func arcInset(rad Radius, d float32) float32 {
	if rad.X <= 0 || rad.Y <= 0 || d >= rad.Y {
		return 0
	}
	t := (rad.Y - d) / rad.Y
	if t > 1 {
		t = 1
	}
	return rad.X * (1 - float32(math.Sqrt(float64(1-t*t))))
}

func (b *Bitmap) fillRowCover(y int, x0, x1 float32, c Color) {
	if x1 <= x0 {
		return
	}
	if x0 == float32(int32(x0)) && x1 == float32(int32(x1)) {
		b.FillRect(Rect4(int32(x0), int32(y), int32(x1), int32(y)+1), c, nil)
		return
	}
	leftPixel := int(math.Floor(float64(x0)))
	rightPixel := int(math.Floor(float64(x1)))

	if leftPixel == rightPixel {
		cov := x1 - x0
		if cov > 0.001 {
			if cov >= 0.999 {
				b.FillRect(Rect4(int32(leftPixel), int32(y), int32(leftPixel+1), int32(y)+1), c, nil)
			} else {
				b.Set(leftPixel, y, BlendOver(b.At(leftPixel, y), ScaleColor(c, cov)))
			}
		}
		return
	}

	leftCov := float32(float64(leftPixel+1) - float64(x0))
	if leftCov >= 0.999 {
		b.FillRect(Rect4(int32(leftPixel), int32(y), int32(leftPixel+1), int32(y)+1), c, nil)
	} else if leftCov > 0.001 {
		b.Set(leftPixel, y, BlendOver(b.At(leftPixel, y), ScaleColor(c, leftCov)))
	}

	if rightPixel > leftPixel+1 {
		b.FillRect(Rect4(int32(leftPixel+1), int32(y), int32(rightPixel), int32(y)+1), c, nil)
	}

	rightCov := float32(float64(x1) - float64(rightPixel))
	if rightCov >= 0.999 {
		b.FillRect(Rect4(int32(rightPixel), int32(y), int32(rightPixel+1), int32(y)+1), c, nil)
	} else if rightCov > 0.001 {
		b.Set(rightPixel, y, BlendOver(b.At(rightPixel, y), ScaleColor(c, rightCov)))
	}
}

func ScaleColor(c Color, k float32) Color {
	s := func(v uint8) uint32 { return uint32(float32(v) * k) }
	return Color(s(c.R())<<24 | s(c.G())<<16 | s(c.B())<<8 | s(c.A()))
}
