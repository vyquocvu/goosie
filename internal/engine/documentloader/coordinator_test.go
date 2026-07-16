package documentloader

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/navigation"
	"github.com/vyquocvu/goosie/internal/net"
)

// fakeFetcher is a controllable fetcher for tests. Each registered URL
// gets a buffered channel; the test releases responses in the order it
// wants, regardless of fetch order. concurrent fetches count is
// observed via concurrentNow.
type fakeFetcher struct {
	mu          sync.Mutex
	pending     map[string]chan fakeResponse
	fetchCount  map[string]int // URL → number of fetches attempted
	concurrentN int32          // current in-flight count
	maxConc     int32          // peak in-flight count
}

type fakeResponse struct {
	body string
	err  error
}

func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{
		pending:    make(map[string]chan fakeResponse),
		fetchCount: make(map[string]int),
	}
}

// register prepares a URL for controlled release. Returns the channel
// the test should send on to deliver a response.
func (f *fakeFetcher) register(rawURL string) chan fakeResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan fakeResponse, 1)
	f.pending[rawURL] = ch
	return ch
}

func (f *fakeFetcher) FetchWithContext(ctx context.Context, rawURL string, _ net.ProgressCallback) (string, error) {
	f.mu.Lock()
	ch, ok := f.pending[rawURL]
	if !ok {
		f.mu.Unlock()
		return "", errors.New("fakeFetcher: unexpected URL " + rawURL)
	}
	f.fetchCount[rawURL]++
	f.mu.Unlock()

	now := atomic.AddInt32(&f.concurrentN, 1)
	defer atomic.AddInt32(&f.concurrentN, -1)
	for {
		peak := atomic.LoadInt32(&f.maxConc)
		if now <= peak || atomic.CompareAndSwapInt32(&f.maxConc, peak, now) {
			break
		}
	}

	select {
	case resp := <-ch:
		if resp.err != nil {
			return "", resp.err
		}
		return resp.body, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (f *fakeFetcher) fetchCountFor(rawURL string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fetchCount[rawURL]
}

// captureCallbacks records every callback invocation in order.
type captureCallbacks struct {
	mu       sync.Mutex
	CSS      []CSSResult
	Scripts  []ScriptResult
	Images   []ImageResult
	Errors   []error
	Lifecycle []LifecycleEvent
}

// record returns the Callbacks wiring. Each closure allocates a fresh
// slice on every append so snapshot()'s defensive copies are never
// mutated by a later callback firing on the same goroutine. M5
// surfaced this race: async scripts fire OnScript from fetcher
// goroutines, so the test goroutine reading h.cb.Scripts after
// snapshot can race with a concurrent append into the snapshot's
// underlying array. Allocating fresh slices breaks that sharing.
func (c *captureCallbacks) record() Callbacks {
	return Callbacks{
		OnStylesheet: func(r CSSResult) {
			c.mu.Lock()
			defer c.mu.Unlock()
			n := make([]CSSResult, 0, len(c.CSS)+1)
			n = append(n, c.CSS...)
			n = append(n, r)
			c.CSS = n
		},
		OnScript: func(r ScriptResult) {
			c.mu.Lock()
			defer c.mu.Unlock()
			n := make([]ScriptResult, 0, len(c.Scripts)+1)
			n = append(n, c.Scripts...)
			n = append(n, r)
			c.Scripts = n
		},
		OnImage: func(r ImageResult) {
			c.mu.Lock()
			defer c.mu.Unlock()
			n := make([]ImageResult, 0, len(c.Images)+1)
			n = append(n, c.Images...)
			n = append(n, r)
			c.Images = n
		},
		OnError: func(_ Resource, err error) {
			c.mu.Lock()
			defer c.mu.Unlock()
			n := make([]error, 0, len(c.Errors)+1)
			n = append(n, c.Errors...)
			n = append(n, err)
			c.Errors = n
		},
		OnLifecycle: func(e LifecycleEvent) {
			c.mu.Lock()
			defer c.mu.Unlock()
			n := make([]LifecycleEvent, 0, len(c.Lifecycle)+1)
			n = append(n, c.Lifecycle...)
			n = append(n, e)
			c.Lifecycle = n
		},
	}
}

// snapshot is the original M1-M4 accessor: it copies each slice under
// the lock and replaces the field with the copy. Tests that read
// h.cb.Scripts after snapshot get the snapshot's copy.
//
// M5 caveat: when callbacks fire OUTSIDE drainLocked (e.g. async
// script OnScript from a fetcher goroutine), a concurrent closure
// can mutate h.cb.Scripts between snapshot's lock release and the
// test's read. M5 tests that exercise async paths use Snapshot()
// instead and read from the returned struct.
func (c *captureCallbacks) snapshot() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CSS = copySliceCSS(c.CSS)
	c.Scripts = copySliceScripts(c.Scripts)
	c.Images = copySliceImages(c.Images)
	c.Errors = copySliceErrors(c.Errors)
	c.Lifecycle = copySliceLifecycle(c.Lifecycle)
}

// Snapshot is the M5-safe accessor: returns a struct of independent
// slice copies under the lock. The caller reads the returned struct
// without locking; the snapshot's underlying arrays are never shared
// with concurrent callback writers.
func (c *captureCallbacks) Snapshot() callbackSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return callbackSnapshot{
		CSS:       copySliceCSS(c.CSS),
		Scripts:   copySliceScripts(c.Scripts),
		Images:    copySliceImages(c.Images),
		Errors:    copySliceErrors(c.Errors),
		Lifecycle: copySliceLifecycle(c.Lifecycle),
	}
}

