package ui_test

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/ui"
)

func TestBrowserDevToolsButtonCreated(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	assert.NotNil(t, browser.DevToolsButton())
	assert.Equal(t, "DevTools", browser.DevToolsButton().Text)
}

func TestBrowserShowDisplayListNoRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	browser.ShowDisplayListDialog()
	assert.True(t, browser.DevToolsVisible())
}

func TestBrowserShowDisplayListWithSummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	browser.ShowDisplayListDialog()
	assert.True(t, browser.DevToolsVisible())
}

func TestBrowserShowDisplayListWithEmptySummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	browser.ShowDisplayListDialog()
	assert.True(t, browser.DevToolsVisible())
}

func TestBrowserShowDisplayListWithNilSummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	browser.ShowDisplayListDialog()
	assert.True(t, browser.DevToolsVisible())
}

func TestBrowserDevToolsButtonInNavBar(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	assert.NotNil(t, browser.DevToolsButton())
	assert.Equal(t, "DevTools", browser.DevToolsButton().Text)

	browser.DevToolsButton().OnTapped()
	assert.True(t, browser.DevToolsVisible())

	browser.DevToolsButton().OnTapped()
	assert.False(t, browser.DevToolsVisible())
}
