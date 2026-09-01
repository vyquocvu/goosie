package golden_test

import (
	"github.com/vyquocvu/goosie/internal/renderer/frame/golden"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

// Test fixtures — deterministic display command sequences for golden comparison.
// These use only CmdFill, CmdBorder, CmdClipPush/Pop, CmdOpacityPush/Pop
// to guarantee bit-identical output across platforms (pure Go CPU rasterizer).

func TestGoldenFillRedRect(t *testing.T) {
	vp := frame.NewViewport(200, 100, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 100), Color: frame.White},
		{Kind: raster.CmdFill, Rect: frame.NewRect(20, 20, 60, 60), Color: frame.NewColor(255, 0, 0, 255)},
	}
	assertGolden(t, "fill_red_rect", vp, cmds)
}

func TestGoldenBorderBlueRect(t *testing.T) {
	vp := frame.NewViewport(200, 100, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 100), Color: frame.White},
		{
			Kind:  raster.CmdBorder,
			Rect:  frame.NewRect(10, 10, 80, 80),
			Color: frame.Transparent,
			Border: raster.BorderSpec{
				Top:    raster.SideSpec{Width: 2, Color: frame.NewColor(0, 0, 255, 255)},
				Right:  raster.SideSpec{Width: 2, Color: frame.NewColor(0, 0, 255, 255)},
				Bottom: raster.SideSpec{Width: 2, Color: frame.NewColor(0, 0, 255, 255)},
				Left:   raster.SideSpec{Width: 2, Color: frame.NewColor(0, 0, 255, 255)},
			},
		},
	}
	assertGolden(t, "border_blue_rect", vp, cmds)
}

func TestGoldenClippedFill(t *testing.T) {
	vp := frame.NewViewport(200, 200, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 200), Color: frame.White},
		// Clip to a 50×50 region.
		{Kind: raster.CmdClipPush, Rect: frame.NewRect(25, 25, 50, 50)},
		// Fill rect that extends beyond the clip.
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 200), Color: frame.NewColor(0, 200, 0, 255)},
		{Kind: raster.CmdClipPop},
	}
	assertGolden(t, "clipped_fill", vp, cmds)
}

func TestGoldenOpacityBlend(t *testing.T) {
	vp := frame.NewViewport(200, 100, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 100), Color: frame.NewColor(0, 0, 255, 255)},
		{Kind: raster.CmdOpacityPush, Opacity: 0.5},
		{Kind: raster.CmdFill, Rect: frame.NewRect(25, 0, 50, 100), Color: frame.NewColor(255, 0, 0, 255)},
		{Kind: raster.CmdOpacityPop},
	}
	assertGolden(t, "opacity_blend", vp, cmds)
}

func TestGoldenCompositeScene(t *testing.T) {
	vp := frame.NewViewport(300, 200, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		// Background.
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 300, 200), Color: frame.NewColor(240, 240, 240, 255)},
		// Header bar.
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 300, 40), Color: frame.NewColor(50, 50, 50, 255)},
		// Sidebar.
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 40, 80, 160), Color: frame.NewColor(200, 200, 200, 255)},
		// Content area.
		{Kind: raster.CmdFill, Rect: frame.NewRect(90, 50, 200, 30), Color: frame.NewColor(255, 255, 255, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(90, 90, 200, 30), Color: frame.NewColor(255, 255, 255, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(90, 130, 200, 30), Color: frame.NewColor(255, 255, 255, 255)},
		// Border around content card.
		{
			Kind:  raster.CmdBorder,
			Rect:  frame.NewRect(85, 45, 210, 120),
			Color: frame.Transparent,
			Border: raster.BorderSpec{
				Top:    raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
				Right:  raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
				Bottom: raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
				Left:   raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
			},
		},
		// Footer accent.
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 190, 300, 10), Color: frame.NewColor(0, 120, 215, 255)},
	}
	assertGolden(t, "composite_scene", vp, cmds)
}

func TestGoldenNestedClip(t *testing.T) {
	vp := frame.NewViewport(200, 200, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 200), Color: frame.White},
		// Outer clip.
		{Kind: raster.CmdClipPush, Rect: frame.NewRect(10, 10, 180, 180)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 200), Color: frame.NewColor(200, 200, 200, 255)},
		// Inner clip — intersection of outer and inner.
		{Kind: raster.CmdClipPush, Rect: frame.NewRect(30, 30, 80, 80)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 200, 200), Color: frame.NewColor(100, 100, 255, 255)},
		{Kind: raster.CmdClipPop},
		{Kind: raster.CmdClipPop},
	}
	assertGolden(t, "nested_clip", vp, cmds)
}

func TestGoldenEmptyFrame(t *testing.T) {
	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{}
	assertGolden(t, "empty_frame", vp, cmds)
}

// assertGolden renders the commands and compares against a stored golden image.
// In update mode (GOOSIE_UPDATE_GOLDEN=1), writes the golden image instead.
func assertGolden(t *testing.T, name string, vp frame.Viewport, cmds []raster.DisplayCmd) {
	t.Helper()
	cfg := golden.DefaultConfig()
	golden.AssertGolden(t, name, cfg, vp, cmds)
}

// TestGoldenUpdateAndRegress verifies the full round-trip: update then compare.
func TestGoldenUpdateAndRegress(t *testing.T) {
	tmpDir := t.TempDir()
	goldenDir := filepath.Join(tmpDir, "golden")
	updateDir := filepath.Join(tmpDir, "update")

	cfg := golden.Config{
		Tolerance:     1,
		MaxDiffPixels: 0,
		GoldenDir:     goldenDir,
		UpdateDir:     updateDir,
	}

	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 100, 100), Color: frame.NewColor(128, 64, 32, 255)},
	}

	// Phase 1: generate golden.
	os.Setenv("GOOSIE_UPDATE_GOLDEN", "1")
	golden.AssertGolden(t, "roundtrip", cfg, vp, cmds)
	os.Unsetenv("GOOSIE_UPDATE_GOLDEN")

	goldenPath := filepath.Join(goldenDir, "roundtrip.png")
	if _, err := os.Stat(goldenPath); err != nil {
		t.Fatalf("golden file not created: %v", err)
	}

	// Phase 2: compare — should match.
	golden.AssertGolden(t, "roundtrip", cfg, vp, cmds)
}

