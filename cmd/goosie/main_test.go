package main

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

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
			layer, _, _, _, _, err := loadURLCtx(context.Background(), client, fonts, u.String(), tc.w, tc.h, tc.scale, t.TempDir(), false, false)
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

	layer, _, _, _, _, err := loadURLCtx(context.Background(), client, fonts, u.String(), 800, 600, 1, t.TempDir(), false, false)
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
	html := `<!DOCTYPE html><html><body><div style="width:131073px;height:262144px;background:red"></div></body></html>`
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

	layer, _, _, _, _, err := loadURLCtx(context.Background(), client, fonts, u.String(), 800, 600, 1, t.TempDir(), false, false)
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
		pool:       pool,
		navResults: make(chan navResult, 16),
		imgResults: make(chan imgResult, 16),
		zoom:       1.0,
	}
	f.tabMgr = tabs.NewManager(nil)
	f.tabMgr.NewTab()
	return f
}

func TestApplyNavResultNoHistoryPreservesForwardTrail(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.History().Push("https://a/")
	tab.History().Push("https://b/")
	tab.History().Push("https://c/")
	tab.History().Back()
	tab.Nav.Serial = 7

	f.applyNavResult(navResult{tabID: tab.ID(), serial: 7, url: "https://b/", noHistory: true})

	entries, _ := tab.History().Entries()
	want := []string{"https://a/", "https://b/", "https://c/"}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("history = %v, want %v; forward trail must survive a back-navigation", entries, want)
	}
	if !tab.History().CanForward() {
		t.Error("CanForward() = false after back-navigation; forward history was destroyed")
	}
	if tab.URL() != "https://b/" {
		t.Errorf("tab.URL = %q, want https://b/", tab.URL())
	}
}

func TestApplyNavResultNormalNavPushesHistory(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.History().Push("https://a/")
	tab.Nav.Serial = 3

	f.applyNavResult(navResult{tabID: tab.ID(), serial: 3, url: "https://b/"})

	entries, _ := tab.History().Entries()
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
	if active.URL() != "https://b/" || active.Title() != "B" {
		t.Errorf("active tab = %q/%q, want https://b//B", active.URL(), active.Title())
	}
	if _, idx := active.History().Entries(); idx != 1 {
		t.Errorf("active history index = %d, want 1", idx)
	}
	if f.restoredActiveURL != "https://b/" {
		t.Errorf("restoredActiveURL = %q, want https://b/", f.restoredActiveURL)
	}
}

