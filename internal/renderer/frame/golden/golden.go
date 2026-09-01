// Package golden provides golden image testing for the raster backend.
//
// M6.5: Renders deterministic fixtures at fixed viewport sizes, compares
// output with tolerance rules, and stores intentional updates separately
// from test execution.
package golden

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

// Config controls golden image comparison behavior.
type Config struct {
	// Tolerance is the maximum per-channel difference (0-255) allowed
	// between corresponding pixels. Default: 1 (allows rounding differences).
	Tolerance uint8

	// MaxDiffPixels is the maximum number of pixels that may differ beyond
	// tolerance. Default: 0 (exact match within tolerance).
	MaxDiffPixels int

	// GoldenDir is the directory containing golden reference images.
	// Default: "testdata/golden".
	GoldenDir string

	// UpdateDir is the directory where -update writes new golden images.
	// Default: "testdata/golden-update".
	UpdateDir string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Tolerance:     1,
		MaxDiffPixels: 0,
		GoldenDir:     "testdata/golden",
		UpdateDir:     "testdata/golden-update",
	}
}

// CompareResult holds the result of a golden image comparison.
type CompareResult struct {
	Match       bool
	TotalPixels int
	DiffPixels  int
	MaxDelta    uint8
	MeanDelta   float64
}

// CompareImages compares two images pixel-by-pixel with tolerance.
// Both images must have the same dimensions.
func CompareImages(got, want image.Image, tolerance uint8) CompareResult {
	bounds := got.Bounds()
	if bounds != want.Bounds() {
		return CompareResult{
			Match:       false,
			TotalPixels: bounds.Dx() * bounds.Dy(),
			DiffPixels:  bounds.Dx() * bounds.Dy(),
			MaxDelta:    255,
		}
	}

	var diffPixels int
	var totalDelta uint64
	var maxDelta uint8
	total := bounds.Dx() * bounds.Dy()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gr, gg, gb, ga := got.At(x, y).RGBA()
			wr, wg, wb, wa := want.At(x, y).RGBA()

			dr := absDiff(uint8(gr>>8), uint8(wr>>8))
			dg := absDiff(uint8(gg>>8), uint8(wg>>8))
			db := absDiff(uint8(gb>>8), uint8(wb>>8))
			da := absDiff(uint8(ga>>8), uint8(wa>>8))

			maxChannel := max(dr, dg, db, da)
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

	return CompareResult{
		Match:       diffPixels == 0,
		TotalPixels: total,
		DiffPixels:  diffPixels,
		MaxDelta:    maxDelta,
		MeanDelta:   meanDelta,
	}
}

// AssertGolden renders a frame using the CPU backend and compares it against
// a stored golden image. If the -update flag is set (via env var
// GOOSIE_UPDATE_GOLDEN=1), the golden image is written instead of compared.
func AssertGolden(t *testing.T, name string, cfg Config, vp frame.Viewport, cmds []raster.DisplayCmd) {
	t.Helper()

	if cfg.GoldenDir == "" {
		cfg.GoldenDir = DefaultConfig().GoldenDir
	}
	if cfg.UpdateDir == "" {
		cfg.UpdateDir = DefaultConfig().UpdateDir
	}

	// Render the frame.
	dw, dh := vp.DeviceSize()
	backend := raster.NewCPUBackend(int(dw), int(dh))
	if err := backend.BeginFrame(vp); err != nil {
		t.Fatalf("BeginFrame: %v", err)
	}
	img, err := backend.Rasterize(cmds, nil)
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if err := backend.EndFrame(); err != nil {
		t.Fatalf("EndFrame: %v", err)
	}
	backend.Close()

	gotRGBA := ToRGBA(img)
	goldenPath := filepath.Join(cfg.GoldenDir, name+".png")

	// Update mode: write golden image and skip comparison.
	if os.Getenv("GOOSIE_UPDATE_GOLDEN") == "1" {
		if err := writePNG(goldenPath, gotRGBA); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden: %s", goldenPath)
		return
	}

	// Load reference golden image.
	wantRGBA, err := loadPNG(goldenPath)
	if err != nil {
		// No golden exists — write to update dir for review.
		updatePath := filepath.Join(cfg.UpdateDir, name+".png")
		if werr := writePNG(updatePath, gotRGBA); werr != nil {
			t.Fatalf("write update: %v", werr)
		}
		t.Fatalf("no golden image at %s — wrote candidate to %s (set GOOSIE_UPDATE_GOLDEN=1 to accept)", goldenPath, updatePath)
	}

	// Compare.
	result := CompareImages(gotRGBA, wantRGBA, cfg.Tolerance)
	if !result.Match {
		if result.DiffPixels <= cfg.MaxDiffPixels {
			return // Within acceptable diff budget.
		}
		// Write diff image for debugging.
		diffPath := filepath.Join(cfg.UpdateDir, name+"_diff.png")
		diffImg := CreateDiffImage(gotRGBA, wantRGBA, cfg.Tolerance)
		_ = writePNG(diffPath, diffImg)

		t.Errorf("golden mismatch %q: %d/%d pixels differ (max delta=%d, mean=%.2f)\n  diff: %s",
			name, result.DiffPixels, result.TotalPixels, result.MaxDelta, result.MeanDelta, diffPath)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func ToRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba
}

func writePNG(path string, img *image.RGBA) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func loadPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	return ToRGBA(img), nil
}

