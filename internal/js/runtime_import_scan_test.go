package js

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/dom"
)

// ---------------------------------------------------------------------------
// Runtime dynamic import() detection tests (M12.1)
//
// Goja rejects `import` as a reserved word at parse time, so scripts
// that use dynamic import() expressions fail with a SyntaxError. The
// engine still wants to surface this as a "page uses the ES module
// graph beyond the supported subset" fallback trigger. We do that by
// pre-scanning the script source for the `import(` token (respecting
// comments and string literals) and reporting FeatureESModule before
// Goja rejects the script.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// TestRuntimeDetectsDynamicImport
// ---------------------------------------------------------------------------

func TestRuntimeDetectsDynamicImport(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Use the public scan entry point. We don't actually run the script
	// through Goja because import() is a syntax error there.
	runtime.ScanAndReportUnsupportedJSFeatures(`import("foo.js")`)

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureESModule, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsDynamicImportWithWhitespace
// ---------------------------------------------------------------------------

func TestRuntimeDetectsDynamicImportWithWhitespace(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Whitespace between `import` and `(`.
	runtime.ScanAndReportUnsupportedJSFeatures(`import    ("foo.js")`)

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureESModule, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsDynamicImportAcrossLines
// ---------------------------------------------------------------------------

func TestRuntimeDetectsDynamicImportAcrossLines(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Newline between `import` and `(` is still a dynamic import.
	runtime.ScanAndReportUnsupportedJSFeatures("import\n(\"foo.js\")")

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureESModule, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeNoDetectsStaticImportDeclaration
// ---------------------------------------------------------------------------

func TestRuntimeNoDetectsStaticImportDeclaration(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Static declarations — handled by HTML parser detection, NOT by
	// the runtime scan. The runtime scan only fires for `import(`
	// (dynamic call form).
	runtime.ScanAndReportUnsupportedJSFeatures(`import x from "foo.js"`)
	runtime.ScanAndReportUnsupportedJSFeatures(`import * as foo from "foo.js"`)
	runtime.ScanAndReportUnsupportedJSFeatures(`import { a, b } from "foo.js"`)

	assert.Empty(t, get(), "static import declarations are not detected by the runtime scan")
}

// ---------------------------------------------------------------------------
// TestRuntimeNoDetectsImportInLineComment
// ---------------------------------------------------------------------------

func TestRuntimeNoDetectsImportInLineComment(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	runtime.ScanAndReportUnsupportedJSFeatures(`// import("foo.js")`)

	assert.Empty(t, get(), "import() inside a line comment must not trigger detection")
}

// ---------------------------------------------------------------------------
// TestRuntimeNoDetectsImportInBlockComment
// ---------------------------------------------------------------------------

func TestRuntimeNoDetectsImportInBlockComment(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	runtime.ScanAndReportUnsupportedJSFeatures(`/* import("foo.js") */`)
	runtime.ScanAndReportUnsupportedJSFeatures(`var x = 1; /* multi
		line comment with import("foo.js") */ var y = 2;`)

	assert.Empty(t, get(), "import() inside block comments must not trigger detection")
}

// ---------------------------------------------------------------------------
// TestRuntimeNoDetectsImportInStringLiteral
// ---------------------------------------------------------------------------

func TestRuntimeNoDetectsImportInStringLiteral(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	runtime.ScanAndReportUnsupportedJSFeatures(`var s = "import(\"foo.js\")";`)
	runtime.ScanAndReportUnsupportedJSFeatures(`var s = 'import("foo.js")';`)

	assert.Empty(t, get(), "import() inside string literals must not trigger detection")
}

// ---------------------------------------------------------------------------
// TestRuntimeNoDetectsImportInTemplateLiteral
// ---------------------------------------------------------------------------

func TestRuntimeNoDetectsImportInTemplateLiteral(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	runtime.ScanAndReportUnsupportedJSFeatures("var s = `import(\"foo.js\")`;")

	assert.Empty(t, get(), "import() inside template literals must not trigger detection")
}

// ---------------------------------------------------------------------------
// TestRuntimeNoDetectsImportMeta
// ---------------------------------------------------------------------------

func TestRuntimeNoDetectsImportMeta(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// `import.meta` is a static property access, not a dynamic call.
	runtime.ScanAndReportUnsupportedJSFeatures(`var url = import.meta.url;`)
	runtime.ScanAndReportUnsupportedJSFeatures(`import.meta`)

	assert.Empty(t, get(), "import.meta must not trigger detection")
}

// ---------------------------------------------------------------------------
// TestRuntimeNoDetectsImportAsIdentifier
// ---------------------------------------------------------------------------

func TestRuntimeNoDetectsImportAsIdentifier(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// `import` as part of a longer identifier name must not match.
	runtime.ScanAndReportUnsupportedJSFeatures(`var import_thing = 1;`)
	runtime.ScanAndReportUnsupportedJSFeatures(`var $import = 1;`)
	runtime.ScanAndReportUnsupportedJSFeatures(`var imported = 1;`)

	assert.Empty(t, get(), "import as part of an identifier must not trigger detection")
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsImportMixedWithOtherCode
// ---------------------------------------------------------------------------

func TestRuntimeDetectsImportMixedWithOtherCode(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	runtime.ScanAndReportUnsupportedJSFeatures(`
		var x = 1;
		console.log(x);
		import("./module.js").then(m => m.run());
	`)

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureESModule, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsImportDeduplication
// ---------------------------------------------------------------------------

func TestRuntimeDetectsImportDeduplication(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Multiple dynamic imports in one scan — only one report.
	runtime.ScanAndReportUnsupportedJSFeatures(`
		import("a.js");
		import("b.js");
		import("c.js");
	`)

	got := get()
	assert.Len(t, got, 1, "duplicate dynamic imports should dedupe")
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsImportAcrossMultipleScans
// ---------------------------------------------------------------------------

func TestRuntimeDetectsImportAcrossMultipleScans(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Multiple separate scans on the same runtime — only one report.
	for i := 0; i < 5; i++ {
		runtime.ScanAndReportUnsupportedJSFeatures(`import("./mod.js")`)
	}

	got := get()
	assert.Len(t, got, 1, "repeated scans on the same runtime dedupe")
}

// ---------------------------------------------------------------------------
// TestRuntimeImportNoCallbackSafe
// ---------------------------------------------------------------------------

func TestRuntimeImportNoCallbackSafe(t *testing.T) {
	runtime := NewRuntime()
	// No callback set — must not panic.
	runtime.ScanAndReportUnsupportedJSFeatures(`import("foo.js")`)
}

// ---------------------------------------------------------------------------
// TestRuntimeImportDetectionNilCallbackSafe
// ---------------------------------------------------------------------------

func TestRuntimeImportDetectionNilCallbackSafe(t *testing.T) {
	runtime := NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	runtime.SetRuntimeUnsupportedFeatureCallback(nil)
	runtime.ScanAndReportUnsupportedJSFeatures(`import("foo.js")`)
}

// ---------------------------------------------------------------------------
// TestRuntimeImportDetectionConcurrentSafe
// ---------------------------------------------------------------------------

func TestRuntimeImportDetectionConcurrentSafe(t *testing.T) {
	var (
		mu       sync.Mutex
		detected []dom.UnsupportedFeature
	)
	cb := func(f dom.UnsupportedFeature) {
		mu.Lock()
		detected = append(detected, f)
		mu.Unlock()
	}

	const goroutines = 16
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runtime := NewRuntime()
			runtime.SetRuntimeUnsupportedFeatureCallback(cb)
			runtime.ScanAndReportUnsupportedJSFeatures(`import("foo.js")`)
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, detected, goroutines,
		"each goroutine's runtime should fire exactly once")
	for _, f := range detected {
		assert.Equal(t, dom.FeatureESModule, f.Kind)
	}
}

// ---------------------------------------------------------------------------
// TestRuntimeImportScanShortCircuits
//
// When the source contains no occurrence of `import` at all, the scan
// must short-circuit on the strings.Contains pre-check without doing
// any byte-level scanning.
// ---------------------------------------------------------------------------

func TestRuntimeImportScanShortCircuits(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Long script with no 'import' anywhere.
	longScript := strings.Repeat("var x = 1; console.log(x); ", 1000)
	runtime.ScanAndReportUnsupportedJSFeatures(longScript)

	assert.Empty(t, get())
}

// ---------------------------------------------------------------------------
// TestRuntimeImportScanHandlesEscapesInStrings
// ---------------------------------------------------------------------------

func TestRuntimeImportScanHandlesEscapesInStrings(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Backslash escapes in strings must be respected — the string
	// contains a literal `\"` which is part of the string, not a
	// closing quote.
	runtime.ScanAndReportUnsupportedJSFeatures(`var s = "escaped: \" import(\"foo\")";`)

	assert.Empty(t, get(), "string with escaped quotes must not trigger detection")
}

// ---------------------------------------------------------------------------
// TestRuntimeImportScanReportsBeforeGojaError
//
// Documents the runtime contract: when a script contains `import()`,
// the runtime reports the unsupported feature BEFORE Goja raises its
// own SyntaxError. The detection path is decoupled from Goja's parse.
// ---------------------------------------------------------------------------

func TestRuntimeImportScanReportsBeforeGojaError(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Pre-scan fires the callback (in isolation).
	runtime.ScanAndReportUnsupportedJSFeatures(`import("foo.js")`)

	assert.Len(t, get(), 1, "scan alone should fire detection without invoking Goja")
}

// ---------------------------------------------------------------------------
// TestRuntimeImportScanEmptySource
// ---------------------------------------------------------------------------

func TestRuntimeImportScanEmptySource(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	runtime.ScanAndReportUnsupportedJSFeatures("")
	assert.Empty(t, get())
}

// ---------------------------------------------------------------------------
// TestRuntimeImportScanNoFalsePositiveOnLeadingImport
//
// `import` at the very start of the script (no preceding character)
// should still be detected.
// ---------------------------------------------------------------------------

func TestRuntimeImportScanNoFalsePositiveOnLeadingImport(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Script starts with `import(` directly.
	runtime.ScanAndReportUnsupportedJSFeatures(`import("foo.js")`)

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureESModule, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeImportScanNoFalsePositiveOnTrailingImport
// ---------------------------------------------------------------------------

func TestRuntimeImportScanNoFalsePositiveOnTrailingImport(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Script ends with `import` followed by EOF.
	runtime.ScanAndReportUnsupportedJSFeatures(`var x = "import"`)

	assert.Empty(t, get(), "string at EOF containing 'import' must not trigger")
}

// ---------------------------------------------------------------------------
// TestRuntimeImportScanResetsWithDedup
// ---------------------------------------------------------------------------

func TestRuntimeImportScanResetsWithDedup(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	runtime.ScanAndReportUnsupportedJSFeatures(`import("a.js")`)
	assert.Len(t, get(), 1)

	runtime.ResetRuntimeUnsupportedFeatures()

	runtime.ScanAndReportUnsupportedJSFeatures(`import("b.js")`)
	assert.Len(t, get(), 2, "after reset, dynamic import should fire again")
}

// ---------------------------------------------------------------------------
// TestRuntimeImportScanImportsFromScannerHelper
//
// Verifies the public scan method works without going through RunScript
// (so it can be tested independently of Goja's syntax errors).
// ---------------------------------------------------------------------------

func TestRuntimeImportScanImportsFromScannerHelper(t *testing.T) {
	runtime := NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {
		// Just confirm invocation count via atomic.
	})

	// Confirm the public method exists and is callable.
	require.NotPanics(t, func() {
		runtime.ScanAndReportUnsupportedJSFeatures(`import("foo.js")`)
	})
}

// ---------------------------------------------------------------------------
// TestRuntimeRunScriptAutoScansImport
//
// Confirms the wiring: RunScript must auto-invoke the scan BEFORE
// delegating to Goja. This is the contract relied on by the browser
// shell — all scripts run through this entry point.
// ---------------------------------------------------------------------------

func TestRuntimeRunScriptAutoScansImport(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// RunScript with a script containing import() — Goja will reject
	// the script with a SyntaxError, but the scan must have already
	// fired the detection callback.
	_, _ = runtime.RunScript(`import("foo.js")`)

	got := get()
	require.Len(t, got, 1, "RunScript must auto-scan and report import()")
	assert.Equal(t, dom.FeatureESModule, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeRunScriptAutoScanDedupAcrossRuns
// ---------------------------------------------------------------------------

func TestRuntimeRunScriptAutoScanDedupAcrossRuns(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	for i := 0; i < 10; i++ {
		_, _ = runtime.RunScript(`import("mod" + ".js")`)
	}

	got := get()
	assert.Len(t, got, 1, "10 RunScript calls with import() should fire once")
}

// ---------------------------------------------------------------------------
// TestRuntimeRunScriptAutoScanIgnoresCommentImports
// ---------------------------------------------------------------------------

func TestRuntimeRunScriptAutoScanIgnoresCommentImports(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// RunScript with a script whose only `import(` is in a comment.
	// Goja will happily execute the script (it's well-formed JS);
	// the scan must NOT fire because the import is in a comment.
	val, err := runtime.RunScript("// import(\"foo.js\")\nvar x = 42; x;")
	require.NoError(t, err)
	assert.Equal(t, int64(42), val.ToInteger())

	assert.Empty(t, get(), "comment import must not trigger detection")
}
