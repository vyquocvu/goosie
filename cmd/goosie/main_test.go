package main

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/session"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestParsePrivateFlag(t *testing.T) {
	c, err := parse([]string{"-private"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.private {
		t.Fatal("-private did not set config.private")
	}
	c, err = parse([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if c.private {
		t.Fatal("default run set config.private")
	}
}

func TestParseRejectsInvalidViewport(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"zero width", []string{"-width", "0"}},
		{"negative height", []string{"-height", "-1"}},
		{"dpr zero", []string{"-dpr", "0"}},
		{"dpr NaN", []string{"-dpr", "NaN"}},
		{"dpr Inf", []string{"-dpr", "Inf"}},
		{"dpr too large", []string{"-dpr", "9"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(tc.args)
			if err == nil {
				t.Fatalf("parse(%v) succeeded, want error", tc.args)
			}
		})
	}
}

func TestLoadURLCtxRejectsInvalidViewport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html></html>"))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		w, h  int
		scale float32
	}{
		{"zero width", 0, 600, 1},
		{"NaN scale", 800, 600, float32(math.NaN())},
		{"Inf scale", 800, 600, float32(math.Inf(1))},
		{"round to zero", 1, 1, 0.001},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			layer, _, _, _, err := loadURLCtx(context.Background(), client, fonts, u.String(), tc.w, tc.h, tc.scale, t.TempDir())
			if err == nil {
				t.Fatalf("loadURLCtx(%d,%d,%v) succeeded, want error", tc.w, tc.h, tc.scale)
			}
			if layer != nil {
				t.Fatalf("loadURLCtx returned non-nil layer on error")
			}
		})
	}
}

func TestLoadURLCtxValidSmallDocument(t *testing.T) {
	html := `<!DOCTYPE html><html><body><div style="width:64px;height:64px;background:red"></div></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	layer, _, _, _, err := loadURLCtx(context.Background(), client, fonts, u.String(), 800, 600, 1, t.TempDir())
	if err != nil {
		t.Fatalf("loadURLCtx failed: %v", err)
	}
	if layer == nil {
		t.Fatal("loadURLCtx returned nil layer")
	}
	if layer.Bounds.Empty() {
		t.Fatal("loadURLCtx returned empty layer")
	}
}

func TestLoadURLCtxRejectsOversizedDocument(t *testing.T) {
	html := `<!DOCTYPE html><html><body><div style="width:65537px;height:65536px;background:red"></div></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	layer, _, _, _, err := loadURLCtx(context.Background(), client, fonts, u.String(), 800, 600, 1, t.TempDir())
	if err == nil {
		t.Fatal("loadURLCtx succeeded on oversized document")
	}
	if layer != nil {
		t.Fatal("loadURLCtx returned non-nil layer on oversized document")
	}
}

func TestDevSizeRounding(t *testing.T) {
	c := config{width: 800, height: 600, dpr: 1.5}
	size := c.devSize()
	if size.W != 1200 || size.H != 900 {
		t.Errorf("devSize() = %v, want {1200 900}", size)
	}
}

func TestConfigDevSize(t *testing.T) {
	c := config{width: 100, height: 100, dpr: 2}
	got := c.devSize()
	want := frame.Size{W: 200, H: 200}
	if got != want {
		t.Errorf("devSize() = %v, want %v", got, want)
	}
}

// newNavTestPath builds just enough framePath for applyNavResult and the session
// helpers: a real scheduler with an idle raster pool, and a one-tab manager.
func newNavTestPath(t *testing.T) *framePath {
	t.Helper()
	spec := paint.SceneSpec{DocHeight: 256}
	_, layer := paint.BuildLayer(spec)
	pool := raster.New(1, 4, func(j raster.Job) error { return nil })
	pool.Start(context.Background())
	t.Cleanup(func() { _ = pool.Close() })
	sched := raster.NewScheduler(layer, pool, frame.Viewport{Size: frame.Size{W: 256, H: 256}}, 1, raster.Pref{})
	f := &framePath{
		sched:      sched,
		navResults: make(chan navResult, 16),
	}
	f.tabMgr = tabs.NewManager(nil)
	f.tabMgr.NewTab()
	return f
}

func TestApplyNavResultNoHistoryPreservesForwardTrail(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.History.Push("https://a/")
	tab.History.Push("https://b/")
	tab.History.Push("https://c/")
	tab.History.Back()
	tab.Nav.Serial = 7

	f.applyNavResult(navResult{tabID: tab.ID, serial: 7, url: "https://b/", noHistory: true})

	entries, _ := tab.History.Entries()
	want := []string{"https://a/", "https://b/", "https://c/"}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("history = %v, want %v; forward trail must survive a back-navigation", entries, want)
	}
	if !tab.History.CanForward() {
		t.Error("CanForward() = false after back-navigation; forward history was destroyed")
	}
	if tab.URL != "https://b/" {
		t.Errorf("tab.URL = %q, want https://b/", tab.URL)
	}
}

