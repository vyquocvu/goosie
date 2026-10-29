// Package wpt runs curated Web Platform Tests against Goosie's engine.
//
// WPT is the canonical spec-compliance suite for the web platform. It contains
// three kinds of tests: testharness.js (JavaScript assertions), reftests (pixel
// comparison against a reference), and manual tests (human-driven). This harness
// executes testharness.js assertions in Goosie's DOM/JS engine and runs reftests
// through the headless renderer. Manual tests remain human-driven. The configured
// directories are a partial conformance set; reports include test counts and
// skip reasons.
//
// The runner works by:
//  1. Downloading (or reusing) a pinned WPT checkout
//  2. Serving the test files over HTTP (WPT requires a server for resource paths)
//  3. Executing testharness pages in Goosie's engine or rendering reftests
//  4. Comparing the render to a reference image (from Chromium via Playwright, or
//     a pre-generated golden)
//  5. Reporting pass/fail/skip/error/timeout as JSON and an HTML dashboard
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
	// pass/fail through the testharness API.
	TestHarness TestType = "testharness"

	// RefTest is a reftest: the test page is rendered and compared pixel-by-pixel
	// to a reference page. This is the primary test type goosie can run.
	RefTest TestType = "reftest"

	// ManualTest requires human interaction and is never automated.
	ManualTest TestType = "manual"

	// SmokeTest is a crash-regression page whose oracle is that a bounded
	// headless document load completes without crashing the engine process.
	SmokeTest TestType = "smoke"

	// SupportFile is an unmarked helper document referenced by an automated
	// test, rather than an independent WPT case.
	SupportFile TestType = "support"

	// UnclassifiedTest is a discovered page without an automated test marker.
	// Keep it visible in reports until it is explicitly classified as a test or
	// documented as non-test content.
	UnclassifiedTest TestType = "unclassified"

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
	// Variants records each configured testharness query variant.
	Variants []TestVariantResult `json:"variants,omitempty"`
}

// TestVariantResult records one query-selected testharness execution.
type TestVariantResult struct {
	Query      string `json:"query"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
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

// MeetsMinPassRate reports whether the suite's pass rate clears a floor.
// An empty run never meets any positive floor: zero discovered tests is a
// harness failure, not a passing suite. A non-positive floor disables the
// check so ad-hoc runs can still report without gating.
func (r SuiteResult) MeetsMinPassRate(min float64) bool {
	if min <= 0 {
		return true
	}
	if r.Total-r.Skipped <= 0 {
		return false
	}
	return r.PassRate >= min
}

// CheckSuiteThreshold enforces the suite-level pass-rate floor, returning a
// descriptive error when the run lands below it. A non-positive floor
// disables the check for ad-hoc runs. Empty runs report through
// MeetsMinPassRate as failures; callers that skip those explicitly do so
// before calling here.
func CheckSuiteThreshold(result SuiteResult, minRate float64) error {
	if minRate <= 0 {
		return nil
	}
	if result.MeetsMinPassRate(minRate) {
		return nil
	}
	return fmt.Errorf("wpt: pass rate %.1f%% below floor %.1f%% (%d/%d passed, %d failed, %d errored)",
		result.PassRate, minRate, result.Passed, result.Total-result.Skipped,
		result.Failed, result.Errored)
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
	// Include restricts discovery to paths or glob patterns relative to the WPT
	// root. Empty means the configured directories are used in full.
	Include []string `json:"include"`
	// TestVariants selects query-string variants for parameterized testharness
	// files, keyed by WPT-relative path.
	TestVariants map[string][]string `json:"test_variants"`
	// Exclude lists glob patterns for tests to skip.
	Exclude []string `json:"exclude"`
	// PassThreshold is the pixel-match percentage required for a reftest to pass.
	// Defaults to 95.0.
	PassThreshold float64 `json:"pass_threshold"`
	// MinPassRate is the suite-level pass-rate floor: TestWPTSuite fails when
	// the run's PassRate lands below it. Zero (or absent) disables the gate
	// for ad-hoc runs. Raise it as rendering support grows; never lower it to
	// make a red run green.
	MinPassRate float64 `json:"min_pass_rate"`
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
