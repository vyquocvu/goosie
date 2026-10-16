package frame_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
)

func TestRectAlgebraOnInvertedInput(t *testing.T) {
	r := frame.Rect4(30, 40, 10, 20)
	if r.IsNormal() {
		t.Fatal("inverted rect reported as normal")
	}
	if got := r.W(); got != -20 {
		t.Fatalf("W on inverted rect = %d, want -20 (negative, not silently flipped)", got)
	}
	c := r.Canon()
	if want := frame.Rect4(10, 20, 30, 40); c != want {
		t.Fatalf("Canon = %v, want %v", c, want)
	}
	if c.W() != 20 || c.H() != 20 {
		t.Fatalf("Canon size = %dx%d, want 20x20", c.W(), c.H())
	}
	if got := c.Size(); got != (frame.Size{W: 20, H: 20}) {
		t.Fatalf("Size = %v, want 20x20", got)
	}
	if got := (frame.Size{W: 20, H: 10}).Area(); got != 200 {
		t.Fatalf("Area = %d, want 200", got)
	}
}

func TestRectContainsFarEdgeExclusive(t *testing.T) {
	// Two horizontally adjacent tiles must never both claim the shared edge,
	// because a double-claimed pixel is a double-blitted tile.
	left := frame.Rect4(0, 0, 8, 8)
	right := frame.Rect4(8, 0, 16, 8)
	if !left.Contains(frame.Point{X: 0, Y: 0}) {
		t.Fatal("top-left corner is not inside its own rect")
	}
	if left.Contains(frame.Point{X: 8, Y: 0}) {
		t.Fatal("far edge reported as inside")
	}
	if !right.Contains(frame.Point{X: 8, Y: 0}) {
		t.Fatal("shared edge belongs to neither rect")
	}
	if !left.Contains(frame.Point{X: 7, Y: 7}) {
		t.Fatal("last covered pixel reported as outside")
	}
}

func TestRectIntersectionAndUnion(t *testing.T) {
	a := frame.Rect4(0, 0, 10, 10)
	b := frame.Rect4(5, 5, 15, 15)
	if !a.Intersects(b) {
		t.Fatal("overlapping rects reported disjoint")
	}
	if got := a.Intersection(b); got != frame.Rect4(5, 5, 10, 10) {
		t.Fatalf("intersection = %v, want 5,5 5x5", got)
	}
	// Touching edges share no pixel.
	c := frame.Rect4(10, 0, 20, 10)
	if a.Intersects(c) {
		t.Fatal("edge-touching rects reported intersecting")
	}
	if got := a.Intersection(c); !got.Empty() {
		t.Fatalf("disjoint intersection = %v, want empty", got)
	}
	if got := a.Union(c); got != frame.Rect4(0, 0, 20, 10) {
		t.Fatalf("union = %v, want 0,0 20x10", got)
	}
	if got := (frame.Rect{}).Union(a); got != a {
		t.Fatalf("union with empty = %v, want the non-empty operand", got)
	}
	if got := a.Translate(3, -2); got != frame.Rect4(3, -2, 13, 8) {
		t.Fatalf("translate = %v", got)
	}
}

func TestRectFToDeviceRoundsOutwardAndInsideInward(t *testing.T) {
	// 0.5 CSS px at scale 1 covers half a device pixel. A clip must not paint
	// it; a coverage bound must not drop it. Both rules are the same number
	// resolved in opposite directions, so they need one fixture.
	r := frame.RectF4(0.5, 0.5, 2.5, 2.5)
	if got, want := r.ToDevice(1), frame.Rect4(0, 0, 3, 3); got != want {
		t.Fatalf("ToDevice = %v, want %v (outward)", got, want)
	}
	if got, want := r.Inside(1), frame.Rect4(1, 1, 2, 2); got != want {
		t.Fatalf("Inside = %v, want %v (inward)", got, want)
	}
	// Negative coordinates are where a naive int32 conversion truncates toward
	// zero and maps content into the wrong tile.
	neg := frame.RectF4(-2.5, -2.5, -0.5, -0.5)
	if got, want := neg.ToDevice(1), frame.Rect4(-3, -3, 0, 0); got != want {
		t.Fatalf("ToDevice on negatives = %v, want %v", got, want)
	}
	if got, want := neg.Inside(1), frame.Rect4(-2, -2, -1, -1); got != want {
		t.Fatalf("Inside on negatives = %v, want %v", got, want)
	}
	if got := r.ToDevice(0); !got.Empty() {
		t.Fatalf("zero scale = %v, want empty rather than a division by zero", got)
	}
	// DPR 2 with a fractional CSS position is the ordinary case, not the edge case.
	if got, want := frame.RectF4(1.25, 1.25, 3.75, 3.75).ToDevice(2), frame.Rect4(2, 2, 8, 8); got != want {
		t.Fatalf("ToDevice at DPR2 = %v, want %v", got, want)
	}
}

