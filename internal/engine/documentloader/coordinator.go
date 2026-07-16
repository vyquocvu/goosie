package documentloader

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	"github.com/vyquocvu/goosie/internal/net"
)

// Fetcher is the subset of *net.Fetcher used by the coordinator. It is
// declared here so tests can supply an httptest-backed implementation
// without depending on the concrete net package.
//
// The coordinator never calls the fetcher outside the resource context
// returned by navigation.Scheduler.AddResource, so cancellation flows
// from the navigation context into every in-flight HTTP request. The
// progress callback mirrors *net.Fetcher; pass nil if not interested.
type Fetcher interface {
	FetchWithContext(ctx context.Context, url string, onProgress net.ProgressCallback) (string, error)
}

// Callbacks receives coordinator outputs. All callbacks are optional;
// nil entries are skipped. Callbacks are invoked serially from a single
// goroutine owned by the coordinator, so callers do not need their own
// synchronization when reacting to a single coordinator.
//
// OnStylesheet / OnScript / OnImage are invoked exactly once per
// resource that was successfully fetched and CSP-cleared. OnError is
// invoked for every resource that was skipped (CSP, invalid URL,
// unsupported mode) or that failed to fetch. OnLifecycle fires once at
// HandleDocumentEnd.
type Callbacks struct {
	OnStylesheet func(CSSResult)
	OnScript     func(ScriptResult)
	OnImage      func(ImageResult)
	OnError      func(resource Resource, err error)
	OnLifecycle  func(LifecycleEvent)
}

// Options configures a new Coordinator.
type Options struct {
	// NavigationID identifies the active navigation. The coordinator
	// rejects results from any fetcher context whose embedded ID does
	// not match this value.
	NavigationID navigation.ID
	// NavigationContext is cancelled when the navigation ends. The
	// coordinator uses it as the parent for every fetched resource.
	// Required.
	NavigationContext context.Context
	// FinalURL is the document URL after redirects. Used as the base
	// for every relative resource URL resolution. Required.
	FinalURL string
	// CSP is the parsed Content-Security-Policy from the main document
	// response. nil means no policy; every http(s) URL is allowed.
	CSP *net.CSPPolicy
	// Scheduler registers subresource loads and bounds concurrency.
	// Required.
	Scheduler *navigation.Scheduler
	// Fetcher performs the actual network requests. Required.
	Fetcher Fetcher
	// Callbacks deliver results. All fields optional.
	Callbacks Callbacks
	// Recorder receives phase timings for the subresource pipeline.
	// nil disables per-phase recording for this navigation.
	Recorder *metrics.Recorder
}

// Coordinator owns subresource lifecycle for one navigation. It is not
// safe to reuse across navigations; create a new Coordinator for each
// document load. The zero value is not usable; construct via New.
type Coordinator struct {
	navID    navigation.ID
	navCtx   context.Context
	baseURL  string
	baseURLP *url.URL
	csp      *net.CSPPolicy
	sched    *navigation.Scheduler
	fetcher  Fetcher
	cb       Callbacks
	rec      *metrics.Recorder

	mu        sync.Mutex
	closed    bool
	nextPos   int              // next Position to assign
	results   []bufferedResult // results completed in any order, drained at HandleDocumentEnd
	inFlight  sync.WaitGroup   // counts active fetches
	asyncN    int32            // active async script callbacks
	asyncDone chan struct{}    // closed when asyncN hits 0 (after HandleDocumentEnd)
	drainDone bool             // true after HandleDocumentEnd has run
	finalized chan struct{}    // closed when HandleDocumentEnd / Cancel completes
}

// bufferedResult is a completed result waiting to be emitted in
// document order. position is the document-order index assigned when
// HandleResource was called.
type bufferedResult struct {
	position int
	kind     ResourceKind
	css      *CSSResult
	script   *ScriptResult
	image    *ImageResult
}

