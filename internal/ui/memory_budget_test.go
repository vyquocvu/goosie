package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/memory"
)

func TestBrowserMemoryButtonCreated(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.memoryButton)
	assert.Equal(t, "Mem", browser.memoryButton.Text)
}

func TestBrowserShowMemoryDialogNoManager(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	browser.showMemoryDialog()
}

func TestBrowserShowMemoryDialogWithManager(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	mgr := memory.NewManager(memory.Config{
		GlobalLimit: 512_000_000,
		Limits: map[memory.Component]uint64{
			memory.ComponentDOM: 100_000_000,
		},
	})
	mgr.UpdateUsage(memory.ComponentDOM, 50_000_000)
	mgr.UpdateUsage(memory.ComponentStyle, 10_000_000)

	browser.deps.Memory = mgr
	browser.showMemoryDialog()
}

func TestFormatMemoryStats(t *testing.T) {
	stats := memory.Stats{
		Limits: map[memory.Component]uint64{
			memory.ComponentDOM: 100_000_000,
		},
		Usage: map[memory.Component]uint64{
			memory.ComponentDOM:   50_000_000,
			memory.ComponentStyle: 10_000_000,
		},
		GlobalLimit: 512_000_000,
		TotalUsage:  60_000_000,
	}

	output := formatMemoryStats(stats)

	assert.Contains(t, output, "Global Budget")
	assert.Contains(t, output, "60.00 MB")
	assert.Contains(t, output, "512.00 MB")
	assert.Contains(t, output, "Per-Component Budgets")
	assert.Contains(t, output, "50.00 MB")
	assert.Contains(t, output, "10.00 MB")
}

func TestFormatMemoryStatsNoLimits(t *testing.T) {
	stats := memory.Stats{
		Usage:       map[memory.Component]uint64{memory.ComponentDOM: 50_000_000},
		GlobalLimit: 0,
		TotalUsage:  50_000_000,
	}

	output := formatMemoryStats(stats)

	assert.Contains(t, output, "unlimited")
	assert.NotContains(t, output, "512")
}

func TestFormatMemoryStatsEmpty(t *testing.T) {
	stats := memory.Stats{
		Limits:      map[memory.Component]uint64{},
		Usage:       map[memory.Component]uint64{},
		GlobalLimit: 0,
		TotalUsage:  0,
	}

	output := formatMemoryStats(stats)

	assert.Contains(t, output, "Global Budget")
	assert.Contains(t, output, "0 B")
	assert.Contains(t, output, "Per-Component Budgets")
}

func TestMemoryButtonInNavBar(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.memoryButton)
	assert.Equal(t, "Mem", browser.memoryButton.Text)

	browser.memoryButton.OnTapped()
}

func TestMemoryFormatUsesFormatBytes(t *testing.T) {
	assert.Equal(t, "1.00 KB", formatBytes(1000))
	assert.Equal(t, "1.00 MB", formatBytes(1000*1000))
	assert.Equal(t, "1.00 GB", formatBytes(1000*1000*1000))
}
