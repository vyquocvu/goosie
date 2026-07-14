package raster

import (
	"image"
	"image/color"
	"testing"
	"math/rand"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// ---------------------------------------------------------------------------
// FrameBuffer tests
// ---------------------------------------------------------------------------

func TestNewFrameBuffer(t *testing.T) {
	fb := NewFrameBuffer(100, 50)
	b := fb.Bounds()
	if b.Dx() != 100 || b.Dy() != 50 {
		t.Errorf("bounds = %v, want 100x50", b)
	}
}

func TestFrameBufferZeroSize(t *testing.T) {
	fb := NewFrameBuffer(0, 0)
	b := fb.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 {
		t.Errorf("zero-size buffer should clamp to at least 1x1, got %v", b)
	}
}

func TestFrameBufferReset(t *testing.T) {
	fb := NewFrameBuffer(10, 10)
	// Write a pixel.
	fb.img.SetRGBA(5, 5, color.RGBA{R: 255, A: 255})
	fb.Reset()
	r, g, b, a := fb.img.At(5, 5).RGBA()
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("Reset should clear pixels, got (%d,%d,%d,%d)", r, g, b, a)
	}
}

func TestFrameBufferResizeSameSize(t *testing.T) {
	fb := NewFrameBuffer(100, 50)
	img1 := fb.Image()
	resized := fb.Resize(100, 50)
	if resized {
		t.Error("Resize to same size should return false")
	}
	if fb.Image() != img1 {
		t.Error("Resize to same size should not reallocate")
	}
}

func TestFrameBufferResizeDifferentSize(t *testing.T) {
	fb := NewFrameBuffer(100, 50)
	resized := fb.Resize(200, 100)
	if !resized {
		t.Error("Resize to different size should return true")
	}
	b := fb.Bounds()
	if b.Dx() != 200 || b.Dy() != 100 {
		t.Errorf("after resize bounds = %v, want 200x100", b)
	}
}

// ---------------------------------------------------------------------------
// CPUBackend lifecycle tests
// ---------------------------------------------------------------------------

func TestCPUBackendBeginEndFrame(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	if err := b.BeginFrame(vp); err != nil {
		t.Fatal(err)
	}
	if err := b.EndFrame(); err != nil {
		t.Fatal(err)
	}
}

func TestCPUBackendClose(t *testing.T) {
	b := NewCPUBackend(100, 100)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	// Operations after close should error.
	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	if err := b.BeginFrame(vp); err == nil {
		t.Error("BeginFrame after Close should error")
	}
}

func TestCPUBackendDoubleClose(t *testing.T) {
	b := NewCPUBackend(100, 100)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal("second Close should not error")
	}
}

// ---------------------------------------------------------------------------
// Fill rasterization tests
// ---------------------------------------------------------------------------

