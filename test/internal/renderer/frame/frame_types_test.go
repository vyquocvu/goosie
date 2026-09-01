// Package frame defines platform-neutral frame types for the raster backend.
//
// M6.1: These types form the contract between the display list builder and
// any raster backend (CPU, GPU, or Fyne adapter). No backend-specific types
// (e.g. Fyne, image.Image) appear in this package.
package frame_test

import (
	"image/color"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// ---------------------------------------------------------------------------
// Color tests
// ---------------------------------------------------------------------------

func TestNewColor(t *testing.T) {
	c := frame.NewColor(255, 128, 64, 200)
	if c.R() != 255 {
		t.Errorf("R() = %d, want 255", c.R())
	}
	if c.G() != 128 {
		t.Errorf("G() = %d, want 128", c.G())
	}
	if c.B() != 64 {
		t.Errorf("B() = %d, want 64", c.B())
	}
	if c.A() != 200 {
		t.Errorf("A() = %d, want 200", c.A())
	}
}

func TestColorTransparent(t *testing.T) {
	if frame.Transparent != (frame.Color{}) {
		t.Error("frame.Transparent should be zero value")
	}
	if frame.Transparent.A() != 0 {
		t.Error("frame.Transparent alpha should be 0")
	}
}

func TestColorOpaque(t *testing.T) {
	if frame.Opaque.A() != 255 {
		t.Errorf("frame.Opaque alpha = %d, want 255", frame.Opaque.A())
	}
}

func TestColorFromStdColor(t *testing.T) {
	std := color.RGBA{R: 10, G: 20, B: 30, A: 40}
	c := frame.FromStdColor(std)
	if c.R() != 10 || c.G() != 20 || c.B() != 30 || c.A() != 40 {
		t.Errorf("frame.FromStdColor mismatch: %+v", c)
	}
}

func TestColorToStdColor(t *testing.T) {
	c := frame.NewColor(10, 20, 30, 40)
	std := c.StdColor()
	want := color.RGBA{R: 10, G: 20, B: 30, A: 40}
	if std != want {
		t.Errorf("StdColor() = %v, want %v", std, want)
	}
}

func TestColorRoundtrip(t *testing.T) {
	original := frame.NewColor(123, 45, 67, 89)
	roundtrip := frame.FromStdColor(original.StdColor())
	if roundtrip != original {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", roundtrip, original)
	}
}

func TestColorWithAlpha(t *testing.T) {
	c := frame.NewColor(100, 150, 200, 255)
	half := c.WithAlpha(128)
	if half.R() != 100 || half.G() != 150 || half.B() != 200 {
		t.Error("WithAlpha should preserve RGB")
	}
	if half.A() != 128 {
		t.Errorf("WithAlpha alpha = %d, want 128", half.A())
	}
}

func TestColorIsOpaque(t *testing.T) {
	if !frame.NewColor(1, 2, 3, 255).IsOpaque() {
		t.Error("alpha=255 should be opaque")
	}
	if frame.NewColor(1, 2, 3, 254).IsOpaque() {
		t.Error("alpha=254 should not be opaque")
	}
}

func TestColorIsFullyTransparent(t *testing.T) {
	if !frame.NewColor(1, 2, 3, 0).IsFullyTransparent() {
		t.Error("alpha=0 should be fully transparent")
	}
	if frame.NewColor(1, 2, 3, 1).IsFullyTransparent() {
		t.Error("alpha=1 should not be fully transparent")
	}
}

func TestColorLerp(t *testing.T) {
	black := frame.NewColor(0, 0, 0, 0)
	white := frame.NewColor(255, 255, 255, 255)
	mid := frame.LerpColor(black, white, 0.5)
	// Allow ±1 rounding tolerance per channel
	for _, ch := range []uint8{mid.R(), mid.G(), mid.B(), mid.A()} {
		if ch < 126 || ch > 128 {
			t.Errorf("Lerp(0.5) channel = %d, want ~127", ch)
		}
	}
}

func TestColorLerpEndpoints(t *testing.T) {
	a := frame.NewColor(10, 20, 30, 40)
	b := frame.NewColor(200, 100, 50, 255)
	start := frame.LerpColor(a, b, 0.0)
	end := frame.LerpColor(a, b, 1.0)
	if start != a {
		t.Errorf("Lerp(t=0) = %+v, want %+v", start, a)
	}
	if end != b {
		t.Errorf("Lerp(t=1) = %+v, want %+v", end, b)
	}
}

func TestColorPacking(t *testing.T) {
	// Verify that packing is consistent: same RGBA → same internal representation.
	a := frame.NewColor(1, 2, 3, 4)
	b := frame.NewColor(1, 2, 3, 4)
	if a != b {
		t.Error("identical colors should be equal")
	}
	// Different colors → different representation.
	c := frame.NewColor(5, 6, 7, 8)
	if a == c {
		t.Error("different colors should not be equal")
	}
}

// ---------------------------------------------------------------------------
// Point tests
// ---------------------------------------------------------------------------

func TestPointZero(t *testing.T) {
	p := frame.PointZero
	if p.X != 0 || p.Y != 0 {
		t.Errorf("frame.PointZero = %+v, want {0,0}", p)
	}
}

func TestPointNew(t *testing.T) {
	p := frame.NewPoint(3.5, -7.25)
	if p.X != 3.5 || p.Y != -7.25 {
		t.Errorf("frame.NewPoint = %+v", p)
	}
}

func TestPointAdd(t *testing.T) {
	a := frame.NewPoint(1, 2)
	b := frame.NewPoint(3, 4)
	sum := a.Add(b)
	if sum.X != 4 || sum.Y != 6 {
		t.Errorf("Add = %+v, want {4,6}", sum)
	}
}

func TestPointSub(t *testing.T) {
	a := frame.NewPoint(5, 10)
	b := frame.NewPoint(2, 3)
	diff := a.Sub(b)
	if diff.X != 3 || diff.Y != 7 {
		t.Errorf("Sub = %+v, want {3,7}", diff)
	}
}

func TestPointScale(t *testing.T) {
	p := frame.NewPoint(2, 3).Scale(2.5)
	if p.X != 5 || p.Y != 7.5 {
		t.Errorf("Scale = %+v, want {5,7.5}", p)
	}
}

func TestPointDistanceTo(t *testing.T) {
	a := frame.NewPoint(0, 0)
	b := frame.NewPoint(3, 4)
	d := a.DistanceTo(b)
	if d < 4.99 || d > 5.01 {
		t.Errorf("DistanceTo = %f, want ~5.0", d)
	}
}

func TestPointEqual(t *testing.T) {
	a := frame.NewPoint(1, 2)
	b := frame.NewPoint(1, 2)
	c := frame.NewPoint(1, 3)
	if !a.Equal(b) {
		t.Error("identical points should be equal")
	}
	if a.Equal(c) {
		t.Error("different points should not be equal")
	}
}

// ---------------------------------------------------------------------------
// Rect tests
// ---------------------------------------------------------------------------

func TestRectNew(t *testing.T) {
	r := frame.NewRect(10, 20, 100, 50)
	if r.X != 10 || r.Y != 20 || r.W != 100 || r.H != 50 {
		t.Errorf("frame.NewRect = %+v", r)
	}
}

func TestRectZero(t *testing.T) {
	r := frame.RectZero
	if r.X != 0 || r.Y != 0 || r.W != 0 || r.H != 0 {
		t.Errorf("frame.RectZero = %+v", r)
	}
}

func TestRectMaxX(t *testing.T) {
	r := frame.NewRect(10, 20, 100, 50)
	if r.MaxX() != 110 {
		t.Errorf("MaxX() = %f, want 110", r.MaxX())
	}
}

func TestRectMaxY(t *testing.T) {
	r := frame.NewRect(10, 20, 100, 50)
	if r.MaxY() != 70 {
		t.Errorf("MaxY() = %f, want 70", r.MaxY())
	}
}

func TestRectContains(t *testing.T) {
	r := frame.NewRect(0, 0, 100, 100)
	tests := []struct {
		p    frame.Point
		want bool
	}{
		{frame.NewPoint(50, 50), true},
		{frame.NewPoint(0, 0), true},
		{frame.NewPoint(99.9, 99.9), true},
		{frame.NewPoint(100, 100), false}, // exclusive upper bound
		{frame.NewPoint(-1, 50), false},
		{frame.NewPoint(50, -1), false},
	}
	for _, tt := range tests {
		if got := r.Contains(tt.p); got != tt.want {
			t.Errorf("Contains(%+v) = %v, want %v", tt.p, got, tt.want)
		}
	}
}

func TestRectIntersects(t *testing.T) {
	r := frame.NewRect(0, 0, 100, 100)
	tests := []struct {
		other frame.Rect
		want  bool
	}{
		{frame.NewRect(50, 50, 100, 100), true},   // overlap
		{frame.NewRect(100, 0, 50, 50), false},    // touching edge
		{frame.NewRect(-50, -50, 50, 50), false},  // touching edge
		{frame.NewRect(10, 10, 20, 20), true},     // fully inside
		{frame.NewRect(-10, -10, 200, 200), true}, // fully enclosing
	}
	for _, tt := range tests {
		if got := r.Intersects(tt.other); got != tt.want {
			t.Errorf("Intersects(%+v) = %v, want %v", tt.other, got, tt.want)
		}
	}
}

func TestRectIntersection(t *testing.T) {
	a := frame.NewRect(0, 0, 100, 100)
	b := frame.NewRect(50, 50, 100, 100)
	got := a.Intersection(b)
	want := frame.NewRect(50, 50, 50, 50)
	if got != want {
		t.Errorf("Intersection = %+v, want %+v", got, want)
	}
}

func TestRectIntersectionEmpty(t *testing.T) {
	a := frame.NewRect(0, 0, 10, 10)
	b := frame.NewRect(20, 20, 10, 10)
	got := a.Intersection(b)
	if got != frame.RectZero {
		t.Errorf("non-overlapping Intersection = %+v, want frame.RectZero", got)
	}
}

func TestRectUnion(t *testing.T) {
	a := frame.NewRect(10, 10, 20, 20)
	b := frame.NewRect(50, 50, 20, 20)
	got := a.Union(b)
	want := frame.NewRect(10, 10, 60, 60)
	if got != want {
		t.Errorf("Union = %+v, want %+v", got, want)
	}
}

func TestRectExpand(t *testing.T) {
	r := frame.NewRect(10, 10, 20, 20).Expand(5)
	want := frame.NewRect(5, 5, 30, 30)
	if r != want {
		t.Errorf("Expand = %+v, want %+v", r, want)
	}
}

func TestRectIsEmpty(t *testing.T) {
	if !frame.RectZero.IsEmpty() {
		t.Error("frame.RectZero should be empty")
	}
	if !frame.NewRect(0, 0, 0, 10).IsEmpty() {
		t.Error("zero-width rect should be empty")
	}
	if frame.NewRect(0, 0, 1, 1).IsEmpty() {
		t.Error("1x1 rect should not be empty")
	}
}

func TestRectEqual(t *testing.T) {
	a := frame.NewRect(1, 2, 3, 4)
	b := frame.NewRect(1, 2, 3, 4)
	c := frame.NewRect(1, 2, 3, 5)
	if !a.Equal(b) {
		t.Error("identical rects should be equal")
	}
	if a.Equal(c) {
		t.Error("different rects should not be equal")
	}
}

// ---------------------------------------------------------------------------
// Handle tests
// ---------------------------------------------------------------------------

func TestImageHandleZero(t *testing.T) {
	var h frame.ImageHandle
	if h.Valid() {
		t.Error("zero frame.ImageHandle should not be valid")
	}
}

func TestImageHandleValid(t *testing.T) {
	h := frame.ImageHandle(42)
	if !h.Valid() {
		t.Error("non-zero frame.ImageHandle should be valid")
	}
}

func TestFontHandleZero(t *testing.T) {
	var h frame.FontHandle
	if h.Valid() {
		t.Error("zero frame.FontHandle should not be valid")
	}
}

func TestFontHandleValid(t *testing.T) {
	h := frame.FontHandle(7)
	if !h.Valid() {
		t.Error("non-zero frame.FontHandle should be valid")
	}
}

func TestHandleEquality(t *testing.T) {
	a := frame.ImageHandle(1)
	b := frame.ImageHandle(1)
	c := frame.ImageHandle(2)
	if a != b {
		t.Error("same handles should be equal")
	}
	if a == c {
		t.Error("different handles should not be equal")
	}
}

// ---------------------------------------------------------------------------
// Glyph and TextRun tests
// ---------------------------------------------------------------------------

func TestGlyphZero(t *testing.T) {
	g := frame.Glyph{}
	if g.ID != 0 || g.Advance != 0 || g.XOffset != 0 || g.YOffset != 0 {
		t.Errorf("zero Glyph = %+v", g)
	}
}

func TestTextRunEmpty(t *testing.T) {
	tr := frame.TextRun{}
	if tr.Len() != 0 {
		t.Errorf("empty TextRun Len() = %d, want 0", tr.Len())
	}
	if tr.Width() != 0 {
		t.Errorf("empty TextRun Width() = %f, want 0", tr.Width())
	}
}

func TestTextRunWithGlyphs(t *testing.T) {
	glyphs := []frame.Glyph{
		{ID: 1, Advance: 8.0},
		{ID: 2, Advance: 6.5},
		{ID: 3, Advance: 7.0},
	}
	tr := frame.TextRun{
		Font:     frame.FontHandle(1),
		FontSize: 16,
		Color:    frame.NewColor(0, 0, 0, 255),
		Glyphs:   glyphs,
	}
	if tr.Len() != 3 {
		t.Errorf("Len() = %d, want 3", tr.Len())
	}
	want := float32(8.0 + 6.5 + 7.0)
	if tr.Width() != want {
		t.Errorf("Width() = %f, want %f", tr.Width(), want)
	}
}

func TestTextRunIsEmpty(t *testing.T) {
	empty := frame.TextRun{}
	nonEmpty := frame.TextRun{Glyphs: []frame.Glyph{{ID: 1, Advance: 5}}}
	if !empty.IsEmpty() {
		t.Error("empty TextRun should report IsEmpty")
	}
	if nonEmpty.IsEmpty() {
		t.Error("non-empty TextRun should not report IsEmpty")
	}
}

// ---------------------------------------------------------------------------
// PixelScale tests
// ---------------------------------------------------------------------------

func TestPixelScaleDefault(t *testing.T) {
	ps := frame.PixelScaleDefault
	if ps.Scale != 1.0 {
		t.Errorf("default scale = %f, want 1.0", ps.Scale)
	}
	if ps.DPI != 96 {
		t.Errorf("default DPI = %f, want 96", ps.DPI)
	}
}

func TestPixelScaleFromDPI(t *testing.T) {
	ps := frame.PixelScaleFromDPI(192)
	if ps.DPI != 192 {
		t.Errorf("DPI = %f, want 192", ps.DPI)
	}
	if ps.Scale != 2.0 {
		t.Errorf("Scale = %f, want 2.0", ps.Scale)
	}
}

func TestPixelScaleFromDPIZero(t *testing.T) {
	ps := frame.PixelScaleFromDPI(0)
	if ps.DPI != 96 {
		t.Errorf("zero DPI should fallback to 96, got %f", ps.DPI)
	}
	if ps.Scale != 1.0 {
		t.Errorf("zero DPI scale should be 1.0, got %f", ps.Scale)
	}
}

func TestPixelScaleFromDPINegative(t *testing.T) {
	ps := frame.PixelScaleFromDPI(-10)
	if ps.DPI != 96 {
		t.Errorf("negative DPI should fallback to 96, got %f", ps.DPI)
	}
}

func TestPixelScaleToPixels(t *testing.T) {
	ps := frame.PixelScale{Scale: 2.0, DPI: 192}
	if got := ps.ToPixels(10); got != 20 {
		t.Errorf("ToPixels(10) = %f, want 20", got)
	}
}

func TestPixelScaleToDevicePixels(t *testing.T) {
	ps := frame.PixelScale{Scale: 2.0, DPI: 192}
	dp := ps.ToDevicePixels(10)
	if dp != 20 {
		t.Errorf("ToDevicePixels(10) = %d, want 20", dp)
	}
}

func TestPixelScaleFromDevicePixels(t *testing.T) {
	ps := frame.PixelScale{Scale: 2.0, DPI: 192}
	lp := ps.FromDevicePixels(20)
	if lp != 10 {
		t.Errorf("FromDevicePixels(20) = %f, want 10", lp)
	}
}

// ---------------------------------------------------------------------------
// Viewport tests
// ---------------------------------------------------------------------------

func TestViewportNew(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	if v.Width != 800 || v.Height != 600 {
		t.Errorf("Viewport size = %fx%f, want 800x600", v.Width, v.Height)
	}
	if v.ScrollX != 0 || v.ScrollY != 0 {
		t.Errorf("Viewport scroll = %+v,%+v, want 0,0", v.ScrollX, v.ScrollY)
	}
	if v.PixelScale.Scale != 1.0 {
		t.Errorf("Viewport pixel scale = %f, want 1.0", v.PixelScale.Scale)
	}
}

func TestViewportZeroSize(t *testing.T) {
	v := frame.NewViewport(0, 0, frame.PixelScaleDefault)
	if v.Width != 0 || v.Height != 0 {
		t.Errorf("zero viewport should have 0 size, got %fx%f", v.Width, v.Height)
	}
}

func TestViewportNegativeSize(t *testing.T) {
	v := frame.NewViewport(-100, -200, frame.PixelScaleDefault)
	if v.Width != 0 || v.Height != 0 {
		t.Errorf("negative viewport should clamp to 0, got %fx%f", v.Width, v.Height)
	}
}

func TestViewportWithScroll(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScaleDefault).WithScroll(0, 100)
	if v.ScrollY != 100 {
		t.Errorf("WithScroll ScrollY = %f, want 100", v.ScrollY)
	}
	if v.Width != 800 {
		t.Error("WithScroll should preserve width")
	}
}

