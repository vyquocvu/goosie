package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/surface"
)

func testManagerConfig(t *testing.T) config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	c, err := parse([]string{
		"-backend", platform.Headless,
		"-width", "320", "-height", "240", "-dpr", "1",
		"-download-dir", home,
	})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return c
}

func waitWindows(t *testing.T, m *windowManager, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := len(m.windows)
		m.mu.Unlock()
		if n == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	m.mu.Lock()
	n := len(m.windows)
	m.mu.Unlock()
	t.Fatalf("window count = %d, want %d", n, want)
}

func TestManagerSpawnTwoWindows(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	if f1 == f2 {
		t.Fatal("spawn returned the same framePath twice")
	}
	if f1.window == f2.window {
		t.Fatal("two windows share one platform window")
	}
	waitWindows(t, m, 2)
	if f1.loop == nil || f2.loop == nil {
		t.Fatal("a spawned window has no loop")
	}
}

func TestManagerCloseOneKeepsOthers(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	waitWindows(t, m, 2)

	m.closeWindow(f1)
	waitWindows(t, m, 1)

	select {
	case <-m.done:
		t.Fatal("manager signalled done while a window is still open")
	default:
	}
	m.mu.Lock()
	stillThere := len(m.windows) == 1 && m.windows[0] == f2
	m.mu.Unlock()
	if !stillThere {
		t.Fatal("closing window 1 removed window 2")
	}
}

func TestManagerQuitOnLastWindow(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	waitWindows(t, m, 2)

	m.closeWindow(f1)
	waitWindows(t, m, 1)
	m.closeWindow(f2)

	select {
	case <-m.done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager never signalled done after the last window closed")
	}
	waitWindows(t, m, 0)
}

func TestManagerRunSavesAndTearsDown(t *testing.T) {
	c := testManagerConfig(t)
	m, err := newWindowManager(c)
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	if m.profile.bookmarks != nil {
		m.profile.bookmarks.Toggle("https://example.test/", "Example")
	}

	runErr := make(chan error, 1)
	go func() { runErr <- m.run() }()

	var primary *framePath
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		primary = m.primary
		m.mu.Unlock()
		if primary != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if primary == nil {
		t.Fatal("run never spawned a primary window")
	}
	m.closeWindow(primary)

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after the window closed")
	}

	if _, err := os.Stat(c.sessionFile); err != nil {
		t.Fatalf("session file not saved on clean exit: %v", err)
	}
	if _, err := os.Stat(c.sessionActiveFile); !os.IsNotExist(err) {
		t.Fatalf("crash journal not removed on clean exit: %v", err)
	}
}

func TestManagerLastTabClosesWindow(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	waitWindows(t, m, 2)

	tabs1 := f1.tabMgr.Tabs()
	if len(tabs1) != 1 {
		t.Fatalf("f1 tab count = %d, want 1", len(tabs1))
	}
	f1.closeTab(tabs1[0].ID())

	waitWindows(t, m, 1)
	m.mu.Lock()
	kept := len(m.windows) == 1 && m.windows[0] == f2
	m.mu.Unlock()
	if !kept {
		t.Fatal("closing window 1's last tab did not leave window 2 registered")
	}
	select {
	case <-m.done:
		t.Fatal("manager quit while window 2 was still open")
	default:
	}
}

func TestManagerSharesProfileStores(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}

	if f1.bookmarks != f2.bookmarks {
		t.Error("windows do not share one bookmark store")
	}
	if f1.history != f2.history {
		t.Error("windows do not share one history store")
	}
	if f1.storage != f2.storage {
		t.Error("windows do not share one localStorage registry")
	}
	if f1.client != f2.client {
		t.Error("windows do not share one HTTP client")
	}
	if f1.bookmarks == nil {
		t.Fatal("bookmark store is nil; a non-private config with a writable HOME must load one")
	}
	f1.bookmarks.Toggle("https://shared.test/", "Shared")
	if !f2.bookmarks.Contains("https://shared.test/") {
		t.Error("a bookmark added in window 1 is not visible in window 2")
	}
}

func TestManagerPerWindowTabIsolation(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}

	if f1.tabMgr == f2.tabMgr {
		t.Fatal("windows share one tab manager")
	}
	before := f2.tabMgr.Count()
	f1.newTab()
	f1.newTab()
	if f1.tabMgr.Count() != before+2 {
		t.Fatalf("f1 tab count = %d, want %d", f1.tabMgr.Count(), before+2)
	}
	if f2.tabMgr.Count() != before {
		t.Fatalf("f2 tab count changed to %d; windows are not isolated", f2.tabMgr.Count())
	}
}

