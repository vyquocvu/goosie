//go:build darwin && cgo

package raster_test

import (
	"fmt"
	"image"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func TestCGBackendBeginEndFrame(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	if err := b.BeginFrame(vp); err != nil {
		t.Fatal(err)
	}
	if err := b.EndFrame(); err != nil {
		t.Fatal(err)
	}
}

func TestCGBackendClose(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	if err := b.BeginFrame(vp); err == nil {
		t.Error("BeginFrame after Close should error")
	}
}

func TestCGBackendDoubleClose(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal("second Close should not error")
	}
}

func TestCGBackendRepeatedFrames(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	for i := 0; i < 5; i++ {
		if err := b.BeginFrame(vp); err != nil {
			t.Fatalf("BeginFrame iteration %d: %v", i, err)
		}
		if _, err := b.Rasterize(nil, nil); err != nil {
			t.Fatalf("Rasterize iteration %d: %v", i, err)
		}
		if err := b.EndFrame(); err != nil {
			t.Fatalf("EndFrame iteration %d: %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Fill
// ---------------------------------------------------------------------------

func TestCGBackendFillSolid(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 10, Y: 10, W: 20, H: 20}, Color: frame.NewColor(255, 0, 0, 255)},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	r, g, bb, a := img.At(15, 15).RGBA()
	if r>>8 != 255 || g>>8 != 0 || bb>>8 != 0 || a>>8 != 255 {
		t.Errorf("inside fill = (%d,%d,%d,%d), want red", r>>8, g>>8, bb>>8, a>>8)
	}
	r, g, bb, a = img.At(50, 50).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("outside fill = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

func TestCGBackendFillWithDirtyRegion(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 100, H: 100}, Color: frame.NewColor(0, 255, 0, 255)},
	}
	dirty := []frame.Rect{{X: 10, Y: 10, W: 5, H: 5}}
	img, err := b.Rasterize(cmds, dirty)
	if err != nil {
		t.Fatal(err)
	}

	r, g, bb, a := img.At(12, 12).RGBA()
	if r>>8 != 0 || g>>8 != 255 || bb>>8 != 0 || a>>8 != 255 {
		t.Errorf("inside dirty = (%d,%d,%d,%d), want green", r>>8, g>>8, bb>>8, a>>8)
	}
	r, g, bb, a = img.At(50, 50).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("outside dirty = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

// ---------------------------------------------------------------------------
// Clip
// ---------------------------------------------------------------------------

func TestCGBackendFillClipped(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdClipPush, Rect: frame.Rect{X: 20, Y: 20, W: 30, H: 30}},
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 100, H: 100}, Color: frame.NewColor(0, 0, 255, 255)},
		{Kind: raster.CmdClipPop},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	r, g, bb, a := img.At(25, 25).RGBA()
	if r>>8 != 0 || g>>8 != 0 || bb>>8 != 255 || a>>8 != 255 {
		t.Errorf("inside clip = (%d,%d,%d,%d), want blue", r>>8, g>>8, bb>>8, a>>8)
	}
	r, g, bb, a = img.At(5, 5).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("outside clip = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

func TestCGBackendNestedClips(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdClipPush, Rect: frame.Rect{X: 0, Y: 0, W: 80, H: 80}},
		{Kind: raster.CmdClipPush, Rect: frame.Rect{X: 20, Y: 20, W: 40, H: 40}},
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 100, H: 100}, Color: frame.NewColor(0, 255, 0, 255)},
		{Kind: raster.CmdClipPop},
		{Kind: raster.CmdClipPop},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	r, g, bb, a := img.At(30, 30).RGBA()
	if g>>8 != 255 {
		t.Errorf("inside both clips = (%d,%d,%d,%d), want green", r>>8, g>>8, bb>>8, a>>8)
	}
	r, g, bb, a = img.At(5, 5).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("outside inner clip = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

// ---------------------------------------------------------------------------
// Opacity
// ---------------------------------------------------------------------------

func TestCGBackendFillWithOpacity(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdOpacityPush, Opacity: 0.5},
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 50, H: 50}, Color: frame.NewColor(255, 0, 0, 255)},
		{Kind: raster.CmdOpacityPop},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, a := img.At(25, 25).RGBA()
	alpha := a >> 8
	if alpha < 120 || alpha > 135 {
		t.Errorf("opacity pixel alpha = %d, want ~128", alpha)
	}
}

