// Command goosie is v2's browser binary: the M1 frame path behind a window.
//
// M1 has no document loader, so what it draws is one of the synthetic scenes in
// paint: a checkerboard page with positioned text over it. That is not a placeholder
// in the binary's own terms - the frame path's criteria are all about tiles, budgets,
// and pacing, and a page whose contents are known analytically is what makes them
// measurable. The flags are the ones a real browser will have: the scene replaces the
// document, nothing else in the shape of the command changes.
//
// Two modes share one code path. Interactive mode opens a window and draws frames as
// the display asks for them. Gate mode (-gate) drives a fixed number of scroll frames
// through the same surface.Loop and reports the timings, which is how the nightly
// macOS job measures real vsync pacing without a finger on the glass.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/session"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

const (
	// gateScrollStep is -gate's per-frame scroll delta, in device pixels. It is the
	// same 100px the warm criterion in test/gate walks with, so the two runs
	// measure the same travel.
	gateScrollStep = 100
	// defaultFrames is -frames' default: the gate's 600-frame sweep.
	defaultFrames = 600
	// displayPeriod stands in for a 60Hz panel when the backend is paced by a clock
	// rather than by a display. A headless gate run measures the frame path against
	// the same interval the macOS shim will be held to.
	displayPeriod = 16 * time.Millisecond
	// benchPeriod is -bench's clock. It is not zero because the headless window paces
	// itself with a timer, and a period of zero means the default rather than as fast
	// as possible.
	benchPeriod = time.Microsecond
	// rasterQueue is how many tile jobs may be offered at once. The gate suite uses
	// the same depth, so a CLI run never refuses work a test run would have taken.
	rasterQueue = 4096
	// interactiveDocHeight is the document an interactive run scrolls, in device
	// pixels: the 20,000 px procedural page M1's first criterion names.
	interactiveDocHeight = 20_000
	// glyphAtlasBudget is per layer and is not one of M1's criteria. A tile is 256px
	// on a side, so 4 MiB holds more glyph masks than one screen of text needs.
	glyphAtlasBudget = 4 << 20
	// ringFrames is how many frame marks an interactive run keeps for its final
	// report: two seconds at 60Hz, which is the window the report is read against.
	ringFrames = 120
)

// navResult is the outcome of a per-tab navigation, delivered on navResults.
type navResult struct {
	tabID        uint64
	serial       uint64
	layer        *frame.Layer
	bgColor      frame.Color
	session      *engine.Session
	err          error
	url          string
	downloadPath string
	noHistory    bool
}

// downloadDone reports a completed download instead of a rendered document;
// the navigation goroutine converts it into navResult.downloadPath.
type downloadDone struct{ path string }

func (e downloadDone) Error() string { return "downloaded to " + e.path }

// errUsage marks a bad invocation, which the shell should see as exit status 2 rather
// than as a failure deep inside a run.
var errUsage = errors.New("usage")

// config is one parsed invocation.
type config struct {
	url         string
	gate        bool
	bench       bool
	screenshot  bool
	backend     string
	frames      int
	scene       string
	out         string
	width       int
	height      int
	dpr         float64
	downloadDir string
	private      bool
	profile      string
	sessionFile  string
	cookieFile   string
	bookmarkFile string
	historyFile  string
}

// stateDir is where this invocation keeps persistent state: the default
// profile lives in ~/.goosie, a named one in ~/.goosie/profiles/<name>.
func (c config) stateDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".goosie"
	}
	base := filepath.Join(home, ".goosie")
	if c.profile == "" {
		return base
	}
	return filepath.Join(base, "profiles", c.profile)
}

// devSize is the surface in device pixels: the units the frame path, the scheduler,
// and the shim all measure a viewport in.
func (c config) devSize() frame.Size {
	return frame.Size{
		W: int32(float64(c.width)*c.dpr + 0.5),
		H: int32(float64(c.height)*c.dpr + 0.5),
	}
}

// vsyncPeriod is the interval a clock-paced backend ticks at. A native backend is
// paced by its display and never reads this.
func (c config) vsyncPeriod() time.Duration {
	if c.bench {
		return benchPeriod
	}
	return displayPeriod
}

// paced reports whether this run is driven by a fixed frame count rather than by a
// person closing the window.
func (c config) paced() bool { return c.gate || c.bench || c.screenshot }