// New constructs a Coordinator. Returns an error if any required field
// is missing or invalid.
func New(opts Options) (*Coordinator, error) {
	if opts.NavigationContext == nil {
		return nil, errors.New("documentloader: NavigationContext is required")
	}
	if opts.Scheduler == nil {
		return nil, errors.New("documentloader: Scheduler is required")
	}
	if opts.Fetcher == nil {
		return nil, errors.New("documentloader: Fetcher is required")
	}
	if opts.FinalURL == "" {
		return nil, errors.New("documentloader: FinalURL is required")
	}
	baseURL, err := url.Parse(opts.FinalURL)
	if err != nil || baseURL == nil {
		return nil, fmt.Errorf("documentloader: FinalURL %q: %w", opts.FinalURL, ErrInvalidBaseURL)
	}
	c := &Coordinator{
		navID:    opts.NavigationID,
		navCtx:   opts.NavigationContext,
		baseURL:  opts.FinalURL,
		baseURLP: baseURL,
		csp:      opts.CSP,
		sched:    opts.Scheduler,
		fetcher:  opts.Fetcher,
		cb:       opts.Callbacks,
		rec:      opts.Recorder,
		finalized: make(chan struct{}),
		asyncDone: make(chan struct{}),
	}
	if c.rec != nil {
		c.rec.BeginPhase(metrics.PhaseNavigation)
	}
	return c, nil
}

// FinalURL returns the document URL the coordinator resolves against.
// Callers use it to build absolute URLs or to verify CSP inputs.
func (c *Coordinator) FinalURL() string { return c.baseURL }

// IsActive reports whether the coordinator still owns its navigation.
// Returns false once Cancel or HandleDocumentEnd has been called or
// when the navigation context has been cancelled.
func (c *Coordinator) IsActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case <-c.navCtx.Done():
		return false
	default:
		return true
	}
}

// HandleResource schedules or emits one discovered resource. It is
// safe to call from the streaming parser goroutine and from multiple
// goroutines concurrently.
//
// For each resource the coordinator:
//  1. assigns the next document-order position;
//  2. resolves the URL against FinalURL;
//  3. consults CSP for the appropriate directive;
//  4. fetches via the navigation-scoped Scheduler context (or emits
//     inline scripts immediately);
//  5. emits the result via the matching callback;
//  6. calls Scheduler.RemoveResource exactly once.
//
// HandleResource returns immediately. The fetch and result emission
// happen on a coordinator-owned goroutine so the streaming parser is
// never blocked on a slow network response.
//
// Skipped resources (CSP block, invalid URL, unsupported mode,
// inactive navigation) report via OnError with a *SkippedError.
func (c *Coordinator) HandleResource(r Resource) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		c.skip(r, "coordinator closed")
		return
	}
	select {
	case <-c.navCtx.Done():
		c.mu.Unlock()
		c.skip(r, "navigation cancelled")
		return
	default:
	}
	r.Position = c.nextPos
	c.nextPos++
	// M5: external async scripts are tracked via asyncN, NOT inFlight.
	// HandleDocumentEnd waits on inFlight; we don't want async fetches
	// that may still be in flight at drain time to block document_end.
	// asyncN is incremented here (when the fetch starts) and
	// decremented by fireAsync after the callback returns. This
	// ensures the load event waits for async fetches that are still
	// pending at drain time, not just those that have begun executing.
	isAsyncExternalScript := !r.Inline && r.Kind == KindScript && r.ScriptMode == ScriptModeAsync
	if isAsyncExternalScript {
		atomic.AddInt32(&c.asyncN, 1)
	} else {
		c.inFlight.Add(1)
	}
	c.mu.Unlock()

	go c.processResource(r)
}

