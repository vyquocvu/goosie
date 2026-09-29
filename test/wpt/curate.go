package wpt

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Curator selects WPT tests that goosie can meaningfully run. The selection is
// conservative: a test that might require unsupported features is skipped rather
// than run and misreported as a failure. The curation rules are encoded here
// rather than in configuration so that the defaults are always correct for the
// current engine state.

// FeatureSupport describes what goosie can render. This is the single source
// of truth for what the curator allows.
type FeatureSupport struct {
	// HTML parsing and basic rendering.
	HTMLBasic bool
	// CSS color, font-size, margin, padding, border, background.
	CSSBasic bool
	// CSS flexbox.
	CSSFlexbox bool
	// CSS grid.
	CSSGrid bool
	// CSS positioning (absolute, relative, fixed).
	CSSPositioning bool
	// CSS selectors (class, id, descendant, child).
	CSSSelectors bool
	// HTML tables.
	HTMLTables bool
	// HTML forms (visual rendering only, no interaction).
	HTMLForms bool
	// HTML lists (ol, ul, dl).
	HTMLLists bool
	// Canvas API.
	Canvas bool
	// SVG rendering.
	SVG bool
	// WebGL.
	WebGL bool
	// JavaScript DOM manipulation.
	JSDOM bool
	// Web Animations / CSS animations.
	CSSAnimations bool
	// Media queries.
	MediaQueries bool
	// CSS custom properties (variables).
	CSSVariables bool
	// CSS calc().
	CSSCalc bool
	// CSS gradients.
	CSSGradients bool
	// Pseudo-classes (:hover, :focus, etc.).
	CSSPseudoClasses bool
	// Pseudo-elements (::before, ::after).
	CSSPseudoElements bool
}

// GoosieSupport returns the feature set goosie currently supports. This is
// updated as the engine grows; the curator reads it to decide what to include.
func GoosieSupport() FeatureSupport {
	return FeatureSupport{
		HTMLBasic:         true,
		CSSBasic:          true,
		CSSFlexbox:        true,
		CSSGrid:           false, // not yet implemented
		CSSPositioning:    true,
		CSSSelectors:      true,
		HTMLTables:        true,
		HTMLForms:         true,
		HTMLLists:         true,
		Canvas:            false,
		SVG:               false,
		WebGL:             false,
		JSDOM:             false,
		CSSAnimations:     false,
		MediaQueries:      true,
		CSSVariables:      true,
		CSSCalc:           true,
		CSSGradients:      false,
		CSSPseudoClasses:  true,
		CSSPseudoElements: false,
	}
}

// CuratedDirs returns the WPT directories goosie should test, based on its
// current feature support. Each entry maps a WPT directory to a human-readable
// description of what it tests.
func CuratedDirs(support FeatureSupport) []CuratedDir {
	var dirs []CuratedDir

	if support.HTMLBasic {
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/text-level-semantics",
			Description: "HTML text-level semantics (a, em, strong, etc.)",
			Type:        RefTest,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/document-metadata/the-title-element",
			Description: "HTML title element",
			Type:        RefTest,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/dom/elements/global-attributes",
			Description: "HTML global attributes (class, id, dir)",
			Type:        RefTest,
		})
	}

	if support.CSSBasic {
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-color",
			Description: "CSS color values",
			Type:        RefTest,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-fonts",
			Description: "CSS font properties",
			Type:        RefTest,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-backgrounds",
			Description: "CSS background properties",
			Type:        RefTest,
		})
	}

	if support.CSSFlexbox {
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-flexbox",
			Description: "CSS flexbox layout",
			Type:        RefTest,
		})
	}

	if support.CSSGrid {
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-grid",
			Description: "CSS grid layout",
			Type:        RefTest,
		})
	}

	if support.CSSPositioning {
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-position",
			Description: "CSS positioned layout",
			Type:        RefTest,
		})
	}

	if support.CSSSelectors {
		dirs = append(dirs, CuratedDir{
			Path:        "css/selectors",
			Description: "CSS selector matching",
			Type:        RefTest,
		})
	}

	if support.HTMLTables {
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-table",
			Description: "CSS table rendering",
			Type:        RefTest,
		})
	}

	if support.HTMLLists {
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-lists",
			Description: "CSS list styling",
			Type:        RefTest,
		})
	}

	return dirs
}

// CuratedDir is a WPT directory with metadata about why it was selected.
type CuratedDir struct {
	// Path is the WPT-relative directory path.
	Path string
	// Description explains what this directory tests.
	Description string
	// Type is the kind of test expected in this directory.
	Type TestType
}

// DiscoverTests walks a WPT directory and returns the test files it contains,
// classified by type. Only reftests and testharness tests are returned; manual
// tests are always excluded.
func DiscoverTests(wptRoot, dir string, excludes []string) ([]DiscoveredTest, error) {
	absDir := filepath.Join(wptRoot, dir)
	info, err := os.Stat(absDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "discover", Path: absDir, Err: os.ErrInvalid}
	}

	var tests []DiscoveredTest
	err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".html" && ext != ".htm" && ext != ".xhtml" && ext != ".svg" {
			return nil
		}

		relPath, err := filepath.Rel(wptRoot, path)
		if err != nil {
			return nil
		}

		// Check exclusions.
		for _, pattern := range excludes {
			matched, err := filepath.Match(pattern, filepath.Base(path))
			if err == nil && matched {
				return nil
			}
			// Also match against the relative path.
			matched, err = filepath.Match(pattern, relPath)
			if err == nil && matched {
				return nil
			}
		}

		testType := classifyTest(path)
		if testType == ManualTest {
			return nil
		}

		dt := DiscoveredTest{
			Path:    relPath,
			AbsPath: path,
			Type:    testType,
			FullDir: dir,
		}

		// For reftests, find the reference file.
		if testType == RefTest {
			refPath, relation := findReference(path, wptRoot)
			dt.RefPath = refPath
			dt.RefRelation = relation
		}

		tests = append(tests, dt)
		return nil
	})
	return tests, err
}

