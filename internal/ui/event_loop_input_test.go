package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"

	"github.com/vyquocvu/goosie/internal/engine/eventloop"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// scrollRecorder wraps MockHTMLRendererComp and records the viewport and
// latency calls the tab's event-loop drain makes, so tests can assert the
// "one latest-viewport render per drain" contract end to end.
type scrollRecorder struct {
	MockHTMLRendererComp

	mu               sync.Mutex
	viewports        []eventloop.Viewport
	inputToPresent   []time.Duration
	coalescedScrolls []int
}

func (r *scrollRecorder) SetViewport(y, height float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.viewports = append(r.viewports, eventloop.Viewport{Y: y, Height: height})
}

func (r *scrollRecorder) RecordInputToPresent(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inputToPresent = append(r.inputToPresent, d)
}

func (r *scrollRecorder) RecordCoalescedScroll(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.coalescedScrolls = append(r.coalescedScrolls, n)
}

func (r *scrollRecorder) appliedViewports() []eventloop.Viewport {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]eventloop.Viewport, len(r.viewports))
	copy(out, r.viewports)
	return out
}

// newInputTestBrowser builds a headless browser whose active tab uses the
// given recorder as its HTMLRenderer, so input wiring can be driven without
// a real Fyne event loop.
func newInputTestBrowser(t *testing.T, rec HTMLRenderer) (*Browser, *Tab) {
	t.Helper()
	app := test.NewApp()
	t.Cleanup(app.Quit)

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w, true) // headless: do() runs inline
	browser.RendererFactory = func() HTMLRenderer { return rec }

	tab := browser.ActiveTab()
	assert.NotNil(t, tab, "browser should have an active tab")
	assert.NotNil(t, tab.eventLoop, "tab should own an engine event loop")
	return browser, tab
}

// mouseRecorder wraps MockHTMLRendererComp and records the hit-test and
// navigation calls the tab's mouse drain makes, returning a fixed
// hit-testable node so click/hover dispatch can be observed end to end.
type mouseRecorder struct {
	MockHTMLRendererComp

	mu      sync.Mutex
	hits    []fyne.Position
	navURLs []string
}

func (r *mouseRecorder) HitTest(x, y float32) (*renderer.RenderNode, *renderer.LayoutBox) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hits = append(r.hits, fyne.NewPos(x, y))
	return &renderer.RenderNode{ID: 7, TagName: "div"}, &renderer.LayoutBox{}
}

func (r *mouseRecorder) recordedHits() []fyne.Position {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]fyne.Position, len(r.hits))
	copy(out, r.hits)
	return out
}

// TestTabEventLoop_MouseMoveBurstCoalescesToLatestHover is the PR9 guard
// for the loop's latest-wins mouse slot: a burst of MouseMoved posts must
// collapse into one drained hover hit-test at the final position, with the
// coalesced delta reported to the loop metrics.
func TestTabEventLoop_MouseMoveBurstCoalescesToLatestHover(t *testing.T) {
	rec := &mouseRecorder{}
	browser, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()
	browser.devToolsVisible = true

	const n = 10
	for i := 0; i < n; i++ {
		assert.NoError(t, tab.eventLoop.PostInput(eventloop.InputEvent{
			Type:      eventloop.InputMouseMove,
			X:         float32(i * 5),
			Y:         10,
			Timestamp: time.Now(),
		}))
	}
	tab.drainInputLoop()

	hits := rec.recordedHits()
	assert.Len(t, hits, 1, "a mouse-move burst must produce one drained hover hit test")
	if len(hits) == 1 {
		assert.Equal(t, fyne.NewPos(float32((n-1)*5), 10), hits[0],
			"the latest mouse position must win")
	}
	// The hover inspect callback fired for the hit node (headless do()
	// runs inline) and selected it in the inspect panel.
	assert.Equal(t, int64(7), browser.inspectPanel.selectedNode.ID,
		"hover over a new element must dispatch the inspect callback")

	m := tab.eventLoop.Metrics()
	assert.Equal(t, uint64(n), m.InputEventsReceived)
	assert.Equal(t, uint64(n-1), m.CoalescedMouseMoves,
		"all but the first mouse move are coalesced")
}

