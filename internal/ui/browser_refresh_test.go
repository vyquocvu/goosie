package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
)

type viewportRenderer struct {
	MockHTMLRenderer
	updated fyne.CanvasObject
}

func (r *viewportRenderer) UpdateViewport() fyne.CanvasObject {
	return r.updated
}

func TestRefreshTabContentReplacesCanvasAfterAsyncStyles(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	oldContent := widget.NewLabel("unstyled")
	updatedContent := widget.NewLabel("styled")
	tab := &Tab{
		contentScroll: container.NewScroll(oldContent),
		htmlRenderer:  &viewportRenderer{updated: updatedContent},
	}

	refreshTabContent(tab)

	require.Same(t, updatedContent, tab.contentScroll.Content)
}
