package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/session"
	"github.com/vyquocvu/goosie/internal/tabs"
)

// newRestoreTestPath builds an isolated framePath configured with a temporary
// state directory and session active journal file for crash recovery testing.
func newRestoreTestPath(t *testing.T) (*framePath, string) {
	t.Helper()
	dir := t.TempDir()
	spec := paint.SceneSpec{DocHeight: 256}
	_, layer := paint.BuildLayer(spec)
	pool := raster.New(1, 4, func(j raster.Job) error { return nil })
	pool.Start(context.Background())
	t.Cleanup(func() { _ = pool.Close() })
	sched := raster.NewScheduler(layer, pool, frame.Viewport{Size: frame.Size{W: 256, H: 256}}, 1, raster.Pref{})

	activeFile := filepath.Join(dir, "session_active.json")
	permFile := filepath.Join(dir, "session.json")

	f := &framePath{
		sched:      sched,
		pool:       pool,
		navResults: make(chan navResult, 16),
		imgResults: make(chan imgResult, 16),
		zoom:       1.0,
		config: config{
			sessionFile:       permFile,
			sessionActiveFile: activeFile,
		},
	}
	f.tabMgr = tabs.NewManager(nil)
	firstTab := f.tabMgr.NewTab()
	firstTab.SetLayer(layer)
	return f, dir
}

// TestCrashRecovery_DetectsCrashedJournal verifies that a pre-existing active session journal
// on startup is detected as an abnormal termination, loading crashedSession and setting the initial URL to goosie://restore.
func TestCrashRecovery_DetectsCrashedJournal(t *testing.T) {
	dir := t.TempDir()
	activeFile := filepath.Join(dir, "session_active.json")
	permFile := filepath.Join(dir, "session.json")

	// Simulate abnormal shutdown: session_active.json exists with 2 tabs
	crashedState := session.State{
		Tabs: []session.TabState{
			{URL: "https://example.com/one", Title: "Tab One"},
			{URL: "https://example.com/two", Title: "Tab Two"},
		},
		Active: 1,
	}
	if err := session.Save(activeFile, crashedState); err != nil {
		t.Fatalf("session.Save crashed state: %v", err)
	}

	c := config{
		sessionFile:       permFile,
		sessionActiveFile: activeFile,
		width:             800,
		height:            600,
		dpr:               1.0,
	}

	// Replicate startup logic
	f, _ := newRestoreTestPath(t)
	f.config = c

	if info, err := os.Stat(c.sessionActiveFile); err == nil && !info.IsDir() {
		if st, err := session.Load(c.sessionActiveFile); err == nil && len(st.Tabs) > 0 {
			f.crashedSession = &st
		}
	}

	if f.crashedSession == nil {
		t.Fatal("expected crashedSession to be detected and loaded")
	}
	if len(f.crashedSession.Tabs) != 2 {
		t.Fatalf("crashedSession tabs = %d, want 2", len(f.crashedSession.Tabs))
	}
	if f.crashedSession.Tabs[1].URL != "https://example.com/two" {
		t.Errorf("crashed tab 1 URL = %q, want https://example.com/two", f.crashedSession.Tabs[1].URL)
	}

	firstTab := f.tabMgr.Active()
	if c.url != "" {
		firstTab.SetURL(normalizeURL(c.url))
	} else if f.crashedSession != nil {
		firstTab.SetURL("goosie://restore")
		firstTab.SetTitle("Restore Session - Goosie")
		f.restoredActiveURL = "goosie://restore"
	}

	if firstTab.URL() != "goosie://restore" {
		t.Errorf("firstTab URL = %q, want goosie://restore", firstTab.URL())
	}
	if firstTab.Title() != "Restore Session - Goosie" {
		t.Errorf("firstTab Title = %q, want Restore Session - Goosie", firstTab.Title())
	}
}

