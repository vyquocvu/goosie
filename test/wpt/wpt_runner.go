package wpt

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Runner executes WPT tests against Goosie's document engine or headless
// renderer and collects assertion or pixel-comparison results.
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

// Run executes the configured WPT directories, or the default curated set
// when no explicit directory list is supplied, and returns aggregate results.
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

	// Discover tests across the selected directories. An explicit directory
	// list is an override, so full-suite runs can name WPT roots that are not
	// part of the default curated catalog (for example, html and dom).
	curation, err := selectedWPTDirectories(r.cfg.Directories, r.support)
	if err != nil {
		return nil, err
	}
	allTests, err := discoverWPTTests(wptDir, curation, r.cfg.Exclude, r.cfg.Include, len(r.cfg.Directories) > 0)
	if err != nil {
		return nil, err
	}
	if len(r.cfg.Include) > 0 && len(allTests) == 0 {
		return nil, fmt.Errorf("wpt: include patterns matched no tests: %s", strings.Join(r.cfg.Include, ", "))
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

// includedTest reports whether a discovered WPT path matches one of the
// configured allowlist patterns. Patterns may match the full WPT-relative path
// or just the file name, which keeps focused runs concise.
func includedTest(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	base := filepath.Base(path)
	for _, pattern := range patterns {
		if path == pattern || base == pattern {
			return true
		}
		if matched, err := filepath.Match(pattern, path); err == nil && matched {
			return true
		}
		if matched, err := filepath.Match(pattern, base); err == nil && matched {
			return true
		}
	}
	return false
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

	if dt.Type == TestHarness {
		return r.runTestHarness(dt)
	}
	if dt.Type == SmokeTest {
		return r.runSmokeTest(dt)
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

	// A match relation uses the configured similarity threshold. A mismatch
	// relation only requires that the two images differ after per-pixel
	// tolerance; the match threshold is not a minimum-difference requirement.
	if reftestRelationPasses(score, dt.RefRelation, r.cfg.PassThreshold) {
		tr.Status = "pass"
	} else if dt.RefRelation == "mismatch" {
		tr.Status = "fail"
		tr.Message = fmt.Sprintf("pixel match %.1f%% (expected mismatch)", score)
	} else if maxDiff, totalAllowed, ok := readFuzzyMeta(dt.AbsPath); ok && fuzzyPasses(testPNG, refPNG, maxDiff, totalAllowed) {
		tr.Status = "pass"
		tr.Message = fmt.Sprintf("fuzzy maxDifference=%d totalPixels=%d", maxDiff, totalAllowed)
	} else {
		tr.Status = "fail"
		tr.Message = fmt.Sprintf("pixel match %.1f%% < threshold %.1f%%", score, r.cfg.PassThreshold)
	}

	return tr
}

// runSmokeTest executes a WPT crash regression through the configured headless
// renderer. These pages do not contain an assertion harness; process exit from
// the bounded render is the documented no-crash oracle.
func (r *Runner) runSmokeTest(dt DiscoveredTest) TestResult {
	tr := TestResult{Path: dt.Path, Type: dt.Type}
	if r.server == nil {
		tr.Status = "error"
		tr.Message = "WPT server is not running"
		return tr
	}
	testURL := r.server.URL() + "/" + strings.ReplaceAll(dt.Path, string(os.PathSeparator), "/")
	outPath := filepath.Join(r.cfg.OutputDir, "smoke", dt.Path)
	outPath = strings.TrimSuffix(outPath, ".html") + ".png"
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		tr.Status = "error"
		tr.Message = "create smoke output directory: " + err.Error()
		return tr
	}
	started := time.Now()
	if err := r.renderPage(testURL, outPath); err != nil {
		tr.Status = "error"
		tr.Message = "crash-regression render failed: " + err.Error()
		return tr
	}
	tr.Status = "pass"
	tr.DurationMS = time.Since(started).Milliseconds()
	tr.Message = "bounded headless render completed; no-crash smoke oracle"
	return tr
}

func reftestRelationPasses(score float64, relation string, threshold float64) bool {
	if relation == "mismatch" {
		return score < 100
	}
	return score >= threshold
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

// readFuzzyMeta parses the test's `<meta name="fuzzy">` allowance, e.g.
// `content="maxDifference=0-96;totalPixels=0-400"`. Multiple metas combine
// by taking the most lenient allowance of each kind. ok is false when the
// file declares nothing parseable.
func readFuzzyMeta(testPath string) (maxDiff, totalAllowed int, ok bool) {
	data, err := os.ReadFile(testPath)
	if err != nil {
		return 0, 0, false
	}
	// Find meta tags naming fuzzy (quoted or not), then read the content
	// value of each. RE2 has no backreferences, so the name check compares
	// the unquoted value instead.
	tagRe := regexp.MustCompile(`(?i)<meta[^>]*>`)
	attrRe := regexp.MustCompile(`(?i)\b(name|content)\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	for _, tag := range tagRe.FindAllString(string(data), -1) {
		name, content := "", ""
		for _, m := range attrRe.FindAllStringSubmatch(tag, -1) {
			val := m[3]
			if val == "" {
				val = m[4]
			}
			if val == "" {
				val = m[5]
			}
			if strings.EqualFold(m[1], "name") {
				name = strings.ToLower(val)
			} else {
				content = val
			}
		}
		if name != "fuzzy" || content == "" {
			continue
		}
		for _, part := range strings.Split(content, ";") {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) != 2 {
				continue
			}
			lo, hi, found := strings.Cut(strings.TrimSpace(kv[1]), "-")
			if !found {
				continue
			}
			_ = lo
			n, err := strconv.Atoi(strings.TrimSpace(hi))
			if err != nil {
				continue
			}
			switch strings.TrimSpace(strings.ToLower(kv[0])) {
			case "maxdifference":
				if n > maxDiff {
					maxDiff = n
				}
				ok = true
			case "totalpixels":
				if n > totalAllowed {
					totalAllowed = n
				}
				ok = true
			}
		}
	}
	return maxDiff, totalAllowed, ok
}

// fuzzyPasses recounts the comparison with the test's own tolerance: at
// most totalAllowed pixels may differ by more than maxDiff in any channel.
// This is the WPT reftest rule for anti-aliasing and rounding slop, applied
// only when the strict threshold already failed.
func fuzzyPasses(pathA, pathB string, maxDiff, totalAllowed int) bool {
	imgA, err := loadPNG(pathA)
	if err != nil {
		return false
	}
	imgB, err := loadPNG(pathB)
	if err != nil {
		return false
	}
	boundsA := imgA.Bounds()
	boundsB := imgB.Bounds()
	w := min(boundsA.Dx(), boundsB.Dx())
	h := min(boundsA.Dy(), boundsB.Dy())
	if w == 0 || h == 0 {
		return false
	}
	over := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r1, g1, b1, _ := imgA.At(boundsA.Min.X+x, boundsA.Min.Y+y).RGBA()
			r2, g2, b2, _ := imgB.At(boundsB.Min.X+x, boundsB.Min.Y+y).RGBA()
			d := absDiff(int(r1>>8), int(r2>>8))
			if v := absDiff(int(g1>>8), int(g2>>8)); v > d {
				d = v
			}
			if v := absDiff(int(b1>>8), int(b2>>8)); v > d {
				d = v
			}
			if d > maxDiff {
				over++
				if over > totalAllowed {
					return false
				}
			}
		}
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func selectedWPTDirectories(configured []string, support FeatureSupport) ([]CuratedDir, error) {
	if len(configured) == 0 {
		return CuratedDirs(support), nil
	}
	selected := make([]CuratedDir, 0, len(configured))
	seen := make(map[string]struct{}, len(configured))
	for _, directory := range configured {
		directory = path.Clean(strings.TrimSpace(directory))
		if directory == "." || directory == "" || path.IsAbs(directory) || directory == ".." || strings.HasPrefix(directory, "../") {
			return nil, fmt.Errorf("wpt: invalid configured directory %q", directory)
		}
		if _, duplicate := seen[directory]; duplicate {
			continue
		}
		seen[directory] = struct{}{}
		selected = append(selected, CuratedDir{Path: directory, Description: directory})
	}
	return selected, nil
}

func discoverWPTTests(wptRoot string, directories []CuratedDir, excludes, includes []string, strictDirectories bool) ([]DiscoveredTest, error) {
	var allTests []DiscoveredTest
	for _, directory := range directories {
		tests, err := DiscoverTests(wptRoot, directory.Path, excludes)
		if err != nil {
			if strictDirectories {
				return nil, fmt.Errorf("wpt: discover configured directory %q: %w", directory.Path, err)
			}
			continue
		}
		for i := range tests {
			tests[i].FullDir = directory.Path
		}
		for _, test := range tests {
			if includedTest(test.Path, includes) {
				allTests = append(allTests, test)
			}
		}
	}
	return allTests, nil
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