func TestViewportWithNegativeScroll(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScaleDefault).WithScroll(-10, -20)
	if v.ScrollX != 0 || v.ScrollY != 0 {
		t.Errorf("negative scroll should clamp to 0, got (%f,%f)", v.ScrollX, v.ScrollY)
	}
}

func TestViewportDeviceSize(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScale{Scale: 2.0, DPI: 192})
	dw, dh := v.DeviceSize()
	if dw != 1600 || dh != 1200 {
		t.Errorf("DeviceSize = (%d,%d), want (1600,1200)", dw, dh)
	}
}

func TestViewportVisibleRect(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScaleDefault).WithScroll(0, 100)
	vr := v.VisibleRect()
	if vr.X != 0 || vr.Y != 100 || vr.W != 800 || vr.H != 600 {
		t.Errorf("VisibleRect = %+v, want {0,100,800,600}", vr)
	}
}

func TestViewportEqual(t *testing.T) {
	a := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	b := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	c := frame.NewViewport(1024, 768, frame.PixelScaleDefault)
	if !a.Equal(b) {
		t.Error("identical viewports should be equal")
	}
	if a.Equal(c) {
		t.Error("different viewports should not be equal")
	}
}

// ---------------------------------------------------------------------------
// FrameSnapshot tests
// ---------------------------------------------------------------------------

