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
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
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

func TestInterceptContentKeyConsumedWhenFocused(t *testing.T) {
	cw := &chromeWindow{
		toolbar: &toolbar.State{},
		events:  make(chan surface.Event, 8),
	}
	key := surface.Event{Kind: surface.EvKey, Key: 'x'}
	if cw.intercept(key) {
		t.Fatal("key consumed with no focused control")
	}

	consumed := false
	cw.onContentKey = func(k rune, m surface.KeyMod) bool { consumed = true; return true }
	if !cw.intercept(key) {
		t.Fatal("key not consumed when onContentKey handled it")
	}
	if !consumed {
		t.Fatal("onContentKey not called")
	}

	cw.onContentKey = func(k rune, m surface.KeyMod) bool { return false }
	if cw.intercept(key) {
		t.Fatal("key consumed when onContentKey declined")
	}
}

func TestInterceptFocusClickCoordinates(t *testing.T) {
	cw := &chromeWindow{
		toolbar: &toolbar.State{},
		events:  make(chan surface.Event, 8),
	}
	var gotX, gotY int32
	cw.onFocusClick = func(x, y int32) bool { gotX, gotY = x, y; return true }

	click := surface.Event{
		Kind:   surface.EvPointer,
		Button: surface.ButtonLeft,
		Pos:    frame.Point{X: 40, Y: int32(totalChromeHeight) + 12},
	}
	if !cw.intercept(click) {
		t.Fatal("focus click not consumed")
	}
	if gotX != 40 || gotY != 12 {
		t.Fatalf("focus click at (%d,%d), want (40,12)", gotX, gotY)
	}

	cw.onFocusClick = func(x, y int32) bool { return false }
	if cw.intercept(click) {
		t.Fatal("focus click consumed when handler declined")
	}
}

func TestInterceptDragSelectMotion(t *testing.T) {
	cw := &chromeWindow{
		toolbar: &toolbar.State{},
		events:  make(chan surface.Event, 8),
	}
	var gotAction surface.PointerAction
	var gotX, gotY int32
	cw.onDragSelect = func(a surface.PointerAction, x, y int32) bool {
		gotAction, gotX, gotY = a, x, y
		return true
	}

	motion := surface.Event{
		Kind:   surface.EvPointer,
		Action: surface.PointerMotion,
		Button: surface.ButtonNone,
		Pos:    frame.Point{X: 55, Y: int32(totalChromeHeight) + 7},
	}
	if !cw.intercept(motion) {
		t.Fatal("drag motion not consumed")
	}
	if gotAction != surface.PointerMotion || gotX != 55 || gotY != 7 {
		t.Fatalf("drag select got action %v (%d,%d), want motion (55,7)", gotAction, gotX, gotY)
	}

	cw.onDragSelect = func(a surface.PointerAction, x, y int32) bool { return false }
	if cw.intercept(motion) {
		t.Fatal("drag motion consumed when handler declined")
	}
}

func TestInterceptResizeReportsLogicalContentSize(t *testing.T) {
	cw := &chromeWindow{
		toolbar: &toolbar.State{},
		events:  make(chan surface.Event, 8),
		scale:   2,
	}
	var gotW, gotH int
	cw.onResize = func(w, h int) { gotW, gotH = w, h }

	// The shim reports device pixels; the host must receive logical content size.
	resize := surface.Event{
		Kind: surface.EvResize,
		Size: frame.Size{W: 2880, H: 1800},
	}
	if cw.intercept(resize) {
		t.Fatal("resize consumed")
	}
	wantH := (1800 - int(totalChromeHeight)*2) / 2
	if gotW != 1440 || gotH != wantH {
		t.Fatalf("resize reported (%d,%d), want (1440,%d)", gotW, gotH, wantH)
	}
}

func TestInterceptDragSelectPressOrder(t *testing.T) {
	cw := &chromeWindow{
		toolbar: &toolbar.State{},
		events:  make(chan surface.Event, 8),
	}
	cw.onFocusClick = func(x, y int32) bool { return false }
	var actions []surface.PointerAction
	cw.onDragSelect = func(a surface.PointerAction, x, y int32) bool {
		actions = append(actions, a)
		return true
	}

	press := surface.Event{
		Kind:   surface.EvPointer,
		Button: surface.ButtonLeft,
		Pos:    frame.Point{X: 10, Y: int32(totalChromeHeight) + 3},
	}
	if !cw.intercept(press) {
		t.Fatal("press not consumed by drag select")
	}
	if len(actions) != 1 || actions[0] != surface.PointerPress {
		t.Fatalf("drag select saw %v, want one press", actions)
	}

	actions = nil
	cw.onFocusClick = func(x, y int32) bool { return true }
	if !cw.intercept(press) {
		t.Fatal("press not consumed by focus click")
	}
	if len(actions) != 0 {
		t.Fatal("drag select saw press that focus consumed")
	}
}

func TestInterceptCopyShortcut(t *testing.T) {
	cw := &chromeWindow{
		toolbar: &toolbar.State{},
		events:  make(chan surface.Event, 8),
	}
	copied := false
	cw.onCopy = func() bool { copied = true; return true }

	key := surface.Event{Kind: surface.EvKey, Key: 'c', Mods: surface.ModCommand}
	if !cw.intercept(key) {
		t.Fatal("Cmd+C not consumed")
	}
	if !copied {
		t.Fatal("onCopy not called")
	}

	cw.onCopy = func() bool { return false }
	if cw.intercept(key) {
		t.Fatal("Cmd+C consumed when onCopy declined")
	}
	if cw.intercept(surface.Event{Kind: surface.EvKey, Key: 'c'}) {
		t.Fatal("plain C consumed")
	}
}

func TestBlankLayerIsSingleWhiteFill(t *testing.T) {
	dl, layer := blankLayer(frame.Size{W: 512, H: 256})
	if dl.Len() != 1 {
		t.Fatalf("blank page holds %d commands, want 1", dl.Len())
	}
	cmd := dl.At(0)
	if cmd.Kind != paint.CmdFill || cmd.Rect != frame.Rect4(0, 0, 512, 256) || cmd.Color != frame.RGB(255, 255, 255) {
		t.Errorf("blank page command = %+v, want one white fill of the viewport", cmd)
	}
	if layer.Bounds != frame.Rect4(0, 0, 512, 256) {
		t.Errorf("blank layer bounds = %v, want the viewport", layer.Bounds)
	}
}

func TestBuildInteractiveOpensBlankPage(t *testing.T) {
	f, err := build(config{width: 640, height: 480, dpr: 1, private: true, downloadDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer f.pool.Close()
	defer f.client.Close()
	if f.spec.Checkerboard || f.spec.TextRuns != 0 {
		t.Errorf("startup spec = %+v, want a blank page", f.spec)
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Layer == nil {
		t.Fatal("first tab has no layer to present")
	}
}

func TestBuildKeepsSyntheticSceneForPacedAndExplicitRuns(t *testing.T) {
	paced, err := build(config{width: 640, height: 480, dpr: 1, gate: true, frames: 4, scene: "checkerboard", private: true, downloadDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer paced.pool.Close()
	defer paced.client.Close()
	if !paced.spec.Checkerboard {
		t.Error("gate run dropped the checkerboard scene")
	}

	demo, err := build(config{width: 640, height: 480, dpr: 1, scene: "plain", sceneSet: true, private: true, downloadDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer demo.pool.Close()
	defer demo.client.Close()
	if demo.spec.Checkerboard || demo.spec.TextRuns == 0 {
		t.Errorf("explicit -scene spec = %+v, want the plain scene", demo.spec)
	}
}