// parse turns os.Args into a config, and rejects the combinations that would draw
// something other than what was asked for.
func parse(args []string) (config, error) {
	var c config
	fs := flag.NewFlagSet("goosie", flag.ContinueOnError)
	fs.StringVar(&c.url, "url", "", "document to load")
	fs.BoolVar(&c.gate, "gate", false, "drive -frames scripted scroll frames at real pacing, then report")
	fs.BoolVar(&c.bench, "bench", false, "run frames as fast as the clock allows, headless, and report the rate")
	fs.BoolVar(&c.screenshot, "screenshot", false, "render -url to a PNG at -out and exit")
	fs.StringVar(&c.backend, "backend", "", "window backend: headless or native (empty means whatever this machine has)")
	fs.IntVar(&c.frames, "frames", defaultFrames, "frames to draw with -gate and -bench")
	fs.StringVar(&c.scene, "scene", "checkerboard", "synthetic document: checkerboard or plain")
	fs.StringVar(&c.out, "out", "", "write the frame report as JSON here, or - for stdout")
	fs.IntVar(&c.width, "width", 1440, "viewport width in CSS pixels")
	fs.IntVar(&c.height, "height", 900, "viewport height in CSS pixels")
	fs.Float64Var(&c.dpr, "dpr", 2, "device pixel ratio")
	dlDir, err := os.UserHomeDir()
	if err != nil || dlDir == "" {
		dlDir = "."
	} else {
		dlDir = filepath.Join(dlDir, "Downloads")
	}
	fs.StringVar(&c.downloadDir, "download-dir", dlDir, "directory where downloads are saved")
	fs.BoolVar(&c.private, "private", false, "run a private session: cookies are dropped on exit")
	fs.StringVar(&c.profile, "profile", "", "named profile: state lives under ~/.goosie/profiles/<name> instead of ~/.goosie")
	fs.StringVar(&c.sessionFile, "session-file", "", "file where the open tabs are saved between runs")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: goosie [-scene checkerboard] [-width 1440] [-height 900] [-dpr 2]")
		fmt.Fprintln(fs.Output(), "       goosie -gate -frames 600 -out gate.json")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return c, fmt.Errorf("%w: help requested", errUsage)
		}
		return c, fmt.Errorf("%w: %v", errUsage, err)
	}
	switch {
	case c.width <= 0 || c.height <= 0:
		return c, fmt.Errorf("%w: -width and -height must be positive", errUsage)
	case math.IsNaN(c.dpr) || c.dpr <= 0 || c.dpr > 8:
		return c, fmt.Errorf("%w: -dpr must be in (0, 8]", errUsage)
	case c.frames <= 0:
		return c, fmt.Errorf("%w: -frames must be positive", errUsage)
	case c.profile == "." || c.profile == ".." || strings.ContainsAny(c.profile, "/\\"):
		return c, fmt.Errorf("%w: -profile must be a bare name", errUsage)
	}
	if _, err := sceneSpec(c.scene); err != nil {
		return c, err
	}
	if c.bench || c.screenshot {
		// A rate run that depended on a panel's refresh would report the panel, and
		// the nightly job wants the machine. An explicit -backend still wins, so a
		// caller who means to measure a real display can say so with -gate.
		if fs.Lookup("backend").Value.String() == "" {
			c.backend = platform.Headless
		}
	}
	if c.screenshot {
		if c.out == "" {
			return c, fmt.Errorf("%w: -screenshot requires -out <path.png>", errUsage)
		}
		if c.url == "" {
			return c, fmt.Errorf("%w: -screenshot requires -url", errUsage)
		}
	}
	if c.sessionFile == "" {
		c.sessionFile = filepath.Join(c.stateDir(), "session.json")
	}
	c.cookieFile = filepath.Join(c.stateDir(), "cookies.json")
	c.bookmarkFile = filepath.Join(c.stateDir(), "bookmarks.json")
	c.historyFile = filepath.Join(c.stateDir(), "history.json")
	return c, nil
}

// sceneSpec maps -scene to a document. Two scenes is the whole list because two is
// what the criteria distinguish: a page of flat colour exercises the grid and the
// budget, and only a page with positioned glyphs exercises the atlas.
func sceneSpec(name string) (paint.SceneSpec, error) {
	switch name {
	case "checkerboard":
		return paint.SceneSpec{Checkerboard: true, TextRuns: paint.DefaultTextRuns}, nil
	case "plain":
		return paint.SceneSpec{Checkerboard: false, TextRuns: paint.DefaultTextRuns}, nil
	}
	return paint.SceneSpec{}, fmt.Errorf("%w: unknown -scene %q (want checkerboard or plain)", errUsage, name)
}

// framePath is the assembled pipeline: everything between a vsync and a Present, with
// the pieces a report has to name kept reachable.
type framePath struct {
	window   surface.Window
	loop     *surface.Loop
	sched    *raster.Scheduler
	pool     *raster.Pool
	composer *surface.Composer
	layer    *frame.Layer
	rec      *frame.FrameRecorder
	spec     paint.SceneSpec
	config   config
	toolbar  *toolbar.State
	tabMgr   *tabs.TabManager
	client   net.HTTP
	fonts    *raster.Fonts
	bookmarks *bookmarks.Store
	history   *history.Store
	started    time.Time
	navResults chan navResult
	zoom       float64
	findMatches []engine.Match
	findIdx     int
	selDrag     bool
	restoredActiveURL string
}

