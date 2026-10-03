// Package wpt runs curated Web Platform Tests against goosie's headless renderer.
//
// WPT is the canonical spec-compliance suite for the web platform. It contains
// three kinds of tests: testharness.js (JavaScript assertions), reftests (pixel
// comparison against a reference), and manual tests (human-driven). This harness
// drives goosie in headless screenshot mode and scores pixels, so it only runs
// reftests and parse-only tests: it never collects the testharness.js
// result-reporting protocol those assertions publish through. This package
// curates a subset of WPT directories that exercise HTML parsing and CSS
// rendering and measures how many of those tests produce pixel-identical output
// to a reference browser.
//
// The runner works by:
//  1. Downloading (or reusing) a pinned WPT checkout
//  2. Serving the test files over HTTP (WPT requires a server for resource paths)
//  3. Rendering each test page with goosie's headless screenshot mode
//  4. Comparing the render to a reference image (from Chromium via Playwright, or
//     a pre-generated golden)
//  5. Reporting pass/fail/skip as JSON and an HTML dashboard
package wpt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultWPTCommit is the pinned WPT revision the harness tests against.
// Pinning prevents surprise regressions when WPT updates a test expectation.
const DefaultWPTCommit = "11ae8c1179be2dd38070846b01c2e9338ef81770"

// WPTRepo is the upstream repository URL.
const WPTRepo = "https://github.com/web-platform-tests/wpt"

// TestType classifies a WPT test by its execution model.
type TestType string

const (
	// TestHarness is a testharness.js test: JavaScript assertions that report
	// pass/fail through the testharness API. Running them means collecting that
	// reporting protocol, which this screenshot-based harness does not do.
	TestHarness TestType = "testharness"

	// RefTest is a reftest: the test page is rendered and compared pixel-by-pixel
	// to a reference page. This is the primary test type goosie can run.
	RefTest TestType = "reftest"

	// ManualTest requires human interaction and is never automated.
	ManualTest TestType = "manual"

	// VisualTest is a screenshot-only test with no automated pass/fail.
	VisualTest TestType = "visual"
)

// TestResult is the outcome of running one WPT test.
type TestResult struct {
	// Path is the WPT-relative path, e.g. "css/css-color/parsing/color-001.html".
	Path string `json:"path"`
	// Type is the test classification.
	Type TestType `json:"type"`
	// Status is the outcome: pass, fail, skip, error, timeout.
	Status string `json:"status"`
	// Score is the pixel-match percentage for reftests (0-100).
	Score float64 `json:"score,omitempty"`
	// Message describes failures or skip reasons.
	Message string `json:"message,omitempty"`
	// DurationMS is how long the test took to render.
	DurationMS int64 `json:"duration_ms,omitempty"`
	// RefPath is the reference file path for reftests.
	RefPath string `json:"ref_path,omitempty"`
	// RefRelation is "match" or "mismatch".
	RefRelation string `json:"ref_relation,omitempty"`
}

// SuiteResult is the aggregate outcome of a WPT test run.
type SuiteResult struct {
	// Commit is the WPT revision that was tested.
	Commit string `json:"commit"`
	// Timestamp is when the run started (ISO 8601).
	Timestamp string `json:"timestamp"`
	// Total is the number of tests discovered.
	Total int `json:"total"`
	// Passed is the number of tests that passed.
	Passed int `json:"passed"`
	// Failed is the number of tests that failed.
	Failed int `json:"failed"`
	// Skipped is the number of tests skipped (unsupported feature).
	Skipped int `json:"skipped"`
	// Errored is the number of tests that encountered an error.
	Errored int `json:"errored"`
	// PassRate is the percentage of non-skipped tests that passed.
	PassRate float64 `json:"pass_rate"`
	// Results are the per-test outcomes.
	Results []TestResult `json:"results"`
	// Duration is the total run duration in seconds.
	Duration float64 `json:"duration_s"`
}

// Config is the WPT harness configuration, loaded from testdata/wpt-config.json.
type Config struct {
	// WPTDir is the local path to the WPT checkout. If empty, the harness
	// downloads to a temp directory.
	WPTDir string `json:"wpt_dir"`
	// Commit pins the WPT revision. Empty means use DefaultWPTCommit.
	Commit string `json:"commit"`
	// Directories lists the WPT subdirectories to test, e.g. ["css/css-color"].
	Directories []string `json:"directories"`
	// Exclude lists glob patterns for tests to skip.
	Exclude []string `json:"exclude"`
	// PassThreshold is the pixel-match percentage required for a reftest to pass.
	// Defaults to 95.0.
	PassThreshold float64 `json:"pass_threshold"`
	// ViewportWidth is the viewport width in CSS pixels for rendering.
	ViewportWidth int `json:"viewport_width"`
	// ViewportHeight is the viewport height in CSS pixels for rendering.
	ViewportHeight int `json:"viewport_height"`
	// DPR is the device pixel ratio for rendering.
	DPR float64 `json:"dpr"`
	// Timeout is the per-test timeout in seconds.
	Timeout int `json:"timeout"`
	// GoosieBinary is the path to the goosie binary. If empty, uses ./goosie.
	GoosieBinary string `json:"goosie_binary"`
	// OutputDir is where render artifacts are written.
	OutputDir string `json:"output_dir"`
	// ReferenceDir is where reference renders from Chromium live.
	ReferenceDir string `json:"reference_dir"`
	// GenerateReferences renders references with goosie itself (for bootstrapping
	// the golden set when no Chromium renders are available).
	GenerateReferences bool `json:"generate_references"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Commit:         DefaultWPTCommit,
		PassThreshold:  95.0,
		ViewportWidth:  800,
		ViewportHeight: 600,
		DPR:            1.0,
		Timeout:        30,
		GoosieBinary:   "./goosie",
		OutputDir:      "/tmp/wpt-results",
		ReferenceDir:   "/tmp/wpt-references",
	}
}

// LoadConfig reads a Config from a JSON file.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("wpt: read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("wpt: parse config %s: %w", path, err)
	}
	if cfg.Commit == "" {
		cfg.Commit = DefaultWPTCommit
	}
	if cfg.PassThreshold <= 0 {
		cfg.PassThreshold = 95.0
	}
	if cfg.ViewportWidth <= 0 {
		cfg.ViewportWidth = 800
	}
	if cfg.ViewportHeight <= 0 {
		cfg.ViewportHeight = 600
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30
	}
	return cfg, nil
}

// WPTDir returns the effective WPT checkout directory, creating it if needed.
func (c *Config) WPTCheckoutDir() string {
	if c.WPTDir != "" {
		return c.WPTDir
	}
	return filepath.Join(os.TempDir(), "goosie-wpt")
}