func TestApplyNavResultNormalNavPushesHistory(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.History.Push("https://a/")
	tab.Nav.Serial = 3

	f.applyNavResult(navResult{tabID: tab.ID, serial: 3, url: "https://b/"})

	entries, _ := tab.History.Entries()
	want := []string{"https://a/", "https://b/"}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("history = %v, want %v", entries, want)
	}
}

func TestApplySessionStateRebuildsTabs(t *testing.T) {
	f := newNavTestPath(t)
	st := session.State{
		Tabs: []session.TabState{
			{URL: "https://a/", Title: "A", History: []string{"https://a/"}, HistoryIndex: 0},
			{URL: "https://b/", Title: "B", History: []string{"https://a/", "https://b/"}, HistoryIndex: 1},
			{URL: "https://c/", Title: "C"},
		},
		Active: 1,
	}
	f.applySessionState(st)

	if got := f.tabMgr.Count(); got != 3 {
		t.Fatalf("tab count = %d, want 3", got)
	}
	active := f.tabMgr.Active()
	if active.URL != "https://b/" || active.Title != "B" {
		t.Errorf("active tab = %q/%q, want https://b//B", active.URL, active.Title)
	}
	if _, idx := active.History.Entries(); idx != 1 {
		t.Errorf("active history index = %d, want 1", idx)
	}
	if f.restoredActiveURL != "https://b/" {
		t.Errorf("restoredActiveURL = %q, want https://b/", f.restoredActiveURL)
	}
}

func TestSessionStateRoundTrip(t *testing.T) {
	src := newNavTestPath(t)
	t1 := src.tabMgr.Active()
	t1.URL = "https://b/"
	t1.History.Push("https://a/")
	t1.History.Push("https://b/")
	t2 := src.tabMgr.NewTab()
	t2.URL = "https://c/"
	t2.History.Push("https://c/")

	path := filepath.Join(t.TempDir(), "session.json")
	if err := session.Save(path, sessionState(src)); err != nil {
		t.Fatal(err)
	}
	loaded, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	dst := newNavTestPath(t)
	dst.applySessionState(loaded)
	if dst.tabMgr.Count() != 2 {
		t.Fatalf("restored %d tabs, want 2", dst.tabMgr.Count())
	}
	first := dst.tabMgr.Tabs()[0]
	if entries, _ := first.History.Entries(); !reflect.DeepEqual(entries, []string{"https://a/", "https://b/"}) {
		t.Errorf("first tab history = %v", entries)
	}
	if first.URL != "https://b/" {
		t.Errorf("first tab URL = %q, want https://b/", first.URL)
	}
	second := dst.tabMgr.Tabs()[1]
	if second.URL != "https://c/" {
		t.Errorf("second tab URL = %q, want https://c/", second.URL)
	}
	if dst.tabMgr.ActiveIndex() != 0 {
		t.Errorf("active index = %d, want 0", dst.tabMgr.ActiveIndex())
	}
}

func TestParseSessionFileFlag(t *testing.T) {
	c, err := parse([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if c.sessionFile == "" || !strings.HasSuffix(c.sessionFile, "session.json") {
		t.Errorf("default session file = %q, want a path ending in session.json", c.sessionFile)
	}
	c, err = parse([]string{"-session-file", "/tmp/mine.json"})
	if err != nil {
		t.Fatal(err)
	}
	if c.sessionFile != "/tmp/mine.json" {
		t.Errorf("session file = %q, want /tmp/mine.json", c.sessionFile)
	}
}

func TestParseProfileScopesStateFiles(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	c, err := parse([]string{"-profile", "work"})
	if err != nil {
		t.Fatal(err)
	}
	wantSession := filepath.Join(home, ".goosie", "profiles", "work", "session.json")
	if c.sessionFile != wantSession {
		t.Errorf("profile session file = %q, want %q", c.sessionFile, wantSession)
	}
	wantCookies := filepath.Join(home, ".goosie", "profiles", "work", "cookies.json")
	if c.cookieFile != wantCookies {
		t.Errorf("profile cookie file = %q, want %q", c.cookieFile, wantCookies)
	}

	c, err = parse([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if c.cookieFile != filepath.Join(home, ".goosie", "cookies.json") {
		t.Errorf("default cookie file = %q, want under ~/.goosie", c.cookieFile)
	}
	if strings.Contains(c.cookieFile, "profiles") {
		t.Errorf("default cookie file must not live under profiles/: %q", c.cookieFile)
	}
}

func TestParseProfileRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"../etc", "a/b", `a\b`, "..", "."} {
		if _, err := parse([]string{"-profile", name}); err == nil {
			t.Errorf("parse(-profile %q) succeeded, want a usage error", name)
		}
	}
}