// build wires a scene, a grid, a pool, a composer, a scheduler, and a window into a
// frame path sized for c.
func build(c config) (*framePath, error) {
	dev := c.devSize()
	scale := float32(c.dpr)

	fonts, err := raster.NewFonts()
	if err != nil {
		return nil, fmt.Errorf("goosie: %w", err)
	}

	var client net.HTTP
	if c.private {
		client = net.DefaultPrivateClient()
	} else {
		persistent, err := net.DefaultClientWithCookies(c.cookieFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goosie: cookies: %v\n", err)
			client = net.DefaultClient()
		} else {
			client = persistent
		}
	}

	var bookmarkStore *bookmarks.Store
	var historyStore *history.Store
	if !c.private {
		bookmarkStore, err = bookmarks.Load(c.bookmarkFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goosie: bookmarks: %v\n", err)
			bookmarkStore = nil
		}
		historyStore, err = history.Load(c.historyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goosie: history: %v\n", err)
			historyStore = nil
		}
	}

	var layer *frame.Layer
	var spec paint.SceneSpec

	if c.url != "" {
		// Defer URL fetch until after the window opens. Start with a placeholder
		// layer so the user sees the toolbar immediately, not a blank wait.
		spec = paint.SceneSpec{DocHeight: dev.H}
		_, layer = paint.BuildLayer(spec)
	} else {
		spec, err = sceneSpec(c.scene)
		if err != nil {
			_ = client.Close()
			return nil, err
		}
		cols := spec.Cols
		if cols <= 0 {
			cols = paint.DefaultCols
		}
		if c.paced() {
			if spec.DocHeight <= 0 {
				spec.DocHeight = paint.DefaultDocHeight
			}
			spec.BudgetTiles = int64((spec.DocHeight+frame.TileSize-1)/frame.TileSize)*int64(cols) + 8
		} else {
			spec.DocHeight = interactiveDocHeight
		}
		_, layer = paint.BuildLayer(spec)
	}

	wp := raster.New(raster.DefaultWorkers(), rasterQueue, raster.DefaultRaster(fonts, raster.NewGlyphAtlas(glyphAtlasBudget, fonts)))
	wp.Start(context.Background())

	capacity := ringFrames
	if c.paced() {
		capacity = c.frames
	}
	f := &framePath{
		sched:      raster.NewScheduler(layer, wp, frame.Viewport{Size: dev}, scale, raster.Pref{}),
		pool:       wp,
		composer:   surface.NewComposer(dev, frame.NewBitmapPool(dev, 2)),
		layer:      layer,
		rec:        frame.NewFrameRecorder(capacity),
		spec:       spec,
		config:     c,
		client:     client,
		fonts:      fonts,
		bookmarks:  bookmarkStore,
		history:    historyStore,
		navResults: make(chan navResult, 16),
		zoom:       1.0,
	}
	if !c.paced() {
		go f.drainNavResults()
	}
	if !c.paced() {
		f.toolbar = toolbar.NewState(dev.W, fonts)
		f.tabMgr = tabs.NewManager(func() {
			if f.window != nil {
				_ = f.window.Close()
			}
		})
		f.tabMgr.OnChange = func() {
			f.syncTabToToolbar()
		}
		firstTab := f.tabMgr.NewTab()
		if c.url != "" {
			firstTab.URL = normalizeURL(c.url)
			firstTab.Title = firstTab.URL
		} else if !c.private {
			if st, err := session.Load(c.sessionFile); err != nil {
				fmt.Fprintf(os.Stderr, "goosie: session: %v\n", err)
			} else if len(st.Tabs) > 0 {
				f.applySessionState(st)
			}
		}
		f.syncTabToToolbar()

		f.toolbar.OnNavigate = func(rawURL string) {
			f.navigateTab(rawURL)
		}
		f.toolbar.OnTraverse = func(delta int) {
			f.traverseTab(delta)
		}
		f.toolbar.OnReload = func() {
			f.reloadTab()
		}
		f.toolbar.OnFindChanged = func(query string) {
			f.runFind(query)
		}
		f.toolbar.OnFindNext = func(backward bool) {
			f.findStep(backward)
		}
		f.toolbar.OnFindClose = func() {
			f.closeFind()
		}
		if cb := platform.NewClipboard(); cb != nil {
			f.toolbar.Clipboard = cb
		}
	}
	f.sched.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{layer}, Background: frame.RGB(248, 248, 248)})
	return f, nil
}

// syncTabToToolbar copies the active tab's state into the toolbar for display.
func (f *framePath) syncTabToToolbar() {
	if f.toolbar == nil || f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	f.toolbar.URL = tab.URL
	f.toolbar.Input = tab.URL
	f.toolbar.Cursor = len([]rune(tab.URL))
	f.toolbar.SelStart = f.toolbar.Cursor
	f.toolbar.SelEnd = f.toolbar.Cursor
	f.toolbar.History = tab.History
	f.toolbar.SetLoading(tab.Nav.Loading)
	f.toolbar.Error = tab.Error
}

