package wpt

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWPTSuite runs the configured WPT directories against goosie. With the
// default config this is the curated set; a full conformance config can select
// broader roots. It discovers tests, renders them, compares references, and
// reports results.
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
//   - WPT_INCLUDE: comma-separated WPT-relative paths or file-name patterns
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

	// The suite fails below its configured floor (ratchet: set to the measured
	// rate, raised as rendering support grows, never lowered to green a red
	// run). Individual failures below that are still listed per-test above.
	if result.Total == 0 {
		t.Skip("no WPT tests discovered")
	}
	if err := CheckSuiteThreshold(*result, cfg.MinPassRate); err != nil {
		t.Error(err)
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
	parserTestsCurated := false
	domParsingCurated := false
	customElementsCurated := false
	blobAPICurated := false
	fileAPICurated := false
	messageChannelsCurated := false
	inputElementCurated := false
	fieldsetTestsCurated := false
	dialogTestsCurated := false
	detailsTestsCurated := false
	contentEditableCurated := false
	dragAndDropCurated := false
	dragEventsCurated := false
	dragDataStoreCurated := false
	domRangesCurated := false
	domNodesCurated := false
	domEventsCurated := false
	anchorLinksCurated := false
	for _, d := range dirs {
		if d.Path == "" {
			t.Error("curated dir has empty path")
		}
		if d.Description == "" {
			t.Errorf("curated dir %q has empty description", d.Path)
		}
		if d.Path == "html/syntax/parsing" {
			parserTestsCurated = true
		}
		if d.Path == "domparsing" && d.Type == TestHarness {
			domParsingCurated = true
		}
		if d.Path == "html/semantics/interactive-elements/the-dialog-element" && d.Type == TestHarness {
			dialogTestsCurated = true
		}
		if d.Path == "html/semantics/interactive-elements/the-details-element" && d.Type == TestHarness {
			detailsTestsCurated = true
		}
		if d.Path == "html/semantics/forms/the-fieldset-element" && d.Type == TestHarness {
			fieldsetTestsCurated = true
		}
		if d.Path == "html/semantics/forms/the-input-element" && d.Type == TestHarness {
			inputElementCurated = true
		}
		if d.Path == "html/editing/editing-0/contenteditable" && d.Type == TestHarness {
			contentEditableCurated = true
		}
		if d.Path == "html/editing/dnd/the-draggable-attribute" && d.Type == TestHarness {
			dragAndDropCurated = true
		}
		if d.Path == "html/editing/dnd/events" && d.Type == TestHarness {
			dragEventsCurated = true
		}
		if d.Path == "html/editing/dnd/datastore" && d.Type == TestHarness {
			dragDataStoreCurated = true
		}
		if d.Path == "dom/ranges" && d.Type == TestHarness {
			domRangesCurated = true
		}
		if d.Path == "dom/nodes" && d.Type == TestHarness {
			domNodesCurated = true
		}
		if d.Path == "dom/events" && d.Type == TestHarness {
			domEventsCurated = true
		}
		if d.Path == "html/semantics/links/links-created-by-a-and-area-elements" && d.Type == TestHarness {
			anchorLinksCurated = true
		}
		if d.Path == "custom-elements" && d.Type == TestHarness {
			customElementsCurated = true
		}
		if d.Path == "FileAPI/blob" && d.Type == TestHarness {
			blobAPICurated = true
		}
		if d.Path == "FileAPI/file" && d.Type == TestHarness {
			fileAPICurated = true
		}
		if d.Path == "webmessaging/message-channels" && d.Type == TestHarness {
			messageChannelsCurated = true
		}
	}
	if !parserTestsCurated {
		t.Error("HTML syntax parsing WPTs should be available to the conformance runner")
	}
	if !domParsingCurated {
		t.Error("DOM Parsing WPTs should be discoverable for DOMParser/XMLSerializer conformance work")
	}
	if !dialogTestsCurated {
		t.Error("HTML dialog WPTs should be discoverable while dialog behavior remains partial")
	}
	if !detailsTestsCurated {
		t.Error("HTML details WPTs should be discoverable while details behavior remains partial")
	}
	if !fieldsetTestsCurated {
		t.Error("HTML fieldset WPTs should be discoverable while disabledness behavior remains partial")
	}
	if !inputElementCurated {
		t.Error("HTML input element WPTs should be discoverable while value and checkedness behavior remains partial")
	}
	if !contentEditableCurated {
		t.Error("contenteditable WPTs should be discoverable while editing behavior remains partial")
	}
	if !dragAndDropCurated {
		t.Error("draggable attribute WPTs should be discoverable while drag-and-drop behavior remains partial")
	}
	if !dragEventsCurated {
		t.Error("drag event WPTs should be discoverable while drag-and-drop behavior remains partial")
	}
	if !dragDataStoreCurated {
		t.Error("drag data store WPTs should be discoverable while drag-and-drop behavior remains partial")
	}
	if !domRangesCurated {
		t.Error("DOM Range WPTs should be discoverable while Range behavior remains partial")
	}
	if !domNodesCurated {
		t.Error("DOM Node WPTs should be discoverable while Node behavior remains partial")
	}
	if !domEventsCurated {
		t.Error("DOM EventTarget WPTs should be discoverable while event behavior remains partial")
	}
	if !anchorLinksCurated {
		t.Error("HTML anchor and area WPTs should be discoverable while link behavior remains partial")
	}
	if !customElementsCurated {
		t.Error("custom element WPTs should be discoverable while implementation gaps remain")
	}
	if !blobAPICurated || !fileAPICurated {
		t.Errorf("File API WPT directories should be discoverable; Blob=%v File=%v", blobAPICurated, fileAPICurated)
	}
	if !messageChannelsCurated {
		t.Error("MessageChannel/MessagePort WPTs should be discoverable while implementation gaps remain")
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
	if err := os.WriteFile(filepath.Join(tmpDir, "test.html.headers"), []byte("Content-Language: ko\nContent-Language: ja\n"), 0o644); err != nil {
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
	response, err := http.Get(url + "/test.html")
	if err != nil {
		t.Fatalf("GET test file: %v", err)
	}
	defer response.Body.Close()
	values := response.Header.Values("Content-Language")
	if len(values) != 2 || values[0] != "ko" || values[1] != "ja" {
		t.Errorf("Content-Language headers = %v, want [ko ja]", values)
	}
	if got, want := response.Header.Get("Content-Type"), "text/html"; got != want {
		t.Errorf("default HTML Content-Type = %q, want %q without an invented charset", got, want)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "encoded.html"), []byte(testHTML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "encoded.html.headers"), []byte("Content-Type: text/html; charset=windows-1252\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	encodedResponse, err := http.Get(url + "/encoded.html")
	if err != nil {
		t.Fatalf("GET encoded test file: %v", err)
	}
	defer encodedResponse.Body.Close()
	if got, want := encodedResponse.Header.Get("Content-Type"), "text/html; charset=windows-1252"; got != want {
		t.Errorf(".headers Content-Type = %q, want %q", got, want)
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
		"include": ["css/selectors/focus-within-001.html", "focus-within-002.html"],
		"test_variants": {"editing/run/delete.html": ["?1-10", "?11-20"]},
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
	if len(cfg.Include) != 2 || cfg.Include[0] != "css/selectors/focus-within-001.html" || cfg.Include[1] != "focus-within-002.html" {
		t.Errorf("Include = %v, want the two configured WPT paths", cfg.Include)
	}
	if got := cfg.TestVariants["editing/run/delete.html"]; len(got) != 2 || got[0] != "?1-10" || got[1] != "?11-20" {
		t.Errorf("TestVariants = %v, want the two editing test query variants", cfg.TestVariants)
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

func TestWPTLoadTestConfigMergesConfiguredVariants(t *testing.T) {
	t.Setenv("WPT_CONFIG", "")
	cfg := loadTestConfig(t)
	want := []string{"?Backspace,ul", "?Backspace,ol", "?Delete,ul", "?Delete,ol"}
	got := cfg.TestVariants["input-events/input-events-get-target-ranges-deleting-in-list-items.tentative.html"]
	if len(got) != len(want) {
		t.Fatalf("configured input-events variants = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("configured input-events variants = %v, want %v", got, want)
		}
	}
}

func TestWPTTestVariantURL(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		want  string
		bad   bool
	}{
		{name: "no variant", want: "http://wpt.invalid/editing/run/delete.html"},
		{name: "subset query", query: "?1-10", want: "http://wpt.invalid/editing/run/delete.html?1-10"},
		{name: "ordinary query", query: "?locale=en-US", want: "http://wpt.invalid/editing/run/delete.html?locale=en-US"},
		{name: "reject empty variant", query: "?", bad: true},
		{name: "reject fragment", query: "?1-10#x", bad: true},
		{name: "reject malformed escape", query: "?x=%zz", bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testVariantURL("http://wpt.invalid/editing/run/delete.html", tc.query)
			if tc.bad {
				if err == nil {
					t.Fatalf("testVariantURL accepted invalid query %q", tc.query)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("testVariantURL(%q) = %q, %v; want %q", tc.query, got, err, tc.want)
			}
		})
	}
}

func TestWPTIncludeFilter(t *testing.T) {
	for _, tc := range []struct {
		path     string
		patterns []string
		want     bool
	}{
		{"css/selectors/focus-within-001.html", []string{"css/selectors/focus-within-001.html"}, true},
		{"css/selectors/focus-within-001.html", []string{"focus-within-00?.html"}, true},
		{"css/selectors/focus-within-001.html", []string{"focus-within-002.html"}, false},
		{"css/selectors/focus-within-001.html", nil, true},
	} {
		if got := includedTest(tc.path, tc.patterns); got != tc.want {
			t.Errorf("includedTest(%q, %v) = %v, want %v", tc.path, tc.patterns, got, tc.want)
		}
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
			wantSkip: false,
		},
		{
			dt:       DiscoveredTest{Path: "css/test.html", Type: TestHarness, FullDir: "css"},
			wantSkip: false,
		},
		{
			dt:       DiscoveredTest{Path: "html/forms/manual.html", Type: ManualTest, FullDir: "html/forms"},
			wantSkip: true,
		},
		{
			dt:       DiscoveredTest{Path: "html/forms/unknown.html", Type: UnclassifiedTest, FullDir: "html/forms"},
			wantSkip: true,
		},
		{
			dt:       DiscoveredTest{Path: "html/forms/helper-child.html", Type: SupportFile, FullDir: "html/forms"},
			wantSkip: true,
		},
		{
			dt:       DiscoveredTest{Path: "html/forms/no-crash.html", Type: SmokeTest, FullDir: "html/forms"},
			wantSkip: false,
		},
	}

	for _, tt := range tests {
		skip, reason := ShouldSkip(tt.dt, support)
		if skip != tt.wantSkip {
			t.Errorf("ShouldSkip(%s) = %v, %v; want %v", tt.dt.Path, skip, reason, tt.wantSkip)
		}
	}
}

func TestDiscoverTestsKeepsManualAndUnclassifiedPagesVisible(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "html", "forms")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]string{
		"interactive.html":        `<meta name="flags" content="interact"><script src="testharness.js"></script>`,
		"file-manual.html":        `<script src="testharness.js"></script>`,
		"automated.html":          `<script src="testharness.js"></script><form action=helper-child.html></form>`,
		"unmarked.html":           `<p>content without a WPT marker</p>`,
		"range-reftest.html":      `<link rel=match href=range-ref.html>`,
		"href-first-reftest.html": `<link href='other-ref.html' rel='mismatch'>`,
		"no-crash.html":           `<p>must not crash</p>`,
		"helper-child.html":       `<script>parent.postMessage(location.href, '*')</script>`,
	}
	for name, source := range fixtures {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "resources"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, source := range map[string]string{
		"reference-ref.html":                      `<p>reference image</p>`,
		"range-ref.html":                          `<p>reference image</p>`,
		"other-ref.html":                          `<p>reference image</p>`,
		filepath.Join("resources", "helper.html"): `<script src="testharness.js"></script>`,
	} {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tests, err := DiscoverTests(root, filepath.Join("html", "forms"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != len(fixtures) {
		t.Fatalf("discovered %d candidates, want %d", len(tests), len(fixtures))
	}
	types := make(map[string]TestType, len(tests))
	for _, test := range tests {
		types[filepath.Base(test.Path)] = test.Type
	}
	want := map[string]TestType{
		"interactive.html":  ManualTest,
		"file-manual.html":  ManualTest,
		"automated.html":    TestHarness,
		"unmarked.html":     UnclassifiedTest,
		"no-crash.html":     SmokeTest,
		"helper-child.html": SupportFile,
	}
	for name, expected := range want {
		if types[name] != expected {
			t.Errorf("%s classified as %q, want %q", name, types[name], expected)
		}
	}
	for _, test := range tests {
		if test.Path == filepath.Join("html", "forms", "range-reftest.html") {
			if test.Type != RefTest || test.RefRelation != "match" || test.RefPath != filepath.Join(dir, "range-ref.html") {
				t.Errorf("unquoted reftest discovery = type %q, relation %q, path %q", test.Type, test.RefRelation, test.RefPath)
			}
		}
		if test.Path == filepath.Join("html", "forms", "href-first-reftest.html") {
			if test.Type != RefTest || test.RefRelation != "mismatch" || test.RefPath != filepath.Join(dir, "other-ref.html") {
				t.Errorf("href-first reftest discovery = type %q, relation %q, path %q", test.Type, test.RefRelation, test.RefPath)
			}
		}
		if test.Type != ManualTest && test.Type != SupportFile && test.Type != UnclassifiedTest {
			continue
		}
		skipped, reason := ShouldSkip(test, GoosieSupport())
		if !skipped || reason == "" {
			t.Errorf("%s must produce an explicit skip reason, got skipped=%v reason=%q", test.Path, skipped, reason)
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
	if v := os.Getenv("WPT_INCLUDE"); v != "" {
		cfg.Include = splitAndTrim(v)
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
					if len(fileCfg.TestVariants) > 0 && len(cfg.TestVariants) == 0 {
						cfg.TestVariants = fileCfg.TestVariants
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

// TestReadFuzzyMeta pins fuzzy-tolerance parsing: the most lenient
// allowance wins, unparseable metas are ignored, and files without the
// meta report ok=false so strict scoring still applies.
func TestReadFuzzyMeta(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := dir + "/" + name
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	md, ta, ok := readFuzzyMeta(write("a.html", `<meta name="fuzzy" content="maxDifference=0-96;totalPixels=0-400">`))
	if !ok || md != 96 || ta != 400 {
		t.Errorf("fuzzy = (%d,%d,%v), want (96,400,true)", md, ta, ok)
	}
	md, ta, ok = readFuzzyMeta(write("b.html", `<meta name=fuzzy content='maxDifference=0-10; totalPixels=0-5'>`))
	if !ok || md != 10 || ta != 5 {
		t.Errorf("single-quote fuzzy = (%d,%d,%v), want (10,5,true)", md, ta, ok)
	}
	_, _, ok = readFuzzyMeta(write("c.html", `<meta name="viewport" content="width=1">`))
	if ok {
		t.Error("non-fuzzy meta reported ok=true")
	}
}

// TestFuzzyPasses pins the tolerance recount: identical images pass any
// allowance, fully different images fail a zero allowance but pass a
// lenient one.
func TestFuzzyPasses(t *testing.T) {
	tmpDir := t.TempDir()
	imgA := filepath.Join(tmpDir, "a.png")
	imgB := filepath.Join(tmpDir, "b.png")
	writeTestPNG(t, imgA, 2, 2, [4]byte{255, 0, 0, 255})
	writeTestPNG(t, imgB, 2, 2, [4]byte{255, 0, 0, 255})
	if !fuzzyPasses(imgA, imgB, 0, 0) {
		t.Error("identical images fail fuzzy 0/0")
	}
	imgC := filepath.Join(tmpDir, "c.png")
	writeTestPNG(t, imgC, 2, 2, [4]byte{0, 255, 0, 255})
	if fuzzyPasses(imgA, imgC, 0, 0) {
		t.Error("different images pass fuzzy 0/0")
	}
	if !fuzzyPasses(imgA, imgC, 255, 4) {
		t.Error("different images fail fuzzy 255/4")
	}
}

// TestWPTCheckoutReusable pins the stale-checkout contract: a matching
// marker reuses only when content sits behind it. A marker alone (truncated
// download, external cleanup) must re-download, never run an empty suite.
func TestWPTCheckoutReusable(t *testing.T) {
	const pin = "abc123"
	if checkoutReusable(t.TempDir(), pin) {
		t.Error("missing checkout reported reusable")
	}
	only := t.TempDir()
	if err := os.WriteFile(filepath.Join(only, ".goosie-wpt-commit"), []byte(pin+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if checkoutReusable(only, pin) {
		t.Error("marker-only checkout reported reusable")
	}
	stale := t.TempDir()
	if err := os.WriteFile(filepath.Join(stale, ".goosie-wpt-commit"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "css"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if checkoutReusable(stale, pin) {
		t.Error("wrong-commit checkout reported reusable")
	}
	full := t.TempDir()
	if err := os.WriteFile(filepath.Join(full, ".goosie-wpt-commit"), []byte(pin+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "css"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !checkoutReusable(full, pin) {
		t.Error("pinned checkout with content not reused")
	}
}

// TestWPTMinPassRateFloor pins the suite-level ratchet: a floor of zero
// disables (ad-hoc runs still report), an empty run never meets a positive
// floor, and the boundary is inclusive so a floor set to today's measured
// rate stays green until support regresses.
func TestWPTMinPassRateFloor(t *testing.T) {
	full := SuiteResult{Total: 100, Passed: 80, Failed: 20, PassRate: 80.0}
	if !full.MeetsMinPassRate(0) {
		t.Error("zero floor must disable the check")
	}
	if !full.MeetsMinPassRate(80.0) {
		t.Error("80% run must meet an 80.0 floor (inclusive boundary)")
	}
	if full.MeetsMinPassRate(80.1) {
		t.Error("80% run must not meet an 80.1 floor")
	}
	empty := SuiteResult{}
	if empty.MeetsMinPassRate(1.0) {
		t.Error("empty run must not meet a positive floor (harness failure, not a pass)")
	}
	allSkipped := SuiteResult{Total: 10, Skipped: 10}
	if allSkipped.MeetsMinPassRate(1.0) {
		t.Error("all-skipped run must not meet a positive floor")
	}
}

// TestWPTSuiteThresholdCheck pins the enforcement the suite itself calls: a
// disabled floor never errors, a run at the floor passes, and anything below
// errors with the counts attached.
func TestWPTSuiteThresholdCheck(t *testing.T) {
	full := SuiteResult{Total: 100, Passed: 80, Failed: 20, PassRate: 80.0}
	if err := CheckSuiteThreshold(full, 0); err != nil {
		t.Errorf("disabled floor errored: %v", err)
	}
	if err := CheckSuiteThreshold(full, 80.0); err != nil {
		t.Errorf("run at floor errored: %v", err)
	}
	if err := CheckSuiteThreshold(full, 85.0); err == nil {
		t.Error("80% run against an 85.0 floor did not error")
	} else if got := err.Error(); !strings.Contains(got, "80/100") {
		t.Errorf("threshold error = %q, want the counts attached", got)
	}
}
