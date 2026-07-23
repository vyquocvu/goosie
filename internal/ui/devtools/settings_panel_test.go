package devtools

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

type mockSettingsProvider struct {
	homepage        string
	defaultSearch   string
	enableJS        bool
	enableImages    bool
}

func (m *mockSettingsProvider) GetHomepage() string            { return m.homepage }
func (m *mockSettingsProvider) GetDefaultSearchEngine() string { return m.defaultSearch }
func (m *mockSettingsProvider) GetEnableJavaScript() bool      { return m.enableJS }
func (m *mockSettingsProvider) GetEnableImages() bool          { return m.enableImages }

func TestSettingsPanel_InitialState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newSettingsPanel(nil).(*settingsPanel)
	assert.NotNil(t, p)
	assert.Contains(t, p.label.Text, "No settings available")
}

func TestSettingsPanel_NilProvider(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newSettingsPanel(func() *TabContext { return &TabContext{} }).(*settingsPanel)
	p.RefreshFrom(&TabContext{})
	assert.Contains(t, p.label.Text, "No settings provider available")
}

func TestSettingsPanel_RefreshFrom(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		Settings: &mockSettingsProvider{
			homepage:      "https://example.com",
			defaultSearch: "Google",
			enableJS:      true,
			enableImages:  false,
		},
	}
	p := newSettingsPanel(func() *TabContext { return ctx }).(*settingsPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.label.Text, "https://example.com")
	assert.Contains(t, p.label.Text, "Google")
	assert.Contains(t, p.label.Text, "Enabled")
	assert.Contains(t, p.label.Text, "Disabled")
}

func TestSettingsPanel_RefreshFromNilTabContext(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newSettingsPanel(nil).(*settingsPanel)
	p.RefreshFrom(nil)
	assert.Contains(t, p.label.Text, "No settings available")
}

func TestSettingsPanel_AllDisabled(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		Settings: &mockSettingsProvider{
			homepage:      "about:blank",
			defaultSearch: "DuckDuckGo",
			enableJS:      false,
			enableImages:  false,
		},
	}
	p := newSettingsPanel(func() *TabContext { return ctx }).(*settingsPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.label.Text, "Disabled")
	assert.NotContains(t, p.label.Text, "Enabled")
}
