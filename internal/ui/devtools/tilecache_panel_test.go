package devtools

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/memory"
)

func TestTileCachePanel_InitialState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newTileCachePanelContent(func() *TabContext { return &TabContext{} })
	assert.NotNil(t, p)
}

func TestTileCachePanel_WithMemoryManager(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	cfg := memory.Config{
		Limits: map[memory.Component]uint64{
			memory.ComponentTile: 1024 * 1024,
		},
	}
	mgr := memory.NewManager(cfg)
	ctx := &TabContext{Memory: mgr}
	p := newTileCachePanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}

func TestTileCachePanel_WithRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		Renderer: &mockRendererProvider{
			summary:  map[string]int{},
			commands: nil,
		},
	}
	p := newTileCachePanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}

func TestTileCachePanel_UnlimitedBudget(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mgr := memory.NewManager(memory.Config{})
	ctx := &TabContext{Memory: mgr}
	p := newTileCachePanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}
