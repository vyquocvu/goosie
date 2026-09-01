package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// Benchmarks for M12.1 runtime unsupported feature detection hook.
// ---------------------------------------------------------------------------

// BenchmarkRuntimeCreateElementDivNoCallback measures the baseline
// cost of document.createElement for a supported tag with no callback
// installed. Used to confirm the bridge adds negligible overhead
// when there is nothing to report.
func BenchmarkRuntimeCreateElementDivNoCallback(b *testing.B) {
	runtime := js.NewRuntime()
	// Intentionally do NOT install a callback.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = runtime.RunScript(`document.createElement('div');`)
	}
}

// BenchmarkRuntimeCreateElementCanvasNoCallback measures the cost of
// the detection path when the callback is nil. The bridge still runs
// (with a nil-check short-circuit), but no allocation is required.
func BenchmarkRuntimeCreateElementCanvasNoCallback(b *testing.B) {
	runtime := js.NewRuntime()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Dedup means only the first call traverses to the bridge.
		// We rotate tags to ensure each call hits the bridge at least
		// once across iterations.
		if i%2 == 0 {
			_, _ = runtime.RunScript(`document.createElement('canvas');`)
		} else {
			_, _ = runtime.RunScript(`document.createElement('video');`)
		}
	}
}

// BenchmarkRuntimeCreateElementCanvasWithCallback measures the cost of
// the detection path when a callback IS installed. This is the cost
// page authors pay when fallback triggers are wired up.
func BenchmarkRuntimeCreateElementCanvasWithCallback(b *testing.B) {
	runtime := js.NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {
		// No-op: simulates a typical fallback.Policy.Record call
		// without measuring downstream cost.
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Rotate tags so dedup does not collapse the benchmark to
		// "early return" territory after the first iteration.
		if i%2 == 0 {
			_, _ = runtime.RunScript(`document.createElement('canvas');`)
		} else {
			_, _ = runtime.RunScript(`document.createElement('video');`)
		}
	}
}

// BenchmarkRuntimeDetectionOverhead measures the per-call overhead of
// the bridge when reporting is active. We benchmark the full bridge
// function call (string switch + dedup + callback dispatch).
func BenchmarkRuntimeDetectionOverhead(b *testing.B) {
	runtime := js.NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 'div' is a supported tag — bridge returns early on the
		// Go-side switch without dispatching the callback.
		_, _ = runtime.RunScript(`document.createElement('div');`)
	}
}

// BenchmarkRuntimeReportPathDirect measures the Go-side
// reportRuntimeUnsupportedFeature hot path directly (no JS).
func BenchmarkRuntimeReportPathDirect(b *testing.B) {
	runtime := js.NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Alternate kinds so dedup doesn't dominate the measurement.
		if i%2 == 0 {
			runtime.ReportRuntimeUnsupportedFeature(dom.FeatureCanvas)
		} else {
			runtime.ReportRuntimeUnsupportedFeature(dom.FeatureVideo)
		}
	}
}

// BenchmarkRuntimeResetDedup measures the cost of clearing the dedup
// cache between navigations. Expected to be O(1) amortized (map clear).
func BenchmarkRuntimeResetDedup(b *testing.B) {
	runtime := js.NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})

	// Pre-warm the dedup map with all known kinds.
	for _, k := range []dom.UnsupportedFeatureKind{
		dom.FeatureCanvas, dom.FeatureVideo, dom.FeatureAudio,
		dom.FeatureIframe, dom.FeatureObject, dom.FeatureEmbed,
	} {
		runtime.ReportRuntimeUnsupportedFeature(k)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.ResetRuntimeUnsupportedFeatures()
	}
}
