package js

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

// ---------------------------------------------------------------------------
// Benchmarks for M12.1 WebSocket / Worker / ServiceWorker stubs.
// ---------------------------------------------------------------------------

// BenchmarkRuntimeWebSocketStub measures the cost of `new WebSocket(url)`
// when the runtime detection callback is installed.
func BenchmarkRuntimeWebSocketStub(b *testing.B) {
	runtime := NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Each iteration creates a fresh WebSocket. The dedup map
		// means only the first call traverses to the bridge.
		// We alternate to exercise the dedup-bypass path on every
		// other call.
		if i%2 == 0 {
			_, _ = runtime.RunScript(`new WebSocket("wss://a");`)
		} else {
			_, _ = runtime.RunScript(`new WebSocket("wss://b");`)
		}
	}
}

// BenchmarkRuntimeWorkerStub measures `new Worker(url)`.
func BenchmarkRuntimeWorkerStub(b *testing.B) {
	runtime := NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			_, _ = runtime.RunScript(`new Worker("a.js");`)
		} else {
			_, _ = runtime.RunScript(`new Worker("b.js");`)
		}
	}
}

// BenchmarkRuntimeServiceWorkerRegister measures
// `navigator.serviceWorker.register(url)`.
func BenchmarkRuntimeServiceWorkerRegister(b *testing.B) {
	runtime := NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			_, _ = runtime.RunScript(`navigator.serviceWorker.register("/a.js");`)
		} else {
			_, _ = runtime.RunScript(`navigator.serviceWorker.register("/b.js");`)
		}
	}
}

// BenchmarkRuntimeWebSocketStubDedupHit measures the dedup path —
// after the first call, subsequent calls are short-circuited.
func BenchmarkRuntimeWebSocketStubDedupHit(b *testing.B) {
	runtime := NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})

	// Pre-warm the dedup cache.
	runtime.reportRuntimeUnsupportedFeature(dom.FeatureWebSocket)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = runtime.RunScript(`new WebSocket("wss://a");`)
	}
}

// BenchmarkRuntimeWebSocketStubNoCallback measures the cost when no
// callback is installed.
func BenchmarkRuntimeWebSocketStubNoCallback(b *testing.B) {
	runtime := NewRuntime()
	// No callback installed.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = runtime.RunScript(`new WebSocket("wss://a");`)
	}
}

// BenchmarkRuntimeWebSocketStubMethodsAccess measures the cost of
// invoking stub methods (close, send, etc.) on the returned object.
func BenchmarkRuntimeWebSocketStubMethodsAccess(b *testing.B) {
	runtime := NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = runtime.RunScript(`
			var ws = new WebSocket("wss://a");
			ws.close();
			ws.send("hi");
			ws.addEventListener("message", function(){});
		`)
	}
}

// BenchmarkRuntimeWebSocketReportPathDirect measures the Go-side
// reportRuntimeUnsupportedFeature hot path for FeatureWebSocket
// directly (no JS, no Goja). This isolates the bridge overhead from
// Goja's VM execution cost.
func BenchmarkRuntimeWebSocketReportPathDirect(b *testing.B) {
	runtime := NewRuntime()
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Alternate kinds so dedup doesn't dominate.
		if i%2 == 0 {
			runtime.reportRuntimeUnsupportedFeature(dom.FeatureWebSocket)
		} else {
			runtime.reportRuntimeUnsupportedFeature(dom.FeatureServiceWorker)
		}
	}
}
