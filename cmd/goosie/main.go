package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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
	gateScrollStep            = 100
	keyPageUp            rune = 0xF706
	keyPageDown          rune = 0xF707
	defaultFrames             = 600
	displayPeriod             = 16 * time.Millisecond
	benchPeriod               = time.Microsecond
	rasterQueue               = 4096
	interactiveDocHeight      = 20_000
	glyphAtlasBudget          = 4 << 20
	ringFrames                = 120
)

var processStart = time.Now()

var errUsage = errors.New("usage")

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

type framePath struct {
	window                surface.Window
	loop                  *surface.Loop
	sched                 *raster.Scheduler
	pool                  *raster.Pool
	composer              *surface.Composer
	layer                 *frame.Layer
	rec                   *frame.FrameRecorder
	spec                  paint.SceneSpec
	config                config
	newWindow             func()
	sizeMu                sync.RWMutex
	toolbar               *toolbar.State
	tabMgr                *tabs.TabManager
	client                net.HTTP
	privateClient         net.HTTP
	fonts                 *raster.Fonts
	bookmarks             *bookmarks.Store
	history               *history.Store
	downloads             *download.Manager
	started               time.Time
	navResults            chan navResult
	imgResults            chan imgResult
	drainWG               sync.WaitGroup
	zoom                  float64
	runCtx                context.Context
	runCancel             context.CancelFunc
	navWg                 sync.WaitGroup
	sessionSnapshotEvery  time.Duration
	crashedMu             sync.RWMutex
	sessionDisabled       atomic.Bool
	sessionMu             sync.Mutex
	resubmitMu            sync.Mutex
	resubmits             map[uint64]*pendingResubmit
	scriptTimeout         time.Duration
	document              string
	docHeight             int32
	findMu                sync.Mutex
	findMatches           []engine.Match
	findIdx               int
	findQuery             string
	findSignature         findSignature
	selDrag               bool
	contentDrag           *contentDragGesture
	restoredActiveURL     string
	restoredScrollTabID   uint64
	restoredScrollY       int32
	storage               *js.OriginStorageRegistry
	incognitoStorage      *js.OriginStorageRegistry
	broadcastHub          *js.BroadcastHub
	incognitoBroadcastHub *js.BroadcastHub
	emulationPreset       *DevicePreset
	pacedSession          *engine.Session
	pacedBG               frame.Color
	mediaBackend          media.Backend
	theme                 string
	crashedSession        *session.State
	userscripts           *userscript.Manager
	flags                 *flags.Manager
	perms                 *perms.Manager
	preferences           preferences.Settings
	preferencesMu         sync.RWMutex
	inspectRepaintMu      sync.Mutex
	inspectRepaintPending []*tabs.Tab
}

func build(c config) (*framePath, error) {
	p, err := buildProfile(c)
	if err != nil {
		return nil, err
	}
	return buildWindow(c, p)
}