// ---------------------------------------------------------------------------
// Border
// ---------------------------------------------------------------------------

func TestCGBackendBorderTop(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdBorder, Rect: frame.Rect{X: 10, Y: 10, W: 50, H: 50}, Border: raster.BorderSpec{Top: raster.SideSpec{Width: 3, Color: frame.NewColor(255, 0, 0, 255)}}},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	r, g, bb, a := img.At(20, 10).RGBA()
	if r>>8 != 255 || g>>8 != 0 || bb>>8 != 0 || a>>8 != 255 {
		t.Errorf("top border = (%d,%d,%d,%d), want red", r>>8, g>>8, bb>>8, a>>8)
	}
	r, g, bb, a = img.At(20, 15).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("below top border = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

func TestCGBackendBorderAllSides(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	b.BeginFrame(vp)

	red := frame.NewColor(255, 0, 0, 255)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdBorder, Rect: frame.Rect{X: 10, Y: 10, W: 50, H: 50}, Border: raster.BorderSpec{
			Top: raster.SideSpec{Width: 2, Color: red}, Right: raster.SideSpec{Width: 2, Color: red},
			Bottom: raster.SideSpec{Width: 2, Color: red}, Left: raster.SideSpec{Width: 2, Color: red},
		}},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		x, y int
		desc string
	}{
		{20, 10, "top"}, {20, 58, "bottom"}, {10, 20, "left"}, {58, 20, "right"},
	} {
		r, g, bb, a := img.At(tt.x, tt.y).RGBA()
		if r>>8 != 255 || g>>8 != 0 || bb>>8 != 0 || a>>8 != 255 {
			t.Errorf("%s border at (%d,%d) = (%d,%d,%d,%d), want red", tt.desc, tt.x, tt.y, r>>8, g>>8, bb>>8, a>>8)
		}
	}
	r, g, bb, a := img.At(30, 30).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("center = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

// ---------------------------------------------------------------------------
// Empty / Transparent
// ---------------------------------------------------------------------------

func TestCGBackendEmptyCommands(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err != nil {
		t.Fatal(err)
	}
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
	r, g, bb, a := img.At(50, 50).RGBA()
	if r != 0 || g != 0 || bb != 0 || a != 0 {
		t.Errorf("empty frame pixel = (%d,%d,%d,%d), want transparent", r, g, bb, a)
	}
}

// ---------------------------------------------------------------------------
// HiDPI
// ---------------------------------------------------------------------------

func TestCGBackendHiDPI(t *testing.T) {
	b, err := raster.NewCGBackend(200, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ps := frame.PixelScale{Scale: 2.0, DPI: 192}
	vp := frame.NewViewport(100, 100, ps)
	b.BeginFrame(vp)

	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 200, H: 200}, Color: frame.NewColor(128, 128, 128, 255)},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatal(err)
	}

	rgba := img.(*image.RGBA)
	bounds := rgba.Bounds()
	if bounds.Dx() != 200 || bounds.Dy() != 200 {
		t.Errorf("HiDPI buffer = %v, want 200x200", bounds)
	}
}

// ---------------------------------------------------------------------------
// Cross-backend comparison: CG output vs CPU output
// ---------------------------------------------------------------------------

type cgCmpResult struct {
	Match       bool
	TotalPixels int
	DiffPixels  int
	MaxDelta    uint8
	MeanDelta   float64
}

func (r cgCmpResult) String() string {
	return fmt.Sprintf("pixels=%d diff=%d maxDelta=%d meanDelta=%.3f match=%v",
		r.TotalPixels, r.DiffPixels, r.MaxDelta, r.MeanDelta, r.Match)
}

