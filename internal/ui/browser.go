package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/engine/eventloop"
	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	"github.com/vyquocvu/goosie/internal/engine/session"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/memory"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/profile"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/ui/devtools"

	"golang.org/x/net/html"
)

// fixedHeightLayout is a custom layout that sets a fixed height for a widget
type fixedHeightLayout struct {
	height float32
}

func (l *fixedHeightLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(0, l.height)
	}
	// Use the widget's minimum width but override the height
	minSize := objects[0].MinSize()
	return fyne.NewSize(minSize.Width, l.height)
}

func (l *fixedHeightLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	// Position the widget to fill the width but constrain height
	objects[0].Resize(fyne.NewSize(size.Width, l.height))
	objects[0].Move(fyne.NewPos(0, 0))
}

// fixedSizeLayout is a custom layout that pins an object to an exact width × height.
type fixedSizeLayout struct {
	width, height float32
}

func (l *fixedSizeLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.width, l.height)
}

func (l *fixedSizeLayout) Layout(objects []fyne.CanvasObject, _ fyne.Size) {
	for _, o := range objects {
		o.Resize(fyne.NewSize(l.width, l.height))
		o.Move(fyne.NewPos(0, 0))
	}
}

// compactBtn wraps a button in a fixed-size container so it renders as a
// small oval/capsule rather than a full-height bar button.
func compactBtn(btn *widget.Button) *fyne.Container {
	const btnH float32 = 28
	// For icon-only / short labels keep it square-ish; wider labels get more room.
	minW := btn.MinSize().Width
	if minW < btnH {
		minW = btnH
	}
	return container.New(&fixedSizeLayout{width: minW, height: btnH}, btn)
}

// NavigationCallback is a function that is called when navigation is requested
type NavigationCallback func(url string)

type BrowserDependencies struct {
	Profile       *profile.Profile
	Bookmarks     *profile.BookmarkStore
	History       *profile.HistoryStore
	SessionStore  *profile.SessionStore
	NavSession    *session.Session
	SettingsStore *profile.SettingsStore
	Storage       *profile.StorageStore
	Network       *goosienet.Service
	Memory        *memory.Manager
	App           fyne.App
	Window        fyne.Window
	Headless      bool
}

// Browser represents the browser UI
type Browser struct {
	app                 fyne.App
	window              fyne.Window
	state               *BrowserState
	settings            *Settings
	themeManager        *ThemeManager
	urlEntry            *widget.Entry
	backButton          *widget.Button
	forwardButton       *widget.Button
	refreshButton       *widget.Button
	bookmarkButton      *widget.Button
	bookmarksListButton *widget.Button
	historyButton       *widget.Button
	downloadsButton     *widget.Button
	settingsButton      *widget.Button
	consoleButton       *widget.Button
	loadingBar          *widget.ProgressBarInfinite
	loadingBarContainer *fyne.Container
	onNavigate          NavigationCallback
	tabs                *container.DocTabs
	tabItems            []*Tab
	consolePanel        *ConsolePanel
	inspectPanel        *InspectPanel
	inspectButton       *widget.Button
	// devToolsMenu assembles and shows the right-click context menu. A
	// single instance is reused across right-click events.
	devToolsMenu       *DevToolsContextMenu
	devToolsDock       *devtools.Dock
	devToolsVisible    bool
	devToolsSplit      *container.Split
	dockContainer      *fyne.Container
	breadcrumbBar      *BreadcrumbBar
	breadcrumbBox      *fyne.Container
	screenshotButton   *widget.Button
	devToolsButton     *widget.Button
	dirtyOverlayButton *widget.Button
	fpsButton          *widget.Button
	fpsBar             *FPSBar
	// lastFPSBarUpdate gates updateFPSBar so it fires at most ~5 Hz during
	// rapid scrolling. Without this, every scroll frame calls label.SetText +
	// label.Refresh() — an extra Fyne widget repaint per scroll tick.
	lastFPSBarUpdate time.Time
	RendererFactory  func() HTMLRenderer
	deps             BrowserDependencies
	shortcuts        *ShortcutRegistry
	headless         bool

	// UseRasterCanvas enables the experimental single-surface raster
	// canvas (M1.3) instead of the tree-of-widgets viewport. When true,
	// new tabs display rendered pixels through one InteractiveRasterCanvas
	// widget; Fyne only acts as the window/display layer and Goosie owns
	// 100% of the graphics pipeline. Flag-gated for A/B comparison.
	UseRasterCanvas bool
}

// windowResizeWatcher wraps content and fires a callback when its size
// changes (e.g. on window resize). It sits at the top of the content
// hierarchy to detect size changes propagated by Fyne's layout system.
// Resize events are debounced (100ms) to avoid excessive re-renders during
// continuous window resizing.
type windowResizeWatcher struct {
	widget.BaseWidget
	content     fyne.CanvasObject
	lastSize    fyne.Size
	onResize    func(fyne.Size)
	resizeTimer *time.Timer
	resizeMu    sync.Mutex
}

func newWindowResizeWatcher(content fyne.CanvasObject, onResize func(fyne.Size)) *windowResizeWatcher {
	w := &windowResizeWatcher{
		content:  content,
		onResize: onResize,
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *windowResizeWatcher) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.content)
}

func (w *windowResizeWatcher) Resize(size fyne.Size) {
	w.BaseWidget.Resize(size)
	if w.lastSize != size && w.onResize != nil {
		w.lastSize = size

		w.resizeMu.Lock()
		if w.resizeTimer != nil {
			w.resizeTimer.Stop()
		}

		w.resizeTimer = time.AfterFunc(100*time.Millisecond, func() {
			w.onResize(size)
		})
		w.resizeMu.Unlock()
	}
}

// Tab represents a single browser tab
type Tab struct {
	title         string
	content       fyne.CanvasObject
	contentBox    *widget.RichText
	contentScroll *container.Scroll
	htmlRenderer  HTMLRenderer
	state         *BrowserState
	browser       *Browser
	jsRuntime     *js.Runtime
	rawSource     string

	// eventLoop is the tab's bounded engine scheduling policy for input.
	// Scroll events land in the loop's latest-wins slot and click/key
	// events in its FIFO, so a burst of scrolls collapses into one
	// latest-viewport render while discrete input keeps its order. It is
	// driven purely from the Fyne main thread (PostInput from the
	// scroll/key callbacks, DrainInput from a deferred UI turn) and is
	// closed when the tab closes. See internal/engine/eventloop.
	eventLoop *eventloop.Loop

	// drainScheduled is a UI-thread flag guaranteeing at most one input
	// drain is queued per UI turn: further scroll/key events while a
	// drain is pending only update the loop's coalesced state.
	drainScheduled bool
	// lastCoalescedScroll is the last CoalescedScrollEvents counter value
	// seen by the drain, used to report the per-render coalesced delta to
	// the renderer's FrameMetrics HUD.
	lastCoalescedScroll uint64
	// renderGateMu guards lastPresent and boundaryTimer. In the real app
	// both are touched on the UI thread only; the mutex makes the
	// frame-boundary timer safe when browser.do runs inline (headless mode
	// and tests), where the timer callback executes on its own goroutine.
	renderGateMu sync.Mutex
	// lastPresent is the wall time of the most recent scroll-frame present.
	// scheduleScrollPresent gates on it against the loop's frame budget so
	// sub-frame scroll bursts collapse into one canvas rebuild per display
	// frame instead of one per UI turn.
	lastPresent time.Time
	// boundaryTimer defers a scroll present to the frame boundary (at most
	// one pending). The render request stays in the loop's replace-latest
	// queue while it waits, so the boundary always paints the newest
	// viewport — no final frame is ever lost.
	boundaryTimer *time.Timer
	// docGeneration increments every time a new document is rendered into
	// the tab; it feeds the loop's Generation so scroll renders from a
	// superseded document are dropped before presentation.
	docGeneration uint64

	// Mouse routing state (PR9), all UI-thread owned.
	// lastViewportY mirrors the loop's latest drained scroll offset so
	// mouse hit-tests can convert widget-space positions to content
	// coordinates without reaching into the canvas.
	lastViewportY float32
	// lastPaintedViewportY / lastPaintedViewportHeight track the viewport
	// of the most recently painted frame. presentRenderResult skips the
	// canvas rebuild when these match the incoming request — e.g. at
	// rubber-band boundaries or when coalesced events share the same final
	// Y. Kept separate from lastViewportY, which is updated in the drain
	// for mouse hit-testing and is always equal to the incoming request Y.
	lastPaintedViewportY      float32
	lastPaintedViewportHeight float32
	// lastHoverHit throttles hover hit-tests: MouseMoved posts fire at
	// display refresh rate, and each hit test walks the layout tree.
	lastHoverHit time.Time
	// lastHoverNodeID suppresses repeated inspect callbacks while the
	// pointer stays over the same element.
	lastHoverNodeID int64

	// jsSession is the single-owner goroutine wrapper around jsRuntime
	// (M8.1). All script execution and runtime configuration goes through
	// it so the goja VM is only ever touched by its owner goroutine;
	// async callbacks (timers, fetch completions) route back to the owner
	// via the runtime's enqueueTask hook. Created by SetJSRuntime and
	// closed on navigation/tab close.
	jsSession *js.Session

	// rasterCanvas is the experimental single-surface raster widget
	// (M1.3). Non-nil only when Browser.UseRasterCanvas is true. When
	// active, it replaces contentScroll as the viewport and handles all
	// pointer/keyboard interaction directly.
	rasterCanvas *InteractiveRasterCanvas
}

// window interface to allow testing
type window interface {
	SetTitle(string)
	SetContent(fyne.CanvasObject)
	ShowAndRun()
	Resize(fyne.Size)
}

// NewBrowser creates a new browser UI
func NewBrowser() *Browser {
	a := app.NewWithID("com.github.vyquocvu.goosie")
	w := a.NewWindow("Goosie")

	// Set window size
	w.Resize(fyne.NewSize(1000, 700))

	return newBrowserInternal(a, w)
}