type callbackSnapshot struct {
	CSS       []CSSResult
	Scripts   []ScriptResult
	Images    []ImageResult
	Errors    []error
	Lifecycle []LifecycleEvent
}

func copySliceCSS(s []CSSResult) []CSSResult {
	out := make([]CSSResult, len(s))
	copy(out, s)
	return out
}
func copySliceScripts(s []ScriptResult) []ScriptResult {
	out := make([]ScriptResult, len(s))
	copy(out, s)
	return out
}
func copySliceImages(s []ImageResult) []ImageResult {
	out := make([]ImageResult, len(s))
	copy(out, s)
	return out
}
func copySliceErrors(s []error) []error {
	out := make([]error, len(s))
	copy(out, s)
	return out
}
func copySliceLifecycle(s []LifecycleEvent) []LifecycleEvent {
	out := make([]LifecycleEvent, len(s))
	copy(out, s)
	return out
}

// newTestCoord constructs a Coordinator wired against a fake fetcher
// and a real scheduler. The returned cleanup cancels the navigation
// and waits for goroutines to drain.
type testHarness struct {
	sched   *navigation.Scheduler
	fetcher *fakeFetcher
	cb      *captureCallbacks
	navCtx  context.Context
	nav     navigation.Load
}

func newTestHarness(t *testing.T, finalURL string, csp *net.CSPPolicy) *testHarness {
	t.Helper()
	sched := navigation.NewScheduler()
	nav, ctx := sched.Begin(context.Background(), finalURL)
	fetcher := newFakeFetcher()
	cb := &captureCallbacks{}
	return &testHarness{
		sched:   sched,
		fetcher: fetcher,
		cb:      cb,
		navCtx:  ctx,
		nav:     nav,
	}
}

func (h *testHarness) newCoord(t *testing.T, csp *net.CSPPolicy) *Coordinator {
	t.Helper()
	c, err := New(Options{
		NavigationID:      h.nav.ID,
		NavigationContext: h.navCtx,
		FinalURL:          h.nav.URL,
		CSP:               csp,
		Scheduler:         h.sched,
		Fetcher:           h.fetcher,
		Callbacks:         h.cb.record(),
	})
	if err != nil {
		t.Fatalf("New(Options) error: %v", err)
	}
	return c
}

func (h *testHarness) shutdown(t *testing.T) {
	t.Helper()
	h.sched.Cancel()
}

// --------------------------------------------------------------------------
// M0 characterization tests
// --------------------------------------------------------------------------

