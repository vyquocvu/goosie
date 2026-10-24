package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/session"
	"github.com/vyquocvu/goosie/internal/surface"
)

// TestSessionStateCapturesScroll verifies a saved snapshot carries each tab's
// scroll offset (Bug C4: restores always landed back at the top).
func TestSessionStateCapturesScroll(t *testing.T) {
	f, _ := newRestoreTestPath(t)
	active := f.tabMgr.Active()
	active.SetURL("https://a.example/")
	active.SetScrollY(320)
	other := f.tabMgr.NewTab()
	other.SetURL("https://b.example/")
	other.SetScrollY(640)

	st := sessionState(f)
	if len(st.Tabs) != 2 {
		t.Fatalf("saved tabs = %d, want 2", len(st.Tabs))
	}
	if st.Tabs[0].ScrollY != 320 {
		t.Errorf("tab 0 ScrollY = %d, want 320", st.Tabs[0].ScrollY)
	}
	if st.Tabs[1].ScrollY != 640 {
		t.Errorf("tab 1 ScrollY = %d, want 640", st.Tabs[1].ScrollY)
	}
}

// TestApplyTabStateRestoresScroll verifies a restored tab reopens at its saved
// offset instead of the top of the page.
func TestApplyTabStateRestoresScroll(t *testing.T) {
	f, _ := newRestoreTestPath(t)
	applyTabState(f.tabMgr.Active(), session.TabState{
		URL: "https://a.example/", Title: "A", ScrollY: 320,
	})
	if got := f.tabMgr.Active().ScrollY(); got != 320 {
		t.Errorf("restored ScrollY = %d, want 320", got)
	}
}

// TestSessionScrollSurvivesSaveRestoreRoundTrip drives the full journal path:
// save, reload from disk, restore into a fresh tab set.
func TestSessionScrollSurvivesSaveRestoreRoundTrip(t *testing.T) {
	f, dir := newRestoreTestPath(t)
	active := f.tabMgr.Active()
	active.SetURL("https://a.example/")
	active.SetScrollY(320)

	path := filepath.Join(dir, "session.json")
	if err := session.Save(path, sessionState(f)); err != nil {
		t.Fatalf("session.Save: %v", err)
	}
	loaded, err := session.Load(path)
	if err != nil {
		t.Fatalf("session.Load: %v", err)
	}

	dst, _ := newRestoreTestPath(t)
	dst.applySessionState(loaded)
	if got := dst.tabMgr.Active().ScrollY(); got != 320 {
		t.Errorf("round-trip ScrollY = %d, want 320", got)
	}
	// The viewport itself is moved when the restore's load lands; see
	// TestRestoredScrollSurvivesNavigationReset.
}

// TestRestoredScrollSurvivesNavigationReset verifies the session restore's own
// load (fresh navigation semantics) re-applies the saved offset after the
// top-of-page reset rather than losing it.
func TestRestoredScrollSurvivesNavigationReset(t *testing.T) {
	f := newNavTestPath(t)
	f.config.width, f.config.height, f.config.dpr = 800, 600, 1.0
	tab := f.tabMgr.Active()
	tab.SetURL("https://a.example/")
	tab.SetScrollY(320)
	f.markRestoredScroll(tab)

	f.applyNavResult(navResult{tabID: tab.ID(), serial: tab.Nav.Serial, url: "https://a.example/"})

	if got := tab.ScrollY(); got != 320 {
		t.Errorf("ScrollY after restored navigation = %d, want 320", got)
	}
	// The intent is one-shot and consumed: a later navigation opens at the top.
	f.applyNavResult(navResult{tabID: tab.ID(), serial: tab.Nav.Serial, url: "https://b.example/"})
	if got := tab.ScrollY(); got != 0 {
		t.Errorf("ScrollY after a later navigation = %d, want 0", got)
	}
}

// TestLegacyJournalWithoutScrollRestoresAtTop keeps forward compatibility: a
// journal written before scroll persistence (no scroll keys) restores at the
// top instead of erroring.
func TestLegacyJournalWithoutScrollRestoresAtTop(t *testing.T) {
	f, _ := newRestoreTestPath(t)
	activeFile := f.config.sessionActiveFile
	legacy := `{"tabs":[{"url":"https://legacy.example/","title":"Legacy"}],"active":0}`
	if err := os.WriteFile(activeFile, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy journal: %v", err)
	}
	loaded, err := session.Load(activeFile)
	if err != nil {
		t.Fatalf("session.Load legacy journal: %v", err)
	}
	f.applySessionState(loaded)
	tab := f.tabMgr.Active()
	if tab.URL() != "https://legacy.example/" {
		t.Errorf("restored URL = %q, want https://legacy.example/", tab.URL())
	}
	if tab.ScrollY() != 0 {
		t.Errorf("legacy restore ScrollY = %d, want 0", tab.ScrollY())
	}
}

// TestRestoreCrashedSessionKeepsScroll verifies the crash journal path: the
// active crashed tab re-opens at its recorded offset once its load lands.
func TestRestoreCrashedSessionKeepsScroll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// A page taller than the 600px viewport, so the restored 240px offset
		// survives the scheduler's clamp instead of being legitimately zeroed.
		fmt.Fprint(w, "<!DOCTYPE html><html><body>"+strings.Repeat("<p>row</p>", 400)+"</body></html>")
	}))
	defer server.Close()

	f, _ := newRestoreTestPath(t)
	f.config.width, f.config.height, f.config.dpr = 800, 600, 1.0
	f.runCtx, f.runCancel = context.WithCancel(context.Background())
	defer f.runCancel()
	f.client = net.DefaultClient()
	defer f.client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	f.fonts = fonts

	crashedState := session.State{
		Tabs: []session.TabState{
			{URL: server.URL + "/page", Title: "Crashed", ScrollY: 240},
		},
		Active: 0,
	}
	f.tabMgr.Active().SetURL("goosie://restore")
	f.crashedSession = &crashedState

	f.restoreCrashedSession(crashedState)

	tab := f.tabMgr.Active()
	if tab.URL() != server.URL+"/page" {
		t.Fatalf("active URL = %q, want %s/page", tab.URL(), server.URL)
	}
	// The reload is in flight; when it lands the restore offset must survive.
	select {
	case res := <-f.navResults:
		f.applyNavResult(res)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the restore navigation")
	}
	if got := tab.ScrollY(); got != 240 {
		t.Errorf("restored crashed ScrollY = %d, want 240", got)
	}
	// Drain the published plan: the viewport resolves against the layer a
	// frame owns, so the restored page's bounds only bind after BeginFrame.
	f.sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
	if got := f.sched.Viewport().Offset.Y; got != 240 {
		t.Errorf("restored crashed viewport offset = %d, want 240", got)
	}
}