// TestTabEventLoop_LeftClickHitTestsWithScrollOffset is the PR9 guard for
// discrete click dispatch: a left click drains in FIFO order and the drain
// hit-tests at the content coordinates (widget position + the latest
// drained scroll offset), selecting the element for inspection.
func TestTabEventLoop_LeftClickHitTestsWithScrollOffset(t *testing.T) {
	rec := &mouseRecorder{}
	browser, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()
	browser.devToolsVisible = true // Scroll first and let its drain apply the viewport, exactly as a real
	// scroll burst would on its own UI turn; then click. The drain mirrors
	// the applied viewport into the tab so the click hit-tests in content
	// coordinates.
	assert.NoError(t, tab.eventLoop.PostInput(eventloop.InputEvent{
		Type:      eventloop.InputScroll,
		Viewport:  eventloop.Viewport{Y: 42, Height: 600},
		Timestamp: time.Now(),
	}))
	tab.drainInputLoop()

	assert.NoError(t, tab.eventLoop.PostInput(eventloop.InputEvent{
		Type:      eventloop.InputClick,
		Button:    1,
		X:         10,
		Y:         20,
		Timestamp: time.Now(),
	}))
	tab.drainInputLoop()

	hits := rec.recordedHits()
	assert.Len(t, hits, 1)
	if len(hits) == 1 {
		assert.Equal(t, fyne.NewPos(10, 62), hits[0],
			"click hit test must add the latest drained scroll offset")
	}
	assert.Equal(t, int64(7), browser.inspectPanel.selectedNode.ID,
		"left click must select the hit element")
}

// TestTabEventLoop_RightClickOpensContextMenu is the PR9 guard for the
// right-click path: button-2 clicks drain through the loop, hit-test at
// the scroll-adjusted position, and reach the dev-tools context menu (no
// panic is the baseline; the popup show is a fyne-side concern).
func TestTabEventLoop_RightClickOpensContextMenu(t *testing.T) {
	rec := &mouseRecorder{}
	browser, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()
	assert.NotNil(t, browser.devToolsMenu, "test browser should own a context menu")

	assert.NoError(t, tab.eventLoop.PostInput(eventloop.InputEvent{
		Type:      eventloop.InputClick,
		Button:    2,
		X:         50,
		Y:         60,
		AbsX:      150,
		AbsY:      160,
		Timestamp: time.Now(),
	}))

	assert.NotPanics(t, func() { tab.drainInputLoop() })

	hits := rec.recordedHits()
	assert.Len(t, hits, 1)
	if len(hits) == 1 {
		assert.Equal(t, fyne.NewPos(50, 60), hits[0])
	}
}

// TestTabEventLoop_LinkTapNavigatesInFIFOOrder is the PR9 guard for
// hyperlink taps: a click carrying a URL dispatches navigation through
// the loop's ordered FIFO, interleaved with plain clicks and keys without
// reordering.
func TestTabEventLoop_LinkTapNavigatesInFIFOOrder(t *testing.T) {
	rec := &mouseRecorder{}
	browser, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()

	var navigated []string
	browser.SetNavigationCallback(func(url string) { navigated = append(navigated, url) })

	posted := []eventloop.InputEvent{
		{Type: eventloop.InputClick, Button: 1, X: 1, Y: 1, Timestamp: time.Now()},
		{Type: eventloop.InputClick, URL: "https://example.com/a", Timestamp: time.Now()},
		{Type: eventloop.InputClick, Key: string(fyne.KeyF12), Timestamp: time.Now()},
		{Type: eventloop.InputClick, URL: "https://example.com/b", Timestamp: time.Now()},
	}
	for _, ev := range posted {
		assert.NoError(t, tab.eventLoop.PostInput(ev))
	}
	tab.drainInputLoop()

	assert.Equal(t, []string{"https://example.com/a", "https://example.com/b"}, navigated,
		"link taps must navigate in FIFO order through the loop")
	assert.Equal(t, uint64(len(posted)), tab.eventLoop.Metrics().InputEventsReceived,
		"all clicks are counted, none dropped")
}

// refreshRecorder wraps MockHTMLRendererComp and counts UpdateViewport
// calls so tests can assert the typed-mutation present hook refreshes the
// canvas exactly once per invocation.
type refreshRecorder struct {
	MockHTMLRendererComp

	mu      sync.Mutex
	updates int
}

func (r *refreshRecorder) UpdateViewport() fyne.CanvasObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates++
	return nil
}

// TestTabRefreshFromMutationMarshalsCanvasRefresh is the PR6 guard: the
// typed-mutation sink's present hook must marshal one canvas refresh onto
// the UI thread (headless do() runs inline) with no full reparse.
func TestTabRefreshFromMutationMarshalsCanvasRefresh(t *testing.T) {
	rec := &refreshRecorder{}
	_, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()

	tab.RefreshFromMutation()
	tab.RefreshFromMutation()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Equal(t, 2, rec.updates)
}