// HandleDocumentEnd waits for all in-flight fetches to settle, emits
// any buffered results in document order, then fires EventDocumentEnd.
// After HandleDocumentEnd returns, the coordinator is closed and any
// further HandleResource calls are reported as skipped.
func (c *Coordinator) HandleDocumentEnd(ctx context.Context) error {
	if c.rec != nil {
		c.rec.BeginPhase(metrics.PhaseParse)
	}
	// Wait for in-flight fetches, but respect the caller's deadline.
	done := make(chan struct{})
	go func() {
		c.inFlight.Wait()
		close(done)
	}()
	var waitErr error
	select {
	case <-done:
	case <-ctx.Done():
		waitErr = ctx.Err()
	case <-c.navCtx.Done():
		waitErr = c.navCtx.Err()
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return waitErr
	}
	c.closed = true
	c.drainLocked()
	c.drainDone = true
	close(c.finalized)
	c.mu.Unlock()

	// M5: Emit EventDOMContentLoaded after classic + deferred drain.
	// Async scripts may still be in flight; EventLoad fires below
	// once they all complete.
	if c.cb.OnLifecycle != nil {
		c.cb.OnLifecycle(EventDOMContentLoaded)
	}

	// If no async scripts are in flight (zero counter), fire EventLoad
	// and EventDocumentEnd immediately. Otherwise spawn a watcher that
	// fires them when async drains. HandleDocumentEnd returns
	// promptly either way; callers that want to wait for load can
	// poll Snapshot().Lifecycle or wait on Done() / a custom signal.
	if atomic.LoadInt32(&c.asyncN) == 0 {
		c.fireLoadEvents()
	} else {
		go c.waitAsyncAndFireLoad()
	}

	if c.rec != nil {
		c.rec.EndPhase(metrics.PhaseParse)
		c.rec.BeginPhase(metrics.PhaseStyle)
		c.rec.EndPhase(metrics.PhaseStyle)
	}
	return waitErr
}

// fireLoadEvents emits EventLoad and EventDocumentEnd. Safe to call
// from any goroutine; the callbacks are read-only via c.cb.
func (c *Coordinator) fireLoadEvents() {
	if c.cb.OnLifecycle == nil {
		return
	}
	c.cb.OnLifecycle(EventLoad)
	c.cb.OnLifecycle(EventDocumentEnd)
}

// waitAsyncAndFireLoad blocks until asyncN reaches zero, then fires
// EventLoad and EventDocumentEnd. Also returns early if the
// coordinator is cancelled or the navigation context expires.
func (c *Coordinator) waitAsyncAndFireLoad() {
	select {
	case <-c.asyncDone:
		// all async callbacks completed
	case <-c.navCtx.Done():
		// navigation cancelled; skip load events
		return
	}
	c.fireLoadEvents()
}

// Cancel marks the coordinator closed, cancels all in-flight fetches
// via the navigation context, and fires EventDocumentEnd without
// emitting buffered results. Use this when a new navigation preempts
// the current one.
func (c *Coordinator) Cancel() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.finalized)
	c.mu.Unlock()
}

// Done returns a channel closed after HandleDocumentEnd or Cancel.
// Tests use this to synchronize without polling.
func (c *Coordinator) Done() <-chan struct{} { return c.finalized }

// skip is a small helper that invokes OnError with a SkippedError if
// the caller registered one. Used when the coordinator intentionally
// does not fetch a resource.
func (c *Coordinator) skip(r Resource, reason string) {
	if c.cb.OnError != nil {
		c.cb.OnError(r, &SkippedError{Reason: reason})
	}
}

