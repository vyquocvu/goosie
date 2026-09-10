// Geometry primitives. The package-level coordinate-space contract lives in
// doc.go; everything here is value types with no locking and no allocation.

package frame

import "fmt"

// Point is an integer position in device pixels.
type Point struct{ X, Y int32 }

// Translate returns the point shifted by dx, dy. Tile and blit math is written
// almost entirely as translations of origins and rects, so Point carries the same
// operation as Rect instead of making every call site add by hand.
func (p Point) Translate(dx, dy int32) Point { return Point{X: p.X + dx, Y: p.Y + dy} }

// Sub returns the difference of two points, which is a delta rather than a
// position. Scroll deltas and damage offsets are both expressed this way.
func (p Point) Sub(o Point) Point { return Point{X: p.X - o.X, Y: p.Y - o.Y} }

// Add returns the sum of two points.
func (p Point) Add(o Point) Point { return Point{X: p.X + o.X, Y: p.Y + o.Y} }

// PointF is a position in layout (CSS) pixels.
type PointF struct{ X, Y float32 }

// Translate returns the point shifted by dx, dy.
func (p PointF) Translate(dx, dy float32) PointF { return PointF{X: p.X + dx, Y: p.Y + dy} }

// ToDevice scales a layout point to the nearest device pixel. Glyph positions
// want the nearest pixel rather than the enclosing one, so this rounds half away
// from zero where RectF.ToDevice rounds outward.
func (p PointF) ToDevice(scale float32) Point {
	return Point{X: round32(p.X * scale), Y: round32(p.Y * scale)}
}

// Size is a width and height in device pixels.
type Size struct{ W, H int32 }

// Area returns the pixel count as an int64 so large surfaces cannot overflow.
func (s Size) Area() int64 { return int64(s.W) * int64(s.H) }

// Empty reports whether either dimension is non-positive.
func (s Size) Empty() bool { return s.W <= 0 || s.H <= 0 }

// Rect is an axis-aligned region in content space, stored as edges rather than
// origin-plus-size so that intersection and emptiness need no sign reasoning.
type Rect struct{ X0, Y0, X1, Y1 int32 }

// Rect constructs a Rect from its edges without normalizing them.
func Rect4(x0, y0, x1, y1 int32) Rect { return Rect{X0: x0, Y0: y0, X1: x1, Y1: y1} }

// RectAt returns a rect of the given size with its top-left at min.
func RectAt(min Point, sz Size) Rect {
	return Rect{X0: min.X, Y0: min.Y, X1: min.X + sz.W, Y1: min.Y + sz.H}
}

// W returns the horizontal extent. Prefer Canon first; an inverted rect reports
// a negative width rather than silently flipping.
func (r Rect) W() int32 { return r.X1 - r.X0 }

// H returns the vertical extent.
func (r Rect) H() int32 { return r.Y1 - r.Y0 }

// Size returns the extent as a Size.
func (r Rect) Size() Size { return Size{W: r.W(), H: r.H()} }

// Origin returns the top-left corner.
func (r Rect) Origin() Point { return Point{X: r.X0, Y: r.Y0} }

// Empty reports whether the rect covers no pixels.
func (r Rect) Empty() bool { return r.X1 <= r.X0 || r.Y1 <= r.Y0 }

// IsNormal reports whether the edges are ordered. Algebra methods assume it.
func (r Rect) IsNormal() bool { return r.X1 >= r.X0 && r.Y1 >= r.Y0 }

// Canon returns the equivalent rect with ordered edges.
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

// Contains reports whether p lies inside the rect. The far edges are exclusive
// so two horizontally adjacent rects never both contain the shared edge, which
// is what keeps a tile lookup from double-counting a pixel.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X0 && p.X < r.X1 && p.Y >= r.Y0 && p.Y < r.Y1
}

// Intersects reports whether the two rects share at least one pixel. Touching
// edges do not count.
func (r Rect) Intersects(o Rect) bool {
	if r.Empty() || o.Empty() {
		return false
	}
	return r.X0 < o.X1 && o.X0 < r.X1 && r.Y0 < o.Y1 && o.Y0 < r.Y1
}

// Intersection returns the shared region, or an empty Rect if there is none.
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

// Union returns the smallest rect covering both. An empty argument returns the
// other unchanged rather than a degenerate rect at the origin.
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

// Translate returns the rect shifted by dx, dy.
func (r Rect) Translate(dx, dy int32) Rect {
	return Rect{X0: r.X0 + dx, Y0: r.Y0 + dy, X1: r.X1 + dx, Y1: r.Y1 + dy}
}

func (r Rect) String() string {
	return fmt.Sprintf("Rect(%d,%d %dx%d)", r.X0, r.Y0, r.W(), r.H())
}

// RectF is a rect in layout pixels.
type RectF struct{ X0, Y0, X1, Y1 float32 }

// RectF4 constructs a RectF from its edges.
func RectF4(x0, y0, x1, y1 float32) RectF { return RectF{X0: x0, Y0: y0, X1: x1, Y1: y1} }

// Canon returns the equivalent RectF with ordered edges.
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

// Empty reports whether the rect covers no area.
func (r RectF) Empty() bool { return r.X1 <= r.X0 || r.Y1 <= r.Y0 }

// Intersects reports whether two layout rects share area.
func (r RectF) Intersects(o RectF) bool {
	if r.Empty() || o.Empty() {
		return false
	}
	return r.X0 < o.X1 && o.X0 < r.X1 && r.Y0 < o.Y1 && o.Y0 < r.Y1
}

// Translate returns the rect shifted by dx, dy.
func (r RectF) Translate(dx, dy float32) RectF {
	return RectF{X0: r.X0 + dx, Y0: r.Y0 + dy, X1: r.X1 + dx, Y1: r.Y1 + dy}
}

// ToDevice scales into device space with outward rounding, so a fractional
// device pixel is never dropped at the edges of a clip or tile. A zero scale
// yields an empty rect rather than a division by zero.
func (r RectF) ToDevice(scale float32) Rect { return r.Canon().ToDeviceExact(scale) }

// ToDeviceExact is ToDevice without normalization, kept separate so the
// rounding rule has one implementation.
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

// Inside scales into device space with inward rounding: the largest device rect
// fully covered by the layout rect. Used for clip bounds, where painting a
// pixel outside the clip is worse than leaving an edge pixel unpainted.
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

// Viewport is the visible window into content space, in device pixels.
type Viewport struct {
	Offset Point
	Size   Size
}

// Rect returns the viewport as a content-space rect, which is what tile and
// damage math consumes.
func (v Viewport) Rect() Rect { return RectAt(v.Offset, v.Size) }

// Contains reports whether a content-space point is currently visible.
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
