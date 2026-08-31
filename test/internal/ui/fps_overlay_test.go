package ui_test

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/ui"
)

func TestBrowserFPSButtonCreated(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	assert.NotNil(t, browser.FPSButton())
	assert.Equal(t, "FPS", browser.FPSButton().Text)
}

func TestBrowserToggleFPSOverlayNoRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	// Without a renderer the toggle is a no-op and should not panic.
	browser.ToggleFPSOverlay()
}

func TestBrowserToggleFPSOverlayWithRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.SetHTMLRenderer(&MockHTMLRendererComp{})

	// Toggle on
	browser.ToggleFPSOverlay()
	assert.Equal(t, "FPS✓", browser.FPSButton().Text)

	// Toggle off
	browser.ToggleFPSOverlay()
	assert.Equal(t, "FPS", browser.FPSButton().Text)
}

func TestBrowserFPSButtonInNavBar(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	assert.NotNil(t, browser.FPSButton())
	assert.Equal(t, "FPS", browser.FPSButton().Text)

	// Without a renderer the button tap leaves the text unchanged.
	browser.FPSButton().OnTapped()
	assert.Equal(t, "FPS", browser.FPSButton().Text)
}

func TestBrowserFPSButtonToggleWithRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.SetHTMLRenderer(&MockHTMLRendererComp{})

	browser.FPSButton().OnTapped()
	assert.Equal(t, "FPS✓", browser.FPSButton().Text)

	browser.FPSButton().OnTapped()
	assert.Equal(t, "FPS", browser.FPSButton().Text)
}
