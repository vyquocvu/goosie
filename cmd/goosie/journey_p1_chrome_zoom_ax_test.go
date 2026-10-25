package main

import (
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func journeyKey(b *browserE2E, t *testing.T, key rune) {
	t.Helper()
	b.inject(t, surface.Event{Kind: surface.EvKey, Key: key, Mods: surface.ModCommand})
}

func navSerial(b *browserE2E) uint64 {
	tab := b.f.tabMgr.Active()
	if tab == nil {
		return 0
	}
	tab.Nav.Mu.Lock()
	defer tab.Nav.Mu.Unlock()
	return tab.Nav.Serial
}

func navLoading(b *browserE2E) bool {
	tab := b.f.tabMgr.Active()
	if tab == nil {
		return false
	}
	tab.Nav.Mu.Lock()
	defer tab.Nav.Mu.Unlock()
	return tab.Nav.Loading
}

func toolbarNode(t *testing.T, b *browserE2E, id string) ax.Node {
	t.Helper()
	n, ok := findAXNode(b.window.AccessibilityTree(), id)
	if !ok {
		t.Fatalf("AX node %q missing; %s", id, b.diagnostics())
	}
	return n
}

func sendZoomKey(t *testing.T, b *browserE2E, key rune, want string) {
	t.Helper()
	journeyKey(b, t, key)
	b.waitFor(t, "zoom becomes "+want, func() bool { return b.f.toolbar.ZoomPercent() == want })
}

// awaitZoomNode returns the published zoom badge once the accessibility tree
// carries it with the whole-percent label the toolbar paints. The tree is frozen
// per presented frame, so it can lag the toolbar state by one frame; polling the
// published value (instead of a frame counter) is what keeps this race-free.
//
// It has no escape hatch: it returns only a matching node, and on deadline it
// fails by naming what the tree actually held - no node at all, or a node with a
// different value - alongside the label the badge paints. A missing node is
// therefore a failure, never a skipped assertion.
func awaitZoomNode(t *testing.T, b *browserE2E, want string) ax.Node {
	t.Helper()
	timer := time.NewTimer(e2eWaitLimit)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	stale := ax.Node{}
	sawNode := false
	for {
		if n, ok := findAXNode(b.window.AccessibilityTree(), "goosie.zoom"); ok {
			if n.Value == want {
				return n
			}
			stale, sawNode = n, true
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			if sawNode {
				t.Fatalf("zoom AX node published value %q, want %q (the badge paints %q); %s",
					stale.Value, want, b.f.toolbar.ZoomPercent(), b.diagnostics())
			}
			t.Fatalf("no goosie.zoom AX node was published within %s, want value %q (the badge paints %q); %s",
				e2eWaitLimit, want, b.f.toolbar.ZoomPercent(), b.diagnostics())
		}
	}
}

func screenshotInk(t *testing.T, b *browserE2E) int {
	t.Helper()
	path := filepath.Join(t.TempDir(), "frame.png")
	if err := b.window.WritePNG(path); err != nil {
		t.Fatalf("write headless frame: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open headless frame: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode headless frame: %v", err)
	}
	chromeBottom := tabs.TabBarHeight + toolbar.ToolbarHeight
	ink := 0
	for y := chromeBottom; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			r, g, bb, a := img.At(x, y).RGBA()
			if a != 0 && (r+g+bb)/3 < 0xC000 {
				ink++
			}
		}
	}
	return ink
}

func waitTilesIdle(t *testing.T, b *browserE2E, description string) {
	t.Helper()
	b.waitFor(t, description, func() bool {
		last := b.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0 && b.f.sched.Stats().Rasterized > 0
	})
}

