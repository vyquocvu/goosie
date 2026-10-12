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

// RadialGradient is a colour ramp radiating from a centre point.
// Shape is "circle" or "ellipse". Cx, Cy are the centre as fractions (0..1).
type RadialGradient struct {
	Shape string
	Cx    float32
	Cy    float32
	Stops []GradientStop
}

// Empty reports whether the ramp has too few colours to blend between. A single
// stop is a flat fill, and the caller keeps its background colour for that.
func (g LinearGradient) Empty() bool { return len(g.Stops) < 2 }

// Empty reports whether the radial gradient has nothing to paint.
func (g RadialGradient) Empty() bool { return len(g.Stops) < 2 }

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
	// One sortedness check per box: a sorted ramp takes the binary-search
	// path per pixel, an unsorted one keeps the exact linear scan. The two
	// agree on every sorted input (the lower bound is the first segment the
	// scan would stop at), so the pixels are identical either way.
	sorted := stopsSorted(g.Stops)
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
		// Hoisted out of the pixel loop: the clipped row extent is fixed for
		// the row, and every x it covers is inside cl, hence inside the bitmap.
		// One slice per row replaces the per-pixel bounds checks and headers.
		// A rounded span can narrow past the tile clip (x1 < x0); the original
		// pixel loop then simply did not execute, so neither does this.
		xi0, xi1 := int(math.Floor(x0)), int(math.Ceil(x1))
		if xi1 <= xi0 {
			continue
		}
		rowBase := y*b.Stride + xi0*4
		row := b.RGBA[rowBase : rowBase+(xi1-xi0)*4]
		for x := xi0; x < xi1; x++ {
			cov := math.Min(x1, float64(x+1)) - math.Max(x0, float64(x))
			if cov <= 0 {
				continue
			}
			t := (base + (float64(x)+0.5-cx)*dirX) / length
			var c Color
			if sorted {
				c = sampleGradientSorted(g.Stops, t)
			} else {
				c = sampleGradient(g.Stops, t)
			}
			if cov < 0.999 {
				c = ScaleColor(c, float32(cov))
			}
			p := row[(x-xi0)*4 : (x-xi0)*4+4 : (x-xi0)*4+4]
			dst := Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
			out := BlendOver(dst, c)
			p[0] = byte(out >> 24)
			p[1] = byte(out >> 16)
			p[2] = byte(out >> 8)
			p[3] = byte(out)
		}
	}
}

// stopsSorted reports whether the ramp's positions never step backwards. Only
// then does the binary-search sampler below agree with the linear scan; the
// caller checks once per box and the per-pixel path stays allocation-free.
func stopsSorted(stops []GradientStop) bool {
	for i := 1; i < len(stops); i++ {
		if stops[i].At < stops[i-1].At {
			return false
		}
	}
	return true
}

// sampleGradientSorted picks the ramp's colour at t for a sorted ramp. It is
// sampleGradient with the segment found by binary search instead of a linear
// scan: with stops[0].At < t < stops[last].At the lower bound is the first
// index whose stop reaches t, so stops[lo-1].At < t <= stops[lo].At, which is
// exactly the first segment the scan stops at, and the blend expression is the
// scan's own verbatim. Unsorted ramps keep the linear scan.
func sampleGradientSorted(stops []GradientStop, t float64) Color {
	if t <= float64(stops[0].At) {
		return stops[0].Color
	}
	last := stops[len(stops)-1]
	if t >= float64(last.At) {
		return last.Color
	}
	lo, hi := 1, len(stops)-1
	for lo < hi {
		mid := (lo + hi) >> 1
		if float64(stops[mid].At) < t {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	s, e := stops[lo-1], stops[lo]
	f := float64(0)
	if span := float64(e.At - s.At); span > 0 {
		f = (t - float64(s.At)) / span
	}
	return mixColor(s.Color, e.Color, f)
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

// FillRadialGradient paints r with a radial gradient. The gradient radiates
// from the centre point (Cx, Cy) as fractions of the box. For "ellipse" shape,
// the radius scales with the box aspect ratio; for "circle", it uses the
// smaller dimension.
func (b *Bitmap) FillRadialGradient(r Rect, rad Corners, g RadialGradient, writes *int64) {
	if g.Empty() {
		return
	}
	cl := r.Intersection(b.Bounds())
	if cl.Empty() {
		return
	}
	if writes != nil {
		*writes += int64(cl.W()) * int64(cl.H())
	}
	cx := float64(r.X0) + float64(g.Cx)*float64(r.W())
	cy := float64(r.Y0) + float64(g.Cy)*float64(r.H())
	// Radius: for ellipse, use the distance to the farthest corner; for circle,
	// use the smaller dimension.
	var radius float64
	if g.Shape == "circle" {
		radius = math.Min(float64(r.W()), float64(r.H())) / 2
	} else {
		// Ellipse: use the distance to the farthest corner from centre.
		dx := math.Max(cx-float64(r.X0), float64(r.X1)-cx)
		dy := math.Max(cy-float64(r.Y0), float64(r.Y1)-cy)
		radius = math.Sqrt(dx*dx + dy*dy)
	}
	if radius <= 0 {
		return
	}
	sorted := stopsSorted(g.Stops)
	for y := int(cl.Y0); y < int(cl.Y1); y++ {
		x0, x1 := float64(cl.X0), float64(cl.X1)
		if !rad.Empty() {
			l, rr := rad.spanAt(r, y)
			x0, x1 = math.Max(x0, float64(l)), math.Min(x1, float64(rr))
		}
		py := float64(y) + 0.5
		// Hoisted: the clipped row extent is fixed for the row, and every x it
		// covers is inside cl, hence inside the bitmap. One slice per row
		// replaces the per-pixel bounds checks and headers. As above, a span
		// narrowed past the clip executes nothing, exactly like the old loop.
		xi0, xi1 := int(math.Floor(x0)), int(math.Ceil(x1))
		if xi1 <= xi0 {
			continue
		}
		rowBase := y*b.Stride + xi0*4
		row := b.RGBA[rowBase : rowBase+(xi1-xi0)*4]
		dy := py - cy
		for x := xi0; x < xi1; x++ {
			cov := math.Min(x1, float64(x+1)) - math.Max(x0, float64(x))
			if cov <= 0 {
				continue
			}
			px := float64(x) + 0.5
			dx := px - cx
			dist := math.Sqrt(dx*dx + dy*dy)
			t := dist / radius
			if t > 1 {
				t = 1
			}
			var c Color
			if sorted {
				c = sampleGradientSorted(g.Stops, t)
			} else {
				c = sampleGradient(g.Stops, t)
			}
			if cov < 0.999 {
				c = ScaleColor(c, float32(cov))
			}
			p := row[(x-xi0)*4 : (x-xi0)*4+4 : (x-xi0)*4+4]
			dst := Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
			out := BlendOver(dst, c)
			p[0] = byte(out >> 24)
			p[1] = byte(out >> 16)
			p[2] = byte(out >> 8)
			p[3] = byte(out)
		}
	}
}

// mixColor blends two premultiplied colours. Linear interpolation is what keeps
// the result premultiplied, and the pair's alphas move with it.
func mixColor(a, b Color, f float64) Color {
	ch := func(av, bv uint8) uint32 {
		return uint32(float64(av) + (float64(bv)-float64(av))*f)
	}
	return Color(ch(a.R(), b.R())<<24 | ch(a.G(), b.G())<<16 | ch(a.B(), b.B())<<8 | ch(a.A(), b.A()))
}
