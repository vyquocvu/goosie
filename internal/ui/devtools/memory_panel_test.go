package devtools

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/memory"
)

func TestMemoryPanel_InitialState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newMemoryPanel(nil).(*memoryPanel)
	assert.NotNil(t, p)
	assert.Equal(t, "Memory manager not available.", p.detailsLabel.Text)
	assert.False(t, p.domProgress.Visible())
	assert.False(t, p.layoutProgress.Visible())
	assert.False(t, p.imagesProgress.Visible())
	assert.False(t, p.jsProgress.Visible())
	assert.False(t, p.globalProgress.Visible())
}

func TestMemoryPanel_RefreshFrom(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	cfg := memory.Config{
		Limits: map[memory.Component]uint64{
			memory.ComponentDOM:    1000,
			memory.ComponentLayout: 2000,
			memory.ComponentImage:  3000,
			memory.ComponentScript: 4000,
		},
		GlobalLimit: 10000,
	}
	mgr := memory.NewManager(cfg)
	mgr.UpdateUsage(memory.ComponentDOM, 500)
	mgr.UpdateUsage(memory.ComponentLayout, 1500)
	mgr.UpdateUsage(memory.ComponentImage, 1000)
	mgr.UpdateUsage(memory.ComponentScript, 2000)

	ctx := &TabContext{
		Memory: mgr,
	}

	p := newMemoryPanel(func() *TabContext { return ctx }).(*memoryPanel)
	p.RefreshFrom(ctx)

	// Check labels update
	assert.Contains(t, p.domUsageLabel.Text, "DOM: 500 B / 1000 B")
	assert.Contains(t, p.layoutUsageLabel.Text, "Layout: 1.5 KB / 2.0 KB")
	assert.Contains(t, p.imagesUsageLabel.Text, "Images & Tiles: 1000 B / 2.9 KB")
	assert.Contains(t, p.jsUsageLabel.Text, "JavaScript: 2.0 KB / 3.9 KB")
	assert.Contains(t, p.globalUsageLabel.Text, "Global Heap: 4.9 KB / 9.8 KB")

	// Check progress bars are visible and show correct ratios
	assert.True(t, p.domProgress.Visible())
	assert.Equal(t, 0.5, p.domProgress.Value)

	assert.True(t, p.layoutProgress.Visible())
	assert.Equal(t, 0.75, p.layoutProgress.Value)

	assert.True(t, p.imagesProgress.Visible())
	assert.InDelta(t, 0.333, p.imagesProgress.Value, 0.005)

	assert.True(t, p.jsProgress.Visible())
	assert.Equal(t, 0.5, p.jsProgress.Value)

	assert.True(t, p.globalProgress.Visible())
	assert.Equal(t, 0.5, p.globalProgress.Value)
}

func TestMemoryPanel_GCButtonTrigger(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	gcCalled := false
	p := newMemoryPanel(nil).(*memoryPanel)
	p.onGC = func() {
		gcCalled = true
	}

	// Trigger Tapped
	p.gcBtn.Tapped(&fyne.PointEvent{})
	assert.True(t, gcCalled)
}

func TestMemoryPanel_Visual(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	cfg := memory.Config{
		Limits: map[memory.Component]uint64{
			memory.ComponentDOM:    1024 * 1024,     // 1MB
			memory.ComponentLayout: 2 * 1024 * 1024, // 2MB
			memory.ComponentImage:  4 * 1024 * 1024, // 4MB
			memory.ComponentScript: 8 * 1024 * 1024, // 8MB
		},
		GlobalLimit: 16 * 1024 * 1024, // 16MB
	}
	mgr := memory.NewManager(cfg)
	mgr.UpdateUsage(memory.ComponentDOM, 256*1024)
	mgr.UpdateUsage(memory.ComponentLayout, 1024*1024)
	mgr.UpdateUsage(memory.ComponentImage, 3*1024*1024)
	mgr.UpdateUsage(memory.ComponentScript, 2*1024*1024)

	ctx := &TabContext{
		Memory: mgr,
	}

	w := app.NewWindow("Memory — visual audit")
	w.Resize(fyne.NewSize(600, 500))
	p := newMemoryPanel(func() *TabContext { return ctx }).(*memoryPanel)
	w.SetContent(&p.Container)
	w.Show()

	p.RefreshFrom(ctx)
	w.Canvas().Refresh(w.Content())
	time.Sleep(100 * time.Millisecond)

	img := w.Canvas().Capture()
	require.NotNil(t, img)

	root := moduleRoot(t)
	for _, dir := range []string{
		filepath.Join(root, "screenshots"),
		filepath.Join(root, ".gstack", "artifacts"),
	} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		path := filepath.Join(dir, "memory_panel.png")
		f, err := os.Create(path)
		require.NoError(t, err)
		require.NoError(t, png.Encode(f, img.(image.Image)))
		_ = f.Close()
	}
}