// TestGoldenMismatchDetection verifies that rendering different commands
// produces images that fail comparison.
func TestGoldenMismatchDetection(t *testing.T) {
	vp := frame.NewViewport(50, 50, frame.PixelScaleDefault)

	cmdsA := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 50, 50), Color: frame.NewColor(255, 0, 0, 255)},
	}
	cmdsB := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 50, 50), Color: frame.NewColor(0, 0, 255, 255)},
	}

	// Render both scenes using separate backends.
	backendA := raster.NewCPUBackend(50, 50)
	if err := backendA.BeginFrame(vp); err != nil {
		t.Fatal(err)
	}
	backendA.Rasterize(cmdsA, nil)
	imgA := golden.ToRGBA(backendA.FrameBuffer().Image())
	backendA.EndFrame()
	backendA.Close()

	backendB := raster.NewCPUBackend(50, 50)
	if err := backendB.BeginFrame(vp); err != nil {
		t.Fatal(err)
	}
	backendB.Rasterize(cmdsB, nil)
	imgB := golden.ToRGBA(backendB.FrameBuffer().Image())
	backendB.EndFrame()
	backendB.Close()

	// They should not match.
	result := golden.CompareImages(imgA, imgB, 0)
	if result.Match {
		t.Error("red and blue fills should not match")
	}
	if result.DiffPixels != 2500 {
		t.Errorf("DiffPixels = %d, want 2500 (50×50)", result.DiffPixels)
	}
}

// TestGoldenTolerance verifies that small differences within tolerance pass.
func TestGoldenTolerance(t *testing.T) {
	tmpDir := t.TempDir()
	goldenDir := filepath.Join(tmpDir, "golden")
	updateDir := filepath.Join(tmpDir, "update")

	cfg := golden.Config{
		Tolerance:     2,
		MaxDiffPixels: 5,
		GoldenDir:     goldenDir,
		UpdateDir:     updateDir,
	}

	vp := frame.NewViewport(10, 10, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 10, 10), Color: frame.NewColor(100, 100, 100, 255)},
	}

	// Create golden.
	os.Setenv("GOOSIE_UPDATE_GOLDEN", "1")
	golden.AssertGolden(t, "tolerance_test", cfg, vp, cmds)
	os.Unsetenv("GOOSIE_UPDATE_GOLDEN")

	// Same commands — should match within tolerance.
	golden.AssertGolden(t, "tolerance_test", cfg, vp, cmds)
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkGoldenRasterFillRect(b *testing.B) {
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 800, 600), Color: frame.NewColor(200, 200, 200, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(100, 100, 200, 200), Color: frame.NewColor(255, 0, 0, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(400, 300, 200, 200), Color: frame.NewColor(0, 0, 255, 255)},
	}
	backend := raster.NewCPUBackend(800, 600)
	if err := backend.BeginFrame(vp); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.Rasterize(cmds, nil)
	}
	b.StopTimer()
	backend.EndFrame()
	backend.Close()
}

func BenchmarkGoldenRasterComposite(b *testing.B) {
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 800, 600), Color: frame.NewColor(240, 240, 240, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 800, 40), Color: frame.NewColor(50, 50, 50, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 40, 200, 560), Color: frame.NewColor(200, 200, 200, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(210, 50, 580, 60), Color: frame.NewColor(255, 255, 255, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(210, 120, 580, 60), Color: frame.NewColor(255, 255, 255, 255)},
		{Kind: raster.CmdFill, Rect: frame.NewRect(210, 190, 580, 60), Color: frame.NewColor(255, 255, 255, 255)},
		{
			Kind:  raster.CmdBorder,
			Rect:  frame.NewRect(205, 45, 590, 210),
			Color: frame.Transparent,
			Border: raster.BorderSpec{
				Top:    raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
				Right:  raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
				Bottom: raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
				Left:   raster.SideSpec{Width: 1, Color: frame.NewColor(180, 180, 180, 255)},
			},
		},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 590, 800, 10), Color: frame.NewColor(0, 120, 215, 255)},
	}
	backend := raster.NewCPUBackend(800, 600)
	if err := backend.BeginFrame(vp); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.Rasterize(cmds, nil)
	}
	b.StopTimer()
	backend.EndFrame()
	backend.Close()
}

func BenchmarkGoldenRasterClipOpacity(b *testing.B) {
	vp := frame.NewViewport(400, 300, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 400, 300), Color: frame.White},
		{Kind: raster.CmdClipPush, Rect: frame.NewRect(50, 50, 300, 200)},
		{Kind: raster.CmdOpacityPush, Opacity: 0.7},
		{Kind: raster.CmdFill, Rect: frame.NewRect(0, 0, 400, 300), Color: frame.NewColor(0, 180, 0, 255)},
		{Kind: raster.CmdOpacityPop},
		{Kind: raster.CmdFill, Rect: frame.NewRect(100, 100, 100, 100), Color: frame.NewColor(180, 0, 0, 255)},
		{Kind: raster.CmdClipPop},
	}
	backend := raster.NewCPUBackend(400, 300)
	if err := backend.BeginFrame(vp); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.Rasterize(cmds, nil)
	}
	b.StopTimer()
	backend.EndFrame()
	backend.Close()
}