func TestSessionStateRoundTrip(t *testing.T) {
	src := newNavTestPath(t)
	t1 := src.tabMgr.Active()
	t1.SetURL("https://b/")
	t1.History().Push("https://a/")
	t1.History().Push("https://b/")
	t2 := src.tabMgr.NewTab()
	t2.SetURL("https://c/")
	t2.History().Push("https://c/")

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
	if entries, _ := first.History().Entries(); !reflect.DeepEqual(entries, []string{"https://a/", "https://b/"}) {
		t.Errorf("first tab history = %v", entries)
	}
	if first.URL() != "https://b/" {
		t.Errorf("first tab URL = %q, want https://b/", first.URL())
	}
	second := dst.tabMgr.Tabs()[1]
	if second.URL() != "https://c/" {
		t.Errorf("second tab URL = %q, want https://c/", second.URL())
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

// TestInterceptBareTabReachesContentKey asserts a bare Tab is routed to
// onContentKey (for focus traversal) rather than being swallowed by the
// browser-tab shortcut, which only fires for Cmd+Tab.
func TestInterceptBareTabReachesContentKey(t *testing.T) {
	cw := &chromeWindow{
		toolbar: &toolbar.State{},
		events:  make(chan surface.Event, 8),
	}
	var gotKey rune
	var gotMods surface.KeyMod
	called := false
	cw.onContentKey = func(k rune, m surface.KeyMod) bool {
		gotKey, gotMods, called = k, m, true
		return true
	}
	if !cw.intercept(surface.Event{Kind: surface.EvKey, Key: '\t', Mods: surface.ModShift}) {
		t.Fatal("bare Shift+Tab not consumed")
	}
	if !called {
		t.Fatal("onContentKey not called for bare Tab")
	}
	if gotKey != '\t' {
		t.Errorf("content key = %q, want Tab", gotKey)
	}
	if gotMods&surface.ModShift == 0 {
		t.Error("Shift modifier not forwarded for bare Tab")
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

	// The shim reports the whole window surface in device pixels: the content
	// viewport plus the chrome band chromeWindow composites on top. The host
	// must see the logical content size, and the loop must see a resize that
	// carries the content device size - forwarding the window size makes the
	// scheduler size a buffer that Present then grows again by the chrome
	// band, and the stretched bitmap puts every hover a line below its pixels.
	resize := surface.Event{
		Kind:  surface.EvResize,
		Size:  frame.Size{W: 2880, H: 1800},
		Scale: 2,
	}
	if !cw.intercept(resize) {
		t.Fatal("resize not consumed")
	}
	wantH := (1800 - int(totalChromeHeight)*2) / 2
	if gotW != 1440 || gotH != wantH {
		t.Fatalf("resize reported (%d,%d), want (1440,%d)", gotW, gotH, wantH)
	}

	var fwd surface.Event
	select {
	case fwd = <-cw.events:
	default:
		t.Fatal("no resized event forwarded to the loop")
	}
	if fwd.Kind != surface.EvResize {
		t.Fatalf("forwarded kind %v, want EvResize", fwd.Kind)
	}
	if fwd.Size != (frame.Size{W: 2880, H: int32(wantH) * 2}) {
		t.Fatalf("forwarded size %+v, want the content device size {2880 %d}", fwd.Size, int32(wantH)*2)
	}
	if fwd.Scale != 2 {
		t.Fatalf("forwarded scale %v, want the window's 2", fwd.Scale)
	}
	select {
	case extra := <-cw.events:
		t.Fatalf("second forwarded event %+v", extra)
	default:
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
	if tab == nil || tab.Layer() == nil {
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

// errTraversalBoom stands in for a failed traversal load.
var errTraversalBoom = errors.New("boom")

// TestApplyNavResultFailedTraversalKeepsCursor guards transactional
// back/forward: the cursor moves only when the traversal's page displays. A
// failed load used to strand it on a page that never rendered because Back()
// moved it before the load started.
func TestApplyNavResultFailedTraversalKeepsCursor(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.History().Push("https://a/")
	tab.History().Push("https://b/")
	tab.Nav.Serial = 5

	f.applyNavResult(navResult{
		tabID: tab.ID(), serial: 5, url: "https://a/",
		err:          errTraversalBoom,
		noHistory:    true,
		traverse:     -1,
		traverseBase: 1,
	})

	if got := tab.History().Current(); got != "https://b/" {
		t.Errorf("cursor = %q after failed traversal, want https://b/", got)
	}
	if !tab.History().CanBack() {
		t.Error("CanBack() = false after failed traversal; the trail must survive")
	}
}

// TestApplyNavResultSuccessfulTraversalCommitsCursor pins the other half: a
// peeked step lands when its page displays.
func TestApplyNavResultSuccessfulTraversalCommitsCursor(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.History().Push("https://a/")
	tab.History().Push("https://b/")
	tab.History().Push("https://c/")
	tab.Nav.Serial = 9

	f.applyNavResult(navResult{
		tabID: tab.ID(), serial: 9, url: "https://b/",
		noHistory: true, traverse: -1, traverseBase: 2,
	})

	if got := tab.History().Current(); got != "https://b/" {
		t.Errorf("cursor = %q after successful traversal, want https://b/", got)
	}
}

// TestApplyNavResultStaleTraversalDrops pins the interleave guard: a typed
// URL landing first owns the cursor, and the older traversal must not yank
// it back.
func TestApplyNavResultStaleTraversalDrops(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.History().Push("https://a/")
	tab.History().Push("https://b/")
	tab.History().Push("https://d/")
	tab.Nav.Serial = 9

	f.applyNavResult(navResult{
		tabID: tab.ID(), serial: 9, url: "https://a/",
		noHistory: true, traverse: -1, traverseBase: 1,
	})

	if got := tab.History().Current(); got != "https://d/" {
		t.Errorf("cursor = %q after stale traversal, want https://d/", got)
	}
}

// TestApplyNavResultFreshNavResetsScroll guards navigation commitment: a new
// page opens at the top instead of inheriting the old page's scroll offset.
// Reloads and traversals keep their offset.
func TestApplyNavResultFreshNavResetsScroll(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.History().Push("https://a/")
	tab.Nav.Serial = 4
	tab.SetScrollY(500)
	f.sched.SetViewport(frame.Viewport{Offset: frame.Point{Y: 500}, Size: frame.Size{W: 256, H: 256}})

	f.applyNavResult(navResult{tabID: tab.ID(), serial: 4, url: "https://b/"})

	if tab.ScrollY() != 0 {
		t.Errorf("tab.ScrollY = %d, want 0 (fresh navigation opens at top)", tab.ScrollY())
	}
	if y := f.sched.Viewport().Offset.Y; y != 0 {
		t.Errorf("viewport offset = %d, want 0", y)
	}
}

// tallNavTestPath is newNavTestPath with a scrollable document, so viewport
// offsets survive the scheduler's clamp and scroll behavior is observable.
// The scheduler clamps against its construction layer, so the height has to
// be tall from the start rather than swapped in later.
func tallNavTestPath(t *testing.T) *framePath {
	t.Helper()
	spec := paint.SceneSpec{DocHeight: 2048}
	_, layer := paint.BuildLayer(spec)
	pool := raster.New(1, 4, func(j raster.Job) error { return nil })
	pool.Start(context.Background())
	t.Cleanup(func() { _ = pool.Close() })
	sched := raster.NewScheduler(layer, pool, frame.Viewport{Size: frame.Size{W: 256, H: 256}}, 1, raster.Pref{})
	f := &framePath{
		sched:      sched,
		pool:       pool,
		navResults: make(chan navResult, 16),
		imgResults: make(chan imgResult, 16),
	}
	f.tabMgr = tabs.NewManager(nil)
	f.tabMgr.NewTab()
	return f
}

// TestApplyNavResultTraverseKeepsScroll pins the other half: back/forward
// and reload success leave the viewport offset alone.
func TestApplyNavResultTraverseKeepsScroll(t *testing.T) {
	f := tallNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.History().Push("https://a/")
	tab.History().Push("https://b/")
	tab.Nav.Serial = 6
	f.sched.SetViewport(frame.Viewport{Offset: frame.Point{Y: 120}, Size: frame.Size{W: 256, H: 256}})

	f.applyNavResult(navResult{
		tabID: tab.ID(), serial: 6, url: "https://a/",
		noHistory: true, traverse: -1, traverseBase: 1,
	})

	if y := f.sched.Viewport().Offset.Y; y != 120 {
		t.Errorf("viewport offset = %d, want 120 (traversal preserves offset)", y)
	}
}

// TestShutdownCancelsAndJoinsNavigations guards teardown ordering: in-flight
// loads root at the run scope, so cancelling it aborts them, and the waiter
// proves no loader still runs afterwards instead of leaking into teardown.
func TestShutdownCancelsAndJoinsNavigations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	f := newNavTestPath(t)
	f.client = net.DefaultClient()
	defer f.client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	f.fonts = fonts
	f.config = config{width: 800, height: 600, dpr: 1}
	f.runCtx, f.runCancel = context.WithCancel(context.Background())

	f.navigateTab(server.URL + "/hangs")
	deadline := time.Now().Add(5 * time.Second)
	for {
		tab := f.tabMgr.Active()
		tab.Nav.Mu.Lock()
		loading := tab.Nav.Loading
		tab.Nav.Mu.Unlock()
		if loading || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if tab := f.tabMgr.Active(); !tab.Nav.Loading {
		t.Fatal("navigation never started loading")
	}

	f.runCancel()
	joined := make(chan struct{})
	go func() {
		f.navWg.Wait()
		close(joined)
	}()
	select {
	case <-joined:
	case <-time.After(10 * time.Second):
		t.Fatal("navWg.Wait did not return after run-cancel; the loader is not joined")
	}
}

// TestLoadURLCtxReportsFinalURLAfterRedirect pins what the address bar and
// the history must record: the URL that displayed, not the one typed. A
// redirect used to leave the requested URL committed while subresources
// resolved against the final one.
func TestLoadURLCtxReportsFinalURLAfterRedirect(t *testing.T) {
	html := `<!DOCTYPE html><html><body><div style="width:64px;height:64px;background:red"></div></body></html>`
	var final string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			return
		}
		final = r.URL.Path
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer server.Close()
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, _, got, err := loadURLCtx(context.Background(), client, fonts, server.URL+"/old", 800, 600, 1, t.TempDir(), false, false)
	if err != nil {
		t.Fatalf("loadURLCtx redirected: %v", err)
	}
	if got != server.URL+"/new" {
		t.Errorf("final URL = %q, want %q", got, server.URL+"/new")
	}
	if final != "/new" {
		t.Errorf("served path = %q, want /new (redirect not followed)", final)
	}
}

// TestApplyNavResultTraversalRewritesRedirectedEntry pins the history side:
// traversing back to an entry that redirects replaces that entry with the
// final URL instead of recording the requested one alongside it.
func TestApplyNavResultTraversalRewritesRedirectedEntry(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.History().Push("https://a/")
	tab.History().Push("https://b/")
	tab.Nav.Serial = 11

	f.applyNavResult(navResult{
		tabID: tab.ID(), serial: 11, url: "https://a-final/",
		noHistory: true, traverse: -1, traverseBase: 1,
	})

	entries, idx := tab.History().Entries()
	want := []string{"https://a-final/", "https://b/"}
	if !reflect.DeepEqual(entries, want) || idx != 0 {
		t.Errorf("history = %v@%d, want %v@0 (entry rewritten in place)", entries, idx, want)
	}
	if tab.URL() != "https://a-final/" {
		t.Errorf("tab.URL = %q, want the final URL", tab.URL())
	}
}

// TestCertFailureReachesAddressBar pins the Gate 3 security display for the
// failure half: a TLS server whose certificate the default client does not
// trust must fail the load with a certificate error, and that error must land
// in the address bar (red text) with loading cleared — never a silent blank.
func TestCertFailureReachesAddressBar(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html></html>"))
	}))
	defer server.Close()
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, _, _, loadErr := loadURLCtx(context.Background(), client, fonts, server.URL, 800, 600, 1, t.TempDir(), false, false)
	if loadErr == nil {
		t.Fatal("loadURLCtx against an untrusted TLS server succeeded, want a certificate error")
	}
	if !strings.Contains(strings.ToLower(loadErr.Error()), "certificate") {
		t.Fatalf("load error = %v, want a certificate failure", loadErr)
	}

	f := newNavTestPath(t)
	f.toolbar = toolbar.NewState(800, nil)
	tab := f.tabMgr.Active()
	tab.Nav.Serial = 21
	f.applyNavResult(navResult{tabID: tab.ID(), serial: 21, url: server.URL, err: loadErr})

	if tab.Loading() {
		t.Error("tab still loading after a failed navigation")
	}
	if got := f.toolbar.Error; !strings.Contains(strings.ToLower(got), "certificate") {
		t.Errorf("address bar error = %q, want the certificate failure", got)
	}
}

// TestSlowImageAbortsAtFetchBudget pins in-flight cancellation: the engine's
// image budget stops new spawns past its deadline, but a hung host must also
// abort the workers already fetching instead of riding the 30s client
// timeout. The page itself still builds; the stalled image is omitted.
func TestSlowImageAbortsAtFetchBudget(t *testing.T) {
	img := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // hang until the client gives up
	}))
	defer img.Close()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><img src="` + img.URL + `/p.png"></body></html>`))
	}))
	defer page.Close()
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, _, _, sess, _, err := loadURLCtx(context.Background(), client, fonts, page.URL, 800, 600, 1, t.TempDir(), false, false)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("loadURLCtx with a hung image: %v (images must degrade, not fail)", err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
	if elapsed >= 25*time.Second {
		t.Fatalf("load took %v, want the ~6s fetch budget, not the 30s client timeout", elapsed)
	}
	t.Logf("hung-image load settled in %v", elapsed)
}

// TestTabStripWheelScrollsWhenOverflowing pins wheel ownership: a scroll over
// the tab bar moves the strip when tabs overflow, and is consumed so the
// document underneath does not scroll too.
func TestTabStripWheelScrollsWhenOverflowing(t *testing.T) {
	mgr := tabs.NewManager(nil)
	for i := 0; i < 40; i++ {
		mgr.NewTab()
	}
	cw := &chromeWindow{
		toolbar: toolbar.NewState(1000, nil),
		tabMgr:  mgr,
		events:  make(chan surface.Event, 8),
		scale:   1,
	}
	wheel := surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: 120}, Pos: frame.Point{X: 500, Y: 10}}
	if !cw.intercept(wheel) {
		t.Fatal("wheel over tab bar not consumed")
	}
	if got := mgr.TabScroll(); got != 120 {
		t.Fatalf("TabScroll() = %d, want 120", got)
	}
	huge := surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: 99999}, Pos: frame.Point{X: 500, Y: 10}}
	if !cw.intercept(huge) {
		t.Fatal("over-scroll not consumed")
	}
	if got := mgr.TabScroll(); got != 2016 {
		t.Fatalf("TabScroll() = %d, want 2016 (clamped to max)", got)
	}
	// Over the document the same wheel must reach the loop, not the strip.
	content := surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: 120}, Pos: frame.Point{X: 500, Y: 200}}
	if cw.intercept(content) {
		t.Fatal("wheel over document consumed by tab strip")
	}
	if got := mgr.TabScroll(); got != 2016 {
		t.Fatalf("TabScroll() = %d, want 2016 (content scroll must not move strip)", got)
	}
}

// TestTabStripWheelFallsThroughWhenFitting: with no overflow the strip cannot
// scroll, so the wheel always belongs to the document.
func TestTabStripWheelFallsThroughWhenFitting(t *testing.T) {
	mgr := tabs.NewManager(nil)
	for i := 0; i < 3; i++ {
		mgr.NewTab()
	}
	cw := &chromeWindow{
		toolbar: toolbar.NewState(1000, nil),
		tabMgr:  mgr,
		events:  make(chan surface.Event, 8),
		scale:   1,
	}
	wheel := surface.Event{Kind: surface.EvScroll, Delta: frame.Point{Y: 120}, Pos: frame.Point{X: 500, Y: 10}}
	if cw.intercept(wheel) {
		t.Fatal("wheel consumed by a fitting strip (max offset 0)")
	}
}

// TestSwitchTabScrollsActiveIntoView: switching to a clipped tab (keyboard,
// session restore) must bring it on screen, since the pointer cannot reach
// what the strip clipped away.
func TestSwitchTabScrollsActiveIntoView(t *testing.T) {
	f := newNavTestPath(t)
	f.config.width = 1000
	for i := 0; i < 39; i++ {
		f.tabMgr.NewTab()
	}
	last := f.tabMgr.Tabs()[39]
	f.switchTab(last.ID())
	if got := f.tabMgr.TabScroll(); got != 2016 {
		t.Fatalf("TabScroll() = %d, want 2016 (last tab scrolled into view)", got)
	}
	first := f.tabMgr.Tabs()[0]
	f.switchTab(first.ID())
	if got := f.tabMgr.TabScroll(); got != 0 {
		t.Fatalf("TabScroll() = %d, want 0 (first tab resets strip)", got)
	}
}

// tickWin is a chromeWindow backend with a drivable event channel.
type tickWin struct {
	events chan surface.Event
}

func (w *tickWin) Events() <-chan surface.Event              { return w.events }
func (w *tickWin) Present(*frame.Bitmap, []frame.Rect) error { return nil }
func (w *tickWin) SetCursor(surface.Cursor)                  {}
func (w *tickWin) SetIME(bool)                               {}
func (w *tickWin) ScaleFactor() float32                      { return 1 }
func (w *tickWin) Close() error                              { return nil }

// TestChromePumpFiresTickCallbacks pins the pump's periodic hook: the tick
// callback runs while events flow, and the pump still exits when the window
// closes. A zero interval disables ticks (all existing constructions).
func TestChromePumpFiresTickCallbacks(t *testing.T) {
	evs := make(chan surface.Event, 8)
	ticks := make(chan struct{}, 64)
	cw := &chromeWindow{
		Window:       &tickWin{events: evs},
		toolbar:      toolbar.NewState(800, nil),
		tabMgr:       tabs.NewManager(nil),
		events:       make(chan surface.Event, 8),
		scale:        1,
		tickInterval: time.Millisecond,
		onTick: func() {
			select {
			case ticks <- struct{}{}:
			default:
			}
		},
	}
	done := make(chan struct{})
	go func() { cw.pump(); close(done) }()
	evs <- surface.Event{Kind: surface.EvPointer, Action: surface.PointerMotion, Button: surface.ButtonNone, Pos: frame.Point{X: 700, Y: 500}}
	time.Sleep(30 * time.Millisecond)
	close(evs)
	<-done
	if n := len(ticks); n < 1 {
		t.Fatalf("onTick fired %d times in 30ms at 1ms cadence, want >= 1", n)
	}
}

// TestPumpJSRepaintsMutatedTab is the host half of the event loop: a timer
// mutation applied by PumpJS must reach a fresh layer and a published plan,
// and a quiescent tab must publish nothing further.
func TestPumpJSRepaintsMutatedTab(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><p id="t">waiting</p><script>setTimeout(function(){document.getElementById('t').textContent='fired';},0);</script></body></html>`))
	}))
	defer server.Close()
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	f := newNavTestPath(t)
	f.config.width = 800
	f.config.dpr = 1
	f.zoom = 1
	_, _, _, sess, _, err := loadURLCtx(context.Background(), client, fonts, server.URL, 800, 600, 1, t.TempDir(), false, false)
	if err != nil {
		t.Fatalf("loadURLCtx: %v", err)
	}
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	before := f.sched.PlanStats().Published
	f.pumpJS()
	if tab.Layer() == nil {
		t.Fatal("no layer after pumpJS with a due timer")
	}
	if got := f.sched.PlanStats().Published; got <= before {
		t.Fatal("pumpJS with a mutation published no frame plan")
	}
	published := f.sched.PlanStats().Published
	f.pumpJS()
	if got := f.sched.PlanStats().Published; got != published {
		t.Fatal("quiescent pumpJS published a plan")
	}
}

