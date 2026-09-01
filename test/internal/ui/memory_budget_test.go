package ui_test

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/ui"
)

func TestBrowserDevToolsButtonCreatedMem(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	assert.NotNil(t, browser.DevToolsButton())
	assert.Equal(t, "DevTools", browser.DevToolsButton().Text)
}

func TestBrowserShowMemoryDialogNoManager(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	browser.ShowMemoryDialog()
	assert.True(t, browser.DevToolsVisible())
}

func TestBrowserDevToolsButtonToggling(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	assert.False(t, browser.DevToolsVisible())

	browser.DevToolsButton().OnTapped()
	assert.True(t, browser.DevToolsVisible())

	browser.DevToolsButton().OnTapped()
	assert.False(t, browser.DevToolsVisible())
}