// qc:BR-126
func TestJourneyBackForwardDisabledStateAndClicks(t *testing.T) {
	var mu sync.Mutex
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path]++
		mu.Unlock()
		fmt.Fprintf(w, "<!doctype html><title>%s</title><h1 id=\"%s\">%s</h1>", r.URL.Path, strings.TrimPrefix(r.URL.Path, "/"), r.URL.Path)
	}))
	defer server.Close()
	b := newBrowserE2E(t)

	back := toolbarNode(t, b, "goosie.back")
	forward := toolbarNode(t, b, "goosie.forward")
	if !back.Disabled || !forward.Disabled {
		t.Fatalf("new-tab traversal disabled = back:%t forward:%t, want true/true", back.Disabled, forward.Disabled)
	}
	startURL := b.activeURL()
	serialBeforeClicks := navSerial(b)
	mu.Lock()
	before := requests["/a"] + requests["/b"]
	mu.Unlock()
	b.clickNode(t, "goosie.back")
	b.clickNode(t, "goosie.forward")
	mu.Lock()
	after := requests["/a"] + requests["/b"]
	mu.Unlock()
	if b.activeURL() != startURL || after != before || navSerial(b) != serialBeforeClicks {
		t.Fatalf("disabled traversal changed URL/request/serial: URL=%q requests=%d serial=%d, want %q/%d/%d", b.activeURL(), after, navSerial(b), startURL, before, serialBeforeClicks)
	}

	a, bb := server.URL+"/a", server.URL+"/b"
	b.navigateViaAddressBar(t, a)
	b.waitFor(t, "page A loaded", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "a")
		return ok && b.activeURL() == a && !navLoading(b)
	})
	b.navigateViaAddressBar(t, bb)
	b.waitFor(t, "page B loaded", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "b")
		return ok && b.activeURL() == bb && !navLoading(b)
	})
	back, forward = toolbarNode(t, b, "goosie.back"), toolbarNode(t, b, "goosie.forward")
	if back.Disabled || !forward.Disabled {
		t.Fatalf("A→B traversal disabled = back:%t forward:%t, want false/true", back.Disabled, forward.Disabled)
	}
}

// qc:BR-127
func TestJourneyStopLoadingAndCompletedStopNoOp(t *testing.T) {
	slowEntered := make(chan struct{})
	canceled := make(chan struct{})
	var enteredOnce, canceledOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			enteredOnce.Do(func() { close(slowEntered) })
			<-r.Context().Done()
			canceledOnce.Do(func() { close(canceled) })
			return
		}
		fmt.Fprint(w, `<!doctype html><h1 id="old-doc">Original document</h1>`)
	}))
	defer server.Close()
	b := newBrowserE2E(t)
	oldURL := server.URL + "/old"
	b.navigateViaAddressBar(t, oldURL)
	b.waitFor(t, "original document loaded", func() bool { _, ok := findAXNode(b.window.AccessibilityTree(), "old-doc"); return ok && !navLoading(b) })

	b.navigateViaAddressBar(t, server.URL+"/slow")
	select {
	case <-slowEntered:
	case <-time.After(e2eWaitLimit):
		t.Fatal("timed out waiting for slow request to reach server")
	}
	b.waitFor(t, "reload AX control says Stop loading", func() bool { return toolbarNode(t, b, "goosie.reload").Label == "Stop loading" })
	b.clickNode(t, "goosie.reload")
	b.waitFor(t, "Stop cancels request and restores idle toolbar", func() bool { return !navLoading(b) && toolbarNode(t, b, "goosie.reload").Label == "Reload" })
	select {
	case <-canceled:
	default:
		t.Fatal("server request context was not canceled after Stop")
	}
	stoppedURL := b.activeURL()
	if _, ok := findAXNode(b.window.AccessibilityTree(), "old-doc"); !ok {
		t.Fatal("Stop removed the previously displayed document")
	}

	// Exercise the callback target after completion, as the native toolbar would
	// have no Stop action to dispatch once Loading is false.
	if b.f.toolbar.OnStop == nil {
		t.Fatal("toolbar Stop callback is not wired")
	}
	b.f.toolbar.OnStop()
	if b.activeURL() != stoppedURL {
		t.Fatalf("URL after completed Stop callback = %q, want unchanged %q", b.activeURL(), stoppedURL)
	}
	if tab := b.f.tabMgr.Active(); tab == nil || tab.Session() == nil {
		t.Fatal("completed Stop callback removed the active document session")
	}
	if _, ok := findAXNode(b.window.AccessibilityTree(), "old-doc"); !ok {
		t.Fatal("completed Stop callback changed the loaded document")
	}
}