func cgCompareImages(got, want image.Image, tolerance uint8) cgCmpResult {
	bounds := got.Bounds()
	if bounds != want.Bounds() {
		return cgCmpResult{Match: false, TotalPixels: bounds.Dx() * bounds.Dy(), DiffPixels: bounds.Dx() * bounds.Dy(), MaxDelta: 255}
	}

	var diffPixels int
	var totalDelta uint64
	var maxDelta uint8
	total := bounds.Dx() * bounds.Dy()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gr, gg, gb, ga := got.At(x, y).RGBA()
			wr, wg, wb, wa := want.At(x, y).RGBA()

			dr := cgAbsDiff(uint8(gr>>8), uint8(wr>>8))
			dg := cgAbsDiff(uint8(gg>>8), uint8(wg>>8))
			db := cgAbsDiff(uint8(gb>>8), uint8(wb>>8))
			da := cgAbsDiff(uint8(ga>>8), uint8(wa>>8))

			maxChannel := dr
			if dg > maxChannel {
				maxChannel = dg
			}
			if db > maxChannel {
				maxChannel = db
			}
			if da > maxChannel {
				maxChannel = da
			}
			if maxChannel > maxDelta {
				maxDelta = maxChannel
			}

			totalDelta += uint64(dr) + uint64(dg) + uint64(db) + uint64(da)
			if maxChannel > tolerance {
				diffPixels++
			}
		}
	}

	meanDelta := float64(0)
	if total > 0 {
		meanDelta = float64(totalDelta) / float64(total*4)
	}
	return cgCmpResult{Match: diffPixels == 0, TotalPixels: total, DiffPixels: diffPixels, MaxDelta: maxDelta, MeanDelta: meanDelta}
}

func cgAbsDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func testBackendEquivalence(t *testing.T, name string, w, h int, cmds []raster.DisplayCmd) {
	t.Helper()

	vp := frame.NewViewport(float32(w), float32(h), frame.PixelScaleDefault)

	cpuB := raster.NewCPUBackend(w, h)
	cpuB.BeginFrame(vp)
	cpuImg, err := cpuB.Rasterize(cmds, nil)
	if err != nil {
		t.Fatalf("CPU Rasterize: %v", err)
	}
	cpuB.EndFrame()

	cgB, err := raster.NewCGBackend(w, h)
	if err != nil {
		t.Fatalf("raster.NewCGBackend: %v", err)
	}
	defer cgB.Close()
	cgB.BeginFrame(vp)
	cgImg, err := cgB.Rasterize(cmds, nil)
	if err != nil {
		t.Fatalf("CG Rasterize: %v", err)
	}
	cgB.EndFrame()

	result := cgCompareImages(cgImg, cpuImg, 2)
	if !result.Match {
		t.Errorf("%s: CG output differs from CPU output: %s", name, result.String())
	}
	cpuB.Close()
}

func TestCGBackendVsCPUBackendFill(t *testing.T) {
	testBackendEquivalence(t, "cg_vs_cpu_fill", 200, 100, []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 100), Color: frame.White},
		{Kind: raster.CmdFill, Rect: frame.NewRect(20, 20, 60, 60), Color: frame.NewColor(255, 0, 0, 255)},
	})
}

func TestCGBackendVsCPUBackendBorder(t *testing.T) {
	testBackendEquivalence(t, "cg_vs_cpu_border", 200, 100, []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 100), Color: frame.White},
		{Kind: raster.CmdBorder, Rect: frame.NewRect(10, 10, 80, 80), Color: frame.Transparent, Border: raster.BorderSpec{
			Top:    raster.SideSpec{Width: 2, Color: frame.NewColor(0, 0, 255, 255)},
			Right:  raster.SideSpec{Width: 2, Color: frame.NewColor(0, 0, 255, 255)},
			Bottom: raster.SideSpec{Width: 2, Color: frame.NewColor(0, 0, 255, 255)},
			Left:   raster.SideSpec{Width: 2, Color: frame.NewColor(0, 0, 255, 255)},
		}},
	})
}

func TestCGBackendVsCPUBackendClip(t *testing.T) {
	testBackendEquivalence(t, "cg_vs_cpu_clip", 200, 200, []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 200), Color: frame.White},
		{Kind: raster.CmdClipPush, Rect: frame.NewRect(25, 25, 50, 50)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 200), Color: frame.NewColor(0, 200, 0, 255)},
		{Kind: raster.CmdClipPop},
	})
}

func TestCGBackendVsCPUBackendOpacity(t *testing.T) {
	testBackendEquivalence(t, "cg_vs_cpu_opacity", 200, 100, []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 100), Color: frame.NewColor(0, 0, 255, 255)},
		{Kind: raster.CmdOpacityPush, Opacity: 0.5},
		{Kind: raster.CmdFill, Rect: frame.NewRect(25, 0, 50, 100), Color: frame.NewColor(255, 0, 0, 255)},
		{Kind: raster.CmdOpacityPop},
	})
}

