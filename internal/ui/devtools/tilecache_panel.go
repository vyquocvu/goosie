package devtools

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/memory"
)

type tileCachePanel struct {
	fyne.Container
	label *widget.Label
}

func newTileCachePanelContent(activeTab func() *TabContext) fyne.CanvasObject {
	p := &tileCachePanel{
		label: widget.NewLabel("No tile cache data available."),
	}
	p.label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		ctx := activeTab()
		if ctx == nil {
			p.label.SetText("No active tab.")
			return
		}

		var b strings.Builder
		b.WriteString("Tile Cache Infrastructure\n\n")
		b.WriteString("  TileCache:    Available (internal/renderer/frame/compositor/tiles.go)\n")
		b.WriteString("  GlyphCache:   Available (internal/renderer/frame/cache/cache.go)\n")
		b.WriteString("  ImageCache:   Available (internal/renderer/frame/cache/cache.go)\n")
		b.WriteString("  IntrinsicSize: Available (internal/renderer/intrinsic_size_cache.go)\n\n")

		b.WriteString("Status:\n")
		if ctx.Memory != nil {
			stats := ctx.Memory.Stats()
			tileLimit, hasTileLimit := stats.Limits[memory.ComponentTile]
			if hasTileLimit && tileLimit > 0 {
				b.WriteString(fmt.Sprintf("  Tile Budget:   %s\n", formatBytes(int64(tileLimit))))
			} else {
				b.WriteString("  Tile Budget:   unlimited\n")
			}
		}
		b.WriteString(fmt.Sprintf("  Render Tree:   %s\n", map[bool]string{true: "Yes", false: "No"}[ctx.Renderer != nil]))
		b.WriteString("\n")
		b.WriteString("Note: Tile caching is infrastructure-ready but not yet\n")
		b.WriteString("integrated into the document rendering pipeline.\n")
		b.WriteString("It activates when the compositor-based rendering path\n")
		b.WriteString("is wired into RenderWithViewport.\n")
		p.label.SetText(b.String())
	})

	topBar := container.NewBorder(nil, nil, refreshBtn, nil,
		widget.NewLabelWithStyle("Tile Cache Inspector", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	return container.NewBorder(topBar, nil, nil, nil, container.NewScroll(p.label))
}
