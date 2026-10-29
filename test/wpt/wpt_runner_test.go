package wpt

import (
	"net/url"
	"path/filepath"
	"testing"
)

func TestWPTLocalTransportMapsOnlyConfiguredLoopbackAliases(t *testing.T) {
	base := "http://wpt.localhost:12345"
	for _, tc := range []struct {
		input string
		want  string
		ok    bool
	}{
		{"https://wpt-other.localhost:12345/a/b?q=1", "http://wpt.localhost:12345/a/b?q=1", true},
		{"http://www1.wpt.localhost:12345/a", "http://wpt.localhost:12345/a", true},
		{"https://example.test:12345/a", "", false},
		{"https://wpt-other.localhost:12346/a", "", false},
		{"https://user@wpt-other.localhost:12345/a", "", false},
	} {
		candidate, err := url.Parse(tc.input)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", tc.input, err)
		}
		transport, ok := wptLocalTransportURL(candidate, base)
		if ok != tc.ok {
			t.Errorf("wptLocalTransportURL(%q) ok=%t, want %t", tc.input, ok, tc.ok)
		}
		if ok && transport.String() != tc.want {
			t.Errorf("wptLocalTransportURL(%q) = %q, want %q", tc.input, transport, tc.want)
		}
	}
}

func TestConfiguredWPTDirectoriesOverrideCuratedCatalog(t *testing.T) {
	dirs, err := selectedWPTDirectories([]string{"html", "dom", "html"}, FeatureSupport{})
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 || dirs[0].Path != "html" || dirs[1].Path != "dom" {
		t.Fatalf("selected directories = %#v, want [html dom]", dirs)
	}

	defaultDirs, err := selectedWPTDirectories(nil, FeatureSupport{HTMLBasic: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultDirs) == 0 || defaultDirs[0].Path == "html" || defaultDirs[0].Path == "dom" {
		t.Fatalf("default selection did not use curated directories: %#v", defaultDirs)
	}
}

func TestConfiguredWPTDirectoriesRejectPathsOutsideCheckout(t *testing.T) {
	for _, directory := range []string{"../outside", "html/../../outside", "/tmp/wpt"} {
		if _, err := selectedWPTDirectories([]string{directory}, FeatureSupport{}); err == nil {
			t.Errorf("selectedWPTDirectories(%q) succeeded", directory)
		}
	}
}

func TestExplicitWPTDirectoryDiscoveryReportsMissingRoots(t *testing.T) {
	root := t.TempDir()
	dirs := []CuratedDir{{Path: "html", Description: "html"}}
	if _, err := discoverWPTTests(root, dirs, nil, nil, true); err == nil {
		t.Fatal("explicit directory discovery silently accepted a missing WPT root")
	}
	if _, err := discoverWPTTests(root, dirs, nil, nil, false); err != nil {
		t.Fatalf("default curated discovery should tolerate unavailable optional roots: %v", err)
	}
}

func TestTask55FullConfigSelectsHTMLDOMAndDependentSuites(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "testdata", "wpt-task55-full-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"html", "dom", "domparsing", "input-events", "webidl", "css", "webgl", "media", "fetch", "webmessaging", "service-workers", "IndexedDB"} {
		found := false
		for _, directory := range cfg.Directories {
			if directory == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("full TASK-055 config omits %q", required)
		}
	}
	if len(cfg.Exclude) != 0 {
		t.Fatalf("full TASK-055 config excludes tests: %v", cfg.Exclude)
	}
	if cfg.MinPassRate != 100 {
		t.Fatalf("full TASK-055 pass-rate floor = %v, want 100", cfg.MinPassRate)
	}
}

func TestReftestMismatchPassesForAnyToleratedPixelDifference(t *testing.T) {
	if reftestRelationPasses(100, "mismatch", 95) {
		t.Fatal("identical images passed a mismatch reftest")
	}
	if !reftestRelationPasses(99.999, "mismatch", 95) {
		t.Fatal("a real pixel difference was rejected by the match threshold")
	}
	if !reftestRelationPasses(95, "match", 95) {
		t.Fatal("a match at the configured threshold failed")
	}
	if reftestRelationPasses(94.999, "match", 95) {
		t.Fatal("a match below the configured threshold passed")
	}
}
