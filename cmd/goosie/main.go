// Command goosie is v2's browser binary: the document engine behind a window.
//
// An interactive run opens a window on a blank new tab and draws whatever the
// address bar or -url loads into it. The synthetic scenes in paint are test
// documents: a checkerboard page with positioned text over it, whose contents
// are known analytically, which is what makes the frame path's criteria about
// tiles, budgets, and pacing measurable. A paced run always draws one, and
// -scene names one for an interactive demo; nothing else in the shape of the
// command changes.
//
// Two modes share one code path. Interactive mode opens a window and draws frames as
// the display asks for them. Gate mode (-gate) drives a fixed number of scroll frames
// through the same surface.Loop and reports the timings, which is how the nightly
// macOS job measures real vsync pacing without a finger on the glass.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/js"
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

// processStart is the instant this process began its own work. Package
// initialisation runs before main, so it is closer to exec than any timestamp taken
// inside main() can be, and it is what the report's startup_ms measures against:
// "sub-second cold start" as one number a gate can read. What sits outside it is
// kernel exec, dynamic loading and the runtime's own start-up.
var processStart = time.Now()

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
	// traverse commits a back/forward step on success: the delta (-1/+1) and
	// the cursor index the peek saw. A failed traversal leaves the cursor
	// where it was instead of stranding it on a page that never loaded.
	traverse     int
	traverseBase int
}

// downloadDone reports a completed download instead of a rendered document;
// the navigation goroutine converts it into navResult.downloadPath.
type downloadDone struct{ path string }

func (e downloadDone) Error() string { return "downloaded to " + e.path }

// imgResult reports that a session's deferred images have landed; the drain
// goroutine reflows and repaints the owning tab if it is still showing the
// same navigation.
type imgResult struct {
	tabID   uint64
	serial  uint64
	session *engine.Session
}

// errUsage marks a bad invocation, which the shell should see as exit status 2 rather
// than as a failure deep inside a run.
var errUsage = errors.New("usage")

// config is one parsed invocation.
type config struct {
	url               string
	gate              bool
	bench             bool
	screenshot        bool
	printPDF          string
	backend           string
	frames            int
	scene             string
	sceneSet          bool
	out               string
	width             int
	height            int
	dpr               float64
	downloadDir       string
	private           bool
	profile           string
	clearData         bool
	sessionFile       string
	sessionActiveFile string
	cookieFile        string
	bookmarkFile      string
	historyFile       string
	theme             string
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
	fs.StringVar(&c.printPDF, "print-pdf", "", "export document to PDF at path and exit")
	fs.StringVar(&c.backend, "backend", "", "window backend: headless or native (empty means whatever this machine has)")
	fs.IntVar(&c.frames, "frames", defaultFrames, "frames to draw with -gate and -bench")
	fs.StringVar(&c.scene, "scene", "checkerboard", "synthetic test document: checkerboard or plain")
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
	fs.StringVar(&c.theme, "theme", "system", "appearance theme: system, light, dark, high-contrast")
	fs.BoolVar(&c.clearData, "clear-data", false, "clear browsing data (cookies, history, session; keeps bookmarks) and exit")
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
	fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "scene" {
			c.sceneSet = true
		}
	})
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
	if c.printPDF != "" {
		if fs.Lookup("backend").Value.String() == "" {
			c.backend = platform.Headless
		}
		if c.url == "" && len(fs.Args()) > 0 {
			c.url = fs.Args()[0]
		}
	}
	if c.sessionFile == "" {
		c.sessionFile = filepath.Join(c.stateDir(), "session.json")
	}
	c.sessionActiveFile = filepath.Join(c.stateDir(), "session_active.json")
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

// blankLayer is the new-tab document: one white fill the size of the viewport.
// The synthetic scene builder has no empty setting - every zero field means a
// default, so even a bare spec builds cells, borders, and text - and a blank
// page therefore gets its own one-command list. The tile budget covers the
// viewport once over, plus the slack a scrolled page would want.
func blankLayer(dev frame.Size) (*paint.LayerDL, *frame.Layer) {
	list := paint.NewList(1)
	list.Append(paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(0, 0, dev.W, dev.H),
		Color: frame.RGB(255, 255, 255),
	})
	dl := list.Build(1)
	cols := (dev.W + frame.TileSize - 1) / frame.TileSize
	rows := (dev.H + frame.TileSize - 1) / frame.TileSize
	budgetTiles := int64(rows*cols) + 8
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, int(budgetTiles))
	layer := frame.NewLayer(1, dl.Extent(), budgetTiles*frame.TileSizeBytes(), pool)
	layer.SetContent(dl)
	return dl, layer
}

// framePath is the assembled pipeline: everything between a vsync and a Present, with
// the pieces a report has to name kept reachable.
type framePath struct {
	window        surface.Window
	loop          *surface.Loop
	sched         *raster.Scheduler
	pool          *raster.Pool
	composer      *surface.Composer
	layer         *frame.Layer
	rec           *frame.FrameRecorder
	spec          paint.SceneSpec
	config        config
	toolbar       *toolbar.State
	tabMgr        *tabs.TabManager
	client        net.HTTP
	privateClient net.HTTP
	fonts         *raster.Fonts
	bookmarks     *bookmarks.Store
	history       *history.Store
	downloads     *download.Manager
	started       time.Time
	navResults    chan navResult
	imgResults    chan imgResult
	zoom          float64
	// runCtx roots navigation contexts so shutdown cancels in-flight loads
	// (they used to hang off context.Background and outlive the window);
	// navWg joins them before the window closes. closeTab still cancels a
	// tab's own load immediately without waiting on the network.
	runCtx    context.Context
	runCancel context.CancelFunc
	navWg     sync.WaitGroup
	// document and docHeight say what the run depicts: the page's URL or "" for a synthetic
	// scene, and its laid-out height in device pixels. Both go into the artifact, which
	// otherwise carries timings with no statement of what they were taken on.
	document          string
	docHeight         int32
	findMatches       []engine.Match
	findIdx           int
	selDrag           bool
	restoredActiveURL string
	storage           *js.OriginStorageRegistry
	incognitoStorage  *js.OriginStorageRegistry
	emulationPreset   *DevicePreset
	theme             string
	crashedSession    *session.State
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

	// The synthetic scenes are test documents: a paced run measures the frame
	// path against them, and an explicit -scene asks for one by name. An
	// interactive run with no URL opens on the same blank page the new-tab
	// button creates, and one with a URL starts on a placeholder layer so the
	// user sees the toolbar immediately, not a blank wait.
	if c.url == "" && (c.paced() || c.sceneSet) {
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
	} else {
		spec = paint.SceneSpec{DocHeight: dev.H}
		_, layer = blankLayer(dev)
	}

	wp := raster.New(raster.DefaultWorkers(), rasterQueue, raster.DefaultRaster(fonts, raster.NewGlyphAtlas(glyphAtlasBudget, fonts)))
	wp.Start(context.Background())

	capacity := ringFrames
	if c.paced() {
		capacity = c.frames
	}
	rec := frame.NewFrameRecorder(capacity)
	// The recorder writes the artifact the pacing gate reads, so the cold start it
	// reports has to be measured by the same instrument that measured the frames -
	// and the one fact it cannot observe is when the process began.
	rec.SetStartupReference(processStart)
	f := &framePath{
		sched:            raster.NewScheduler(layer, wp, frame.Viewport{Size: dev}, scale, raster.Pref{}),
		pool:             wp,
		composer:         surface.NewComposer(dev, frame.NewBitmapPool(dev, 2)),
		layer:            layer,
		docHeight:        spec.DocHeight,
		rec:              rec,
		spec:             spec,
		config:           c,
		client:           client,
		privateClient:    net.DefaultPrivateClient(),
		fonts:            fonts,
		bookmarks:        bookmarkStore,
		history:          historyStore,
		downloads:        download.NewManager(c.downloadDir),
		navResults:       make(chan navResult, 16),
		imgResults:       make(chan imgResult, 16),
		zoom:             1.0,
		storage:          js.NewOriginStorageRegistry(filepath.Join(c.stateDir(), "storage"), false),
		incognitoStorage: js.NewOriginStorageRegistry("", true),
		theme:            c.theme,
	}
	if f.theme == "" {
		f.theme = "system"
	}
	if !c.paced() {
		go f.drainNavResults()
		go f.drainImageResults()
	}
	if !c.paced() {
		f.toolbar = toolbar.NewState(dev.W, fonts)
		f.toolbar.SetScale(int32(c.dpr + 0.5))
		f.tabMgr = tabs.NewManager(func() {
			if f.window != nil {
				_ = f.window.Close()
			}
		})
		f.tabMgr.OnChange = func() {
			f.syncTabToToolbar()
			f.saveActiveSession()
		}
		firstTab := f.tabMgr.NewTab()
		firstTab.SetLayer(layer)
		if f.isDarkMode() {
			firstTab.SetBGColor(frame.RGB(24, 24, 24))
		} else {
			firstTab.SetBGColor(frame.RGB(255, 255, 255))
		}
		if !c.private && !c.paced() && c.sessionActiveFile != "" {
			if info, err := os.Stat(c.sessionActiveFile); err == nil && !info.IsDir() {
				if st, err := session.Load(c.sessionActiveFile); err == nil && len(st.Tabs) > 0 {
					f.crashedSession = &st
				}
			}
		}
		if c.url != "" {
			firstTab.SetURL(normalizeURL(c.url))
			firstTab.SetTitle(firstTab.URL())
		} else if f.crashedSession != nil {
			firstTab.SetURL("goosie://restore")
			firstTab.SetTitle("Restore Session - Goosie")
			f.restoredActiveURL = "goosie://restore"
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
		f.toolbar.OnDownloads = func() {
			f.navigateTab("goosie://downloads")
		}
		if f.downloads != nil {
			f.downloads.Subscribe(func(it *download.Item) {
				if f.toolbar != nil {
					f.toolbar.SetDownloadStats(f.downloads.ActiveCount(), f.downloads.TotalCount(), f.downloads.OverallProgress())
				}
			})
		}
		f.toolbar.OnQuerySuggest = func(query string) []toolbar.Suggestion {
			return f.querySuggestions(query)
		}
		f.toolbar.OnResetZoom = func() {
			f.handleZoom(0)
		}
		if cb := platform.NewClipboard(); cb != nil {
			f.toolbar.Clipboard = cb
		}
	}
	bg := frame.RGB(248, 248, 248)
	if !c.paced() {
		bg = frame.RGB(255, 255, 255)
	}
	f.sched.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{layer}, Background: bg})
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
	tab.Nav.Mu.Lock()
	loading := tab.Nav.Loading
	tab.Nav.Mu.Unlock()
	f.toolbar.SyncFromTab(tab.URL(), loading, tab.Error(), tab.History())
	f.toolbar.SetIncognito(tab.Incognito())
}