func TestFrameSnapshotNew(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	snap := frame.NewFrameSnapshot(1, v, nil)
	if snap.Generation != 1 {
		t.Errorf("Generation = %d, want 1", snap.Generation)
	}
	if !snap.Viewport.Equal(v) {
		t.Errorf("Viewport mismatch")
	}
	if snap.CommandCount != 0 {
		t.Errorf("CommandCount = %d, want 0", snap.CommandCount)
	}
}

func TestFrameSnapshotImmutable(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	snap := frame.NewFrameSnapshot(1, v, nil)
	// Attempting to read fields should be safe from any goroutine.
	if snap.Generation != 1 {
		t.Error("immutable snapshot generation changed")
	}
	if snap.Viewport.Width != 800 {
		t.Error("immutable snapshot viewport changed")
	}
}

func TestFrameSnapshotBounds(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScaleDefault).WithScroll(0, 100)
	snap := frame.NewFrameSnapshot(5, v, nil)
	bounds := snap.Bounds()
	if bounds.X != 0 || bounds.Y != 100 || bounds.W != 800 || bounds.H != 600 {
		t.Errorf("Bounds = %+v, want {0,100,800,600}", bounds)
	}
}

func TestFrameSnapshotDeviceBounds(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScale{Scale: 2.0, DPI: 192})
	snap := frame.NewFrameSnapshot(1, v, nil)
	dw, dh := snap.DeviceBounds()
	if dw != 1600 || dh != 1200 {
		t.Errorf("DeviceBounds = (%d,%d), want (1600,1200)", dw, dh)
	}
}

