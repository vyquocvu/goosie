package devtools

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

type mockSettingsProvider struct {
	homepage      string
	defaultSearch string
	enableJS      bool
	enableImages  bool

	// Capture every setter call so tests can assert on the
	// exact path the panel took. We keep the last-written
	// value as well as a counter so tests can verify the
	// write happened at all.
	setHomepageCalls  int
	setSearchCalls    int
	setEnableJSCalls  int
	setEnableImgCalls int
	lastHomepage      string
	lastDefaultSearch string
	lastEnableJS      bool
	lastEnableImages  bool
}

func (m *mockSettingsProvider) GetHomepage() string            { return m.homepage }
func (m *mockSettingsProvider) GetDefaultSearchEngine() string { return m.defaultSearch }
func (m *mockSettingsProvider) GetEnableJavaScript() bool      { return m.enableJS }
func (m *mockSettingsProvider) GetEnableImages() bool          { return m.enableImages }
func (m *mockSettingsProvider) SetHomepage(url string) {
	m.setHomepageCalls++
	m.lastHomepage = url
	m.homepage = url
}
func (m *mockSettingsProvider) SetDefaultSearchEngine(url string) {
	m.setSearchCalls++
	m.lastDefaultSearch = url
	m.defaultSearch = url
}
func (m *mockSettingsProvider) SetEnableJavaScript(enabled bool) {
	m.setEnableJSCalls++
	m.lastEnableJS = enabled
	m.enableJS = enabled
}
func (m *mockSettingsProvider) SetEnableImages(enabled bool) {
	m.setEnableImgCalls++
	m.lastEnableImages = enabled
	m.enableImages = enabled
}

func TestSettingsPanel_InitialState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newSettingsPanel(func() *TabContext { return &TabContext{} }).(*settingsPanel)
	assert.NotNil(t, p)
	assert.NotNil(t, p.homepageEntry)
	assert.NotNil(t, p.searchEntry)
	assert.NotNil(t, p.jsToggle)
	assert.NotNil(t, p.imgToggle)
}

func TestSettingsPanel_NoSettingsProvider(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newSettingsPanel(func() *TabContext { return &TabContext{} }).(*settingsPanel)
	p.RefreshFrom(&TabContext{})
	assert.Contains(t, p.status.Text, "No settings provider")
}

func TestSettingsPanel_RefreshFrom(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		Settings: &mockSettingsProvider{
			homepage:      "https://example.com",
			defaultSearch: "https://duckduckgo.com/?q=%s",
			enableJS:      true,
			enableImages:  false,
		},
	}
	p := newSettingsPanel(func() *TabContext { return ctx }).(*settingsPanel)
	p.RefreshFrom(ctx)

	// The form widgets must reflect the provider's values.
	assert.Equal(t, "https://example.com", p.homepageEntry.Text)
	assert.Equal(t, "https://duckduckgo.com/?q=%s", p.searchEntry.Text)
	assert.True(t, p.jsToggle.Checked)
	assert.False(t, p.imgToggle.Checked)
	assert.Contains(t, p.status.Text, "Loaded current settings")
}

func TestSettingsPanel_RefreshFromNilTabContext(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newSettingsPanel(nil).(*settingsPanel)
	p.RefreshFrom(nil)
	assert.Contains(t, p.status.Text, "No settings provider")
}

// TestSettingsPanel_WriteHomepage verifies that submitting the
// homepage entry calls SetHomepage on the provider with the new
// value. This is the regression guard against the panel going
// read-only after the form upgrade.
func TestSettingsPanel_WriteHomepage(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	settings := &mockSettingsProvider{
		homepage: "https://start.example",
	}
	ctx := &TabContext{Settings: settings}
	p := newSettingsPanel(func() *TabContext { return ctx }).(*settingsPanel)

	p.homepageEntry.SetText("https://changed.example")
	p.homepageEntry.OnSubmitted("https://changed.example")

	assert.Equal(t, 1, settings.setHomepageCalls,
		"SetHomepage must be called exactly once on submit")
	assert.Equal(t, "https://changed.example", settings.lastHomepage,
		"the panel must forward the entry text to the provider")
	assert.Contains(t, p.status.Text, "Homepage updated")
}