// clientForTab returns the ephemeral private client for incognito tabs or the default client.
func (f *framePath) clientForTab(tab *tabs.Tab) net.HTTP {
	if tab != nil && tab.Incognito() && f.privateClient != nil {
		return f.privateClient
	}
	return f.client
}

// newIncognitoTab opens a new private incognito tab.
func (f *framePath) newIncognitoTab() *tabs.Tab {
	tab := f.tabMgr.NewIncognitoTab()
	dev := frame.Size{W: int32(f.config.width), H: int32(f.config.height)}
	_, layer := blankLayer(dev)
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(40, 44, 52))
	f.switchTab(tab.ID())
	return tab
}

// isDarkMode reports whether dark appearance mode is active.
func (f *framePath) isDarkMode() bool {
	if f == nil {
		return false
	}
	switch strings.ToLower(f.theme) {
	case "dark":
		return true
	case "light", "high-contrast":
		return false
	default:
		return false
	}
}

// isHighContrast reports whether high contrast appearance mode is active.
func (f *framePath) isHighContrast() bool {
	if f == nil {
		return false
	}
	return strings.ToLower(f.theme) == "high-contrast"
}

// setTheme updates the browser appearance theme.
func (f *framePath) setTheme(theme string) {
	if f == nil {
		return
	}
	f.theme = strings.ToLower(strings.TrimSpace(theme))
	if f.tabMgr != nil {
		for _, tab := range f.tabMgr.Tabs() {
			if tab.Incognito() {
				tab.SetBGColor(frame.RGB(40, 44, 52))
			} else if f.isDarkMode() {
				tab.SetBGColor(frame.RGB(24, 24, 24))
			} else {
				tab.SetBGColor(frame.RGB(255, 255, 255))
			}
			if s := tab.Session(); s != nil {
				s.SetDarkMode(f.isDarkMode())
				s.SetHighContrast(f.isHighContrast())
			}
		}
	}
	if f.client != nil {
		f.reloadTab()
	}
}

// querySuggestions searches bookmarks, visit history, and search engine completions for omnibox query.
func (f *framePath) querySuggestions(query string) []toolbar.Suggestion {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	lowerQ := strings.ToLower(q)
	var out []toolbar.Suggestion
	seen := make(map[string]bool)

	// 1. Search Google query suggestion
	searchURL := "https://www.google.com/search?q=" + url.QueryEscape(q)
	out = append(out, toolbar.Suggestion{
		Title: "Search Google for " + q,
		URL:   searchURL,
		Kind:  toolbar.SuggestSearch,
	})
	seen[searchURL] = true

	// Internal system pages (goosie://)
	internalPages := []struct {
		Title string
		URL   string
	}{
		{"About Goosie (Version & Diagnostics)", "goosie://version"},
		{"Bookmarks Manager", "goosie://bookmarks"},
		{"Browsing History", "goosie://history"},
		{"Downloads Manager", "goosie://downloads"},
	}
	for _, p := range internalPages {
		pagePath := strings.TrimPrefix(p.URL, "goosie://")
		matchScheme := strings.HasPrefix(lowerQ, "goosie") || strings.HasPrefix(lowerQ, "about")
		matchContent := strings.Contains(strings.ToLower(p.Title), lowerQ) || strings.Contains(pagePath, lowerQ)
		if matchScheme || matchContent {
			if !seen[p.URL] {
				seen[p.URL] = true
				out = append(out, toolbar.Suggestion{
					Title: p.Title,
					URL:   p.URL,
					Kind:  toolbar.SuggestBookmark,
				})
				if len(out) >= 8 {
					return out
				}
			}
		}
	}

	// 2. Bookmarks matching query
	if f.bookmarks != nil {
		for _, bm := range f.bookmarks.List() {
			if strings.Contains(strings.ToLower(bm.URL), lowerQ) || strings.Contains(strings.ToLower(bm.Title), lowerQ) {
				if !seen[bm.URL] {
					seen[bm.URL] = true
					out = append(out, toolbar.Suggestion{
						Title: bm.Title,
						URL:   bm.URL,
						Kind:  toolbar.SuggestBookmark,
					})
					if len(out) >= 8 {
						return out
					}
				}
			}
		}
	}

	// 3. History visits matching query (newest first)
	if f.history != nil {
		entries := f.history.Entries()
		for i := len(entries) - 1; i >= 0; i-- {
			h := entries[i]
			if strings.Contains(strings.ToLower(h.URL), lowerQ) || strings.Contains(strings.ToLower(h.Title), lowerQ) {
				if !seen[h.URL] {
					seen[h.URL] = true
					out = append(out, toolbar.Suggestion{
						Title: h.Title,
						URL:   h.URL,
						Kind:  toolbar.SuggestHistory,
					})
					if len(out) >= 8 {
						return out
					}
				}
			}
		}
	}

	// 3. History visits matching query (newest first)
	if f.history != nil {
		entries := f.history.Entries()
		for i := len(entries) - 1; i >= 0; i-- {
			h := entries[i]
			if strings.Contains(strings.ToLower(h.URL), lowerQ) || strings.Contains(strings.ToLower(h.Title), lowerQ) {
				if !seen[h.URL] {
					seen[h.URL] = true
					out = append(out, toolbar.Suggestion{
						Title: h.Title,
						URL:   h.URL,
						Kind:  toolbar.SuggestHistory,
					})
					if len(out) >= 6 {
						return out
					}
				}
			}
		}
	}

	return out
}

