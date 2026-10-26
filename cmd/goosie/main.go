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
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/flags"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/perms"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/session"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
	"github.com/vyquocvu/goosie/internal/userscript"
)

const (
	// gateScrollStep is -gate's per-frame scroll delta, in device pixels. It is the
	// same 100px the warm criterion in test/gate walks with, so the two runs
	// measure the same travel.
	gateScrollStep = 100
	// The Darwin shim maps PageUp/PageDown to these private-use runes alongside
	// the existing arrow, Home, and End mappings.
	keyPageUp   rune = 0xF706
	keyPageDown rune = 0xF707
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

// errUsage marks a bad invocation, which the shell should see as exit status 2 rather
// than as a failure deep inside a run.
var errUsage = errors.New("usage")

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
	drainWG       sync.WaitGroup
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
	// restoredScrollTabID/restoredScrollY carry the offset a session restore
	// recorded for the active tab, so the restore's own load can re-apply it
	// after the fresh-navigation top-of-page reset. 0 means no pending intent.
	restoredScrollTabID uint64
	restoredScrollY     int32
	storage             *js.OriginStorageRegistry
	incognitoStorage    *js.OriginStorageRegistry
	emulationPreset     *DevicePreset
	// pacedSession is the document a screenshot or gate run loaded
	// synchronously, and pacedBG the canvas colour its plan was published with.
	// Both are set once before the loop starts and afterwards touched only by
	// the driver's pump goroutine (see pumpPaced), which is the same single
	// thread that ticks the JS runtime. The interactive path keeps its
	// documents in tabMgr instead.
	pacedSession *engine.Session
	pacedBG      frame.Color
	// mediaBackend plays this run's video, and is nil when the window that was
	// opened has no way to: a headless run, or a platform with no decoder. It
	// comes from the window rather than from the machine so a headless run on a
	// Mac does not claim a capability it is not using.
	mediaBackend   media.Backend
	theme          string
	crashedSession *session.State
	userscripts    *userscript.Manager
	flags          *flags.Manager
	perms          *perms.Manager
	preferences    preferences.Settings
	preferencesMu  sync.RWMutex
}