func newBrowserInternal(a fyne.App, w fyne.Window, headless ...bool) *Browser {
	// Capture the Fyne main goroutine ID. NewBrowser (and the test
	// helpers that drive newBrowserInternal directly) construct the
	// app on the goroutine that Fyne's glfw driver pins with
	// runtime.LockOSThread in its init — so this is the same goroutine
	// the event loop runs on. Capturing here lets doAndWait decide
	// whether a caller is already on the main thread (and can run
	// synchronously) or on a worker goroutine (and needs to marshal via
	// fyne.DoAndWait).
	captureMainGoroutineID()

	h := len(headless) > 0 && headless[0]
	state := NewBrowserState()
	settings := NewSettings()
	themeManager := NewThemeManager(a, h)

	// Create thin, full-width loading progress bar with 5px height (initially hidden)
	loadingBar := widget.NewProgressBarInfinite()
	loadingBar.Hide()

	// Wrap the progress bar in a container with fixed height of 5px
	loadingBarContainer := container.New(&fixedHeightLayout{height: 5}, loadingBar)
	loadingBarContainer.Hide()

	browser := &Browser{
		app:                 a,
		window:              w,
		state:               state,
		settings:            settings,
		themeManager:        themeManager,
		loadingBar:          loadingBar,
		loadingBarContainer: loadingBarContainer,
		tabItems:            []*Tab{},
		headless:            h,
	}

	// Create console panel
	browser.consolePanel = NewConsolePanel(browser.toggleDevTools)
	browser.consolePanel.SetRefreshCallback(func() {
		// Clear console messages in the active tab's runtime
		if tab := browser.ActiveTab(); tab != nil && tab.jsRuntime != nil {
			tab.jsRuntime.ClearConsoleMessages()
			tab.jsRuntime.ClearJavaScriptErrors()
		}
	})
	browser.consolePanel.SetExecuteCallback(func(source string) {
		if tab := browser.ActiveTab(); tab != nil && tab.jsRuntime != nil {
			// Route through the session owner so the goja VM is only
			// touched by its owner goroutine (M8.1). The UI thread
			// blocks until the script finishes, as before.
			value, err := tab.RunScriptOnOwner(source)
			if err != nil {
				browser.consolePanel.AddMessage(js.ConsoleMessage{Level: "error", Message: err.Error(), Timestamp: time.Now(), Data: err.Error()})
				return
			}
			browser.consolePanel.AddMessage(js.ConsoleMessage{Level: "log", Message: value.String(), Timestamp: time.Now(), Data: value.String()})
		}
	})

	// Create breadcrumb bar (hidden until an element is selected)
	browser.breadcrumbBar = NewBreadcrumbBar(func(node *renderer.RenderNode) {
		browser.inspectPanel.SetElement(node, nil)
	})
	browser.breadcrumbBox = container.NewMax(browser.breadcrumbBar.CanvasObject())
	browser.breadcrumbBox.Hide()

	// Create fixed FPS bar (hidden until toggled on)
	browser.fpsBar = NewFPSBar()
	themeManager.AddListener(func(tt ThemeType) {
		browser.fpsBar.SetTheme(tt)
	})

	// Create inspect panel
	browser.inspectPanel = NewInspectPanel(browser.toggleDevTools)
	browser.inspectPanel.SetSelectNodeCallback(func(node *renderer.RenderNode, _ *renderer.LayoutBox) {
		if node != nil {
			browser.breadcrumbBar.SetSelection(node, nil)
			browser.breadcrumbBox.Show()
		} else {
			browser.breadcrumbBox.Hide()
		}
	})
	browser.inspectPanel.SetScrollToCallback(func(x, y float32) {
		if tab := browser.ActiveTab(); tab != nil && tab.contentScroll != nil {
			tab.contentScroll.ScrollToOffset(fyne.NewPos(0, y))
		}
	})

	// Create the unified dev tools dock. The activeTab closure lets the
	// dock pull live data from whichever tab is currently selected.
	browser.devToolsDock = devtools.NewDock(func() *devtools.TabContext {
		tab := browser.ActiveTab()
		if tab == nil {
			return nil
		}
		currentURL := tab.state.GetCurrentURL()
		secSummary := ""
		secInfo := devtools.SecurityInfo{}
		if strings.HasPrefix(currentURL, "https://") {
			secSummary = "TLS connection"
			if sum := browser.deps.Network.Security(); sum.Subject != "" {
				secSummary = "TLS " + sum.Subject
				secInfo = devtools.SecurityInfo{
					Scheme:    "https",
					Subject:   sum.Subject,
					Issuer:    sum.Issuer,
					NotBefore: sum.NotBefore.Format("Jan 2, 2006"),
					NotAfter:  sum.NotAfter.Format("Jan 2, 2006"),
				}
			}
		} else if strings.HasPrefix(currentURL, "http://") {
			secSummary = "No encryption"
			secInfo = devtools.SecurityInfo{Scheme: "http"}
		}
		var renderStats map[string]time.Duration
		if rr, ok := tab.htmlRenderer.(interface {
			Stats() map[string]time.Duration
		}); ok {
			renderStats = rr.Stats()
		}
		a11y := devtools.NewRenderTreeAccessibilityProvider(func() *renderer.RenderNode {
			if tab.htmlRenderer == nil {
				return nil
			}
			return tab.htmlRenderer.GetRoot()
		})
		ctx := &devtools.TabContext{
			Memory:          browser.deps.Memory,
			Renderer:        tab.htmlRenderer,
			JSRuntime:       tab.jsRuntime,
			RawSource:       tab.GetRawSource(),
			RequestLog:      &requestLogAdapter{log: browser.deps.Network.Log()},
			Storage:         browser.deps.Storage,
			CurrentURL:      currentURL,
			SecuritySummary: secSummary,
			SecurityInfo:    secInfo,
			Settings:        browser.settings,
			MetricsRecorder: &metricsAdapter{log: browser.deps.Network.Log()},
			SourceCache:     browser.deps.Network,
			RenderStats:     renderStats,
			Accessibility:   a11y,
		}
		return ctx
	}, h)

	// Wire the real InspectPanel and ConsolePanel into the dock tabs.
	browser.devToolsDock.SetElementsContent(browser.inspectPanel.CanvasObject())
	browser.devToolsDock.SetConsoleContent(browser.consolePanel.CanvasObject())

	// Build the dev-tools right-click context menu. The same instance
	// is reused across right-click events.
	browser.devToolsMenu = NewDevToolsContextMenu(DevToolsContextMenuOptions{
		Clipboard:           fyne.CurrentApp().Clipboard(),
		OnInspect:           browser.devToolsInspectAction,
		OnViewSource:        browser.devToolsViewSourceAction,
		OnViewComputedStyle: browser.devToolsViewComputedStyleAction,
	})

	firstTab := browser.newTabInternal()
	browser.tabItems = append(browser.tabItems, firstTab)

	browser.tabs = container.NewDocTabs(firstTab.AsTabItem())
	browser.tabs.CreateTab = func() *container.TabItem {
		tab := browser.NewTab()
		return tab.AsTabItem()
	}
	browser.tabs.OnSelected = func(tab *container.TabItem) {
		browser.updateNavigationButtons()
		browser.updateConsoleFromActiveTab()
		if activeTab := browser.ActiveTab(); activeTab != nil && activeTab.htmlRenderer != nil {
			if activeTab.htmlRenderer.FPSOverlayEnabled() {
				browser.fpsButton.SetText("FPS✓")
				if browser.fpsBar != nil {
					browser.fpsBar.Show()
					browser.updateFPSBar()
				}
			} else {
				browser.fpsButton.SetText("FPS")
				if browser.fpsBar != nil {
					browser.fpsBar.Hide()
				}
			}
		}
		if browser.devToolsVisible {
			if activeTab := browser.ActiveTab(); activeTab != nil {
				browser.inspectPanel.SetRenderer(activeTab.htmlRenderer)
			}
		}
	}
	browser.tabs.OnClosed = func(tabItem *container.TabItem) {
		for i, tab := range browser.tabItems {
			if tab.content == tabItem.Content {
				// Release the tab's input loop and JS session so late
				// work is rejected predictably instead of queued.
				tab.eventLoop.Close()
				tab.CloseJSSession()
				browser.tabItems = append(browser.tabItems[:i], browser.tabItems[i+1:]...)
				break
			}
		}
		if len(browser.tabItems) == 0 {
			newTab := browser.NewTab()
			browser.tabs.Append(newTab.AsTabItem())
		}
		browser.updateNavigationButtons()
	}
	browser.tabs.SetTabLocation(container.TabLocationTop)

	browser.createNavigationControls()

	return browser
}

func NewBrowserWithDependencies(deps BrowserDependencies) *Browser {
	var a fyne.App
	var w fyne.Window
	if deps.App != nil {
		a = deps.App
	} else {
		a = app.NewWithID("com.github.vyquocvu.goosie")
	}
	if deps.Window != nil {
		w = deps.Window
	} else {
		w = a.NewWindow("Goosie")
		// Set window size
		w.Resize(fyne.NewSize(1000, 700))
	}

	browser := newBrowserInternal(a, w, deps.Headless)
	browser.deps = deps
	browser.headless = deps.Headless
	browser.shortcuts = NewShortcutRegistry()
	browser.registerDefaultShortcuts()
	if deps.SettingsStore != nil {
		browser.settings.ApplyProfileSettings(deps.SettingsStore.Get())
	}
	if deps.Bookmarks != nil {
		for _, bookmark := range deps.Bookmarks.List() {
			browser.state.AddBookmark(bookmark.URL)
		}
	}
	return browser
}

func (b *Browser) registerDefaultShortcuts() {
	if b.shortcuts == nil {
		return
	}
	b.shortcuts.Register("focus-address", func() {
		if b.urlEntry != nil && b.window != nil {
			b.window.Canvas().Focus(b.urlEntry)
		}
	})
	b.shortcuts.Register("new-tab", func() {
		if b.tabs != nil {
			b.tabs.Append(b.NewTab().AsTabItem())
		}
	})
	b.shortcuts.Register(devToolsShortcutName, func() {
		b.toggleDevTools()
	})
}

// toggleConsole opens the dev tools dock and selects the Console tab.
func (b *Browser) toggleConsole() {
	if !b.devToolsVisible {
		b.toggleDevTools()
	} else {
		b.devToolsDock.SelectTab("Console")
		b.devToolsDock.Refresh()
	}
}

// ShowDevTools ensures the DevTools dock is visible and selects the given
// tab (if non-empty). Used by headless screenshot tooling and automation.
func (b *Browser) ShowDevTools(tab string) {
	if b.devToolsDock == nil {
		return
	}
	b.devToolsDock.EnsureTabs()
	// Re-wire panels that the dock constructs lazily so headless mode sees
	// the full InspectPanel / ConsolePanel content rather than stubs.
	if b.inspectPanel != nil {
		b.devToolsDock.SetElementsContent(b.inspectPanel.CanvasObject())
	}
	if b.consolePanel != nil {
		b.devToolsDock.SetConsoleContent(b.consolePanel.CanvasObject())
	}
	if !b.devToolsVisible {
		b.toggleDevTools()
	}
	if tab != "" {
		b.devToolsDock.SelectTab(tab)
	}
	b.devToolsDock.Refresh()
	// Force a re-layout of the split + dock containers so the newly
	// populated dock is rendered before the screenshot is taken.
	if b.dockContainer != nil {
		b.dockContainer.Refresh()
	}
	if b.devToolsSplit != nil {
		b.devToolsSplit.SetOffset(0.65)
		b.devToolsSplit.Refresh()
	}
	if b.window != nil && b.window.Canvas() != nil {
		b.window.Canvas().Refresh(b.window.Content())
	}
}

