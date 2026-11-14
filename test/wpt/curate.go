package wpt

import (
	"bufio"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type FeatureSupport struct {
	HTMLBasic           bool
	CSSBasic            bool
	CSSFlexbox          bool
	CSSGrid             bool
	CSSPositioning      bool
	CSSSelectors        bool
	HTMLTables          bool
	HTMLForms           bool
	HTMLDialogs         bool
	HTMLPopovers        bool
	HTMLContentEditable bool
	HTMLDragAndDrop     bool
	DOMRanges           bool
	DOMNodes            bool
	HTMLLists           bool
	Canvas              bool
	SVG                 bool
	WebGL               bool
	JSDOM               bool
	CSSAnimations       bool
	MediaQueries        bool
	CSSVariables        bool
	CSSCalc             bool
	CSSGradients        bool
	CSSPseudoClasses    bool
	CSSPseudoElements   bool
}

func GoosieSupport() FeatureSupport {
	return FeatureSupport{
		HTMLBasic:           true,
		CSSBasic:            true,
		CSSFlexbox:          true,
		CSSGrid:             true,
		CSSPositioning:      true,
		CSSSelectors:        true,
		HTMLTables:          true,
		HTMLForms:           true,
		HTMLDialogs:         true,
		HTMLPopovers:        true,
		HTMLContentEditable: true,
		HTMLDragAndDrop:     true,
		DOMRanges:           true,
		DOMNodes:            true,
		HTMLLists:           true,
		Canvas:              false,
		SVG:                 false,
		WebGL:               false,
		JSDOM:               false,
		CSSAnimations:       true,
		MediaQueries:        true,
		CSSVariables:        true,
		CSSCalc:             true,
		CSSGradients:        true,
		CSSPseudoClasses:    true,
		CSSPseudoElements:   true,
	}
}

func CuratedDirs(support FeatureSupport) []CuratedDir {
	var dirs []CuratedDir

	if support.HTMLBasic {
		dirs = append(dirs, CuratedDir{
			Path:        "dom/events",
			Description: "DOM EventTarget propagation, listener options, and dispatch behavior",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/text-level-semantics",
			Description: "HTML text-level semantics (a, em, strong, etc.)",
			Type:        RefTest,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/links/links-created-by-a-and-area-elements",
			Description: "HTML anchor and area URL reflection and hyperlink attributes",
			Type:        TestHarness,
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
		dirs = append(dirs, CuratedDir{
			Path:        "html/syntax/parsing",
			Description: "HTML tokenizer and tree-construction parsing behavior",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "domparsing",
			Description: "DOMParser and XMLSerializer parsing and serialization behavior",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/webappapis/scripting/events",
			Description: "HTML event handler IDL attributes and processing",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "custom-elements",
			Description: "Custom element registry, upgrade, and lifecycle behavior",
			Type:        TestHarness,
		})
	}

	if support.HTMLForms {
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/forms/the-input-element",
			Description: "HTML input state, value sanitization, selection, and radio behavior",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/forms/the-select-element",
			Description: "HTML select state, option selection, and picker behavior",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/forms/the-fieldset-element",
			Description: "HTML fieldset disabledness, legend exceptions, and constraint validation",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/forms/the-form-element",
			Description: "HTMLFormElement collections, reflected attributes, and form ownership",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/forms/form-submission-0",
			Description: "HTML form submission algorithms and constructed form data sets",
			Type:        TestHarness,
		})
	}

	if support.HTMLDialogs {
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/interactive-elements/the-dialog-element",
			Description: "HTML dialog element state, focus, close, and modal behavior",
			Type:        TestHarness,
		})
	}

	if support.HTMLBasic {
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/interactive-elements/the-details-element",
			Description: "HTML details open state, summary activation, toggle events, and exclusive groups",
			Type:        TestHarness,
		})
	}

	if support.HTMLPopovers {
		dirs = append(dirs, CuratedDir{
			Path:        "html/semantics/popovers",
			Description: "HTML popover state, invokers, dismissal, and event behavior",
			Type:        TestHarness,
		})
	}

	if support.HTMLContentEditable {
		dirs = append(dirs, CuratedDir{
			Path:        "editing/run",
			Description: "HTML editing command conformance tests",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/editing/editing-0/contenteditable",
			Description: "contenteditable enumerated attribute and DOM reflection",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/editing/editing-0/making-entire-documents-editable-the-designmode-idl-attribute",
			Description: "designMode state and contenteditable interaction",
			Type:        TestHarness,
		})
	}

	if support.HTMLDragAndDrop {
		dirs = append(dirs, CuratedDir{
			Path:        "html/editing/dnd/the-draggable-attribute",
			Description: "HTML draggable attribute reflection and enumerated values",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/editing/dnd/events",
			Description: "HTML drag-and-drop event ordering and pointer suppression",
			Type:        TestHarness,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "html/editing/dnd/datastore",
			Description: "HTML drag data store and DataTransferItemList APIs",
			Type:        TestHarness,
		})
	}

	if support.DOMRanges {
		dirs = append(dirs, CuratedDir{
			Path:        "dom/ranges",
			Description: "DOM Range boundary, mutation, and selection behavior",
			Type:        TestHarness,
		})
	}

	if support.DOMNodes {
		dirs = append(dirs, CuratedDir{
			Path:        "dom/nodes",
			Description: "DOM Node tree operations, document construction, and node position behavior",
			Type:        TestHarness,
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

	if support.CSSAnimations {
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-animations",
			Description: "CSS @keyframes animations",
			Type:        RefTest,
		})
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-transitions",
			Description: "CSS transitions",
			Type:        RefTest,
		})
	}

	if support.CSSGradients {
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-images",
			Description: "CSS images and gradients",
			Type:        RefTest,
		})
	}

	if support.CSSPseudoElements {
		dirs = append(dirs, CuratedDir{
			Path:        "css/css-pseudo",
			Description: "CSS pseudo-elements",
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
			Path:        "css/css-tables",
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

	dirs = append(dirs,
		CuratedDir{Path: "FileAPI/blob", Description: "Blob construction, slicing, and byte access", Type: TestHarness},
		CuratedDir{Path: "FileAPI/file", Description: "File construction and metadata", Type: TestHarness},
		CuratedDir{Path: "webmessaging/message-channels", Description: "MessageChannel and MessagePort delivery, startup, closure, and transfer", Type: TestHarness},
	)

	return dirs
}

type CuratedDir struct {
	Path        string
	Description string
	Type        TestType
}

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
			return nil
		}
		if info.IsDir() {
			if path != absDir && strings.EqualFold(info.Name(), "resources") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		anyTest := strings.HasSuffix(strings.ToLower(path), ".any.js")
		if ext != ".html" && ext != ".htm" && ext != ".xhtml" && ext != ".svg" && !anyTest {
			return nil
		}

		relPath, err := filepath.Rel(wptRoot, path)
		if err != nil {
			return nil
		}
		if isReferenceSupportFile(filepath.Base(path)) {
			return nil
		}

		for _, pattern := range excludes {
			matched, err := filepath.Match(pattern, filepath.Base(path))
			if err == nil && matched {
				return nil
			}
			matched, err = filepath.Match(pattern, relPath)
			if err == nil && matched {
				return nil
			}
		}

		testType := classifyTest(path)
		if testType == UnclassifiedTest && referencedBySiblingForm(path) {
			testType = SupportFile
		}
		dt := DiscoveredTest{
			Path:    relPath,
			AbsPath: path,
			Type:    testType,
			FullDir: dir,
		}

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

func isReferenceSupportFile(base string) bool {
	base = strings.ToLower(base)
	for _, suffix := range []string{"-ref.html", "_ref.html", ".ref.html", "-notref.html", "_notref.html", ".notref.html", "-reference.html"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

type DiscoveredTest struct {
	Path        string
	AbsPath     string
	Type        TestType
	FullDir     string
	RefPath     string
	RefRelation string
}

func classifyTest(path string) TestType {
	if strings.HasSuffix(strings.ToLower(path), ".any.js") {
		return TestHarness
	}
	base := strings.ToLower(filepath.Base(path))
	if strings.Contains(base, "-manual.") || strings.Contains(base, ".manual.") || strings.HasPrefix(base, "manual-") {
		return ManualTest
	}
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

		if linesRead > 50 {
			break
		}

		lower := strings.ToLower(line)
		if strings.Contains(lower, "<meta") && strings.Contains(lower, "flags") && (strings.Contains(lower, "interact") || strings.Contains(lower, "manual")) {
			return ManualTest
		}

		if refLinkRe.MatchString(line) || refHrefFirstRe.MatchString(line) {
			return RefTest
		}

		if strings.Contains(lower, "manual") && strings.Contains(lower, `rel="help"`) {
			return ManualTest
		}

		if strings.Contains(lower, "testharness.js") {
			return TestHarness
		}
	}

	if strings.Contains(base, "-crash.") {
		return SmokeTest
	}
	return UnclassifiedTest
}

var formActionAttrRe = regexp.MustCompile(`(?is)<form\b[^>]*\baction\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)

func referencedBySiblingForm(candidate string) bool {
	dir := filepath.Dir(candidate)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == filepath.Base(candidate) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".html" && ext != ".htm" && ext != ".xhtml" {
			continue
		}
		parent := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(parent)
		if err != nil || len(data) > 1<<20 {
			continue
		}
		lower := strings.ToLower(string(data))
		if !strings.Contains(lower, "testharness.js") {
			continue
		}
		for _, attrs := range formActionAttrRe.FindAllSubmatch(data, -1) {
			action := ""
			for _, group := range attrs[1:] {
				if len(group) != 0 {
					action = string(group)
					break
				}
			}
			parsed, err := url.Parse(action)
			if err != nil || parsed.IsAbs() || parsed.Host != "" {
				continue
			}
			target := parsed.Path
			if target == "" {
				continue
			}
			targetAbs, err := filepath.Abs(filepath.Join(dir, filepath.FromSlash(target)))
			if err == nil && targetAbs == candidateAbs {
				return true
			}
		}
	}
	return false
}

var refLinkRe = regexp.MustCompile(`(?i)<link\b[^>]*\brel\s*=\s*["']?(match|mismatch)["']?[^>]*\bhref\s*=\s*["']?([^\s"'<>]+)`)
var refHrefFirstRe = regexp.MustCompile(`(?i)<link\b[^>]*\bhref\s*=\s*["']?([^\s"'<>]+)["']?[^>]*\brel\s*=\s*["']?(match|mismatch)["']?`)

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

func resolveRefPath(testPath, href, wptRoot string) string {
	if strings.HasPrefix(href, "/") {
		return filepath.Join(wptRoot, href)
	}
	return filepath.Join(filepath.Dir(testPath), href)
}

func ShouldSkip(dt DiscoveredTest, support FeatureSupport) (bool, string) {
	switch dt.Type {
	case ManualTest:
		return true, "requires human interaction and is not run automatically"
	case SupportFile:
		return true, "referenced form-action helper; exercised by its parent test"
	case UnclassifiedTest:
		return true, "no automated WPT marker found; classification review required"
	}

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