// navigateTab loads a URL on the active tab.
func (f *framePath) navigateTab(rawURL string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	f.closeFind()
	u := normalizeURL(rawURL)

	tab.Nav.Mu.Lock()
	if tab.Nav.Cancel != nil {
		tab.Nav.Cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	tab.Nav.Cancel = cancel
	tab.Nav.Serial++
	serial := tab.Nav.Serial
	tab.Nav.Loading = true
	tab.Nav.Mu.Unlock()

	tab.Loading = true
	tab.Error = ""
	f.toolbar.SetLoading(true)
	f.toolbar.Error = ""

	go func() {
		layer, _, bgColor, sess, err := loadURLCtx(ctx, f.client, f.fonts, u, f.config.width, f.config.height, float32(f.config.dpr), f.config.downloadDir)
		res := navResult{tabID: tab.ID, serial: serial, url: u}
		var dd downloadDone
		if errors.As(err, &dd) {
			res.downloadPath = dd.path
		} else {
			res.err = err
		}
		res.layer, res.bgColor, res.session = layer, bgColor, sess
		f.navResults <- res
	}()
}

// drainNavResults reads navigation results from the channel and applies them.
func (f *framePath) drainNavResults() {
	for result := range f.navResults {
		f.applyNavResult(result)
	}
}

// applyNavResult applies a completed navigation result to its tab.
func (f *framePath) applyNavResult(result navResult) {
	tab := f.tabMgr.TabByID(result.tabID)
	if tab == nil {
		return
	}

	tab.Nav.Mu.Lock()
	if tab.Nav.Serial != result.serial {
		tab.Nav.Mu.Unlock()
		return
	}
	tab.Nav.Loading = false
	tab.Nav.Mu.Unlock()

	tab.Loading = false
	if result.downloadPath != "" {
		if f.tabMgr.Active() == tab {
			f.toolbar.SetLoading(false)
		}
		fmt.Fprintf(os.Stderr, "goosie: saved %s\n", result.downloadPath)
		return
	}
	if result.err != nil {
		tab.Error = result.err.Error()
		if f.tabMgr.Active() == tab {
			f.toolbar.SetLoading(false)
			f.toolbar.Error = result.err.Error()
		}
		fmt.Fprintln(os.Stderr, result.err)
		return
	}

	tab.Layer = result.layer
	tab.Session = result.session
	tab.BGColor = result.bgColor
	tab.URL = result.url
	tab.Title = result.url
	if !result.noHistory {
		tab.History.Push(result.url)
	}
	if f.history != nil {
		f.history.Record(result.url, result.url)
	}

	if f.tabMgr.Active() == tab {
		f.syncTabToToolbar()
		f.sched.SetPlan(frame.FramePlan{
			Serial:     result.serial,
			Layers:     []*frame.Layer{result.layer},
			Background: result.bgColor,
		})
	}
}

// toggleBookmark stars or unstars the active tab's page and persists immediately.
func (f *framePath) toggleBookmark() {
	tab := f.tabMgr.Active()
	if tab == nil || tab.URL == "" || f.bookmarks == nil {
		return
	}
	f.bookmarks.Toggle(tab.URL, tab.Title)
	if err := f.bookmarks.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "goosie: save bookmarks: %v\n", err)
	}
}

// traverseTab handles back/forward on the active tab.
func (f *framePath) traverseTab(delta int) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	var url string
	var ok bool
	if delta < 0 {
		url, ok = tab.History.Back()
	} else if delta > 0 {
		url, ok = tab.History.Forward()
	}
	if !ok || url == "" {
		return
	}
	f.navigateTabNoHistory(url)
}

// navigateTabNoHistory loads a URL without pushing to history.
func (f *framePath) navigateTabNoHistory(rawURL string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	f.closeFind()
	u := normalizeURL(rawURL)

	tab.Nav.Mu.Lock()
	if tab.Nav.Cancel != nil {
		tab.Nav.Cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	tab.Nav.Cancel = cancel
	tab.Nav.Serial++
	serial := tab.Nav.Serial
	tab.Nav.Loading = true
	tab.Nav.Mu.Unlock()

	tab.Loading = true
	tab.Error = ""
	f.toolbar.SetLoading(true)

	go func() {
		layer, _, bgColor, sess, err := loadURLCtx(ctx, f.client, f.fonts, u, f.config.width, f.config.height, float32(f.config.dpr), f.config.downloadDir)
		res := navResult{tabID: tab.ID, serial: serial, url: u, noHistory: true}
		var dd downloadDone
		if errors.As(err, &dd) {
			res.downloadPath = dd.path
		} else {
			res.err = err
		}
		res.layer, res.bgColor, res.session = layer, bgColor, sess
		f.navResults <- res
	}()
}

// reloadTab re-fetches the active tab's URL.
func (f *framePath) reloadTab() {
	tab := f.tabMgr.Active()
	if tab == nil || tab.URL == "" {
		return
	}
	f.navigateTabNoHistory(tab.URL)
}

// applySessionState rebuilds the tab set from a saved session. The first saved
// tab overwrites the fresh New Tab; background tabs stay unloaded until shown.
func (f *framePath) applySessionState(st session.State) {
	tabsList := f.tabMgr.Tabs()
	if len(tabsList) == 0 || len(st.Tabs) == 0 {
		return
	}
	applyTabState(tabsList[0], st.Tabs[0])
	for _, ts := range st.Tabs[1:] {
		applyTabState(f.tabMgr.NewTab(), ts)
	}
	if st.Active > 0 {
		restored := f.tabMgr.Tabs()
		if st.Active < len(restored) {
			f.tabMgr.SwitchTo(restored[st.Active].ID)
		}
	}
	if active := f.tabMgr.Active(); active != nil && active.URL != "" {
		f.restoredActiveURL = active.URL
	}
}

func applyTabState(t *tabs.Tab, ts session.TabState) {
	t.URL = ts.URL
	t.Title = ts.Title
	t.History = toolbar.RestoreHistory(ts.History, ts.HistoryIndex)
	if t.Title == "" {
		t.Title = "New Tab"
	}
}

// sessionState snapshots the open tabs for saving.
func sessionState(f *framePath) session.State {
	var st session.State
	for _, tab := range f.tabMgr.Tabs() {
		entries, idx := tab.History.Entries()
		st.Tabs = append(st.Tabs, session.TabState{
			URL:          tab.URL,
			Title:        tab.Title,
			History:      entries,
			HistoryIndex: idx,
		})
	}
	st.Active = f.tabMgr.ActiveIndex()
	return st
}

// handleResize reflows and repaints the active tab after the window is resized.
func (f *framePath) handleResize(contentW, contentH int) {
	f.config.width = contentW
	f.config.height = contentH

	tab := f.tabMgr.Active()
	if tab == nil || tab.Session == nil {
		return
	}

	if err := tab.Session.Reflow(float32(contentW)); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		return
	}

	f.repaintTab(tab)
}

