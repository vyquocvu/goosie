package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// Benchmarks for M12.1 dynamic import() detection scanner.
// ---------------------------------------------------------------------------

// BenchmarkRuntimeImportScanNoMatch measures the cost of the scan
// when the script does NOT contain `import` at all. This should
// short-circuit on the strings.Contains pre-check and be near-free.
func BenchmarkRuntimeImportScanNoMatch(b *testing.B) {
	runtime := js.NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	script := strings.Repeat("var x = 1; console.log(x); ", 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.ScanAndReportUnsupportedJSFeatures(script)
	}
}

// BenchmarkRuntimeImportScanNoCallback measures the cost when no
// callback is installed. Should also short-circuit cheaply.
func BenchmarkRuntimeImportScanNoCallback(b *testing.B) {
	runtime := js.NewRuntime()
	// No callback installed.
	script := strings.Repeat("var x = 1; console.log(x); ", 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.ScanAndReportUnsupportedJSFeatures(script)
	}
}

// BenchmarkRuntimeImportScanLongScript measures the cost on a
// realistic-sized script without a dynamic import. Exercises the
// per-byte scanner loop.
func BenchmarkRuntimeImportScanLongScript(b *testing.B) {
	runtime := js.NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	// Long script containing the substring "import" but never as
	// `import(` (so the scanner must walk all the way through).
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("var imported = 1; var otherVar = 2; ")
	}
	script := sb.String()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.ScanAndReportUnsupportedJSFeatures(script)
	}
}

// BenchmarkRuntimeImportScanWithMatch measures the cost when the
// script DOES contain `import(`. The scanner short-circuits as
// soon as the first match is found and reports.
func BenchmarkRuntimeImportScanWithMatch(b *testing.B) {
	runtime := js.NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	script := "var pre = 1;\n" + strings.Repeat("var x = 1; ", 50) + "\nimport('foo.js')"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.ScanAndReportUnsupportedJSFeatures(script)
	}
}

// BenchmarkRuntimeImportScanShortCircuitOnDedup measures the cost
// when the dedup cache has already absorbed the kind — the scanner
// still walks the whole script, but the report is a no-op.
func BenchmarkRuntimeImportScanShortCircuitOnDedup(b *testing.B) {
	runtime := js.NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})

	// Pre-warm the dedup cache.
	runtime.ReportRuntimeUnsupportedFeature(dom.FeatureESModule)

	script := strings.Repeat("var x = 1; ", 100) + "\nimport('foo.js')"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.ScanAndReportUnsupportedJSFeatures(script)
	}
}

// BenchmarkRuntimeImportScanWithStringLiteral measures the cost on
// scripts with many string literals (forces the scanner to track
// string state).
func BenchmarkRuntimeImportScanWithStringLiteral(b *testing.B) {
	runtime := js.NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(`var s = "import(\"foo.js\")"; var y = 2; `)
	}
	script := sb.String()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.ScanAndReportUnsupportedJSFeatures(script)
	}
}

// BenchmarkRuntimeImportScanWithComments measures the cost on
// scripts with many comments.
func BenchmarkRuntimeImportScanWithComments(b *testing.B) {
	runtime := js.NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("// import(\"foo.js\")\n/* and import(\"bar.js\") */\nvar x = 1; ")
	}
	script := sb.String()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.ScanAndReportUnsupportedJSFeatures(script)
	}
}