// navContext roots one navigation load: the run scope when it exists so
// shutdown cancels in-flight loads, Background for bare test harnesses.
func (f *framePath) navContext() context.Context {
	if f.runCtx != nil {
		return f.runCtx
	}
	return context.Background()
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
	ctx, cancel := context.WithCancel(f.navContext())
	if !tab.ShieldsEnabled() {
		ctx = net.WithShieldsDisabled(ctx, true)
	}
	tab.Nav.Cancel = cancel

	tab.Nav.Serial++
	serial := tab.Nav.Serial
	tab.Nav.Loading = true
	tab.Nav.Mu.Unlock()

	tab.SetLoading(true)
	tab.SetError("")
	if f.toolbar != nil {
		f.toolbar.SetLoading(true)
		f.toolbar.SetError("")
	}

	f.navWg.Add(1)
	client := f.clientForTab(tab)
	go func() {
		defer f.navWg.Done()
		layer, _, bgColor, sess, finalURL, err := f.loadDocumentCtx(ctx, client, u, f.config.width, f.config.height, float32(f.config.dpr), f.config.downloadDir, true, true)
		res := navResult{tabID: tab.ID(), serial: serial, url: u}
		if finalURL != "" {
			// A redirect lands the final URL: the address bar and the
			// history record what displayed, not what was typed.
			res.url = finalURL
		}
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

// loadDocumentCtx loads an internal page (goosie://) or a network/file resource.
func (f *framePath) loadDocumentCtx(ctx context.Context, client net.HTTP, u string, viewportW, viewportH int, scale float32, downloadDir string, deferImages, deferFonts bool) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, string, error) {
	if strings.HasPrefix(strings.ToLower(u), "goosie://") {
		return f.renderInternalPage(ctx, u, viewportW, viewportH, scale)
	}
	if client == nil {
		client = f.client
	}
	storage := f.storage
	if client == f.privateClient {
		storage = f.incognitoStorage
	}
	tabID := ""
	if f.tabMgr != nil {
		if active := f.tabMgr.Active(); active != nil {
			tabID = strconv.FormatUint(active.ID(), 10)
		}
	}
	appMode := appearanceMode{dark: f.isDarkMode(), highContrast: f.isHighContrast()}
	return loadURLCtx(ctx, client, f.fonts, u, viewportW, viewportH, scale, downloadDir, deferImages, deferFonts, f.downloads, storage, tabID, appMode)
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

	tab.SetLoading(false)
	if result.downloadPath != "" {
		if f.toolbar != nil && f.tabMgr.Active() == tab {
			f.toolbar.SetLoading(false)
		}
		fmt.Fprintf(os.Stderr, "goosie: saved %s\n", result.downloadPath)
		if f.toolbar != nil && f.downloads != nil {
			f.toolbar.SetDownloadStats(f.downloads.ActiveCount(), f.downloads.TotalCount(), f.downloads.OverallProgress())
		}
		return
	}
	if result.err != nil {
		tab.SetError(result.err.Error())
		if f.toolbar != nil && f.tabMgr.Active() == tab {
			f.toolbar.SetLoading(false)
			f.toolbar.SetError(result.err.Error())
		}
		fmt.Fprintln(os.Stderr, result.err)

		// Build in-viewport error page so the tab is not left blank or showing obsolete content.
		if !errors.Is(result.err, context.Canceled) {
			errLayer, errSess, errBG := f.buildErrorPage(result.url, result.err)
			if errLayer != nil {
				oldSession := tab.Session()
				tab.SetLayer(errLayer)
				tab.SetSession(errSess)
				tab.SetBGColor(errBG)
				tab.SetURL(result.url)
				tab.SetTitle("Problem loading page")
				if oldSession != nil && oldSession != errSess {
					oldSession.Close()
				}
				if f.tabMgr.Active() == tab {
					f.syncTabToToolbar()
					f.sched.SetPlan(frame.FramePlan{
						Serial:     result.serial,
						Layers:     []*frame.Layer{errLayer},
						Background: tab.BGColor(),
					})
					f.sched.SetViewport(frame.Viewport{
						Offset: frame.Point{},
						Size:   f.config.devSize(),
					})
					f.publishAccessibility()
				}
			}
		}
		return
	}

	oldSession := tab.Session()
	tab.SetLayer(result.layer)
	tab.SetSession(result.session)
	tab.SetBGColor(result.bgColor)
	tab.SetURL(result.url)
	tabTitle := result.url
	if strings.HasPrefix(strings.ToLower(result.url), "goosie://") {
		title, _ := f.buildInternalPageHTML(result.url)
		if title != "" {
			tabTitle = title
		}
	}
	tab.SetTitle(tabTitle)
	if oldSession != nil && oldSession != result.session {
		// The replaced document's realm tears down with it.
		oldSession.Close()
	}
	if !result.noHistory {
		tab.History().Push(result.url)
	}
	if result.traverse != 0 {
		// The peeked back/forward step commits only now that its page
		// displayed; a stale commit (another navigation landed first) drops.
		// A redirect rewrites the entry it landed on with the final URL.
		if tab.History().Step(result.traverse, result.traverseBase) {
			tab.History().ReplaceCurrent(result.url)
		}
	}
	if !tab.Incognito() && f.history != nil && !strings.HasPrefix(strings.ToLower(result.url), "goosie://") {
		f.history.Record(result.url, result.url)
	}

	if f.tabMgr.Active() == tab {
		f.syncTabToToolbar()
		f.sched.SetPlan(frame.FramePlan{
			Serial:     result.serial,
			Layers:     []*frame.Layer{result.layer},
			Background: tab.BGColor(),
		})
		if !result.noHistory {
			// A fresh navigation opens at the top: without the reset the
			// new page inherited whatever offset the old one scrolled to.
			// Reloads and traversals keep their offset (per-entry restore
			// is a later feature, not silent loss).
			tab.SetScrollY(0)
			f.sched.SetViewport(frame.Viewport{
				Offset: frame.Point{},
				Size:   f.config.devSize(),
			})

			// If the URL has a fragment (#anchor), scroll to the element
			if href := result.url; strings.Contains(href, "#") {
				f.scrollToAnchor(tab, href)
			}
		}
		f.publishAccessibility()
	}

	// The first frame is out with the document and CSS only; fonts and images
	// arrive on their own goroutines and repaint when they land.
	if sess := result.session; sess != nil {
		tabID, serial := result.tabID, result.serial
		if sess.DeferredFontsPending() > 0 {
			go sess.LoadDeferredFonts(func(loaded int) {
				if loaded == 0 {
					return
				}
				sess.RefreshAfterFonts()
				f.imgResults <- imgResult{tabID: tabID, serial: serial, session: sess}
			})
		}
		if sess.DeferredImagesPending() > 0 {
			go sess.LoadDeferredImages(func(loaded int) {
				if loaded == 0 {
					return
				}
				f.imgResults <- imgResult{tabID: tabID, serial: serial, session: sess}
			})
		}
	}
	f.saveActiveSession()
}

// drainImageResults reads deferred-image completions and applies them.
func (f *framePath) drainImageResults() {
	for res := range f.imgResults {
		f.applyImageResult(res)
	}
}

// applyImageResult reflows and repaints the tab whose deferred images just
// landed, unless a newer navigation (or a reload that rebuilt the session)
// has superseded it.
func (f *framePath) applyImageResult(res imgResult) {
	tab := f.tabMgr.TabByID(res.tabID)
	if tab == nil || tab.Session() != res.session {
		return
	}
	tab.Nav.Mu.Lock()
	cur := tab.Nav.Serial
	tab.Nav.Mu.Unlock()
	if cur != res.serial {
		return
	}
	if err := tab.Session().Reflow(float32(f.config.width)); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow after images:", err)
		return
	}
	f.repaintTab(tab, frame.Rect{})
}

// toggleBookmark stars or unstars the active tab's page and persists immediately.
func (f *framePath) toggleBookmark() {
	tab := f.tabMgr.Active()
	if tab == nil || tab.URL() == "" || f.bookmarks == nil {
		return
	}
	f.bookmarks.Toggle(tab.URL(), tab.Title())
	if err := f.bookmarks.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "goosie: save bookmarks: %v\n", err)
	}
}

// traverseTab handles back/forward on the active tab. The cursor moves only
// when the traversal's load succeeds (committed in applyNavResult): peeking
// here keeps a failed load from stranding the cursor on a page that never
// displayed.
func (f *framePath) traverseTab(delta int) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	var url string
	var base int
	var ok bool
	if delta < 0 {
		url, base, ok = tab.History().PeekBack()
	} else if delta > 0 {
		url, base, ok = tab.History().PeekForward()
	}
	if !ok || url == "" {
		return
	}
	f.navigateTabNoHistoryTraverse(url, delta, base)
}

// navigateTabNoHistory loads a URL without pushing to history.
func (f *framePath) navigateTabNoHistory(rawURL string) {
	f.navigateTabNoHistoryTraverse(rawURL, 0, 0)
}

// navigateTabNoHistoryTraverse is navigateTabNoHistory carrying a traversal
// commit: on success the history cursor steps by delta from base.
func (f *framePath) navigateTabNoHistoryTraverse(rawURL string, delta, base int) {
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
	ctx, cancel := context.WithCancel(f.navContext())
	tab.Nav.Cancel = cancel
	tab.Nav.Serial++
	serial := tab.Nav.Serial
	tab.Nav.Loading = true
	tab.Nav.Mu.Unlock()

	tab.SetLoading(true)
	tab.SetError("")
	if f.toolbar != nil {
		f.toolbar.SetLoading(true)
	}

	f.navWg.Add(1)
	client := f.clientForTab(tab)
	go func() {
		defer f.navWg.Done()
		layer, _, bgColor, sess, finalURL, err := f.loadDocumentCtx(ctx, client, u, f.config.width, f.config.height, float32(f.config.dpr), f.config.downloadDir, true, true)
		res := navResult{tabID: tab.ID(), serial: serial, url: u, noHistory: true, traverse: delta, traverseBase: base}
		if finalURL != "" {
			res.url = finalURL
		}
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
	if tab == nil || tab.URL() == "" {
		return
	}
	f.navigateTabNoHistory(tab.URL())
}

// scrollToAnchor scrolls the active tab's viewport to an element with matching ID.
// It extracts the fragment from the URL and finds the corresponding layout object.
func (f *framePath) scrollToAnchor(tab *tabs.Tab, href string) {
	sess := tab.Session()
	if sess == nil {
		return
	}

	// Extract fragment from URL
	u, err := url.Parse(href)
	if err != nil {
		return
	}
	fragment := u.Fragment
	if fragment == "" {
		return
	}

	// Find target element by scrolling to its position
	targetY := sess.ScrollToID(fragment)
	if targetY > 0 {
		tab.SetScrollY(int32(targetY))
		f.sched.SetViewport(frame.Viewport{
			Offset: frame.Point{X: 0, Y: int32(targetY)},
			Size:   f.config.devSize(),
		})
	}
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
			f.tabMgr.SwitchTo(restored[st.Active].ID())
		}
	}
	if active := f.tabMgr.Active(); active != nil && active.URL() != "" {
		f.restoredActiveURL = active.URL()
	}
}

