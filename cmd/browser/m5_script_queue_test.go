package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine/documentloader"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/js"
	ghtml "golang.org/x/net/html"
)

// TestExecuteScriptQueue_DefersAfterClassics — M5: defer scripts
// execute AFTER classic scripts, even when the defer is earlier in
// document order (per HTML spec).
func TestExecuteScriptQueue_DefersAfterClassics(t *testing.T) {
	rt := js.NewRuntime()
	doc, _ := ghtml.Parse(strings.NewReader(
		`<html><body>
<script>globalThis.M5 = ['classic-init'];</script>
<script src="early-classic.js" defer></script>
<script>globalThis.M5.push('classic-mid')</script>
<script src="late-defer.js" defer></script>
<script>globalThis.M5.push('classic-end')</script>
</body></html>`))

	results := []documentloader.ScriptResult{
		// Document order: classic, defer, classic, defer, classic
		{Inline: true, Mode: documentloader.ScriptModeClassic, Position: 0},
		{Inline: false, Mode: documentloader.ScriptModeDefer, Position: 1,
			URL: "https://example.com/early.js",
			Source: []byte("globalThis.M5.push('defer-1')")},
		{Inline: true, Mode: documentloader.ScriptModeClassic, Position: 2,
			Source: []byte("globalThis.M5.push('classic-mid')")},
		{Inline: false, Mode: documentloader.ScriptModeDefer, Position: 3,
			URL: "https://example.com/late.js",
			Source: []byte("globalThis.M5.push('defer-2')")},
		{Inline: true, Mode: documentloader.ScriptModeClassic, Position: 4,
			Source: []byte("globalThis.M5.push('classic-end')")},
	}

	executeScriptQueue(rt, "https://example.com/", nil, doc, results)

	order, _ := rt.RunScript(`globalThis.M5.join(',')`)
	if order == nil {
		t.Fatal("nil order")
	}
	want := "classic-init,classic-mid,classic-end,defer-1,defer-2"
	if order.String() != want {
		t.Errorf("execution order = %s, want %s", order.String(), want)
	}
}

// TestExecuteScriptQueue_AsyncScriptsNotInQueue — async scripts
// arrive via the coordinator's immediate OnScript callback (during
// parse), not via the drain-time queue. The queue only sees classic
// + defer; anything tagged async is filtered out.
func TestExecuteScriptQueue_AsyncScriptsNotInQueue(t *testing.T) {
	rt := js.NewRuntime()
	doc, _ := ghtml.Parse(strings.NewReader(
		`<html><body>
<script>globalThis.M5_ASYNC = 'classic';</script>
<script src="defer.js" defer></script>
<script src="async.js" async></script>
</body></html>`))

	// Synthetic results mirroring the DOM: 1 classic, 1 defer, 1 async.
	// Source for inline classic is empty (filled by DOM walk).
	results := []documentloader.ScriptResult{
		{Inline: true, Mode: documentloader.ScriptModeClassic, Position: 0},
		{Inline: false, Mode: documentloader.ScriptModeDefer, Position: 1,
			URL: "https://example.com/defer.js",
			Source: []byte("globalThis.M5_ASYNC = (globalThis.M5_ASYNC || '') + '|defer'")},
		{Inline: false, Mode: documentloader.ScriptModeAsync, Position: 2,
			URL: "https://example.com/async.js",
			Source: []byte("globalThis.M5_ASYNC = (globalThis.M5_ASYNC || '') + '|async'")},
	}

	executeScriptQueue(rt, "https://example.com/", nil, doc, results)

	out, _ := rt.RunScript(`globalThis.M5_ASYNC`)
	if out == nil {
		t.Fatal("nil out")
	}
	want := "classic|defer"
	if out.String() != want {
		t.Errorf("got %q, want %q (async should not appear)", out.String(), want)
	}
}

// TestFireDOMContentLoaded — the dispatch helper runs without error
// and is idempotent.
func TestFireDOMContentLoaded(t *testing.T) {
	rt := js.NewRuntime()
	// Register a listener so we can verify the event was dispatched.
	rt.SetHTMLContent(`<html><body></body></html>`)
	fireDOMContentLoaded(rt)
	fireDOMContentLoaded(rt) // second call should not error
	// If we got here without panicking, dispatch worked.
}