// toggleDevTools toggles the unified dev tools dock and persists the state.
func (b *Browser) toggleDevTools() {
	b.devToolsVisible = !b.devToolsVisible
	if b.devToolsSplit != nil {
		if b.devToolsVisible {
			b.devToolsSplit.Offset = 0.65
			if b.dockContainer != nil {
				b.dockContainer.Show()
			}
			b.devToolsDock.Show()
			b.devToolsDock.Refresh()
			if tab := b.ActiveTab(); tab != nil {
				b.inspectPanel.SetRenderer(tab.htmlRenderer)
			}
		} else {
			b.devToolsSplit.Offset = 1.0
			if b.dockContainer != nil {
				b.dockContainer.Hide()
			}
			b.devToolsDock.Hide()
		}
	}
	b.persistDevToolsState()
	if b.window != nil && b.window.Content() != nil {
		b.window.Content().Refresh()
	}
}

// restoreDevToolsState reads the profile and applies any saved dock visibility
// and split offset. Called once during Show() after devToolsSplit is created.
func (b *Browser) restoreDevToolsState() {
	if b.deps.SettingsStore == nil {
		return
	}
	s := b.deps.SettingsStore.Get()
	if s.DevToolsOpen && b.devToolsSplit != nil {
		b.devToolsVisible = true
		if b.dockContainer != nil {
			b.dockContainer.Show()
		}
		b.devToolsDock.Show()
		b.devToolsDock.Refresh()
		if s.DevToolsSplitOffset > 0 && s.DevToolsSplitOffset < 1 {
			b.devToolsSplit.Offset = s.DevToolsSplitOffset
		} else {
			b.devToolsSplit.Offset = 0.65
		}
	}
}

// persistDevToolsState writes the current dock visibility and split offset to
// the profile so they survive browser restarts.
func (b *Browser) persistDevToolsState() {
	if b.deps.SettingsStore == nil {
		return
	}
	s := b.deps.SettingsStore.Get()
	s.DevToolsOpen = b.devToolsVisible
	if b.devToolsSplit != nil {
		s.DevToolsSplitOffset = b.devToolsSplit.Offset
	}
	_ = b.deps.SettingsStore.Set(s) // best-effort
}

// toggleInspect opens the dev tools dock and selects the Elements tab.
func (b *Browser) toggleInspect() {
	if !b.devToolsVisible {
		b.toggleDevTools()
	} else {
		b.devToolsDock.SelectTab("Elements")
		b.devToolsDock.Refresh()
	}
}

// showDevToolsMenu renders and shows the right-click dev-tools menu for
// the active tab. The caller is responsible for hopping onto the Fyne
// UI goroutine before invoking this method.
func (b *Browser) showDevToolsMenu(node *renderer.RenderNode, layout *renderer.LayoutBox, abs fyne.Position) {
	if b.devToolsMenu == nil {
		return
	}
	// Anchor the popup to the active tab's content scroll container so
	// Fyne can position it correctly relative to the window.
	var anchor fyne.CanvasObject
	if tab := b.ActiveTab(); tab != nil && tab.contentScroll != nil {
		anchor = tab.contentScroll
	} else {
		anchor = b.window.Canvas().Content()
	}
	b.devToolsMenu.Show(anchor, node, layout, abs)
}

// devToolsInspectAction is the "Inspect Element" callback for the
// right-click context menu. It reveals the dev tools dock, switches
// to the Elements tab, and selects the right-clicked element.
func (b *Browser) devToolsInspectAction(node *renderer.RenderNode, layout *renderer.LayoutBox) {
	if node == nil {
		return
	}
	if !b.devToolsVisible {
		b.toggleDevTools()
	} else {
		b.devToolsDock.SelectTab("Elements")
		b.devToolsDock.Refresh()
	}
	tab := b.ActiveTab()
	if tab == nil {
		return
	}
	b.inspectPanel.SetRenderer(tab.htmlRenderer)
	b.inspectPanel.SetElement(node, layout)
}

// InspectElement programmatically inspects the given node and layout in the browser's DevTools.
func (b *Browser) InspectElement(node *renderer.RenderNode, layout *renderer.LayoutBox) {
	b.devToolsInspectAction(node, layout)
}

// devToolsViewSourceAction is the "View Source" callback. It shows a
// small dialog containing the outer HTML of the right-clicked node.
// It is a no-op when the user right-clicks empty canvas area.
func (b *Browser) devToolsViewSourceAction(node *renderer.RenderNode, _ *renderer.LayoutBox) {
	if node == nil {
		return
	}
	html := renderOuterHTMLString(node)
	if html == "" {
		html = "(empty)"
	}
	entry := widget.NewMultiLineEntry()
	entry.SetText(html)
	entry.Disable()
	scroll := container.NewVScroll(entry)
	dialog.ShowCustom("Element Source", "Close", scroll, b.window)
}

