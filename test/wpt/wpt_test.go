package wpt

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestWPTSuite runs the curated WPT tests against goosie. This is the main
// entry point for CI integration. It discovers tests, renders them, compares
// to references, and reports results.
//
// The test requires:
//   - A built goosie binary (at ./goosie or configured via WPT_GOOSIE_BINARY)
//   - A WPT checkout (at /tmp/goosie-wpt or configured via WPT_DIR)
//   - Reference renders (at /tmp/wpt-references or configured via WPT_REF_DIR)
//
// Environment variables:
//   - WPT_GOOSIE_BINARY: path to the goosie binary
//   - WPT_DIR: path to a WPT checkout (skips download)
//   - WPT_CONFIG: path to wpt-config.json
//   - WPT_DIRS: comma-separated list of WPT directories to test (overrides config)
//   - WPT_SKIP_DOWNLOAD: if "1", skip the WPT download (requires WPT_DIR)
func TestWPTSuite(t *testing.T) {
	cfg := loadTestConfig(t)

	// Ensure we have a WPT checkout.
	checkout := NewCheckout(cfg)
	wptDir, err := checkout.Ensure()
	if err != nil {
		t.Fatalf("wpt checkout: %v", err)
	}
	cfg.WPTDir = wptDir

	// Run the suite.
	runner := NewRunner(cfg)
	result, err := runner.Run()
	if err != nil {
		t.Fatalf("wpt run: %v", err)
	}

	// Write reports.
	reporter := NewReporter(result)
	reporter.PrintSummary(os.Stderr)

	jsonPath := filepath.Join(cfg.OutputDir, "results.json")
	if err := reporter.WriteJSONFile(jsonPath); err != nil {
		t.Logf("wpt: write JSON report: %v", err)
	} else {
		t.Logf("wpt: JSON report: %s", jsonPath)
	}

	htmlPath := filepath.Join(cfg.OutputDir, "report.html")
	if err := reporter.WriteHTMLFile(htmlPath); err != nil {
		t.Logf("wpt: write HTML report: %v", err)
	} else {
		t.Logf("wpt: HTML report: %s", htmlPath)
	}

	// The test passes if we have a positive pass rate. Individual test failures
	// are reported but do not fail the suite (yet - as goosie's CSS support
	// grows, the threshold should increase).
	if result.Total == 0 {
		t.Skip("no WPT tests discovered")
	}
	if result.Passed == 0 && result.Failed > 0 {
		t.Errorf("wpt: 0/%d tests passed (%.1f%% pass rate)",
			result.Total-result.Skipped, result.PassRate)
	}

	t.Logf("wpt: %d/%d passed (%.1f%% pass rate), %d skipped, %d errored",
		result.Passed, result.Total-result.Skipped, result.PassRate,
		result.Skipped, result.Errored)
}

// TestWPTCurator verifies that the curator correctly classifies tests.
func TestWPTCurator(t *testing.T) {
	support := GoosieSupport()
	dirs := CuratedDirs(support)

	if len(dirs) == 0 {
		t.Fatal("no curated directories")
	}

	// Verify each curated directory has a description.
	for _, d := range dirs {
		if d.Path == "" {
			t.Error("curated dir has empty path")
		}
		if d.Description == "" {
			t.Errorf("curated dir %q has empty description", d.Path)
		}
	}

	// Verify the support flags are consistent.
	if !support.HTMLBasic {
		t.Error("HTMLBasic should be supported")
	}
	if !support.CSSBasic {
		t.Error("CSSBasic should be supported")
	}
	if support.Canvas {
		t.Error("Canvas should not be supported yet")
	}
	if support.WebGL {
		t.Error("WebGL should not be supported yet")
	}
}

// TestWPTServer verifies the HTTP server starts and serves files.
func TestWPTServer(t *testing.T) {
	// Create a temp directory with a test file.
	tmpDir := t.TempDir()
	testHTML := `<!DOCTYPE html><html><body><h1>Test</h1></body></html>`
	if err := os.WriteFile(filepath.Join(tmpDir, "test.html"), []byte(testHTML), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(tmpDir)
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Stop()

	url := srv.URL()
	if url == "" {
		t.Fatal("server URL is empty")
	}
	if srv.Port() == 0 {
		t.Fatal("server port is 0")
	}
}

// TestWPTImageCompare verifies the pixel comparison function.
func TestWPTImageCompare(t *testing.T) {
	// Create two identical small PNGs and verify 100% match.
	tmpDir := t.TempDir()
	imgA := filepath.Join(tmpDir, "a.png")
	imgB := filepath.Join(tmpDir, "b.png")

	// Create a simple 2x2 red PNG.
	writeTestPNG(t, imgA, 2, 2, [4]byte{255, 0, 0, 255})
	writeTestPNG(t, imgB, 2, 2, [4]byte{255, 0, 0, 255})

	score, err := CompareImages(imgA, imgB)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if score != 100.0 {
		t.Errorf("identical images: got %.1f%%, want 100%%", score)
	}

	// Create a different image and verify < 100%.
	imgC := filepath.Join(tmpDir, "c.png")
	writeTestPNG(t, imgC, 2, 2, [4]byte{0, 255, 0, 255})

	score2, err := CompareImages(imgA, imgC)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if score2 >= 100.0 {
		t.Errorf("different images: got %.1f%%, want < 100%%", score2)
	}
}

// TestWPTConfigDefaults verifies the default configuration.
func TestWPTConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.PassThreshold != 95.0 {
		t.Errorf("PassThreshold = %f, want 95.0", cfg.PassThreshold)
	}
	if cfg.ViewportWidth != 800 {
		t.Errorf("ViewportWidth = %d, want 800", cfg.ViewportWidth)
	}
	if cfg.ViewportHeight != 600 {
		t.Errorf("ViewportHeight = %d, want 600", cfg.ViewportHeight)
	}
	if cfg.Commit != DefaultWPTCommit {
		t.Errorf("Commit = %q, want %q", cfg.Commit, DefaultWPTCommit)
	}
}