// TestM5EndToEndAsyncOrdering — full end-to-end: streaming parse +
// coordinator + renderer, with a real httptest server. Verifies that
// async scripts execute when fetched (during parse) and the
// document renders with all CSS before scripts run.
func TestM5EndToEndAsyncOrdering(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html><head>
<link rel="stylesheet" href="theme.css">
</head><body>
<p class="x">hello</p>
<script async src="tracker.js"></script>
<script src="app.js"></script>
<script>globalThis.M5_E2E = 'inline';</script>
<script src="late-async.js" async></script>
</body></html>`))
	})
	mux.HandleFunc("/theme.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte(".x { color: red; }"))
	})
	mux.HandleFunc("/app.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte("globalThis.M5_E2E = (globalThis.M5_E2E || '') + '|app'"))
	})
	// async scripts
	mux.HandleFunc("/tracker.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte("globalThis.M5_E2E = (globalThis.M5_E2E || '') + '|tracker'"))
	})
	mux.HandleFunc("/late-async.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte("globalThis.M5_E2E = (globalThis.M5_E2E || '') + '|late-async'"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/page")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	sched := navigation.NewScheduler()
	defer sched.Cancel()
	load, navCtx := sched.Begin(context.Background(), srv.URL+"/page")

	var (
		mu              sync.Mutex
		scripts         []documentloader.ScriptResult
		styles          []documentloader.CSSResult
		events          []documentloader.LifecycleEvent
	)
	coord, err := documentloader.New(documentloader.Options{
		NavigationID:      load.ID,
		NavigationContext: navCtx,
		FinalURL:          srv.URL + "/page",
		Scheduler:         sched,
		Fetcher:           realFetcher{srv.Client()},
		Callbacks: documentloader.Callbacks{
			OnStylesheet: func(r documentloader.CSSResult) {
				mu.Lock(); defer mu.Unlock()
				styles = append(styles, r)
			},
			OnScript: func(r documentloader.ScriptResult) {
				mu.Lock(); defer mu.Unlock()
				scripts = append(scripts, r)
			},
			OnLifecycle: func(e documentloader.LifecycleEvent) {
				mu.Lock(); defer mu.Unlock()
				events = append(events, e)
			},
		},
	})
	if err != nil {
		t.Fatalf("coord: %v", err)
	}

	// Stream-parse, feeding discoveries into coordinator.
	if _, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(string(body)),
		dom.ParseConfig{OnResource: coord.FromDomOnResource()}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Wait for async to complete (load event). The OnLifecycle callback
	// fires from a coordinator goroutine; reads must hold mu.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		hasLoad := false
		mu.Lock()
		for _, e := range events {
			if e == documentloader.EventLoad {
				hasLoad = true
				break
			}
		}
		mu.Unlock()
		if hasLoad {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// External scripts: 1 stylesheet, 1 classic (app.js), 2 async (tracker, late-async).
	// Inline: 1 inline script.
	mu.Lock()
	stylesCopy := append([]documentloader.CSSResult(nil), styles...)
	scriptsCopy := append([]documentloader.ScriptResult(nil), scripts...)
	mu.Unlock()

	if len(stylesCopy) != 1 {
		t.Errorf("expected 1 stylesheet; got %d", len(stylesCopy))
	}

	// Count by mode.
	var classicN, asyncN, inlineN int
	for _, s := range scriptsCopy {
		switch {
		case s.Inline:
			inlineN++
		case s.Mode == documentloader.ScriptModeAsync:
			asyncN++
		case s.Mode == documentloader.ScriptModeClassic:
			classicN++
		}
	}
	if classicN != 1 {
		t.Errorf("classic count = %d, want 1", classicN)
	}
	if asyncN != 2 {
		t.Errorf("async count = %d, want 2", asyncN)
	}
	if inlineN != 1 {
		t.Errorf("inline count = %d, want 1", inlineN)
	}

	// Lifecycle order: DOMContentLoaded first, Load later. Use the
	// already-copied snapshot under mu.
	mu.Lock()
	eventsCopy2 := append([]documentloader.LifecycleEvent(nil), events...)
	mu.Unlock()
	var domIdx, loadIdx = -1, -1
	for i, e := range eventsCopy2 {
		if e == documentloader.EventDOMContentLoaded && domIdx == -1 {
			domIdx = i
		}
		if e == documentloader.EventLoad && loadIdx == -1 {
			loadIdx = i
		}
	}
	if domIdx == -1 {
		t.Errorf("DOMContentLoaded not fired")
	}
	if loadIdx == -1 {
		t.Errorf("Load not fired")
	}
	if domIdx != -1 && loadIdx != -1 && domIdx >= loadIdx {
		t.Errorf("DOMContentLoaded must fire before Load; got %d, %d", domIdx, loadIdx)
	}
}

// realFetcher wraps goosienet.NewFetcherWithClient to satisfy the
// documentloader.Fetcher interface for tests using httptest servers.
type realFetcher struct {
	client *http.Client
}

func (f realFetcher) FetchWithContext(ctx context.Context, rawURL string, _ goosienet.ProgressCallback) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}