// qc:BR-128
func TestJourneyReloadRestoresReloadLabelAndRequestsExactlyOnce(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		count := requests
		mu.Unlock()
		fmt.Fprintf(w, "<!doctype html><h1 id=\"render\">Render %d</h1>", count)
	}))
	defer server.Close()
	b := newBrowserE2E(t)
	url := server.URL + "/page"
	b.navigateViaAddressBar(t, url)
	b.waitFor(t, "first render completes", func() bool { _, ok := findAXNode(b.window.AccessibilityTree(), "render"); return ok && !navLoading(b) })
	if got := toolbarNode(t, b, "goosie.reload").Label; got != "Reload" {
		t.Fatalf("idle reload label = %q, want Reload", got)
	}
	serialBefore := navSerial(b)
	mu.Lock()
	requestsBefore := requests
	mu.Unlock()
	b.clickNode(t, "goosie.reload")
	b.waitFor(t, "reload completes with second render and serial advance", func() bool {
		return axContainsLabel(b.window.AccessibilityTree(), "Render 2") && !navLoading(b) && navSerial(b) > serialBefore
	})
	mu.Lock()
	gotRequests := requests
	mu.Unlock()
	if gotRequests != requestsBefore+1 {
		t.Fatalf("reload server requests = %d, want exactly one more than %d", gotRequests, requestsBefore)
	}
	if got := toolbarNode(t, b, "goosie.reload").Label; got != "Reload" {
		t.Fatalf("reload label after completion = %q, want Reload", got)
	}
}

// qc:BR-129
func TestJourneyZoomInGrowsContentAndDisplaysPercentage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><style>body{font:24px sans-serif}</style><h1 id="zoom-copy">Zoom content grows</h1>`)
	}))
	defer server.Close()
	b := newBrowserE2E(t)
	presentsBeforeLoad := b.window.Stats().Presents
	b.navigateViaAddressBar(t, server.URL)
	b.waitFor(t, "zoom fixture loaded", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "zoom-copy")
		return ok && !navLoading(b)
	})
	b.waitFor(t, "initial page frame presented", func() bool { return b.window.Stats().Presents > presentsBeforeLoad })
	waitTilesIdle(t, b, "initial page tiles to finish rasterizing")
	initialInk := screenshotInk(t, b)
	if got := b.f.toolbar.ZoomPercent(); got != "100%" {
		t.Fatalf("initial zoom = %q, want 100%%", got)
	}
	presentsBeforeZoom := b.window.Stats().Presents
	for i, want := range []string{"110%", "120%", "130%", "140%", "150%"} {
		sendZoomKey(t, b, '+', want)
		got, err := strconv.Atoi(strings.TrimSuffix(b.f.toolbar.ZoomPercent(), "%"))
		if err != nil || got != 110+(i*10) {
			t.Fatalf("zoom after Cmd+Plus #%d = %q, want monotonically increasing %s", i+1, b.f.toolbar.ZoomPercent(), want)
		}
	}
	b.waitFor(t, "zoomed page frame presented", func() bool { return b.window.Stats().Presents > presentsBeforeZoom })
	waitTilesIdle(t, b, "zoomed page tiles to finish rasterizing")
	zoomedInk := screenshotInk(t, b)
	if zoomedInk <= initialInk {
		t.Fatalf("content ink pixels at 150%% = %d, want more than 100%% count %d", zoomedInk, initialInk)
	}
	// The badge the address bar paints at 150% must be readable in AX. The old
	// guard skipped this whole check when the node was absent, proving nothing;
	// awaitZoomNode fails outright on absence and on a stale value.
	zoom := awaitZoomNode(t, b, "150%")
	if zoom.Value != "150%" || zoom.Value != b.f.toolbar.ZoomPercent() {
		t.Fatalf("zoom AX indicator value = %q, want the 150%% label the badge paints (%q)", zoom.Value, b.f.toolbar.ZoomPercent())
	}
	if zoom.Role != ax.RoleMeter {
		t.Fatalf("zoom AX indicator role = %v, want meter", zoom.Role)
	}
	rowTop, rowBottom := float32(tabs.TabBarHeight), float32(tabs.TabBarHeight+toolbar.ToolbarHeight)
	if zoom.X0 >= zoom.X1 || zoom.Y0 < rowTop || zoom.Y1 > rowBottom {
		t.Fatalf("zoom AX indicator rect %+v is not a visible box inside the toolbar row [%g,%g]", zoom, rowTop, rowBottom)
	}
}

// qc:BR-130
func TestJourneyZoomOutClampsAndStillAcceptsPageDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><h1 id="zoom-top">Top</h1><div style="height:2400px">long page</div><h2 id="zoom-bottom">Bottom</h2>`)
	}))
	defer server.Close()
	b := newBrowserE2E(t)
	b.navigateViaAddressBar(t, server.URL)
	b.waitFor(t, "long zoom fixture loaded", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "zoom-bottom")
		return ok && !navLoading(b)
	})
	for _, want := range []string{"90%", "80%", "70%", "60%", "50%", "40%", "30%", "25%", "25%", "25%", "25%", "25%"} {
		sendZoomKey(t, b, '-', want)
	}
	if got := b.f.toolbar.ZoomPercent(); got != "25%" || minZoom != 0.25 || got == "500%" {
		t.Fatalf("zoom lower bound = %q, want minZoom %0.2f and never maxZoom %0.1f", got, minZoom, maxZoom)
	}
	topBefore := toolbarNode(t, b, "zoom-top")
	b.clickNode(t, "zoom-top")
	b.pressKey(t, keyPageDown)
	b.waitFor(t, "PageDown scrolls after reaching zoom bound", func() bool { return toolbarNode(t, b, "zoom-top").Y0 < topBefore.Y0 })
}

