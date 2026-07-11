// Package frame defines platform-neutral frame types for the raster backend.
//
// M6.1: These types form the contract between the display list builder and
// any raster backend (CPU, GPU, or Fyne adapter). No backend-specific types
// (e.g. Fyne, image.Image) appear in this package.
package frame

import (
	"hash/fnv"
	"image/color"
	"math"
)

// ---------------------------------------------------------------------------
// Color — packed 32-bit RGBA
// ---------------------------------------------------------------------------

// Color is a compact, value-type RGBA color stored as a single uint32.
// Layout: R in bits 24-31, G in bits 16-23, B in bits 8-15, A in bits 0-7.
// Using a packed representation avoids the interface allocation of color.Color.
type Color struct {
	packed uint32
}

// Sentinel colors.
var (
	Transparent = Color{}                   // RGBA(0,0,0,0)
	Opaque      = Color{packed: 0x000000FF} // RGBA(0,0,0,255)
	White       = Color{packed: 0xFFFFFFFF} // RGBA(255,255,255,255)
	Black       = Color{packed: 0x000000FF} // alias for Opaque
)

// NewColor creates a Color from individual 8-bit channels.
func NewColor(r, g, b, a uint8) Color {
	return Color{packed: uint32(r)<<24 | uint32(g)<<16 | uint32(b)<<8 | uint32(a)}
}