func applyTabState(t *tabs.Tab, ts session.TabState) {
	t.SetURL(ts.URL)
	t.SetTitle(ts.Title)
	t.SetHistory(toolbar.RestoreHistory(ts.History, ts.HistoryIndex))
	if t.Title() == "" {
		t.SetTitle("New Tab")
	}
}

// sessionState snapshots the open tabs for saving.
func sessionState(f *framePath) session.State {
	var st session.State
	activeTab := f.tabMgr.Active()
	savedActiveIdx := 0
	for _, tab := range f.tabMgr.Tabs() {
		if tab.Incognito() {
			continue // Incognito tabs are ephemeral and never persisted
		}
		if tab == activeTab {
			savedActiveIdx = len(st.Tabs)
		}
		entries, idx := tab.History().Entries()
		st.Tabs = append(st.Tabs, session.TabState{
			URL:          tab.URL(),
			Title:        tab.Title(),
			History:      entries,
			HistoryIndex: idx,
		})
	}
	st.Active = savedActiveIdx
	return st
}

// saveActiveSession persists the current open tab state to sessionActiveFile.
// This acts as a crash recovery journal: on clean exit the file is deleted. If the
// process crashes or is killed, the file survives so the session can be
// recovered on next launch.
func (f *framePath) saveActiveSession() {
	if f == nil || f.config.private || f.config.paced() || f.config.sessionActiveFile == "" {
		return
	}
	if f.crashedSession != nil {
		// Do not overwrite the crashed journal while the restore prompt is active.
		return
	}
	if f.tabMgr == nil {
		return
	}
	st := sessionState(f)
	if len(st.Tabs) == 0 {
		return
	}
	// Do not persist a state consisting solely of the restore prompt itself
	if len(st.Tabs) == 1 && strings.HasPrefix(strings.ToLower(st.Tabs[0].URL), "goosie://restore") {
		return
	}
	_ = session.Save(f.config.sessionActiveFile, st)
}

// restoreCrashedSession restores the tabs from a previous crashed session.
func (f *framePath) restoreCrashedSession(st session.State) {
	if len(st.Tabs) == 0 || f.tabMgr == nil {
		return
	}
	tabsList := f.tabMgr.Tabs()
	// If the active/only tab is the restore page, overwrite it with the first crashed tab.
	if len(tabsList) == 1 && strings.HasPrefix(strings.ToLower(tabsList[0].URL()), "goosie://restore") {
		f.applySessionState(st)
	} else {
		for _, ts := range st.Tabs {
			t := f.tabMgr.NewTab()
			applyTabState(t, ts)
		}
		if st.Active >= 0 {
			all := f.tabMgr.Tabs()
			idx := len(all) - len(st.Tabs) + st.Active
			if idx >= 0 && idx < len(all) {
				f.tabMgr.SwitchTo(all[idx].ID())
			}
		}
	}
	if active := f.tabMgr.Active(); active != nil && active.URL() != "" && !strings.HasPrefix(strings.ToLower(active.URL()), "goosie://restore") {
		f.navigateTab(active.URL())
	}
	f.saveActiveSession()
}

// handleResize reflows and repaints the active tab after the window is resized.
func (f *framePath) handleResize(contentW, contentH int) {
	f.config.width = contentW
	f.config.height = contentH

	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}

	reflowW := float32(contentW)
	if f.emulationPreset != nil {
		reflowW = float32(f.emulationPreset.Width)
	}

	if err := tab.Session().Reflow(reflowW); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		return
	}

	f.repaintTab(tab, frame.Rect{})
}

// pumpJS runs one event-loop turn for the active tab and repaints on change.
// Background tabs stay frozen (and cheap) until shown. It runs on the chrome
// pump thread like every input callback; see the wiring at openWindow.
func (f *framePath) pumpJS() {
	if f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	sess := tab.Session()
	if sess == nil {
		return
	}
	if !sess.PumpJS(float32(f.config.width)) {
		return
	}
	f.repaintTab(tab, frame.Rect{})
}

// selectionDamage unions the previous selection bounds with the current
// ones, scaled to device pixels: a highlight change damages exactly where
// ink appeared or disappeared. Either side may be absent (a collapsed caret
// paints nothing, a cleared highlight leaves nothing); both absent yields an
// empty rect, which repaintTab reads as full damage.
func (f *framePath) selectionDamage(tab *tabs.Tab, oldR frame.Rect, oldOK bool) frame.Rect {
	scale := float32(f.config.dpr) * float32(f.zoom)
	var x0, y0, x1, y1 int32
	first := true
	add := func(r frame.Rect) {
		dx0 := int32(math.Floor(float64(r.X0) * float64(scale)))
		dy0 := int32(math.Floor(float64(r.Y0) * float64(scale)))
		dx1 := int32(math.Ceil(float64(r.X1) * float64(scale)))
		dy1 := int32(math.Ceil(float64(r.Y1) * float64(scale)))
		if first || dx0 < x0 {
			x0 = dx0
		}
		if first || dy0 < y0 {
			y0 = dy0
		}
		if first || dx1 > x1 {
			x1 = dx1
		}
		if first || dy1 > y1 {
			y1 = dy1
		}
		first = false
	}
	if oldOK {
		add(oldR)
	}
	if newR, ok := tab.Session().SelectionBounds(); ok {
		add(newR)
	}
	if first {
		return frame.Rect{}
	}
	return frame.Rect{X0: x0, Y0: y0, X1: x1, Y1: y1}
}

// dirtyOrFull resolves a repaint damage rect: an empty rect (callers that
// cannot know better: navigation-adjacent, focus, IME) falls back to the
// whole extent, which is always correct and only less optimal.
func dirtyOrFull(dirty, extent frame.Rect) frame.Rect {
	if dirty.Empty() {
		return extent
	}
	return dirty
}

// repaintTab rebuilds the active tab's layer from its session and hands the
// plan to the scheduler. dirty is the device-pixel damage in layer space;
// an empty rect means the whole extent.
//
// When the tab already holds a layer over the same extent and budget, the
// layer (and its tile cache and pool) is reused: the new content is
// attached and only dirty tiles go stale via InvalidateRect, and no plan is
// republished — the loop re-reads the shared layer pointer every frame and
// re-rasterizes exactly what went stale. A new layer (and full plan) happens
// only when geometry demands it.
//
// Versions deliberately do not advance on reuse: bumping the content version
// would skew any frame already rasterizing the previous list (its completion
// arrives stamped with the old display-list version and would be dropped as
// stale forever, requeued every frame). Invalidating dirty tiles at a fixed
// version converges by construction instead.
func (f *framePath) repaintTab(tab *tabs.Tab, dirty frame.Rect) {
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	if f.emulationPreset != nil {
		effectiveDPR = f.emulationPreset.DPR * float32(f.zoom)
	}
	list, err := tab.Session().PaintChecked(effectiveDPR)

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
	layerBounds := extent.Canon()
	if layerBounds.X0 < 0 {
		layerBounds.X0 = 0
	}
	if layerBounds.Y0 < 0 {
		layerBounds.Y0 = 0
	}

	if lyr := tab.Layer(); lyr != nil && lyr.Bounds == layerBounds && lyr.Grid.Stats().Budget == budgetBytes {
		// Reuse: keep the tile cache and pool. Attach first, then stale:
		// a frame racing between the two paints old pixels that the
		// invalidation immediately marks for redo, so it converges on the
		// next frame instead of sticking. Versions stay fixed, so every
		// completion stamps the layer's own version and nothing is dropped
		// as stale.
		lyr.SetContent(dl)
		lyr.InvalidateRect(dirtyOrFull(dirty, layerBounds))
		f.publishAccessibility()
		return
	}

	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)

	tab.SetLayer(layer)
	// A background tab's repaint must not steal the screen; switchTab
	// presents the fresh layer when the tab comes forward.
	if f.tabMgr.Active() == tab {
		f.sched.SetPlan(frame.FramePlan{
			Serial:     tab.Nav.Serial,
			Layers:     []*frame.Layer{layer},
			Background: tab.BGColor(),
		})
	}
	f.publishAccessibility()
}

