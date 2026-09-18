package frame

import "math"

// GradientStop is one colour on a gradient line. At is its distance along the
// line as a fraction of that line's length, measured from its start.
type GradientStop struct {
	At    float32
	Color Color
}

// LinearGradient is a colour ramp drawn straight across a box. Angle is CSS's:
// 0deg points at the top of the box and grows clockwise, so the default 180deg
// runs from the top edge down.
type LinearGradient struct {
	Angle float32
	Stops []GradientStop
}

// Empty reports whether the ramp has too few colours to blend between. A single
// stop is a flat fill, and the caller keeps its background colour for that.
func (g LinearGradient) Empty() bool { return len(g.Stops) < 2 }

// FillLinearGradient paints r with the ramp, rounding the corners along rad.
//
// The gradient line is the one CSS defines: it crosses the box's centre at the
// ramp's angle and is |W·sin θ| + |H·cos θ| long, which is what makes a 45deg
// ramp reach the corners instead of running out halfway to them. Everything
// outside the line's 0..1 span takes the nearest stop's colour.
func (b *Bitmap) FillLinearGradient(r Rect, rad Corners, g LinearGradient, writes *int64) {
	if g.Empty() {
		return
	}
	cl := r.Intersection(b.Bounds())
	if cl.Empty() {
		return
	}
	a := float64(g.Angle) * math.Pi / 180
	dirX, dirY := math.Sin(a), -math.Cos(a)
	length := math.Abs(float64(r.W())*dirX) + math.Abs(float64(r.H())*dirY)
	if length <= 0 {
		return
	}
	cx := (float64(r.X0) + float64(r.X1)) / 2
	cy := (float64(r.Y0) + float64(r.Y1)) / 2
	if writes != nil {
		*writes += int64(cl.W()) * int64(cl.H())
	}
	for y := int(cl.Y0); y < int(cl.Y1); y++ {
		x0, x1 := float64(cl.X0), float64(cl.X1)
		if !rad.Empty() {
			l, rr := rad.spanAt(r, y)
			x0, x1 = math.Max(x0, float64(l)), math.Min(x1, float64(rr))
		}
		py := float64(y) + 0.5
		// Along the gradient line, so the whole row shares one term and only the
		// horizontal projection has to be recomputed per pixel.
		base := (py-cy)*dirY + length/2
		for x := int(math.Floor(x0)); x < int(math.Ceil(x1)); x++ {
			cov := math.Min(x1, float64(x+1)) - math.Max(x0, float64(x))
			if cov <= 0 {
				continue
			}
			t := (base + (float64(x)+0.5-cx)*dirX) / length
			c := sampleGradient(g.Stops, t)
			if cov < 0.999 {
				c = ScaleColor(c, float32(cov))
			}
			b.Set(x, y, BlendOver(b.At(x, y), c))
		}
	}
}

// sampleGradient picks the ramp's colour at t, which may fall outside the
// declared stops and takes the nearest one's colour there.
func sampleGradient(stops []GradientStop, t float64) Color {
	if t <= float64(stops[0].At) {
		return stops[0].Color
	}
	last := stops[len(stops)-1]
	if t >= float64(last.At) {
		return last.Color
	}
	for i := 0; i+1 < len(stops); i++ {
		s, e := stops[i], stops[i+1]
		if t < float64(s.At) || t > float64(e.At) {
			continue
		}
		f := float64(0)
		if span := float64(e.At - s.At); span > 0 {
			f = (t - float64(s.At)) / span
		}
		return mixColor(s.Color, e.Color, f)
	}
	return last.Color
}

// mixColor blends two premultiplied colours. Linear interpolation is what keeps
// the result premultiplied, and the pair's alphas move with it.
func mixColor(a, b Color, f float64) Color {
	ch := func(av, bv uint8) uint32 {
		return uint32(float64(av) + (float64(bv)-float64(av))*f)
	}
	return Color(ch(a.R(), b.R())<<24 | ch(a.G(), b.G())<<16 | ch(a.B(), b.B())<<8 | ch(a.A(), b.A()))
}