// devToolsViewComputedStyleAction is the "View Computed Style" callback.
// It mirrors the Styles tab in InspectPanel by presenting the rendered
// node's computed style as a key/value list inside a dialog. When the
// hit node has no computed style — e.g. text nodes — the dialog
// explains that no styles are available.
func (b *Browser) devToolsViewComputedStyleAction(node *renderer.RenderNode, _ *renderer.LayoutBox) {
	if node == nil {
		return
	}
	box := container.NewVBox()
	if node.ComputedStyle == nil {
		box.Add(widget.NewLabel("No computed styles available for this node."))
	} else {
		style := node.ComputedStyle
		box.Add(widget.NewLabelWithStyle(
			fmt.Sprintf("Computed style <%s>", node.TagName),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		box.Add(widget.NewLabel(fmt.Sprintf("display: %s", style.Display)))
		box.Add(widget.NewLabel(fmt.Sprintf("font-size: %.1fpx", style.FontSize)))
		box.Add(widget.NewLabel(fmt.Sprintf("width: %s", style.Width)))
		box.Add(widget.NewLabel(fmt.Sprintf("height: %s", style.Height)))
		box.Add(widget.NewLabel(fmt.Sprintf("color: %v", style.Color)))
	}
	scroll := container.NewVScroll(box)
	dialog.ShowCustom("Computed Style", "Close", scroll, b.window)
}

// renderOuterHTMLString is a thin wrapper around renderOuterHTML so
// the dialog callbacks (which live in this file) have a stable local
// surface for HTML serialisation.
func renderOuterHTMLString(node *renderer.RenderNode) string {
	return renderOuterHTML(node)
}

// toggleDirtyOverlay toggles the dirty-region overlay visualization on the
// active tab. When enabled, semi-transparent colored rectangles are shown
// over each paint command to indicate repaint regions.
func (b *Browser) toggleDirtyOverlay() {
	tab := b.ActiveTab()
	if tab == nil || tab.htmlRenderer == nil {
		return
	}

	enabled := !tab.htmlRenderer.DirtyOverlayEnabled()
	tab.htmlRenderer.SetDirtyOverlayEnabled(enabled)

	if enabled {
		b.dirtyOverlayButton.SetText("Ov✓")
	} else {
		b.dirtyOverlayButton.SetText("Ov")
	}

	// Force re-render to show or hide overlays
	tab.htmlRenderer.Refresh()
}

// toggleFPSOverlay toggles the live fixed FPS bar for the active tab.
// When enabled, the fixed bar displays live frame rate and latency metrics.
func (b *Browser) toggleFPSOverlay() {
	tab := b.ActiveTab()
	if tab == nil || tab.htmlRenderer == nil {
		return
	}

	enabled := !tab.htmlRenderer.FPSOverlayEnabled()
	tab.htmlRenderer.SetFPSOverlayEnabled(enabled)

	if enabled {
		b.fpsButton.SetText("FPS✓")
		if b.fpsBar != nil {
			b.fpsBar.Show()
			b.updateFPSBar()
		}
	} else {
		b.fpsButton.SetText("FPS")
		if b.fpsBar != nil {
			b.fpsBar.Hide()
		}
	}

	// Force re-render to update metrics
	tab.htmlRenderer.Refresh()
}

// fpsBarThrottle is the minimum interval between FPS bar UI updates.
// The bar shows live frame-rate stats but doesn't need to refresh faster
// than the human eye can read (~5 Hz), saving a label.Refresh() call per
// scroll frame.
const fpsBarThrottle = 200 * time.Millisecond

// updateFPSBar updates the live FPS metrics displayed in the fixed bar.
// It is throttled to at most ~5 Hz so it doesn't contribute a redundant
// Fyne widget Refresh() on every scroll tick.
func (b *Browser) updateFPSBar() {
	if b.fpsBar == nil || !b.fpsBar.Visible() {
		return
	}
	if time.Since(b.lastFPSBarUpdate) < fpsBarThrottle {
		return
	}
	b.lastFPSBarUpdate = time.Now()
	tab := b.ActiveTab()
	if tab == nil || tab.htmlRenderer == nil {
		return
	}
	stats := tab.htmlRenderer.FPSStats()
	metrics := tab.htmlRenderer.FrameMetrics()
	b.fpsBar.Update(stats, metrics)
}

// newTabInternal creates a new tab without adding it to the tab container
func (b *Browser) newTabInternal() *Tab {
	contentBox := widget.NewRichTextFromMarkdown("Welcome to Goosie! Enter a URL above to start browsing.")
	contentBox.Wrapping = fyne.TextWrapWord
	contentScroll := container.NewScroll(contentBox)
	var htmlRenderer HTMLRenderer
	if b.RendererFactory != nil {
		htmlRenderer = b.RendererFactory()
		if htmlRenderer != nil {
			htmlRenderer.SetWindow(b.window)
			htmlRenderer.SetNavigationCallback(func(url string) {
				if b.onNavigate != nil {
					b.onNavigate(url)
				}
			})
		}
	}

	tabState := NewBrowserState()

	tab := &Tab{
		title:         "New Tab",
		content:       contentScroll,
		contentBox:    contentBox,
		contentScroll: contentScroll,
		htmlRenderer:  htmlRenderer,
		state:         tabState,
		browser:       b,
	}

	// M1.3: When the raster canvas flag is enabled, create an
	// InteractiveRasterCanvas and wire it to the renderer. The raster
	// canvas replaces the tree-of-widgets viewport with a single pixel
	// surface that Goosie fully controls.
	if b.UseRasterCanvas && htmlRenderer != nil {
		rc := NewInteractiveRasterCanvas(htmlRenderer)
		rc.SetNavigateCallback(func(url string) {
			if b.onNavigate != nil {
				b.onNavigate(url)
			}
		})
		rc.SetInspectCallback(func(node *renderer.RenderNode, layout *renderer.LayoutBox) {
			tab.handleInspect(node, layout)
		})
		rc.SetContextMenuCallback(func(node *renderer.RenderNode, layout *renderer.LayoutBox, pos fyne.Position) {
			tab.handleContextMenu(node, layout, pos)
		})
		tab.rasterCanvas = rc
		tab.content = rc
	}
	deviceFPS := GetDeviceScreenRefreshRate()
	if deviceFPS <= 0 {
		deviceFPS = 60.0
	}
	frameDuration := time.Duration(float64(time.Second) / deviceFPS)
	if htmlRenderer != nil {
		if tr, ok := htmlRenderer.(interface{ SetTargetFPS(float64) }); ok {
			tr.SetTargetFPS(deviceFPS)
		}
	}

	tab.eventLoop = eventloop.New(eventloop.Config{
		InputQueueSize: 128,
		FrameBudget:    eventloop.NewFrameBudget(frameDuration),
		// The present callback runs on the UI thread (via
		// ProcessPendingResults) and is the only place a completed
		// render touches the canvas. PR4 keeps this callback unchanged
		// and moves the request execution off-thread.
		Present: tab.presentRenderResult,
	})
	return tab
}

// NewTab creates a new browser tab and adds it to the tab container
func (b *Browser) NewTab() *Tab {
	tab := b.newTabInternal()
	b.tabItems = append(b.tabItems, tab)
	return tab
}

// ActiveTab returns the currently active tab
func (b *Browser) ActiveTab() *Tab {
	if len(b.tabItems) == 0 {
		return nil
	}
	selectedIndex := b.tabs.SelectedIndex()
	if selectedIndex < 0 || selectedIndex >= len(b.tabItems) {
		return nil
	}
	return b.tabItems[selectedIndex]
}

// onWindowResize handles window resize events by updating the active tab's
// renderer dimensions and triggering a full re-render with the new layout.
func (b *Browser) onWindowResize(_ fyne.Size) {
	tab := b.ActiveTab()
	if tab == nil || tab.htmlRenderer == nil {
		return
	}

	contentSize := tab.contentScroll.Size()
	if contentSize.Width <= 0 || contentSize.Height <= 0 {
		return
	}

	tab.htmlRenderer.SetSize(contentSize.Width, contentSize.Height)
	tab.htmlRenderer.Refresh()

	canvasObject := tab.htmlRenderer.UpdateViewport()
	if canvasObject != nil {
		tab.contentScroll.Content = canvasObject
		tab.contentScroll.Refresh()
	}
}

// SetContent updates the displayed content (plain text)
func (b *Browser) SetContent(content string) {
	if tab := b.ActiveTab(); tab != nil {
		tab.contentBox.ParseMarkdown(content)
	}
}

// SetHTMLContent updates the displayed content from markdown-formatted HTML
func (b *Browser) SetHTMLContent(content string) {
	if tab := b.ActiveTab(); tab != nil {
		tab.contentBox.ParseMarkdown(content)
	}
}

// RenderHTMLContent renders HTML content using the canvas-based renderer on the active tab
func (b *Browser) RenderHTMLContent(ctx context.Context, htmlContent string) error {
	tab := b.ActiveTab()
	if tab == nil {
		return nil
	}
	return tab.RenderHTML(ctx, htmlContent)
}

// RenderParsedContent renders an already-parsed HTML node with the
// supplied external stylesheets on the active tab. It is the M3
// snapshot entry point exposed at the UI layer; callers (typically
// cmd/browser after the documentloader coordinator has finished
// fetching CSS) hand in a fully assembled set of stylesheets and the
// renderer does not perform any further network I/O for them.
func (b *Browser) RenderParsedContent(ctx context.Context, doc *html.Node, externalCSS []renderer.ExternalCSS) error {
	tab := b.ActiveTab()
	if tab == nil {
		return nil
	}
	return tab.RenderParsedContent(ctx, doc, externalCSS)
}

// RenderHTML renders HTML content using the canvas-based renderer for this specific tab
func (t *Tab) RenderHTML(ctx context.Context, htmlContent string) error {
	t.ensureHTMLRenderer()
	// A new document supersedes any pending scroll render from the
	// previous document; the loop drops results carrying the old
	// generation before presentation.
	t.bumpDocumentGeneration()

	// PR4 render split: renderers that implement frameSplitter build the
	// engine state (parse/style/layout) on the caller's goroutine and
	// only the canvas-object build is marshalled onto the Fyne main
	// thread. Renderers without the split fall back to the legacy
	// single-phase path (everything marshalled onto the UI thread).
	if fs, ok := t.htmlRenderer.(frameSplitter); ok {
		if err := fs.BuildHTML(ctx, htmlContent); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// Superseded navigation; not an error worth surfacing.
				return nil
			}
			return err
		}
		t.presentEngineFrame()
		return nil
	}

	// RenderHTML internally creates and mutates Fyne canvas objects
	// (canvas.Text, canvas.Rectangle, widget.NewButton, container
	// Refresh, etc.). All of those must run on the Fyne main thread —
	// see the comment on Browser.doAndWait — so marshal here when the
	// caller is on a non-Fyne goroutine (e.g. the navigation fetch
	// goroutine, the documentloader mutation-coalescer timer, the image
	// loader). When the caller is already on the main thread we run
	// directly to avoid a deadlock via fyne.DoAndWait.
	var (
		canvasObject fyne.CanvasObject
		renderErr    error
	)
	t.browser.doAndWait(func() {
		if t.browser.headless {
			t.contentScroll.Resize(fyne.NewSize(1000, 600))
			t.htmlRenderer.SetSize(1000, 600)
		}
		canvasObject, renderErr = t.htmlRenderer.RenderHTML(ctx, htmlContent)
	})
	if renderErr != nil {
		return renderErr
	}

	t.publishCanvasObject(canvasObject)
	return nil
}

// RenderParsedContent is the M3 snapshot entry point at the tab level.
// Behavior matches RenderHTML for the UI surface (lazy init, refresh
// callbacks, devtools wiring) but routes through renderer.RenderParsed
// so no further CSS fetching happens inside the renderer.
func (t *Tab) RenderParsedContent(ctx context.Context, doc *html.Node, externalCSS []renderer.ExternalCSS) error {
	t.ensureHTMLRenderer()
	// A new document supersedes any pending scroll render from the
	// previous document; the loop drops results carrying the old
	// generation before presentation.
	t.bumpDocumentGeneration()

	// PR4 render split: renderers that implement frameSplitter build the
	// engine state on the caller's goroutine — in production the
	// navigation and mutation-coalescer worker goroutines, so the Fyne
	// main thread is no longer blocked by parse/style/layout — and only
	// the canvas-object build is marshalled onto the UI thread.
	if fs, ok := t.htmlRenderer.(frameSplitter); ok {
		if err := fs.BuildParsed(ctx, doc, externalCSS); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// Superseded navigation; the session guards already
				// cover this, so do not surface an error UI.
				return nil
			}
			return err
		}
		t.presentEngineFrame()
		return nil
	}

	// Legacy fallback for renderers without the split: marshal the whole
	// render onto the Fyne main thread. Without this marshalling a burst
	// of JS DOM mutations logs the "Error in Fyne call thread, this
	// should have been called in fyne.Do[AndWait]" diagnostic and the
	// app appears not-responding because fyne.DoAndWait from main
	// deadlocks.
	var (
		canvasObject fyne.CanvasObject
		renderErr    error
	)
	t.browser.doAndWait(func() {
		if t.browser.headless {
			t.contentScroll.Resize(fyne.NewSize(1000, 600))
			t.htmlRenderer.SetSize(1000, 600)
		}
		canvasObject, renderErr = t.htmlRenderer.RenderParsed(ctx, doc, externalCSS)
	})
	if renderErr != nil {
		return renderErr
	}

	t.publishCanvasObject(canvasObject)
	return nil
}

// presentEngineFrame marshals the frameSplitter's canvas-object build
// onto the Fyne main thread and publishes the result. It is the PR4
// "present" half of the render split: the heavy engine phases already ran
// off-thread in the preceding Build* call, so the UI thread only
// constructs and refreshes canvas objects here.
func (t *Tab) presentEngineFrame() {
	if t.htmlRenderer == nil {
		return
	}
	fs, ok := t.htmlRenderer.(frameSplitter)
	if !ok {
		return
	}
	var canvasObject fyne.CanvasObject
	t.browser.doAndWait(func() {
		if t.browser.headless {
			t.contentScroll.Resize(fyne.NewSize(1000, 600))
			t.htmlRenderer.SetSize(1000, 600)
		}
		canvasObject = fs.PresentFrame()
	})
	if canvasObject != nil {
		t.publishCanvasObject(canvasObject)
	}
}