// TestCrashRecovery_InternalPageRender verifies HTML output for goosie://restore
// when crashed tabs exist and when no crashed session is present.
func TestCrashRecovery_InternalPageRender(t *testing.T) {
	f, _ := newRestoreTestPath(t)

	// Case 1: No crashed session
	f.crashedSession = nil
	title, htmlContent := buildRestorePageHTML(f, "goosie://restore")
	if title != "Session Restore - Goosie" {
		t.Errorf("title = %q, want Session Restore - Goosie", title)
	}
	if !strings.Contains(htmlContent, "No crashed session was detected") {
		t.Errorf("expected empty-state message in HTML, got:\n%s", htmlContent)
	}

	// Case 2: Crashed session present with 2 tabs
	f.crashedSession = &session.State{
		Tabs: []session.TabState{
			{URL: "https://golang.org", Title: "Go Programming Language"},
			{URL: "https://news.ycombinator.com", Title: "Hacker News"},
		},
		Active: 0,
	}

	title, htmlContent = buildRestorePageHTML(f, "goosie://restore")
	if title != "Restore Session - Goosie" {
		t.Errorf("title = %q, want Restore Session - Goosie", title)
	}
	if !strings.Contains(htmlContent, "Goosie did not shut down cleanly") {
		t.Error("expected crash warning header in HTML")
	}
	if !strings.Contains(htmlContent, "Restore 2 Tabs") {
		t.Error("expected 'Restore 2 Tabs' action link in HTML")
	}
	if !strings.Contains(htmlContent, "Don't Restore") {
		t.Error("expected 'Don't Restore' action link in HTML")
	}
	if !strings.Contains(htmlContent, "Go Programming Language") || !strings.Contains(htmlContent, "https://golang.org") {
		t.Error("expected first tab title/URL in tabs list")
	}
	if !strings.Contains(htmlContent, "Hacker News") || !strings.Contains(htmlContent, "https://news.ycombinator.com") {
		t.Error("expected second tab title/URL in tabs list")
	}
	if !strings.Contains(htmlContent, "Active") {
		t.Error("expected active badge on the currently active crashed tab")
	}
}

// TestCrashRecovery_ActionRestore verifies that clicking 'Restore' applies the crashed tabs,
// clears the crashed state, and deletes session_active.json.
func TestCrashRecovery_ActionRestore(t *testing.T) {
	f, _ := newRestoreTestPath(t)
	activeFile := f.config.sessionActiveFile

	crashedState := session.State{
		Tabs: []session.TabState{
			{URL: "https://docs.goosie.dev/start", Title: "Getting Started"},
			{URL: "https://docs.goosie.dev/api", Title: "API Reference"},
		},
		Active: 1,
	}
	if err := session.Save(activeFile, crashedState); err != nil {
		t.Fatalf("session.Save: %v", err)
	}

	// Active tab currently shows goosie://restore
	f.tabMgr.Active().SetURL("goosie://restore")
	f.tabMgr.Active().SetTitle("Restore Session - Goosie")
	f.crashedSession = &crashedState

	title, htmlContent := buildRestorePageHTML(f, "goosie://restore?action=restore")
	if title != "Session Restored - Goosie" {
		t.Errorf("title = %q, want Session Restored - Goosie", title)
	}
	if !strings.Contains(htmlContent, "Successfully restored 2 tabs") {
		t.Errorf("HTML missing restored confirmation: %s", htmlContent)
	}

	// Verify crashedSession is cleared
	if f.crashedSession != nil {
		t.Error("expected crashedSession to be nil after restore")
	}

	// Verify session_active.json now journals the restored session state
	loaded, err := session.Load(activeFile)
	if err != nil {
		t.Fatalf("failed to load active journal after restore: %v", err)
	}
	if len(loaded.Tabs) != 2 || loaded.Tabs[0].URL != "https://docs.goosie.dev/start" {
		t.Errorf("active journal state after restore = %+v, want 2 restored tabs", loaded.Tabs)
	}

	// Verify tab manager now has 2 restored tabs
	if count := f.tabMgr.Count(); count != 2 {
		t.Fatalf("tabMgr.Count() = %d, want 2", count)
	}
	all := f.tabMgr.Tabs()
	if all[0].URL() != "https://docs.goosie.dev/start" {
		t.Errorf("tab 0 URL = %q, want https://docs.goosie.dev/start", all[0].URL())
	}
	if all[1].URL() != "https://docs.goosie.dev/api" {
		t.Errorf("tab 1 URL = %q, want https://docs.goosie.dev/api", all[1].URL())
	}
	if f.tabMgr.Active() != all[1] {
		t.Errorf("active tab = %v, want tab 1", f.tabMgr.Active())
	}
}

