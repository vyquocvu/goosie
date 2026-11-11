package main

import (
	"os"
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
