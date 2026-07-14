package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestBrowserDirtyOverlayButtonCreated(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.dirtyOverlayButton)
	assert.Equal(t, "Ov", browser.dirtyOverlayButton.Text)
}

func TestBrowserToggleDirtyOverlayNoRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	browser.toggleDirtyOverlay()
}

func TestBrowserToggleDirtyOverlayWithRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.htmlRenderer = &MockHTMLRendererComp{}

	// Toggle on
	browser.toggleDirtyOverlay()
	assert.Equal(t, "Ov✓", browser.dirtyOverlayButton.Text)

	// Toggle off
	browser.toggleDirtyOverlay()
	assert.Equal(t, "Ov", browser.dirtyOverlayButton.Text)
}

func TestBrowserDirtyOverlayButtonInNavBar(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.dirtyOverlayButton)
	assert.Equal(t, "Ov", browser.dirtyOverlayButton.Text)

	// Without a renderer the button tap is a no-op (toggle returns early)
	browser.dirtyOverlayButton.OnTapped()
	assert.Equal(t, "Ov", browser.dirtyOverlayButton.Text)
}

func TestBrowserDirtyOverlayButtonToggleWithRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.htmlRenderer = &MockHTMLRendererComp{}

	browser.dirtyOverlayButton.OnTapped()
	assert.Equal(t, "Ov✓", browser.dirtyOverlayButton.Text)

	browser.dirtyOverlayButton.OnTapped()
	assert.Equal(t, "Ov", browser.dirtyOverlayButton.Text)
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
