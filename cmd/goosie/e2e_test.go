package main

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

const e2eWaitLimit = 8 * time.Second

type browserE2E struct {
	f        *framePath
	window   *headless.Window
	stopLoop context.CancelFunc
	loopDone chan error
	closed   bool
}

func newBrowserE2E(t *testing.T) *browserE2E {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	c, err := parse([]string{
		"-backend", platform.Headless,
		"-width", "640",
		"-height", "480",
		"-dpr", "1",
		"-download-dir", filepath.Join(home, "Downloads"),
	})
	if err != nil {
		t.Fatalf("parse E2E config: %v", err)
	}
	f, err := build(c)
	if err != nil {
		t.Fatalf("build browser: %v", err)
	}
	b := &browserE2E{f: f}
	t.Cleanup(func() { b.close(t) })

	f.runCtx, f.runCancel = context.WithCancel(context.Background())
	if err := f.openWindow(); err != nil {
		t.Fatalf("open headless browser window: %v", err)
	}
	cw, ok := f.window.(*chromeWindow)
	if !ok {
		t.Fatalf("window type = %T, want *chromeWindow", f.window)
	}
	b.window, ok = cw.Window.(*headless.Window)
	if !ok {
		t.Fatalf("platform window type = %T, want *headless.Window", cw.Window)
	}

	loopCtx, stopLoop := context.WithCancel(context.Background())
	b.stopLoop = stopLoop
	b.loopDone = make(chan error, 1)
	go func() { b.loopDone <- f.loop.Run(loopCtx) }()
	return b
}

func (b *browserE2E) close(t *testing.T) {
	t.Helper()
	if b == nil || b.closed || b.f == nil {
		return
	}
	b.closed = true
	f := b.f
	if f.runCancel != nil {
		f.runCancel()
	}
	if b.stopLoop != nil {
		b.stopLoop()
	}
	if b.loopDone != nil {
		select {
		case err := <-b.loopDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("stop browser event loop: %v", err)
			}
		case <-time.After(e2eWaitLimit):
			t.Error("browser event loop did not stop after cancellation")
		}
	}
	if f.window != nil {
		if err := f.window.Close(); err != nil {
			t.Errorf("close browser window: %v", err)
		}
	}
	if f.client != nil {
		if err := f.client.Close(); err != nil {
			t.Errorf("close HTTP client: %v", err)
		}
	}
	if f.privateClient != nil {
		if err := f.privateClient.Close(); err != nil {
			t.Errorf("close private HTTP client: %v", err)
		}
	}

	navDone := make(chan struct{})
	go func() {
		f.navWg.Wait()
		close(navDone)
	}()
	select {
	case <-navDone:
	case <-time.After(e2eWaitLimit):
		t.Error("navigation workers did not stop after cancellation")
		return
	}
	if f.navResults != nil {
		close(f.navResults)
	}
	if f.imgResults != nil {
		close(f.imgResults)
	}
	drainsDone := make(chan struct{})
	go func() {
		f.drainWG.Wait()
		close(drainsDone)
	}()
	select {
	case <-drainsDone:
	case <-time.After(e2eWaitLimit):
		t.Error("browser result-drain workers did not stop")
	}
	f.closeAllSessions()
	if f.pool != nil {
		if err := f.pool.Close(); err != nil {
			t.Errorf("close raster workers: %v", err)
		}
	}
}

func (b *browserE2E) inject(t *testing.T, event surface.Event) {
	t.Helper()
	if err := b.window.Inject(event); err != nil {
		t.Fatalf("inject %s event: %v", event.Kind, err)
	}
}

func (b *browserE2E) pressKey(t *testing.T, key rune) {
	t.Helper()
	b.inject(t, surface.Event{Kind: surface.EvKey, Key: key})
}

func (b *browserE2E) click(t *testing.T, x, y int32) {
	t.Helper()
	point := frame.Point{X: x, Y: y}
	b.inject(t, surface.Event{Kind: surface.EvPointer, Action: surface.PointerPress, Button: surface.ButtonLeft, Pos: point})
	b.inject(t, surface.Event{Kind: surface.EvPointer, Action: surface.PointerRelease, Button: surface.ButtonLeft, Pos: point})
}

func (b *browserE2E) clickNode(t *testing.T, id string) {
	t.Helper()
	node, ok := findAXNode(b.window.AccessibilityTree(), id)
	if !ok {
		t.Fatalf("accessibility node %q is missing; %s", id, b.diagnostics())
	}
	b.click(t, int32((node.X0+node.X1)/2), int32((node.Y0+node.Y1)/2))
}

