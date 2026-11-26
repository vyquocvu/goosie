package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/session"
	"github.com/vyquocvu/goosie/internal/surface"
)

func managerSessionConfig(t *testing.T) config {
	t.Helper()
	c := testManagerConfig(t)
	c.width = 640
	c.height = 480
	settings := preferences.Defaults()
	settings.StartupMode = preferences.StartupRestore
	if err := preferences.Save(c.preferencesFile, settings); err != nil {
		t.Fatalf("save startup preferences: %v", err)
	}
	return c
}

func managerSessionPage(label string) string {
	return fmt.Sprintf(`<!doctype html><html><head><title>%s</title></head><body><h1>%s</h1>%s</body></html>`, label, label, strings.Repeat("<p>row</p>", 400))
}

func managerSessionServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, managerSessionPage(strings.TrimPrefix(r.URL.Path, "/")))
	}))
	t.Cleanup(server.Close)
	return server
}

func waitForManagerState(t *testing.T, description string, condition func() bool) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", description)
		}
	}
}

func managerWindowList(m *windowManager) []*framePath {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*framePath(nil), m.windows...)
}

func waitForManagerPage(t *testing.T, f *framePath, wantURL string) {
	t.Helper()
	waitForManagerState(t, wantURL+" to load", func() bool {
		tab := f.tabMgr.Active()
		return tab != nil && tab.URL() == wantURL && tab.Session() != nil && !tab.Loading()
	})
}

func scrollManagerWindow(t *testing.T, f *framePath, delta int32) int32 {
	t.Helper()
	start := f.sched.Viewport().Offset.Y
	window := f.window
	if cw, ok := window.(*chromeWindow); ok {
		window = cw.Window
	}
	injector, ok := window.(interface{ Inject(surface.Event) error })
	if !ok {
		t.Fatalf("window %T cannot inject events", window)
	}
	if err := injector.Inject(surface.Event{
		Kind:  surface.EvScroll,
		Delta: frame.Point{Y: delta},
		Pos:   frame.Point{X: 300, Y: 300},
	}); err != nil {
		t.Fatalf("inject scroll: %v", err)
	}
	var offset int32
	waitForManagerState(t, "window scroll", func() bool {
		offset = f.sched.Viewport().Offset.Y
		return offset > start
	})
	return offset
}

func populateManagerSessionWindow(t *testing.T, m *windowManager, baseURL, label string, scrollDelta int32) (*framePath, []string, [][]string, []int, []int32) {
	t.Helper()
	firstURL := baseURL + "/" + label + "-a"
	f, err := m.spawn(firstURL)
	if err != nil {
		t.Fatalf("spawn %s window: %v", label, err)
	}
	waitForManagerPage(t, f, firstURL)
	firstScroll := scrollManagerWindow(t, f, scrollDelta)

	f.newTab()
	secondURL := baseURL + "/" + label + "-b"
	f.navigateTab(secondURL)
	waitForManagerPage(t, f, secondURL)
	activeURL := baseURL + "/" + label + "-c"
	f.navigateTab(activeURL)
	waitForManagerPage(t, f, activeURL)
	activeScroll := scrollManagerWindow(t, f, scrollDelta+200)
	if activeScroll == firstScroll {
		t.Fatalf("%s window tabs have indistinguishable scroll positions: %d", label, firstScroll)
	}

	return f,
		[]string{firstURL, activeURL},
		[][]string{{firstURL}, {secondURL, activeURL}},
		[]int{0, 1},
		[]int32{firstScroll, activeScroll}
}

func assertManagerSessionWindow(t *testing.T, f *framePath, urls []string, histories [][]string, indexes []int, scrolls []int32) {
	t.Helper()
	tabs := f.tabMgr.Tabs()
	if len(tabs) != len(urls) {
		t.Fatalf("tab count = %d, want %d", len(tabs), len(urls))
	}
	if active := f.tabMgr.Active(); active == nil || active != tabs[len(tabs)-1] {
		t.Fatalf("active tab = %v, want last tab", active)
	}
	for i, tab := range tabs {
		if got := tab.URL(); got != urls[i] {
			t.Errorf("tab %d URL = %q, want %q", i, got, urls[i])
		}
		entries, index := tab.History().Entries()
		if !reflect.DeepEqual(entries, histories[i]) || index != indexes[i] {
			t.Errorf("tab %d history = %v@%d, want %v@%d", i, entries, index, histories[i], indexes[i])
		}
		if got := tab.ScrollY(); got != scrolls[i] {
			t.Errorf("tab %d ScrollY = %d, want %d", i, got, scrolls[i])
		}
	}
	wantOffset := scrolls[len(scrolls)-1]
	waitForManagerState(t, "active viewport restoration", func() bool {
		return f.sched.Viewport().Offset.Y == wantOffset
	})
	if got := f.sched.Viewport().Offset.Y; got != wantOffset {
		t.Errorf("active viewport offset = %d, want %d", got, wantOffset)
	}
}

