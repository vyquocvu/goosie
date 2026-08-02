package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

func TestBrowserFPSButtonCreated(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.fpsButton)
	assert.Equal(t, "FPS", browser.fpsButton.Text)
}

func TestBrowserToggleFPSOverlayNoRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	// Without a renderer the toggle is a no-op and should not panic.
	browser.toggleFPSOverlay()
}

func TestBrowserToggleFPSOverlayWithRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.htmlRenderer = &MockHTMLRendererComp{}

	// Toggle on
	browser.toggleFPSOverlay()
	assert.Equal(t, "FPS✓", browser.fpsButton.Text)

	// Toggle off
	browser.toggleFPSOverlay()
	assert.Equal(t, "FPS", browser.fpsButton.Text)
}

func TestBrowserFPSButtonInNavBar(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.fpsButton)
	assert.Equal(t, "FPS", browser.fpsButton.Text)

	// Without a renderer the button tap leaves the text unchanged.
	browser.fpsButton.OnTapped()
	assert.Equal(t, "FPS", browser.fpsButton.Text)
}

func TestBrowserFPSButtonToggleWithRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.htmlRenderer = &MockHTMLRendererComp{}

	browser.fpsButton.OnTapped()
	assert.Equal(t, "FPS✓", browser.fpsButton.Text)

	browser.fpsButton.OnTapped()
	assert.Equal(t, "FPS", browser.fpsButton.Text)
}
