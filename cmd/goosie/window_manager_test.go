package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/platform"
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

// waitWindows polls the manager's window count until it equals want or the
// deadline passes. Teardown runs on a goroutine, so the count settles async.
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
	// Both loops are live: each window presents independently.
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
	// f2 is still registered and its loop has not been torn down.
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

	// Seed a bookmark so the shared history/bookmark save path has state.
	if m.profile.bookmarks != nil {
		m.profile.bookmarks.Toggle("https://example.test/", "Example")
	}

	runErr := make(chan error, 1)
	go func() { runErr <- m.run() }()

	// Wait for the primary window to exist, then close it to end the run.
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

	// f1 has exactly one tab (the startup tab). Closing it fires onCloseLast,
	// which closes f1's window; the manager must remove f1 and keep f2.
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

	// Wait for the primary window and its first presented frame. By then run()
	// has taken its errs snapshot (in the old, primary-only code) and is blocked
	// at <-m.done, so a channel appended now stands for a window spawned later.
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