// ensureHTMLRenderer initializes the lazy renderer on first use and
// wires the navigation / inspect / refresh / context-menu callbacks.
// Shared by RenderHTML and RenderParsedContent. The current page URL is
// synced into the renderer on every call (not just creation) so that
// path-only hrefs (e.g. <a href="/path">) resolve against the page
// being rendered — even when the renderer was created eagerly by
// newTabInternal or a later navigation rendered a new page.
func (t *Tab) ensureHTMLRenderer() {
	if t.htmlRenderer == nil {
		if t.browser.RendererFactory == nil {
			// RenderHTML/RenderParsedContent will surface this as a render
			// error on the next call; we keep the panic-free path here.
			return
		}
		t.htmlRenderer = t.browser.RendererFactory()
		if t.htmlRenderer == nil {
			return
		}
		t.htmlRenderer.SetWindow(t.browser.window)
		t.htmlRenderer.SetHeadless(t.browser.headless)
		t.htmlRenderer.SetNavigationCallback(func(url string) {
			if t.browser.onNavigate != nil {
				t.browser.onNavigate(url)
			}
		})

		// Set up inspect callback. This is the direct-dispatch fallback:
		// when the mouse poster below is wired, canvas mouse events post
		// into the event loop instead and the drain dispatches through the
		// same handleInspect/handleContextMenu helpers.
		t.htmlRenderer.SetInspectCallback(func(node *renderer.RenderNode, layout *renderer.LayoutBox) {
			t.handleInspect(node, layout)
		})

		// Set up right-click context menu callback. Marshalled onto the UI
		// goroutine before showing the popup because fyne widgets must be
		// touched from the main thread.
		t.htmlRenderer.SetContextMenuCallback(func(node *renderer.RenderNode, layout *renderer.LayoutBox, abs fyne.Position) {
			t.handleContextMenu(node, layout, abs)
		})

		// Route canvas mouse events through the engine event loop (PR9):
		// the canvas posts raw immutable MouseInput values here, the tab
		// maps them into the loop's slots (mouse-move latest-wins, clicks
		// FIFO), and the drain owns hit-testing + dispatch. Passing nil
		// would restore the canvas's direct dispatch.
		t.htmlRenderer.SetMouseInputCallback(t.postCanvasMouseInput)

		// Set up refresh callback for the renderer
		t.htmlRenderer.SetRefreshCallback(func() {
			t.browser.do(func() {
				// Trigger a refresh of the scroll container to show changes
				refreshTabContent(t)
				// Also refresh inspector if visible
				if t.browser.devToolsVisible {
					t.browser.inspectPanel.SetRenderer(t.htmlRenderer)
				}
			})
		})

		// Sync Fyne scroll position with the renderer viewport so viewport
		// culling and hit-testing follow the user's scroll.
		//
		// The previous implementation collapsed bursts through the
		// renderer's ScrollCoalescer and deferred a single presentation
		// via fyne.Do. The engine event loop now owns that policy: the
		// scroll callback only posts an immutable InputEvent into the
		// tab's bounded loop (latest-wins slot), and one drain per UI
		// turn applies the latest viewport and refreshes once. A burst
		// of wheel events therefore produces exactly one canvas rebuild
		// and refresh, and the loop's counters feed the FrameMetrics
		// HUD (coalesced scroll, input-to-present latency).
		t.contentScroll.OnScrolled = func(pos fyne.Position) {
			if t.htmlRenderer == nil {
				return
			}
			scrollSize := t.contentScroll.Size()
			t.htmlRenderer.SetViewport(pos.Y, scrollSize.Height)
			t.lastViewportY = pos.Y
			t.postScrollViewport(pos.Y, scrollSize.Height)
		}
	}

	// Set the current URL for resolving relative links on every render.
	t.htmlRenderer.SetCurrentURL(t.state.GetCurrentURL())
}

// publishCanvasObject installs the rendered canvas into the scroll
// container on the main thread. Shared by RenderHTML and
// RenderParsedContent.
func (t *Tab) publishCanvasObject(canvasObject fyne.CanvasObject) {
	t.browser.do(func() {
		if t.browser.headless {
			t.contentScroll.Resize(fyne.NewSize(1000, 600))
			t.htmlRenderer.SetSize(1000, 600)
		}
		t.contentScroll.Content = canvasObject
		t.contentScroll.Refresh()
		if t.browser.devToolsVisible {
			t.browser.inspectPanel.SetRenderer(t.htmlRenderer)
		}
		t.browser.updateFPSBar()
	})
}

// RefreshFromMutation marshals a post-mutation canvas refresh onto the
// Fyne main thread. It is the present hook the JS typed-mutation sink
// calls from the session owner goroutine: the renderer has already
// applied the mutation values and invalidated its display-list cache, so
// this turn only rebuilds and repaints the canvas from the updated trees
// — no full reparse.
func (t *Tab) RefreshFromMutation() {
	if t == nil || t.browser == nil || t.htmlRenderer == nil {
		return
	}
	t.browser.do(func() {
		refreshTabContent(t)
	})
}

func refreshTabContent(tab *Tab) {
	if tab == nil || tab.htmlRenderer == nil || tab.contentScroll == nil {
		return
	}
	content := tab.htmlRenderer.UpdateViewport()
	if content == nil {
		return
	}
	// Only refresh the outer scroll container when the content reference changed
	// (i.e. a new document was loaded). For scroll updates, RenderWithViewport
	// already called contentRoot.Refresh() internally — calling
	// contentScroll.Refresh() on top is a redundant second full repaint.
	if tab.contentScroll.Content != content {
		tab.contentScroll.Content = content
		tab.contentScroll.Refresh()
	}
	if tab.browser != nil {
		tab.browser.updateFPSBar()
	}
}

// postScrollViewport routes a scroll event into the tab's engine event
// loop. The loop keeps a latest-wins scroll slot, so a burst of wheel
// events collapses into one viewport; the drain (scheduled on the next
// UI turn) applies it once. This is the engine event loop's replacement
// for calling the renderer's ScrollCoalescer directly from OnScrolled.
func (t *Tab) postScrollViewport(y, height float32) {
	if t.eventLoop == nil || t.htmlRenderer == nil {
		return
	}
	if err := t.eventLoop.PostInput(eventloop.InputEvent{
		Type:      eventloop.InputScroll,
		Viewport:  eventloop.Viewport{Y: y, Height: height},
		Timestamp: time.Now(),
	}); err != nil {
		// ErrClosed after tab close, ErrInputQueueFull for the bounded
		// ordered queue — both are safe to drop: the next scroll simply
		// replaces the latest state.
		return
	}
	t.scheduleInputDrain()
}

// postKeyInput routes a key press into the tab's engine event loop. Keys
// use the loop's bounded FIFO so their order relative to other discrete
// input (clicks) is preserved when the drain dispatches them.
func (t *Tab) postKeyInput(key string) {
	if t.eventLoop == nil {
		return
	}
	if err := t.eventLoop.PostInput(eventloop.InputEvent{
		Type:      eventloop.InputKey,
		Key:       key,
		Timestamp: time.Now(),
	}); err != nil {
		return
	}
	t.scheduleInputDrain()
}

// hoverThrottle is the minimum interval between drained hover hit-tests.
// MouseMoved posts arrive at display refresh rate and each hit test walks
// the layout tree, so both the poster and the drain gate on this window.
const hoverThrottle = 80 * time.Millisecond

// postCanvasMouseInput is the renderer's mouse-input poster hook (PR9).
// The canvas calls it on the Fyne main thread for every pointer event;
// the tab maps each renderer.MouseInput into the engine loop's matching
// slot — mouse-move into the latest-wins slot (a ~60fps burst collapses
// to one drained position), clicks and link taps into the ordered FIFO —
// and schedules the single per-turn drain. Mouse moves inside the hover
// window are dropped here, before the loop, because handleMouseMove would
// discard them anyway: the pointer stream never pays the post + drain
// cost for positions that cannot produce a hit test.
func (t *Tab) postCanvasMouseInput(input renderer.MouseInput) {
	if t.eventLoop == nil {
		return
	}
	// Pre-throttle the ~60fps mouse-move stream. Clicks and link taps
	// are discrete and never throttled.
	if input.Kind == renderer.MouseInputMove && time.Since(t.lastHoverHit) < hoverThrottle {
		return
	}
	event := eventloop.InputEvent{
		X:         input.X,
		Y:         input.Y,
		Button:    input.Button,
		AbsX:      input.AbsX,
		AbsY:      input.AbsY,
		URL:       input.URL,
		Timestamp: time.Now(),
	}
	switch input.Kind {
	case renderer.MouseInputMove:
		event.Type = eventloop.InputMouseMove
	case renderer.MouseInputClick:
		event.Type = eventloop.InputClick
	case renderer.MouseInputLinkTap:
		event.Type = eventloop.InputClick
	default:
		return
	}
	if err := t.eventLoop.PostInput(event); err != nil {
		return
	}
	t.scheduleInputDrain()
}

// handleInspect marshals an element-inspection update onto the UI
// goroutine and shows it in the dev-tools inspect panel. Shared by the
// direct canvas dispatch and the event-loop drain.
func (t *Tab) handleInspect(node *renderer.RenderNode, layout *renderer.LayoutBox) {
	if t == nil || t.browser == nil {
		return
	}
	t.browser.do(func() {
		if t.browser.devToolsVisible {
			t.browser.inspectPanel.SetRenderer(t.htmlRenderer)
			t.browser.inspectPanel.SetElement(node, layout)
		}
	})
}

// handleContextMenu marshals a right-click context-menu popup onto the
// UI goroutine. Shared by the direct canvas dispatch and the event-loop
// drain.
func (t *Tab) handleContextMenu(node *renderer.RenderNode, layout *renderer.LayoutBox, abs fyne.Position) {
	if t == nil || t.browser == nil {
		return
	}
	t.browser.do(func() {
		if t.browser.devToolsMenu == nil {
			return
		}
		t.browser.showDevToolsMenu(node, layout, abs)
	})
}

// handleMouseMove is the drain's dispatch for the loop's latest mouse
// position. It throttles hover hit-tests (MouseMoved posts fire at
// display refresh rate and each hit test walks the layout tree) and only
// fires the inspect callback when the pointer moves onto a different
// element.
func (t *Tab) handleMouseMove(x, y float32) {
	if t.htmlRenderer == nil {
		return
	}
	if time.Since(t.lastHoverHit) < hoverThrottle {
		return
	}
	t.lastHoverHit = time.Now()

	node, layout := t.htmlRenderer.HitTest(x, y+t.lastViewportY)
	if node != nil && layout != nil {
		if node.ID != t.lastHoverNodeID {
			t.lastHoverNodeID = node.ID
			t.handleInspect(node, layout)
		}
	} else {
		t.lastHoverNodeID = 0
	}
}

// handleClick is the drain's dispatch for a discrete click: link taps
// navigate directly, left clicks hit-test and select for inspection,
// right clicks hit-test and open the dev-tools context menu at the
// cursor.
func (t *Tab) handleClick(ev eventloop.InputEvent) {
	if t.htmlRenderer == nil {
		return
	}
	if ev.URL != "" {
		if t.browser.onNavigate != nil {
			t.browser.onNavigate(ev.URL)
		}
		return
	}
	node, layout := t.htmlRenderer.HitTest(ev.X, ev.Y+t.lastViewportY)
	if ev.Button == 2 {
		t.handleContextMenu(node, layout, fyne.NewPos(ev.AbsX, ev.AbsY))
		return
	}
	if node != nil && layout != nil {
		t.handleInspect(node, layout)
	}
}

// scheduleInputDrain queues at most one drain per UI turn. All callers
// (scroll/key callbacks) and the drain itself run on the Fyne main
// thread, so the flag is a plain bool. Events that arrive while a drain
// is pending only update the loop's coalesced state and are picked up by
// the already-queued drain.
func (t *Tab) scheduleInputDrain() {
	if t.drainScheduled {
		return
	}
	t.drainScheduled = true
	t.browser.do(func() {
		t.drainScheduled = false
		t.drainInputLoop()
	})
}

