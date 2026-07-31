package devtools

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// settingsPanel renders the browser settings as an editable form.
// The previous implementation only displayed the values; this
// version lets the user change them in-place. Changes are applied
// immediately through the settingsProvider, which is responsible
// for persistence (typically the browser profile store).
//
// The form is split into two sections: General (homepage and
// default search engine) and Privacy (JS and image toggles). This
// matches the spec in ROADMAP_DEVTOOLS.md and the look of Chrome's
// DevTools Settings tab.
type settingsPanel struct {
	fyne.Container

	// Form widgets, exported for tests.
	homepageEntry *widget.Entry
	searchEntry   *widget.Entry
	jsToggle      *widget.Check
	imgToggle     *widget.Check
	status        *widget.Label

	// activeTabFn is the callback the write helpers use to
	// reach the active tab context. Stored as a field so tests
	// can swap it after construction.
	activeTabFn func() *TabContext
}

func newSettingsPanel(activeTab func() *TabContext) fyne.CanvasObject {
	p := &settingsPanel{
		status: widget.NewLabel(""),
	}
	p.setActiveTabFn(activeTab)
	p.status.Wrapping = fyne.TextWrapWord

	// Form widgets, populated lazily on first refresh so the
	// displayed values match the live provider state.
	p.homepageEntry = widget.NewEntry()
	p.homepageEntry.PlaceHolder = "https://example.com"
	p.searchEntry = widget.NewEntry()
	p.searchEntry.PlaceHolder = "https://duckduckgo.com/?q=%s"
	p.jsToggle = widget.NewCheck("Enable JavaScript", nil)
	p.imgToggle = widget.NewCheck("Load images", nil)

	// Each control writes back to the provider. We capture the
	// current value via the provider's Get* so we know the
	// "after" state without re-reading. The OnChanged / OnSubmitted
	// callbacks are the canonical Fyne entry points for the
	// entry (OnSubmitted fires on Enter) and the check
	// (OnChanged fires on toggle).
	p.homepageEntry.OnSubmitted = func(s string) { p.writeHomepage(s) }
	p.searchEntry.OnSubmitted = func(s string) { p.writeSearch(s) }
	p.jsToggle.OnChanged = func(b bool) { p.writeEnableJS(b) }
	p.imgToggle.OnChanged = func(b bool) { p.writeEnableImages(b) }

	// Status line that confirms the last successful write. The
	// label is short so it fits a single line at the bottom of
	// the form.
	refreshBtn := widget.NewButton("Refresh", func() {
		if activeTab != nil {
			p.refreshFrom(activeTab())
		}
	})

	// Form layout: two sections, each with bold header and
	// labelled form rows. The labels are short so the form
	// fits in the standard DevTools tab width.
	general := container.NewVBox(
		widget.NewLabelWithStyle("General", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Homepage"),
		p.homepageEntry,
		widget.NewLabel("Default search engine"),
		p.searchEntry,
	)
	privacy := container.NewVBox(
		widget.NewLabelWithStyle("Privacy", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		p.jsToggle,
		p.imgToggle,
	)

	body := container.NewVSplit(general, privacy)
	bottomBar := container.NewBorder(nil, nil, refreshBtn, p.status, nil)

	topBar := widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	header := container.NewBorder(nil, nil, topBar, nil,
		container.NewHBox(refreshBtn, widget.NewLabel("")))

	p.Container = *container.NewBorder(header, bottomBar, nil, nil, body)

	// Initial population: defer one tick so the dock has a
	// chance to install the activeTab callback. Production
	// callers always wire activeTab before construction
	// completes; this is just belt-and-braces.
	if activeTab != nil {
		p.refreshFrom(activeTab())
	}
	// Return *p (not &p.Container) so callers can type-assert
	// back to *settingsPanel if they need access to the form
	// widgets. The embedded fyne.Container still satisfies the
	// fyne.CanvasObject contract via the promoted methods.
	return p
}

// RefreshFrom pulls the current settings from the active tab
// context and updates the form fields. Call this when the active
// tab changes so the form follows the active document.
func (p *settingsPanel) RefreshFrom(ctx *TabContext) {
	p.refreshFrom(ctx)
}

func (p *settingsPanel) refreshFrom(ctx *TabContext) {
	if ctx == nil || ctx.Settings == nil {
		p.status.SetText("No settings provider available.")
		return
	}
	s := ctx.Settings
	p.homepageEntry.SetText(s.GetHomepage())
	p.searchEntry.SetText(s.GetDefaultSearchEngine())
	p.jsToggle.SetChecked(s.GetEnableJavaScript())
	p.imgToggle.SetChecked(s.GetEnableImages())
	p.status.SetText("Loaded current settings.")
}

// writeHomepage persists the homepage URL via the active tab's
// settings provider. The provider is responsible for both the
// in-memory update and any on-disk persistence.
func (p *settingsPanel) writeHomepage(url string) {
	ctx := p.activeTab()
	if ctx == nil || ctx.Settings == nil {
		p.status.SetText("No settings provider available.")
		return
	}
	ctx.Settings.SetHomepage(url)
	p.status.SetText(fmt.Sprintf("Homepage updated to %s", url))
}

// writeSearch persists the default search-engine URL.
func (p *settingsPanel) writeSearch(url string) {
	ctx := p.activeTab()
	if ctx == nil || ctx.Settings == nil {
		p.status.SetText("No settings provider available.")
		return
	}
	ctx.Settings.SetDefaultSearchEngine(url)
	p.status.SetText("Search engine updated.")
}

// writeEnableJS toggles JavaScript execution.
func (p *settingsPanel) writeEnableJS(enabled bool) {
	ctx := p.activeTab()
	if ctx == nil || ctx.Settings == nil {
		p.status.SetText("No settings provider available.")
		return
	}
	ctx.Settings.SetEnableJavaScript(enabled)
	if enabled {
		p.status.SetText("JavaScript enabled.")
	} else {
		p.status.SetText("JavaScript disabled.")
	}
}

// writeEnableImages toggles image loading.
func (p *settingsPanel) writeEnableImages(enabled bool) {
	ctx := p.activeTab()
	if ctx == nil || ctx.Settings == nil {
		p.status.SetText("No settings provider available.")
		return
	}
	ctx.Settings.SetEnableImages(enabled)
	if enabled {
		p.status.SetText("Image loading enabled.")
	} else {
		p.status.SetText("Image loading disabled.")
	}
}

// activeTab is a small indirection that lets tests inject a
// custom callback without subclassing. The constructor captures
// the activeTab callback into a closure stored on the panel; the
// helper returns nil when the production caller hasn't wired one.
func (p *settingsPanel) activeTab() *TabContext {
	if p.activeTabFn == nil {
		return nil
	}
	return p.activeTabFn()
}

// setActiveTabFn installs the activeTab callback on the panel.
// Production callers wire it once at construction time; tests can
// swap it later via the same setter to inject a custom provider.
func (p *settingsPanel) setActiveTabFn(fn func() *TabContext) {
	p.activeTabFn = fn
}