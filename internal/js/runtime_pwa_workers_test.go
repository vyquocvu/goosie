package js

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/dom"
)

// ---------------------------------------------------------------------------
// Runtime WebSocket / Worker / ServiceWorker detection tests (M12.1)
//
// The engine does not implement WebSocket, Web Worker, or ServiceWorker.
// These tests verify that JS API surface usage of these features is
// detected and reported to the runtime detection callback so the
// fallback layer can mark the page for compatibility. The stubs that
// back these APIs must return no-op objects so that chained calls
// don't blow up the page's JS execution.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// WebSocket
// ---------------------------------------------------------------------------

// TestRuntimeDetectsWebSocketConstructor verifies `new WebSocket(url)`
// is detected as FeatureWebSocket.
func TestRuntimeDetectsWebSocketConstructor(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	val, err := runtime.RunScript(`
		try {
			var ws = new WebSocket("wss://example.com/socket");
			"ok: " + typeof ws + " " + ws.readyState;
		} catch (e) {
			"err: " + e.toString();
		}
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), "ok: object",
		"WebSocket stub must return an object, not throw")

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureWebSocket, got[0].Kind)
}

// TestRuntimeDetectsWebSocketDeduplication — multiple WebSocket
// constructions fire the callback once.
func TestRuntimeDetectsWebSocketDeduplication(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`
		new WebSocket("wss://a");
		new WebSocket("wss://b");
		new WebSocket("wss://c");
	`)
	require.NoError(t, err)

	got := get()
	assert.Len(t, got, 1, "WebSocket constructor should dedupe")
}

// TestRuntimeDetectsWebSocketNoCallbackSafe — no panic when callback
// is nil.
func TestRuntimeDetectsWebSocketNoCallbackSafe(t *testing.T) {
	runtime := NewRuntime()
	// No callback installed.
	_, err := runtime.RunScript(`new WebSocket("wss://example.com");`)
	require.NoError(t, err, "WebSocket without callback must not error")
}

// TestRuntimeWebSocketStubReturnsUsableObject — the stub must have
// close/send/addEventListener methods that don't throw, so chained
// calls don't crash the page.
func TestRuntimeWebSocketStubReturnsUsableObject(t *testing.T) {
	runtime := NewRuntime()

	val, err := runtime.RunScript(`
		var ws = new WebSocket("wss://example.com");
		// Each method must exist and be callable without throwing.
		ws.close();
		ws.send("hello");
		var added = false;
		ws.addEventListener("message", function() { added = true; });
		typeof ws.close + " " + typeof ws.send + " " + typeof ws.addEventListener;
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), "function function function",
		"WebSocket stub must expose close/send/addEventListener as functions")
}

// ---------------------------------------------------------------------------
// Worker
// ---------------------------------------------------------------------------

// TestRuntimeDetectsWorkerConstructor verifies `new Worker(url)` is
// detected as FeatureWebWorker.
func TestRuntimeDetectsWorkerConstructor(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	val, err := runtime.RunScript(`
		try {
			var w = new Worker("worker.js");
			"ok: " + typeof w;
		} catch (e) {
			"err: " + e.toString();
		}
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), "ok: object",
		"Worker stub must return an object, not throw")

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureWebWorker, got[0].Kind)
}

// TestRuntimeDetectsWorkerDeduplication.
func TestRuntimeDetectsWorkerDeduplication(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`
		new Worker("a.js");
		new Worker("b.js");
	`)
	require.NoError(t, err)

	got := get()
	assert.Len(t, got, 1, "Worker constructor should dedupe")
}

// TestRuntimeDetectsWorkerNoCallbackSafe.
func TestRuntimeDetectsWorkerNoCallbackSafe(t *testing.T) {
	runtime := NewRuntime()
	_, err := runtime.RunScript(`new Worker("worker.js");`)
	require.NoError(t, err, "Worker without callback must not error")
}

// TestRuntimeWorkerStubReturnsUsableObject.
func TestRuntimeWorkerStubReturnsUsableObject(t *testing.T) {
	runtime := NewRuntime()

	val, err := runtime.RunScript(`
		var w = new Worker("worker.js");
		w.postMessage({ hello: "world" });
		w.terminate();
		typeof w.postMessage + " " + typeof w.terminate + " " + typeof w.addEventListener;
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), "function function function",
		"Worker stub must expose postMessage/terminate/addEventListener as functions")
}

// ---------------------------------------------------------------------------
// ServiceWorker
// ---------------------------------------------------------------------------