// build wires a scene, a grid, a pool, a composer, a scheduler, and a window into a
// frame path sized for c.
func build(c config) (*framePath, error) {
	dev := c.devSize()
	scale := float32(c.dpr)
	profilePreferences, prefErr := preferences.Load(c.preferencesFile)
	if prefErr != nil {
		fmt.Fprintf(os.Stderr, "goosie: preferences: %v\n", prefErr)
	}
	effectiveDir, err := effectiveDownloadDir(c.downloadDir, c.downloadDirExplicit, profilePreferences.DownloadDir)
	if err != nil {
		return nil, fmt.Errorf("goosie: download directory: %w", err)
	}
	c.downloadDir = effectiveDir
	profilePreferences.DownloadDir = c.downloadDir

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
		userscripts:      userscript.NewManager(c.userscriptDir),
		flags:            flags.NewManager(c.flagsFile),
		perms:            perms.NewManager(c.permsFile),
		preferences:      profilePreferences,
	}
	if f.theme == "" {
		f.theme = "system"
	}
	if !c.paced() && f.userscripts != nil {
		_ = f.userscripts.Load()
	}
	if !c.paced() && f.flags != nil {
		_ = f.flags.Load()
	}
	if !c.paced() && f.perms != nil {
		_ = f.perms.Load()
	}
	if !c.paced() {
		f.drainWG.Add(2)
		go func() {
			defer f.drainWG.Done()
			f.drainNavResults()
		}()
		go func() {
			defer f.drainWG.Done()
			f.drainImageResults()
		}()
	}
	if !c.paced() {
		f.toolbar = toolbar.NewState(dev.W, fonts)
		f.toolbar.SetScale(int32(c.dpr + 0.5))
		f.tabMgr = tabs.NewManager(func() {
			if f.window != nil {
				_ = f.window.Close()
			}
		})
		firstTab := f.tabMgr.NewTab()
		if c.appURL != "" {
			firstTab.SetStandaloneApp(true)
		}
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
			firstTab.SetURL(normalizeAddress(c.url, profilePreferences.SearchURLTemplate))
			firstTab.SetTitle(firstTab.URL())
		} else if f.crashedSession != nil {
			firstTab.SetURL("goosie://restore")
			firstTab.SetTitle("Restore Session - Goosie")
			f.restoredActiveURL = "goosie://restore"
		} else if shouldRestoreSession(c.private, profilePreferences.StartupMode) {
			if st, err := session.Load(c.sessionFile); err != nil {
				fmt.Fprintf(os.Stderr, "goosie: session: %v\n", err)
			} else if len(st.Tabs) > 0 {
				f.applySessionState(st)
			}
		}
		f.syncTabToToolbar()
		// Wire persistence only after startup has inspected and applied the
		// previous active-session journal. NewTab fires OnChange immediately;
		// installing the callback first would overwrite the crash journal with
		// the empty startup tab before it can be read.
		f.tabMgr.OnChange = func() {
			f.syncTabToToolbar()
			f.saveActiveSession()
		}
		f.saveActiveSession()

		f.toolbar.OnNavigate = func(rawURL string) {
			f.navigateTab(rawURL)
		}
		f.toolbar.OnTraverse = func(delta int) {
			f.traverseTab(delta)
		}
		f.toolbar.OnReload = func() {
			f.reloadTab()
		}
		f.toolbar.OnStop = func() {
			f.stopTab()
		}
		f.toolbar.OnFindChanged = func(query string) {
			f.runFind(query)
		}
		f.toolbar.OnFindNext = func(backward bool) {
			f.findStep(backward)
		}
		f.toolbar.OnFindClose = func() {
			f.closeFind()
			f.clearFindHighlights()
		}
		f.toolbar.OnDownloads = func() {
			f.navigateTab("goosie://downloads")
		}
		if f.downloads != nil {
			f.downloads.Subscribe(func(it *download.Item) {
				if f.toolbar != nil {
					f.toolbar.SetDownloadStats(f.downloads.ActiveCount(), f.downloads.TotalCount(), f.downloads.OverallProgress())
					f.publishAccessibility()
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

func (f *framePath) currentPreferences() preferences.Settings {
	f.preferencesMu.RLock()
	defer f.preferencesMu.RUnlock()
	return f.preferences
}

func (f *framePath) setPreferences(settings preferences.Settings) {
	f.preferencesMu.Lock()
	f.preferences = settings
	f.preferencesMu.Unlock()
	if f.downloads != nil {
		f.downloads.SetDir(settings.DownloadDir)
	}
}

func (f *framePath) downloadDirectory() string {
	if dir := f.currentPreferences().DownloadDir; dir != "" {
		return dir
	}
	return f.config.downloadDir
}

func shouldRestoreSession(private bool, startupMode string) bool {
	return !private && startupMode != preferences.StartupNewTab
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

	// 1. Search suggestion using the profile's configured engine.
	searchURL, err := preferences.SearchURL(f.currentPreferences().SearchURLTemplate, q)
	if err != nil {
		searchURL = "https://www.google.com/search?q=" + url.QueryEscape(q)
	}
	out = append(out, toolbar.Suggestion{
		Title: "Search the web for " + q,
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
		{"Browser Settings", "goosie://settings"},
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

	// 2. Bookmarks and history visits from the profile. A private tab must
	// never read either store; the -private run loads no stores at startup,
	// but the gate holds regardless. Search suggestions and internal pages
	// above are profile-independent and stay available.
	if !f.omniboxIsPrivate() {
		var profile []toolbar.Suggestion
		if f.bookmarks != nil {
			for _, bm := range f.bookmarks.List() {
				if suggestionMatches(lowerQ, bm.URL, bm.Title) {
					profile = append(profile, toolbar.Suggestion{
						Title: bm.Title,
						URL:   bm.URL,
						Kind:  toolbar.SuggestBookmark,
					})
				}
			}
		}
		if f.history != nil {
			entries := f.history.Entries()
			for i := len(entries) - 1; i >= 0; i-- {
				h := entries[i]
				if suggestionMatches(lowerQ, h.URL, h.Title) {
					profile = append(profile, toolbar.Suggestion{
						Title: h.Title,
						URL:   h.URL,
						Kind:  toolbar.SuggestHistory,
					})
				}
			}
		}
		for _, s := range profile {
			if !seen[s.URL] {
				seen[s.URL] = true
				out = append(out, s)
				if len(out) >= 8 {
					return out
				}
			}
		}
	}

	return out
}

// omniboxIsPrivate reports whether the omnibox must behave privately: the
// active tab is incognito, or the whole run started with -private.
func (f *framePath) omniboxIsPrivate() bool {
	if f.config.private {
		return true
	}
	if f.tabMgr != nil {
		if tab := f.tabMgr.Active(); tab != nil && tab.Incognito() {
			return true
		}
	}
	return false
}

// suggestionMatches reports whether one profile entry matches the lowered
// omnibox query by URL or title.
func suggestionMatches(lowerQ, entryURL, title string) bool {
	return strings.Contains(strings.ToLower(entryURL), lowerQ) || strings.Contains(strings.ToLower(title), lowerQ)
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
	// Asked of the window rather than of the platform: a headless run on a Mac
	// would otherwise be told the machine can play video, and every <video> in it
	// would report a position no frame ever followed.
	f.mediaBackend = platform.MediaBackend(w)
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
			cw.setTick(f.config.vsyncPeriod(), f.pumpChrome)
			cw.onContentHover = f.hoverContent
			cw.onContentActive = f.setContentActive
			cw.onActiveStyle = f.refreshActiveContent
			cw.onMediaControl = f.handleMediaControl
			cw.onNewIncognitoTab = func() { f.newIncognitoTab() }
			cw.onInspect = func() { f.navigateTab("goosie://inspect") }
			cw.onViewSource = f.viewSourceCurrentTab
			cw.onDuplicateTab = func() { f.duplicateTab() }
			cw.onReaderMode = func() { f.toggleReaderMode() }
			cw.bookmarks = f.bookmarks
			cw.onContextMenu = f.handleContextMenu
			cw.onContextMenuAction = f.handleContextMenuAction
			cw.onPrintPDF = f.printActiveTabToPDF
			cw.onDeviceEmulation = func() { f.cycleDeviceEmulation() }
			cw.onAccessibilityChanged = f.publishAccessibility
			cw.onAccessibilitySetValue = func(contentX, contentY int32, value string) bool {
				tab := f.tabMgr.Active()
				if tab == nil || tab.Session() == nil {
					return false
				}
				sx, sy := f.docPoint(contentX, contentY)
				focusChanged := tab.Session().FocusControl(sx, sy)
				if !tab.Session().FocusedEditable() {
					return false
				}
				f.setWindowIME(true)
				valueChanged := tab.Session().SetFocusedText(value)
				if focusChanged || valueChanged {
					f.reflowAfterControlEdit(tab)
				}
				f.repaintTab(tab, frame.Rect{})
				return true
			}
		}

	} else {
		f.window = w
	}
	f.publishAccessibility()
	f.loop = surface.NewLoop(f.window, f.sched, f.composer, f.rec)
	return nil
}

// programLabel is the prefix this command puts on every line it writes to
// stderr. The wrappers inside cmd/goosie carry it too, so printing one of them
// verbatim put it on the line twice: measured on a document whose CSS width is
// `8e8e8px`, the refusal arrived as
//
//	goosie: goosie: build session: CSS numeric value outside safe geometry range
//
// which reads like two programs reporting and makes the useful half stop being
// the start of the line.
const programLabel = "goosie: "

// programErrorLine returns the line main prints for err: the label exactly once,
// then whatever context the wrappers hold. Repeated wrapping can stack the label
// more than once, so it is dropped as often as it appears at the front; a label
// anywhere else in the message is someone else's context and stays.
func programErrorLine(err error) string {
	msg := err.Error()
	for strings.HasPrefix(msg, programLabel) {
		msg = strings.TrimPrefix(msg, programLabel)
	}
	return programLabel + msg
}

// main pins this goroutine to the process's main thread before anything can open a
// window, because AppKit requires the main thread and says so by aborting the process
// rather than by returning an error.
func main() {
	runtime.LockOSThread()
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, programErrorLine(err))
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
			fmt.Fprintln(os.Stderr, programErrorLine(err))
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
			layer, docSpec, bgColor, sess, _, err := loadURLCtx(context.Background(), f.client, f.fonts, normalizeURL(c.url), c.width, c.height, float32(c.dpr), c.downloadDir, false, false)
			if err != nil {
				return err
			}
			// The driver's scroll range comes from f.layer, so a paced document run that
			// swapped the plan but not this field would scroll a one-viewport page.
			f.layer = layer
			// The session is kept, not discarded: a paced run has no chrome window to
			// tick the page's event loop, so the driver pumps this session once per
			// vsync (see the watchVsync wiring below). The colour is what the plan was
			// published with, which pumpPaced compares against to notice a script that
			// changed the page background.
			f.pacedSession = sess
			f.pacedBG = bgColor
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
		// A paced run builds no chrome window, so the tick that pumps the page's
		// event loop in an interactive run (chromeWindow.setTick -> pumpChrome)
		// has no counterpart here. The driver's pump is that counterpart: one
		// event-loop turn per vsync, on its single goroutine. Attached before
		// Run so no measured frame sees an unpumped page.
		d.watchVsync(f.pumpJS)
		finish = d.done
	} else if c.screenshot {
		sd := newSteadyDriver(f, steadyFloorFrames)
		f.loop = surface.NewLoop(sd, f.sched, f.composer, f.rec)
		// Same seam as the gate driver: every vsync pumps the page before the run
		// asks whether it may settle, so a timer or image completion that lands
		// mid-run is drawn rather than truncated.
		sd.watchVsync(f.pumpJS)
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
		// Every document goes before the window does. Stopping a video means
		// stopping audio on the process main thread, and the window closing is what
		// stops the loop that services that thread — so after this point there is
		// nothing left that could answer. Doing it here is also what makes "the app
		// quit" mean "the video stopped", rather than relying on process exit to
		// take the audio device with it.
		f.closeAllSessions()
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
		if f.flags != nil {
			if err := f.flags.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "goosie: save flags: %v\n", err)
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
