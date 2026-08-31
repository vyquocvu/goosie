package js_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// Runtime canvas/unsupported API detection tests (M12.1)
//
// These tests verify that the JS runtime reports unsupported engine features
// (canvas, video, etc.) when JavaScript creates them via document.createElement.
// The detection feeds the fallback layer's fallback.Policy.
// ---------------------------------------------------------------------------

// captureCallback returns a callback that records all detected features
// into the given slice, plus a getter that returns them as a copy.
func captureCallback() (cb func(dom.UnsupportedFeature), get func() []dom.UnsupportedFeature) {
	var mu sync.Mutex
	var detected []dom.UnsupportedFeature
	cb = func(f dom.UnsupportedFeature) {
		mu.Lock()
		detected = append(detected, f)
		mu.Unlock()
	}
	get = func() []dom.UnsupportedFeature {
		mu.Lock()
		defer mu.Unlock()
		out := make([]dom.UnsupportedFeature, len(detected))
		copy(out, detected)
		return out
	}
	return
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsCanvasCreateElement
// ---------------------------------------------------------------------------

func TestRuntimeDetectsCanvasCreateElement(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`document.createElement('canvas');`)
	require.NoError(t, err)

	got := get()
	require.Len(t, got, 1, "should detect exactly one canvas")
	assert.Equal(t, dom.FeatureCanvas, got[0].Kind, "kind should be canvas")
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsVideoCreateElement
// ---------------------------------------------------------------------------

func TestRuntimeDetectsVideoCreateElement(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`document.createElement('video');`)
	require.NoError(t, err)

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureVideo, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsAudioCreateElement
// ---------------------------------------------------------------------------

func TestRuntimeDetectsAudioCreateElement(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`document.createElement('audio');`)
	require.NoError(t, err)

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureAudio, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsIframeCreateElement
// ---------------------------------------------------------------------------

func TestRuntimeDetectsIframeCreateElement(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`document.createElement('iframe');`)
	require.NoError(t, err)

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureIframe, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsCanvasDeduplication
// ---------------------------------------------------------------------------

func TestRuntimeDetectsCanvasDeduplication(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Create canvas many times — callback should fire only once per kind.
	_, err := runtime.RunScript(`
		for (var i = 0; i < 50; i++) {
			document.createElement('canvas');
		}
	`)
	require.NoError(t, err)

	got := get()
	assert.Len(t, got, 1, "canvas detection should deduplicate within a single runtime")
	assert.Equal(t, dom.FeatureCanvas, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsMultipleKindsDistinct
// ---------------------------------------------------------------------------

func TestRuntimeDetectsMultipleKindsDistinct(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`
		document.createElement('canvas');
		document.createElement('video');
		document.createElement('audio');
		document.createElement('iframe');
	`)
	require.NoError(t, err)

	got := get()
	require.Len(t, got, 4, "should detect 4 distinct unsupported kinds")

	seen := make(map[dom.UnsupportedFeatureKind]bool)
	for _, f := range got {
		seen[f.Kind] = true
	}
	assert.True(t, seen[dom.FeatureCanvas], "should detect canvas")
	assert.True(t, seen[dom.FeatureVideo], "should detect video")
	assert.True(t, seen[dom.FeatureAudio], "should detect audio")
	assert.True(t, seen[dom.FeatureIframe], "should detect iframe")
}

// ---------------------------------------------------------------------------
// TestRuntimeNoCallbackDoesNotPanic
// ---------------------------------------------------------------------------

func TestRuntimeNoCallbackDoesNotPanic(t *testing.T) {
	runtime := js.NewRuntime()
	// No callback set — should not panic on createElement of unsupported tags.
	_, err := runtime.RunScript(`
		document.createElement('canvas');
		document.createElement('video');
		document.createElement('div'); // supported — must not be reported
	`)
	require.NoError(t, err, "unsupported element creation without callback must not error")
}

// ---------------------------------------------------------------------------
// TestRuntimeSupportedTagsNotDetected
// ---------------------------------------------------------------------------

func TestRuntimeSupportedTagsNotDetected(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`
		document.createElement('div');
		document.createElement('p');
		document.createElement('span');
		document.createElement('img');
		document.createElement('a');
		document.createElement('h1');
	`)
	require.NoError(t, err)

	got := get()
	assert.Empty(t, got, "supported tags must not trigger detection")
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsCanvasCaseInsensitive
// ---------------------------------------------------------------------------

func TestRuntimeDetectsCanvasCaseInsensitive(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Mixed-case tag names should still be detected as canvas.
	_, err := runtime.RunScript(`
		document.createElement('CANVAS');
		document.createElement('Canvas');
		document.createElement('canvas');
	`)
	require.NoError(t, err)

	got := get()
	assert.Len(t, got, 1, "all case variants should deduplicate to a single canvas report")
	assert.Equal(t, dom.FeatureCanvas, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsCanvasThroughAppendChild
// ---------------------------------------------------------------------------

func TestRuntimeDetectsCanvasThroughAppendChild(t *testing.T) {
	runtime := js.NewRuntime()
	runtime.SetHTMLContent(`<html><body><div id="target"></div></body></html>`)
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// createElement + appendChild is the "page behavior" pattern.
	_, err := runtime.RunScript(`
		var canvas = document.createElement('canvas');
		canvas.width = 800;
		canvas.height = 600;
		var target = document.getElementById('target');
		target.appendChild(canvas);
	`)
	require.NoError(t, err)

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureCanvas, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsCanvasFromInlineScript
//
// SetHTMLContent populates the DOM but does NOT execute inline <script>
// tags (those are extracted and run separately by the browser shell).
// The runtime detection path runs whenever any JS reaches
// document.createElement — so we simulate the inline script body by
// running it explicitly here.
// ---------------------------------------------------------------------------

func TestRuntimeDetectsCanvasFromInlineScript(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// The HTML has no <canvas> — only an inline script that creates one.
	// Proves the runtime detection path is independent of HTML parsing.
	runtime.SetHTMLContent(`<html><body><h1>Hello</h1></body></html>`)

	const inlineScript = `
		var c = document.createElement('canvas');
		c.id = 'chart';
		document.body.appendChild(c);
	`
	_, err := runtime.RunScript(inlineScript)
	require.NoError(t, err)

	got := get()
	require.Len(t, got, 1, "inline script creating canvas should be detected")
	assert.Equal(t, dom.FeatureCanvas, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectionCallbackConcurrentSafe
//
// Goja VMs are not goroutine-safe — concurrent RunScript calls on the
// SAME Runtime race against the VM's own proxy state. We instead fan out
// to N separate Runtimes, each driven by one goroutine, which still
// proves that the Go-side callback piece is goroutine-safe.
// ---------------------------------------------------------------------------

func TestRuntimeDetectionCallbackConcurrentSafe(t *testing.T) {
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
			runtime := js.NewRuntime()
			runtime.SetRuntimeUnsupportedFeatureCallback(cb)
			_, _ = runtime.RunScript(`document.createElement('canvas');`)
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, detected, goroutines,
		"each goroutine runs its own runtime and should fire once")
	assert.Equal(t, dom.FeatureCanvas, detected[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectionDedupUnderRepeatedRuns
// ---------------------------------------------------------------------------

func TestRuntimeDetectionDedupUnderRepeatedRuns(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	for i := 0; i < 100; i++ {
		_, err := runtime.RunScript(`document.createElement('canvas');`)
		require.NoError(t, err)
	}

	got := get()
	assert.Len(t, got, 1, "100 canvas creations should still deduplicate to one report")
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectionResetClearsDedup
// ---------------------------------------------------------------------------

func TestRuntimeDetectionResetClearsDedup(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`document.createElement('canvas');`)
	require.NoError(t, err)
	assert.Len(t, get(), 1)

	runtime.ResetRuntimeUnsupportedFeatures()

	_, err = runtime.RunScript(`document.createElement('canvas');`)
	require.NoError(t, err)
	assert.Len(t, get(), 2, "after reset, detection should fire again")
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectsCanvasCallbackInvocationCount
// ---------------------------------------------------------------------------

func TestRuntimeDetectsCanvasCallbackInvocationCount(t *testing.T) {
	runtime := js.NewRuntime()
	var count int64
	runtime.SetRuntimeUnsupportedFeatureCallback(func(f dom.UnsupportedFeature) {
		atomic.AddInt64(&count, 1)
	})

	_, err := runtime.RunScript(`
		document.createElement('canvas');
		document.createElement('canvas');
		document.createElement('video');
		document.createElement('video');
	`)
	require.NoError(t, err)

	assert.Equal(t, int64(2), atomic.LoadInt64(&count),
		"two distinct kinds should invoke the callback exactly twice")
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectionIndependentFromParser
// ---------------------------------------------------------------------------

func TestRuntimeDetectionIndependentFromParser(t *testing.T) {
	// The runtime detection path is fully independent of the HTML parser
	// detection path. They share only the dom.UnsupportedFeature type.
	// This test confirms the runtime-only path works without ever calling
	// dom.NewParser().
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Never call SetHTMLContent — no HTML is ever parsed.
	_, err := runtime.RunScript(`document.createElement('canvas');`)
	require.NoError(t, err)

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureCanvas, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectionEmptyTagName
// ---------------------------------------------------------------------------

func TestRuntimeDetectionEmptyTagName(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	_, err := runtime.RunScript(`document.createElement('');`)
	require.NoError(t, err, "empty tag must not error")

	assert.Empty(t, get(), "empty tag name must not trigger any detection")
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectionScriptErrorDoesNotMaskDetection
// ---------------------------------------------------------------------------

func TestRuntimeDetectionScriptErrorDoesNotMaskDetection(t *testing.T) {
	runtime := js.NewRuntime()
	cb, get := captureCallback()
	runtime.SetRuntimeUnsupportedFeatureCallback(cb)

	// Script contains both a canvas creation and a runtime error.
	// The canvas detection should still fire — errors don't roll back
	// the report.
	_, _ = runtime.RunScript(`
		document.createElement('canvas');
		undefinedVariable.foo;
	`)

	got := get()
	require.Len(t, got, 1)
	assert.Equal(t, dom.FeatureCanvas, got[0].Kind)
}

// ---------------------------------------------------------------------------
// TestRuntimeDetectionNilCallbackSafe
// ---------------------------------------------------------------------------

func TestRuntimeDetectionNilCallbackSafe(t *testing.T) {
	runtime := js.NewRuntime()
	// Set callback then clear it — must remain nil-safe.
	runtime.SetRuntimeUnsupportedFeatureCallback(func(dom.UnsupportedFeature) {})
	runtime.SetRuntimeUnsupportedFeatureCallback(nil)

	_, err := runtime.RunScript(`document.createElement('canvas');`)
	require.NoError(t, err, "clearing callback must not error")
}