// TestTabStripChevronClicksScrollByPage pins the last mouse-only path past
// the minimum: the right chevron pages forward, the left pages back, both
// clamped to the strip range. With no overflow the same points hit nothing.
func TestTabStripChevronClicksScrollByPage(t *testing.T) {
	mgr := tabs.NewManager(nil)
	for i := 0; i < 40; i++ {
		mgr.NewTab()
	}
	cw := &chromeWindow{
		toolbar: toolbar.NewState(1000, nil),
		tabMgr:  mgr,
		events:  make(chan surface.Event, 8),
		scale:   1,
	}
	press := func(x, y int32) surface.Event {
		return surface.Event{Kind: surface.EvPointer, Action: surface.PointerPress, Button: surface.ButtonLeft, Pos: frame.Point{X: x, Y: y}}
	}
	left, right, ok := tabs.ChevronRects(40, 1000)
	if !ok {
		t.Fatal("no chevrons for 40 tabs at 1000px")
	}
	step := tabs.TabScrollPage(40, 1000)
	rx, ry := (right.X0+right.X1)/2, (right.Y0+right.Y1)/2
	if !cw.intercept(press(rx, ry)) {
		t.Fatal("right chevron press not consumed")
	}
	if got := mgr.TabScroll(); got != step {
		t.Fatalf("TabScroll() = %d, want one page %d", got, step)
	}
	lx, ly := (left.X0+left.X1)/2, (left.Y0+left.Y1)/2
	if !cw.intercept(press(lx, ly)) {
		t.Fatal("left chevron press not consumed")
	}
	if got := mgr.TabScroll(); got != 0 {
		t.Fatalf("TabScroll() = %d, want 0 (paged back, clamped)", got)
	}

	fitting := tabs.NewManager(nil)
	for i := 0; i < 3; i++ {
		fitting.NewTab()
	}
	cw.tabMgr = fitting
	switched, opened := false, false
	cw.onSwitch = func(uint64) { switched = true }
	cw.onNewTab = func() { opened = true }
	// A press in an empty tab-bar region is still consumed (the strip owns
	// its band), but it must fire nothing and move nothing.
	cw.intercept(press(rx, ry))
	if switched || opened {
		t.Fatal("chevron-zone press with no chevrons switched or opened a tab")
	}
	if got := fitting.TabScroll(); got != 0 {
		t.Fatalf("TabScroll() = %d, want 0", got)
	}
}

