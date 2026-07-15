package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

func TestTabSetGetRawSource(t *testing.T) {
	tab := &Tab{}
	assert.Equal(t, "", tab.GetRawSource(), "fresh tab has empty source")

	html := "<html><body><p>Hello</p></body></html>"
	tab.SetRawSource(html)
	assert.Equal(t, html, tab.GetRawSource())
}

func TestTabSetRawSourceEmpty(t *testing.T) {
	tab := &Tab{}
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
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.devToolsButton)
	assert.Equal(t, "DevTools", browser.devToolsButton.Text)
}

func TestBrowserShowSourceDialogWithEmptySource(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	tab := browser.ActiveTab()
	assert.NotNil(t, tab)
	assert.Equal(t, "", tab.GetRawSource())

	// showSourceDialog should open the dock without panic
	browser.showSourceDialog()
	assert.True(t, browser.devToolsVisible)
}

func TestBrowserShowSourceDialogWithContent(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	tab := browser.ActiveTab()
	assert.NotNil(t, tab)

	html := "<html><body><p>Hello, World!</p></body></html>"
	tab.SetRawSource(html)
	assert.Equal(t, html, tab.GetRawSource())

	// showSourceDialog should open the dock without panic
	browser.showSourceDialog()
	assert.True(t, browser.devToolsVisible)
}

func TestTabSourceSurvivesTabSwitch(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

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
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.devToolsButton)
	assert.Equal(t, "DevTools", browser.devToolsButton.Text)
	assert.False(t, browser.devToolsVisible)

	// Click to open
	browser.devToolsButton.OnTapped()
	assert.True(t, browser.devToolsVisible)

	// Click to close
	browser.devToolsButton.OnTapped()
	assert.False(t, browser.devToolsVisible)
}