// processResource performs URL resolution, CSP gating, scheduling, and
// fetch for one resource. Extracted from HandleResource so the
// goroutine bookkeeping stays in one place.
//
// For classic + defer scripts (and CSS, images), the in-flight slot
// is released via the deferred inFlight.Done() below. External async
// scripts do NOT participate in inFlight (HandleResource skips the
// Add for them); their completion is tracked via asyncN inside
// fireAsync. Inline async scripts still participate in inFlight
// because they don't have an external fetch to await.
func (c *Coordinator) processResource(r Resource) {
	isAsyncExternalScript := !r.Inline && r.Kind == KindScript && r.ScriptMode == ScriptModeAsync
	if !isAsyncExternalScript {
		defer c.inFlight.Done()
	}

	// Inline scripts: emit immediately, no fetch.
	if r.Inline {
		if r.Kind != KindScript {
			c.skip(r, "inline non-script resource")
			return
		}
		if r.ScriptMode.IsUnsupported() {
			c.skip(r, "unsupported script mode: "+r.ScriptMode.String())
			return
		}
		c.emitScript(ScriptResult{
			Source:   append([]byte(nil), r.Source...),
			Inline:   true,
			Mode:     ScriptModeClassic,
			Position: r.Position,
		})
		return
	}

	// External resource: resolve, CSP-check, schedule, fetch.
	resolved, err := ResolveURL(c.baseURL, r.URL)
	if err != nil {
		c.skip(r, err.Error())
		return
	}
	if !IsHTTPOrHTTPS(resolved) {
		// data:, blob:, file:, etc. — out of scope for M1.
		c.skip(r, "non-http(s) scheme: "+resolved)
		return
	}
	if reason := c.cspCheck(r.Kind, resolved); reason != "" {
		c.skip(r, reason)
		return
	}

	priority := c.priorityFor(r.Kind)
	load, resCtx := c.sched.AddResource(c.navCtx, resolved, priority)
	if load.ID == navigation.Invalid {
		// Either the scheduler's rate limiter rejected the load or
		// the parent context was already cancelled. Either way we
		// should not fetch.
		c.skip(r, "scheduler rejected resource")
		return
	}
	cleanupOnce := sync.Once{}
	cleanup := func() { cleanupOnce.Do(func() { c.sched.RemoveResource(load.ID) }) }
	defer cleanup()

	// Re-check that we are still the active navigation before issuing
	// the request. Between AddResource (which acquired a scheduler slot)
	// and now, the navigation could have been pre-empted by a newer
	// Begin() call. We do not compare resource IDs (those are unrelated
	// to navigation IDs); we ask the scheduler directly.
	if !c.sched.IsActive(c.navID) {
		c.skip(r, "navigation no longer active")
		return
	}

	start := time.Now()
	body, fetchErr := c.fetcher.FetchWithContext(resCtx, resolved, nil)
	if fetchErr != nil {
		// Treat context cancellation as a benign outcome (we asked for it).
		if errors.Is(fetchErr, context.Canceled) || errors.Is(fetchErr, context.DeadlineExceeded) {
			c.skip(r, "fetch cancelled")
			return
		}
		c.skip(r, "fetch error: "+fetchErr.Error())
		return
	}
	if c.rec != nil {
		c.rec.AddCounters(metrics.Counters{BytesDownloaded: int64(len(body))})
	}

	switch r.Kind {
	case KindCSS:
		c.emitCSS(CSSResult{
			URL:      r.URL,
			Resolved: resolved,
			Source:   []byte(body),
			Position: r.Position,
		})
	case KindScript:
		result := ScriptResult{
			URL:      r.URL,
			Resolved: resolved,
			Source:   []byte(body),
			Inline:   false,
			Mode:     r.ScriptMode,
			Position: r.Position,
		}
		// M5: async scripts fire OnScript immediately on fetch complete;
		// classic + defer are buffered for drain-time emission in source
		// order so the caller can dispatch them after the document is
		// parsed.
		if r.ScriptMode == ScriptModeAsync {
			c.fireAsync(result)
			return
		}
		c.emitScript(result)
	case KindImage:
		c.emitImage(ImageResult{
			URL:      r.URL,
			Resolved: resolved,
			Source:   []byte(body),
			Position: r.Position,
		})
	}
	_ = start // reserved for future per-resource timing
}