// publishAccessibility hands the active document's accessibility tree to a
// window that can publish it. The tree's coordinates are document CSS pixels;
// the platform wants window content coordinates, so every rect is shifted by
// the scroll offset before it leaves. A backend with no accessibility story,
// or a tab with no session, publishes nothing - clearing what a previous
// document left behind.
func (f *framePath) publishAccessibility() {
	aw, ok := f.window.(surface.AccessibilityWindow)
	if !ok {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		aw.SetAccessibility(nil)
		return
	}
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	vp := f.sched.Viewport()
	aw.SetAccessibility(shiftAx(tab.Session().AccessibilityTree(),
		float32(vp.Offset.X)/effectiveDPR, float32(vp.Offset.Y)/effectiveDPR))
}

// shiftAx copies the tree with every rect translated by (dx, dy).
func shiftAx(nodes []ax.Node, dx, dy float32) []ax.Node {
	out := make([]ax.Node, len(nodes))
	for i, n := range nodes {
		n.X0 -= dx
		n.Y0 -= dy
		n.X1 -= dx
		n.Y1 -= dy
		n.Children = shiftAx(n.Children, dx, dy)
		out[i] = n
	}
	return out
}

// docPoint converts a content-area point to document coordinates through the
// active zoom and scroll offset.
func (f *framePath) docPoint(contentX, contentY int32) (float32, float32) {
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	vp := f.sched.Viewport()
	return float32(contentX)/effectiveDPR + float32(vp.Offset.X)/effectiveDPR,
		float32(contentY)/effectiveDPR + float32(vp.Offset.Y)/effectiveDPR
}

// focusClick focuses or blurs a form control under a content-area click, handles
// activatable controls (checkbox, radio, submit/reset button), and repaints.
func (f *framePath) focusClick(contentX, contentY int32) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return false
	}
	dx, dy := f.docPoint(contentX, contentY)

	// Check if click hit an activatable control (checkbox, radio, submit button, reset button)
	if tab.Session().ClickAt(dx, dy) {
		if sub, ok := tab.Session().PopSubmittedForm(); ok {
			f.handleFormSubmission(tab, sub)
			return true
		}
		if err := tab.Session().Reflow(float32(f.config.width)); err != nil {
			fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		}
		f.repaintTab(tab, frame.Rect{})
		return true
	}

	if !tab.Session().FocusControl(dx, dy) {
		return false
	}
	f.setWindowIME(tab.Session().Focused() != nil)
	f.repaintTab(tab, frame.Rect{})
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
	if tab == nil || tab.Session() == nil {
		return false
	}
	switch ev.IME {
	case surface.IMEMarked:
		if tab.Session().SetMarked(ev.Text) {
			f.repaintTab(tab, frame.Rect{})
		}
		return true
	case surface.IMECommit:
		if tab.Session().CommitText(ev.Text) {
			if err := tab.Session().Reflow(float32(f.config.width)); err != nil {
				fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
			}
		}
		f.repaintTab(tab, frame.Rect{})
		return true
	}
	return false
}

// dragSelect drives text selection from content-area pointer events. A press
// starts a selection unless a form control is focused, motion extends the
// active one, and release just ends the gesture.
func (f *framePath) dragSelect(action surface.PointerAction, contentX, contentY int32) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return false
	}
	switch action {
	case surface.PointerPress:
		if tab.Session().Focused() != nil {
			return false
		}
		dx, dy := f.docPoint(contentX, contentY)
		oldR, oldOK := tab.Session().SelectionBounds()
		changed := tab.Session().SelectAt(dx, dy)
		f.selDrag = true
		if changed {
			f.repaintTab(tab, f.selectionDamage(tab, oldR, oldOK))
		}
		return true
	case surface.PointerMotion:
		if !f.selDrag {
			return false
		}
		dx, dy := f.docPoint(contentX, contentY)
		oldR, oldOK := tab.Session().SelectionBounds()
		if !tab.Session().SelectTo(dx, dy) {
			return true
		}
		f.repaintTab(tab, f.selectionDamage(tab, oldR, oldOK))
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
	if tab == nil || tab.Session() == nil || !tab.Session().HasSelection() {
		return false
	}
	if cb := platform.NewClipboard(); cb != nil {
		cb.Write(tab.Session().SelectionText())
	}
	return true
}

// contentKey applies a keyboard edit to the focused control. The event is
// consumed whenever a control is focused so the address bar is not the only
// text sink; the tab reflows when the value changed and repaints otherwise.
func (f *framePath) contentKey(key rune, mods surface.KeyMod) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return false
	}

	cmd := mods&surface.ModCommand != 0 || mods&surface.ModControl != 0

	// Bare Tab drives the full key-event path (keydown → focus-move → keyup).
	// It must work with nothing focused so it can start the tab cycle, so this
	// is checked before the focused-control bail below. Cmd/Ctrl+Tab is a
	// browser-tab shortcut and is consumed earlier in the chrome window.
	if key == '\t' && !cmd {
		shift := mods&surface.ModShift != 0
		alt := mods&surface.ModOption != 0
		tab.Session().PressKey("Tab", shift, false, alt, false)
		if err := tab.Session().Reflow(float32(f.config.width)); err != nil {
			fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
			return true
		}
		f.repaintTab(tab, frame.Rect{})
		return true
	}

	if tab.Session().Focused() == nil {
		if cmd && (key == 'a' || key == 'A') {
			if tab.Session().SelectAll() {
				f.repaintTab(tab, frame.Rect{})
				return true
			}
		}
		return false
	}

	if cmd {
		switch key {
		case 'v', 'V':
			if tab.Session().FocusedEditable() {
				if cb := platform.NewClipboard(); cb != nil {
					clipText := cb.Read()
					if clipText != "" {
						if tab.Session().InsertText(clipText) {
							if err := tab.Session().Reflow(float32(f.config.width)); err != nil {
								fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
							}
							f.repaintTab(tab, frame.Rect{})
							return true
						}
					}
				}
			}
		case 'a', 'A':
			if tab.Session().FocusedEditable() {
				if tab.Session().SelectAllControl() {
					f.repaintTab(tab, frame.Rect{})
					return true
				}
			} else {
				if tab.Session().SelectAll() {
					f.repaintTab(tab, frame.Rect{})
					return true
				}
			}
		case 'c', 'C':
			if tab.Session().FocusedEditable() {
				txt := tab.Session().FocusedText()
				if txt != "" {
					if cb := platform.NewClipboard(); cb != nil {
						cb.Write(txt)
						return true
					}
				}
			}
		}
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

	changed := tab.Session().Edit(action, key)
	if sub, ok := tab.Session().PopSubmittedForm(); ok {
		f.handleFormSubmission(tab, sub)
		return true
	}
	if changed {
		if action == engine.EditEscape {
			f.setWindowIME(false)
		}
		if err := tab.Session().Reflow(float32(f.config.width)); err != nil {
			fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
			return true
		}
	}
	f.repaintTab(tab, frame.Rect{})
	return true
}

// handleFormSubmission dispatches a form submission by resolving action and encoding parameters.
func (f *framePath) handleFormSubmission(tab *tabs.Tab, sub *engine.FormSubmission) {
	if tab == nil || sub == nil {
		return
	}
	targetURL := resolveHref(tab.URL(), sub.Action)
	if targetURL == "" {
		targetURL = tab.URL()
	}
	method := strings.ToUpper(sub.Method)
	if method == "" || method == "GET" {
		if sub.Encoded != "" {
			if strings.Contains(targetURL, "?") {
				targetURL = targetURL + "&" + sub.Encoded
			} else {
				targetURL = targetURL + "?" + sub.Encoded
			}
		}
		f.navigateTab(targetURL)
	} else if method == "POST" {
		f.navigateTab(targetURL)
	}
}

// handleContextMenu constructs the context menu for the clicked element.
func (f *framePath) handleContextMenu(contentX, contentY, winX, winY int32) {
	cw, ok := f.window.(*chromeWindow)
	if !ok || cw.contextMenu == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	docX, docY := f.docPoint(contentX, contentY)
	linkURL := tab.Session().HitTestLink(docX, docY)
	if linkURL != "" {
		linkURL = resolveHref(tab.URL(), linkURL)
	}
	imageURL := tab.Session().HitTestImage(docX, docY)
	if imageURL != "" {
		imageURL = resolveHref(tab.URL(), imageURL)
	}
	selText := tab.Session().SelectionText()
	isEditable := tab.Session().FocusedEditable()
	focusedText := tab.Session().FocusedText()
	_, _, canBack := tab.History().PeekBack()
	_, _, canForward := tab.History().PeekForward()

	items := BuildContextMenuItems(linkURL, imageURL, selText, focusedText, isEditable, canBack, canForward)
	winW := int32(f.config.width) * cw.sc()
	winH := int32(f.config.height) * cw.sc()
	if cw.chromeBitmap != nil {
		winW = int32(cw.chromeBitmap.W)
	}
	if cw.outBitmap != nil {
		winH = int32(cw.outBitmap.H)
	}
	cw.contextMenu.Show(winX, winY, winW, winH, items, cw.sc())
}

