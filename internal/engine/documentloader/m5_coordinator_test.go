package documentloader

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestAsyncScriptFiresImmediately — M5: async scripts fire OnScript
// the moment their fetch completes, not at drain time.
func TestAsyncScriptFiresImmediately(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	ch := h.fetcher.register("https://example.com/async.js")
	coord.HandleResource(Resource{
		Kind:       KindScript,
		URL:        "async.js",
		ScriptMode: ScriptModeAsync,
	})

	waitForFetch(t, h.fetcher, time.Second, "https://example.com/async.js")
	ch <- fakeResponse{body: "console.log('async')"}

	// Wait for the async callback to fire.
	deadline := time.Now().Add(time.Second)
	var s callbackSnapshot
	for time.Now().Before(deadline) {
		s = h.cb.Snapshot()
		if len(s.Scripts) >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if len(s.Scripts) != 1 {
		t.Fatalf("async did not fire immediately; scripts=%d", len(s.Scripts))
	}
	if s.Scripts[0].Mode != ScriptModeAsync {
		t.Errorf("script mode = %v, want async", s.Scripts[0].Mode)
	}

	// Drain — async should not be re-emitted.
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	s = h.cb.Snapshot()
	if len(s.Scripts) != 1 {
		t.Errorf("async should not be re-emitted at drain; got %d", len(s.Scripts))
	}
}

// TestDeferScriptBuffersUntilDrain — M5: defer scripts buffer until
// HandleDocumentEnd; they execute in source order with classics.
func TestDeferScriptBuffersUntilDrain(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	ch1 := h.fetcher.register("https://example.com/defer.js")
	coord.HandleResource(Resource{
		Kind:       KindScript,
		URL:        "defer.js",
		ScriptMode: ScriptModeDefer,
	})

	waitForFetch(t, h.fetcher, time.Second, "https://example.com/defer.js")

	// Defer must NOT fire yet.
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		s := h.cb.Snapshot()
		if len(s.Scripts) >= 1 {
			t.Fatal("defer fired before drain")
		}
		time.Sleep(5 * time.Millisecond)
	}

	ch1 <- fakeResponse{body: "console.log('defer')"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	s := h.cb.Snapshot()
	if len(s.Scripts) != 1 {
		t.Fatalf("expected 1 deferred script at drain; got %d", len(s.Scripts))
	}
	if s.Scripts[0].Mode != ScriptModeDefer {
		t.Errorf("script mode = %v, want defer", s.Scripts[0].Mode)
	}
}

// TestModuleScriptStillSkipped — M5 keeps module scripts unsupported.
func TestModuleScriptStillSkipped(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	coord.HandleResource(Resource{
		Kind:       KindScript,
		URL:        "module.js",
		ScriptMode: ScriptModeModule,
	})

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	s := h.cb.Snapshot()

	if len(s.Scripts) != 0 {
		t.Errorf("module script should be skipped; got %d", len(s.Scripts))
	}
	if h.fetcher.fetchCountFor("https://example.com/module.js") != 0 {
		t.Errorf("module script should not be fetched")
	}
	var skip *SkippedError
	if !findSkip(s.Errors, &skip) || skip == nil {
		t.Errorf("expected SkippedError for module, got %v", s.Errors)
	}
}

// TestMixedModesOrder — classic + defer drain together in source order;
// async fires earlier. Verifies the M5 acceptance ordering.
func TestMixedModesOrder(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	chAsync := h.fetcher.register("https://example.com/async.js")
	chClassic := h.fetcher.register("https://example.com/classic.js")
	chDefer := h.fetcher.register("https://example.com/defer.js")

	coord.HandleResource(Resource{Kind: KindScript, URL: "classic.js"})
	coord.HandleResource(Resource{Kind: KindScript, URL: "async.js",
		ScriptMode: ScriptModeAsync})
	coord.HandleResource(Resource{Kind: KindScript, URL: "defer.js",
		ScriptMode: ScriptModeDefer})

	waitForFetch(t, h.fetcher, time.Second,
		"https://example.com/classic.js",
		"https://example.com/async.js",
		"https://example.com/defer.js")

	// Release async first — it should fire immediately.
	chAsync <- fakeResponse{body: "/* async */"}

	// Wait for async callback.
	deadline := time.Now().Add(time.Second)
	var s callbackSnapshot
	for time.Now().Before(deadline) {
		s = h.cb.Snapshot()
		if len(s.Scripts) >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if len(s.Scripts) != 1 || s.Scripts[0].Mode != ScriptModeAsync {
		t.Fatalf("expected 1 async callback before drain; got %d scripts", len(s.Scripts))
	}

	chClassic <- fakeResponse{body: "/* classic */"}
	chDefer <- fakeResponse{body: "/* defer */"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	s = h.cb.Snapshot()

	if len(s.Scripts) != 3 {
		t.Fatalf("expected 3 scripts total; got %d", len(s.Scripts))
	}
	wantOrder := []ScriptMode{ScriptModeAsync, ScriptModeClassic, ScriptModeDefer}
	for i, want := range wantOrder {
		if s.Scripts[i].Mode != want {
			t.Errorf("scripts[%d].Mode = %v, want %v", i, s.Scripts[i].Mode, want)
		}
	}
}

// TestLifecycleOrderAsyncAfterDrain — EventLoad fires after async
// scripts complete (not at drain time).
func TestLifecycleOrderAsyncAfterDrain(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	chAsync := h.fetcher.register("https://example.com/async.js")
	coord.HandleResource(Resource{
		Kind:       KindScript,
		URL:        "async.js",
		ScriptMode: ScriptModeAsync,
	})
	waitForFetch(t, h.fetcher, time.Second, "https://example.com/async.js")

	chAsync <- fakeResponse{body: "console.log('async')"}

	// Wait for async to fire.
	deadline := time.Now().Add(time.Second)
	var s callbackSnapshot
	for time.Now().Before(deadline) {
		s = h.cb.Snapshot()
		if len(s.Scripts) >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}

	s = h.cb.Snapshot()
	if len(s.Lifecycle) < 3 {
		t.Fatalf("expected at least 3 lifecycle events; got %d (%v)",
			len(s.Lifecycle), s.Lifecycle)
	}
	if s.Lifecycle[0] != EventDOMContentLoaded {
		t.Errorf("lifecycle[0] = %v, want DOMContentLoaded", s.Lifecycle[0])
	}
}

// TestLifecycleLoadWaitsForAsync — EventLoad does NOT fire until async
// scripts finish.
func TestLifecycleLoadWaitsForAsync(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	chAsync := h.fetcher.register("https://example.com/async.js")
	coord.HandleResource(Resource{
		Kind:       KindScript,
		URL:        "async.js",
		ScriptMode: ScriptModeAsync,
	})
	waitForFetch(t, h.fetcher, time.Second, "https://example.com/async.js")

	// Drain while async is still pending.
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}

	s := h.cb.Snapshot()
	if len(s.Lifecycle) != 1 {
		t.Fatalf("expected only 1 lifecycle event before async; got %d (%v)",
			len(s.Lifecycle), s.Lifecycle)
	}
	if s.Lifecycle[0] != EventDOMContentLoaded {
		t.Errorf("first lifecycle = %v, want DOMContentLoaded", s.Lifecycle[0])
	}

	// Release async.
	chAsync <- fakeResponse{body: "console.log('async')"}

	// Wait for Load + DocumentEnd.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s = h.cb.Snapshot()
		if len(s.Lifecycle) >= 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(s.Lifecycle) != 3 {
		t.Fatalf("expected 3 lifecycle events after async; got %d (%v)",
			len(s.Lifecycle), s.Lifecycle)
	}
	if s.Lifecycle[1] != EventLoad {
		t.Errorf("lifecycle[1] = %v, want Load", s.Lifecycle[1])
	}
	if s.Lifecycle[2] != EventDocumentEnd {
		t.Errorf("lifecycle[2] = %v, want DocumentEnd", s.Lifecycle[2])
	}
}

// TestAsyncCallbackThreadSafety — concurrent async callbacks + drain
// don't race. Race detector catches any issue.
func TestAsyncCallbackThreadSafety(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	const N = 8
	chs := make([]chan fakeResponse, N)
	for i := 0; i < N; i++ {
		chs[i] = h.fetcher.register("https://example.com/async-" + itoa(i) + ".js")
		coord.HandleResource(Resource{
			Kind:       KindScript,
			URL:        "async-" + itoa(i) + ".js",
			ScriptMode: ScriptModeAsync,
		})
	}

	for _, ch := range chs {
		ch <- fakeResponse{body: "/* done */"}
	}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Wait for async callbacks to land in scripts.
	deadline := time.Now().Add(2 * time.Second)
	var s callbackSnapshot
	for time.Now().Before(deadline) {
		s = h.cb.Snapshot()
		if len(s.Scripts) >= N {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if int32(len(s.Scripts)) != N {
		t.Errorf("expected %d async scripts; got %d", N, len(s.Scripts))
	}
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func findSkip(errs []error, target **SkippedError) bool {
	for _, e := range errs {
		var s *SkippedError
		if asSkipped(e, &s) {
			*target = s
			return true
		}
	}
	return false
}

func asSkipped(err error, target **SkippedError) bool {
	if err == nil {
		return false
	}
	if s, ok := err.(*SkippedError); ok {
		*target = s
		return true
	}
	return false
}

var _ atomic.Int32