// TestLoadErrorsOmitQuerySecrets: navigation failures surface redacted URLs
// (diagnostics must omit query secrets by default), keeping the host.
func TestLoadErrorsOmitQuerySecrets(t *testing.T) {
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, _, err = loadURLCtx(context.Background(), client, fonts,
		"http://127.0.0.1:1/unreachable?token=secret123", 800, 600, 1, t.TempDir(), false, false)
	if err == nil {
		t.Fatal("loadURLCtx against a closed port succeeded")
	}
	if strings.Contains(err.Error(), "secret123") {
		t.Fatalf("load error leaks query secret: %v", err)
	}
}

// TestTabBarClicksFollowShrunkTabs pins draw/click agreement under overflow:
// with ten tabs at 1000 logical pixels the strip shrinks every tab, so a press
// at the drawn center of tab 5 must switch to tab 5 (not whichever fixed-200px
// tab happens to cover that point), and a press on the drawn new-tab button
// must open a tab (not fall past a button stranded off-screen).
func TestTabBarClicksFollowShrunkTabs(t *testing.T) {
	const tabCount, width = 10, 1000
	mgr := tabs.NewManager(nil)
	for i := 0; i < tabCount; i++ {
		mgr.NewTab()
	}
	list := mgr.Tabs()
	cw := &chromeWindow{
		toolbar: toolbar.NewState(width, nil),
		tabMgr:  mgr,
		events:  make(chan surface.Event, 8),
		scale:   1,
	}
	var switched uint64
	cw.onSwitch = func(id uint64) { switched = id }
	press := func(x, y int32) surface.Event {
		return surface.Event{Kind: surface.EvPointer, Action: surface.PointerPress, Button: surface.ButtonLeft, Pos: frame.Point{X: x, Y: y}}
	}

	r := tabs.TabRect(5, 0, tabCount, width)
	if r.W() >= int32(tabs.TabWidth) {
		t.Fatalf("tab 5 width = %d, want shrunk below %d", r.W(), tabs.TabWidth)
	}
	cx, cy := (r.X0+r.X1)/2, int32(tabs.TabBarHeight)/2
	if !cw.intercept(press(cx, cy)) {
		t.Fatal("press on tab 5 not consumed")
	}
	if switched != list[5].ID() {
		t.Fatalf("switched to tab %d, want tab 5 (id %d)", switched, list[5].ID())
	}

	btn := tabs.NewTabButtonRect(tabCount, 0, width)
	if btn.X1 > width {
		t.Fatalf("new-tab button ends at %d, beyond clickable width %d", btn.X1, width)
	}
	opened := false
	cw.onNewTab = func() { opened = true }
	if !cw.intercept(press((btn.X0+btn.X1)/2, (btn.Y0+btn.Y1)/2)) {
		t.Fatal("press on new-tab button not consumed")
	}
	if !opened {
		t.Fatal("press on new-tab button did not open a tab")
	}
}