func TestCGBackendVsCPUBackendNestedClip(t *testing.T) {
	testBackendEquivalence(t, "cg_vs_cpu_nested_clip", 200, 200, []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 200), Color: frame.White},
		{Kind: raster.CmdClipPush, Rect: frame.NewRect(10, 10, 180, 180)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 200), Color: frame.NewColor(200, 200, 200, 255)},
		{Kind: raster.CmdClipPush, Rect: frame.NewRect(30, 30, 80, 80)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 200), Color: frame.NewColor(100, 100, 255, 255)},
		{Kind: raster.CmdClipPop},
		{Kind: raster.CmdClipPop},
	})
}

func TestCGBackendVsCPUBackendComposite(t *testing.T) {
	testBackendEquivalence(t, "cg_vs_cpu_composite", 300, 200, []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 300, 200), Color: frame.NewColor(240, 240, 240, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 300, 40), Color: frame.NewColor(50, 50, 50, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 40, 80, 160), Color: frame.NewColor(200, 200, 200, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(90, 50, 200, 30), Color: frame.NewColor(255, 255, 255, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(90, 90, 200, 30), Color: frame.NewColor(255, 255, 255, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(90, 130, 200, 30), Color: frame.NewColor(255, 255, 255, 255)},
		{Kind: raster.CmdBorder, Rect: frame.NewRect(85, 45, 210, 120), Color: frame.Transparent, Border: raster.BorderSpec{
			Top:    raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
			Right:  raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
			Bottom: raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
			Left:   raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
		}},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 190, 300, 10), Color: frame.NewColor(0, 120, 215, 255)},
	})
}

func TestCGBackendVsCPUBackendEmpty(t *testing.T) {
	testBackendEquivalence(t, "cg_vs_cpu_empty", 100, 100, nil)
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkCGFillRect100x100(b *testing.B) {
	backend, err := raster.NewCGBackend(400, 400)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	vp := frame.NewViewport(400, 400, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 50, Y: 50, W: 100, H: 100}, Color: frame.NewColor(255, 0, 0, 255)},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.BeginFrame(vp)
		backend.Rasterize(cmds, nil)
		backend.EndFrame()
	}
}

func BenchmarkCGFillRectFullFrame(b *testing.B) {
	backend, err := raster.NewCGBackend(800, 600)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 800, H: 600}, Color: frame.NewColor(200, 200, 200, 255)},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.BeginFrame(vp)
		backend.Rasterize(cmds, nil)
		backend.EndFrame()
	}
}

func BenchmarkCGFillWithClip(b *testing.B) {
	backend, err := raster.NewCGBackend(400, 400)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	vp := frame.NewViewport(400, 400, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdClipPush, Rect: frame.Rect{X: 50, Y: 50, W: 200, H: 200}},
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 400, H: 400}, Color: frame.NewColor(0, 255, 0, 255)},
		{Kind: raster.CmdClipPop},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.BeginFrame(vp)
		backend.Rasterize(cmds, nil)
		backend.EndFrame()
	}
}

func BenchmarkCGBorderAllSides(b *testing.B) {
	backend, err := raster.NewCGBackend(400, 400)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	vp := frame.NewViewport(400, 400, frame.PixelScaleDefault)
	red := frame.NewColor(255, 0, 0, 255)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdBorder, Rect: frame.Rect{X: 50, Y: 50, W: 200, H: 150}, Border: raster.BorderSpec{
			Top: raster.SideSpec{Width: 2, Color: red}, Right: raster.SideSpec{Width: 2, Color: red},
			Bottom: raster.SideSpec{Width: 2, Color: red}, Left: raster.SideSpec{Width: 2, Color: red},
		}},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.BeginFrame(vp)
		backend.Rasterize(cmds, nil)
		backend.EndFrame()
	}
}

func benchmarkCGFill(b *testing.B, w, h int) {
	backend, err := raster.NewCGBackend(w, h)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	vp := frame.NewViewport(float32(w), float32(h), frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: float32(w), H: float32(h)}, Color: frame.NewColor(255, 0, 0, 255)},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.BeginFrame(vp)
		backend.Rasterize(cmds, nil)
		backend.EndFrame()
	}
}

func BenchmarkCGRasterFillSmall(b *testing.B)  { benchmarkCGFill(b, 100, 100) }
func BenchmarkCGRasterFillMedium(b *testing.B) { benchmarkCGFill(b, 400, 400) }
func BenchmarkCGRasterFillLarge(b *testing.B)  { benchmarkCGFill(b, 800, 600) }