func TestManagerSaveRestartRestoresEveryWindow(t *testing.T) {
	c := managerSessionConfig(t)
	server := managerSessionServer(t)
	var wantURLs [2][]string
	var wantHistories [2][][]string
	var wantIndexes [2][]int
	var wantScrolls [2][]int32

	func() {
		m, err := newWindowManager(c)
		if err != nil {
			t.Fatalf("newWindowManager: %v", err)
		}
		defer m.cleanup()

		f1, urls1, histories1, indexes1, scrolls1 := populateManagerSessionWindow(t, m, server.URL, "one", 120)
		f2, urls2, histories2, indexes2, scrolls2 := populateManagerSessionWindow(t, m, server.URL, "two", 320)
		wantURLs = [2][]string{urls1, urls2}
		wantHistories = [2][][]string{histories1, histories2}
		wantIndexes = [2][]int{indexes1, indexes2}
		wantScrolls = [2][]int32{scrolls1, scrolls2}

		privateTab := f1.newIncognitoTab()
		privateTab.SetURL("https://private.example/never-persist")
		f1.switchTab(f1.tabMgr.Tabs()[1].ID())
		if f1 == f2 {
			t.Fatal("manager reused one framePath for both windows")
		}

		m.save()
		data, err := os.ReadFile(c.sessionFile)
		if err != nil {
			t.Fatalf("read saved session: %v", err)
		}
		if strings.Contains(string(data), privateTab.URL()) {
			t.Fatalf("private tab leaked into saved session: %s", data)
		}
		st, err := session.Load(c.sessionFile)
		if err != nil {
			t.Fatalf("load saved session: %v", err)
		}
		if len(st.Windows) != 2 {
			t.Fatalf("saved window count = %d, want 2", len(st.Windows))
		}
		for i, window := range st.Windows {
			if window.Active != wantIndexes[i][1] {
				t.Errorf("saved window %d active = %d, want %d", i, window.Active, wantIndexes[i][1])
			}
			for j, tab := range window.Tabs {
				if tab.ScrollY != int(wantScrolls[i][j]) {
					t.Errorf("saved window %d tab %d ScrollY = %d, want %d", i, j, tab.ScrollY, wantScrolls[i][j])
				}
			}
		}
		if !reflect.DeepEqual(st.Tabs, st.Windows[0].Tabs) || st.Active != st.Windows[0].Active {
			t.Fatalf("legacy fields do not mirror the primary window: %+v@%d", st.Tabs, st.Active)
		}
	}()

	restored, err := newWindowManager(c)
	if err != nil {
		t.Fatalf("newWindowManager for restart: %v", err)
	}
	t.Cleanup(restored.cleanup)
	runErr := make(chan error, 1)
	go func() { runErr <- restored.run() }()
	waitForManagerState(t, "two restored windows", func() bool {
		return len(managerWindowList(restored)) == 2
	})
	windows := managerWindowList(restored)
	waitForManagerPage(t, windows[0], wantURLs[0][1])
	waitForManagerPage(t, windows[1], wantURLs[1][1])
	for i, f := range windows {
		assertManagerSessionWindow(t, f, wantURLs[i], wantHistories[i], wantIndexes[i], wantScrolls[i])
	}

	restored.stop()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("restarted manager run: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("restarted manager did not stop")
	}
	st, err := session.Load(c.sessionFile)
	if err != nil {
		t.Fatalf("load session after clean shutdown: %v", err)
	}
	if len(st.Windows) != 2 {
		t.Fatalf("window count after clean shutdown = %d, want 2", len(st.Windows))
	}
	if _, err := os.Stat(c.sessionActiveFile); !os.IsNotExist(err) {
		t.Fatalf("crash journal remains after clean shutdown: %v", err)
	}
}