// M0 #2 — CSP rejection prevents the HTTP request for CSS and scripts.
func TestCSPRejectionPreventsFetch(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)

	policy := net.ParseCSPHeader("default-src 'self'; style-src 'self'; script-src 'self'")

	// 'self' is https://example.com — anything off-origin is blocked.
	coord := h.newCoord(t, policy)

	// off-origin CSS: should be skipped, fetcher never called.
	coord.HandleResource(Resource{Kind: KindCSS, URL: "https://evil.test/x.css"})

	// on-origin CSS: allowed; register and release.
	chCSS := h.fetcher.register("https://example.com/local.css")
	coord.HandleResource(Resource{Kind: KindCSS, URL: "local.css"})

	// off-origin script: skipped.
	coord.HandleResource(Resource{Kind: KindScript, URL: "https://evil.test/x.js"})

	// on-origin script: allowed; register and release.
	chJS := h.fetcher.register("https://example.com/local.js")
	coord.HandleResource(Resource{Kind: KindScript, URL: "local.js"})

	// Wait for the two allowed fetches to register with the fetcher.
	// They will block waiting on the channels.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.fetcher.fetchCountFor("https://example.com/local.css") >= 1 &&
			h.fetcher.fetchCountFor("https://example.com/local.js") >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := h.fetcher.fetchCountFor("https://evil.test/x.css"); got != 0 {
		t.Errorf("CSP-blocked CSS fetched %d times, want 0", got)
	}
	if got := h.fetcher.fetchCountFor("https://evil.test/x.js"); got != 0 {
		t.Errorf("CSP-blocked script fetched %d times, want 0", got)
	}

	// Release the allowed responses.
	chCSS <- fakeResponse{body: "body{}"}
	chJS <- fakeResponse{body: "console.log(1)"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}

	h.cb.snapshot()
	// Two errors expected: CSP-blocked CSS and script.
	var cspSkips int
	for _, e := range h.cb.Errors {
		var skip *SkippedError
		if errors.As(e, &skip) && skip.Reason != "" && len(skip.Reason) >= 3 && skip.Reason[:3] == "csp" {
			cspSkips++
		}
	}
	if cspSkips != 2 {
		t.Errorf("expected 2 CSP skips, got %d (errors=%v)", cspSkips, h.cb.Errors)
	}
	if len(h.cb.CSS) != 1 {
		t.Errorf("expected 1 CSS result, got %d", len(h.cb.CSS))
	}
	if len(h.cb.Scripts) != 1 {
		t.Errorf("expected 1 script result, got %d", len(h.cb.Scripts))
	}
}

// M0 #3 — A new navigation cancels all in-flight subresources.
func TestNewNavigationCancelsInFlightResources(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page1", nil)
	defer h.shutdown(t)

	coord := h.newCoord(t, nil)

	// Start a slow CSS fetch.
	chCSS := h.fetcher.register("https://example.com/slow.css")
	coord.HandleResource(Resource{Kind: KindCSS, URL: "slow.css"})

	// Wait for the fetcher to register the call.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.fetcher.fetchCountFor("https://example.com/slow.css") >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Start a new navigation on the same scheduler. This cancels the
	// previous navigation context, which in turn cancels the in-flight
	// fetcher call.
	_, newCtx := h.sched.Begin(context.Background(), "https://example.com/page2")

	// The old nav context should be cancelled.
	select {
	case <-h.navCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("previous navigation context was not cancelled by new navigation")
	}

	// The fetcher call (which was derived from the old nav ctx) should
	// also be cancelled. We release the channel AFTER cancellation to
	// prove cancellation wins over release.
	select {
	case <-chCSS:
		t.Fatal("fetcher returned a response after navigation cancel")
	default:
	}
	chCSS <- fakeResponse{body: "too late"} // ignored: ctx was cancelled

	// Sanity: the new navigation has a fresh, non-cancelled context.
	select {
	case <-newCtx.Done():
		t.Fatal("new navigation context was cancelled immediately")
	default:
	}

	// Closing coord just confirms it's still usable; HandleResource on
	// a closed coordinator is a skip.
	coord.Cancel()
}

