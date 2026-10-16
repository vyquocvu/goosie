package frame

import "math"

// Radius is one corner of a rounded box: the horizontal and vertical radii of the
// quarter ellipse that cuts it, in device pixels.
type Radius struct{ X, Y float32 }

// Corners are a box's four corner radii, ordered top-left, top-right,
// bottom-right, bottom-left, which is the order the CSS shorthand fills them.
type Corners struct{ TL, TR, BR, BL Radius }

// Empty reports whether every corner is square, which lets a caller keep the
// whole-row fast path for the boxes that declare no radius at all.
func (c Corners) Empty() bool { return c == Corners{} }

// Scaled returns the radii in device pixels for a layer drawn at scale.
func (c Corners) Scaled(scale float32) Corners {
	mul := func(r Radius) Radius { return Radius{X: r.X * scale, Y: r.Y * scale} }
	return Corners{TL: mul(c.TL), TR: mul(c.TR), BR: mul(c.BR), BL: mul(c.BL)}
}

// Shrunk returns the radii of the box inset by the given edge widths, which is
// how a rounded border's inner outline is derived: a corner loses as much radius
// as the edges meeting it take in width.
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

// FillRounded paints r with c, cutting its corners along rad. It is FillRect for
// boxes that declare a radius: rows outside the corner bands keep the whole-row
// copy, and only the rows an arc crosses take the per-pixel coverage path.
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

// StrokeRounded paints the ring between the outer rounded box and the box its
// edge widths leave inside, one colour per edge. Corner bands belong to the
// horizontal edges, which is how a real browser miters them.
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

// spanAt returns the horizontal extent the rounded outline covers across row y.
// The row's centre line decides the arc inset, so a corner's edge pixels pick up
// partial coverage instead of snapping to whole pixels.
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

// arcInset is how far a corner's quarter ellipse reaches into the row running d
// device pixels in from the edge that corner sits on.
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

// fillRowCover writes one row between two horizontal edges, blending the pixels
// an arc only partly covers.
func (b *Bitmap) fillRowCover(y int, x0, x1 float32, c Color) {
	if x1 <= x0 {
		return
	}
	if x0 == float32(int32(x0)) && x1 == float32(int32(x1)) {
		b.FillRect(Rect4(int32(x0), int32(y), int32(x1), int32(y)+1), c, nil)
		return
	}
	for x := int(math.Floor(float64(x0))); x < int(math.Ceil(float64(x1))); x++ {
		cov := math.Min(float64(x1), float64(x+1)) - math.Max(float64(x0), float64(x))
		if cov <= 0 {
			continue
		}
		if cov >= 0.999 {
			b.Set(x, y, c)
			continue
		}
		b.Set(x, y, BlendOver(b.At(x, y), ScaleColor(c, float32(cov))))
	}
}

// ScaleColor reduces a premultiplied colour to a fraction of its own coverage,
// which is the only scaling that leaves it premultiplied. Opacity and partial
// pixel coverage both arrive here.
func ScaleColor(c Color, k float32) Color {
	s := func(v uint8) uint32 { return uint32(float32(v) * k) }
	return Color(s(c.R())<<24 | s(c.G())<<16 | s(c.B())<<8 | s(c.A()))
}