// TestTabBarDragReordersTabs pins pointer reordering: press switches (as
// before), a motion past the threshold drags, and release commits the drop
// slot. A press-release without motion leaves the order alone.
func TestTabBarDragReordersTabs(t *testing.T) {
	const tabCount, width = 4, 1000
	mgr := tabs.NewManager(nil)
	for i := 0; i < tabCount; i++ {
		mgr.NewTab()
	}
	list := mgr.Tabs()
	cw := &chromeWindow{
		toolbar: toolbar.NewState(width, nil),
		tabMgr:  mgr,
		events:  make(chan surface.Event, 8),
		scale:   1,
	}
	var switched uint64
	cw.onSwitch = func(id uint64) { switched = id }
	press := func(x, y int32) surface.Event {
		return surface.Event{Kind: surface.EvPointer, Action: surface.PointerPress, Button: surface.ButtonLeft, Pos: frame.Point{X: x, Y: y}}
	}
	motion := func(x, y int32) surface.Event {
		return surface.Event{Kind: surface.EvPointer, Action: surface.PointerMotion, Button: surface.ButtonLeft, Pos: frame.Point{X: x, Y: y}}
	}
	release := func(x, y int32) surface.Event {
		return surface.Event{Kind: surface.EvPointer, Action: surface.PointerRelease, Button: surface.ButtonLeft, Pos: frame.Point{X: x, Y: y}}
	}

	// Drag tab 0 past tab 1: press, move well beyond the threshold, release.
	r0 := tabs.TabRect(0, 0, tabCount, width)
	cx, cy := (r0.X0+r0.X1)/2, int32(tabs.TabBarHeight)/2
	cw.intercept(press(cx, cy))
	if switched != list[0].ID() {
		t.Fatalf("press switched to %d, want tab 0", switched)
	}
	cw.intercept(motion(cx+150, cy))
	cw.intercept(release(cx+150, cy))
	got := mgr.Tabs()
	if len(got) != tabCount || got[0].ID() != list[1].ID() || got[1].ID() != list[0].ID() {
		t.Fatalf("order after drag = %v, want tab 0 moved past tab 1", tabIDsOf(got))
	}
	if active := mgr.Active(); active.ID() != list[0].ID() {
		t.Fatal("active tab changed across a drag of itself")
	}

	// Plain press-release without motion keeps the order.
	before := tabIDsOf(mgr.Tabs())
	r1 := tabs.TabRect(1, 0, tabCount, width)
	cw.intercept(press((r1.X0+r1.X1)/2, cy))
	cw.intercept(release((r1.X0+r1.X1)/2, cy))
	if after := tabIDsOf(mgr.Tabs()); !reflect.DeepEqual(before, after) {
		t.Fatalf("order changed without drag: %v -> %v", before, after)
	}
}

