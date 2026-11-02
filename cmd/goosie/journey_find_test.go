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

func findJourneyPage(t *testing.T, body, script string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><html><body style=\"margin:0;font-size:16px;\">" + body + script + "</body></html>"))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func openFindWithQuery(t *testing.T, b *browserE2E, query string) {
	t.Helper()
	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 'f', Mods: surface.ModCommand})
	b.waitFor(t, "find opens", func() bool {
		active, _, _ := b.f.toolbar.FindCounts()
		return active
	})
	for _, key := range query {
		b.pressKey(t, key)
	}
}

// qc:BR-135
//
// Cmd+G steps to the next match and Cmd+Shift+G to the previous one, whether
// find is open or the shortcut has to reopen it with the restored query. Both
// directions wrap at the ends of the result set.
func TestJourneyFindShortcutsStepAndReopen(t *testing.T) {
	var body strings.Builder
	body.WriteString("<p>needle one</p>")
	for i := 0; i < 40; i++ {
		body.WriteString("<p>filler filler filler filler</p>")
	}
	body.WriteString("<p>needle two</p><p>needle three</p>")
	srvURL := findJourneyPage(t, body.String(), "")

	b := newBrowserE2E(t)
	b.navigateViaAddressBar(t, srvURL)
	b.waitFor(t, "page loaded", func() bool {
		tab := b.f.tabMgr.Active()
		return b.activeURL() == srvURL && tab != nil && tab.Session() != nil
	})

	openFindWithQuery(t, b, "needle")
	b.waitFor(t, "three matches", func() bool {
		active, total, idx := b.f.toolbar.FindCounts()
		return active && total == 3 && idx == 0
	})

	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 'g', Mods: surface.ModCommand})
	b.waitFor(t, "Cmd+G selects the next match", func() bool {
		_, total, idx := b.f.toolbar.FindCounts()
		return total == 3 && idx == 1
	})

	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 'G', Mods: surface.ModCommand | surface.ModShift})
	b.waitFor(t, "Cmd+Shift+G selects the previous match", func() bool {
		_, total, idx := b.f.toolbar.FindCounts()
		return total == 3 && idx == 0
	})

	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 'G', Mods: surface.ModCommand | surface.ModShift})
	b.waitFor(t, "Cmd+Shift+G wraps backward to the last match", func() bool {
		_, total, idx := b.f.toolbar.FindCounts()
		return total == 3 && idx == 2
	})

	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 'g', Mods: surface.ModCommand})
	b.waitFor(t, "Cmd+G wraps forward to the first match", func() bool {
		_, total, idx := b.f.toolbar.FindCounts()
		return total == 3 && idx == 0
	})

	b.pressKey(t, 0x1b)
	b.waitFor(t, "Escape closes find", func() bool {
		active, _, _ := b.f.toolbar.FindCounts()
		return !active
	})

	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 'g', Mods: surface.ModCommand})
	b.waitFor(t, "Cmd+G reopens find at the first match", func() bool {
		active, total, idx := b.f.toolbar.FindCounts()
		return active && total == 3 && idx == 0
	})

	b.pressKey(t, 0x1b)
	b.waitFor(t, "Escape closes find again", func() bool {
		active, _, _ := b.f.toolbar.FindCounts()
		return !active
	})

	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 'G', Mods: surface.ModCommand | surface.ModShift})
	b.waitFor(t, "Cmd+Shift+G reopens find at the last match", func() bool {
		active, total, idx := b.f.toolbar.FindCounts()
		return active && total == 3 && idx == 2
	})
}

// A find that is already open must follow the document: nodes added or removed
// by a page script update the count and keep the highlight on a live match.
func TestJourneyFindTracksDOMMutations(t *testing.T) {
	script := `<script>
setTimeout(function () {
  var p = document.createElement('p');
  p.textContent = 'needle beta';
  document.body.appendChild(p);
}, 800);
setTimeout(function () {
  var first = document.getElementById('first');
  first.parentNode.removeChild(first);
}, 1800);
</script>`
	srvURL := findJourneyPage(t, `<p id="first">needle alpha</p>`, script)

	b := newBrowserE2E(t)
	b.navigateViaAddressBar(t, srvURL)
	b.waitFor(t, "page loaded", func() bool {
		tab := b.f.tabMgr.Active()
		return b.activeURL() == srvURL && tab != nil && tab.Session() != nil
	})

	openFindWithQuery(t, b, "needle")
	b.waitFor(t, "one match before the script runs", func() bool {
		active, total, _ := b.f.toolbar.FindCounts()
		return active && total == 1
	})

	b.waitFor(t, "script-added match appears in the count", func() bool {
		_, total, _ := b.f.toolbar.FindCounts()
		return total == 2
	})

	b.waitFor(t, "script-removed match disappears from the count", func() bool {
		_, total, _ := b.f.toolbar.FindCounts()
		return total == 1
	})
}