// drainInputLoop consumes the tab's pending input in the loop's policy
// order — ordered user intent (clicks/keys) first, then latest scroll
// state — and applies it. Scroll input becomes one latest-viewport
// render; key input dispatches to the browser. This is the single place
// input leaves the event loop for presentation.
func (t *Tab) drainInputLoop() {
	if t.eventLoop == nil || t.htmlRenderer == nil {
		return
	}
	events := t.eventLoop.DrainInput()
	if len(events) == 0 {
		return
	}

	scheduledRender := false
	for _, ev := range events {
		switch ev.Type {
		case eventloop.InputScroll:
			// Scroll becomes a render request through the loop's
			// latest-only queue: a newer scroll replaces the queued
			// request and cancels the superseded one, so a burst still
			// produces exactly one render of the final viewport. The
			// request is executed and presented at the end of this same
			// UI turn (executeRenderRequest below) — no deferred second
			// turn — so input-to-present latency stays at one turn.
			t.lastViewportY = ev.Viewport.Y
			if _, err := t.eventLoop.ScheduleRender(context.Background(), eventloop.RenderRequest{
				Generation: t.eventLoop.Generation(),
				Viewport:   ev.Viewport,
				Reason:     eventloop.RenderReasonViewport,
				Created:    ev.Timestamp,
			}); err == nil {
				scheduledRender = true
			}
		case eventloop.InputKey:
			if ev.Key == string(fyne.KeyF12) {
				t.browser.toggleDevTools()
			}
		case eventloop.InputMouseMove:
			// Latest-wins slot: the burst of ~60fps MouseMoved posts
			// drained here is one position per UI turn; dispatch is
			// throttled and delta-checked inside.
			t.handleMouseMove(ev.X, ev.Y)
		case eventloop.InputClick:
			// Discrete clicks and link taps drained in FIFO order relative
			// to keys and other clicks. Dispatch does the hit test with
			// the tab's mirror of the latest scroll offset.
			t.handleClick(ev)
		}
	}

	// Present the final render request. When at least one frame has elapsed
	// since the last present it executes immediately on this UI turn; when
	// a present happened within the frame budget it defers to the frame
	// boundary so sub-frame scroll bursts collapse into one canvas rebuild
	// per display frame (PR12 frame-gated present). The loop's
	// replace-latest queue + generation/cancellation gate (in
	// ProcessPendingResults) apply either way.
	if scheduledRender {
		t.scheduleScrollPresent()
	}
}

// scheduleScrollPresent executes the queued scroll render immediately when
// at least one frame has elapsed since the last present; otherwise it arms
// the frame-boundary timer (at most one pending). The render request stays
// in the loop's replace-latest queue while it waits, so a newer scroll
// arriving before the boundary supersedes the queued viewport and the
// boundary paints the latest position.
func (t *Tab) scheduleScrollPresent() {
	t.renderGateMu.Lock()
	remaining := t.eventLoop.FrameBudget().Duration - time.Since(t.lastPresent)
	if remaining <= 0 {
		t.renderGateMu.Unlock()
		t.executeRenderRequest()
		return
	}
	if t.boundaryTimer != nil {
		t.renderGateMu.Unlock()
		return // already armed; the boundary picks up the latest request
	}
	t.boundaryTimer = time.AfterFunc(remaining, func() {
		t.browser.do(func() {
			t.renderGateMu.Lock()
			t.boundaryTimer = nil
			t.renderGateMu.Unlock()
			if t.eventLoop != nil && t.htmlRenderer != nil {
				t.executeRenderRequest()
			}
		})
	})
	t.renderGateMu.Unlock()
}

// executeRenderRequest consumes the loop's latest render request (the
// queue holds at most one; ScheduleRender replaced and cancelled older
// ones) and submits its completion. The heavy render work is the
// present-side canvas rebuild on the UI thread, and the
// generation/cancellation gate lives in ProcessPendingResults so a stale
// request is never painted.
func (t *Tab) executeRenderRequest() {
	if t.eventLoop == nil || t.htmlRenderer == nil {
		return
	}
	var req eventloop.RenderRequest
	select {
	case req = <-t.eventLoop.RenderRequests():
	default:
		return
	}
	_ = t.eventLoop.SubmitRenderResult(eventloop.RenderResult{
		Request:  req,
		Finished: time.Now(),
	})
	t.eventLoop.ProcessPendingResults()
}

// presentRenderResult is the loop's present callback, invoked only for
// current (non-stale) render results after the generation and
// cancellation checks. It is the single place a completed scroll render
// touches the renderer and canvas, and it runs on the UI thread.
func (t *Tab) presentRenderResult(result eventloop.RenderResult) {
	if result.Err != nil || t.htmlRenderer == nil {
		return
	}
	// Skip the canvas rebuild when the viewport position is identical to the
	// last painted frame (e.g. rubber-band at top/bottom, or coalesced events
	// that share the same final Y). Always record metrics so the HUD stays
	// accurate, but don't pay for a full RenderWithViewport pass.
	// NOTE: we compare against lastPainted*, not lastViewportY — the latter
	// is updated in drainInputLoop for mouse hit-testing and equals newY.
	newY := result.Request.Viewport.Y
	newH := result.Request.Viewport.Height
	if !t.lastPresent.IsZero() &&
		newY == t.lastPaintedViewportY &&
		newH == t.lastPaintedViewportHeight {
		t.htmlRenderer.RecordInputToPresent(time.Since(result.Request.Created))
		// Still tick JS frame scheduler so rAF callbacks fire.
		if t.jsRuntime != nil && t.jsRuntime.FrameScheduler() != nil {
			t.jsRuntime.FrameScheduler().Tick()
		}
		return
	}
	// Report how many scroll events this render collapsed.
	m := t.eventLoop.Metrics()
	if delta := m.CoalescedScrollEvents - t.lastCoalescedScroll; delta > 0 {
		t.htmlRenderer.RecordCoalescedScroll(int(delta))
		t.lastCoalescedScroll = m.CoalescedScrollEvents
	}
	t.htmlRenderer.SetViewport(newY, newH)
	t.htmlRenderer.RecordInputToPresent(time.Since(result.Request.Created))
	refreshTabContent(t)
	if t.jsRuntime != nil && t.jsRuntime.FrameScheduler() != nil {
		t.jsRuntime.FrameScheduler().Tick()
	}
	t.lastPaintedViewportY = newY
	t.lastPaintedViewportHeight = newH
	t.renderGateMu.Lock()
	t.lastPresent = time.Now()
	t.renderGateMu.Unlock()
}

// bumpDocumentGeneration advances the tab's engine generation when a new
// document is rendered, cancelling any render scheduled under the prior
// generation so stale scroll renders are dropped before presentation.
func (t *Tab) bumpDocumentGeneration() {
	if t.eventLoop == nil {
		return
	}
	t.docGeneration++
	t.eventLoop.SetGeneration(eventloop.Generation{
		Navigation: t.docGeneration,
		Document:   t.docGeneration,
		DOM:        t.docGeneration,
		Style:      t.docGeneration,
		Layout:     t.docGeneration,
	})
}

// SetNavigationCallback sets the callback for when navigation is requested
func (b *Browser) SetNavigationCallback(callback NavigationCallback) {
	b.onNavigate = callback
}

// Show displays the browser window
func (b *Browser) Show() {
	// Wrap URL entry in a fixed-height container so the nav bar stays slim.
	const navH float32 = 32
	urlContainer := container.New(&fixedHeightLayout{height: navH}, b.urlEntry)

	// Compact nav buttons — each wrapped to stay small and oval.
	leftBtns := container.NewHBox(
		compactBtn(b.backButton),
		compactBtn(b.forwardButton),
		compactBtn(b.refreshButton),
	)
	rightBtns := container.NewHBox(
		compactBtn(b.bookmarkButton),
		compactBtn(b.bookmarksListButton),
		compactBtn(b.historyButton),
		compactBtn(b.downloadsButton),
		compactBtn(b.screenshotButton),
		compactBtn(b.devToolsButton),
		compactBtn(b.dirtyOverlayButton),
		compactBtn(b.fpsButton),
		compactBtn(b.consoleButton),
		compactBtn(b.inspectButton),
		compactBtn(b.settingsButton),
	)

	// Nav bar: nav buttons on left, action buttons on right, URL entry in centre.
	navBar := container.New(
		&fixedHeightLayout{height: navH},
		container.NewBorder(nil, nil, leftBtns, rightBtns, urlContainer),
	)

	// Dev tools dock container. In headless mode the dock starts with no
	// tabs; -devtools tooling populates them via ShowDevTools.
	b.dockContainer = container.NewMax(b.devToolsDock.CanvasObject())
	b.dockContainer.Hide()

	// Vertical split: page content on top, dev tools dock on bottom
	b.devToolsSplit = container.NewVSplit(b.tabs, b.dockContainer)
	b.devToolsSplit.Offset = 1.0
	b.restoreDevToolsState()

	// Wrap with nav bar, breadcrumb bar, loading bar, and fixed FPS bar
	var topContent []fyne.CanvasObject
	if b.fpsBar != nil {
		topContent = []fyne.CanvasObject{navBar, b.loadingBarContainer, b.fpsBar.CanvasObject()}
	} else {
		topContent = []fyne.CanvasObject{navBar, b.loadingBarContainer}
	}
	mainContent := container.NewBorder(
		container.NewVBox(topContent...),
		b.breadcrumbBox,
		nil, nil,
		b.devToolsSplit,
	)

	// Background rectangle
	bg := canvas.NewRectangle(theme.BackgroundColor())
	b.themeManager.AddListener(func(_ ThemeType) {
		bg.FillColor = theme.BackgroundColor()
		bg.Refresh()
	})

	// Wrap in a resize-detecting widget to trigger re-renders on window resize
	var contentRoot fyne.CanvasObject = mainContent
	if !b.headless {
		contentRoot = newWindowResizeWatcher(mainContent, b.onWindowResize)
	}

	contentWithBg := container.NewMax(bg, contentRoot)
	b.window.SetContent(contentWithBg)

	// Register F12 to toggle dev tools. The key is routed through the
	// active tab's engine event loop (FIFO with clicks) so the toggle
	// runs in the same drained turn as any other pending input — the
	// order in which the user pressed them is preserved.
	b.window.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyF12 {
			if tab := b.ActiveTab(); tab != nil {
				tab.postKeyInput(string(ev.Name))
			}
		}
	})

	// Save dev tools state on close
	b.window.SetOnClosed(func() {
		b.persistDevToolsState()
	})

	b.window.ShowAndRun()
}