// TestRuntimeDetectsServiceWorkerRegister verifies
// `navigator.serviceWorker.register(url)` is detected as
// FeatureServiceWorker.
func TestRuntimeDetectsServiceWorkerRegister(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	val, err := runtime.RunScript(`
		try {
			// register() returns a promise; await its rejection.
			var p = navigator.serviceWorker.register("/sw.js");
			"ok: " + typeof p;
		} catch (e) {
			"err: " + e.toString();
		}
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), "ok: object",
		"serviceWorker.register must return a Promise-like object")

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureServiceWorker, got[0].Kind)
}

// TestRuntimeDetectsServiceWorkerDeduplication.
func TestRuntimeDetectsServiceWorkerDeduplication(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`
		navigator.serviceWorker.register("/a.js");
		navigator.serviceWorker.register("/b.js");
	`)
	require.NoError(t, err)

	got := get()
	assert.Len(t, got, 1, "serviceWorker.register should dedupe")
}

// TestRuntimeDetectsServiceWorkerNoCallbackSafe.
func TestRuntimeDetectsServiceWorkerNoCallbackSafe(t *testing.T) {
	runtime := NewRuntime()
	_, err := runtime.RunScript(`navigator.serviceWorker.register("/sw.js");`)
	require.NoError(t, err, "serviceWorker.register without callback must not error")
}

// TestRuntimeServiceWorkerStubHasExpectedMethods.
func TestRuntimeServiceWorkerStubHasExpectedMethods(t *testing.T) {
	runtime := NewRuntime()

	val, err := runtime.RunScript(`
		var sw = navigator.serviceWorker;
		var t = typeof sw;
		t + " reg:" + typeof sw.register + " get:" + typeof sw.getRegistration + " regs:" + typeof sw.getRegistrations;
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), "object reg:function get:function regs:function",
		"serviceWorker stub must expose register/getRegistration/getRegistrations as functions")
}

// TestRuntimeServiceWorkerGetRegistrationReturnsNull.
func TestRuntimeServiceWorkerGetRegistrationReturnsNull(t *testing.T) {
	runtime := NewRuntime()

	val, err := runtime.RunScript(`var r = navigator.serviceWorker.getRegistration(); typeof r;`)
	require.NoError(t, err)
	assert.Equal(t, "object", val.String(), "getRegistration stub returns null")
}

// ---------------------------------------------------------------------------
// Cross-feature isolation
// ---------------------------------------------------------------------------

// TestRuntimeDetectsEachKindDistinctly — when the page uses multiple
// unsupported APIs, each is detected as its own kind.
func TestRuntimeDetectsEachKindDistinctly(t *testing.T) {
	runtime := NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`
		new WebSocket("wss://example.com");
		new Worker("w.js");
		navigator.serviceWorker.register("/sw.js");
	`)
	require.NoError(t, err)

	got := get()
	require.Len(t, got, 3, "all three kinds must be detected independently")

	seen := make(map[dom.UnsupportedFeatureKind]bool)
	for _, f := range got {
		seen[f.Kind] = true
	}
	assert.True(t, seen[dom.FeatureWebSocket], "must detect WebSocket")
	assert.True(t, seen[dom.FeatureWebWorker], "must detect Worker")
	assert.True(t, seen[dom.FeatureServiceWorker], "must detect ServiceWorker")
}

// TestRuntimeWebSocketStubConcurrentSafe — multiple goroutines each
// owning a Runtime all fire their callbacks.
func TestRuntimeWebSocketStubConcurrentSafe(t *testing.T) {
	var (
		mu       sync.Mutex
		detected []dom.UnsupportedFeature
	)
	cb := func(f dom.UnsupportedFeature) {
		mu.Lock()
		detected = append(detected, f)
		mu.Unlock()
	}

	const goroutines = 8
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runtime := NewRuntime()
			runtime.SetRuntimeUnsupportedFeatureCallback(cb)
			_, _ = runtime.RunScript(`new WebSocket("wss://example.com");`)
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, detected, goroutines)
	for _, f := range detected {
		assert.Equal(t, dom.FeatureWebSocket, f.Kind)
	}
}

// TestRuntimeUnsupportedFeatureKindStrings — verify the new kinds
// have stable String() representations.
func TestRuntimeUnsupportedFeatureKindStrings(t *testing.T) {
	cases := []struct {
		k    dom.UnsupportedFeatureKind
		want string
	}{
		{dom.FeatureWebSocket, "websocket"},
		{dom.FeatureWebWorker, "web-worker"},
		{dom.FeatureServiceWorker, "service-worker"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, c.k.String(), "String() for %d", c.k)
	}
}

// TestRuntimeUnsupportedFeatureKindRoundTrip — every defined kind
// must have a non-empty String() and a unique name.
func TestRuntimeUnsupportedFeatureKindRoundTrip(t *testing.T) {
	allKinds := []dom.UnsupportedFeatureKind{
		dom.FeatureCanvas, dom.FeatureVideo, dom.FeatureAudio,
		dom.FeatureIframe, dom.FeatureESModule, dom.FeatureObject,
		dom.FeatureEmbed, dom.FeaturePWAManifest,
		dom.FeatureWebSocket, dom.FeatureWebWorker, dom.FeatureServiceWorker,
	}
	seen := make(map[string]bool)
	for _, k := range allKinds {
		s := k.String()
		assert.NotEmpty(t, s, "kind %d must have a String()", k)
		assert.False(t, seen[s], "duplicate String() value: %q for kind %d", s, k)
		seen[s] = true
	}
}