func TestCPUBackendFillSolid(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []DisplayCmd{
		{
			Kind:  CmdFill,
			Rect:  frame.Rect{X: 10, Y: 10, W: 20, H: 20},
			Color: frame.NewColor(255, 0, 0, 255),
		},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Check pixel inside the filled rect.
	r, g, bb, a := img.At(15, 15).RGBA()
	if r>>8 != 255 || g>>8 != 0 || bb>>8 != 0 || a>>8 != 255 {
		t.Errorf("inside fill = (%d,%d,%d,%d), want red", r>>8, g>>8, bb>>8, a>>8)
	}

	// Check pixel outside the filled rect.
	r, g, bb, a = img.At(50, 50).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("outside fill = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

func TestCPUBackendFillWithDirtyRegion(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	// Fill a large rect but only dirty a small region.
	cmds := []DisplayCmd{
		{
			Kind:  CmdFill,
			Rect:  frame.Rect{X: 0, Y: 0, W: 100, H: 100},
			Color: frame.NewColor(0, 255, 0, 255),
		},
	}
	dirty := []frame.Rect{{X: 10, Y: 10, W: 5, H: 5}}
	img, err := b.Rasterize(cmds, dirty)
	if err != nil {
		t.Fatal(err)
	}

	// Inside dirty region should be green.
	r, g, bb, a := img.At(12, 12).RGBA()
	if r>>8 != 0 || g>>8 != 255 || bb>>8 != 0 || a>>8 != 255 {
		t.Errorf("inside dirty = (%d,%d,%d,%d), want green", r>>8, g>>8, bb>>8, a>>8)
	}

	// Outside dirty region should be transparent (not rasterized).
	r, g, bb, a = img.At(80, 80).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("outside dirty = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

func TestCPUBackendFillClipped(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []DisplayCmd{
		{Kind: CmdClipPush, Rect: frame.Rect{X: 20, Y: 20, W: 30, H: 30}},
		{
			Kind:  CmdFill,
			Rect:  frame.Rect{X: 0, Y: 0, W: 100, H: 100},
			Color: frame.NewColor(0, 0, 255, 255),
		},
		{Kind: CmdClipPop},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Inside clip should be blue.
	r, g, bb, a := img.At(25, 25).RGBA()
	if r>>8 != 0 || g>>8 != 0 || bb>>8 != 255 || a>>8 != 255 {
		t.Errorf("inside clip = (%d,%d,%d,%d), want blue", r>>8, g>>8, bb>>8, a>>8)
	}

	// Outside clip should be transparent.
	r, g, bb, a = img.At(5, 5).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("outside clip = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

func TestCPUBackendFillWithOpacity(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []DisplayCmd{
		{Kind: CmdOpacityPush, Opacity: 0.5},
		{
			Kind:  CmdFill,
			Rect:  frame.Rect{X: 0, Y: 0, W: 50, H: 50},
			Color: frame.NewColor(255, 0, 0, 255),
		},
		{Kind: CmdOpacityPop},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Pixel should be red at ~50% opacity.
	_, _, _, a := img.At(25, 25).RGBA()
	alpha := a >> 8
	if alpha < 120 || alpha > 135 {
		t.Errorf("opacity pixel alpha = %d, want ~128", alpha)
	}
}

// ---------------------------------------------------------------------------
// Border rasterization tests
// ---------------------------------------------------------------------------

func TestCPUBackendBorderTop(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []DisplayCmd{
		{
			Kind: CmdBorder,
			Rect: frame.Rect{X: 10, Y: 10, W: 50, H: 50},
			Border: BorderSpec{
				Top: SideSpec{Width: 3, Color: frame.NewColor(255, 0, 0, 255)},
			},
		},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Top border at y=10 should be red.
	r, g, bb, a := img.At(20, 10).RGBA()
	if r>>8 != 255 || g>>8 != 0 || bb>>8 != 0 || a>>8 != 255 {
		t.Errorf("top border = (%d,%d,%d,%d), want red", r>>8, g>>8, bb>>8, a>>8)
	}

	// Below top border at y=15 should be transparent.
	r, g, bb, a = img.At(20, 15).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("below top border = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

func TestCPUBackendBorderAllSides(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	red := frame.NewColor(255, 0, 0, 255)
	cmds := []DisplayCmd{
		{
			Kind: CmdBorder,
			Rect: frame.Rect{X: 10, Y: 10, W: 50, H: 50},
			Border: BorderSpec{
				Top:    SideSpec{Width: 2, Color: red},
				Right:  SideSpec{Width: 2, Color: red},
				Bottom: SideSpec{Width: 2, Color: red},
				Left:   SideSpec{Width: 2, Color: red},
			},
		},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Check all four edges.
	tests := []struct {
		x, y int
		desc string
	}{
		{20, 10, "top"},
		{20, 58, "bottom"},
		{10, 20, "left"},
		{58, 20, "right"},
	}
	for _, tt := range tests {
		r, g, bb, a := img.At(tt.x, tt.y).RGBA()
		if r>>8 != 255 || g>>8 != 0 || bb>>8 != 0 || a>>8 != 255 {
			t.Errorf("%s border at (%d,%d) = (%d,%d,%d,%d), want red",
				tt.desc, tt.x, tt.y, r>>8, g>>8, bb>>8, a>>8)
		}
	}

	// Center should be transparent (no fill).
	r, g, bb, a := img.At(30, 30).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("center = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

// ---------------------------------------------------------------------------
// Nested clip tests
// ---------------------------------------------------------------------------

func TestCPUBackendNestedClips(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []DisplayCmd{
		{Kind: CmdClipPush, Rect: frame.Rect{X: 0, Y: 0, W: 80, H: 80}},
		{Kind: CmdClipPush, Rect: frame.Rect{X: 20, Y: 20, W: 40, H: 40}},
		{
			Kind:  CmdFill,
			Rect:  frame.Rect{X: 0, Y: 0, W: 100, H: 100},
			Color: frame.NewColor(0, 255, 0, 255),
		},
		{Kind: CmdClipPop},
		{Kind: CmdClipPop},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Inside both clips.
	r, g, bb, a := img.At(30, 30).RGBA()
	if g>>8 != 255 {
		t.Errorf("inside both clips = (%d,%d,%d,%d), want green", r>>8, g>>8, bb>>8, a>>8)
	}

	// Inside outer but outside inner clip.
	r, g, bb, a = img.At(5, 5).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("outside inner clip = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

// ---------------------------------------------------------------------------
// HiDPI tests
// ---------------------------------------------------------------------------

func TestCPUBackendHiDPI(t *testing.T) {
	b := NewCPUBackend(200, 200)
	defer b.Close()

	ps := frame.PixelScale{Scale: 2.0, DPI: 192}
	vp := frame.NewViewport(100, 100, ps)
	b.BeginFrame(vp)

	cmds := []DisplayCmd{
		{
			Kind:  CmdFill,
			Rect:  frame.Rect{X: 0, Y: 0, W: 200, H: 200},
			Color: frame.NewColor(128, 128, 128, 255),
		},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Device buffer should be 200x200.
	rgba := img.(*image.RGBA)
	bounds := rgba.Bounds()
	if bounds.Dx() != 200 || bounds.Dy() != 200 {
		t.Errorf("HiDPI buffer = %v, want 200x200", bounds)
	}
}

// ---------------------------------------------------------------------------
// Empty command list tests
// ---------------------------------------------------------------------------

func TestCPUBackendEmptyCommands(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	img, err := b.Rasterize(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if img == nil {
		t.Fatal("Rasterize(nil) should return non-nil image")
	}

	// All pixels should be transparent.
	r, g, bb, a := img.At(50, 50).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("empty frame pixel = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

// ---------------------------------------------------------------------------
// Blend pixel tests
// ---------------------------------------------------------------------------

func TestBlendPixelOpaque(t *testing.T) {
	pix := make([]byte, 4)
	src := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	blendPixel(pix, 0, src)
	if pix[0] != 255 || pix[1] != 0 || pix[2] != 0 || pix[3] != 255 {
		t.Errorf("opaque blend = (%d,%d,%d,%d), want red", pix[0], pix[1], pix[2], pix[3])
	}
}

func TestBlendPixelTransparent(t *testing.T) {
	pix := []byte{100, 100, 100, 255}
	src := color.RGBA{R: 0, G: 0, B: 0, A: 0}
	blendPixel(pix, 0, src)
	// Should not change.
	if pix[0] != 100 || pix[1] != 100 || pix[2] != 100 || pix[3] != 255 {
		t.Errorf("transparent blend changed pixel: %v", pix)
	}
}

func TestBlendPixelHalfAlpha(t *testing.T) {
	pix := []byte{0, 0, 0, 255} // black opaque background
	src := color.RGBA{R: 255, G: 255, B: 255, A: 128}
	blendPixel(pix, 0, src)
	// Should be roughly 128,128,128,255.
	if pix[0] < 120 || pix[0] > 135 {
		t.Errorf("half-alpha R = %d, want ~128", pix[0])
	}
}

// ---------------------------------------------------------------------------
// applyOpacity tests
// ---------------------------------------------------------------------------

func TestApplyOpacityFull(t *testing.T) {
	c := frame.NewColor(255, 0, 0, 255)
	got := applyOpacity(c, 1.0)
	if got != c {
		t.Errorf("opacity 1.0 should return same color")
	}
}

func TestApplyOpacityZero(t *testing.T) {
	c := frame.NewColor(255, 0, 0, 255)
	got := applyOpacity(c, 0.0)
	if !got.IsFullyTransparent() {
		t.Errorf("opacity 0.0 should be fully transparent")
	}
}

func TestApplyOpacityHalf(t *testing.T) {
	c := frame.NewColor(255, 0, 0, 200)
	got := applyOpacity(c, 0.5)
	if got.A() < 95 || got.A() > 105 {
		t.Errorf("opacity 0.5 on alpha=200 = %d, want ~100", got.A())
	}
	if got.R() != 255 {
		t.Error("opacity should not change RGB")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkFillRect100x100(b *testing.B) {
	backend := NewCPUBackend(400, 400)
	vp := frame.NewViewport(400, 400, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	cmds := []DisplayCmd{
		{Kind: CmdFill, Rect: frame.Rect{X: 50, Y: 50, W: 100, H: 100}, Color: frame.NewColor(255, 0, 0, 255)},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkFillRectFullFrame(b *testing.B) {
	backend := NewCPUBackend(800, 600)
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	cmds := []DisplayCmd{
		{Kind: CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 800, H: 600}, Color: frame.NewColor(200, 200, 200, 255)},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkFillWithClip(b *testing.B) {
	backend := NewCPUBackend(400, 400)
	vp := frame.NewViewport(400, 400, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	cmds := []DisplayCmd{
		{Kind: CmdClipPush, Rect: frame.Rect{X: 50, Y: 50, W: 200, H: 200}},
		{Kind: CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 400, H: 400}, Color: frame.NewColor(0, 255, 0, 255)},
		{Kind: CmdClipPop},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkBorderAllSides(b *testing.B) {
	backend := NewCPUBackend(400, 400)
	vp := frame.NewViewport(400, 400, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	red := frame.NewColor(255, 0, 0, 255)
	cmds := []DisplayCmd{
		{
			Kind: CmdBorder,
			Rect: frame.Rect{X: 50, Y: 50, W: 200, H: 150},
			Border: BorderSpec{
				Top:    SideSpec{Width: 2, Color: red},
				Right:  SideSpec{Width: 2, Color: red},
				Bottom: SideSpec{Width: 2, Color: red},
				Left:   SideSpec{Width: 2, Color: red},
			},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkDirtyRegionSmall(b *testing.B) {
	backend := NewCPUBackend(800, 600)
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	cmds := []DisplayCmd{
		{Kind: CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 800, H: 600}, Color: frame.NewColor(200, 200, 200, 255)},
	}
	dirty := []frame.Rect{{X: 100, Y: 100, W: 50, H: 50}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, dirty)
	}
}

func BenchmarkBlendPixel(b *testing.B) {
	pix := make([]byte, 4)
	src := color.RGBA{R: 128, G: 64, B: 32, A: 200}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pix[0], pix[1], pix[2], pix[3] = 0, 0, 0, 0
		blendPixel(pix, 0, src)
	}
}

func TestCPUBackendRasterText(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	textRun := frame.TextRun{
		Font:     0,
		FontSize: 16,
		Color:    frame.NewColor(0, 0, 255, 255), // Blue
		Glyphs: []frame.Glyph{
			{ID: 65, Advance: 8, XOffset: 0, YOffset: 0},
		},
	}

	cmds := []DisplayCmd{
		{
			Kind:    CmdText,
			Rect:    frame.Rect{X: 10, Y: 10, W: 20, H: 20},
			TextRun: textRun,
		},
	}

	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	var blueFound bool
	for y := 10; y < 30; y++ {
		for x := 10; x < 30; x++ {
			_, _, bl, a := img.At(x, y).RGBA()
			if bl>>8 == 255 && a>>8 == 255 {
				blueFound = true
				break
			}
		}
	}
	if !blueFound {
		t.Error("expected to find some blue pixels from text rendering")
	}
}

func TestCPUBackendRasterSVG(t *testing.T) {
	b := NewCPUBackend(100, 100)
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20">
		<rect x="0" y="0" width="20" height="20" fill="#00ff00"/>
	</svg>`)

	cmds := []DisplayCmd{
		{
			Kind: CmdImage,
			Rect: frame.Rect{X: 10, Y: 10, W: 20, H: 20},
			Image: ImageSpec{
				Data: svgData,
			},
		},
	}

	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	var greenFound bool
	for y := 10; y < 30; y++ {
		for x := 10; x < 30; x++ {
			_, g, _, a := img.At(x, y).RGBA()
			if g>>8 == 255 && a>>8 == 255 {
				greenFound = true
				break
			}
		}
	}
	if !greenFound {
		t.Error("expected to find some green pixels from SVG rendering")
	}
}

func BenchmarkRasterText(b *testing.B) {
	backend := NewCPUBackend(400, 400)
	vp := frame.NewViewport(400, 400, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	textRun := frame.TextRun{
		Font:     0,
		FontSize: 16,
		Color:    frame.NewColor(0, 0, 255, 255),
		Glyphs: []frame.Glyph{
			{ID: 65, Advance: 8, XOffset: 0, YOffset: 0},
			{ID: 66, Advance: 8, XOffset: 8, YOffset: 0},
			{ID: 67, Advance: 8, XOffset: 16, YOffset: 0},
		},
	}
	cmds := []DisplayCmd{
		{Kind: CmdText, Rect: frame.Rect{X: 50, Y: 50, W: 100, H: 100}, TextRun: textRun},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

// ---------------------------------------------------------------------------
// M11.1 CPU Raster Benchmarks
// ---------------------------------------------------------------------------

func benchmarkFill(b *testing.B, w, h int) {
	backend := NewCPUBackend(w, h)
	vp := frame.NewViewport(float32(w), float32(h), frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	cmds := []DisplayCmd{
		{Kind: CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: float32(w), H: float32(h)}, Color: frame.NewColor(255, 0, 0, 255)},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkRasterFillSmall(b *testing.B)  { benchmarkFill(b, 100, 100) }
func BenchmarkRasterFillMedium(b *testing.B) { benchmarkFill(b, 400, 400) }
func BenchmarkRasterFillLarge(b *testing.B)  { benchmarkFill(b, 800, 600) }

func benchmarkText(b *testing.B, glyphCount int) {
	backend := NewCPUBackend(800, 600)
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	glyphs := make([]frame.Glyph, glyphCount)
	for i := range glyphs {
		glyphs[i] = frame.Glyph{
			ID:      uint32(65 + i%26),
			Advance: 8,
			XOffset: float32(i * 8),
			YOffset: 0,
		}
	}
	textRun := frame.TextRun{
		Font:     0,
		FontSize: 16,
		Color:    frame.NewColor(0, 0, 0, 255),
		Glyphs:   glyphs,
	}
	cmds := []DisplayCmd{
		{Kind: CmdText, Rect: frame.Rect{X: 10, Y: 10, W: float32(glyphCount * 8), H: 20}, TextRun: textRun},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkRasterTextShort(b *testing.B)  { benchmarkText(b, 3) }
func BenchmarkRasterTextMedium(b *testing.B) { benchmarkText(b, 50) }
func BenchmarkRasterTextLong(b *testing.B)   { benchmarkText(b, 500) }

func benchmarkImage(b *testing.B, srcW, srcH, dstW, dstH int) {
	backend := NewCPUBackend(800, 600)
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	srcImg := image.NewRGBA(image.Rect(0, 0, srcW, srcH))
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			srcImg.Set(x, y, color.RGBA{uint8(x * 255 / srcW), uint8(y * 255 / srcH), 128, 255})
		}
	}
	cmds := []DisplayCmd{
		{
			Kind: CmdImage,
			Rect: frame.Rect{X: 10, Y: 10, W: float32(dstW), H: float32(dstH)},
			Image: ImageSpec{Img: srcImg},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkRasterImageSmall(b *testing.B)    { benchmarkImage(b, 32, 32, 100, 100) }
func BenchmarkRasterImageMedium(b *testing.B)   { benchmarkImage(b, 256, 256, 200, 200) }
func BenchmarkRasterImageScaleUp(b *testing.B)  { benchmarkImage(b, 32, 32, 400, 400) }
func BenchmarkRasterImageScaleDown(b *testing.B) { benchmarkImage(b, 800, 600, 200, 150) }

func BenchmarkRasterBorderFourSides(b *testing.B) {
	backend := NewCPUBackend(400, 400)
	vp := frame.NewViewport(400, 400, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	red := frame.NewColor(255, 0, 0, 255)
	green := frame.NewColor(0, 255, 0, 255)
	blue := frame.NewColor(0, 0, 255, 255)
	yellow := frame.NewColor(255, 255, 0, 255)
	cmds := []DisplayCmd{
		{
			Kind: CmdBorder,
			Rect: frame.Rect{X: 50, Y: 50, W: 300, H: 300},
			Border: BorderSpec{
				Top:    SideSpec{Width: float32(4), Color: red},
				Right:  SideSpec{Width: float32(4), Color: green},
				Bottom: SideSpec{Width: float32(4), Color: blue},
				Left:   SideSpec{Width: float32(4), Color: yellow},
			},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkRasterClipDepth(b *testing.B) {
	backend := NewCPUBackend(800, 600)
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	cmds := make([]DisplayCmd, 0, 21)
	for i := 0; i < 10; i++ {
		cmds = append(cmds, DisplayCmd{
			Kind: CmdClipPush,
			Rect: frame.Rect{X: float32(10 + i*5), Y: float32(10 + i*5), W: float32(780 - i*10), H: float32(580 - i*10)},
		})
	}
	cmds = append(cmds, DisplayCmd{
		Kind: CmdFill, Color: frame.NewColor(0, 128, 255, 255),
		Rect: frame.Rect{X: 50, Y: 50, W: 700, H: 500},
	})
	for i := 0; i < 10; i++ {
		cmds = append(cmds, DisplayCmd{Kind: CmdClipPop})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkRasterOpacityDepth(b *testing.B) {
	backend := NewCPUBackend(800, 600)
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	cmds := make([]DisplayCmd, 0, 21)
	for i := 0; i < 10; i++ {
		cmds = append(cmds, DisplayCmd{Kind: CmdOpacityPush, Opacity: 0.9})
	}
	cmds = append(cmds, DisplayCmd{
		Kind: CmdFill, Color: frame.NewColor(0, 128, 255, 255),
		Rect: frame.Rect{X: 0, Y: 0, W: 800, H: 600},
	})
	for i := 0; i < 10; i++ {
		cmds = append(cmds, DisplayCmd{Kind: CmdOpacityPop})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkRasterMixedPage(b *testing.B) {
	backend := NewCPUBackend(800, 600)
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	srcImg := image.NewRGBA(image.Rect(0, 0, 200, 150))
	for y := 0; y < 150; y++ {
		for x := 0; x < 200; x++ {
			srcImg.Set(x, y, color.RGBA{128, 200, 255, 255})
		}
	}
	glyphs := make([]frame.Glyph, 100)
	for i := range glyphs {
		glyphs[i] = frame.Glyph{
			ID: uint32(65 + i%26), Advance: 8,
			XOffset: float32(i * 8), YOffset: 0,
		}
	}
	cmds := []DisplayCmd{
		{Kind: CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 800, H: 600}, Color: frame.NewColor(255, 255, 255, 255)},
		{Kind: CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 800, H: 80}, Color: frame.NewColor(50, 50, 50, 255)},
		{Kind: CmdBorder, Rect: frame.Rect{X: 0, Y: 0, W: 800, H: 80}, Border: BorderSpec{Bottom: SideSpec{Width: float32(2), Color: frame.NewColor(200, 200, 200, 255)}}},
		{Kind: CmdText, Rect: frame.Rect{X: 20, Y: 10, W: 800, H: 60}, TextRun: frame.TextRun{Font: 0, FontSize: 24, Color: frame.NewColor(255, 255, 255, 255), Glyphs: glyphs[:10]}},
		{Kind: CmdFill, Rect: frame.Rect{X: 0, Y: 80, W: 200, H: 440}, Color: frame.NewColor(240, 240, 240, 255)},
		{Kind: CmdBorder, Rect: frame.Rect{X: 0, Y: 80, W: 200, H: 440}, Border: BorderSpec{Right: SideSpec{Width: float32(1), Color: frame.NewColor(200, 200, 200, 255)}}},
		{Kind: CmdText, Rect: frame.Rect{X: 220, Y: 100, W: 560, H: 400}, TextRun: frame.TextRun{Font: 0, FontSize: 14, Color: frame.NewColor(0, 0, 0, 255), Glyphs: glyphs}},
		{Kind: CmdImage, Rect: frame.Rect{X: 220, Y: 300, W: 200, H: 150}, Image: ImageSpec{Img: srcImg}},
		{Kind: CmdFill, Rect: frame.Rect{X: 0, Y: 520, W: 800, H: 80}, Color: frame.NewColor(50, 50, 50, 255)},
		{Kind: CmdText, Rect: frame.Rect{X: 20, Y: 540, W: 760, H: 40}, TextRun: frame.TextRun{Font: 0, FontSize: 12, Color: frame.NewColor(200, 200, 200, 255), Glyphs: glyphs[:5]}},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}

func BenchmarkRasterManyCommands(b *testing.B) {
	backend := NewCPUBackend(800, 600)
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	backend.BeginFrame(vp)
	rng := rand.New(rand.NewSource(42))
	cmds := make([]DisplayCmd, 500)
	for i := range cmds {
		x := rng.Float32() * 700
		y := rng.Float32() * 500
		w := rng.Float32()*100 + 10
		h := rng.Float32()*100 + 10
		cmds[i] = DisplayCmd{
			Kind: CmdFill,
			Rect: frame.Rect{X: x, Y: y, W: w, H: h},
			Color: frame.NewColor(uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255),
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.fb.Reset()
		backend.Rasterize(cmds, nil)
	}
}
