package ui_test

import (
	"github.com/vyquocvu/goosie/internal/ui"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

func TestTabSetGetRawSource(t *testing.T) {
	tab := &ui.Tab{}
	assert.Equal(t, "", tab.GetRawSource(), "fresh tab has empty source")

	html := "<html><body><p>Hello</p></body></html>"
	tab.SetRawSource(html)
	assert.Equal(t, html, tab.GetRawSource())
}

func TestTabSetRawSourceEmpty(t *testing.T) {
	tab := &ui.Tab{}
	tab.SetRawSource("")
	assert.Equal(t, "", tab.GetRawSource())

	tab.SetRawSource("<html></html>")
	assert.NotEqual(t, "", tab.GetRawSource())

	tab.SetRawSource("")
	assert.Equal(t, "", tab.GetRawSource(), "overwriting with empty clears the source")
}

func TestBrowserDevToolsButtonSourceDialog(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	assert.NotNil(t, browser.DevToolsButton())
	assert.Equal(t, "DevTools", browser.DevToolsButton().Text)
}

func TestBrowserShowSourceDialogWithEmptySource(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	tab := browser.ActiveTab()
	assert.NotNil(t, tab)
	assert.Equal(t, "", tab.GetRawSource())

	// showSourceDialog should open the dock without panic
	browser.ShowSourceDialog()
	assert.True(t, browser.DevToolsVisible())
}

func TestBrowserShowSourceDialogWithContent(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	tab := browser.ActiveTab()
	assert.NotNil(t, tab)

	html := "<html><body><p>Hello, World!</p></body></html>"
	tab.SetRawSource(html)
	assert.Equal(t, html, tab.GetRawSource())

	// showSourceDialog should open the dock without panic
	browser.ShowSourceDialog()
	assert.True(t, browser.DevToolsVisible())
}

func TestTabSourceSurvivesTabSwitch(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	tab1 := browser.ActiveTab()
	tab1.SetRawSource("<html>tab1</html>")

	tab2 := browser.NewTab()
	tab2.SetRawSource("<html>tab2</html>")

	assert.Equal(t, "<html>tab1</html>", tab1.GetRawSource())
	assert.Equal(t, "<html>tab2</html>", tab2.GetRawSource())
}

func TestBrowserDevToolsButtonToggle(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	assert.NotNil(t, browser.DevToolsButton())
	assert.Equal(t, "DevTools", browser.DevToolsButton().Text)
	assert.False(t, browser.DevToolsVisible())

	// Click to open
	browser.DevToolsButton().OnTapped()
	assert.True(t, browser.DevToolsVisible())

	// Click to close
	browser.DevToolsButton().OnTapped()
	assert.False(t, browser.DevToolsVisible())
}