// qc:BR-131
func TestJourneyCmdZeroResetsZoomAndKeepsAXGeometryValid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><h1 id="reset-copy">Reset zoom</h1><div style="height:800px">page</div>`)
	}))
	defer server.Close()
	b := newBrowserE2E(t)
	b.navigateViaAddressBar(t, server.URL)
	b.waitFor(t, "reset fixture loaded", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "reset-copy")
		return ok && !navLoading(b)
	})
	for _, key := range []rune{'+', '+', '+'} {
		journeyKey(b, t, key)
	}
	b.waitFor(t, "zoom above 100 percent", func() bool { return b.f.toolbar.ZoomPercent() == "130%" })
	journeyKey(b, t, '0')
	b.waitFor(t, "Cmd+0 resets zoom and publishes document bounds", func() bool {
		n, ok := findAXNode(b.window.AccessibilityTree(), "reset-copy")
		return ok && b.f.toolbar.ZoomPercent() == "100%" && n.X0 <= n.X1 && n.Y0 <= n.Y1
	})
	if got := b.f.toolbar.ZoomPercent(); got != "100%" {
		t.Fatalf("zoom after Cmd+0 = %q, want 100%%", got)
	}
	// What Cmd+0 promises, measured rather than assumed: the key goes through
	// framePath.handleZoom(0) (cmd/goosie/interaction.go), which sets zoom to 1.0
	// and then calls toolbar.SetZoom(1.0) - not ResetZoom() - so the badge's 2.5s
	// grace is re-armed and "100%" is still painted. zoomBadgeRect is the single
	// rule the badge paint and this node both read, so the node must exist here
	// with 100%, and must be gone once that grace runs out (or after a badge click,
	// which does call ResetZoom). Both hidden branches are pinned without a clock by
	// test/toolbar/zoom_badge_ax_test.go; nothing here is conditional.
	zoom := awaitZoomNode(t, b, "100%")
	if zoom.Value != "100%" || zoom.Value != b.f.toolbar.ZoomPercent() {
		t.Fatalf("zoom AX indicator value after Cmd+0 = %q, want the 100%% label the badge paints (%q)", zoom.Value, b.f.toolbar.ZoomPercent())
	}
}