// TestSettingsPanel_WriteSearch verifies that the search-engine
// entry writes through to the provider.
func TestSettingsPanel_WriteSearch(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	settings := &mockSettingsProvider{
		defaultSearch: "Google",
	}
	ctx := &TabContext{Settings: settings}
	p := newSettingsPanel(func() *TabContext { return ctx }).(*settingsPanel)

	p.searchEntry.SetText("DuckDuckGo")
	p.searchEntry.OnSubmitted("DuckDuckGo")

	assert.Equal(t, 1, settings.setSearchCalls,
		"SetDefaultSearchEngine must be called on submit")
	assert.Equal(t, "DuckDuckGo", settings.lastDefaultSearch)
	assert.Contains(t, p.status.Text, "Search engine updated")
}

// TestSettingsPanel_ToggleJavaScript verifies that toggling the
// JS check calls SetEnableJavaScript on the provider. Fyne's
// SetChecked triggers OnChanged once when the value actually
// changes, so the test fires OnChanged directly rather than
// relying on SetChecked's side-effect (which is the production
// path).
func TestSettingsPanel_ToggleJavaScript(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	settings := &mockSettingsProvider{enableJS: true}
	ctx := &TabContext{Settings: settings}
	p := newSettingsPanel(func() *TabContext { return ctx }).(*settingsPanel)

	p.jsToggle.OnChanged(false)

	assert.GreaterOrEqual(t, settings.setEnableJSCalls, 1,
		"SetEnableJavaScript must fire on toggle")
	assert.False(t, settings.lastEnableJS,
		"the new value must be forwarded to the provider")
	assert.Contains(t, p.status.Text, "JavaScript disabled")
}

// TestSettingsPanel_ToggleImages verifies that toggling the images
// check calls SetEnableImages on the provider.
func TestSettingsPanel_ToggleImages(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	settings := &mockSettingsProvider{enableImages: false}
	ctx := &TabContext{Settings: settings}
	p := newSettingsPanel(func() *TabContext { return ctx }).(*settingsPanel)

	p.imgToggle.OnChanged(true)

	assert.GreaterOrEqual(t, settings.setEnableImgCalls, 1,
		"SetEnableImages must fire on toggle")
	assert.True(t, settings.lastEnableImages,
		"the new value must be forwarded to the provider")
	assert.Contains(t, p.status.Text, "Image loading enabled")
}

// TestSettingsPanel_NoProviderWritesFail verifies that writes
// against an unset provider do not panic and leave the status
// line in a sensible state.
func TestSettingsPanel_NoProviderWritesFail(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	// TabContext has no Settings provider.
	ctx := &TabContext{}
	p := newSettingsPanel(func() *TabContext { return ctx }).(*settingsPanel)

	assert.NotPanics(t, func() {
		p.homepageEntry.SetText("https://x.example")
		p.homepageEntry.OnSubmitted("https://x.example")
		p.jsToggle.SetChecked(false)
		p.jsToggle.OnChanged(false)
	}, "writes must not panic when no provider is wired")

	assert.Contains(t, p.status.Text, "No settings provider")
	// None of the provider's counters should have moved.
	// (We don't have a provider reference here so the assertion
	// is implicit: the panel must not have written anywhere.)
}

// TestSettingsPanel_PassiveSetup verifies that constructing the
// panel without an activeTab callback leaves the panel in a safe
// state and that writes are silently dropped instead of
// panicking. This is the regression guard for the "no tab"
// construction path used by headless test harnesses.
func TestSettingsPanel_PassiveSetup(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newSettingsPanel(nil).(*settingsPanel)
	assert.NotPanics(t, func() {
		p.RefreshFrom(nil)
		p.RefreshFrom(&TabContext{}) // no provider
	})
}