// handleContextMenuAction executes the action chosen from the context menu.
func (f *framePath) handleContextMenuAction(action, data string) {
	switch action {
	case "open_new_tab":
		f.newTab()
		if data != "" {
			f.navigateTab(data)
		}
	case "copy_link", "copy_image":
		if cb := platform.NewClipboard(); cb != nil {
			cb.Write(data)
		}
	case "open_image_tab":
		f.newTab()
		if data != "" {
			f.navigateTab(data)
		}
	case "copy":
		if data != "" {
			if cb := platform.NewClipboard(); cb != nil {
				cb.Write(data)
			}
		} else {
			f.copySelection()
		}
	case "cut":
		if tab := f.tabMgr.Active(); tab != nil && tab.Session() != nil {
			txt := tab.Session().FocusedText()
			if cb := platform.NewClipboard(); cb != nil {
				cb.Write(txt)
			}
			tab.Session().SelectAllControl()
			tab.Session().InsertText("")
			f.repaintTab(tab, frame.Rect{})
		}
	case "paste":
		if cb := platform.NewClipboard(); cb != nil {
			if text := cb.Read(); text != "" {
				if tab := f.tabMgr.Active(); tab != nil && tab.Session() != nil {
					if tab.Session().InsertText(text) {
						if err := tab.Session().Reflow(float32(f.config.width)); err != nil {
							fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
						}
					}
					f.repaintTab(tab, frame.Rect{})
				}
			}
		}
	case "select_all":
		if tab := f.tabMgr.Active(); tab != nil && tab.Session() != nil {
			if tab.Session().FocusedEditable() {
				tab.Session().SelectAllControl()
			} else {
				tab.Session().SelectAll()
			}
			f.repaintTab(tab, frame.Rect{})
		}
	case "search_google":
		if data != "" {
			searchURL := "https://www.google.com/search?q=" + url.QueryEscape(data)
			f.navigateTab(searchURL)
		}
	case "back":
		f.traverseTab(-1)
	case "forward":
		f.traverseTab(1)
	case "reload":
		f.reloadTab()
	case "inspect":
		f.navigateTab("goosie://inspect")
	case "print_pdf":
		f.printActiveTabToPDF()
	}
}

// printActiveTabToPDF exports the currently active tab document to PDF.
func (f *framePath) printActiveTabToPDF() {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	outPath := filepath.Join(f.config.downloadDir, "print.pdf")
	if err := ExportSessionToPDF(tab.Session(), f.fonts, float32(f.config.dpr), outPath); err != nil {
		fmt.Fprintf(os.Stderr, "goosie: print to PDF error: %v\n", err)
		return
	}
	fmt.Printf("goosie: document printed to PDF at %s\n", outPath)
}

func (f *framePath) handleZoom(delta float64) {
	if f.zoom <= 0 {
		f.zoom = 1.0
	}
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
	if f.toolbar != nil {
		f.toolbar.SetZoom(f.zoom)
	}
	f.handleResize(f.config.width, f.config.height)
}

func (f *framePath) handleLinkClick(href string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	f.navigateTab(resolveHref(tab.URL(), href))
}

func (f *framePath) hitTestLink(contentX, contentY int32) string {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return ""
	}
	docX, docY := f.docPoint(contentX, contentY)
	return tab.Session().HitTestLink(docX, docY)
}

// hoverCursor resolves the pointer shape for buttonless motion over the
// document: the hand over a link, the I-beam over editable or selectable
// text, and the default arrow everywhere else. Link wins over text because
// a link's words are text too but only the link is clickable.
func (f *framePath) hoverCursor(contentX, contentY int32) surface.Cursor {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return surface.CursorDefault
	}
	dx, dy := f.docPoint(contentX, contentY)
	if tab.Session().HitTestLink(dx, dy) != "" {
		return surface.CursorPointer
	}
	if tab.Session().HasControlAt(dx, dy) || tab.Session().HasTextAt(dx, dy) {
		return surface.CursorText
	}
	return surface.CursorDefault
}

// runFind recomputes the match set for a new find query on the active tab and
// jumps to the first hit.
func (f *framePath) runFind(query string) {
	if f.toolbar == nil {
		return
	}
	f.findMatches = nil
	f.findIdx = 0
	f.toolbar.ResetFindCounts()
	if strings.TrimSpace(query) == "" {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	f.findMatches = tab.Session().Find(query)
	f.toolbar.SetFindResult(len(f.findMatches), 0)
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
	f.toolbar.SetFindResult(len(f.findMatches), f.findIdx)
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
	f.toolbar.ResetFindCounts()
	if active, _, _ := f.toolbar.FindCounts(); active {
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
		cur.SetScrollY(vp.Offset.Y)
	}
	f.tabMgr.SwitchTo(id)
	newTab := f.tabMgr.Active()
	if newTab == nil {
		return
	}
	f.ensureActiveTabVisible()
	f.syncTabToToolbar()
	if newTab.Layer() != nil {
		f.sched.SetPlan(frame.FramePlan{
			Serial:     newTab.Nav.Serial,
			Layers:     []*frame.Layer{newTab.Layer()},
			Background: newTab.BGColor(),
		})
	}
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: newTab.ScrollY()},
		Size:   f.config.devSize(),
	})
	f.publishAccessibility()
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
		cur.SetScrollY(vp.Offset.Y)
	}
	tab := f.tabMgr.NewTab()
	f.tabMgr.SwitchTo(tab.ID())
	f.ensureActiveTabVisible()
	f.syncTabToToolbar()
	_, layer := blankLayer(f.config.devSize())
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(255, 255, 255))
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor(),
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: 0},
		Size:   f.config.devSize(),
	})
	f.publishAccessibility()
}

// ensureActiveTabVisible scrolls a clipped tab strip just enough to show the
// active tab: keyboard switches, new tabs and session restores land on tabs
// the pointer could never reach otherwise.
func (f *framePath) ensureActiveTabVisible() {
	if f.tabMgr == nil {
		return
	}
	n := int32(f.tabMgr.Count())
	w := int32(f.config.width)
	idx := f.tabMgr.ActiveIndex()
	f.tabMgr.SetTabScroll(tabs.EnsureTabVisible(idx, n, w, f.tabMgr.TabScroll()), n, w)
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
		if sess := tab.Session(); sess != nil {
			sess.Close()
		}
	}
	f.tabMgr.CloseTab(id)
	if f.tabMgr.Count() > 0 {
		tab := f.tabMgr.Active()
		f.ensureActiveTabVisible()
		f.syncTabToToolbar()
		if tab.Layer() != nil {
			f.sched.SetPlan(frame.FramePlan{
				Serial:     tab.Nav.Serial,
				Layers:     []*frame.Layer{tab.Layer()},
				Background: tab.BGColor(),
			})
		}
		f.sched.SetViewport(frame.Viewport{
			Offset: frame.Point{Y: tab.ScrollY()},
			Size:   f.config.devSize(),
		})
		f.publishAccessibility()
	}
}

// reopenTab restores the most recently closed tab and loads its URL.
func (f *framePath) reopenTab() {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	cur := f.tabMgr.Active()
	if cur != nil {
		vp := f.sched.Viewport()
		cur.SetScrollY(vp.Offset.Y)
	}
	tab, ok := f.tabMgr.ReopenTab()
	if !ok || tab == nil {
		return
	}
	f.ensureActiveTabVisible()
	f.syncTabToToolbar()
	_, layer := blankLayer(f.config.devSize())
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(255, 255, 255))
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor(),
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: tab.ScrollY()},
		Size:   f.config.devSize(),
	})
	f.publishAccessibility()
	if u := tab.URL(); u != "" {
		f.navigateTabNoHistory(u)
	}
}

// duplicateTab clones the active tab and navigates it to the same page.
func (f *framePath) duplicateTab() {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	active := f.tabMgr.Active()
	if active == nil {
		return
	}
	vp := f.sched.Viewport()
	active.SetScrollY(vp.Offset.Y)

	tab, ok := f.tabMgr.DuplicateTab(active.ID())
	if !ok || tab == nil {
		return
	}
	f.ensureActiveTabVisible()
	f.syncTabToToolbar()
	_, layer := blankLayer(f.config.devSize())
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(255, 255, 255))
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor(),
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: tab.ScrollY()},
		Size:   f.config.devSize(),
	})
	f.publishAccessibility()
	if u := tab.URL(); u != "" {
		f.navigateTabNoHistory(u)
	}
}

// toggleReaderMode switches the active tab between Reader Mode and original view.
func (f *framePath) toggleReaderMode() {
	if f.tabMgr == nil {
		return
	}
	active := f.tabMgr.Active()
	if active == nil {
		return
	}
	currURL := active.URL()
	if strings.HasPrefix(strings.ToLower(currURL), "goosie://reader") {
		if u, err := url.Parse(currURL); err == nil {
			if orig := u.Query().Get("url"); orig != "" {
				f.navigateTab(orig)
				return
			}
		}
		return
	}
	if currURL != "" && !strings.HasPrefix(strings.ToLower(currURL), "goosie://") {
		f.navigateTab("goosie://reader?url=" + url.QueryEscape(currURL))
	}
}

type appearanceMode struct {
	dark         bool
	highContrast bool
}

