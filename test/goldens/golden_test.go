// Package goldens_test is the pixel regression ratchet: committed renders of
// one fixture per layout family, produced with GOOSIE_SYSTEM_FONTS=0 so the
// pixels are a function of the binary alone (internal/raster/fonts.go).
// A deliberate engine change re-records in the same commit via
// GOOSIE_RECORD_GOLDENS=1 go test ./test/goldens/ -run TestRecordGoldens.
package goldens_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

type goldenEntry struct {
	Family  string `json:"family"`
	Fixture string `json:"fixture"`
	File    string `json:"file"`
	SHA256  string `json:"sha256"`
}

type manifest struct {
	Goldens []goldenEntry `json:"goldens"`
	Render  struct {
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		DPR         int    `json:"dpr"`
		SystemFonts string `json:"systemFonts"`
	} `json:"render"`
}

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

func headlessBinary(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		dir, err := os.MkdirTemp("", "goosie-goldens-bin")
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(dir, "goosie-headless")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/goosie-headless")
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			binErr = fmt.Errorf("build goosie-headless: %v\n%s", err, out)
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return binPath
}

func loadManifest(t *testing.T) manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata", "render-goldens", "manifest.json"))
	if err != nil {
		t.Fatalf("read golden manifest: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse golden manifest: %v", err)
	}
	return m
}

func renderFixture(t *testing.T, bin, fixture string, m manifest, out string) {
	t.Helper()
	src := filepath.Join(repoRoot(t), "testdata", "render", fixture)
	cmd := exec.Command(bin, "-in", src, "-out", out,
		"-width", fmt.Sprint(m.Render.Width),
		"-height", fmt.Sprint(m.Render.Height),
		"-dpr", fmt.Sprint(m.Render.DPR))
	cmd.Env = append(os.Environ(), "GOOSIE_SYSTEM_FONTS=0")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render %s: %v\n%s", fixture, err, combined)
	}
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return img
}

// comparePixels returns the number of differing pixels and the first
// differing coordinate. Images are compared in RGBA so color-model
// differences in the PNG encoding cannot mask a pixel change.
func comparePixels(a, b image.Image) (int, image.Point) {
	ba, bb := a.Bounds(), b.Bounds()
	if ba != bb {
		return -1, image.Point{}
	}
	diffs := 0
	first := image.Point{}
	for y := ba.Min.Y; y < ba.Max.Y; y++ {
		for x := ba.Min.X; x < ba.Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb2, ba2 := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb2 || aa != ba2 {
				if diffs == 0 {
					first = image.Point{X: x, Y: y}
				}
				diffs++
			}
		}
	}
	return diffs, first
}

func TestGoldensMatch(t *testing.T) {
	bin := headlessBinary(t)
	m := loadManifest(t)
	if len(m.Goldens) == 0 {
		t.Fatal("golden manifest is empty")
	}
	for _, g := range m.Goldens {
		t.Run(g.Family, func(t *testing.T) {
			goldenPath := filepath.Join(repoRoot(t), "testdata", "render-goldens", g.File)
			raw, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			sum := sha256.Sum256(raw)
			if hex.EncodeToString(sum[:]) != g.SHA256 {
				t.Fatalf("golden %s sha256 mismatch: file was edited without updating manifest.json", g.File)
			}
			actual := filepath.Join(t.TempDir(), g.File)
			renderFixture(t, bin, g.Fixture, m, actual)
			diffs, first := comparePixels(decodePNG(t, goldenPath), decodePNG(t, actual))
			if diffs != 0 {
				t.Fatalf("%s: %d pixel(s) differ from the committed golden (first at %v); "+
					"if this engine change is deliberate, re-record with "+
					"GOOSIE_RECORD_GOLDENS=1 go test ./test/goldens/ -run TestRecordGoldens",
					g.Fixture, diffs, first)
			}
		})
	}
}

// TestGoldenManifestCoversFamilies keeps the ratchet honest: every layout
// family the spec names must keep a golden, and every fixture must exist.
func TestGoldenManifestCoversFamilies(t *testing.T) {
	m := loadManifest(t)
	want := []string{"block", "inline", "float", "flex", "grid", "table", "positioning"}
	seen := map[string]bool{}
	for _, g := range m.Goldens {
		seen[g.Family] = true
		if _, err := os.Stat(filepath.Join(repoRoot(t), "testdata", "render", g.Fixture)); err != nil {
			t.Errorf("golden fixture missing: %s", g.Fixture)
		}
	}
	for _, family := range want {
		if !seen[family] {
			t.Errorf("layout family %q lost its golden", family)
		}
	}
}

// TestGoldenComparatorDetectsOnePixel is the V0 acceptance: a deliberate
// one-pixel change must be caught. It perturbs a decoded golden in memory;
// no committed file is touched.
func TestGoldenComparatorDetectsOnePixel(t *testing.T) {
	m := loadManifest(t)
	if len(m.Goldens) == 0 {
		t.Fatal("golden manifest is empty")
	}
	g := m.Goldens[0]
	img := decodePNG(t, filepath.Join(repoRoot(t), "testdata", "render-goldens", g.File))
	b := img.Bounds()
	px := image.Point{X: b.Min.X + b.Dx()/2, Y: b.Min.Y + b.Dy()/2}
	r, g2, bl, a := img.At(px.X, px.Y).RGBA()
	perturbed := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			perturbed.Set(x, y, img.At(x, y))
		}
	}
	// Flip one channel by one 8-bit step; keep alpha opaque.
	perturbed.Set(px.X, px.Y, rgba64{r: r ^ 0x100, g: g2, b: bl, a: a})
	diffs, first := comparePixels(img, perturbed)
	if diffs != 1 || first != px {
		t.Fatalf("one-pixel perturbation: diffs=%d first=%v, want 1 at %v", diffs, first, px)
	}
}

type rgba64 struct{ r, g, b, a uint32 }

func (c rgba64) RGBA() (uint32, uint32, uint32, uint32) { return c.r, c.g, c.b, c.a }

// TestRenderIsDeterministic guards the golden premise: two renders of the
// same fixture with system fonts off must be byte-identical.
func TestRenderIsDeterministic(t *testing.T) {
	bin := headlessBinary(t)
	m := loadManifest(t)
	g := m.Goldens[0]
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	renderFixture(t, bin, g.Fixture, m, a)
	renderFixture(t, bin, g.Fixture, m, b)
	ra, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ra, rb) {
		t.Fatal("two embedded-font renders of the same fixture differ; goldens cannot be a binary-only function")
	}
}

// TestRecordGoldens re-records the committed baselines. It only runs when
// explicitly asked, so a normal test run can never overwrite evidence.
func TestRecordGoldens(t *testing.T) {
	if os.Getenv("GOOSIE_RECORD_GOLDENS") != "1" {
		t.Skip("set GOOSIE_RECORD_GOLDENS=1 to re-record the golden baselines")
	}
	bin := headlessBinary(t)
	m := loadManifest(t)
	dir := filepath.Join(repoRoot(t), "testdata", "render-goldens")
	for i := range m.Goldens {
		g := &m.Goldens[i]
		out := filepath.Join(dir, g.File)
		renderFixture(t, bin, g.Fixture, m, out)
		raw, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		g.SHA256 = hex.EncodeToString(sum[:])
		t.Logf("recorded %s (%s)", g.File, g.Fixture)
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