func tabIDsOf(list []*tabs.Tab) []uint64 {
	out := make([]uint64, len(list))
	for i, tab := range list {
		out[i] = tab.ID()
	}
	return out
}

// TestClearDataRemovesTracesKeepsBookmarks pins the v1 clear-data contract:
// cookies, history and session files go; starred bookmarks stay. Missing
// files are fine (idempotent); anything else is an error, not a half-clear.
func TestClearDataRemovesTracesKeepsBookmarks(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("cookies.json", `[]`)
	write("history.json", `[]`)
	write("session.json", `{"tabs":[]}`)
	write("bookmarks.json", `[{"url":"https://keep.example/"}]`)
	c := config{
		cookieFile:   filepath.Join(dir, "cookies.json"),
		historyFile:  filepath.Join(dir, "history.json"),
		sessionFile:  filepath.Join(dir, "session.json"),
		bookmarkFile: filepath.Join(dir, "bookmarks.json"),
	}
	removed, err := c.clearBrowsingData()
	if err != nil {
		t.Fatalf("clearBrowsingData: %v", err)
	}
	if len(removed) != 3 {
		t.Fatalf("removed %d files, want 3 (cookies, history, session)", len(removed))
	}
	for _, name := range []string{"cookies.json", "history.json", "session.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after clear", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "bookmarks.json")); err != nil {
		t.Fatalf("bookmarks.json missing after clear: %v (bookmarks are kept)", err)
	}
	// Idempotent: clearing twice is not an error.
	if _, err := c.clearBrowsingData(); err != nil {
		t.Fatalf("second clearBrowsingData: %v", err)
	}
}