// loadURLCtx is like loadURL but respects context cancellation. With
// deferImages the session collects image references without fetching them, so
// the first frame can go out immediately and LoadDeferredImages repaints once
// they land.
func loadURLCtx(ctx context.Context, client net.HTTP, fonts *raster.Fonts, rawURL string, viewportW, viewportH int, scale float32, downloadDir string, deferImages, deferFonts bool, extra ...any) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, string, error) {
	if err := engine.ValidateViewport(viewportW, viewportH, float64(scale)); err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: viewport: %w", err)
	}
	// Check context before starting the fetch.
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", ctx.Err()
	}
	resp, err := client.Get(ctx, rawURL)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: fetch %s: %w", net.RedactURL(rawURL), err)
	}
	var mgr *download.Manager
	var storageReg *js.OriginStorageRegistry
	var sessionID string
	var appMode appearanceMode
	for _, ex := range extra {
		switch v := ex.(type) {
		case *download.Manager:
			mgr = v
		case *js.OriginStorageRegistry:
			storageReg = v
		case string:
			sessionID = v
		case appearanceMode:
			appMode = v
		}
	}
	if download.ShouldDownload(resp.Headers["Content-Type"], resp.Headers["Content-Disposition"]) {
		name := download.FileName(resp.Headers["Content-Disposition"], resp.URL)
		var itemID string
		if mgr != nil {
			it := mgr.CreateItem(resp.URL, name, int64(len(resp.Body)))
			itemID = it.ID
		}
		savedPath, saveErr := download.Save(downloadDir, name, resp.Body)
		if saveErr != nil {
			if mgr != nil && itemID != "" {
				mgr.Fail(itemID, saveErr)
			}
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", saveErr
		}
		if mgr != nil && itemID != "" {
			mgr.Complete(itemID, savedPath)
		}
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", downloadDone{path: savedPath}
	}
	// Family deadline contexts: the engine's fetch budgets stop new spawns past
	// their deadline, but only context aborts work already in flight. Each
	// synchronous family gets a deadline child of the navigation context, so a
	// hung host settles at the budget instead of the 30s client timeout; tab
	// close and shutdown still cancel everything through the parent. Deferred
	// families keep the plain navigation context: their work starts after this
	// function returns, and post-paint arrivals carry no total deadline by
	// design (superseded results are still dropped by serial).
	cssCtx, cssCancel := context.WithTimeout(ctx, engine.CSSFetchBudget)
	defer cssCancel()
	imgCtx := ctx
	if !deferImages {
		var imgCancel context.CancelFunc
		imgCtx, imgCancel = context.WithTimeout(ctx, engine.ImageFetchBudget)
		defer imgCancel()
	}
	fontCtx := ctx
	if !deferFonts {
		var fontCancel context.CancelFunc
		fontCtx, fontCancel = context.WithTimeout(ctx, engine.FontFetchBudget)
		defer fontCancel()
	}
	// External scripts are a synchronous family like stylesheets: the
	// document cannot run them before they arrive, so they share the
	// deadline-child shape. The engine resolves srcs before calling.
	scriptCtx, scriptCancel := context.WithTimeout(ctx, engine.ScriptFetchBudget)
	defer scriptCancel()
	scriptFetcher := func(base, url string) ([]byte, error) {
		resp, err := client.Get(subresourceCtx(scriptCtx, base), url)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	// fetch()/XMLHttpRequest transport for page scripts. Unlike the
	// families above these fire after the load (event handlers, beacons),
	// so they ride the navigation context rather than a load-scoped
	// deadline: post-paint arrivals carry no total deadline by the same
	// design as deferred images and fonts.
	jsFetch := jsTransport{client: client, ctx: subresourceCtx(ctx, resp.URL)}
	// Linked style sheets are fetched with the same client and the same
	// cancellation: the linker sees absolute http(s) URLs only, because the
	// engine resolves hrefs against the final document URL before calling.
	linker := func(base, href string) (string, error) {
		resp, err := client.Get(subresourceCtx(cssCtx, base), href)
		if err != nil {
			return "", err
		}
		if refused := resp.Headers["Content-Type"]; net.StyleSheetRefused(refused) {
			return "", fmt.Errorf("goosie: %s is served as %q, not as a style sheet", net.RedactURL(href), refused)
		}
		return resp.Text(), nil
	}
	// Image subresources use the same client and cancellation. The engine hands
	// absolute http(s) URLs only, so the fetcher just reads the response bytes.
	imageFetcher := func(base, url string) ([]byte, error) {
		resp, err := client.Get(subresourceCtx(imgCtx, base), url)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	// @font-face resources use the same client. The fetcher is the same shape
	// as the image fetcher, plus the response headers the engine's font CORS
	// check reads; the engine resolves src URLs before calling.
	fontFetcher := func(base, url string) (engine.FontResult, error) {
		resp, err := client.Get(subresourceCtx(fontCtx, base), url)
		if err != nil {
			return engine.FontResult{}, err
		}
		return engine.FontResult{Data: resp.Body, Headers: resp.Headers}, nil
	}
	// Check context after the fetch in case it was cancelled during the network call.
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", ctx.Err()
	}
	// A per-document JS runtime executes the page's classic scripts during
	// the build below. A runtime that fails to construct degrades to no
	// scripting rather than failing the load. The session owns it
	// afterwards and closes it with itself (see applyNavResult/closeTab), so
	// every error return below must close what it created.
	var cookieStore js.CookieStore
	if client != nil {
		cookieStore = client.Jar()
	}
	rt, rtErr := js.New(js.Options{
		URL:          resp.URL,
		HTTPClient:   jsFetch,
		Cookies:      cookieStore,
		Storage:      storageReg,
		SessionID:    sessionID,
		DarkMode:     appMode.dark,
		HighContrast: appMode.highContrast,
	})
	if rtErr != nil {
		fmt.Fprintf(os.Stderr, "goosie: js runtime: %v\n", rtErr)
		rt = nil
	}
	completed := false
	defer func() {
		if !completed && rt != nil {
			_ = rt.Close()
		}
	}()
	sess, err := engine.NewSession(resp.Text(), nil, float32(viewportW),
		engine.WithMetrics(fonts),
		engine.WithViewportH(float32(viewportH)),
		engine.WithLinkedCSS(resp.URL, linker),
		engine.WithScripts(resp.URL, scriptFetcher),
		engine.WithJS(rt),
		engine.WithDarkMode(appMode.dark),
		engine.WithHighContrast(appMode.highContrast),
		// Every load gets a fresh invalidation batch: JS-driven DOM
		// mutations record here so the vsync pump reflows exactly the
		// documents that changed.
		engine.WithInvalidation(&engine.Invalidation{}),
		imageOption(deferImages, resp.URL, imageFetcher),
		fontOption(deferFonts, resp.URL, fontFetcher, fonts))
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: build session: %w", err)
	}
	list, err := sess.PaintChecked(scale)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: paint document: %w", err)
	}
	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: size document cache: %w", err)
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)
	completed = true
	return layer, paint.SceneSpec{DocHeight: extent.H()}, sess.BackgroundColor(), sess, resp.URL, nil
}

// imageOption picks the synchronous or deferred image path for a load.
func imageOption(deferred bool, base string, fetch engine.ImageFetcher) engine.Option {
	if deferred {
		return engine.WithDeferredImages(base, fetch)
	}
	return engine.WithImages(base, fetch)
}

// fontOption picks the synchronous or deferred font path for a load.
func fontOption(deferred bool, base string, fetch engine.FontFetcher, reg engine.FontRegistry) engine.Option {
	if deferred {
		return engine.WithDeferredCustomFontLoading(base, fetch, reg)
	}
	return engine.WithCustomFontLoading(base, fetch, reg)
}

// jsTransport adapts the browser HTTP client to the js.HTTPFetcher contract
// the fetch()/XMLHttpRequest bindings call. GET maps straight through; POST
// carries the Content-Type header (defaulting to the spec's text/plain for
// string bodies) because the client interface exposes only Get and Post.
// Other methods and per-request headers beyond Content-Type are not carried
// by this interface yet, so those requests fail through the bindings' error
// paths rather than silently misbehaving.
type jsTransport struct {
	client net.HTTP
	ctx    context.Context
}

func (t jsTransport) Fetch(url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var resp *net.Response
	var err error
	switch method {
	case "GET", "":
		resp, err = t.client.Get(ctx, url)
	case "POST":
		contentType := "text/plain;charset=UTF-8"
		for k, v := range headers {
			if strings.EqualFold(k, "Content-Type") {
				contentType = v
				break
			}
		}
		resp, err = t.client.Post(ctx, url, contentType, bytes.NewReader(body))
	default:
		return nil, fmt.Errorf("goosie: fetch method %q is not supported", method)
	}
	if err != nil {
		return nil, err
	}
	out := &js.FetchResponse{
		Status:     resp.StatusCode,
		StatusText: http.StatusText(resp.StatusCode),
		Headers:    resp.Headers,
		Body:       resp.Body,
		URL:        resp.URL,
		OK:         resp.StatusCode >= 200 && resp.StatusCode < 300,
	}
	return out, nil
}