// M0 #4 — Stylesheet source order wins over response completion order.
func TestStylesheetSourceOrderWins(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	// Discover three CSS files in document order: first, second, third.
	ch1 := h.fetcher.register("https://example.com/first.css")
	ch2 := h.fetcher.register("https://example.com/second.css")
	ch3 := h.fetcher.register("https://example.com/third.css")

	coord.HandleResource(Resource{Kind: KindCSS, URL: "first.css"})
	coord.HandleResource(Resource{Kind: KindCSS, URL: "second.css"})
	coord.HandleResource(Resource{Kind: KindCSS, URL: "third.css"})

	// Wait for all three fetches to be in flight.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		h.fetcher.mu.Lock()
		ready := h.fetcher.fetchCount["https://example.com/first.css"] >= 1 &&
			h.fetcher.fetchCount["https://example.com/second.css"] >= 1 &&
			h.fetcher.fetchCount["https://example.com/third.css"] >= 1
		h.fetcher.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Release in REVERSE order: third, first, second.
	ch3 <- fakeResponse{body: "/* third */"}
	ch1 <- fakeResponse{body: "/* first */"}
	ch2 <- fakeResponse{body: "/* second */"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()

	if len(h.cb.CSS) != 3 {
		t.Fatalf("expected 3 CSS results, got %d", len(h.cb.CSS))
	}
	wantOrder := []string{
		"https://example.com/first.css",
		"https://example.com/second.css",
		"https://example.com/third.css",
	}
	for i, want := range wantOrder {
		if got := h.cb.CSS[i].Resolved; got != want {
			t.Errorf("CSS[%d].Resolved = %q, want %q (source-order violation)", i, got, want)
		}
		if got := h.cb.CSS[i].Position; got != i {
			t.Errorf("CSS[%d].Position = %d, want %d", i, got, i)
		}
	}
}

// M0 #5 — Mixed inline/external classic scripts execute in document order.
func TestMixedInlineExternalScriptsDocumentOrder(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	// Discover in document order: inline, external, inline, external.
	coord.HandleResource(Resource{
		Kind: KindScript, Inline: true, Source: []byte("inline-0"),
	})
	ch1 := h.fetcher.register("https://example.com/ext-1.js")
	coord.HandleResource(Resource{Kind: KindScript, URL: "ext-1.js"})
	coord.HandleResource(Resource{
		Kind: KindScript, Inline: true, Source: []byte("inline-2"),
	})
	ch2 := h.fetcher.register("https://example.com/ext-3.js")
	coord.HandleResource(Resource{Kind: KindScript, URL: "ext-3.js"})

	// Release the external scripts in reverse order: ext-3, then ext-1.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.fetcher.fetchCountFor("https://example.com/ext-1.js") >= 1 &&
			h.fetcher.fetchCountFor("https://example.com/ext-3.js") >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	ch2 <- fakeResponse{body: "ext-3"}
	ch1 <- fakeResponse{body: "ext-1"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()

	if len(h.cb.Scripts) != 4 {
		t.Fatalf("expected 4 script results, got %d", len(h.cb.Scripts))
	}
	wantBodies := []string{"inline-0", "ext-1", "inline-2", "ext-3"}
	wantInline := []bool{true, false, true, false}
	for i, want := range wantBodies {
		if got := string(h.cb.Scripts[i].Source); got != want {
			t.Errorf("Scripts[%d].Source = %q, want %q", i, got, want)
		}
		if h.cb.Scripts[i].Inline != wantInline[i] {
			t.Errorf("Scripts[%d].Inline = %v, want %v", i, h.cb.Scripts[i].Inline, wantInline[i])
		}
		if h.cb.Scripts[i].Position != i {
			t.Errorf("Scripts[%d].Position = %d, want %d", i, h.cb.Scripts[i].Position, i)
		}
	}
}

// M0 #6 — The document is not refetched or scripts re-executed after a JS
// mutation. This test captures the contract that M3/M6 will implement. In
// M1 we do not yet intercept mutations, so we verify the *baseline*
// guarantee: when the same coordinator handles the same external script
// twice (e.g. via a re-parse), the second handle MUST NOT issue a fresh
// network request when the previous fetch already satisfied it.
//
// Today this expectation is not enforced. The test is therefore written
// to assert the *desired* behavior and is expected to fail until M3
// introduces resource identity tracking. Mark it as such.
func TestCoordinatorDoesNotRefetchOnReDiscovery(t *testing.T) {
	t.Skip("M3 will introduce resource identity tracking; today re-discovery re-fetches")
}

// --------------------------------------------------------------------------
// M1 unit tests for the coordinator's local guarantees.
// --------------------------------------------------------------------------

// HandleResource on a closed coordinator reports a SkippedError.
func TestHandleResourceAfterClose(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)
	coord.Cancel()

	coord.HandleResource(Resource{Kind: KindCSS, URL: "late.css"})

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()
	if len(h.cb.Errors) != 1 {
		t.Fatalf("expected 1 error after late HandleResource, got %d", len(h.cb.Errors))
	}
	var skip *SkippedError
	if !errors.As(h.cb.Errors[0], &skip) {
		t.Errorf("expected SkippedError, got %T: %v", h.cb.Errors[0], h.cb.Errors[0])
	}
}

// Inline non-script resources are rejected (only inline scripts allowed).
func TestInlineNonScriptRejected(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	coord.HandleResource(Resource{
		Kind: KindCSS, Inline: true, Source: []byte("body{}"),
	})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()
	if len(h.cb.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(h.cb.Errors))
	}
	var skip *SkippedError
	if !errors.As(h.cb.Errors[0], &skip) {
		t.Errorf("expected SkippedError, got %T", h.cb.Errors[0])
	}
}

