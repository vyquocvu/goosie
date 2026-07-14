package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	"github.com/vyquocvu/goosie/internal/engine/session"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/memory"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/profile"
	"github.com/vyquocvu/goosie/internal/renderer"
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
	settingsButton      *widget.Button
	consoleButton       *widget.Button
	loadingBar          *widget.ProgressBarInfinite
	loadingBarContainer *fyne.Container
	onNavigate          NavigationCallback
	tabs                *container.DocTabs
	tabItems            []*Tab
	consolePanel        *ConsolePanel
	consoleSplit        *container.Split
	consoleVisible      bool
	consoleContainer    *fyne.Container
	inspectPanel        *InspectPanel
	inspectVisible      bool
	inspectContainer    *fyne.Container
	inspectButton       *widget.Button
	screenshotButton    *widget.Button
	sourceButton        *widget.Button
	memoryButton        *widget.Button
	netQueueButton      *widget.Button
	displayListButton   *widget.Button
	dirtyOverlayButton  *widget.Button
	jsQueueButton       *widget.Button
	RendererFactory     func() HTMLRenderer
	deps                BrowserDependencies
	shortcuts           *ShortcutRegistry
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

func newBrowserInternal(a fyne.App, w fyne.Window) *Browser {
	state := NewBrowserState()
	settings := NewSettings()
	themeManager := NewThemeManager(a)

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
		consoleVisible:      false,
		inspectVisible:      false,
	}

	// Create console panel
	browser.consolePanel = NewConsolePanel(browser.toggleConsole)
	browser.consolePanel.SetRefreshCallback(func() {
		// Clear console messages in the active tab's runtime
		if tab := browser.ActiveTab(); tab != nil && tab.jsRuntime != nil {
			tab.jsRuntime.ClearConsoleMessages()
			tab.jsRuntime.ClearJavaScriptErrors()
		}
	})
	browser.consolePanel.SetExecuteCallback(func(source string) {
		if tab := browser.ActiveTab(); tab != nil && tab.jsRuntime != nil {
			value, err := tab.jsRuntime.RunScript(source)
			if err != nil {
				browser.consolePanel.AddMessage(js.ConsoleMessage{Level: "error", Message: err.Error(), Timestamp: time.Now(), Data: err.Error()})
				return
			}
			browser.consolePanel.AddMessage(js.ConsoleMessage{Level: "log", Message: value.String(), Timestamp: time.Now(), Data: value.String()})
		}
	})

	// Create inspect panel
	browser.inspectPanel = NewInspectPanel(browser.toggleInspect)

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

	browser := newBrowserInternal(a, w)
	browser.deps = deps
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
}

// toggleConsole toggles the visibility of the console panel
func (b *Browser) toggleConsole() {
	if b.consoleVisible {
		b.consoleContainer.Hide()
	} else {
		b.consoleContainer.Show()
	}
	b.consoleVisible = !b.consoleVisible
	b.window.Content().Refresh()
}