func TestColorPackingAndPremultiply(t *testing.T) {
	c := frame.RGB(0x11, 0x22, 0x33)
	if uint32(c) != 0x112233FF {
		t.Fatalf("opaque packing = %#08x, want 0x112233ff", uint32(c))
	}
	if c.R() != 0x11 || c.G() != 0x22 || c.B() != 0x33 || c.A() != 0xFF {
		t.Fatalf("round trip lost a channel: %02x %02x %02x %02x", c.R(), c.G(), c.B(), c.A())
	}
	if !c.Opaque() {
		t.Fatal("opaque color reported translucent; the row-copy fast path keys on this")
	}
	if frame.TransparentBlack.Opaque() {
		t.Fatal("transparent black reported opaque")
	}

	// Premultiplying at half coverage halves the stored channels, and fully
	// transparent stores nothing at all: an unpremultiplied white with A=0
	// would blend as white.
	half := frame.RGBA(255, 255, 255, 128)
	if got := half.R(); got != 128 {
		t.Fatalf("premultiplied half-white red = %d, want 128", got)
	}
	if got := frame.RGBA(255, 255, 255, 0); got != frame.TransparentBlack {
		t.Fatalf("fully transparent = %#08x, want 0", uint32(got))
	}
}

func TestBlendOver(t *testing.T) {
	white := frame.RGB(255, 255, 255)
	black := frame.RGB(0, 0, 0)
	if got := frame.BlendOver(black, white); got != white {
		t.Fatalf("opaque over black = %v, want the source", got)
	}
	if got := frame.BlendOver(white, frame.TransparentBlack); got != white {
		t.Fatalf("transparent over white = %v, want the destination", got)
	}
	half := frame.RGBA(0, 0, 0, 127)
	got := frame.BlendOver(white, half)
	// 255*(1-127/255) = 128, with the +127 rounding term. Without the rounding
	// term repeated compositing drifts dark, which reads as a grey page.
	if got.R() != 128 || got.A() != 255 {
		t.Fatalf("half-black over white = r%d a%d, want r128 a255", got.R(), got.A())
	}
}

func TestViewportRectAndPoints(t *testing.T) {
	vp := frame.Viewport{Offset: frame.Point{X: 0, Y: 100}, Size: frame.Size{W: 800, H: 600}}
	if got, want := vp.Rect(), frame.Rect4(0, 100, 800, 700); got != want {
		t.Fatalf("viewport rect = %v, want %v", got, want)
	}
	if !vp.Contains(frame.Point{X: 10, Y: 150}) {
		t.Fatal("point inside viewport reported as outside")
	}
	if vp.Contains(frame.Point{X: 10, Y: 700}) {
		t.Fatal("first row below the viewport reported as inside")
	}
	wantPoint := frame.Point{X: 3, Y: 7}
	if got := (frame.PointF{X: 3.4, Y: 6.6}).ToDevice(1); got != wantPoint {
		t.Fatalf("point ToDevice = %v, want %v (nearest pixel)", got, wantPoint)
	}
	wantSub := frame.Point{X: 3, Y: -4}
	if got := (frame.Point{X: 5, Y: 5}).Sub(frame.Point{X: 2, Y: 9}); got != wantSub {
		t.Fatalf("Sub = %v, want %v", got, wantSub)
	}
}