func buildWindow(c config, p *profile) (*framePath, error) {
	dev := c.devSize()
	scale := float32(c.dpr)
	c.downloadDir = p.preferences.DownloadDir

	var layer *frame.Layer
	var spec paint.SceneSpec
	var err error

	if c.url == "" && (c.paced() || c.sceneSet) {
		spec, err = sceneSpec(c.scene)
		if err != nil {
			_ = p.client.Close()
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

	wp := raster.New(raster.DefaultWorkers(), rasterQueue, raster.DefaultRaster(p.fonts, raster.NewGlyphAtlas(glyphAtlasBudget, p.fonts)))
	wp.Start(context.Background())

	capacity := ringFrames
	if c.paced() {
		capacity = c.frames
	}
	rec := frame.NewFrameRecorder(capacity)
	rec.SetStartupReference(processStart)
	f := &framePath{
		sched:                 raster.NewScheduler(layer, wp, frame.Viewport{Size: dev}, scale, raster.Pref{}),
		pool:                  wp,
		composer:              surface.NewComposer(dev, frame.NewBitmapPool(dev, 2)),
		layer:                 layer,
		docHeight:             spec.DocHeight,
		rec:                   rec,
		spec:                  spec,
		config:                c,
		client:                p.client,
		privateClient:         p.privateClient,
		fonts:                 p.fonts,
		bookmarks:             p.bookmarks,
		history:               p.history,
		downloads:             p.downloads,
		navResults:            make(chan navResult, 16),
		imgResults:            make(chan imgResult, 16),
		zoom:                  1.0,
		storage:               p.storage,
		incognitoStorage:      p.incognitoStorage,
		broadcastHub:          p.broadcastHub,
		incognitoBroadcastHub: p.incognitoBroadcastHub,
		theme:                 p.theme,
		userscripts:           p.userscripts,
		flags:                 p.flags,
		perms:                 p.perms,
		preferences:           p.preferences,
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
		f.toolbar = toolbar.NewState(dev.W, p.fonts)
		f.toolbar.SetScale(int32(c.dpr + 0.5))
		f.toolbar.SetTheme(f.theme)
		f.tabMgr = tabs.NewManager(func() {
			if f.window != nil {
				_ = f.window.Close()
			}
		})
		f.tabMgr.SetTheme(f.theme)
		firstTab := f.tabMgr.NewTab()
		if c.appURL != "" {
			firstTab.SetStandaloneApp(true)
		}
		firstTab.SetLayer(layer)
		firstTab.SetBGColor(f.chromeTabBG())
		if !c.private && !c.paced() && c.sessionActiveFile != "" {
			if info, err := os.Stat(c.sessionActiveFile); err == nil && !info.IsDir() {
				if st, err := session.Load(c.sessionActiveFile); err == nil && len(st.Tabs) > 0 {
					f.setCrashedSession(&st)
				}
			}
		}
		if c.url != "" {
			firstTab.SetURL(normalizeAddress(c.url, p.preferences.SearchURLTemplate))
			firstTab.SetTitle(firstTab.URL())
		} else if f.crashedSessionState() != nil {
			firstTab.SetURL("goosie://restore")
			firstTab.SetTitle("Restore Session - Goosie")
			f.restoredActiveURL = "goosie://restore"
		} else if shouldRestoreSession(c.private, p.preferences.StartupMode) {
			if st, err := session.Load(c.sessionFile); err != nil {
				fmt.Fprintf(os.Stderr, "goosie: session: %v\n", err)
			} else if len(st.Tabs) > 0 {
				f.applySessionState(st)
			}
		}
		f.syncTabToToolbar()
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
		f.toolbar.OnFindOpen = f.runFind
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

func (f *framePath) isHighContrast() bool {
	if f == nil {
		return false
	}
	return strings.ToLower(f.theme) == "high-contrast"
}

func (f *framePath) chromeTabBG() frame.Color {
	switch {
	case f.isDarkMode():
		return frame.RGB(24, 24, 24)
	case f.isHighContrast():
		return frame.RGB(0, 0, 0)
	default:
		return frame.RGB(255, 255, 255)
	}
}

func (f *framePath) setTheme(theme string) {
	if f == nil {
		return
	}
	f.theme = strings.ToLower(strings.TrimSpace(theme))
	if f.toolbar != nil {
		f.toolbar.SetTheme(f.theme)
	}
	if f.tabMgr != nil {
		f.tabMgr.SetTheme(f.theme)
		for _, tab := range f.tabMgr.Tabs() {
			if tab.Incognito() {
				tab.SetBGColor(frame.RGB(40, 44, 52))
			} else {
				tab.SetBGColor(f.chromeTabBG())
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

func (f *framePath) querySuggestions(query string) []toolbar.Suggestion {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	lowerQ := strings.ToLower(q)
	var out []toolbar.Suggestion
	seen := make(map[string]bool)

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
	now := time.Now()
	candidates := make(map[string]*suggestionCandidate)
	order := 0
	for _, p := range internalPages {
		pagePath := strings.TrimPrefix(p.URL, "goosie://")
		matchScheme := strings.HasPrefix(lowerQ, "goosie") || strings.HasPrefix(lowerQ, "about")
		matchContent := strings.Contains(strings.ToLower(p.Title), lowerQ) || strings.Contains(pagePath, lowerQ)
		if matchScheme || matchContent {
			addSuggestionCandidate(candidates, toolbar.Suggestion{
				Title: p.Title,
				URL:   p.URL,
				Kind:  toolbar.SuggestBookmark,
			}, bookmarkFrecency, time.Time{}, &order)
		}
	}

	if !f.omniboxIsPrivate() {
		if f.bookmarks != nil {
			for _, bm := range f.bookmarks.List() {
				if suggestionMatches(lowerQ, bm.URL, bm.Title) {
					addSuggestionCandidate(candidates, toolbar.Suggestion{
						Title: bm.Title,
						URL:   bm.URL,
						Kind:  toolbar.SuggestBookmark,
					}, bookmarkFrecency, time.Time{}, &order)
				}
			}
		}
		if f.history != nil {
			entries := f.history.Entries()
			counts := make(map[string]int, len(entries))
			for _, h := range entries {
				counts[h.URL]++
			}
			for _, h := range entries {
				if !suggestionMatches(lowerQ, h.URL, h.Title) {
					continue
				}
				visits := h.Visits
				if visits <= 0 {
					visits = counts[h.URL]
				}
				addSuggestionCandidate(candidates, toolbar.Suggestion{
					Title: h.Title,
					URL:   h.URL,
					Kind:  toolbar.SuggestHistory,
				}, history.FrecencyScore(visits, h.At, now), h.At, &order)
			}
		}
	}

	ranked := make([]*suggestionCandidate, 0, len(candidates))
	for _, c := range candidates {
		ranked = append(ranked, c)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if !ranked[i].recent.Equal(ranked[j].recent) {
			return ranked[i].recent.After(ranked[j].recent)
		}
		return ranked[i].order < ranked[j].order
	})
	for _, c := range ranked {
		if seen[c.suggestion.URL] {
			continue
		}
		seen[c.suggestion.URL] = true
		out = append(out, c.suggestion)
		if len(out) >= 8 {
			return out
		}
	}

	return out
}

const bookmarkFrecency = 1.0

type suggestionCandidate struct {
	suggestion toolbar.Suggestion
	score      float64
	recent     time.Time
	order      int
}

func addSuggestionCandidate(pool map[string]*suggestionCandidate, s toolbar.Suggestion, score float64, recent time.Time, order *int) {
	if c, ok := pool[s.URL]; ok {
		if score > c.score {
			c.score = score
		}
		if recent.After(c.recent) {
			c.recent = recent
		}
		if c.suggestion.Kind == toolbar.SuggestHistory && s.Kind == toolbar.SuggestBookmark {
			c.suggestion.Title = s.Title
			c.suggestion.Kind = toolbar.SuggestBookmark
		}
		return
	}
	*order++
	pool[s.URL] = &suggestionCandidate{
		suggestion: s,
		score:      score,
		recent:     recent,
		order:      *order,
	}
}

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

func suggestionMatches(lowerQ, entryURL, title string) bool {
	return strings.Contains(strings.ToLower(entryURL), lowerQ) || strings.Contains(strings.ToLower(title), lowerQ)
}

func (f *framePath) openWindow() error {
	size := f.devSize()
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
	f.mediaBackend = platform.MediaBackend(w)
	if f.toolbar != nil && f.tabMgr != nil {
		cw := newChromeWindow(w, f.toolbar, f.tabMgr,
			func(id uint64) { f.switchTab(id) },
			func() { f.newTab() },
			func(id uint64) { f.closeTab(id) },
			func() { f.reopenTab() },
			f.handleResize,
			f.handleZoom,
			f.handleLinkClick,
			f.toggleBookmark,
			f.focusClick,
			f.contentKeyAction,
			f.dragSelect,
			f.imeEvent,
			f.copySelection,
			f.hitTestLink,
			f.hoverCursor,
			f.fonts,
			int32(f.config.dpr+0.5),
		)
		cw.onContentDrag = f.pointerDrag
		cw.onCancelDrag = f.cancelPointerDrag
		f.window = cw
		if cw, ok := f.window.(*chromeWindow); ok {
			cw.setTick(f.config.vsyncPeriod(), f.pumpChrome)
			cw.onContentHover = f.hoverContent
			cw.onContentActive = f.setContentActive
			cw.onActiveStyle = f.refreshActiveContent
			cw.onMediaControl = f.handleMediaControl
			cw.onNewIncognitoTab = func() { f.newIncognitoTab() }
			cw.onNewWindow = f.newWindow
			cw.onInspect = func() { f.navigateTab("goosie://inspect") }
			cw.onViewSource = f.viewSourceCurrentTab
			cw.onDuplicateTab = func() { f.duplicateTab() }
			cw.onReaderMode = func() { f.toggleReaderMode() }
			cw.bookmarks = f.bookmarks
			cw.onContextMenu = f.handleContextMenu
			cw.onContextMenuAction = f.handleContextMenuAction
			cw.onTabMenuAction = f.handleTabContextMenuAction
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

const programLabel = "goosie: "

func programErrorLine(err error) string {
	msg := err.Error()
	for strings.HasPrefix(msg, programLabel) {
		msg = strings.TrimPrefix(msg, programLabel)
	}
	return programLabel + msg
}

var errScreenshotDownload = errors.New("screenshot response routed to downloader")

type screenshotDownloadError struct{ dd downloadDone }

func (e screenshotDownloadError) Error() string {
	return fmt.Sprintf("goosie: no screenshot for %s: the response was routed to the downloader (%s), saved to %s; nothing to paint", e.dd.url, e.dd.trigger(), e.dd.path)
}

func (e screenshotDownloadError) Unwrap() error { return errScreenshotDownload }

func exitCodeFor(err error) int {
	switch {
	case errors.Is(err, errUsage):
		return 2
	case errors.Is(err, errScreenshotDownload):
		return 3
	default:
		return 1
	}
}

func main() {
	runtime.LockOSThread()
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, programErrorLine(err))
		os.Exit(exitCodeFor(err))
	}
}

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

func run(args []string) (runErr error) {
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
	if !c.paced() && !c.private {
		m, err := newWindowManager(c)
		if err != nil {
			return err
		}
		defer m.cleanup()
		return m.run()
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

	if c.url != "" {
		if c.paced() {
			layer, docSpec, bgColor, sess, _, err := loadURLCtx(context.Background(), f.client, f.fonts, normalizeURL(c.url), c.width, c.height, float32(c.dpr), c.downloadDir, false, false)
			if err != nil {
				var dd downloadDone
				if errors.As(err, &dd) && (c.screenshot || c.screenshotSeq != "") {
					return screenshotDownloadError{dd: dd}
				}
				return err
			}
			f.layer = layer
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

	var d *driver
	var finish chan struct{}
	if c.gate || c.bench {
		d = newDriver(f, c.frames)
		f.loop = surface.NewLoop(d, f.sched, f.composer, f.rec)
		d.watchVsync(f.pumpJS)
		finish = d.done
	} else if c.screenshot || c.screenshotSeq != "" {
		sd := newSteadyDriver(f, steadyFloorFrames)
		f.loop = surface.NewLoop(sd, f.sched, f.composer, f.rec)
		sd.watchVsync(f.pumpJS)
		sd.watchReveal(f.pendingReveal)
		sd.watchIdle(func() bool {
			st := f.loop.Stats()
			return st.Last.Needed == 0 && st.Last.PaintingBytes == 0
		})
		sd.watchProgress(func() (done, outstanding int64) {
			st := f.loop.Stats()
			outstanding = int64(st.Last.Needed) + st.Last.PaintingBytes
			return st.Rasterize, outstanding
		})
		finish = sd.done
		var seq *frameSequence
		if c.screenshotSeq != "" {
			writer, ok := f.window.(interface{ WritePNG(string) error })
			if !ok {
				return fmt.Errorf("%w: -screenshot-seq needs a backend that can write PNGs; run it with -backend headless", errUsage)
			}
			if err := os.MkdirAll(c.screenshotSeq, 0o755); err != nil {
				return fmt.Errorf("goosie: create sequence directory: %w", err)
			}
			backend := c.backend
			if backend == "" {
				backend = platform.Headless
			}
			seq = newFrameSequence(c, c.screenshotSeq, c.seqCount, c.seqInterval, writer.WritePNG, func() (bool, int, int64, int64, int64) {
				st := f.loop.Stats()
				return st.Last.Needed == 0 && st.Last.PaintingBytes == 0, st.Last.Needed, st.Last.PaintingBytes, st.Presents, st.Frames
			}, f.zoom, backend)
			seq.setScroll(seqScroll{
				Screens: c.seqScrollScreens,
				Screen:  func() int32 { return scrollScreenHeight(f.sched.Viewport().Size.H) },
				Scroll: func(dy int32) {
					vp := f.sched.Viewport()
					vp.Offset.Y += dy
					f.sched.SetViewport(vp)
					if sess := f.pacedSession; sess != nil {
						at := f.sched.Viewport().Offset
						sess.SyncViewportScroll(float64(at.X), float64(at.Y))
					}
				},
				Offset: func() int32 { return f.sched.Viewport().Offset.Y },
			})
			sd.watchSequence(seq.capture, seq.horizon())
		}
		defer func() {
			if hw, ok := f.window.(interface{ WritePNG(string) error }); ok {
				if c.out != "" {
					if err := hw.WritePNG(c.out); err != nil {
						fmt.Fprintf(os.Stderr, "goosie: write screenshot: %v\n", err)
					}
				}
			}
			if seq != nil {
				if err := seq.write(); err != nil && runErr == nil {
					runErr = fmt.Errorf("goosie: screenshot sequence: %w", err)
				}
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f.runCtx, f.runCancel = context.WithCancel(context.Background())
	defer f.runCancel()
	go f.runSessionSnapshots(f.runCtx)

	loopErr := make(chan error, 1)
	stopped := make(chan struct{})
	f.started = time.Now()
	go func() {
		err := f.loop.Run(ctx)
		loopErr <- err
		close(stopped)
	}()

	go func() {
		select {
		case <-stopped:
		case <-sig:
		case <-finish:
		}
		cancel()
		<-stopped
		f.runCancel()
		f.navWg.Wait()
		f.closeAllSessions()
		_ = f.window.Close()
		if f.privateClient != nil {
			_ = f.privateClient.Close()
		}
	}()

	if r, ok := f.window.(platform.Runner); ok {
		r.Run()
	}
	<-stopped
	if !c.paced() && !c.private {
		f.sessionMu.Lock()
		if !f.sessionDisabled.Load() {
			if err := session.Save(c.sessionFile, sessionState(f)); err != nil {
				fmt.Fprintf(os.Stderr, "goosie: save session: %v\n", err)
			}
		}
		f.sessionMu.Unlock()
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
	if c.screenshot || c.screenshotSeq != "" {
		return nil
	}
	return f.report(d)
}