// repaintTab rebuilds the active tab's layer from its session and hands the
// plan to the scheduler.
func (f *framePath) repaintTab(tab *tabs.Tab) {
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	list, err := tab.Session.PaintChecked(effectiveDPR)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: paint:", err)
		return
	}

	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: budget:", err)
		return
	}

	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)

	tab.Layer = layer
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor,
	})
}

// docPoint converts a content-area point to document coordinates through the
// active zoom and scroll offset.
func (f *framePath) docPoint(contentX, contentY int32) (float32, float32) {
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	vp := f.sched.Viewport()
	return float32(contentX)/effectiveDPR + float32(vp.Offset.X)/effectiveDPR,
		float32(contentY)/effectiveDPR + float32(vp.Offset.Y)/effectiveDPR
}

// focusClick focuses or blurs a form control under a content-area click and
// repaints when focus state changed.
func (f *framePath) focusClick(contentX, contentY int32) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session == nil {
		return false
	}
	dx, dy := f.docPoint(contentX, contentY)
	if !tab.Session.FocusControl(dx, dy) {
		return false
	}
	f.setWindowIME(tab.Session.Focused() != nil)
	f.repaintTab(tab)
	return true
}

// setWindowIME turns the platform input context on while a document control has
// focus and off whenever focus moves anywhere else, so an IME can only compose
// into the document.
func (f *framePath) setWindowIME(on bool) {
	if f.window != nil {
		f.window.SetIME(on)
	}
}

// imeEvent routes a platform composition event to the active tab's session. A
// marked update repaints the preview; a commit reflows the value. Both are
// consumed whether or not the session was waiting, because the platform input
// context is only ever on for the active tab's control.
func (f *framePath) imeEvent(ev surface.Event) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session == nil {
		return false
	}
	switch ev.IME {
	case surface.IMEMarked:
		if tab.Session.SetMarked(ev.Text) {
			f.repaintTab(tab)
		}
		return true
	case surface.IMECommit:
		if tab.Session.CommitText(ev.Text) {
			if err := tab.Session.Reflow(float32(f.config.width)); err != nil {
				fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
			}
		}
		f.repaintTab(tab)
		return true
	}
	return false
}

// dragSelect drives text selection from content-area pointer events. A press
// starts a selection unless a form control is focused, motion extends the
// active one, and release just ends the gesture.
func (f *framePath) dragSelect(action surface.PointerAction, contentX, contentY int32) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session == nil {
		return false
	}
	switch action {
	case surface.PointerPress:
		if tab.Session.Focused() != nil {
			return false
		}
		dx, dy := f.docPoint(contentX, contentY)
		tab.Session.SelectAt(dx, dy)
		f.selDrag = true
		f.repaintTab(tab)
		return true
	case surface.PointerMotion:
		if !f.selDrag {
			return false
		}
		dx, dy := f.docPoint(contentX, contentY)
		if !tab.Session.SelectTo(dx, dy) {
			return true
		}
		f.repaintTab(tab)
		return true
	case surface.PointerRelease:
		wasDragging := f.selDrag
		f.selDrag = false
		return wasDragging
	}
	return false
}

// copySelection puts the selected text on the system clipboard.
func (f *framePath) copySelection() bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session == nil || !tab.Session.HasSelection() {
		return false
	}
	if cb := platform.NewClipboard(); cb != nil {
		cb.Write(tab.Session.SelectionText())
	}
	return true
}

