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

// moduleRootVisual walks upward from this test file to the directory
// that owns go.mod, so screenshot artifacts land at the repo root
// regardless of the test's working directory.
func moduleRootVisual(t *testing.T) string {
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

// TestSettingsPanelVisual_Rendered renders the Settings panel and
// saves a PNG snapshot. This is the visual audit artifact for the
// devtools resize fix: the status label must stay on a single line
// so the panel's MinSize remains stable (and the splitter drag range
// usable) when the panel is laid out inside an AppTab at a narrow
// width.
func TestSettingsPanelVisual_Rendered(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		Settings: &mockSettingsProvider{
			homepage:      "https://example.com",
			defaultSearch: "https://duckduckgo.com/?q=%s",
			enableJS:      true,
			enableImages:  true,
		},
	}
	p := newSettingsPanel(func() *TabContext { return ctx }).(*settingsPanel)
	p.RefreshFrom(ctx)

	w := app.NewWindow("Settings — visual audit")
	w.Resize(fyne.NewSize(1200, 720))
	w.SetContent(&p.Container)
	w.Show()

	w.Canvas().Refresh(w.Content())
	time.Sleep(120 * time.Millisecond)
	w.Canvas().Refresh(w.Content())

	img := w.Canvas().Capture()
	require.NotNil(t, img)

	root := moduleRootVisual(t)
	for _, dir := range []string{
		filepath.Join(root, "screenshots"),
		filepath.Join(root, ".gstack", "artifacts"),
	} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		path := filepath.Join(dir, "settings_panel.png")
		f, err := os.Create(path)
		require.NoError(t, err)
		require.NoError(t, png.Encode(f, img.(image.Image)))
		_ = f.Close()
	}
}