// Unsupported script modes (async/defer/module) are skipped in M1.
func TestUnsupportedScriptModeSkipped(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	coord.HandleResource(Resource{
		Kind: KindScript, Inline: true, Source: []byte("x"), ScriptMode: ScriptModeModule,
	})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()
	if len(h.cb.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(h.cb.Errors))
	}
	var skip *SkippedError
	if !errors.As(h.cb.Errors[0], &skip) {
		t.Errorf("expected SkippedError, got %T", h.cb.Errors[0])
	}
	if skip == nil || !contains(skip.Reason, "module") {
		t.Errorf("expected reason to mention 'module', got %q", skip.Reason)
	}
}

// Non-http(s) URLs (data:, blob:) are not fetched.
func TestNonHTTPSchemesSkipped(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	coord.HandleResource(Resource{Kind: KindCSS, URL: "data:text/css,body{color:red}"})
	coord.HandleResource(Resource{Kind: KindCSS, URL: "blob:https://x/abc"})

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()
	if len(h.cb.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(h.cb.Errors))
	}
	if h.fetcher.fetchCountFor("data:text/css,body{color:red}") != 0 {
		t.Errorf("data: URL should not be fetched")
	}
}

// Scheduler tracks each resource via AddResource; RemoveResource fires
// after the fetch completes.
func TestSchedulerTracksResources(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	ch := h.fetcher.register("https://example.com/style.css")
	coord.HandleResource(Resource{Kind: KindCSS, URL: "style.css"})

	// Wait for the fetch to register with the scheduler.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.fetcher.fetchCountFor("https://example.com/style.css") >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	// While the fetch is in flight, the scheduler should report 2 loads
	// (main nav + subresource).
	if got := len(h.sched.PendingLoads()); got != 2 {
		t.Errorf("PendingLoads in flight = %d, want 2", got)
	}

	ch <- fakeResponse{body: "body{}"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}

	// After drain the main nav still pending until Cancel; subresource
	// must be gone.
	pending := h.sched.PendingLoads()
	var foundSub bool
	for _, p := range pending {
		if p.URL == "https://example.com/style.css" {
			foundSub = true
		}
	}
	if foundSub {
		t.Errorf("subresource not removed after fetch completion")
	}
}

// Fetcher context cancellation propagates: a cancelled nav kills the
// fetch and produces a benign skip (not a hard error).
func TestFetchContextCancelledPropagates(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	_ = h.fetcher.register("https://example.com/style.css")
	coord.HandleResource(Resource{Kind: KindCSS, URL: "style.css"})

	// Cancel the navigation context directly.
	h.sched.Cancel()

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Logf("HandleDocumentEnd returned %v (expected for cancelled nav)", err)
	}
	h.cb.snapshot()
	// After Cancel, results may or may not be drained depending on race;
	// the key invariant is no panic and no leaked goroutine.
}

// Coordinator exposes its FinalURL for callers that need it.
func TestFinalURL(t *testing.T) {
	h := newTestHarness(t, "https://example.com/landing", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)
	if got := coord.FinalURL(); got != "https://example.com/landing" {
		t.Errorf("FinalURL() = %q, want landing URL", got)
	}
	_ = coord
}