// toggleInspect toggles the visibility of the inspect panel
func (b *Browser) toggleInspect() {
	if b.inspectVisible {
		b.inspectContainer.Hide()
	} else {
		b.inspectContainer.Show()
		// Initialize with current renderer if available
		if tab := b.ActiveTab(); tab != nil && tab.htmlRenderer != nil {
			b.inspectPanel.SetRenderer(tab.htmlRenderer)
		}
	}
	b.inspectVisible = !b.inspectVisible
	b.window.Content().Refresh()
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

	return &Tab{
		title:         "New Tab",
		content:       contentScroll,
		contentBox:    contentBox,
		contentScroll: contentScroll,
		htmlRenderer:  htmlRenderer,
		state:         tabState,
		browser:       b,
	}
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

// RenderHTML renders HTML content using the canvas-based renderer for this specific tab
func (t *Tab) RenderHTML(ctx context.Context, htmlContent string) error {
	// Lazily initialize the renderer if needed
	if t.htmlRenderer == nil {
		if t.browser.RendererFactory == nil {
			return fmt.Errorf("RendererFactory is not set")
		}
		t.htmlRenderer = t.browser.RendererFactory()
		if t.htmlRenderer == nil {
			return fmt.Errorf("RendererFactory returned nil renderer")
		}
		t.htmlRenderer.SetWindow(t.browser.window)
		t.htmlRenderer.SetNavigationCallback(func(url string) {
			if t.browser.onNavigate != nil {
				t.browser.onNavigate(url)
			}
		})
	}
	// Set the current URL for resolving relative links
	currentURL := t.state.GetCurrentURL()
	t.htmlRenderer.SetCurrentURL(currentURL)

	// Set up inspect callback
	t.htmlRenderer.SetInspectCallback(func(node *renderer.RenderNode, layout *renderer.LayoutBox) {
		fyne.Do(func() {
			if t.browser.inspectVisible {
				t.browser.inspectPanel.SetRenderer(t.htmlRenderer)
				t.browser.inspectPanel.SetElement(node, layout)
			}
		})
	})

	// Set up refresh callback for the renderer
	t.htmlRenderer.SetRefreshCallback(func() {
		fyne.Do(func() {
			// Trigger a refresh of the scroll container to show changes
			t.contentScroll.Refresh()
			// Also refresh inspector if visible
			if t.browser.inspectVisible {
				t.browser.inspectPanel.SetRenderer(t.htmlRenderer)
			}
		})
	})

	canvasObject, err := t.htmlRenderer.RenderHTML(ctx, htmlContent)
	if err != nil {
		return err
	}

	// Update the scroll container with the rendered content on the main thread
	fyne.Do(func() {
		t.contentScroll.Content = canvasObject
		t.contentScroll.Refresh()
	})

	return nil
}

func refreshTabContent(tab *Tab) {
	if tab == nil || tab.htmlRenderer == nil || tab.contentScroll == nil {
		return
	}
	content := tab.htmlRenderer.UpdateViewport()
	if content == nil {
		return
	}
	tab.contentScroll.Content = content
	tab.contentScroll.Refresh()
}

// SetNavigationCallback sets the callback for when navigation is requested
func (b *Browser) SetNavigationCallback(callback NavigationCallback) {
	b.onNavigate = callback
}

// Show displays the browser window
func (b *Browser) Show() {
	// Create navigation bar
	navBar := container.NewBorder(nil, nil,
		container.NewHBox(b.backButton, b.forwardButton, b.refreshButton),
		container.NewHBox(b.bookmarkButton, b.screenshotButton, b.sourceButton, b.memoryButton, b.netQueueButton, b.displayListButton, b.dirtyOverlayButton, b.jsQueueButton, b.consoleButton, b.inspectButton, b.settingsButton),
		b.urlEntry,
	)

	// Create main split container (tabs on top, console on bottom when visible)
	b.consoleSplit = container.NewVSplit(
		b.tabs,
		b.consolePanel.GetContainer(),
	)
	b.consoleSplit.Offset = 1.0 // Start with console hidden (all space to tabs)

	// Create main layout with 5px height loading bar
	container.NewBorder(
		container.NewVBox(navBar, b.loadingBarContainer),
		nil, nil, nil,
		b.consoleSplit,
	)

	// Main view with a split for the console
	mainView := container.NewVSplit(b.tabs, b.consolePanel.CanvasObject())
	mainView.Offset = 1.0 // Initially hide console

	// Create a container for the console and hide it initially
	b.consoleContainer = container.NewMax(b.consolePanel.CanvasObject())
	b.consoleContainer.Hide()

	// Create a container for the inspect panel and hide it initially
	b.inspectContainer = container.NewMax(b.inspectPanel.CanvasObject())
	b.inspectContainer.Hide()

	// Create a horizontal split for inspect panel (on the right)
	mainWithInspect := container.NewHSplit(b.tabs, b.inspectContainer)
	mainWithInspect.Offset = 1.0 // Initially hide inspect panel (all space to tabs)

	// Combine the main content with console and inspect panels
	contentWithConsole := container.NewBorder(navBar, nil, nil, nil, container.NewVSplit(mainWithInspect, b.consoleContainer))

	// Create background rectangle
	bg := canvas.NewRectangle(theme.BackgroundColor())

	// Listen for theme changes to update background color
	// Note: We need to use a goroutine or delay because theme changes might take a moment to propagate
	// through Fyne's internal state
	b.themeManager.AddListener(func(_ ThemeType) {
		bg.FillColor = theme.BackgroundColor()
		bg.Refresh()
	})

	// Wrap content with background
	contentWithBg := container.NewMax(bg, contentWithConsole)

	b.window.SetContent(contentWithBg)
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

	// Source button
	b.sourceButton = widget.NewButton("Source", func() {
		b.showSourceDialog()
	})

	// Memory budget button
	b.memoryButton = widget.NewButton("Mem", func() {
		b.showMemoryDialog()
	})

	// Network queue button
	b.netQueueButton = widget.NewButton("Queue", func() {
		b.showNetworkQueueDialog()
	})

	// Display list button
	b.displayListButton = widget.NewButton("DL", func() {
		b.showDisplayListDialog()
	})

	// Dirty overlay button — toggle for paint region visualization
	b.dirtyOverlayButton = widget.NewButton("Ov", func() {
		b.toggleDirtyOverlay()
	})

	// Script task queue button
	b.jsQueueButton = widget.NewButton("JS", func() {
		b.showScriptTaskQueueDialog()
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

// SetJSRuntime sets the tab's JavaScript runtime
func (t *Tab) SetJSRuntime(runtime *js.Runtime) {
	t.jsRuntime = runtime
	if runtime == nil || t.browser == nil {
		return
	}
	if t.browser.deps.Storage != nil {
		runtime.SetLocalStorageAdapter(t.browser.deps.Storage)
	}
	if origin, err := navigation.ParseOrigin(t.state.GetCurrentURL()); err == nil && origin.IsValid() {
		runtime.SetOrigin(origin.String())
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
		if tab.jsRuntime != nil {
			if origin, err := navigation.ParseOrigin(url); err == nil && origin.IsValid() {
				tab.jsRuntime.SetOrigin(origin.String())
			}
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

// ShowLoading displays the loading indicator
func (b *Browser) ShowLoading() {
	// Use fyne.Do to ensure UI updates happen on the main thread
	fyne.Do(func() {
		b.loadingBarContainer.Show()
		b.loadingBar.Show()
	})
}

// HideLoading hides the loading indicator
func (b *Browser) HideLoading() {
	// Use fyne.Do to ensure UI updates happen on the main thread
	fyne.Do(func() {
		b.loadingBar.Hide()
		b.loadingBarContainer.Hide()
	})
}

// UpdateActiveTabTitle updates the title of the active tab
func (b *Browser) UpdateActiveTabTitle(title string) {
	fyne.Do(func() {
		if tab := b.ActiveTab(); tab != nil {
			tab.title = title
			if selected := b.tabs.Selected(); selected != nil {
				selected.Text = title
				b.tabs.Refresh()
			}
		}
	})
}

// showMemoryDialog opens a dialog showing per-component memory budgets and usage.
func (b *Browser) showMemoryDialog() {
	mgr := b.deps.Memory
	if mgr == nil {
		dialog.ShowInformation("Memory Budget", "Memory manager not available.", b.window)
		return
	}

	stats := mgr.Stats()

	// Add display list memory estimate from active tab
	dlEstimate := int64(0)
	if tab := b.ActiveTab(); tab != nil && tab.htmlRenderer != nil {
		dlEstimate = int64(len(tab.htmlRenderer.GetDisplayListCommands())) * 256
		if dlEstimate > 0 {
			stats.Usage[memory.ComponentDisplayList] = uint64(dlEstimate)
		}
	}

	label := widget.NewLabel(formatMemoryStats(stats))
	label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		b.showMemoryDialog()
	})

	scroll := container.NewScroll(label)
	scroll.SetMinSize(fyne.NewSize(600, 400))

	content := container.NewBorder(nil, refreshBtn, nil, nil, scroll)
	dialog.ShowCustom("Memory Budget", "Close", content, b.window)
}

// formatMemoryStats formats a memory.Stats snapshot into a readable string.
func formatMemoryStats(stats memory.Stats) string {
	var b strings.Builder

	b.WriteString("Global Budget\n")
	b.WriteString(fmt.Sprintf("  Total Usage: %s / %s\n\n",
		formatBytes(int64(stats.TotalUsage)),
		formatBytes(int64(stats.GlobalLimit))))

	b.WriteString("Per-Component Budgets\n")
	for _, comp := range memoryDefaultOrder() {
		usage, hasUsage := stats.Usage[comp]
		limit, hasLimit := stats.Limits[comp]
		if !hasUsage && !hasLimit {
			continue
		}
		usageStr := "0 B"
		if hasUsage {
			usageStr = formatBytes(int64(usage))
		}
		limitStr := "unlimited"
		if hasLimit && limit > 0 {
			limitStr = formatBytes(int64(limit))
		}
		b.WriteString(fmt.Sprintf("  %-20s  %s / %s\n", string(comp), usageStr, limitStr))
	}

	return b.String()
}

// showNetworkQueueDialog opens a dialog showing network activity including
// a waterfall view of completed requests and pending loads with priority.
func (b *Browser) showNetworkQueueDialog() {
	var buf strings.Builder

	// Completed requests with waterfall
	if b.deps.Network != nil {
		entries := b.deps.Network.Log().Entries()
		if len(entries) > 0 {
			buf.WriteString(fmt.Sprintf("Completed Requests: %d\n\n", len(entries)))

			maxDur := time.Duration(0)
			for _, e := range entries {
				if e.Duration > maxDur {
					maxDur = e.Duration
				}
			}
			maxBar := 40.0 // max chars for waterfall bar

			for _, e := range entries {
				barLen := 0
				if maxDur > 0 && e.Duration > 0 {
					barLen = int(float64(e.Duration) / float64(maxDur) * maxBar)
					if barLen < 1 {
						barLen = 1
					}
				}
				bar := strings.Repeat("█", barLen)
				status := fmt.Sprintf("%d", e.Status)
				if e.Status == 0 {
					status = "ERR"
				}
				cache := ""
				if e.CacheHit {
					cache = " [cache]"
				}
				url := e.URL
				if len(url) > 60 {
					url = url[:57] + "..."
				}
				buf.WriteString(fmt.Sprintf("%s %-4s %s%s\n", status, e.Method, url, cache))
				buf.WriteString(fmt.Sprintf("  %s %.0fms\n", bar, e.Duration.Seconds()*1000))
			}
			buf.WriteString("\n")
		}
	}

	// Pending loads
	sess := b.deps.NavSession
	if sess != nil {
		loads := sess.PendingLoads()
		if len(loads) > 0 {
			buf.WriteString(fmt.Sprintf("Pending Loads: %d\n", len(loads)))
			for i, load := range loads {
				age := time.Since(load.StartedAt).Round(time.Millisecond)
				url := load.URL
				if len(url) > 60 {
					url = url[:57] + "..."
				}
				buf.WriteString(fmt.Sprintf("  %d. %s\n", i+1, url))
				buf.WriteString(fmt.Sprintf("     Priority: %s  Age: %v\n", load.Priority.String(), age))
			}
		}
	}

	if buf.Len() == 0 {
		dialog.ShowInformation("Network View", "No network activity yet.", b.window)
		return
	}

	label := widget.NewLabel(buf.String())
	label.Wrapping = fyne.TextWrapWord

	cancelBtn := widget.NewButton("Cancel Pending", func() {
		if sess != nil {
			sess.Cancel()
		}
	})

	content := container.NewBorder(nil, container.NewHBox(cancelBtn), nil, nil, container.NewScroll(label))

	dialog.ShowCustom("Network View", "Close", content, b.window)
}

// showScriptTaskQueueDialog opens a dialog showing the JavaScript task queue state.
func (b *Browser) showScriptTaskQueueDialog() {
	tab := b.ActiveTab()
	if tab == nil {
		dialog.ShowInformation("Script Task Queue", "No active tab.", b.window)
		return
	}

	rt := tab.GetJSRuntime()

	var buf strings.Builder
	buf.WriteString("JavaScript Task Queue\n\n")

	if rt == nil {
		buf.WriteString("No JavaScript runtime available.\n")
	} else {
		timers := rt.ActiveTimersCount()
		consoleCount := len(rt.GetConsoleMessages())
		errorCount := len(rt.GetJavaScriptErrors())

		buf.WriteString(fmt.Sprintf("Active Timers (setTimeout/setInterval): %d\n", timers))
		buf.WriteString(fmt.Sprintf("Console Messages:                      %d\n", consoleCount))
		buf.WriteString(fmt.Sprintf("JavaScript Errors:                     %d\n", errorCount))
		buf.WriteString(fmt.Sprintf("Script Running:                        %s\n", map[bool]string{true: "Yes", false: "No"}[rt.RunningScriptCount() > 0]))
	}

	label := widget.NewLabel(buf.String())
	label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		b.showScriptTaskQueueDialog()
	})

	content := container.NewBorder(nil, refreshBtn, nil, nil, container.NewScroll(label))
	dialog.ShowCustom("Script Task Queue", "Close", content, b.window)
}

// showDisplayListDialog opens a dialog showing the display list command details.
func (b *Browser) showDisplayListDialog() {
	tab := b.ActiveTab()
	if tab == nil || tab.htmlRenderer == nil {
		dialog.ShowInformation("Display List", "No renderer available.", b.window)
		return
	}

	cmds := tab.htmlRenderer.GetDisplayListCommands()
	if len(cmds) == 0 {
		dialog.ShowInformation("Display List", "No display list built yet.", b.window)
		return
	}

	summary := tab.htmlRenderer.GetDisplayListSummary()
	total := 0
	for _, c := range summary {
		total += c
	}

	summaryStr := fmt.Sprintf("Total: %d commands\n\n", total)
	for _, name := range displayListTypeOrder() {
		count, ok := summary[name]
		if !ok {
			continue
		}
		summaryStr += fmt.Sprintf("  %-12s %d\n", name+":", count)
	}

	summaryLabel := widget.NewLabel(summaryStr)
	summaryLabel.Wrapping = fyne.TextWrapWord

	var detailBuf strings.Builder
	for i, cmd := range cmds {
		line := fmt.Sprintf("%d. %s", i+1, cmd.Type.String())
		switch cmd.Type {
		case renderer.PaintText:
			txt := cmd.Text
			if len(txt) > 40 {
				txt = txt[:37] + "..."
			}
			line += fmt.Sprintf("  text=%q  font=%.0f  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
				txt, cmd.FontSize, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
		case renderer.PaintRect:
			line += fmt.Sprintf("  pos=(%.0f,%.0f)  size=(%.0f×%.0f)", cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
		case renderer.PaintImage:
			src := cmd.ImageSrc
			if len(src) > 40 {
				src = src[:37] + "..."
			}
			line += fmt.Sprintf("  src=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)", src, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
		case renderer.PaintLink:
			line += fmt.Sprintf("  url=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)", cmd.LinkURL, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
		case renderer.PaintBorder:
			line += fmt.Sprintf("  pos=(%.0f,%.0f)  size=(%.0f×%.0f)  stroke=%.0f",
				cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height, cmd.StrokeWidth)
		case renderer.PaintButton:
			line += fmt.Sprintf("  text=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
				cmd.ButtonText, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
		case renderer.PaintInput:
			line += fmt.Sprintf("  type=%s  value=%s  placeholder=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
				cmd.InputType, cmd.InputValue, cmd.Placeholder, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
		case renderer.PaintTextarea:
			line += fmt.Sprintf("  value=%s  placeholder=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
				cmd.InputValue, cmd.Placeholder, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
		case renderer.PushClip:
			line += fmt.Sprintf("  pos=(%.0f,%.0f)  size=(%.0f×%.0f)  overflow=%s",
				cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height, cmd.ClipOverflow)
		case renderer.PopClip:
		}
		detailBuf.WriteString(line + "\n")
	}

	detailLabel := widget.NewLabel(detailBuf.String())
	detailLabel.Wrapping = fyne.TextWrapWord

	content := container.NewBorder(summaryLabel, nil, nil, nil, container.NewScroll(detailLabel))

	dialog.ShowCustom("Display List Inspector", "Close", content, b.window)
}

// displayListTypeOrder returns command type names in a consistent display order.
func displayListTypeOrder() []string {
	return []string{"Text", "Rect", "Image", "Link", "Border", "Button", "Input", "Textarea", "PushClip", "PopClip"}
}

// memoryDefaultOrder returns a consistent display order for memory components.
func memoryDefaultOrder() []memory.Component {
	return []memory.Component{
		memory.ComponentDOM,
		memory.ComponentStyle,
		memory.ComponentLayout,
		memory.ComponentDisplayList,
		memory.ComponentTile,
		memory.ComponentImage,
		memory.ComponentGlyph,
		memory.ComponentScript,
		memory.ComponentNetworkCache,
		memory.ComponentPageCache,
		memory.ComponentLayoutIntrinsicSize,
	}
}

// showSourceDialog opens a dialog showing the raw HTML source of the current page.
// Uses a read-only MultiLineEntry within a scroll container for navigation and copy.
func (b *Browser) showSourceDialog() {
	tab := b.ActiveTab()
	if tab == nil {
		return
	}
	source := tab.GetRawSource()
	if source == "" {
		dialog.ShowInformation("Page Source", "No source available — the page may not have loaded yet.", b.window)
		return
	}

	sourceEntry := widget.NewMultiLineEntry()
	sourceEntry.SetText(source)
	sourceEntry.Wrapping = fyne.TextWrapOff
	sourceEntry.Disable()

	scroll := container.NewScroll(sourceEntry)
	scroll.SetMinSize(fyne.NewSize(700, 500))

	dialog.ShowCustom("Page Source ("+tab.title+")", "Close", scroll, b.window)
}

// GetApp returns the Fyne application instance for thread-safe operations
func (b *Browser) GetApp() fyne.App {
	return b.app
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