func TestFrameSnapshotGenerationOrdering(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	s1 := frame.NewFrameSnapshot(1, v, nil)
	s2 := frame.NewFrameSnapshot(2, v, nil)
	s3 := frame.NewFrameSnapshot(3, v, nil)
	if !s2.IsNewerThan(s1) {
		t.Error("gen 2 should be newer than gen 1")
	}
	if !s3.IsNewerThan(s2) {
		t.Error("gen 3 should be newer than gen 2")
	}
	if s1.IsNewerThan(s2) {
		t.Error("gen 1 should not be newer than gen 2")
	}
	if s2.IsNewerThan(s2) {
		t.Error("same generation should not be newer")
	}
}

func TestFrameSnapshotContentHash(t *testing.T) {
	v := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	s1 := frame.NewFrameSnapshot(1, v, nil)
	s2 := frame.NewFrameSnapshot(2, v, nil)
	// Same viewport, same (empty) commands → same hash.
	if s1.ContentHash != s2.ContentHash {
		t.Error("identical content should produce same hash")
	}
}

func TestFrameSnapshotContentHashDiffers(t *testing.T) {
	v1 := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	v2 := frame.NewViewport(1024, 768, frame.PixelScaleDefault)
	s1 := frame.NewFrameSnapshot(1, v1, nil)
	s2 := frame.NewFrameSnapshot(2, v2, nil)
	if s1.ContentHash == s2.ContentHash {
		t.Error("different viewports should produce different hash")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkColorCreation(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = frame.NewColor(uint8(i), uint8(i>>1), uint8(i>>2), 255)
	}
}

func BenchmarkColorToStd(b *testing.B) {
	c := frame.NewColor(128, 64, 32, 255)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = c.StdColor()
	}
}

func BenchmarkRectContains(b *testing.B) {
	r := frame.NewRect(0, 0, 800, 600)
	p := frame.NewPoint(400, 300)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.Contains(p)
	}
}

func BenchmarkRectIntersects(b *testing.B) {
	r := frame.NewRect(0, 0, 800, 600)
	other := frame.NewRect(400, 300, 800, 600)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.Intersects(other)
	}
}

func BenchmarkPixelScaleToPixels(b *testing.B) {
	ps := frame.PixelScale{Scale: 2.0, DPI: 192}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ps.ToPixels(100)
	}
}

func BenchmarkFrameSnapshotCreation(b *testing.B) {
	v := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = frame.NewFrameSnapshot(uint64(i), v, nil)
	}
}

func BenchmarkTextRunWidth(b *testing.B) {
	glyphs := make([]frame.Glyph, 50)
	for i := range glyphs {
		glyphs[i] = frame.Glyph{ID: uint32(i), Advance: 8.0}
	}
	tr := frame.TextRun{Font: frame.FontHandle(1), FontSize: 16, Glyphs: glyphs}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = tr.Width()
	}
}