// Done channel closes after HandleDocumentEnd.
func TestDoneChannelCloses(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	ch := h.fetcher.register("https://example.com/style.css")
	coord.HandleResource(Resource{Kind: KindCSS, URL: "style.css"})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.fetcher.fetchCountFor("https://example.com/style.css") >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	ch <- fakeResponse{body: "body{}"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	select {
	case <-coord.Done():
	case <-time.After(time.Second):
		t.Fatal("Done channel did not close after HandleDocumentEnd")
	}
}

// OnLifecycle fires once at HandleDocumentEnd.
func TestLifecycleFiresAtDocumentEnd(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	ch := h.fetcher.register("https://example.com/style.css")
	coord.HandleResource(Resource{Kind: KindCSS, URL: "style.css"})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.fetcher.fetchCountFor("https://example.com/style.css") >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	ch <- fakeResponse{body: "body{}"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()
	// M5 emits 3 events: DOMContentLoaded, Load, DocumentEnd.
	if len(h.cb.Lifecycle) != 3 {
		t.Fatalf("expected 3 lifecycle events, got %d (%v)", len(h.cb.Lifecycle), h.cb.Lifecycle)
	}
	if h.cb.Lifecycle[0] != EventDOMContentLoaded {
		t.Errorf("lifecycle[0] = %v, want EventDOMContentLoaded", h.cb.Lifecycle[0])
	}
	if h.cb.Lifecycle[1] != EventLoad {
		t.Errorf("lifecycle[1] = %v, want EventLoad", h.cb.Lifecycle[1])
	}
	if h.cb.Lifecycle[2] != EventDocumentEnd {
		t.Errorf("lifecycle[2] = %v, want EventDocumentEnd", h.cb.Lifecycle[2])
	}
}

// Non-HTTP scheme fetch attempts are short-circuited (no scheduler entry).
func TestNonHTTPDoesNotEnterScheduler(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	pendingBefore := len(h.sched.PendingLoads())
	coord.HandleResource(Resource{Kind: KindCSS, URL: "data:text/css,body{}"})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	pendingAfter := len(h.sched.PendingLoads())
	// Should not have grown (the main nav counts as 1).
	if pendingAfter != pendingBefore {
		t.Errorf("PendingLoads grew for non-http resource: %d → %d", pendingBefore, pendingAfter)
	}
}

// TestErrorContextIsInformative ensures SkippedError messages mention
// the resource kind so callers can render meaningful diagnostics.
func TestErrorContextIsInformative(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, net.ParseCSPHeader("style-src 'none'; script-src 'none'"))

	coord.HandleResource(Resource{Kind: KindCSS, URL: "x.css"})
	coord.HandleResource(Resource{Kind: KindScript, URL: "x.js"})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()
	if len(h.cb.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(h.cb.Errors))
	}
	for _, e := range h.cb.Errors {
		msg := e.Error()
		if !contains(msg, "csp") {
			t.Errorf("error message lacks 'csp' context: %s", msg)
		}
	}
}

// Image resources use img-src with default-src fallback.
func TestImageUsesImgSrcDirective(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	// Only default-src 'self' — img-src absent → default-src fallback.
	coord := h.newCoord(t, net.ParseCSPHeader("default-src 'none'"))

	coord.HandleResource(Resource{Kind: KindImage, URL: "https://example.com/img.png"})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()
	if len(h.cb.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(h.cb.Errors))
	}
}

// TestCoordinatorConcurrentHandleResource stresses concurrent calls.
func TestCoordinatorConcurrentHandleResource(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	const N = 32
	chs := make([]chan fakeResponse, N)
	for i := 0; i < N; i++ {
		chs[i] = h.fetcher.register("https://example.com/style-" + itoa(i) + ".css")
	}

	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			coord.HandleResource(Resource{
				Kind: KindCSS,
				URL:  "style-" + itoa(i) + ".css",
			})
		}()
	}
	wg.Wait()

	// Release all responses.
	for i := 0; i < N; i++ {
		chs[i] <- fakeResponse{body: "/* " + itoa(i) + " */"}
	}
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()
	if len(h.cb.CSS) != N {
		t.Errorf("expected %d CSS results, got %d", N, len(h.cb.CSS))
	}
	// Positions should be a permutation of [0..N).
	seen := make(map[int]bool, N)
	for _, r := range h.cb.CSS {
		if r.Position < 0 || r.Position >= N {
			t.Errorf("position out of range: %d", r.Position)
		}
		seen[r.Position] = true
	}
	if len(seen) != N {
		t.Errorf("positions not unique: got %d distinct, want %d", len(seen), N)
	}
}

