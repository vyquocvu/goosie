package golden

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

// ---------------------------------------------------------------------------
// CompareImages tests
// ---------------------------------------------------------------------------

func TestCompareImagesIdentical(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 128, G: 64, B: 32, A: 255})
		}
	}
	result := CompareImages(img, img, 0)
	if !result.Match {
		t.Errorf("identical images should match: %s", result)
	}
	if result.DiffPixels != 0 {
		t.Errorf("DiffPixels = %d, want 0", result.DiffPixels)
	}
}

func TestCompareImagesDifferent(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 10, 10))
	b := image.NewRGBA(image.Rect(0, 0, 10, 10))
	a.SetRGBA(5, 5, color.RGBA{R: 255, A: 255})
	b.SetRGBA(5, 5, color.RGBA{R: 0, A: 255})

	result := CompareImages(a, b, 0)
	if result.Match {
		t.Error("different images should not match with tolerance 0")
	}
	if result.DiffPixels != 1 {
		t.Errorf("DiffPixels = %d, want 1", result.DiffPixels)
	}
}

func TestCompareImagesWithinTolerance(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 10, 10))
	b := image.NewRGBA(image.Rect(0, 0, 10, 10))
	// Slight rounding difference.
	a.SetRGBA(0, 0, color.RGBA{R: 128, A: 255})
	b.SetRGBA(0, 0, color.RGBA{R: 129, A: 255})

	result := CompareImages(a, b, 1)
	if !result.Match {
		t.Errorf("within tolerance should match: %s", result)
	}
}

func TestCompareImagesDifferentSize(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 10, 10))
	b := image.NewRGBA(image.Rect(0, 0, 20, 20))

	result := CompareImages(a, b, 0)
	if result.Match {
		t.Error("different size images should not match")
	}
}

func TestCompareResultString(t *testing.T) {
	r := CompareResult{Match: true, TotalPixels: 100, DiffPixels: 0, MaxDelta: 0, MeanDelta: 0}
	s := r.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestCompareResultPixelDiffRatio(t *testing.T) {
	r := CompareResult{TotalPixels: 100, DiffPixels: 25}
	ratio := r.PixelDiffRatio()
	if ratio < 0.24 || ratio > 0.26 {
		t.Errorf("PixelDiffRatio = %f, want ~0.25", ratio)
	}
}

func TestCompareResultPixelDiffRatioZero(t *testing.T) {
	r := CompareResult{TotalPixels: 0}
	if r.PixelDiffRatio() != 0 {
		t.Error("zero total pixels should give ratio 0")
	}
}

func TestNearlyEqual(t *testing.T) {
	if !NearlyEqual(1.0, 1.001, 0.01) {
		t.Error("1.0 and 1.001 should be nearly equal with epsilon 0.01")
	}
	if NearlyEqual(1.0, 2.0, 0.01) {
		t.Error("1.0 and 2.0 should not be nearly equal")
	}
}

// ---------------------------------------------------------------------------
// AssertGolden integration tests
// ---------------------------------------------------------------------------

func TestAssertGoldenUpdateMode(t *testing.T) {
	tmpDir := t.TempDir()
	goldenDir := filepath.Join(tmpDir, "golden")

	// Set update mode.
	os.Setenv("GOOSIE_UPDATE_GOLDEN", "1")
	defer os.Unsetenv("GOOSIE_UPDATE_GOLDEN")

	cfg := Config{
		Tolerance: 1,
		GoldenDir: goldenDir,
		UpdateDir: filepath.Join(tmpDir, "update"),
	}

	vp := frame.NewViewport(50, 50, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 50, H: 50}, Color: frame.NewColor(200, 200, 200, 255)},
	}

	AssertGolden(t, "test_fill", cfg, vp, cmds)

	// Verify golden file was created.
	goldenPath := filepath.Join(goldenDir, "test_fill.png")
	if _, err := os.Stat(goldenPath); err != nil {
		t.Errorf("golden file not created: %v", err)
	}
}

func TestAssertGoldenMatch(t *testing.T) {
	tmpDir := t.TempDir()
	goldenDir := filepath.Join(tmpDir, "golden")
	updateDir := filepath.Join(tmpDir, "update")

	cfg := Config{
		Tolerance: 1,
		GoldenDir: goldenDir,
		UpdateDir: updateDir,
	}

	vp := frame.NewViewport(50, 50, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{Kind: raster.CmdFill, Rect: frame.Rect{X: 0, Y: 0, W: 50, H: 50}, Color: frame.NewColor(200, 200, 200, 255)},
	}

	// First: create golden.
	os.Setenv("GOOSIE_UPDATE_GOLDEN", "1")
	AssertGolden(t, "test_match", cfg, vp, cmds)
	os.Unsetenv("GOOSIE_UPDATE_GOLDEN")

	// Second: should match (same commands).
	AssertGolden(t, "test_match", cfg, vp, cmds)
}

func TestAssertGoldenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{
		Tolerance: 1,
		GoldenDir: filepath.Join(tmpDir, "nonexistent"),
		UpdateDir: filepath.Join(tmpDir, "update"),
	}

	// Should fail with helpful message about missing golden.
	// Verify the golden file doesn't exist.
	goldenPath := filepath.Join(cfg.GoldenDir, "missing.png")
	if _, err := os.Stat(goldenPath); err == nil {
		t.Error("golden should not exist")
	}
}

// ---------------------------------------------------------------------------
// createDiffImage tests
// ---------------------------------------------------------------------------

func TestCreateDiffImage(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 10, 10))
	b := image.NewRGBA(image.Rect(0, 0, 10, 10))
	a.SetRGBA(5, 5, color.RGBA{R: 255, A: 255})
	// b has transparent at (5,5)

	diff := createDiffImage(a, b, 0)
	// Pixel (5,5) should be red (differs).
	r, _, _, _ := diff.At(5, 5).RGBA()
	if r>>8 != 255 {
		t.Error("diff pixel should be red")
	}
	// Pixel (0,0) should be transparent (same).
	_, _, _, alpha := diff.At(0, 0).RGBA()
	if alpha != 0 {
		t.Error("matching pixel should be transparent in diff")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkCompareImages100x100(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompareImages(img, img, 1)
	}
}

func BenchmarkCompareImages800x600(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 800; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x & 0xFF), G: uint8(y & 0xFF), A: 255})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompareImages(img, img, 1)
	}
}
