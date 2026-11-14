package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func longScrollPage(id string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html><head><title>%s</title></head><body><h1 id="%s">%s</h1>`, id, id, id)
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, `<p>%s row %d</p>`, id, i)
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

func (b *browserE2E) scrollContentBy(t *testing.T, deltaY int32) int32 {
	t.Helper()
	b.inject(t, surface.Event{
		Kind:  surface.EvScroll,
		Delta: frame.Point{Y: deltaY},
		Pos:   frame.Point{X: 300, Y: 300},
	})
	var y int32
	b.waitFor(t, "the document to scroll", func() bool {
		y = b.f.sched.Viewport().Offset.Y
		return y > 0
	})
	return y
}

func assertRestoredOffset(t *testing.T, b *browserE2E, want int32, what string) {
	t.Helper()
	b.waitFor(t, what+" restores its offset", func() bool {
		return b.f.sched.Viewport().Offset.Y == want
	})
	if got := b.f.tabMgr.Active().ScrollY(); got != want {
		t.Errorf("%s: tab scroll = %d, want %d", what, got, want)
	}
	start := b.window.Stats().Presents
	b.waitFor(t, what+" to settle on idle frames", func() bool {
		return b.window.Stats().Presents >= start+3
	})
	if got := b.f.sched.Viewport().Offset.Y; got != want {
		t.Errorf("%s: offset drifted to %d after idle frames, want %d", what, got, want)
	}
}

// qc:BR-086
func TestJourneyBackForwardRestoresPerEntryScroll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/long-a":
			fmt.Fprint(w, longScrollPage("long-a"))
		case "/long-b":
			fmt.Fprint(w, longScrollPage("long-b"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	aURL := server.URL + "/long-a"
	bURL := server.URL + "/long-b"

	browser.navigateViaAddressBar(t, aURL)
	browser.waitFor(t, "page A loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "long-a")
		return ok && browser.activeURL() == aURL
	})
	offsetA := browser.scrollContentBy(t, 1200)

	browser.navigateViaAddressBar(t, bURL)
	browser.waitFor(t, "page B loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "long-b")
		return ok && browser.activeURL() == bURL
	})
	if got := browser.f.sched.Viewport().Offset.Y; got != 0 {
		t.Fatalf("fresh navigation to B opened at offset %d, want 0", got)
	}
	offsetB := browser.scrollContentBy(t, 600)
	if offsetA == offsetB {
		t.Fatalf("both pages recorded the same offset %d; the test cannot tell them apart", offsetA)
	}

	browser.clickToolbarButton(t, toolbar.ButtonBack)
	browser.waitFor(t, "back to page A", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "long-a")
		return ok && browser.activeURL() == aURL
	})
	assertRestoredOffset(t, browser, offsetA, "Back to A")

	browser.clickToolbarButton(t, toolbar.ButtonForward)
	browser.waitFor(t, "forward to page B", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "long-b")
		return ok && browser.activeURL() == bURL
	})
	assertRestoredOffset(t, browser, offsetB, "Forward to B")

	browser.clickToolbarButton(t, toolbar.ButtonBack)
	browser.waitFor(t, "back to page A again", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "long-a")
		return ok && browser.activeURL() == aURL
	})
	assertRestoredOffset(t, browser, offsetA, "second Back to A")

	entries, current := browser.f.tabMgr.Active().History().Entries()
	if len(entries) != 2 || current != 0 {
		t.Errorf("history entries = %v (current = %d), want [A B] at index 0", entries, current)
	}
}