// CreateDiffImage produces a visualization where differing pixels are red.
func CreateDiffImage(got, want *image.RGBA, tolerance uint8) *image.RGBA {
	bounds := got.Bounds()
	diff := image.NewRGBA(bounds)
	red := color.RGBA{R: 255, A: 255}
	transparent := color.RGBA{}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gr, gg, gb, ga := got.At(x, y).RGBA()
			wr, wg, wb, wa := want.At(x, y).RGBA()

			dr := absDiff(uint8(gr>>8), uint8(wr>>8))
			dg := absDiff(uint8(gg>>8), uint8(wg>>8))
			db := absDiff(uint8(gb>>8), uint8(wb>>8))
			da := absDiff(uint8(ga>>8), uint8(wa>>8))

			if max(dr, dg, db, da) > tolerance {
				diff.Set(x, y, red)
			} else {
				diff.Set(x, y, transparent)
			}
		}
	}
	return diff
}

// String returns a human-readable summary of the comparison.
func (r CompareResult) String() string {
	return fmt.Sprintf("pixels=%d diff=%d maxDelta=%d meanDelta=%.3f match=%v",
		r.TotalPixels, r.DiffPixels, r.MaxDelta, r.MeanDelta, r.Match)
}

// PixelDiffRatio returns the fraction of pixels that differ [0,1].
func (r CompareResult) PixelDiffRatio() float64 {
	if r.TotalPixels == 0 {
		return 0
	}
	return float64(r.DiffPixels) / float64(r.TotalPixels)
}

// NearlyEqual reports whether two floats are within epsilon.
func NearlyEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) <= epsilon
}

// ---------------------------------------------------------------------------
// M2.2: Dual Backend Parity
// ---------------------------------------------------------------------------

// BackendParityResult holds the result of comparing CPU and CoreGraphics output.
type BackendParityResult struct {
	CPUImage        *image.RGBA
	CGImage         *image.RGBA
	Comparison      CompareResult
	BackendsMatch   bool
	CPUBackendUsed  raster.BackendType
	CGBackendUsed   raster.BackendType
}

// AssertBackendParity renders the same display commands with both CPU and
// CoreGraphics backends (when available) and verifies they produce identical
// output. On non-macOS platforms or when CGo is disabled, this only tests
// the CPU backend and skips the parity check.
func AssertBackendParity(t *testing.T, name string, vp frame.Viewport, cmds []raster.DisplayCmd) *BackendParityResult {
	t.Helper()

	dw, dh := vp.DeviceSize()
	width, height := int(dw), int(dh)

	// Render with CPU backend.
	cpuBackend := raster.NewCPUBackend(width, height)
	if err := cpuBackend.BeginFrame(vp); err != nil {
		t.Fatalf("CPU BeginFrame: %v", err)
	}
	cpuImg, err := cpuBackend.Rasterize(cmds, nil)
	if err != nil {
		t.Fatalf("CPU Rasterize: %v", err)
	}
	if err := cpuBackend.EndFrame(); err != nil {
		t.Fatalf("CPU EndFrame: %v", err)
	}
	cpuBackend.Close()
	cpuRGBA := ToRGBA(cpuImg)

	result := &BackendParityResult{
		CPUImage:       cpuRGBA,
		CPUBackendUsed: raster.BackendCPU,
	}

	// Try to render with CoreGraphics backend.
	cgBackend, cgType, cgErr := raster.NewBackend(width, height, raster.WithBackend(raster.BackendCoreGraphics))
	if cgErr != nil {
		// CoreGraphics not available on this platform — skip parity check.
		t.Logf("CoreGraphics backend not available: %v (CPU-only test)", cgErr)
		result.BackendsMatch = true // Trivially true when only one backend exists.
		return result
	}
	result.CGBackendUsed = cgType

	if err := cgBackend.BeginFrame(vp); err != nil {
		t.Fatalf("CG BeginFrame: %v", err)
	}
	cgImg, err := cgBackend.Rasterize(cmds, nil)
	if err != nil {
		t.Fatalf("CG Rasterize: %v", err)
	}
	if err := cgBackend.EndFrame(); err != nil {
		t.Fatalf("CG EndFrame: %v", err)
	}
	cgBackend.Close()
	cgRGBA := ToRGBA(cgImg)
	result.CGImage = cgRGBA

	// Compare CPU and CG output pixel-by-pixel.
	comparison := CompareImages(cpuRGBA, cgRGBA, 0) // Zero tolerance for exact parity.
	result.Comparison = comparison
	result.BackendsMatch = comparison.Match

	if !comparison.Match {
		// Write diff images for debugging.
		diffDir := "testdata/golden-parity"
		cpuPath := filepath.Join(diffDir, name+"_cpu.png")
		cgPath := filepath.Join(diffDir, name+"_cg.png")
		diffPath := filepath.Join(diffDir, name+"_parity_diff.png")

		_ = writePNG(cpuPath, cpuRGBA)
		_ = writePNG(cgPath, cgRGBA)
		diffImg := CreateDiffImage(cpuRGBA, cgRGBA, 0)
		_ = writePNG(diffPath, diffImg)

		t.Errorf("backend parity failure %q: %d/%d pixels differ (max delta=%d, mean=%.2f)\n  cpu: %s\n  cg:  %s\n  diff: %s",
			name, comparison.DiffPixels, comparison.TotalPixels, comparison.MaxDelta, comparison.MeanDelta,
			cpuPath, cgPath, diffPath)
	}

	return result
}