// createNavigationControls creates all navigation UI controls
func (b *Browser) createNavigationControls() {
	// URL entry
	b.urlEntry = widget.NewEntry()
	b.urlEntry.SetPlaceHolder("Enter URL (e.g., https://example.com)")
	b.urlEntry.OnSubmitted = func(input string) {
		if b.onNavigate != nil && strings.TrimSpace(input) != "" {
			b.onNavigate(ResolveAddressInput(input, b.settings.GetDefaultSearchEngine()))
		}
	}

	// Back button
	b.backButton = widget.NewButton("←", func() {
		if tab := b.ActiveTab(); tab != nil {
			if url, ok := tab.state.GoBack(); ok {
				if b.onNavigate != nil {
					b.onNavigate(url)
				}
			}
		}
	})
	b.backButton.Disable()

	// Forward button
	b.forwardButton = widget.NewButton("→", func() {
		if tab := b.ActiveTab(); tab != nil {
			if url, ok := tab.state.GoForward(); ok {
				if b.onNavigate != nil {
					b.onNavigate(url)
				}
			}
		}
	})
	b.forwardButton.Disable()

	// Refresh button
	b.refreshButton = widget.NewButton("⟳", func() {
		if tab := b.ActiveTab(); tab != nil {
			currentURL := tab.state.GetCurrentURL()
			if b.onNavigate != nil && currentURL != "" {
				b.onNavigate(currentURL)
			}
		}
	})

	// Bookmark button
	b.bookmarkButton = widget.NewButton("☆", func() {
		b.toggleBookmark()
	})
	b.bookmarkButton.Disable()

	// Bookmarks List button
	b.bookmarksListButton = widget.NewButton("🔖", func() {
		b.showBookmarksDialog()
	})

	// History button
	b.historyButton = widget.NewButton("⏳", func() {
		b.showHistoryDialog()
	})

	// Downloads button
	b.downloadsButton = widget.NewButton("📥", func() {
		b.showDownloadsDialog()
	})

	// Console button
	b.consoleButton = widget.NewButton("⊞", func() {
		b.toggleConsole()
	})

	// Inspect button
	b.inspectButton = widget.NewButton("🔍", func() {
		b.toggleInspect()
	})

	// Settings button
	b.settingsButton = widget.NewButton("⚙", func() {
		b.showSettings()
	})

	// Screenshot button
	b.screenshotButton = widget.NewButton("📷", func() {
		b.takeScreenshot()
	})

	// Dev Tools toggle button — replaces source, memory, queue, DL, JS, tile dialogs
	b.devToolsButton = widget.NewButton("DevTools", func() {
		b.toggleDevTools()
	})

	// Dirty overlay button — toggle for paint region visualization
	b.dirtyOverlayButton = widget.NewButton("Ov", func() {
		b.toggleDirtyOverlay()
	})

	// FPS overlay button — toggle the live on-screen FPS HUD
	b.fpsButton = widget.NewButton("FPS", func() {
		b.toggleFPSOverlay()
	})
}

// AsTabItem converts a Tab to a TabItem
func (t *Tab) AsTabItem() *container.TabItem {
	return container.NewTabItem(t.title, t.content)
}

// GetJSRuntime returns the tab's JavaScript runtime
func (t *Tab) GetJSRuntime() *js.Runtime {
	return t.jsRuntime
}

// SetJSRuntime attaches the tab's JavaScript runtime, wrapping it in a
// single-owner js.Session (M8.1) so the goja VM is only ever accessed by
// the session's owner goroutine. Storage/origin wiring runs on that
// owner; callers then use RunScriptOnOwner/SubmitOnOwner for script
// execution and configuration. Passing nil detaches the runtime and
// closes any active session.
func (t *Tab) SetJSRuntime(runtime *js.Runtime) {
	if runtime == nil {
		t.CloseJSSession()
		t.jsRuntime = nil
		return
	}
	t.jsRuntime = runtime
	t.jsSession = js.NewSessionWithRuntime(runtime, js.DefaultSessionConfig())
	go t.jsSession.Run()

	if t.browser == nil {
		return
	}
	// Wire storage and origin on the owner goroutine so no non-owner
	// thread touches runtime state.
	_ = t.jsSession.SubmitAndWait(func(rt *js.Runtime) {
		if t.browser.deps.Storage != nil {
			rt.SetLocalStorageAdapter(t.browser.deps.Storage)
		}
		if origin, err := navigation.ParseOrigin(t.state.GetCurrentURL()); err == nil && origin.IsValid() {
			rt.SetOrigin(origin.String())
		}
	})
}

// RunScriptOnOwner runs a script on the tab's JS session owner goroutine
// and returns its result. It blocks the caller until the owner finishes;
// it must not be called from the owner goroutine itself. Returns
// js.ErrSessionClosed when no session is active.
func (t *Tab) RunScriptOnOwner(source string) (goja.Value, error) {
	if t.jsSession == nil {
		return nil, js.ErrSessionClosed
	}
	return t.jsSession.Eval(source)
}

// SubmitOnOwner schedules a task on the tab's JS session owner goroutine
// and blocks until it completes. It must not be called from the owner
// goroutine itself. Returns js.ErrSessionClosed when no session is active.
func (t *Tab) SubmitOnOwner(fn func(rt *js.Runtime)) error {
	if t.jsSession == nil {
		return js.ErrSessionClosed
	}
	return t.jsSession.SubmitAndWait(fn)
}

// CloseJSSession shuts down the tab's JS session, cancelling its owner
// goroutine and rejecting queued tasks. Called on navigation (before a
// new runtime is attached) and tab close.
func (t *Tab) CloseJSSession() {
	if t.jsSession != nil {
		t.jsSession.Close()
		t.jsSession = nil
	}
}

// GetRenderer returns the tab's HTML renderer
func (t *Tab) GetRenderer() HTMLRenderer {
	return t.htmlRenderer
}

// SetRawSource stores the raw page HTML source for the View Source feature.
func (t *Tab) SetRawSource(html string) {
	t.rawSource = html
}

// GetRawSource returns the stored raw HTML source, or empty string if none.
func (t *Tab) GetRawSource() string {
	return t.rawSource
}

// toggleBookmark adds or removes the current page from bookmarks
func (b *Browser) toggleBookmark() {
	if tab := b.ActiveTab(); tab != nil {
		currentURL := tab.state.GetCurrentURL()
		if currentURL == "" {
			return
		}

		if b.state.IsBookmarked(currentURL) {
			b.state.RemoveBookmark(currentURL)
			if b.deps.Bookmarks != nil {
				_ = b.deps.Bookmarks.Remove(currentURL)
			}
			b.bookmarkButton.SetText("☆")
		} else {
			b.state.AddBookmark(currentURL)
			if b.deps.Bookmarks != nil {
				_ = b.deps.Bookmarks.Add(currentURL, tab.title)
			}
			b.bookmarkButton.SetText("★")
		}
		b.bookmarkButton.Refresh()
	}
}

// NavigateTo navigates to a URL and updates the UI
func (b *Browser) NavigateTo(url string) {
	if tab := b.ActiveTab(); tab != nil {
		tab.state.AddToHistory(url)
		// Sync origin on the JS owner goroutine (best-effort; the runtime
		// may not have a session yet).
		if tab.jsSession != nil {
			_ = tab.SubmitOnOwner(func(rt *js.Runtime) {
				if origin, err := navigation.ParseOrigin(url); err == nil && origin.IsValid() {
					rt.SetOrigin(origin.String())
				}
			})
		}
		if b.deps.History != nil {
			_ = b.deps.History.AddVisit(url, tab.title)
		}
		b.urlEntry.SetText(url)
		b.updateNavigationButtons()
	}
}

// updateNavigationButtons updates the enabled/disabled state of navigation buttons
func (b *Browser) updateNavigationButtons() {
	tab := b.ActiveTab()
	if tab == nil {
		b.backButton.Disable()
		b.forwardButton.Disable()
		b.bookmarkButton.Disable()
		return
	}

	if tab.state.CanGoBack() {
		b.backButton.Enable()
	} else {
		b.backButton.Disable()
	}

	if tab.state.CanGoForward() {
		b.forwardButton.Enable()
	} else {
		b.forwardButton.Disable()
	}

	currentURL := tab.state.GetCurrentURL()
	if currentURL != "" {
		b.bookmarkButton.Enable()
		if b.state.IsBookmarked(currentURL) {
			b.bookmarkButton.SetText("★")
		} else {
			b.bookmarkButton.SetText("☆")
		}
		b.bookmarkButton.Refresh()
	} else {
		b.bookmarkButton.Disable()
	}
}

// GetBookmarks returns the list of bookmarks
func (b *Browser) GetBookmarks() []string {
	return b.state.GetBookmarks()
}

// GetHistory returns the navigation history
func (b *Browser) GetHistory() []string {
	if tab := b.ActiveTab(); tab != nil {
		return tab.state.GetHistory()
	}
	return []string{}
}

// do marshals work onto the Fyne UI thread. In headless mode there is no
// UI event loop, so the function is called directly instead.
func (b *Browser) do(fn func()) {
	if b.headless {
		fn()
		return
	}
	fyne.Do(fn)
}

// doAndWait runs fn on the Fyne UI thread, blocking the caller until fn
// returns. Use this when the caller is on a non-Fyne goroutine (the JS
// runtime, the documentloader mutation-coalescer timer, the image
// loader, etc.) and needs to wait for the Fyne-side work to finish
// before continuing — typically because the return value of the work
// is needed by the caller.
//
// In headless mode the function runs directly. From the Fyne main
// goroutine the function also runs directly so that fyne.DoAndWait —
// which would re-queue onto the very thread that's executing us — does
// not deadlock.
func (b *Browser) doAndWait(fn func()) {
	if b.headless {
		fn()
		return
	}
	if IsMainGoroutine() {
		fn()
		return
	}
	fyne.DoAndWait(fn)
}

// ShowLoading displays the loading indicator
func (b *Browser) ShowLoading() {
	b.do(func() {
		b.loadingBarContainer.Show()
		b.loadingBar.Show()
	})
}

// HideLoading hides the loading indicator
func (b *Browser) HideLoading() {
	b.do(func() {
		b.loadingBar.Hide()
		b.loadingBarContainer.Hide()
	})
}

// UpdateActiveTabTitle updates the title of the active tab
func (b *Browser) UpdateActiveTabTitle(title string) {
	b.do(func() {
		if tab := b.ActiveTab(); tab != nil {
			tab.title = title
			if selected := b.tabs.Selected(); selected != nil {
				selected.Text = title
				b.tabs.Refresh()
			}
		}
	})
}

// showMemoryDialog opens the dev tools dock to the Memory tab.
func (b *Browser) showMemoryDialog() {
	if !b.devToolsVisible {
		b.toggleDevTools()
	} else {
		b.devToolsDock.SelectTab("Memory")
		b.devToolsDock.Refresh()
	}
}