func (b *browserE2E) clickToolbarButton(t *testing.T, button toolbar.Button) {
	t.Helper()
	width := int32(b.f.config.devSize().W)
	r := toolbar.ButtonRect(button, width)
	x := (r.X0 + r.X1) / 2
	y := int32(tabs.TabBarHeight) + (r.Y0+r.Y1)/2
	b.click(t, x, y)
}

func (b *browserE2E) navigateViaAddressBar(t *testing.T, url string) {
	t.Helper()
	b.inject(t, surface.Event{Kind: surface.EvAXSetValue, AXTarget: surface.AXTargetAddressBar, Text: url})
	b.pressKey(t, '\r')
}

func (b *browserE2E) waitFor(t *testing.T, description string, condition func() bool) {
	t.Helper()
	timer := time.NewTimer(e2eWaitLimit)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatalf("timed out waiting for %s; %s", description, b.diagnostics())
		}
	}
}

func (b *browserE2E) diagnostics() string {
	active := b.f.tabMgr.Active()
	if active == nil {
		return fmt.Sprintf("no active tab; presents=%d", b.window.Stats().Presents)
	}
	active.Nav.Mu.Lock()
	loading := active.Nav.Loading
	active.Nav.Mu.Unlock()
	history, _ := active.History().Entries()
	nameNode, nameFound := findAXNode(b.window.AccessibilityTree(), "name")
	return fmt.Sprintf("url=%q loading=%t error=%q history=%v nameAX=(%t,%+v) ime=%t presents=%d injected=%d", active.URL(), loading, active.Error(), history, nameFound, nameNode, b.window.IMEEnabled(), b.window.Stats().Presents, b.window.Stats().Injected)
}

func (b *browserE2E) activeURL() string {
	if active := b.f.tabMgr.Active(); active != nil {
		return active.URL()
	}
	return ""
}

func findAXNode(nodes []ax.Node, id string) (ax.Node, bool) {
	for _, node := range nodes {
		if node.ID == id {
			return node, true
		}
		if found, ok := findAXNode(node.Children, id); ok {
			return found, true
		}
	}
	return ax.Node{}, false
}

func axContainsLabel(nodes []ax.Node, want string) bool {
	for _, node := range nodes {
		if strings.Contains(node.Label, want) || strings.Contains(node.Value, want) || axContainsLabel(node.Children, want) {
			return true
		}
	}
	return false
}

