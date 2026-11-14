package frame

import "fmt"

type Point struct{ X, Y int32 }

func (p Point) Translate(dx, dy int32) Point { return Point{X: p.X + dx, Y: p.Y + dy} }

func (p Point) Sub(o Point) Point { return Point{X: p.X - o.X, Y: p.Y - o.Y} }

func (p Point) Add(o Point) Point { return Point{X: p.X + o.X, Y: p.Y + o.Y} }

type PointF struct{ X, Y float32 }

func (p PointF) Translate(dx, dy float32) PointF { return PointF{X: p.X + dx, Y: p.Y + dy} }

func (p PointF) ToDevice(scale float32) Point {
	return Point{X: round32(p.X * scale), Y: round32(p.Y * scale)}
}

type Size struct{ W, H int32 }

func (s Size) Area() int64 { return int64(s.W) * int64(s.H) }

func (s Size) Empty() bool { return s.W <= 0 || s.H <= 0 }

type Rect struct{ X0, Y0, X1, Y1 int32 }

func Rect4(x0, y0, x1, y1 int32) Rect { return Rect{X0: x0, Y0: y0, X1: x1, Y1: y1} }

func RectAt(min Point, sz Size) Rect {
	return Rect{X0: min.X, Y0: min.Y, X1: min.X + sz.W, Y1: min.Y + sz.H}
}

func (r Rect) W() int32 { return r.X1 - r.X0 }

func (r Rect) H() int32 { return r.Y1 - r.Y0 }

func (r Rect) Size() Size { return Size{W: r.W(), H: r.H()} }

func (r Rect) Origin() Point { return Point{X: r.X0, Y: r.Y0} }

func (r Rect) Empty() bool { return r.X1 <= r.X0 || r.Y1 <= r.Y0 }

func (r Rect) IsNormal() bool { return r.X1 >= r.X0 && r.Y1 >= r.Y0 }

func (r Rect) Canon() Rect {
	out := r
	if out.X1 < out.X0 {
		out.X0, out.X1 = out.X1, out.X0
	}
	if out.Y1 < out.Y0 {
		out.Y0, out.Y1 = out.Y1, out.Y0
	}
	return out
}

func (r Rect) Contains(p Point) bool {
	return p.X >= r.X0 && p.X < r.X1 && p.Y >= r.Y0 && p.Y < r.Y1
}

func (r Rect) Intersects(o Rect) bool {
	if r.Empty() || o.Empty() {
		return false
	}
	return r.X0 < o.X1 && o.X0 < r.X1 && r.Y0 < o.Y1 && o.Y0 < r.Y1
}

func (r Rect) Intersection(o Rect) Rect {
	out := Rect{
		X0: max32(r.X0, o.X0), Y0: max32(r.Y0, o.Y0),
		X1: min32(r.X1, o.X1), Y1: min32(r.Y1, o.Y1),
	}
	if out.Empty() {
		return Rect{}
	}
	return out
}

func (r Rect) Union(o Rect) Rect {
	if r.Empty() {
		return o
	}
	if o.Empty() {
		return r
	}
	return Rect{
		X0: min32(r.X0, o.X0), Y0: min32(r.Y0, o.Y0),
		X1: max32(r.X1, o.X1), Y1: max32(r.Y1, o.Y1),
	}
}

func (r Rect) Translate(dx, dy int32) Rect {
	return Rect{X0: r.X0 + dx, Y0: r.Y0 + dy, X1: r.X1 + dx, Y1: r.Y1 + dy}
}

func (r Rect) String() string {
	return fmt.Sprintf("Rect(%d,%d %dx%d)", r.X0, r.Y0, r.W(), r.H())
}

type RectF struct{ X0, Y0, X1, Y1 float32 }

func RectF4(x0, y0, x1, y1 float32) RectF { return RectF{X0: x0, Y0: y0, X1: x1, Y1: y1} }

func (r RectF) Canon() RectF {
	out := r
	if out.X1 < out.X0 {
		out.X0, out.X1 = out.X1, out.X0
	}
	if out.Y1 < out.Y0 {
		out.Y0, out.Y1 = out.Y1, out.Y0
	}
	return out
}

func (r RectF) Empty() bool { return r.X1 <= r.X0 || r.Y1 <= r.Y0 }

func (r RectF) Intersects(o RectF) bool {
	if r.Empty() || o.Empty() {
		return false
	}
	return r.X0 < o.X1 && o.X0 < r.X1 && r.Y0 < o.Y1 && o.Y0 < r.Y1
}

func (r RectF) Translate(dx, dy float32) RectF {
	return RectF{X0: r.X0 + dx, Y0: r.Y0 + dy, X1: r.X1 + dx, Y1: r.Y1 + dy}
}

func (r RectF) ToDevice(scale float32) Rect { return r.Canon().ToDeviceExact(scale) }

func (r RectF) ToDeviceExact(scale float32) Rect {
	if scale == 0 {
		return Rect{}
	}
	return Rect{
		X0: floor32(r.X0 * scale),
		Y0: floor32(r.Y0 * scale),
		X1: ceil32(r.X1 * scale),
		Y1: ceil32(r.Y1 * scale),
	}
}

func (r RectF) Inside(scale float32) Rect {
	if scale == 0 {
		return Rect{}
	}
	c := r.Canon()
	out := Rect{
		X0: ceil32(c.X0 * scale),
		Y0: ceil32(c.Y0 * scale),
		X1: floor32(c.X1 * scale),
		Y1: floor32(c.Y1 * scale),
	}
	if out.Empty() {
		return Rect{}
	}
	return out
}

func (r RectF) String() string {
	return fmt.Sprintf("RectF(%g,%g %gx%g)", r.X0, r.Y0, r.X1-r.X0, r.Y1-r.Y0)
}

type Viewport struct {
	Offset Point
	Size   Size
}

func (v Viewport) Rect() Rect { return RectAt(v.Offset, v.Size) }

func (v Viewport) Contains(p Point) bool { return v.Rect().Contains(p) }

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}
