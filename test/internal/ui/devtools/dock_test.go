package devtools_test

import (
	"github.com/vyquocvu/goosie/internal/ui/devtools"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

// TestDockContainer_MinSizeFitsWindow is the regression guard for
// the devtools resize bug. The Settings panel contained a wrapping
// status label whose MinSize depended on the current layout width.
// Once the dock was laid out, the status ballooned to dozens of
// lines and the dock Container MinSize.Height grew to 1004 — well
// past a typical 700-pixel window height. With the dock MinSize
// larger than the viewport, the container.Split drag range
// collapsed to a few pixels and the user could not resize the
// devtools pane. The fix keeps the Settings panel MinSize close
// to its pre-layout value, so the dock Container MinSize stays
// within a usable window.
func TestDockContainer_MinSizeFitsWindow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dock := devtools.NewDock(func() *devtools.TabContext { return nil })
	dock.EnsureTabs()

	dockContainer := container.NewMax(dock.CanvasObject())

	w := app.NewWindow("test")
	w.Resize(fyne.NewSize(1000, 700))
	w.SetContent(dockContainer)
	w.Show()

	min := dockContainer.MinSize()
	// The dock MinSize height must be smaller than the window
	// height, otherwise the splitter drag range collapses and the
	// user cannot shrink the devtools pane.
	assert.Less(t, min.Height, float32(700),
		"dock Container MinSize.Height (%v) must fit in a standard window (700)", min.Height)
}