// contentKey applies a keyboard edit to the focused control. The event is
// consumed whenever a control is focused so the address bar is not the only
// text sink; the tab reflows when the value changed and repaints otherwise.
func (f *framePath) contentKey(key rune, mods surface.KeyMod) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session == nil || tab.Session.Focused() == nil {
		return false
	}

	cmd := mods&surface.ModCommand != 0 || mods&surface.ModControl != 0
	if cmd {
		return false
	}

	var action engine.EditAction
	switch {
	case key == 0xF702:
		action = engine.EditLeft
	case key == 0xF703:
		action = engine.EditRight
	case key == 0xF700:
		action = engine.EditUp
	case key == 0xF701:
		action = engine.EditDown
	case key == 0xF704:
		action = engine.EditHome
	case key == 0xF705:
		action = engine.EditEnd
	case key == 0x7f || key == '\b':
		action = engine.EditBackspace
	case key == '\r' || key == '\n':
		action = engine.EditEnter
	case key == 0x1b:
		action = engine.EditEscape
	case key >= 0x20:
		action = engine.EditRune
	default:
		return false
	}

	changed := tab.Session.Edit(action, key)
	if changed {
		if action == engine.EditEscape {
			f.setWindowIME(false)
		}
		if err := tab.Session.Reflow(float32(f.config.width)); err != nil {
			fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
			return true
		}
	}
	f.repaintTab(tab)
	return true
}

func (f *framePath) handleZoom(delta float64) {
	if delta == 0 {
		f.zoom = 1.0
	} else {
		f.zoom += delta
		if f.zoom < 0.25 {
			f.zoom = 0.25
		} else if f.zoom > 4.0 {
			f.zoom = 4.0
		}
	}
	f.handleResize(f.config.width, f.config.height)
}

func (f *framePath) handleLinkClick(href string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	f.navigateTab(href)
}

func (f *framePath) hitTestLink(contentX, contentY int32) string {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session == nil {
		return ""
	}
	docX, docY := f.docPoint(contentX, contentY)
	return tab.Session.HitTestLink(docX, docY)
}

// runFind recomputes the match set for a new find query on the active tab and
// jumps to the first hit.
func (f *framePath) runFind(query string) {
	if f.toolbar == nil {
		return
	}
	f.findMatches = nil
	f.findIdx = 0
	f.toolbar.FindTotal = 0
	f.toolbar.FindIndex = 0
	if strings.TrimSpace(query) == "" {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session == nil {
		return
	}
	f.findMatches = tab.Session.Find(query)
	f.toolbar.FindTotal = len(f.findMatches)
	if len(f.findMatches) > 0 {
		f.scrollToMatch(0)
	}
}

// findStep moves to the next (or previous) match, wrapping around.
func (f *framePath) findStep(backward bool) {
	if len(f.findMatches) == 0 {
		return
	}
	if backward {
		f.findIdx--
		if f.findIdx < 0 {
			f.findIdx = len(f.findMatches) - 1
		}
	} else {
		f.findIdx++
		if f.findIdx >= len(f.findMatches) {
			f.findIdx = 0
		}
	}
	f.toolbar.FindIndex = f.findIdx
	f.scrollToMatch(f.findIdx)
}

// scrollToMatch centers the match's line in the upper part of the viewport.
func (f *framePath) scrollToMatch(idx int) {
	if idx < 0 || idx >= len(f.findMatches) {
		return
	}
	m := f.findMatches[idx]
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	size := f.config.devSize()
	offsetY := int32(m.Y0*effectiveDPR) - size.H/4
	if offsetY < 0 {
		offsetY = 0
	}
	f.sched.SetViewport(frame.Viewport{Offset: frame.Point{Y: offsetY}, Size: size})
}

// closeFind drops the match state; navigation and tab switches end find mode.
func (f *framePath) closeFind() {
	f.findMatches = nil
	f.findIdx = 0
	if f.toolbar == nil {
		return
	}
	f.toolbar.FindTotal = 0
	f.toolbar.FindIndex = 0
	if f.toolbar.FindActive {
		f.toolbar.CloseFind()
	}
}

// switchTab saves the current tab's scroll and switches to the given tab.
func (f *framePath) switchTab(id uint64) {
	f.closeFind()
	f.setWindowIME(false)
	cur := f.tabMgr.Active()
	if cur != nil {
		vp := f.sched.Viewport()
		cur.ScrollY = vp.Offset.Y
	}
	f.tabMgr.SwitchTo(id)
	newTab := f.tabMgr.Active()
	if newTab == nil {
		return
	}
	f.syncTabToToolbar()
	if newTab.Layer != nil {
		f.sched.SetPlan(frame.FramePlan{
			Serial:     newTab.Nav.Serial,
			Layers:     []*frame.Layer{newTab.Layer},
			Background: newTab.BGColor,
		})
	}
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: newTab.ScrollY},
		Size:   f.config.devSize(),
	})
}

// newTab creates a new tab and switches to it.
func (f *framePath) newTab() {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	cur := f.tabMgr.Active()
	if cur != nil {
		vp := f.sched.Viewport()
		cur.ScrollY = vp.Offset.Y
	}
	tab := f.tabMgr.NewTab()
	f.tabMgr.SwitchTo(tab.ID)
	f.syncTabToToolbar()
	spec := paint.SceneSpec{DocHeight: f.config.devSize().H}
	_, layer := paint.BuildLayer(spec)
	tab.Layer = layer
	tab.BGColor = frame.RGB(255, 255, 255)
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor,
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: 0},
		Size:   f.config.devSize(),
	})
}