func TestManagerRestoresLegacySession(t *testing.T) {
	c := managerSessionConfig(t)
	server := managerSessionServer(t)
	firstURL := server.URL + "/legacy-a"
	activeURL := server.URL + "/legacy-b"
	legacy := session.State{
		Tabs: []session.TabState{
			{URL: firstURL, Title: "Legacy A", History: []string{firstURL}, HistoryIndex: 0, ScrollY: 120},
			{URL: activeURL, Title: "Legacy B", History: []string{firstURL, activeURL}, HistoryIndex: 1, ScrollY: 360},
		},
		Active: 1,
	}
	if err := session.Save(c.sessionFile, legacy); err != nil {
		t.Fatalf("save legacy session: %v", err)
	}

	m, err := newWindowManager(c)
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	runErr := make(chan error, 1)
	go func() { runErr <- m.run() }()
	waitForManagerState(t, "legacy window", func() bool {
		return len(managerWindowList(m)) == 1
	})
	f := managerWindowList(m)[0]
	waitForManagerPage(t, f, activeURL)
	assertManagerSessionWindow(t,
		f,
		[]string{firstURL, activeURL},
		[][]string{{firstURL}, {firstURL, activeURL}},
		[]int{0, 1},
		[]int32{120, 360},
	)
	m.stop()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("legacy manager run: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("legacy manager did not stop")
	}
}

func TestManagerCommandLineURLSkipsSavedWindows(t *testing.T) {
	c := managerSessionConfig(t)
	server := managerSessionServer(t)
	if err := session.Save(c.sessionFile, session.State{Windows: []session.WindowState{
		{Tabs: []session.TabState{{URL: server.URL + "/saved-one"}}, Active: 0},
		{Tabs: []session.TabState{{URL: server.URL + "/saved-two"}}, Active: 0},
	}}); err != nil {
		t.Fatalf("save grouped session: %v", err)
	}
	c.url = server.URL + "/override"
	m, err := newWindowManager(c)
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	if err := m.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	windows := managerWindowList(m)
	if len(windows) != 1 {
		t.Fatalf("window count = %d, want 1", len(windows))
	}
	waitForManagerPage(t, windows[0], c.url)
	if got := windows[0].tabMgr.Count(); got != 1 {
		t.Fatalf("tab count = %d, want 1", got)
	}
}

func TestManagerStartupNewTabSkipsSavedWindows(t *testing.T) {
	c := managerSessionConfig(t)
	settings := preferences.Defaults()
	settings.StartupMode = preferences.StartupNewTab
	if err := preferences.Save(c.preferencesFile, settings); err != nil {
		t.Fatalf("save startup preferences: %v", err)
	}
	if err := session.Save(c.sessionFile, session.State{Windows: []session.WindowState{
		{Tabs: []session.TabState{{URL: "https://saved-one.example/"}}, Active: 0},
		{Tabs: []session.TabState{{URL: "https://saved-two.example/"}}, Active: 0},
	}}); err != nil {
		t.Fatalf("save grouped session: %v", err)
	}
	m, err := newWindowManager(c)
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	if err := m.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	windows := managerWindowList(m)
	if len(windows) != 1 {
		t.Fatalf("window count = %d, want 1", len(windows))
	}
	if got := windows[0].tabMgr.Count(); got != 1 {
		t.Fatalf("tab count = %d, want 1", got)
	}
	if got := windows[0].tabMgr.Active().URL(); got != "" {
		t.Fatalf("startup URL = %q, want a new tab", got)
	}
}

func TestManagerSaveSkipsPrivateWindow(t *testing.T) {
	c := managerSessionConfig(t)
	m, err := newWindowManager(c)
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	regular, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn regular window: %v", err)
	}
	regular.tabMgr.Active().SetURL("https://regular.example/")
	private, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn private window: %v", err)
	}
	private.tabMgr.Active().SetURL("https://private.example/")
	private.config.private = true
	m.save()
	data, err := os.ReadFile(c.sessionFile)
	if err != nil {
		t.Fatalf("read saved session: %v", err)
	}
	if strings.Contains(string(data), "private.example") {
		t.Fatalf("private window leaked into saved session: %s", data)
	}
	st, err := session.Load(c.sessionFile)
	if err != nil {
		t.Fatalf("load saved session: %v", err)
	}
	if len(st.Windows) != 1 || st.Windows[0].Tabs[0].URL != "https://regular.example/" {
		t.Fatalf("saved windows = %+v, want only the regular window", st.Windows)
	}
}
