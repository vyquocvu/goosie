package frame

import "math"

type GradientStop struct {
	At    float32
	Color Color
}

type LinearGradient struct {
	Angle float32
	Stops []GradientStop
}

type RadialGradient struct {
	Cx    float32
	Cy    float32
	Rx    float32
	Ry    float32
	Stops []GradientStop
}

func (g LinearGradient) Empty() bool { return len(g.Stops) < 2 }

func (g RadialGradient) Empty() bool { return len(g.Stops) < 2 }

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
	sorted := stopsSorted(g.Stops)
	for y := int(cl.Y0); y < int(cl.Y1); y++ {
		x0, x1 := float64(cl.X0), float64(cl.X1)
		if !rad.Empty() {
			l, rr := rad.spanAt(r, y)
			x0, x1 = math.Max(x0, float64(l)), math.Min(x1, float64(rr))
		}
		py := float64(y) + 0.5
		base := (py-cy)*dirY + length/2
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

func stopsSorted(stops []GradientStop) bool {
	for i := 1; i < len(stops); i++ {
		if stops[i].At < stops[i-1].At {
			return false
		}
	}
	return true
}

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

func (b *Bitmap) FillRadialGradient(r Rect, rad Corners, g RadialGradient, writes *int64) {
	if g.Empty() || g.Rx <= 0 || g.Ry <= 0 {
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
	rx, ry := float64(g.Rx), float64(g.Ry)
	sorted := stopsSorted(g.Stops)
	for y := int(cl.Y0); y < int(cl.Y1); y++ {
		x0, x1 := float64(cl.X0), float64(cl.X1)
		if !rad.Empty() {
			l, rr := rad.spanAt(r, y)
			x0, x1 = math.Max(x0, float64(l)), math.Min(x1, float64(rr))
		}
		py := float64(y) + 0.5
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
			ux, uy := dx/rx, dy/ry
			t := math.Sqrt(ux*ux + uy*uy)
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

func mixColor(a, b Color, f float64) Color {
	ch := func(av, bv uint8) uint32 {
		return uint32(float64(av) + (float64(bv)-float64(av))*f)
	}
	return Color(ch(a.R(), b.R())<<24 | ch(a.G(), b.G())<<16 | ch(a.B(), b.B())<<8 | ch(a.A(), b.A()))
}
