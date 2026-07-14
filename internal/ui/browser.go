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
	"github.com/vyquocvu/goosie/internal/js"
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
	Session       *profile.SessionStore
	SettingsStore *profile.SettingsStore
	Storage       *profile.StorageStore
	Network       *goosienet.Service
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
		container.NewHBox(b.bookmarkButton, b.screenshotButton, b.sourceButton, b.consoleButton, b.inspectButton, b.settingsButton),
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
