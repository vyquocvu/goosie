package devtools_test

import (
	"github.com/vyquocvu/goosie/internal/ui/devtools"
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

	p := devtools.NewMemoryPanel(nil).(*devtools.MemoryPanel)
	assert.NotNil(t, p)
	assert.Equal(t, "Memory manager not available.", p.DetailsLabel().Text)
	assert.False(t, p.DOMProgress().Visible())
	assert.False(t, p.LayoutProgress().Visible())
	assert.False(t, p.ImagesProgress().Visible())
	assert.False(t, p.JSProgress().Visible())
	assert.False(t, p.GlobalProgress().Visible())
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

	ctx := &devtools.TabContext{
		Memory: mgr,
	}

	p := devtools.NewMemoryPanel(func() *devtools.TabContext { return ctx }).(*devtools.MemoryPanel)
	p.RefreshFrom(ctx)

	// Check labels update
	assert.Contains(t, p.DOMUsageLabel().Text, "DOM: 500 B / 1000 B")
	assert.Contains(t, p.LayoutUsageLabel().Text, "Layout: 1.5 KB / 2.0 KB")
	assert.Contains(t, p.ImagesUsageLabel().Text, "Images & Tiles: 1000 B / 2.9 KB")
	assert.Contains(t, p.JSUsageLabel().Text, "JavaScript: 2.0 KB / 3.9 KB")
	assert.Contains(t, p.GlobalUsageLabel().Text, "Global Heap: 4.9 KB / 9.8 KB")

	// Check progress bars are visible and show correct ratios
	assert.True(t, p.DOMProgress().Visible())
	assert.Equal(t, 0.5, p.DOMProgress().Value)

	assert.True(t, p.LayoutProgress().Visible())
	assert.Equal(t, 0.75, p.LayoutProgress().Value)

	assert.True(t, p.ImagesProgress().Visible())
	assert.InDelta(t, 0.333, p.ImagesProgress().Value, 0.005)

	assert.True(t, p.JSProgress().Visible())
	assert.Equal(t, 0.5, p.JSProgress().Value)

	assert.True(t, p.GlobalProgress().Visible())
	assert.Equal(t, 0.5, p.GlobalProgress().Value)
}

func TestMemoryPanel_GCButtonTrigger(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	gcCalled := false
	p := devtools.NewMemoryPanel(nil).(*devtools.MemoryPanel)
	p.SetOnGC(func() {
		gcCalled = true
	})

	// Trigger Tapped
	p.GCBtn().Tapped(&fyne.PointEvent{})
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

	ctx := &devtools.TabContext{
		Memory: mgr,
	}

	w := app.NewWindow("Memory — visual audit")
	w.Resize(fyne.NewSize(600, 500))
	p := devtools.NewMemoryPanel(func() *devtools.TabContext { return ctx }).(*devtools.MemoryPanel)
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
