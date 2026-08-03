package devtools

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
)

// dockRoot walks upward from this test file to the directory that owns
// go.mod, so screenshot artifacts land at the repo root regardless of
// the test's working directory.
func dockRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod above %s", filepath.Dir(file))
		}
		dir = parent
	}
}

// TestDockVisual_Resizable captures the devtools dock with all
// panels wired up so the visual audit can confirm the splitter has
// a usable drag range. Before the dock MinSize fix, the Settings
// panel's status label wrapped to dozens of lines, the dock
// MinSize.Height exceeded the window, and the devtools pane was
// effectively pinned at its full MinSize — i.e., un-resizable.
func TestDockVisual_Resizable(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dock := NewDock(func() *TabContext { return nil })
	dock.EnsureTabs()

	dock.SelectTab("Settings")
	dockContainer := container.NewMax(dock.CanvasObject())

	// Mirror the browser's split: page on top, devtools on bottom.
	page := container.NewVBox()
	pageContent := dockContainer
	split := container.NewVSplit(page, pageContent)
	split.SetOffset(0.65)

	w := app.NewWindow("Dock — visual audit")
	w.Resize(fyne.NewSize(1000, 700))
	w.SetContent(split)
	w.Show()

	w.Canvas().Refresh(w.Content())
	time.Sleep(150 * time.Millisecond)
	w.Canvas().Refresh(w.Content())

	// Capture the open state.
	root := dockRoot(t)
	capture := func(name string) {
		img := w.Canvas().Capture()
		require.NotNil(t, img)
		for _, dir := range []string{
			filepath.Join(root, "screenshots"),
			filepath.Join(root, ".gstack", "artifacts"),
		} {
			require.NoError(t, os.MkdirAll(dir, 0o755))
			path := filepath.Join(dir, name)
			f, err := os.Create(path)
			require.NoError(t, err)
			require.NoError(t, png.Encode(f, img.(image.Image)))
			_ = f.Close()
		}
	}

	capture("devtools_dock_open.png")

	// Push the devtools pane to its minimum so the splitter has
	// actually moved. The Offset is clamped to the range allowed by
	// the dock MinSize, so 0.0 (page takes everything the dock can
	// spare) is the smallest dock configuration.
	split.SetOffset(0.0)
	split.Refresh()
	w.Canvas().Refresh(w.Content())
	time.Sleep(100 * time.Millisecond)
	w.Canvas().Refresh(w.Content())
	capture("devtools_dock_shrunk.png")

	t.Logf("split.Offset = %v", split.Offset)
	t.Logf("page size = %v", page.Size())
	t.Logf("pageContent size = %v", pageContent.Size())
	t.Logf("dockContainer size = %v", dockContainer.Size())
	t.Logf("dock MinSize = %v", dockContainer.MinSize())

	// Verify the dock ended up smaller than its pre-layout MinSize
	// would have allowed before the fix.
	require.Less(t, pageContent.Size().Height, float32(700),
		"the devtools pane must still be visible after shrinking")
}