// DiscoveredTest is a WPT test file found during directory traversal.
type DiscoveredTest struct {
	// Path is the WPT-relative path.
	Path string
	// AbsPath is the absolute filesystem path.
	AbsPath string
	// Type is the test classification.
	Type TestType
	// FullDir is the curated directory this test belongs to.
	FullDir string
	// RefPath is the absolute path to the reference file (reftests only).
	RefPath string
	// RefRelation is "match" or "mismatch" (reftests only).
	RefRelation string
}

// classifyTest determines the test type by reading the HTML file's content.
// The classification is heuristic based on markers in the file.
func classifyTest(path string) TestType {
	f, err := os.Open(path)
	if err != nil {
		return ManualTest
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	linesRead := 0
	for scanner.Scan() {
		line := scanner.Text()
		linesRead++

		// Only scan the first 50 lines for classification markers.
		if linesRead > 50 {
			break
		}

		lower := strings.ToLower(line)

		// Reftest markers: <link rel="match" ...> or <link rel="mismatch" ...>
		if strings.Contains(lower, `rel="match"`) || strings.Contains(lower, `rel='match'`) {
			return RefTest
		}
		if strings.Contains(lower, `rel="mismatch"`) || strings.Contains(lower, `rel='mismatch'`) {
			return RefTest
		}

		// Testharness marker: inclusion of testharness.js
		if strings.Contains(lower, "testharness.js") {
			return TestHarness
		}

		// Manual test marker
		if strings.Contains(lower, "manual") && strings.Contains(lower, `rel="help"`) {
			return ManualTest
		}
	}

	// Default: if we can't classify it, skip it rather than misreport.
	return ManualTest
}

// refLinkRe matches <link rel="match" href="..."> and <link rel="mismatch" href="...">.
var refLinkRe = regexp.MustCompile(`<link[^>]+rel=["'](match|mismatch)["'][^>]+href=["']([^"']+)["']`)
var refHrefFirstRe = regexp.MustCompile(`<link[^>]+href=["']([^"']+)["'][^>]+rel=["'](match|mismatch)["']`)

// findReference locates the reference file for a reftest. It reads the test
// file's <link rel="match" href="..."> or <link rel="mismatch" href="..."> to
// find the reference path.
func findReference(testPath, wptRoot string) (string, string) {
	f, err := os.Open(testPath)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()

		// Try rel first, then href first.
		if m := refLinkRe.FindStringSubmatch(line); m != nil {
			relation := m[1]
			href := m[2]
			absRef := resolveRefPath(testPath, href, wptRoot)
			return absRef, relation
		}
		if m := refHrefFirstRe.FindStringSubmatch(line); m != nil {
			href := m[1]
			relation := m[2]
			absRef := resolveRefPath(testPath, href, wptRoot)
			return absRef, relation
		}
	}
	return "", ""
}

// resolveRefPath turns a reference href into an absolute filesystem path.
// The href may be relative to the test file or absolute from the WPT root.
func resolveRefPath(testPath, href, wptRoot string) string {
	if strings.HasPrefix(href, "/") {
		return filepath.Join(wptRoot, href)
	}
	return filepath.Join(filepath.Dir(testPath), href)
}

// ShouldSkip returns true if a discovered test requires features goosie
// does not support. The reason string explains why.
func ShouldSkip(dt DiscoveredTest, support FeatureSupport) (bool, string) {
	// testharness.js tests need JS DOM, which goosie lacks.
	if dt.Type == TestHarness {
		return true, "testharness.js requires JS DOM"
	}

	// Check for unsupported features by directory name.
	dir := strings.ToLower(dt.FullDir)
	path := strings.ToLower(dt.Path)

	if !support.Canvas && (strings.Contains(dir, "canvas") || strings.Contains(dir, "2dcontext")) {
		return true, "Canvas not supported"
	}
	if !support.SVG && strings.Contains(dir, "svg") {
		return true, "SVG not supported"
	}
	if !support.WebGL && (strings.Contains(dir, "webgl") || strings.Contains(dir, "webgl2")) {
		return true, "WebGL not supported"
	}
	if !support.CSSAnimations && (strings.Contains(dir, "css-animations") || strings.Contains(dir, "css-transitions")) {
		return true, "CSS animations not supported"
	}
	if !support.CSSGradients && strings.Contains(dir, "css-images") && strings.Contains(path, "gradient") {
		return true, "CSS gradients not supported"
	}
	if !support.CSSPseudoElements && strings.Contains(path, "pseudo-element") {
		return true, "CSS pseudo-elements not supported"
	}
	if !support.CSSGrid && strings.Contains(dir, "css-grid") {
		return true, "CSS grid not supported"
	}

	return false, ""
}
