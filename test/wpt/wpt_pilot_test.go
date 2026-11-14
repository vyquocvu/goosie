package wpt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWPTPilot(t *testing.T) {
	pilotDir := findPilotDir(t)

	binary := findGoosieBinary(t)

	cfg := DefaultConfig()
	cfg.WPTDir = pilotDir
	cfg.GoosieBinary = binary
	cfg.OutputDir = t.TempDir()
	cfg.PassThreshold = 90.0
	cfg.Directories = []string{
		"css/css-color",
		"css/css-backgrounds",
		"css/css-flexbox",
		"css/css-position",
		"css/selectors",
		"html/semantics",
	}

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

	srv := NewServer(pilotDir)
	if err := srv.Start(); err != nil {
		t.Fatalf("pilot: start server: %v", err)
	}
	defer srv.Stop()

	runner := NewRunner(cfg)
	runner.server = srv

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

func TestWPTPilotDiscovery(t *testing.T) {
	pilotDir := findPilotDir(t)

	dirs := []string{
		"css/css-color",
		"css/css-backgrounds",
		"css/css-flexbox",
		"css/css-position",
		"css/selectors",
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