// showNetworkQueueDialog opens the dev tools dock to the Network tab.
func (b *Browser) showNetworkQueueDialog() {
	if !b.devToolsVisible {
		b.toggleDevTools()
	} else {
		b.devToolsDock.SelectTab("Network")
		b.devToolsDock.Refresh()
	}
}

// showScriptTaskQueueDialog opens the dev tools dock to the Script Queue tab.
func (b *Browser) showScriptTaskQueueDialog() {
	if !b.devToolsVisible {
		b.toggleDevTools()
	} else {
		b.devToolsDock.SelectTab("Script Queue")
		b.devToolsDock.Refresh()
	}
}

// showTileCacheDialog opens the dev tools dock to the Tile Cache tab.
func (b *Browser) showTileCacheDialog() {
	if !b.devToolsVisible {
		b.toggleDevTools()
	} else {
		b.devToolsDock.SelectTab("Tile Cache")
		b.devToolsDock.Refresh()
	}
}

// showDisplayListDialog opens the dev tools dock to the Display List tab.
func (b *Browser) showDisplayListDialog() {
	if !b.devToolsVisible {
		b.toggleDevTools()
	} else {
		b.devToolsDock.SelectTab("Display List")
		b.devToolsDock.Refresh()
	}
}

// showSourceDialog opens the dev tools dock to the Source tab.
func (b *Browser) showSourceDialog() {
	if !b.devToolsVisible {
		b.toggleDevTools()
	} else {
		b.devToolsDock.SelectTab("Sources")
		b.devToolsDock.Refresh()
	}
}

// GetApp returns the Fyne application instance for thread-safe operations
func (b *Browser) GetApp() fyne.App {
	return b.app
}

// GetWindow returns the browser window
func (b *Browser) GetWindow() fyne.Window {
	return b.window
}

// GetSettings returns the browser settings
func (b *Browser) GetSettings() *Settings {
	return b.settings
}

// showSettings displays the settings dialog
func (b *Browser) showSettings() {
	// Create form entries for settings
	homepageEntry := widget.NewEntry()
	homepageEntry.SetText(b.settings.GetHomepage())
	homepageEntry.SetPlaceHolder("https://example.com")

	searchEngineEntry := widget.NewEntry()
	searchEngineEntry.SetText(b.settings.GetDefaultSearchEngine())
	searchEngineEntry.SetPlaceHolder("https://www.google.com/search?q=")

	jsCheck := widget.NewCheck("Enable JavaScript", func(checked bool) {
		b.settings.SetEnableJavaScript(checked)
	})
	jsCheck.SetChecked(b.settings.GetEnableJavaScript())

	imagesCheck := widget.NewCheck("Enable Images", func(checked bool) {
		b.settings.SetEnableImages(checked)
	})
	imagesCheck.SetChecked(b.settings.GetEnableImages())

	// Theme selection
	themeSelect := widget.NewSelect([]string{"System", "Light", "Dark"}, func(selected string) {
		switch selected {
		case "Light":
			b.themeManager.SetTheme(ThemeLight)
		case "Dark":
			b.themeManager.SetTheme(ThemeDark)
		default:
			b.themeManager.SetTheme(ThemeSystem)
		}
	})
	themeSelect.SetSelected(b.themeManager.Current().String())

	// Create form
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Homepage", Widget: homepageEntry},
			{Text: "Search Engine", Widget: searchEngineEntry},
			{Text: "Theme", Widget: themeSelect},
			{Text: "", Widget: jsCheck},
			{Text: "", Widget: imagesCheck},
		},
		OnSubmit: func() {
			// Save settings
			b.settings.SetHomepage(homepageEntry.Text)
			b.settings.SetDefaultSearchEngine(searchEngineEntry.Text)
			if b.deps.SettingsStore != nil {
				_ = b.deps.SettingsStore.Set(b.settings.ToProfileSettings())
			}
		},
		OnCancel: func() {
			// Do nothing, just close
		},
	}

	// Create custom dialog
	d := dialog.NewCustom("Settings", "Close", form, b.window)
	d.Resize(fyne.NewSize(500, 300))
	d.Show()
}

// updateConsoleFromActiveTab updates the console panel with messages from the active tab
func (b *Browser) updateConsoleFromActiveTab() {
	tab := b.ActiveTab()
	if tab == nil || tab.jsRuntime == nil {
		b.consolePanel.Clear()
		return
	}

	// Get console messages from the tab's runtime
	messages := tab.jsRuntime.GetConsoleMessages()
	b.consolePanel.SetMessages(messages)
}

// GetConsolePanel returns the console panel
func (b *Browser) GetConsolePanel() *ConsolePanel {
	return b.consolePanel
}

// takeScreenshot captures the browser window and saves it as a PNG
func (b *Browser) takeScreenshot() {
	options := DefaultScreenshotOptions()

	// Try to get the user's home directory for a sensible default
	homeDir, err := os.UserHomeDir()
	if err == nil {
		options.Directory = filepath.Join(homeDir, "Pictures")
		// Create directory if it doesn't exist
		os.MkdirAll(options.Directory, 0755)
	}

	filepath, err := TakeScreenshot(b.window, options)
	if err != nil {
		dialog.ShowError(fmt.Errorf("Failed to take screenshot: %w", err), b.window)
		return
	}

	dialog.ShowInformation("Screenshot Saved", fmt.Sprintf("Screenshot saved to:\n%s", filepath), b.window)
}

// requestLogAdapter wraps *net.RequestLog as devtools.requestLogProvider.
type requestLogAdapter struct {
	log *goosienet.RequestLog
}

func (a *requestLogAdapter) Entries() []devtools.NetRequestEntry {
	raw := a.log.Entries()
	out := make([]devtools.NetRequestEntry, len(raw))
	for i, e := range raw {
		n := devtools.NetRequestEntry{
			Method:      e.Method,
			URL:         e.URL,
			Status:      e.Status,
			ContentType: e.ContentType,
			Bytes:       e.Bytes,
			CacheHit:    e.CacheHit,
			Error:       e.Error,
			StartedAt:   e.StartedAt,
			Duration:    e.Duration,
		}
		if len(e.TimingPhases) > 0 {
			n.TimingPhases = make([]devtools.TimingPhase, len(e.TimingPhases))
			for j, tp := range e.TimingPhases {
				n.TimingPhases[j] = devtools.TimingPhase{
					Name:     string(tp.Name),
					Duration: tp.Duration,
				}
			}
		}
		out[i] = n
	}
	return out
}

// metricsAdapter wraps data accessible from the browser as devtools.metricsProvider.
type metricsAdapter struct {
	log *goosienet.RequestLog
}

func (m *metricsAdapter) Snapshot() metrics.Metrics {
	// Build a metrics.Metrics from the request log entries for
	// display in the performance panel. Full engine Recorder
	// integration is a future enhancement.
	entries := m.log.Entries()
	var totalBytes int64
	var cacheHits, cacheMisses int
	for _, e := range entries {
		totalBytes += e.Bytes
		if e.CacheHit {
			cacheHits++
		} else {
			cacheMisses++
		}
	}
	return metrics.Metrics{
		URL: "live",
		Counters: metrics.Counters{
			BytesDownloaded: totalBytes,
			CacheHits:       cacheHits,
			CacheMisses:     cacheMisses,
		},
	}
}

func (b *Browser) showBookmarksDialog() {
	if b.deps.Bookmarks == nil {
		dialog.ShowInformation("Bookmarks", "Bookmarks store is not available.", b.window)
		return
	}
	bookmarks := b.deps.Bookmarks.List()
	if len(bookmarks) == 0 {
		dialog.ShowInformation("Bookmarks", "No bookmarks saved yet.", b.window)
		return
	}

	var list *widget.List
	list = widget.NewList(
		func() int { return len(bookmarks) },
		func() fyne.CanvasObject {
			titleLabel := widget.NewLabel("")
			deleteBtn := widget.NewButton("Delete", nil)
			return container.NewBorder(nil, nil, nil, deleteBtn, titleLabel)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(bookmarks) {
				return
			}
			bm := bookmarks[id]
			border := obj.(*fyne.Container)
			label := border.Objects[0].(*widget.Label)
			label.SetText(fmt.Sprintf("%s (%s)", bm.Title, bm.URL))

			deleteBtn := border.Objects[1].(*widget.Button)
			deleteBtn.OnTapped = func() {
				_ = b.deps.Bookmarks.Remove(bm.URL)
				b.state.RemoveBookmark(bm.URL)
				b.updateNavigationButtons()
				bookmarks = b.deps.Bookmarks.List()
				list.Refresh()
			}
		},
	)

	list.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(bookmarks) {
			return
		}
		bm := bookmarks[id]
		if b.onNavigate != nil {
			b.onNavigate(bm.URL)
		}
	}

	d := dialog.NewCustom("Bookmarks", "Close", container.NewMax(list), b.window)
	d.Resize(fyne.NewSize(500, 400))
	d.Show()
}

func (b *Browser) showHistoryDialog() {
	if b.deps.History == nil {
		dialog.ShowInformation("History", "History store is not available.", b.window)
		return
	}
	visits := b.deps.History.Visits()
	if len(visits) == 0 {
		dialog.ShowInformation("History", "No history visits recorded yet.", b.window)
		return
	}

	var list *widget.List
	list = widget.NewList(
		func() int { return len(visits) },
		func() fyne.CanvasObject {
			titleLabel := widget.NewLabel("")
			return container.NewMax(titleLabel)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(visits) {
				return
			}
			visit := visits[id]
			label := obj.(*fyne.Container).Objects[0].(*widget.Label)
			label.SetText(fmt.Sprintf("[%s] %s (%s)", visit.VisitedAt.Format("15:04:05"), visit.Title, visit.URL))
		},
	)

	list.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(visits) {
			return
		}
		visit := visits[id]
		if b.onNavigate != nil {
			b.onNavigate(visit.URL)
		}
	}

	clearBtn := widget.NewButton("Clear History", func() {
		_ = b.deps.History.Clear()
		visits = b.deps.History.Visits()
		list.Refresh()
	})

	content := container.NewBorder(nil, clearBtn, nil, nil, list)
	d := dialog.NewCustom("History", "Close", content, b.window)
	d.Resize(fyne.NewSize(500, 400))
	d.Show()
}

func (b *Browser) showDownloadsDialog() {
	if b.deps.Network == nil {
		dialog.ShowInformation("Downloads", "Network service is not available.", b.window)
		return
	}
	panel := NewDownloadsPanel()
	panel.SetRecords(b.deps.Network.Downloads())

	d := dialog.NewCustom("Downloads", "Close", panel.CanvasObject(), b.window)
	d.Resize(fyne.NewSize(500, 400))
	d.Show()
}
