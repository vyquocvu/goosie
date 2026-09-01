package ui_test

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/ui"
)

func TestBrowserDirtyOverlayButtonCreated(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	assert.NotNil(t, browser.DirtyOverlayButton())
	assert.Equal(t, "Ov", browser.DirtyOverlayButton().Text)
}

func TestBrowserToggleDirtyOverlayNoRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	browser.ToggleDirtyOverlay()
}

func TestBrowserToggleDirtyOverlayWithRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.SetHTMLRenderer(&MockHTMLRendererComp{})

	// Toggle on
	browser.ToggleDirtyOverlay()
	assert.Equal(t, "Ov✓", browser.DirtyOverlayButton().Text)

	// Toggle off
	browser.ToggleDirtyOverlay()
	assert.Equal(t, "Ov", browser.DirtyOverlayButton().Text)
}

func TestBrowserDirtyOverlayButtonInNavBar(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	assert.NotNil(t, browser.DirtyOverlayButton())
	assert.Equal(t, "Ov", browser.DirtyOverlayButton().Text)

	// Without a renderer the button tap is a no-op (toggle returns early)
	browser.DirtyOverlayButton().OnTapped()
	assert.Equal(t, "Ov", browser.DirtyOverlayButton().Text)
}

func TestBrowserDirtyOverlayButtonToggleWithRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.SetHTMLRenderer(&MockHTMLRendererComp{})

	browser.DirtyOverlayButton().OnTapped()
	assert.Equal(t, "Ov✓", browser.DirtyOverlayButton().Text)

	browser.DirtyOverlayButton().OnTapped()
	assert.Equal(t, "Ov", browser.DirtyOverlayButton().Text)
}

func TestCommandTypeToOverlayColor(t *testing.T) {
	tests := []struct {
		cmdType renderer.PaintCommandType
		name    string
	}{
		{renderer.PaintText, "PaintText"},
		{renderer.PaintRect, "PaintRect"},
		{renderer.PaintImage, "PaintImage"},
		{renderer.PaintLink, "PaintLink"},
		{renderer.PaintBorder, "PaintBorder"},
		{renderer.PaintButton, "PaintButton"},
		{renderer.PaintInput, "PaintInput"},
		{renderer.PaintTextarea, "PaintTextarea"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clr := renderer.CommandTypeToOverlayColor(tt.cmdType)
			assert.NotNil(t, clr)
		})
	}
}

func TestCommandTypeToOverlayColorDefault(t *testing.T) {
	clr := renderer.CommandTypeToOverlayColor(99)
	assert.NotNil(t, clr)
}