// TestCrashRecovery_ActionDismiss verifies that clicking 'Don't Restore' clears
// the crashed state and removes the active journal without modifying current tabs.
func TestCrashRecovery_ActionDismiss(t *testing.T) {
	f, _ := newRestoreTestPath(t)
	activeFile := f.config.sessionActiveFile

	crashedState := session.State{
		Tabs: []session.TabState{
			{URL: "https://site.example/page", Title: "Old Page"},
		},
		Active: 0,
	}
	if err := session.Save(activeFile, crashedState); err != nil {
		t.Fatalf("session.Save: %v", err)
	}

	f.crashedSession = &crashedState
	title, htmlContent := buildRestorePageHTML(f, "goosie://restore?action=dismiss")
	if title != "Session Dismissed - Goosie" {
		t.Errorf("title = %q, want Session Dismissed - Goosie", title)
	}
	if !strings.Contains(htmlContent, "Previous session data has been discarded") {
		t.Errorf("HTML missing dismissed confirmation: %s", htmlContent)
	}

	if f.crashedSession != nil {
		t.Error("expected crashedSession to be nil after dismiss")
	}
	if _, err := os.Stat(activeFile); !os.IsNotExist(err) {
		t.Error("expected session_active.json to be deleted after dismiss")
	}
}

// TestCrashRecovery_SaveActiveSessionJournal verifies that saveActiveSession persists
// open tabs, but does not overwrite the journal while the restore prompt is pending.
func TestCrashRecovery_SaveActiveSessionJournal(t *testing.T) {
	f, _ := newRestoreTestPath(t)
	activeFile := f.config.sessionActiveFile

	// 1. Set open tab
	tab := f.tabMgr.Active()
	tab.SetURL("https://active.example/test")
	tab.SetTitle("Active Test")

	f.saveActiveSession()

	loaded, err := session.Load(activeFile)
	if err != nil {
		t.Fatalf("session.Load: %v", err)
	}
	if len(loaded.Tabs) != 1 || loaded.Tabs[0].URL != "https://active.example/test" {
		t.Fatalf("loaded tabs = %+v, want 1 tab with https://active.example/test", loaded.Tabs)
	}

	// 2. While crashedSession is pending, saveActiveSession must NOT overwrite
	f.crashedSession = &session.State{
		Tabs: []session.TabState{{URL: "https://crashed.example/"}},
	}
	tab.SetURL("https://changed.example/")
	f.saveActiveSession()

	// Should still have original saved tabs on disk
	loaded2, err := session.Load(activeFile)
	if err != nil {
		t.Fatalf("session.Load: %v", err)
	}
	if loaded2.Tabs[0].URL != "https://active.example/test" {
		t.Errorf("journal was overwritten while crashedSession was active: got %q", loaded2.Tabs[0].URL)
	}
}

// TestCrashRecovery_ClearDataRemovesActiveJournal verifies that clearBrowsingData removes
// session_active.json along with cookies, history, and regular session.
func TestCrashRecovery_ClearDataRemovesActiveJournal(t *testing.T) {
	dir := t.TempDir()
	c := config{
		cookieFile:        filepath.Join(dir, "cookies.json"),
		historyFile:       filepath.Join(dir, "history.json"),
		sessionFile:       filepath.Join(dir, "session.json"),
		sessionActiveFile: filepath.Join(dir, "session_active.json"),
		bookmarkFile:      filepath.Join(dir, "bookmarks.json"),
	}

	_ = os.WriteFile(c.cookieFile, []byte("[]"), 0o644)
	_ = os.WriteFile(c.historyFile, []byte("[]"), 0o644)
	_ = os.WriteFile(c.sessionFile, []byte(`{"tabs":[]}`), 0o644)
	_ = os.WriteFile(c.sessionActiveFile, []byte(`{"tabs":[]}`), 0o644)
	_ = os.WriteFile(c.bookmarkFile, []byte("[]"), 0o644)

	removed, err := c.clearBrowsingData()
	if err != nil {
		t.Fatalf("clearBrowsingData: %v", err)
	}
	if len(removed) != 4 {
		t.Fatalf("removed %d files, want 4 (cookies, history, session, session_active)", len(removed))
	}
	if _, err := os.Stat(c.sessionActiveFile); !os.IsNotExist(err) {
		t.Error("session_active.json still exists after clearBrowsingData")
	}
	if _, err := os.Stat(c.bookmarkFile); err != nil {
		t.Error("bookmarkFile was erroneously removed")
	}
}
