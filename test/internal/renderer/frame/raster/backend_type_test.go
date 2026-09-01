package raster_test

import (
	"fmt"
	"image"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// ---------------------------------------------------------------------------
// raster.BackendType
// ---------------------------------------------------------------------------

func TestBackendTypeString(t *testing.T) {
	tests := []struct {
		bt   raster.BackendType
		want string
	}{
		{raster.BackendUnspecified, "unspecified"},
		{raster.BackendCPU, "cpu"},
		{raster.BackendCoreGraphics, "core-graphics"},
	}
	for _, tt := range tests {
		if got := tt.bt.String(); got != tt.want {
			t.Errorf("raster.BackendType(%d).String() = %q, want %q", tt.bt, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// raster.SelectBackend
// ---------------------------------------------------------------------------

func TestSelectBackend(t *testing.T) {
	bt := raster.SelectBackend()
	if bt != raster.BackendCPU && bt != raster.BackendCoreGraphics {
		t.Errorf("raster.SelectBackend() = %v, want cpu or core-graphics", bt)
	}
	// Verify the selected backend can actually be constructed.
	b, _, err := raster.NewBackend(1, 1)
	if err != nil {
		t.Fatalf("raster.NewBackend with auto-select: %v", err)
	}
	b.Close()
}

// ---------------------------------------------------------------------------
// raster.NewBackend
// ---------------------------------------------------------------------------

func TestNewBackendDefault(t *testing.T) {
	b, bt, err := raster.NewBackend(100, 100)
	if err != nil {
		t.Fatalf("raster.NewBackend(100,100): %v", err)
	}
	defer b.Close()

	if bt != raster.BackendCPU && bt != raster.BackendCoreGraphics {
		t.Errorf("raster.NewBackend returned raster.BackendType = %v, want cpu or core-graphics", bt)
	}

	// Verify the backend actually renders.
	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	if err := b.BeginFrame(vp); err != nil {
		t.Fatalf("BeginFrame: %v", err)
	}
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 50, H: 50}, Color: frame.NewColor(255, 0, 0, 255)},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if img == nil {
		t.Fatal("Rasterize returned nil image")
	}
	if err := b.EndFrame(); err != nil {
		t.Fatalf("EndFrame: %v", err)
	}

	// Verify the fill actually drew.
	r, _, _, a := img.At(25, 25).RGBA()
	if r == 0 || a == 0 {
		t.Errorf("pixel at (25,25) = (%d, %d), want non-zero red and alpha", r, a)
	}
}

func TestNewBackendZeroSize(t *testing.T) {
	b, _, err := raster.NewBackend(0, 0)
	if err != nil {
		t.Fatalf("raster.NewBackend(0,0): %v", err)
	}
	defer b.Close()

	vp := frame.NewViewport(0, 0, frame.PixelScaleDefault)
	if err := b.BeginFrame(vp); err != nil {
		t.Fatalf("BeginFrame(0,0): %v", err)
	}
	b.EndFrame()
}

func TestNewBackendCloseTwice(t *testing.T) {
	b, _, err := raster.NewBackend(10, 10)
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

// ---------------------------------------------------------------------------
// Forced CPU backend
// ---------------------------------------------------------------------------

func TestNewBackendForceCPU(t *testing.T) {
	b, bt, err := raster.NewBackend(100, 100, raster.WithBackend(raster.BackendCPU))
	if err != nil {
		t.Fatalf("raster.NewBackend with raster.WithBackend(CPU): %v", err)
	}
	defer b.Close()

	if bt != raster.BackendCPU {
		t.Errorf("raster.NewBackend with raster.WithBackend(CPU) returned type = %v, want cpu", bt)
	}

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	if err := b.BeginFrame(vp); err != nil {
		t.Fatalf("BeginFrame: %v", err)
	}
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 10, H: 10}, Color: frame.NewColor(0, 255, 0, 255)},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if img == nil {
		t.Fatal("Rasterize returned nil image")
	}
	if err := b.EndFrame(); err != nil {
		t.Fatalf("EndFrame: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Forced CG backend
// ---------------------------------------------------------------------------

func TestNewBackendForceCG(t *testing.T) {
	b, bt, err := raster.NewBackend(100, 100, raster.WithBackend(raster.BackendCoreGraphics))
	if err != nil && err.Error() == "core-graphics: cg backend: not supported on this platform" {
		t.Skip("CG backend not supported on this platform")
	}
	if err == raster.ErrCGBackendNotSupported {
		t.Skip("CG backend not supported on this platform")
	}
	if err != nil {
		t.Fatalf("raster.NewBackend with raster.WithBackend(CG): %v", err)
	}
	defer b.Close()

	if bt != raster.BackendCoreGraphics {
		t.Errorf("raster.NewBackend with raster.WithBackend(CG) returned type = %v, want core-graphics", bt)
	}

	// Verify it renders.
	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	if err := b.BeginFrame(vp); err != nil {
		t.Fatalf("BeginFrame: %v", err)
	}
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 50, H: 50}, Color: frame.NewColor(0, 0, 255, 255)},
	}
	img, err := b.Rasterize(cmds, nil)
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if img == nil {
		t.Fatal("Rasterize returned nil image")
	}
	if err := b.EndFrame(); err != nil {
		t.Fatalf("EndFrame: %v", err)
	}
	r, g, bb, a := img.At(25, 25).RGBA()
	if bb>>8 != 255 || a>>8 != 255 {
		t.Errorf("CG fill = (%d,%d,%d,%d), want blue at (25,25)", r>>8, g>>8, bb>>8, a>>8)
	}
}

// ---------------------------------------------------------------------------
// Crash recovery
// ---------------------------------------------------------------------------

// crashBackend intentionally panics to test recovery.
type crashBackend struct{ raster.Backend }

func (b *crashBackend) BeginFrame(vp frame.Viewport) error { panic("crashBackend: BeginFrame") }

func TestNewBackendCrashRecover(t *testing.T) {
	// The crash-recovery feature protects against panics during backend
	// construction. We can't easily inject a panic into CG/CPU constructors,
	// so we verify that the raster.WithCrashRecover option is accepted and doesn't
	// break the normal path.
	b, bt, err := raster.NewBackend(100, 100, raster.WithCrashRecover())
	if err != nil {
		t.Fatalf("raster.NewBackend with raster.WithCrashRecover: %v", err)
	}
	defer b.Close()

	if bt != raster.BackendCPU && bt != raster.BackendCoreGraphics {
		t.Errorf("got type %v, want cpu or core-graphics", bt)
	}

	// Verify the backend works normally.
	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	if err := b.BeginFrame(vp); err != nil {
		t.Fatalf("BeginFrame: %v", err)
	}
	img, err := b.Rasterize(nil, nil)
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if img == nil {
		t.Fatal("Rasterize returned nil image")
	}
	if err := b.EndFrame(); err != nil {
		t.Fatalf("EndFrame: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Functional test: both backends produce the same result
// ---------------------------------------------------------------------------

func TestCPUAndCGProduceSameFill(t *testing.T) {
	cgB, cgBT, err := raster.NewBackend(50, 50, raster.WithBackend(raster.BackendCoreGraphics))
	if err != nil && err.Error() == "core-graphics: cg backend: not supported on this platform" {
		t.Skip("CG backend not supported on this platform")
	}
	if err == raster.ErrCGBackendNotSupported {
		t.Skip("CG backend not supported on this platform")
	}
	if err != nil {
		t.Fatalf("raster.NewBackend(CG): %v", err)
	}
	defer cgB.Close()

	cpuB, cpuBT, err := raster.NewBackend(50, 50, raster.WithBackend(raster.BackendCPU))
	if err != nil {
		t.Fatalf("raster.NewBackend(CPU): %v", err)
	}
	defer cpuB.Close()

	if cgBT != raster.BackendCoreGraphics || cpuBT != raster.BackendCPU {
		t.Fatalf("unexpected types: CG=%v CPU=%v", cgBT, cpuBT)
	}

	vp := frame.NewViewport(50, 50, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 5, Y: 5, W: 40, H: 40}, Color: frame.NewColor(128, 200, 255, 255)},
	}

	cgB.BeginFrame(vp)
	cgImg, err := cgB.Rasterize(cmds, nil)
	if err != nil {
		t.Fatalf("CG Rasterize: %v", err)
	}
	cgB.EndFrame()

	cpuB.BeginFrame(vp)
	cpuImg, err := cpuB.Rasterize(cmds, nil)
	if err != nil {
		t.Fatalf("CPU Rasterize: %v", err)
	}
	cpuB.EndFrame()

	result := backendCompareImages(cgImg, cpuImg, 2)
	if !result.match {
		t.Errorf("CG vs CPU fill mismatch: %s", result.string())
	}
}

// ---------------------------------------------------------------------------
// Inline pixel comparison (avoids import cycle w/ golden package)
// ---------------------------------------------------------------------------

type backCmpResult struct {
	match       bool
	totalPixels int
	diffPixels  int
	maxDelta    uint8
	meanDelta   float64
}

func (r backCmpResult) string() string {
	return fmt.Sprintf("pixels=%d diff=%d maxDelta=%d meanDelta=%.3f match=%v",
		r.totalPixels, r.diffPixels, r.maxDelta, r.meanDelta, r.match)
}

func backendCompareImages(got, want image.Image, tolerance uint8) backCmpResult {
	bounds := got.Bounds()
	if bounds != want.Bounds() {
		return backCmpResult{match: false, totalPixels: bounds.Dx() * bounds.Dy(), diffPixels: bounds.Dx() * bounds.Dy(), maxDelta: 255}
	}

	var diffPixels int
	var totalDelta uint64
	var maxDelta uint8
	total := bounds.Dx() * bounds.Dy()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gr, gg, gb, ga := got.At(x, y).RGBA()
			wr, wg, wb, wa := want.At(x, y).RGBA()

			dr := backAbsDiff(uint8(gr>>8), uint8(wr>>8))
			dg := backAbsDiff(uint8(gg>>8), uint8(wg>>8))
			db := backAbsDiff(uint8(gb>>8), uint8(wb>>8))
			da := backAbsDiff(uint8(ga>>8), uint8(wa>>8))

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
	return backCmpResult{match: diffPixels == 0, totalPixels: total, diffPixels: diffPixels, maxDelta: maxDelta, meanDelta: meanDelta}
}

func backAbsDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