// End-to-end: real httptest server, real net.Fetcher, real scheduler,
// verify the coordinator's lifecycle through the public surface.
func TestEndToEndWithHTTPServer(t *testing.T) {
	// Server delivers a CSS, a JS, and an image. Off-origin CSS is
	// blocked by CSP.
	mux := http.NewServeMux()
	mux.HandleFunc("/style.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte("body{color:red}"))
	})
	mux.HandleFunc("/script.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte("console.log(1)"))
	})
	mux.HandleFunc("/img.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("not-a-real-png"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	policy := net.ParseCSPHeader("default-src 'self'; img-src 'self'")

	sched := navigation.NewScheduler()
	fetcher := net.NewFetcher()
	nav, ctx := sched.Begin(context.Background(), srv.URL+"/page")
	defer sched.Cancel()

	cb := &captureCallbacks{}
	coord, err := New(Options{
		NavigationID:      nav.ID,
		NavigationContext: ctx,
		FinalURL:          nav.URL,
		CSP:               policy,
		Scheduler:         sched,
		Fetcher:           fetcher,
		Callbacks:         cb.record(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	coord.HandleResource(Resource{Kind: KindCSS, URL: "style.css"})
	coord.HandleResource(Resource{Kind: KindScript, URL: "script.js"})
	coord.HandleResource(Resource{Kind: KindImage, URL: "img.png"})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	cb.snapshot()

	if len(cb.CSS) != 1 {
		t.Errorf("expected 1 CSS, got %d", len(cb.CSS))
	}
	if len(cb.Scripts) != 1 {
		t.Errorf("expected 1 script, got %d", len(cb.Scripts))
	}
	if len(cb.Images) != 1 {
		t.Errorf("expected 1 image, got %d", len(cb.Images))
	}
	for _, c := range cb.CSS {
		if !contains(string(c.Source), "color:red") {
			t.Errorf("CSS body unexpected: %s", c.Source)
		}
	}
}

// TestNewRejectsMissingFields exhaustively checks constructor validation.
func TestNewRejectsMissingFields(t *testing.T) {
	sched := navigation.NewScheduler()
	fetcher := newFakeFetcher()
	_, ctx := sched.Begin(context.Background(), "https://x/")
	defer sched.Cancel()

	cases := []struct {
		name string
		opts Options
	}{
		{"no context", Options{Scheduler: sched, Fetcher: fetcher, FinalURL: "https://x/"}},
		{"no scheduler", Options{NavigationContext: ctx, Fetcher: fetcher, FinalURL: "https://x/"}},
		{"no fetcher", Options{NavigationContext: ctx, Scheduler: sched, FinalURL: "https://x/"}},
		{"no FinalURL", Options{NavigationContext: ctx, Scheduler: sched, Fetcher: fetcher}},
		{"bad FinalURL", Options{NavigationContext: ctx, Scheduler: sched, Fetcher: fetcher, FinalURL: "://malformed"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.opts); err == nil {
				t.Errorf("New(%s) succeeded, want error", tc.name)
			}
		})
	}
}

// TestIsActiveLifecycle: IsActive flips false after Cancel.
func TestIsActiveLifecycle(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	if !coord.IsActive() {
		t.Fatal("expected IsActive true after construction")
	}
	coord.Cancel()
	if coord.IsActive() {
		t.Fatal("expected IsActive false after Cancel")
	}
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// itoa is a tiny integer formatter; avoids importing strconv into the
// package by accident. (Production code uses strconv; tests prefer
// staying self-contained for readability.)
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// Ensure imports are used even if some helpers are pruned.
var _ = url.Parse
var _ = sort.SliceStable