// TestParseClearDataFlag pins the CLI surface of the v1 clear-data control.
func TestParseClearDataFlag(t *testing.T) {
	c, err := parse([]string{"-clear-data"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.clearData {
		t.Fatal("-clear-data did not set config.clearData")
	}
	c, err = parse([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if c.clearData {
		t.Fatal("default run set config.clearData")
	}
}

// TestRunClearDataEndToEnd pins the flag wiring: -clear-data with a scratch
// HOME clears the default profile's traces and exits success without opening
// anything.
func TestRunClearDataEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	statedir := filepath.Join(home, ".goosie")
	if err := os.MkdirAll(statedir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cookies.json", "history.json", "session.json", "bookmarks.json"} {
		if err := os.WriteFile(filepath.Join(statedir, name), []byte(`[]`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := run([]string{"-clear-data"}); err != nil {
		t.Fatalf("run -clear-data: %v", err)
	}
	for _, name := range []string{"cookies.json", "history.json", "session.json"} {
		if _, err := os.Stat(filepath.Join(statedir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s survives -clear-data", name)
		}
	}
	if _, err := os.Stat(filepath.Join(statedir, "bookmarks.json")); err != nil {
		t.Fatalf("bookmarks.json missing after -clear-data: %v", err)
	}
}

// TestRepaintTabReusesLayerOnSameExtent pins the incremental present path:
// repainting the same document must keep the tab's layer (and its tile
// cache and pool) instead of allocating afresh. Selection damage then
// stales only the selection's tiles: the flicker proof below settles every
// tile through real frames, drag-selects one word, and requires the rest
// to be reused with only dirty tiles re-rasterized.
func TestRepaintTabReusesLayerOnSameExtent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><p>stable content for repaint</p>` +
			`<p>filler line two</p><p>filler line three</p><p>filler line four</p>` +
			`<p>filler line five</p><p>filler line six</p><p>filler line seven</p>` +
			`<p>filler line eight</p><p>filler line nine</p><p>filler line ten</p>` +
			`<p>filler line eleven</p><p>filler line twelve</p><p>filler line thirteen</p>` +
			`<p>filler line fourteen</p><p>filler line fifteen</p><p>filler line sixteen</p>` +
			`<p>filler line seventeen</p><p>filler line eighteen</p><p>filler line nineteen</p>` +
			`<p>filler line twenty</p></body></html>`))
	}))
	defer server.Close()
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	f := newNavTestPath(t)
	f.config.width = 800
	f.config.dpr = 1
	f.zoom = 1
	_, _, _, sess, _, err := loadURLCtx(context.Background(), client, fonts, server.URL, 800, 600, 1, t.TempDir(), false, false)
	if err != nil {
		t.Fatalf("loadURLCtx: %v", err)
	}
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	f.repaintTab(tab, frame.Rect{})
	first := tab.Layer()
	if first == nil {
		t.Fatal("no layer after first repaintTab")
	}
	f.repaintTab(tab, frame.Rect{})
	second := tab.Layer()
	if second == nil {
		t.Fatal("no layer after second repaintTab")
	}
	if second != first {
		t.Fatal("repaintTab allocated a new layer for an unchanged extent: tile cache discarded")
	}

	// Settle every tile through real frames (BeginFrame/Submit/drain until
	// quiet), mirroring the raster harness drain.
	f.sched.SetViewport(frame.Viewport{Size: frame.Size{W: 800, H: 600}})
	settle := func(t *testing.T) {
		t.Helper()
		for i := 0; i < 500; i++ {
			plan, needed := f.sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
			if plan == nil {
				t.Fatal("BeginFrame returned no plan")
			}
			f.sched.Submit(needed)
			for {
				d, ok := f.pool.Poll()
				if !ok {
					break
				}
				_ = f.sched.HandleDone(d)
			}
			// Give the pool worker a chance to pick up jobs before
			// judging quiet, mirroring the raster harness drain.
			runtime.Gosched()
			if second.Grid.Stats().PaintingBytes == 0 {
				_, check := f.sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
				w := f.sched.Submit(check)
				if w.Needed == 0 {
					return
				}
			}
		}
		t.Fatal("tiles never settled")
	}
	settle(t)
	rasterized := f.sched.Stats().Rasterized
	matches := sess.Find("stable")
	if len(matches) == 0 {
		t.Fatal("Find(stable) found nothing to drag across")
	}
	m := matches[0]
	oldR, oldOK := sess.SelectionBounds()
	midY := (m.Y0 + m.Y1) / 2
	sess.SelectAt(m.X0+1, midY)
	sess.SelectTo(m.X1-1, midY)
	f.repaintTab(tab, f.selectionDamage(tab, oldR, oldOK))

	// Damage scoping, synchronously: the selection's tile is stale, a far
	// tile in the same layer is still current.
	selTile := frame.CoordFor(frame.Point{X: int32(m.X0) + 1, Y: int32(midY)}, frame.TileSize)
	if !second.Needs(selTile) {
		t.Fatal("selection tile not stale after a selection repaint")
	}
	b := second.Bounds
	farTile := frame.CoordFor(frame.Point{X: b.X1 - 10, Y: b.Y1 - 10}, frame.TileSize)
	if second.Needs(farTile) {
		t.Fatal("far tile stale after a word-sized selection change: damage escaped")
	}

	// The test document is one tile wide: one dirty tile is the whole
	// damage, so Needed and the re-raster delta cap at one plus margin while
	// any surviving reuse proves the cache lived. Layer identity above (same
	// pointer across repaints) plus far-tile freshness below are what fail
	// pre-fix, when every repaint allocated a cold layer.
	_, needed := f.sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
	w := f.sched.Submit(needed)
	if w.Needed > 2 {
		t.Fatalf("Needed = %d after a word-sized selection change, want only dirty tiles", w.Needed)
	}
	if w.Reused < 1 {
		t.Fatalf("Reused = %d after a word-sized selection change, want surviving reuse", w.Reused)
	}
	settle(t)
	if got := f.sched.Stats().Rasterized - rasterized; got > 2 {
		t.Fatalf("re-rasterized %d tiles for a word-sized selection change", got)
	}
}