func TestE2ECoreBrowserJourney(t *testing.T) {
	var requestsMu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requests[r.URL.Path]++
		count := requests[r.URL.Path]
		requestsMu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/start":
			page := fmt.Sprintf(`<!doctype html>
<html><head><title>Start</title></head><body>
<a id="next" href="/destination">Open destination</a>
<form id="greeting"><label for="name">Your name</label>
<input id="name" name="name" placeholder="Your name">
<button id="send" type="submit">Submit</button></form>
<p id="load">Start load %d</p>
<p id="result"></p>
<script>
document.getElementById("greeting").addEventListener("submit", function(event) {
  event.preventDefault();
  document.getElementById("result").textContent = "Hello " + document.getElementById("name").value;
});
</script>

</body></html>`, count)
			_, _ = w.Write([]byte(page))
		case "/destination":
			_, _ = fmt.Fprintf(w, `<!doctype html><html><body><h1 id="destination">Destination %d</h1></body></html>`, count)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	startURL := server.URL + "/start"
	destinationURL := server.URL + "/destination"
	presentsBeforeLoad := browser.window.Stats().Presents
	browser.navigateViaAddressBar(t, startURL)
	browser.waitFor(t, "start page and link accessibility", func() bool {
		_, link := findAXNode(browser.window.AccessibilityTree(), "next")
		return browser.activeURL() == startURL && link && axContainsLabel(browser.window.AccessibilityTree(), "Start load 1")
	})
	browser.waitFor(t, "start page frame presentation", func() bool {
		return browser.window.Stats().Presents > presentsBeforeLoad
	})

	browser.clickNode(t, "next")
	browser.waitFor(t, "link navigation to destination", func() bool {
		_, heading := findAXNode(browser.window.AccessibilityTree(), "destination")
		return browser.activeURL() == destinationURL && heading && axContainsLabel(browser.window.AccessibilityTree(), "Destination 1")
	})
	browser.waitFor(t, "back history availability", func() bool {
		active := browser.f.tabMgr.Active()
		return active != nil && active.History().CanBack()
	})

	browser.clickToolbarButton(t, toolbar.ButtonBack)
	browser.waitFor(t, "back navigation to start page", func() bool {
		_, link := findAXNode(browser.window.AccessibilityTree(), "next")
		return browser.activeURL() == startURL && link && axContainsLabel(browser.window.AccessibilityTree(), "Start load 2")
	})
	browser.clickToolbarButton(t, toolbar.ButtonForward)
	browser.waitFor(t, "forward navigation to destination", func() bool {
		_, heading := findAXNode(browser.window.AccessibilityTree(), "destination")
		return browser.activeURL() == destinationURL && heading && axContainsLabel(browser.window.AccessibilityTree(), "Destination 2")
	})
	browser.clickToolbarButton(t, toolbar.ButtonBack)
	browser.waitFor(t, "second back navigation to start page", func() bool {
		_, link := findAXNode(browser.window.AccessibilityTree(), "next")
		return browser.activeURL() == startURL && link && axContainsLabel(browser.window.AccessibilityTree(), "Start load 3")
	})

	startRequestsBeforeReload := requestCount(&requestsMu, requests, "/start")
	browser.clickToolbarButton(t, toolbar.ButtonReload)
	browser.waitFor(t, "reload of start page", func() bool {
		return browser.activeURL() == startURL && requestCount(&requestsMu, requests, "/start") > startRequestsBeforeReload && axContainsLabel(browser.window.AccessibilityTree(), "Start load 4")
	})
	browser.waitFor(t, "reloaded page form controls", func() bool {
		_, input := findAXNode(browser.window.AccessibilityTree(), "name")
		_, submit := findAXNode(browser.window.AccessibilityTree(), "send")
		return input && submit
	})

	browser.clickNode(t, "name")
	browser.waitFor(t, "name field focus", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "name")
		return ok && node.Focused
	})
	for _, key := range "Ada" {
		browser.pressKey(t, key)
	}
	presentsBeforeSubmit := browser.window.Stats().Presents
	browser.clickNode(t, "send")
	browser.waitFor(t, "JavaScript form feedback", func() bool {
		return axContainsLabel(browser.window.AccessibilityTree(), "Hello Ada")
	})
	browser.waitFor(t, "form feedback frame presentation", func() bool {
		return browser.window.Stats().Presents > presentsBeforeSubmit
	})
	browser.waitFor(t, "page tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})

	if got := requestCount(&requestsMu, requests, "/start"); got != 4 {
		t.Errorf("GET /start count = %d, want 4 (initial load, two traversals, and reload)", got)
	}
	if got := requestCount(&requestsMu, requests, "/destination"); got != 2 {
		t.Errorf("GET /destination count = %d, want 2 (link navigation and forward)", got)
	}
	if got := requestCount(&requestsMu, requests, "/submit"); got != 0 {
		t.Errorf("GET /submit count = %d, want 0 because JavaScript prevented native form navigation", got)
	}
	assertE2EPresentedPage(t, browser.window, 640, 480+totalChromeHeight)
}

func requestCount(mu *sync.Mutex, requests map[string]int, path string) int {
	mu.Lock()
	defer mu.Unlock()
	return requests[path]
}

func assertE2EPresentedPage(t *testing.T, window *headless.Window, wantWidth, wantHeight int) {
	t.Helper()
	if stats := window.Stats(); stats.Presents == 0 {
		t.Fatal("headless browser presented no frames")
	}
	path := filepath.Join(t.TempDir(), "browser-journey.png")
	if err := window.WritePNG(path); err != nil {
		t.Fatalf("write presented browser frame: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open presented browser frame: %v", err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode presented browser frame: %v", err)
	}
	if got := img.Bounds().Dx(); got != wantWidth {
		t.Errorf("presented frame width = %d, want %d", got, wantWidth)
	}
	if got := img.Bounds().Dy(); got != wantHeight {
		t.Errorf("presented frame height = %d, want %d", got, wantHeight)
	}
	contentTop := max(img.Bounds().Min.Y, totalChromeHeight)
	for y := contentTop; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || bl != 0xffff {
				return
			}
		}
	}
	t.Error("presented browser frame is blank white")
}