// TestWPTConfigLoad verifies loading a config from JSON.
func TestWPTConfigLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	cfgJSON := `{
		"directories": ["css/css-color"],
		"pass_threshold": 90.0,
		"viewport_width": 1024,
		"viewport_height": 768,
		"timeout": 60
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Directories) != 1 || cfg.Directories[0] != "css/css-color" {
		t.Errorf("Directories = %v, want [css/css-color]", cfg.Directories)
	}
	if cfg.PassThreshold != 90.0 {
		t.Errorf("PassThreshold = %f, want 90.0", cfg.PassThreshold)
	}
	if cfg.ViewportWidth != 1024 {
		t.Errorf("ViewportWidth = %d, want 1024", cfg.ViewportWidth)
	}
	if cfg.Timeout != 60 {
		t.Errorf("Timeout = %d, want 60", cfg.Timeout)
	}
}

// TestShouldSkip verifies the skip logic.
func TestShouldSkip(t *testing.T) {
	support := GoosieSupport()

	tests := []struct {
		dt       DiscoveredTest
		wantSkip bool
	}{
		{
			dt:       DiscoveredTest{Path: "css/css-color/test.html", Type: RefTest, FullDir: "css/css-color"},
			wantSkip: false,
		},
		{
			dt:       DiscoveredTest{Path: "html/canvas/test.html", Type: RefTest, FullDir: "html/canvas"},
			wantSkip: true,
		},
		{
			dt:       DiscoveredTest{Path: "css/css-grid/test.html", Type: RefTest, FullDir: "css/css-grid"},
			wantSkip: true,
		},
		{
			dt:       DiscoveredTest{Path: "css/test.html", Type: TestHarness, FullDir: "css"},
			wantSkip: true,
		},
	}

	for _, tt := range tests {
		skip, reason := ShouldSkip(tt.dt, support)
		if skip != tt.wantSkip {
			t.Errorf("ShouldSkip(%s) = %v, %v; want %v", tt.dt.Path, skip, reason, tt.wantSkip)
		}
	}
}

// loadTestConfig builds a Config from environment variables and the config file.
func loadTestConfig(t *testing.T) Config {
	t.Helper()

	cfgPath := os.Getenv("WPT_CONFIG")
	var cfg Config
	var err error

	if cfgPath != "" {
		cfg, err = LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
	} else {
		cfg = DefaultConfig()
	}

	// Environment overrides.
	if v := os.Getenv("WPT_GOOSIE_BINARY"); v != "" {
		cfg.GoosieBinary = v
	}
	if v := os.Getenv("WPT_DIR"); v != "" {
		cfg.WPTDir = v
	}
	if v := os.Getenv("WPT_DIRS"); v != "" {
		cfg.Directories = splitAndTrim(v)
	}
	if v := os.Getenv("WPT_OUTPUT_DIR"); v != "" {
		cfg.OutputDir = v
	}
	if v := os.Getenv("WPT_REF_DIR"); v != "" {
		cfg.ReferenceDir = v
	}

	// Try to find the config file in testdata if not explicitly set.
	if cfgPath == "" {
		// Look for testdata/wpt-config.json relative to the project root.
		candidates := []string{
			"../../testdata/wpt-config.json",
			"testdata/wpt-config.json",
		}
		for _, c := range candidates {
			if data, err := os.ReadFile(c); err == nil {
				var fileCfg Config
				if json.Unmarshal(data, &fileCfg) == nil {
					// Merge: file config provides defaults, env vars override.
					if len(fileCfg.Directories) > 0 && len(cfg.Directories) == 0 {
						cfg.Directories = fileCfg.Directories
					}
					if fileCfg.PassThreshold > 0 && cfg.PassThreshold == 95.0 {
						cfg.PassThreshold = fileCfg.PassThreshold
					}
				}
				break
			}
		}
	}

	return cfg
}

func splitAndTrim(s string) []string {
	var result []string
	for _, part := range splitComma(s) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// writeTestPNG creates a minimal PNG file for testing using the standard library.
func writeTestPNG(t *testing.T, path string, w, h int, c [4]byte) {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: c[0], G: c[1], B: c[2], A: c[3]})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