// cspCheck returns a non-empty reason if the resource is not allowed
// by the active CSP. Empty string means the resource is permitted.
func (c *Coordinator) cspCheck(kind ResourceKind, resolved string) string {
	if c.csp == nil {
		return ""
	}
	switch kind {
	case KindCSS:
		if err := c.csp.AllowStyle(resolved, c.baseURLP); err != nil {
			return "csp: " + err.Error()
		}
	case KindScript:
		if err := c.csp.AllowScript(resolved, c.baseURLP); err != nil {
			return "csp: " + err.Error()
		}
	case KindImage:
		// Images fall under img-src; CSPPolicy.AllowsURL handles the
		// default-src fallback the same way it does for other directives.
		if !c.csp.AllowsURL("img-src", resolved, c.baseURLP) {
			return "csp: img-src disallows " + resolved
		}
	}
	return ""
}

// priorityFor maps a resource kind to its navigation priority.
func (c *Coordinator) priorityFor(kind ResourceKind) navigation.Priority {
	switch kind {
	case KindCSS:
		return navigation.PriorityBlockingCSS
	case KindScript:
		return navigation.PriorityScript
	case KindImage:
		return navigation.PriorityVisibleImage
	default:
		return navigation.PrioritySpeculative
	}
}

// emitCSS, emitScript, and emitImage buffer the result for document-
// order emission at drain time. Buffering guarantees that even when
// external fetches complete out of order, callbacks fire in the order
// the resources were discovered. This is the "stylesheet source order
// wins over response completion order" guarantee.
func (c *Coordinator) emitCSS(r CSSResult)  { c.buffer(r.Position, bufferedResult{position: r.Position, kind: KindCSS, css: &r}) }
func (c *Coordinator) emitScript(r ScriptResult) {
	c.buffer(r.Position, bufferedResult{position: r.Position, kind: KindScript, script: &r})
}
func (c *Coordinator) emitImage(r ImageResult) {
	c.buffer(r.Position, bufferedResult{position: r.Position, kind: KindImage, image: &r})
}

// buffer stores a completed result at its source position. Drain
// happens once at HandleDocumentEnd. Buffering (rather than emitting
// immediately) is what lets the coordinator preserve document order
// even when external fetches complete out of order.
func (c *Coordinator) buffer(position int, r bufferedResult) {
	c.mu.Lock()
	c.results = append(c.results, r)
	c.mu.Unlock()
}

// fireAsync delivers an async script result to the caller immediately
// rather than buffering it for drain. M5: async scripts execute when
// ready, no source-order guarantee. The callback runs on the fetcher
// goroutine; callers must be thread-safe.
//
// asyncN is incremented by HandleResource when an async fetch starts
// (so in-flight async fetches are counted even before the response
// arrives), and decremented here after the callback returns. When it
// reaches zero AND HandleDocumentEnd has been called, the asyncDone
// channel is closed and EventLoad / EventDocumentEnd fire.
func (c *Coordinator) fireAsync(r ScriptResult) {
	defer func() {
		n := atomic.AddInt32(&c.asyncN, -1)
		if n == 0 {
			c.mu.Lock()
			drained := c.drainDone
			c.mu.Unlock()
			if drained {
				select {
				case <-c.asyncDone:
					// already closed
				default:
					close(c.asyncDone)
				}
			}
		}
	}()
	if c.cb.OnScript != nil {
		c.cb.OnScript(r)
	}
}

// drainLocked emits all buffered results in source-order position.
// Caller must hold c.mu.
func (c *Coordinator) drainLocked() {
	if len(c.results) == 0 {
		return
	}
	sort.SliceStable(c.results, func(i, j int) bool {
		return c.results[i].position < c.results[j].position
	})
	for _, r := range c.results {
		switch r.kind {
		case KindCSS:
			if c.cb.OnStylesheet != nil && r.css != nil {
				c.cb.OnStylesheet(*r.css)
			}
		case KindScript:
			if c.cb.OnScript != nil && r.script != nil {
				c.cb.OnScript(*r.script)
			}
		case KindImage:
			if c.cb.OnImage != nil && r.image != nil {
				c.cb.OnImage(*r.image)
			}
		}
	}
}