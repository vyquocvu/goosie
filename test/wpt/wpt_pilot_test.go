package wpt

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWPTPilot runs a small set of locally-authored WPT-style reftests against
// goosie's headless renderer. This is the pilot: it validates the harness
// end-to-end without requiring a full WPT checkout download.
//
// The pilot fixtures live in testdata/wpt-pilot/ and follow the WPT reftest
// format: each test HTML file has a <link rel="match" href="..."> pointing to
// its reference file.
//
// This test requires the goosie binary to be built. Set WPT_GOOSIE_BINARY to
// point at it, or it defaults to ./goosie in the project root.
func TestWPTPilot(t *testing.T) {
	// Find the pilot directory.
	pilotDir := findPilotDir(t)

	// Find the goosie binary.
	binary := findGoosieBinary(t)

	// Build configuration for the pilot.
	cfg := DefaultConfig()
	cfg.WPTDir = pilotDir
	cfg.GoosieBinary = binary
	cfg.OutputDir = t.TempDir()
	cfg.PassThreshold = 90.0 // Slightly relaxed for the pilot.
	cfg.Directories = []string{
		"css/css-color",
		"css/css-backgrounds",
		"css/css-flexbox",
		"css/css-position",
		"html/semantics",
	}

	// Discover tests in the pilot directory.
	var allTests []DiscoveredTest
	for _, dir := range cfg.Directories {
		tests, err := DiscoverTests(pilotDir, dir, nil)
		if err != nil {
			t.Logf("pilot: skip %s: %v", dir, err)
			continue
		}
		for i := range tests {
			tests[i].FullDir = dir
		}
		allTests = append(allTests, tests...)
	}

	if len(allTests) == 0 {
		t.Skip("no pilot tests discovered")
	}

	t.Logf("pilot: discovered %d tests in %s", len(allTests), pilotDir)

	// Start the server.
	srv := NewServer(pilotDir)
	if err := srv.Start(); err != nil {
		t.Fatalf("pilot: start server: %v", err)
	}
	defer srv.Stop()

	runner := NewRunner(cfg)
	runner.server = srv

	// Run each test.
	var passed, failed, skipped, errored int
	for _, dt := range allTests {
		tr := runner.runOne(dt)

		switch tr.Status {
		case "pass":
			passed++
			t.Logf("  PASS: %s (%.1f%%)", tr.Path, tr.Score)
		case "fail":
			failed++
			t.Logf("  FAIL: %s (%.1f%%) - %s", tr.Path, tr.Score, tr.Message)
		case "skip":
			skipped++
			t.Logf("  SKIP: %s - %s", tr.Path, tr.Message)
		case "error", "timeout":
			errored++
			t.Logf("  ERROR: %s - %s", tr.Path, tr.Message)
		}
	}

	total := passed + failed + skipped + errored
	runnable := total - skipped
	passRate := 0.0
	if runnable > 0 {
		passRate = float64(passed) / float64(runnable) * 100
	}

	t.Logf("pilot results: %d/%d passed (%.1f%%), %d skipped, %d errored",
		passed, runnable, passRate, skipped, errored)

	if failed > 0 {
		t.Errorf("pilot: %d tests failed", failed)
	}
}

// TestWPTPilotDiscovery verifies that the pilot test discovery finds the
// expected tests without running them.
func TestWPTPilotDiscovery(t *testing.T) {
	pilotDir := findPilotDir(t)

	dirs := []string{
		"css/css-color",
		"css/css-backgrounds",
		"css/css-flexbox",
		"css/css-position",
		"html/semantics",
	}

	totalTests := 0
	for _, dir := range dirs {
		tests, err := DiscoverTests(pilotDir, dir, nil)
		if err != nil {
			t.Errorf("discover %s: %v", dir, err)
			continue
		}

		reftests := 0
		for _, dt := range tests {
			if dt.Type == RefTest {
				reftests++
				if dt.RefPath == "" {
					t.Errorf("reftest %s has no reference path", dt.Path)
				}
			}
		}
		t.Logf("  %s: %d tests (%d reftests)", dir, len(tests), reftests)
		totalTests += len(tests)
	}

	if totalTests == 0 {
		t.Error("no tests discovered in pilot directory")
	}
	t.Logf("pilot discovery: %d total tests", totalTests)
}

// findPilotDir locates the wpt-pilot directory.
func findPilotDir(t *testing.T) string {
	t.Helper()

	candidates := []string{
		"../../testdata/wpt-pilot",
		"testdata/wpt-pilot",
		filepath.Join(os.Getenv("PWD"), "testdata/wpt-pilot"),
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs
		}
	}
	t.Skip("wpt-pilot directory not found")
	return ""
}

// findGoosieBinary locates the goosie binary.
func findGoosieBinary(t *testing.T) string {
	t.Helper()

	if v := os.Getenv("WPT_GOOSIE_BINARY"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
	}

	candidates := []string{
		"../../goosie",
		"./goosie",
		"../../bin/goosie",
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	t.Skip("goosie binary not found; build with: go build -o goosie ./cmd/goosie")
	return ""
}