// TestTabEventLoop_ScrollBurstAppliesLatestViewportOnce is the PR2 guard:
// a burst of scroll events posted into the tab's event loop must collapse
// into exactly one drain that renders the latest viewport — no render for
// every event, and the last position wins.
func TestTabEventLoop_ScrollBurstAppliesLatestViewportOnce(t *testing.T) {
	rec := &scrollRecorder{}
	_, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()

	// Simulate a burst of wheel events: N posts, one drain (the UI-turn
	// scheduling that produces a single drain is exercised by posting
	// directly, mirroring what a queued fyne.Do turn would observe).
	const n = 20
	for i := 0; i < n; i++ {
		assert.NoError(t, tab.eventLoop.PostInput(eventloop.InputEvent{
			Type:      eventloop.InputScroll,
			Viewport:  eventloop.Viewport{Y: float32(i * 10), Height: 600},
			Timestamp: time.Now(),
		}))
	}

	tab.drainInputLoop()

	applied := rec.appliedViewports()
	assert.Len(t, applied, 1, "a burst must produce exactly one viewport render")
	if len(applied) == 1 {
		assert.Equal(t, float32((n-1)*10), applied[0].Y, "the latest viewport must win")
		assert.Equal(t, float32(600), applied[0].Height)
	}

	// The renderer-side coalescer must not have been used directly by the
	// tab: the loop's latest-wins slot replaced it for this path. On the
	// recorder it stays nil, proving the tab never called ScheduleScroll.
	assert.Nil(t, rec.coalescer, "tab should not use the renderer coalescer directly")

	m := tab.eventLoop.Metrics()
	assert.Equal(t, uint64(n), m.InputEventsReceived, "every posted scroll is counted")
	assert.Equal(t, uint64(n-1), m.CoalescedScrollEvents, "all but the first event are coalesced")
	rec.mu.Lock()
	inputToPresent := len(rec.inputToPresent)
	rec.mu.Unlock()
	assert.Equal(t, 1, inputToPresent, "input-to-present latency recorded once per render")
}

// TestTabEventLoop_ScrollThroughOnScrolled uses the real OnScrolled entry
// point (the path the Fyne scroll container invokes) and verifies the
// latest viewport is applied.
func TestTabEventLoop_ScrollThroughOnScrolled(t *testing.T) {
	rec := &scrollRecorder{}
	_, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()
	if tab.contentScroll.OnScrolled == nil {
		t.Fatal("OnScrolled should be wired by ensureHTMLRenderer")
	}

	tab.contentScroll.OnScrolled(fyne.NewPos(0, 42))
	tab.contentScroll.OnScrolled(fyne.NewPos(0, 84))
	tab.contentScroll.OnScrolled(fyne.NewPos(0, 126))

	// In headless mode do() runs inline, so each OnScrolled already
	// drained; assert the drain applied the final position and that no
	// further drain finds work.
	tab.drainInputLoop()

	applied := rec.appliedViewports()
	assert.NotEmpty(t, applied, "scrolls should have been applied")
	if len(applied) > 0 {
		assert.Equal(t, float32(126), applied[len(applied)-1].Y,
			"the last drained viewport must be the latest scroll position")
	}
}

// TestTabEventLoop_ClickAndKeyPreserveOrdering verifies discrete input
// keeps FIFO order through the tab wiring: clicks and keys drained in the
// posted sequence, and the browser-level dispatch (F12 toggle) fires once
// per key in that order.
func TestTabEventLoop_ClickAndKeyPreserveOrdering(t *testing.T) {
	rec := &scrollRecorder{}
	browser, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()

	browser.devToolsVisible = false
	posted := []eventloop.InputEvent{
		{Type: eventloop.InputClick, Button: 1, Timestamp: time.Now()},
		{Type: eventloop.InputKey, Key: string(fyne.KeyF12), Timestamp: time.Now()},
		{Type: eventloop.InputClick, Button: 2, Timestamp: time.Now()},
		{Type: eventloop.InputKey, Key: string(fyne.KeyF12), Timestamp: time.Now()},
	}
	for _, ev := range posted {
		assert.NoError(t, tab.eventLoop.PostInput(ev))
	}

	tab.drainInputLoop()

	// The two F12 keys toggle dev tools twice: off -> on -> off. The
	// clicks interleaved between them must not disrupt that order.
	assert.False(t, browser.devToolsVisible,
		"two F12 keys in FIFO order must toggle dev tools off again")
	assert.Equal(t, uint64(len(posted)), tab.eventLoop.Metrics().InputEventsReceived,
		"all discrete input is counted, none dropped")
	assert.Equal(t, uint64(0), tab.eventLoop.Metrics().InputSignalsDropped,
		"FIFO capacity must hold all posted events")
}

