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
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
)

// moduleRoot walks upward from this test file to the directory that owns
// go.mod, so screenshot artifacts land at the repo root regardless of
// the test's working directory.
func moduleRoot(t *testing.T) string {
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

// TestSourcesPanelVisual_Rendered renders the Sources panel with realistic
// live engine data in a headless Fyne window and saves PNG snapshots that
// exercise the main interactive flows: default state, filtered state, and
// cached sub-resource body viewer. These are the visual audit artifacts
// committed under .gstack/artifacts/.
func TestSourcesPanelVisual_Rendered(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	cache := &mockSourceCache{bodies: map[string]string{
		"https://example.com/styles.css": "body { margin: 0; color: #222; }\nh1 { font-weight: 700; }",
		"https://example.com/app.js":     "console.log('boot');\nwindow.addEventListener('load', init);",
	}}
	ctx := sourcesTestContext()
	ctx.SourceCache = cache

	w := app.NewWindow("Sources — visual audit")
	w.Resize(fyne.NewSize(1200, 720))
	p := newSourcesPanel(func() *TabContext { return ctx })
	w.SetContent(&p.Container)
	w.Show()

	p.RefreshFrom(ctx)
	p.tree.Select("res:0")
	captureAndSave(t, w, p, "sources_panel.png")

	// Filtered view with the cached stylesheet selected to prove the
	// network cache feeds real sub-resource bodies into the viewer.
	p.filterEntry.SetText("styles")
	cssIdx := -1
	for i, r := range p.visible {
		if r.Type == "stylesheet" {
			cssIdx = i
		}
	}
	require.NotEqual(t, -1, cssIdx)
	// Drop the prior selection so Tree.Select fires OnSelected again even
	// when the same uid happens to match.
	p.tree.Unselect("res:0")
	p.tree.Select(treeResourceUID(cssIdx))
	captureAndSave(t, w, p, "sources_panel_stylesheet.png")

	// Filtered (different match) view to demonstrate the live filter.
	p.filterEntry.SetText("app")
	scriptIdx := -1
	for i, r := range p.visible {
		if r.Type == "script" {
			scriptIdx = i
		}
	}
	require.NotEqual(t, -1, scriptIdx)
	p.tree.Unselect(treeResourceUID(cssIdx))
	p.tree.Select(treeResourceUID(scriptIdx))
	captureAndSave(t, w, p, "sources_panel_filtered.png")
}

func captureAndSave(t *testing.T, w fyne.Window, p *sourcesPanel, name string) {
	t.Helper()
	p.Refresh()
	w.Canvas().Refresh(w.Content())
	for i := 0; i < 4; i++ {
		time.Sleep(40 * time.Millisecond)
		w.Canvas().Refresh(w.Content())
	}

	img := w.Canvas().Capture()
	require.NotNil(t, img)

	root := moduleRoot(t)
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

// TestSourcesPanelVisual_Empty verifies the panel renders the empty state
// cleanly with no engine data.
func TestSourcesPanelVisual_Empty(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	w := app.NewWindow("Sources — empty")
	w.Resize(fyne.NewSize(1200, 720))
	p := newSourcesPanel(nil)
	w.SetContent(&p.Container)
	w.Show()
	time.Sleep(150 * time.Millisecond)
	w.Canvas().Refresh(w.Content())
	img := w.Canvas().Capture()
	require.NotNil(t, img)

	root := moduleRoot(t)
	for _, dir := range []string{
		filepath.Join(root, "screenshots"),
		filepath.Join(root, ".gstack", "artifacts"),
	} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		path := filepath.Join(dir, "sources_panel_empty.png")
		f, err := os.Create(path)
		require.NoError(t, err)
		require.NoError(t, png.Encode(f, img.(image.Image)))
		_ = f.Close()
	}
}