// subresourceCtx is where the browser's two security halves meet, and the only
// place they can: internal/engine owns the private-network rule but cannot
// import internal/net, and internal/net owns the dial but must stay policy-free.
// A subresource fetched for a public page therefore carries the engine's address
// predicate down to the transport, which resolves the name once, judges every
// answer, and dials the address it judged - so a host name that resolves to
// 169.254.169.254 is refused even though the URL itself looked public.
//
// A local initiator (127.0.0.1, a development server, a file:// document) gets
// ctx back untouched. That is the same distinction resolveSheetURL makes about
// names, and guarding it would refuse a page its own stylesheet.
func subresourceCtx(ctx context.Context, base string) context.Context {
	if engine.InitiatorIsLocal(base) {
		return ctx
	}
	return net.WithAddressGuard(ctx, net.AddressGuard{Blocked: engine.AddressBlocked})
}

// normalizeURL normalizes address bar input into either a dialable URL or a search query.
// It trims surrounding whitespace, normalizes supported schemes (http, https, file),
// treats queries with whitespace or search/punctuation symbols as Google search queries,
// and prefixes bare domains or IPs with https://.
func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	// Case-insensitive scheme check for http, https, and file.
	lower := strings.ToLower(raw)
	for _, scheme := range []string{"https://", "http://", "file://"} {
		if strings.HasPrefix(lower, scheme) {
			return scheme + raw[len(scheme):]
		}
	}
	if strings.HasPrefix(lower, "goosie://") {
		return "goosie://" + strings.ToLower(raw[len("goosie://"):])
	}
	if strings.HasPrefix(lower, "about:") {
		target := strings.TrimPrefix(lower, "about:")
		if target == "" {
			target = "version"
		}
		return "goosie://" + target
	}
	// If input contains whitespace (space, tab, newline), it is a search query.
	if strings.ContainsAny(raw, " \t\r\n") {
		return "https://www.google.com/search?q=" + url.QueryEscape(raw)
	}
	// If host portion (before any '/') contains symbols invalid in a domain name
	// (such as '?', '+', '&', '=', '%', '!', '^', '*', '(', ')', '<', '>', ';'),
	// treat it as a search query.
	hostPart := raw
	if slash := strings.IndexByte(raw, '/'); slash >= 0 {
		hostPart = raw[:slash]
	}
	if strings.ContainsAny(hostPart, "?+&=%!^*()<>;\"'") {
		return "https://www.google.com/search?q=" + url.QueryEscape(raw)
	}
	// Bare domain, IP, or localhost: add https:// prefix
	return "https://" + raw
}

// resolveHref turns a clicked link's href into an absolute URL against the
// document that owns it, per RFC 3986. An unparseable piece falls back to the
// raw href so address-bar normalization still gets a chance.
func resolveHref(base, href string) string {
	if base == "" {
		return href
	}
	b, err := url.Parse(base)
	if err != nil {
		return href
	}
	r, err := url.Parse(href)
	if err != nil {
		return href
	}
	return b.ResolveReference(r).String()
}

// openWindow picks the window last, because a backend that cannot open one is a
// property of the machine the report has to state rather than a construction error.
func (f *framePath) openWindow() error {
	// The surface is the whole window: the content viewport plus the chrome the
	// chromeWindow composites on top of it. config.devSize is the content size
	// alone, so the chrome band is added back here.
	size := f.config.devSize()
	if f.toolbar != nil && f.tabMgr != nil {
		size.H += int32(totalChromeHeight) * int32(f.config.dpr+0.5)
	}
	w, err := platform.Select(platform.Options{
		Backend:     f.config.backend,
		Size:        size,
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
			func() { f.reopenTab() },
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
			f.hoverCursor,
			f.fonts,
			int32(f.config.dpr+0.5),
		)
		// The JS pump rides the chrome event thread at display cadence: one
		// event-loop turn per tick for the active tab, so timers, fetch
		// completions and RAF callbacks fire in a living browser instead of
		// only in unit tests. Same thread as every input callback, so no new
		// thread touches sessions; shutdown needs no join (the pump dies with
		// the window's event stream).
		if cw, ok := f.window.(*chromeWindow); ok {
			cw.tickInterval = f.config.vsyncPeriod()
			cw.onTick = f.pumpJS
			cw.onNewIncognitoTab = func() { f.newIncognitoTab() }
			cw.onInspect = func() { f.navigateTab("goosie://inspect") }
			cw.onDuplicateTab = func() { f.duplicateTab() }
			cw.onReaderMode = func() { f.toggleReaderMode() }
			cw.bookmarks = f.bookmarks
			cw.onContextMenu = f.handleContextMenu
			cw.onContextMenuAction = f.handleContextMenuAction
			cw.onPrintPDF = f.printActiveTabToPDF
			cw.onDeviceEmulation = func() { f.cycleDeviceEmulation() }
		}

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

// clearBrowsingData removes browsing traces (cookies, history, open-tab
// session) for the v1 clear-data control. Starred bookmarks are user-curated
// and stay, matching what clear-history means everywhere else. Missing files
// are fine (idempotent); any other failure aborts with what was removed so
// far so a half-clear is visible, not silent.
func (c config) clearBrowsingData() ([]string, error) {
	var removed []string
	for _, path := range []string{c.cookieFile, c.historyFile, c.sessionFile, c.sessionActiveFile} {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, fmt.Errorf("goosie: clear data: %w", err)
		}
		removed = append(removed, path)
	}
	return removed, nil
}

func run(args []string) error {
	c, err := parse(args)
	if err != nil {
		return err
	}
	if c.clearData {
		removed, err := c.clearBrowsingData()
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "goosie: cleared %d file(s)\n", len(removed))
		return nil
	}
	if c.printPDF != "" {
		if c.url == "" {
			return fmt.Errorf("%w: -print-pdf requires a URL", errUsage)
		}
		fonts, err := raster.NewFonts()
		if err != nil {
			return fmt.Errorf("goosie: fonts: %w", err)
		}
		client := net.DefaultClient()
		defer client.Close()
		_, _, _, sess, _, err := loadURLCtx(context.Background(), client, fonts, normalizeURL(c.url), c.width, c.height, float32(c.dpr), c.downloadDir, false, false)
		if err != nil {
			return fmt.Errorf("goosie: load URL: %w", err)
		}
		defer sess.Close()
		if err := ExportSessionToPDF(sess, fonts, float32(c.dpr), c.printPDF); err != nil {
			return fmt.Errorf("goosie: export PDF: %w", err)
		}
		fmt.Printf("goosie: exported %s to %s\n", c.url, c.printPDF)
		return nil
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
	// A paced run - a screenshot or a measured burst - has nobody to watch that
	// background load: its fixed frame count would present the placeholder tiles before
	// the document lands, so it loads synchronously and the run depicts, and reports, the
	// page it was asked for.
	if c.url != "" {
		if c.paced() {
			layer, docSpec, bgColor, _, _, err := loadURLCtx(context.Background(), f.client, f.fonts, normalizeURL(c.url), c.width, c.height, float32(c.dpr), c.downloadDir, false, false)
			if err != nil {
				return err
			}
			// The driver's scroll range comes from f.layer, so a paced document run that
			// swapped the plan but not this field would scroll a one-viewport page.
			f.layer = layer
			f.sched.SetPlan(frame.FramePlan{
				Serial:     1,
				Layers:     []*frame.Layer{layer},
				Background: bgColor,
			})
			f.document, f.docHeight = c.url, docSpec.DocHeight
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
		sd := newSteadyDriver(f, steadyFloorFrames)
		f.loop = surface.NewLoop(sd, f.sched, f.composer, f.rec)
		sd.watchIdle(func() bool {
			// Idle is not "nothing needed": a frame can want no tiles while the
			// tiles it is waiting on are still on order at a worker. Both halves
			// have to be clear before the pixels in the PNG are all there.
			st := f.loop.Stats()
			return st.Last.Needed == 0 && st.Last.PaintingBytes == 0
		})
		sd.watchProgress(func() (done, outstanding int64) {
			st := f.loop.Stats()
			outstanding = int64(st.Last.Needed) + st.Last.PaintingBytes
			return st.Rasterize, outstanding
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

	// Navigation loads root here so shutdown cancels them; navWg joins them
	// before the window closes rather than leaking blocked sends into a
	// torn-down frame path.
	f.runCtx, f.runCancel = context.WithCancel(context.Background())
	defer f.runCancel()

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
		// Stop navigation loads first so their results cannot land mid-teardown,
		// then join them: a result send into the drain is harmless, but only a
		// join proves no loader still touches the scheduler or tabs.
		f.runCancel()
		f.navWg.Wait()
		_ = f.window.Close()
		if f.privateClient != nil {
			_ = f.privateClient.Close()
		}
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
		if c.sessionActiveFile != "" {
			_ = os.Remove(c.sessionActiveFile)
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
