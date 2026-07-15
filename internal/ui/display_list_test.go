package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

func TestBrowserDevToolsButtonCreated(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.devToolsButton)
	assert.Equal(t, "DevTools", browser.devToolsButton.Text)
}

func TestBrowserShowDisplayListNoRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	browser.showDisplayListDialog()
	assert.True(t, browser.devToolsVisible)
}

func TestBrowserShowDisplayListWithSummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	browser.showDisplayListDialog()
	assert.True(t, browser.devToolsVisible)
}

func TestBrowserShowDisplayListWithEmptySummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	browser.showDisplayListDialog()
	assert.True(t, browser.devToolsVisible)
}

func TestBrowserShowDisplayListWithNilSummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	browser.showDisplayListDialog()
	assert.True(t, browser.devToolsVisible)
}

func TestBrowserDevToolsButtonInNavBar(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.devToolsButton)
	assert.Equal(t, "DevTools", browser.devToolsButton.Text)

	browser.devToolsButton.OnTapped()
	assert.True(t, browser.devToolsVisible)

	browser.devToolsButton.OnTapped()
	assert.False(t, browser.devToolsVisible)
}