func TestDrainLoopErrsCollectsAll(t *testing.T) {
	first := make(chan error, 1)
	first <- nil
	later := make(chan error, 1)
	later <- errors.New("window 2 loop failed")
	last := make(chan error, 1)
	last <- nil
	got := drainLoopErrs([]<-chan error{first, later, last})
	if got == nil || got.Error() != "window 2 loop failed" {
		t.Fatalf("drainLoopErrs = %v, want window 2 loop failed", got)
	}
}

func TestManagerSpawnInstallsNewWindow(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	waitWindows(t, m, 1)
	if f1.newWindow == nil {
		t.Fatal("spawn did not install a newWindow hook")
	}
	f1.newWindow()
	waitWindows(t, m, 2)
}

func TestManagerCollectsLaterWindowError(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	runErr := make(chan error, 1)
	go func() { runErr <- m.run() }()

	var primary *framePath
	deadline := time.Now().Add(3 * time.Second)
	presented := false
	for time.Now().Before(deadline) {
		m.mu.Lock()
		primary = m.primary
		m.mu.Unlock()
		if primary != nil && primary.loop != nil && primary.loop.Stats().Frames > 0 {
			presented = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !presented {
		t.Fatal("primary window never presented a frame")
	}

	laterErr := make(chan error, 1)
	laterErr <- errors.New("later window loop failed")
	m.mu.Lock()
	m.errs = append(m.errs, laterErr)
	m.mu.Unlock()

	m.closeWindow(primary)

	select {
	case err := <-runErr:
		if err == nil || !strings.Contains(err.Error(), "later window loop failed") {
			t.Fatalf("run() = %v, want the later window's loop error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after the primary closed")
	}
}

func TestManagerCmdNSpawnsWindow(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitWindows(t, m, 1)
	cw, ok := f1.window.(*chromeWindow)
	if !ok {
		t.Fatal("primary window is not a chromeWindow")
	}
	tabsBefore := f1.tabMgr.Count()
	if !cw.handleTabShortcut('n', surface.ModCommand) {
		t.Fatal("Cmd+N was not handled")
	}
	waitWindows(t, m, 2)
	if f1.tabMgr.Count() != tabsBefore {
		t.Errorf("window 1 tab count = %d, want %d (Cmd+N must not touch window 1's tabs)",
			f1.tabMgr.Count(), tabsBefore)
	}
}

func TestManagerNewWindowMenuAction(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitWindows(t, m, 1)
	cw, ok := f1.window.(*chromeWindow)
	if !ok {
		t.Fatal("primary window is not a chromeWindow")
	}
	cw.openMainMenu()
	snap := cw.mainMenu.Snapshot()
	found := false
	for _, it := range snap.Items {
		if it.Action == "new_window" {
			found = true
			if it.Label != "New Window" {
				t.Errorf("new_window item label = %q, want New Window", it.Label)
			}
		}
	}
	if !found {
		t.Fatal("main menu has no New Window item")
	}
	f1.handleContextMenuAction("new_window", "")
	waitWindows(t, m, 2)
}

func TestManagerCmdShiftNStillIncognito(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitWindows(t, m, 1)
	cw, ok := f1.window.(*chromeWindow)
	if !ok {
		t.Fatal("primary window is not a chromeWindow")
	}
	before := f1.tabMgr.Count()
	if !cw.handleTabShortcut('n', surface.ModCommand|surface.ModShift) {
		t.Fatal("Cmd+Shift+N was not handled")
	}
	m.mu.Lock()
	n := len(m.windows)
	m.mu.Unlock()
	if n != 1 {
		t.Fatalf("window count = %d, want 1 (Cmd+Shift+N opens a tab, not a window)", n)
	}
	if f1.tabMgr.Count() != before+1 {
		t.Fatalf("tab count = %d, want %d", f1.tabMgr.Count(), before+1)
	}
	if active := f1.tabMgr.Active(); active == nil || !active.Incognito() {
		t.Error("Cmd+Shift+N did not open an incognito tab")
	}
}

func TestManagerNewWindowNilHookNoop(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitWindows(t, m, 1)
	cw, ok := f1.window.(*chromeWindow)
	if !ok {
		t.Fatal("primary window is not a chromeWindow")
	}
	f1.newWindow = nil
	cw.onNewWindow = nil
	if cw.handleTabShortcut('n', surface.ModCommand) {
		t.Error("Cmd+N was handled with a nil onNewWindow hook")
	}
	f1.handleContextMenuAction("new_window", "")
	m.mu.Lock()
	n := len(m.windows)
	m.mu.Unlock()
	if n != 1 {
		t.Fatalf("window count = %d, want 1", n)
	}
}