// TestTabEventLoop_EmptyDrainIsNoop ensures draining with no pending input
// touches nothing (no spurious render or dispatch).
func TestTabEventLoop_EmptyDrainIsNoop(t *testing.T) {
	rec := &scrollRecorder{}
	browser, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()
	browser.devToolsVisible = false

	tab.drainInputLoop()

	assert.Len(t, rec.appliedViewports(), 0, "empty drain must not render")
	assert.False(t, browser.devToolsVisible, "empty drain must not dispatch")
}

// TestTabEventLoop_ClosedLoopRejectsInput guards the tab-close contract:
// after Close, posting fails with ErrClosed instead of silently queueing.
func TestTabEventLoop_ClosedLoopRejectsInput(t *testing.T) {
	_, tab := newInputTestBrowser(t, &scrollRecorder{})
	tab.eventLoop.Close()

	err := tab.eventLoop.PostInput(eventloop.InputEvent{Type: eventloop.InputClick})
	assert.ErrorIs(t, err, eventloop.ErrClosed, "posting after close must fail predictably")
}

// TestTabEventLoop_MousePosterThrottlesHoverMoves is the PR11 guard for
// the pre-throttle: mouse-move posts inside the hover window never reach
// the loop (the drain would discard them anyway), while clicks and link
// taps are always posted.
func TestTabEventLoop_MousePosterThrottlesHoverMoves(t *testing.T) {
	rec := &mouseRecorder{}
	_, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()

	// First move with a cold throttle: posted and drained (headless do()
	// runs inline, so the drain's hit test sets lastHoverHit fresh).
	tab.lastHoverHit = time.Now().Add(-time.Hour)
	tab.postCanvasMouseInput(renderer.MouseInput{Kind: renderer.MouseInputMove, X: 1, Y: 2})
	assert.Equal(t, uint64(1), tab.eventLoop.Metrics().InputEventsReceived,
		"a move outside the hover window must be posted")

	// A move inside the window is dropped before the loop: no post, no
	// drain work, no hit test.
	tab.postCanvasMouseInput(renderer.MouseInput{Kind: renderer.MouseInputMove, X: 3, Y: 4})
	assert.Equal(t, uint64(1), tab.eventLoop.Metrics().InputEventsReceived,
		"a move inside the hover window must be dropped before posting")
	assert.Len(t, rec.recordedHits(), 1, "no extra hit test inside the hover window")

	// Clicks are discrete and never throttled by the hover window.
	tab.postCanvasMouseInput(renderer.MouseInput{Kind: renderer.MouseInputClick, Button: 1, X: 5, Y: 6})
	assert.Equal(t, uint64(2), tab.eventLoop.Metrics().InputEventsReceived,
		"clicks must always be posted")
}

// TestTabEventLoop_FullScrollFlowThroughRenderQueue is the PR3 happy
// path: a scroll burst drained from the loop becomes one render request
// that is executed and presented through the generation-gated present
// callback — exactly one SetViewport with the final position, one
// input-to-present sample, and the coalesced delta reported.
func TestTabEventLoop_FullScrollFlowThroughRenderQueue(t *testing.T) {
	rec := &scrollRecorder{}
	_, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()

	const n = 20
	for i := 0; i < n; i++ {
		assert.NoError(t, tab.eventLoop.PostInput(eventloop.InputEvent{
			Type:      eventloop.InputScroll,
			Viewport:  eventloop.Viewport{Y: float32(i * 10), Height: 600},
			Timestamp: time.Now(),
		}))
	}
	tab.drainInputLoop()

	applied := rec.appliedViewports()
	assert.Len(t, applied, 1, "burst must produce exactly one presented render")
	if len(applied) == 1 {
		assert.Equal(t, float32((n-1)*10), applied[0].Y, "the latest viewport must win")
	}

	rec.mu.Lock()
	inputToPresent := len(rec.inputToPresent)
	coalesced := len(rec.coalescedScrolls)
	rec.mu.Unlock()
	assert.Equal(t, 1, inputToPresent, "input-to-present latency recorded once")
	assert.Equal(t, 1, coalesced, "coalesced-scroll delta reported once")

	m := tab.eventLoop.Metrics()
	assert.Equal(t, uint64(n), m.InputEventsReceived)
	assert.Equal(t, uint64(n-1), m.CoalescedScrollEvents)
	assert.Equal(t, uint64(1), m.RenderRequestsCreated, "one render request per drain")
	assert.Equal(t, uint64(1), m.FramesPresented, "one frame presented")
	assert.Equal(t, uint64(0), m.StaleFramesDropped)

	// PR11: the drain executes the render in the same UI turn — the
	// render-request queue must be empty once drainInputLoop returns,
	// rather than waiting for a deferred execution turn.
	select {
	case req := <-tab.eventLoop.RenderRequests():
		t.Fatalf("drain must execute the render request in-turn, left %v queued", req)
	default:
	}
}

