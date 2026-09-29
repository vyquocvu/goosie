package wpt

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner executes WPT tests against goosie's headless renderer and collects
// results. It drives the goosie binary in screenshot mode for each test,
// then compares the output to a reference image.
type Runner struct {
	cfg     Config
	server  *Server
	support FeatureSupport
}

// NewRunner creates a Runner with the given configuration.
func NewRunner(cfg Config) *Runner {
	return &Runner{
		cfg:     cfg,
		support: GoosieSupport(),
	}
}

// Run executes all curated WPT tests and returns the aggregate results.
func (r *Runner) Run() (*SuiteResult, error) {
	startTime := time.Now()
	result := &SuiteResult{
		Commit:    r.cfg.Commit,
		Timestamp: startTime.Format(time.RFC3339),
	}

	// Ensure output directories exist.
	if err := os.MkdirAll(r.cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("wpt: create output dir: %w", err)
	}

	// Start the WPT HTTP server.
	wptDir := r.cfg.WPTCheckoutDir()
	r.server = NewServer(wptDir)
	if err := r.server.Start(); err != nil {
		return nil, fmt.Errorf("wpt: start server: %w", err)
	}
	defer r.server.Stop()

	// Discover tests across all curated directories.
	curation := CuratedDirs(r.support)
	var allTests []DiscoveredTest
	for _, dir := range curation {
		// If the config specifies directories, only include those.
		if len(r.cfg.Directories) > 0 && !containsDir(r.cfg.Directories, dir.Path) {
			continue
		}
		tests, err := DiscoverTests(wptDir, dir.Path, r.cfg.Exclude)
		if err != nil {
			// Directory might not exist in this WPT checkout; skip silently.
			continue
		}
		for i := range tests {
			tests[i].FullDir = dir.Path
		}
		allTests = append(allTests, tests...)
	}

	result.Total = len(allTests)

	// Run each test.
	for _, dt := range allTests {
		tr := r.runOne(dt)
		result.Results = append(result.Results, tr)

		switch tr.Status {
		case "pass":
			result.Passed++
		case "fail":
			result.Failed++
		case "skip":
			result.Skipped++
		case "error", "timeout":
			result.Errored++
		}
	}

	result.Duration = time.Since(startTime).Seconds()

	// Calculate pass rate over non-skipped tests.
	runnable := result.Total - result.Skipped
	if runnable > 0 {
		result.PassRate = float64(result.Passed) / float64(runnable) * 100
	}

	return result, nil
}

// runOne executes a single WPT test and returns its result.
func (r *Runner) runOne(dt DiscoveredTest) TestResult {
	tr := TestResult{
		Path: dt.Path,
		Type: dt.Type,
	}

	// Check if this test should be skipped.
	if skip, reason := ShouldSkip(dt, r.support); skip {
		tr.Status = "skip"
		tr.Message = reason
		return tr
	}

	// Only reftests are runnable.
	if dt.Type != RefTest {
		tr.Status = "skip"
		tr.Message = fmt.Sprintf("unsupported test type: %s", dt.Type)
		return tr
	}

	if dt.RefPath == "" {
		tr.Status = "skip"
		tr.Message = "no reference file found"
		return tr
	}

	// Build the test URL.
	testURL := r.server.URL() + "/" + strings.ReplaceAll(dt.Path, string(os.PathSeparator), "/")

	// Render the test page.
	testPNG := filepath.Join(r.cfg.OutputDir, "test", dt.Path+".png")
	testPNG = strings.TrimSuffix(testPNG, ".html.png")
	testPNG = strings.TrimSuffix(testPNG, ".htm.png")
	testPNG += ".png"

	if err := os.MkdirAll(filepath.Dir(testPNG), 0o755); err != nil {
		tr.Status = "error"
		tr.Message = fmt.Sprintf("mkdir: %v", err)
		return tr
	}

	startRender := time.Now()
	err := r.renderPage(testURL, testPNG)
	tr.DurationMS = time.Since(startRender).Milliseconds()

	if err != nil {
		tr.Status = "error"
		tr.Message = fmt.Sprintf("render: %v", err)
		return tr
	}

	// Render the reference page.
	refURL := r.server.URL() + "/" + strings.ReplaceAll(dt.RefPath, string(os.PathSeparator), "/")
	// The ref path might be outside the WPT root if it was relative and weird;
	// check that it resolves within the root.
	absRef, _ := filepath.Abs(dt.RefPath)
	absRoot, _ := filepath.Abs(r.cfg.WPTCheckoutDir())
	if !strings.HasPrefix(absRef, absRoot) {
		tr.Status = "error"
		tr.Message = "reference path escapes WPT root"
		return tr
	}

	refRelPath, _ := filepath.Rel(r.cfg.WPTCheckoutDir(), dt.RefPath)
	refURL = r.server.URL() + "/" + strings.ReplaceAll(refRelPath, string(os.PathSeparator), "/")

	refPNG := filepath.Join(r.cfg.OutputDir, "ref", dt.Path+".png")
	refPNG = strings.TrimSuffix(refPNG, ".html.png")
	refPNG = strings.TrimSuffix(refPNG, ".htm.png")
	refPNG += ".png"

	if err := os.MkdirAll(filepath.Dir(refPNG), 0o755); err != nil {
		tr.Status = "error"
		tr.Message = fmt.Sprintf("mkdir ref: %v", err)
		return tr
	}

	if err := r.renderPage(refURL, refPNG); err != nil {
		tr.Status = "error"
		tr.Message = fmt.Sprintf("render ref: %v", err)
		return tr
	}

	// Compare the two renders.
	score, err := compareImages(testPNG, refPNG)
	if err != nil {
		tr.Status = "error"
		tr.Message = fmt.Sprintf("compare: %v", err)
		return tr
	}

	tr.Score = score
	tr.RefPath = dt.RefPath
	tr.RefRelation = dt.RefRelation

	// Determine pass/fail based on the relation type and threshold.
	isMatch := dt.RefRelation == "match" || dt.RefRelation == ""
	if isMatch {
		if score >= r.cfg.PassThreshold {
			tr.Status = "pass"
		} else {
			tr.Status = "fail"
			tr.Message = fmt.Sprintf("pixel match %.1f%% < threshold %.1f%%", score, r.cfg.PassThreshold)
		}
	} else {
		// mismatch: the test should NOT look the same.
		if score < r.cfg.PassThreshold {
			tr.Status = "pass"
		} else {
			tr.Status = "fail"
			tr.Message = fmt.Sprintf("pixel match %.1f%% >= threshold %.1f%% (expected mismatch)", score, r.cfg.PassThreshold)
		}
	}

	return tr
}