// FromStdColor converts a standard library color.Color to a packed Color.
func FromStdColor(c color.Color) Color {
	if c == nil {
		return Transparent
	}
	r, g, b, a := c.RGBA()
	// color.RGBA() returns 16-bit pre-multiplied values; convert to 8-bit.
	return NewColor(uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
}

// StdColor converts to a standard library color.RGBA.
func (c Color) StdColor() color.RGBA {
	return color.RGBA{R: c.R(), G: c.G(), B: c.B(), A: c.A()}
}

// R returns the red channel (0-255).
func (c Color) R() uint8 { return uint8(c.packed >> 24) }

// G returns the green channel (0-255).
func (c Color) G() uint8 { return uint8(c.packed >> 16) }

// B returns the blue channel (0-255).
func (c Color) B() uint8 { return uint8(c.packed >> 8) }

// A returns the alpha channel (0-255).
func (c Color) A() uint8 { return uint8(c.packed) }

// WithAlpha returns a copy with the alpha channel replaced.
func (c Color) WithAlpha(a uint8) Color {
	return Color{packed: (c.packed & 0xFFFFFF00) | uint32(a)}
}

// IsOpaque reports whether alpha == 255.
func (c Color) IsOpaque() bool { return c.A() == 255 }

// IsFullyTransparent reports whether alpha == 0.
func (c Color) IsFullyTransparent() bool { return c.A() == 0 }

// LerpColor linearly interpolates between a and b by t ∈ [0,1].
// t is clamped to [0,1]. Each channel is interpolated independently.
func LerpColor(a, b Color, t float32) Color {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	lerp := func(x, y uint8) uint8 {
		return uint8(math.Round(float64(x) + float64(t)*float64(int(y)-int(x))))
	}
	return NewColor(lerp(a.R(), b.R()), lerp(a.G(), b.G()), lerp(a.B(), b.B()), lerp(a.A(), b.A()))
}

// ---------------------------------------------------------------------------
// Point — 2D coordinate
// ---------------------------------------------------------------------------

// Point is a 2D float32 coordinate in layout-space pixels.
type Point struct {
	X float32
	Y float32
}

// PointZero is the origin (0,0).
var PointZero = Point{}

// NewPoint creates a Point.
func NewPoint(x, y float32) Point { return Point{X: x, Y: y} }

// Add returns p + q.
func (p Point) Add(q Point) Point { return Point{X: p.X + q.X, Y: p.Y + q.Y} }

// Sub returns p - q.
func (p Point) Sub(q Point) Point { return Point{X: p.X - q.X, Y: p.Y - q.Y} }

// Scale returns p * s.
func (p Point) Scale(s float32) Point { return Point{X: p.X * s, Y: p.Y * s} }

// DistanceTo returns the Euclidean distance from p to q.
func (p Point) DistanceTo(q Point) float32 {
	dx := q.X - p.X
	dy := q.Y - p.Y
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// Equal reports whether p and q are exactly equal.
func (p Point) Equal(q Point) bool { return p.X == q.X && p.Y == q.Y }

// ---------------------------------------------------------------------------
// Rect — axis-aligned rectangle
// ---------------------------------------------------------------------------

// Rect is a float32 axis-aligned rectangle used in frame types.
// All coordinates are in layout-space pixels.
type Rect struct {
	X float32 // Left edge
	Y float32 // Top edge
	W float32 // Width (≥ 0)
	H float32 // Height (≥ 0)
}

// RectZero is the zero rectangle.
var RectZero = Rect{}

// NewRect creates a Rect.
func NewRect(x, y, w, h float32) Rect { return Rect{X: x, Y: y, W: w, H: h} }

// MaxX returns the right edge (X + W).
func (r Rect) MaxX() float32 { return r.X + r.W }

// MaxY returns the bottom edge (Y + H).
func (r Rect) MaxY() float32 { return r.Y + r.H }

// Contains reports whether point p lies within the rectangle.
// The rectangle includes its left/top edges but excludes its right/bottom edges.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H
}

// Intersects reports whether r and other overlap.
func (r Rect) Intersects(other Rect) bool {
	return r.X < other.X+other.W && r.X+r.W > other.X &&
		r.Y < other.Y+other.H && r.Y+r.H > other.Y
}

// Intersection returns the overlapping region of r and other.
// Returns RectZero if they do not overlap.
func (r Rect) Intersection(other Rect) Rect {
	x0 := max32(r.X, other.X)
	y0 := max32(r.Y, other.Y)
	x1 := min32(r.X+r.W, other.X+other.W)
	y1 := min32(r.Y+r.H, other.Y+other.H)
	if x1 <= x0 || y1 <= y0 {
		return RectZero
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Union returns the smallest rectangle enclosing both r and other.
func (r Rect) Union(other Rect) Rect {
	if r.IsEmpty() {
		return other
	}
	if other.IsEmpty() {
		return r
	}
	x0 := min32(r.X, other.X)
	y0 := min32(r.Y, other.Y)
	x1 := max32(r.X+r.W, other.X+other.W)
	y1 := max32(r.Y+r.H, other.Y+other.H)
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Expand returns a new rect expanded by amount on all sides.
func (r Rect) Expand(amount float32) Rect {
	return Rect{
		X: r.X - amount,
		Y: r.Y - amount,
		W: r.W + 2*amount,
		H: r.H + 2*amount,
	}
}

// IsEmpty reports whether the rect has zero or negative area.
func (r Rect) IsEmpty() bool { return r.W <= 0 || r.H <= 0 }

// Equal reports whether r and other are exactly equal.
func (r Rect) Equal(other Rect) bool {
	return r.X == other.X && r.Y == other.Y && r.W == other.W && r.H == other.H
}

// ---------------------------------------------------------------------------
// Handles — typed uint32 identifiers for backend-managed resources
// ---------------------------------------------------------------------------

// ImageHandle is a uint32 handle referencing a decoded image in the raster
// backend's image cache. Zero is the invalid/unset sentinel.
type ImageHandle uint32

// Valid reports whether the handle is non-zero (i.e. references a resource).
func (h ImageHandle) Valid() bool { return h != 0 }

// FontHandle is a uint32 handle referencing a loaded font face in the raster
// backend's glyph cache. Zero is the invalid/unset sentinel.
type FontHandle uint32

// Valid reports whether the handle is non-zero.
func (h FontHandle) Valid() bool { return h != 0 }

// ---------------------------------------------------------------------------
// Glyph and TextRun — shaped text
// ---------------------------------------------------------------------------

// Glyph represents a single shaped glyph within a text run.
type Glyph struct {
	ID      uint32  // Font-specific glyph identifier
	Advance float32 // Horizontal advance width in pixels
	XOffset float32 // Horizontal offset from the pen position
	YOffset float32 // Vertical offset from the baseline
}

// TextRun is a sequence of shaped glyphs ready for rasterization.
// All glyphs share the same font, size, and color.
type TextRun struct {
	Font     FontHandle
	FontSize float32
	Color    Color
	Glyphs   []Glyph
}

// Len returns the number of glyphs.
func (tr TextRun) Len() int { return len(tr.Glyphs) }

// IsEmpty reports whether the text run has no glyphs.
func (tr TextRun) IsEmpty() bool { return len(tr.Glyphs) == 0 }

// Width returns the total advance width of all glyphs.
func (tr TextRun) Width() float32 {
	var w float32
	for i := range tr.Glyphs {
		w += tr.Glyphs[i].Advance
	}
	return w
}

// ---------------------------------------------------------------------------
// PixelScale — DPI and device-pixel conversion
// ---------------------------------------------------------------------------

// PixelScale maps layout-space pixels to device pixels.
type PixelScale struct {
	Scale float32 // Device-pixels per layout-pixel (e.g. 2.0 for Retina)
	DPI   float32 // Dots per inch (e.g. 96 for standard, 192 for HiDPI)
}

// PixelScaleDefault is 1x scale at 96 DPI.
var PixelScaleDefault = PixelScale{Scale: 1.0, DPI: 96}

// PixelScaleFromDPI creates a PixelScale from a DPI value.
// The scale factor is derived as dpi/96. Zero or negative DPI falls back to 96.
func PixelScaleFromDPI(dpi float32) PixelScale {
	if dpi <= 0 {
		dpi = 96
	}
	return PixelScale{Scale: dpi / 96, DPI: dpi}
}

// ToPixels converts a layout-space dimension to device pixels (float32).
func (ps PixelScale) ToPixels(layout float32) float32 {
	return layout * ps.Scale
}

// ToDevicePixels converts a layout-space dimension to device pixels (int32, rounded).
func (ps PixelScale) ToDevicePixels(layout float32) int32 {
	return int32(math.Round(float64(layout * ps.Scale)))
}

// FromDevicePixels converts device pixels back to layout-space.
func (ps PixelScale) FromDevicePixels(device int32) float32 {
	if ps.Scale == 0 {
		return 0
	}
	return float32(device) / ps.Scale
}

// ---------------------------------------------------------------------------
// Viewport — frame dimensions and scroll offset
// ---------------------------------------------------------------------------

// Viewport describes the visible area of a frame.
type Viewport struct {
	Width      float32    // Layout-space width
	Height     float32    // Layout-space height
	ScrollX    float32    // Horizontal scroll offset (≥ 0)
	ScrollY    float32    // Vertical scroll offset (≥ 0)
	PixelScale PixelScale // Device pixel mapping
}

// NewViewport creates a Viewport with zero scroll. Negative dimensions clamp to 0.
func NewViewport(width, height float32, ps PixelScale) Viewport {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return Viewport{Width: width, Height: height, PixelScale: ps}
}

// WithScroll returns a copy with the scroll offset set. Negative values clamp to 0.
func (v Viewport) WithScroll(x, y float32) Viewport {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	v.ScrollX = x
	v.ScrollY = y
	return v
}

// DeviceSize returns the viewport size in device pixels.
func (v Viewport) DeviceSize() (width, height int32) {
	return v.PixelScale.ToDevicePixels(v.Width), v.PixelScale.ToDevicePixels(v.Height)
}

// VisibleRect returns the visible region in layout-space coordinates.
func (v Viewport) VisibleRect() Rect {
	return Rect{X: v.ScrollX, Y: v.ScrollY, W: v.Width, H: v.Height}
}

// Equal reports whether v and other are exactly equal.
func (v Viewport) Equal(other Viewport) bool {
	return v.Width == other.Width && v.Height == other.Height &&
		v.ScrollX == other.ScrollX && v.ScrollY == other.ScrollY &&
		v.PixelScale.Scale == other.PixelScale.Scale &&
		v.PixelScale.DPI == other.PixelScale.DPI
}

// ---------------------------------------------------------------------------
// FrameSnapshot — immutable frame
// ---------------------------------------------------------------------------

// FrameSnapshot is an immutable snapshot of a rendered frame.
// Once created, none of its fields change. This allows the raster backend
// and presentation adapter to read it without locking mutable engine state.
type FrameSnapshot struct {
	Generation   uint64   // Monotonic frame counter
	Viewport     Viewport // Viewport at capture time
	CommandCount int      // Number of display commands
	ContentHash  uint64   // FNV-1a hash of viewport + command count for fast comparison
}

// NewFrameSnapshot creates an immutable frame snapshot.
// The commands slice is NOT copied — the caller must ensure it is not mutated
// after this call (or pass nil for an empty frame).
func NewFrameSnapshot(gen uint64, vp Viewport, cmds interface{}) FrameSnapshot {
	snap := FrameSnapshot{
		Generation: gen,
		Viewport:   vp,
	}
	// Compute content hash from viewport + command count.
	h := fnv.New64a()
	// Hash viewport fields.
	buf := [4]uint32{
		math.Float32bits(vp.Width),
		math.Float32bits(vp.Height),
		math.Float32bits(vp.ScrollX),
		math.Float32bits(vp.ScrollY),
	}
	for _, v := range buf {
		b := [4]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
		h.Write(b[:])
	}
	snap.ContentHash = h.Sum64()
	return snap
}

// Bounds returns the visible region in layout-space coordinates.
func (s FrameSnapshot) Bounds() Rect {
	return s.Viewport.VisibleRect()
}

// DeviceBounds returns the frame size in device pixels.
func (s FrameSnapshot) DeviceBounds() (width, height int32) {
	return s.Viewport.DeviceSize()
}

// IsNewerThan reports whether s has a strictly higher generation than other.
func (s FrameSnapshot) IsNewerThan(other FrameSnapshot) bool {
	return s.Generation > other.Generation
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