// closeTab closes a tab by ID and switches to the remaining active tab.
func (f *framePath) closeTab(id uint64) {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	if tab := f.tabMgr.TabByID(id); tab != nil {
		tab.Nav.Mu.Lock()
		if tab.Nav.Cancel != nil {
			tab.Nav.Cancel()
			tab.Nav.Cancel = nil
		}
		tab.Nav.Mu.Unlock()
	}
	f.tabMgr.CloseTab(id)
	if f.tabMgr.Count() > 0 {
		tab := f.tabMgr.Active()
		f.syncTabToToolbar()
		if tab.Layer != nil {
			f.sched.SetPlan(frame.FramePlan{
				Serial:     tab.Nav.Serial,
				Layers:     []*frame.Layer{tab.Layer},
				Background: tab.BGColor,
			})
		}
		f.sched.SetViewport(frame.Viewport{
			Offset: frame.Point{Y: tab.ScrollY},
			Size:   f.config.devSize(),
		})
	}
}

// loadURLCtx is like loadURL but respects context cancellation.
func loadURLCtx(ctx context.Context, client net.HTTP, fonts *raster.Fonts, rawURL string, viewportW, viewportH int, scale float32, downloadDir string) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, error) {
	if err := engine.ValidateViewport(viewportW, viewportH, float64(scale)); err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, fmt.Errorf("goosie: viewport: %w", err)
	}
	// Check context before starting the fetch.
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, ctx.Err()
	}
	resp, err := client.Get(ctx, rawURL)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, fmt.Errorf("goosie: fetch %s: %w", rawURL, err)
	}
	if download.ShouldDownload(resp.Headers["Content-Type"], resp.Headers["Content-Disposition"]) {
		name := download.FileName(resp.Headers["Content-Disposition"], resp.URL)
		savedPath, saveErr := download.Save(downloadDir, name, resp.Body)
		if saveErr != nil {
			return nil, paint.SceneSpec{}, frame.Color(0), nil, saveErr
		}
		return nil, paint.SceneSpec{}, frame.Color(0), nil, downloadDone{path: savedPath}
	}
	// Linked style sheets are fetched with the same client and the same
	// cancellation: the linker sees absolute http(s) URLs only, because the
	// engine resolves hrefs against the final document URL before calling.
	linker := func(base, href string) (string, error) {
		resp, err := client.Get(ctx, href)
		if err != nil {
			return "", err
		}
		if refused := resp.Headers["Content-Type"]; net.StyleSheetRefused(refused) {
			return "", fmt.Errorf("goosie: %s is served as %q, not as a style sheet", href, refused)
		}
		return resp.Text(), nil
	}
	// Image subresources use the same client and cancellation. The engine hands
	// absolute http(s) URLs only, so the fetcher just reads the response bytes.
	imageFetcher := func(base, url string) ([]byte, error) {
		resp, err := client.Get(ctx, url)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	// @font-face resources use the same client. The fetcher is the same shape
	// as the image fetcher; the engine resolves src URLs before calling.
	fontFetcher := func(base, url string) ([]byte, error) {
		resp, err := client.Get(ctx, url)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	// Check context after the fetch in case it was cancelled during the network call.
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, ctx.Err()
	}
	sess, err := engine.NewSession(resp.Text(), nil, float32(viewportW),
		engine.WithMetrics(fonts),
		engine.WithViewportH(float32(viewportH)),
		engine.WithLinkedCSS(resp.URL, linker),
		engine.WithImages(resp.URL, imageFetcher),
		engine.WithCustomFontLoading(resp.URL, fontFetcher, fonts))
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, fmt.Errorf("goosie: build session: %w", err)
	}
	list, err := sess.PaintChecked(scale)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, fmt.Errorf("goosie: paint document: %w", err)
	}
	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, fmt.Errorf("goosie: size document cache: %w", err)
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)
	return layer, paint.SceneSpec{DocHeight: extent.H()}, sess.BackgroundColor(), sess, nil
}

// normalizeURL adds https:// to bare domains so the fetcher has a scheme to dial.
func normalizeURL(raw string) string {
	if raw == "" {
		return raw
	}
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "file://") {
		return raw
	}
	return "https://" + raw
}