// renderPage uses the goosie binary in screenshot mode to render a URL to PNG.
func (r *Runner) renderPage(url, outPath string) error {
	binary := r.cfg.GoosieBinary
	if binary == "" {
		binary = "./goosie"
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.cfg.Timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary,
		"-backend", "headless",
		"-width", fmt.Sprintf("%d", r.cfg.ViewportWidth),
		"-height", fmt.Sprintf("%d", r.cfg.ViewportHeight),
		"-dpr", fmt.Sprintf("%.1f", r.cfg.DPR),
		"-screenshot",
		"-out", outPath,
		"-url", url,
	)
	output, err := cmd.CombinedOutput()

	// Check if the output file was created.
	if _, statErr := os.Stat(outPath); os.IsNotExist(statErr) {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("render timed out after %ds", r.cfg.Timeout)
		}
		return fmt.Errorf("screenshot not created: %s (output: %s, error: %v)", outPath, string(output), err)
	}
	return nil
}

// compareImages computes the pixel-match percentage between two PNG images.
// The algorithm matches testdata/parity.py: every RGB channel must differ by
// at most 30 for a pixel to count as matching.
func compareImages(pathA, pathB string) (float64, error) {
	imgA, err := loadPNG(pathA)
	if err != nil {
		return 0, fmt.Errorf("load %s: %w", pathA, err)
	}
	imgB, err := loadPNG(pathB)
	if err != nil {
		return 0, fmt.Errorf("load %s: %w", pathB, err)
	}

	boundsA := imgA.Bounds()
	boundsB := imgB.Bounds()

	// If sizes differ, compare the overlapping region.
	w := min(boundsA.Dx(), boundsB.Dx())
	h := min(boundsA.Dy(), boundsB.Dy())
	if w == 0 || h == 0 {
		return 0, nil
	}

	const threshold = 30
	matched := 0
	total := w * h

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r1, g1, b1, _ := imgA.At(boundsA.Min.X+x, boundsA.Min.Y+y).RGBA()
			r2, g2, b2, _ := imgB.At(boundsB.Min.X+x, boundsB.Min.Y+y).RGBA()

			// RGBA() returns 16-bit values; scale threshold to match.
			dr := absDiff(int(r1>>8), int(r2>>8))
			dg := absDiff(int(g1>>8), int(g2>>8))
			db := absDiff(int(b1>>8), int(b2>>8))

			if dr <= threshold && dg <= threshold && db <= threshold {
				matched++
			}
		}
	}

	return float64(matched) / float64(total) * 100, nil
}

// loadPNG reads a PNG file and returns the image.
func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func absDiff(a, b int) int {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// containsDir checks if a directory list contains the given path.
func containsDir(dirs []string, path string) bool {
	for _, d := range dirs {
		if d == path || strings.HasPrefix(path, d+"/") {
			return true
		}
	}
	return false
}

// RenderPage is the exported wrapper for rendering a single page. It is used
// by the reference generation mode and by tests.
func (r *Runner) RenderPage(url, outPath string) error {
	return r.renderPage(url, outPath)
}

// CompareImages is the exported wrapper for image comparison.
func CompareImages(pathA, pathB string) (float64, error) {
	return compareImages(pathA, pathB)
}

// PixelScore computes the match percentage and returns it rounded to one decimal.
func PixelScore(score float64) float64 {
	return math.Round(score*10) / 10
}
