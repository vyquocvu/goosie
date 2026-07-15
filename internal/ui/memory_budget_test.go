package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

func TestBrowserDevToolsButtonCreatedMem(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.devToolsButton)
	assert.Equal(t, "DevTools", browser.devToolsButton.Text)
}

func TestBrowserShowMemoryDialogNoManager(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	browser.showMemoryDialog()
	assert.True(t, browser.devToolsVisible)
}

func TestBrowserDevToolsButtonToggling(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.False(t, browser.devToolsVisible)

	browser.devToolsButton.OnTapped()
	assert.True(t, browser.devToolsVisible)

	browser.devToolsButton.OnTapped()
	assert.False(t, browser.devToolsVisible)
}