// openWindow picks the window last, because a backend that cannot open one is a
// property of the machine the report has to state rather than a construction error.
func (f *framePath) openWindow() error {
	w, err := platform.Select(platform.Options{
		Backend:     f.config.backend,
		Size:        f.config.devSize(),
		Scale:       float32(f.config.dpr),
		VsyncPeriod: f.config.vsyncPeriod(),
		Title:       "Goosie",
	})
	if err != nil {
		return fmt.Errorf("goosie: %w", err)
	}
	if f.toolbar != nil && f.tabMgr != nil {
		f.window = newChromeWindow(w, f.toolbar, f.tabMgr,
			func(id uint64) { f.switchTab(id) },
			func() { f.newTab() },
			func(id uint64) { f.closeTab(id) },
			f.handleResize,
			f.handleZoom,
			f.handleLinkClick,
			f.toggleBookmark,
			f.focusClick,
			f.contentKey,
			f.dragSelect,
			f.imeEvent,
			f.copySelection,
			f.hitTestLink,
			f.fonts,
		)
	} else {
		f.window = w
	}
	f.loop = surface.NewLoop(f.window, f.sched, f.composer, f.rec)
	return nil
}

// main pins this goroutine to the process's main thread before anything can open a
// window, because AppKit requires the main thread and says so by aborting the process
// rather than by returning an error.
func main() {
	runtime.LockOSThread()
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "goosie:", err)
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	c, err := parse(args)
	if err != nil {
		return err
	}
	f, err := build(c)
	if err != nil {
		return err
	}
	defer func() { _ = f.pool.Close() }()
	defer func() {
		if err := f.client.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "goosie:", err)
		}
	}()

	if err := f.openWindow(); err != nil {
		return err
	}
	defer func() { _ = f.window.Close() }()

	// If a URL was provided, start loading it now that the window is visible.
	// The user sees the toolbar immediately while the page fetches in the background.
	// A screenshot run has nobody to watch the background load: the fixed frame
	// count would present synthetic tiles before the document lands, so it loads
	// synchronously and the run captures the page it was asked for.
	if c.url != "" && !c.gate && !c.bench {
		if c.screenshot {
			layer, _, bgColor, _, err := loadURLCtx(context.Background(), f.client, f.fonts, normalizeURL(c.url), c.width, c.height, float32(c.dpr), c.downloadDir)
			if err != nil {
				return err
			}
			f.sched.SetPlan(frame.FramePlan{
				Serial:     1,
				Layers:     []*frame.Layer{layer},
				Background: bgColor,
			})
		} else {
			f.navigateTab(c.url)
		}
	} else if f.restoredActiveURL != "" {
		f.navigateTabNoHistory(f.restoredActiveURL)
	}

	// A paced run cannot wait for a finger, so the driver stamps the scroll onto the
	// ticks the display already produces. Everything downstream of it - the loop, the
	// scheduler, the composer, the present - is the code an interactive run executes.
	var d *driver
	var finish chan struct{}
	if c.gate || c.bench {
		d = newDriver(f, c.frames)
		f.loop = surface.NewLoop(d, f.sched, f.composer, f.rec)
		finish = d.done
	} else if c.screenshot {
		sd := newSteadyDriver(f, c.frames)
		f.loop = surface.NewLoop(sd, f.sched, f.composer, f.rec)
		sd.watchIdle(func() bool {
			// Idle is not "nothing needed": a frame can want no tiles while the
			// tiles it is waiting on are still on order at a worker. Both halves
			// have to be clear before the pixels in the PNG are all there.
			st := f.loop.Stats()
			return st.Last.Needed == 0 && st.Last.PaintingBytes == 0
		})
		finish = sd.done
		defer func() {
			// After the loop exits, write the PNG from the headless window.
			if hw, ok := f.window.(interface{ WritePNG(string) error }); ok {
				if err := hw.WritePNG(c.out); err != nil {
					fmt.Fprintf(os.Stderr, "goosie: write screenshot: %v\n", err)
				}
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loopErr := make(chan error, 1)
	stopped := make(chan struct{})
	f.started = time.Now()
	go func() {
		err := f.loop.Run(ctx)
		loopErr <- err
		close(stopped)
	}()

	// Whoever ends the run says so here, and it cannot be the main thread's own
	// goroutine: on a platform with a run loop the main thread is busy turning it, and
	// closing the window is what stops that loop. An interactive run has no frame
	// count to finish on, so its finish channel stays nil and never readies.
	go func() {
		select {
		case <-stopped: // the window's events ran out, which is an orderly shutdown
		case <-sig:
		case <-finish: // the paced run reached its frame count
		}
		// Cancel, then wait for the loop to leave the frame it is drawing, then close.
		// Closing first would put a Present against a closed window in front of the
		// loop's next look at the context, and a run that ended because it finished its
		// frames would exit non-zero for a present nobody asked for.
		cancel()
		<-stopped
		_ = f.window.Close()
	}()

	if r, ok := f.window.(platform.Runner); ok {
		// A backend that owns a run loop gets this thread, which is the one pinned to
		// the process's main thread. Returning from Run means the window is gone.
		r.Run()
	}
	<-stopped
	if !c.paced() && !c.private {
		if err := session.Save(c.sessionFile, sessionState(f)); err != nil {
			fmt.Fprintf(os.Stderr, "goosie: save session: %v\n", err)
		}
		if f.history != nil {
			if err := f.history.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "goosie: save history: %v\n", err)
			}
		}
	}
	if err := <-loopErr; err != nil {
		return fmt.Errorf("goosie: %w", err)
	}
	if c.screenshot {
		return nil
	}
	return f.report(d)
}
