package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// TestJourneyFindInPageCountsStepsScrollsAndEscape drives the whole Cmd+F
// journey through the real window wiring: a count appears without another
// keystroke on reopen, Enter moves to the next hit and scrolls the document,
// Escape closes find mode from wherever focus sits.
func TestJourneyFindInPageCountsStepsScrollsAndEscape(t *testing.T) {
	var page strings.Builder
	page.WriteString("<!doctype html><html><body style=\"margin:0;font-size:16px;\">")
	page.WriteString("<p id=\"first\">needle alpha</p>")
	for i := 0; i < 60; i++ {
		page.WriteString("<p>filler filler filler filler filler</p>")
	}
	page.WriteString("<p id=\"second\">needle omega</p>")
	page.WriteString("</body></html>")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page.String()))
	}))
	defer srv.Close()

	b := newBrowserE2E(t)
	b.navigateViaAddressBar(t, srv.URL)
	b.waitFor(t, "page loaded", func() bool {
		tab := b.f.tabMgr.Active()
		return b.activeURL() == srv.URL && tab != nil && tab.Session() != nil
	})

	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 'f', Mods: surface.ModCommand})
	b.waitFor(t, "find opens", func() bool {
		active, _, _ := b.f.toolbar.FindCounts()
		return active
	})
	for _, key := range "needle" {
		b.pressKey(t, key)
	}
	b.waitFor(t, "two matches", func() bool {
		active, total, _ := b.f.toolbar.FindCounts()
		return active && total == 2
	})
	if got := b.f.tabMgr.Active().ScrollY(); got != 0 {
		t.Fatalf("first match scrolled to %d, want the top of the document", got)
	}

	b.pressKey(t, '\r')
	b.waitFor(t, "second match selected", func() bool {
		_, _, idx := b.f.toolbar.FindCounts()
		return idx == 1
	})
	b.waitFor(t, "document scrolled to the second match", func() bool {
		return b.f.tabMgr.Active().ScrollY() > 0
	})

	// Escape closes find mode and drops the highlights.
	b.pressKey(t, 0x1b)
	b.waitFor(t, "Escape closes find", func() bool {
		active, _, _ := b.f.toolbar.FindCounts()
		return !active
	})

	// Reopening restores the query and its count without another keystroke.
	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 'f', Mods: surface.ModCommand})
	b.waitFor(t, "reopened find shows the restored count", func() bool {
		active, total, _ := b.f.toolbar.FindCounts()
		return active && total == 2
	})
	if focus := b.f.toolbar.GetFocus(); focus != toolbar.FocusAddress {
		t.Fatalf("reopened find focus = %v, want FocusAddress", focus)
	}
}