// TestTabEventLoop_RenderQueueReplacesOldRequest verifies the
// latest-only render queue: scheduling a second render while the first is
// pending cancels the superseded request and the execution turn renders
// only the newest viewport.
func TestTabEventLoop_RenderQueueReplacesOldRequest(t *testing.T) {
	rec := &scrollRecorder{}
	_, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()

	gen := tab.eventLoop.Generation()
	ctx := context.Background()
	first, err := tab.eventLoop.ScheduleRender(ctx, eventloop.RenderRequest{
		Generation: gen,
		Viewport:   eventloop.Viewport{Y: 100, Height: 600},
		Reason:     eventloop.RenderReasonViewport,
	})
	assert.NoError(t, err)
	second, err := tab.eventLoop.ScheduleRender(ctx, eventloop.RenderRequest{
		Generation: gen,
		Viewport:   eventloop.Viewport{Y: 500, Height: 600},
		Reason:     eventloop.RenderReasonViewport,
	})
	assert.NoError(t, err)

	assert.ErrorIs(t, first.Context.Err(), context.Canceled,
		"superseded render request must be cancelled")
	assert.Nil(t, second.Context.Err(), "latest request stays live")

	tab.executeRenderRequest()

	applied := rec.appliedViewports()
	assert.Len(t, applied, 1, "one presented render after replacement")
	if len(applied) == 1 {
		assert.Equal(t, float32(500), applied[0].Y, "the replaced request's viewport wins")
	}
	m := tab.eventLoop.Metrics()
	assert.Equal(t, uint64(2), m.RenderRequestsCreated)
	assert.Equal(t, uint64(1), m.RenderRequestsDropped, "old request replaced, not executed")
	assert.Equal(t, uint64(1), m.FramesPresented)
}

// TestTabEventLoop_StaleRenderDroppedAfterNavigation verifies the PR3
// stale-dropping gate: a scroll render scheduled for the old document is
// never painted once the tab advances to a new document generation.
func TestTabEventLoop_StaleRenderDroppedAfterNavigation(t *testing.T) {
	rec := &scrollRecorder{}
	_, tab := newInputTestBrowser(t, rec)
	tab.ensureHTMLRenderer()

	gen := tab.eventLoop.Generation()
	_, err := tab.eventLoop.ScheduleRender(context.Background(), eventloop.RenderRequest{
		Generation: gen,
		Viewport:   eventloop.Viewport{Y: 200, Height: 600},
		Reason:     eventloop.RenderReasonViewport,
	})
	assert.NoError(t, err)

	// Navigation renders a new document, bumping the generation and
	// cancelling the pending render.
	tab.bumpDocumentGeneration()

	tab.executeRenderRequest()

	assert.Len(t, rec.appliedViewports(), 0,
		"stale render from the previous document must never be painted")
	m := tab.eventLoop.Metrics()
	assert.Equal(t, uint64(1), m.StaleFramesDropped, "stale result dropped by the loop")
	assert.Equal(t, uint64(0), m.FramesPresented)
}

// TestTabEventLoop_DocumentRenderBumpsGeneration guards the wiring that
// makes stale dropping possible: each new document render advances the
// loop's generation and cancels any pending render.
func TestTabEventLoop_DocumentRenderBumpsGeneration(t *testing.T) {
	_, tab := newInputTestBrowser(t, &scrollRecorder{})

	_, err := tab.eventLoop.ScheduleRender(context.Background(), eventloop.RenderRequest{
		Generation: tab.eventLoop.Generation(),
		Viewport:   eventloop.Viewport{Y: 10, Height: 600},
	})
	assert.NoError(t, err)

	tab.bumpDocumentGeneration()
	assert.Equal(t, uint64(1), tab.eventLoop.Generation().Document,
		"generation advances after the first document render")

	tab.bumpDocumentGeneration()
	assert.Equal(t, uint64(2), tab.eventLoop.Generation().Document,
		"generation advances on every document render")
}
