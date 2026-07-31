package devtools

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/memory"
)

// tileCachePanel surfaces the engine's cache and memory budget
// status. The previous implementation printed everything to a
// single label; this version uses a small grid of metric cards so
// the user can scan the cache hit/eviction counters and the
// per-component budget usage at a glance.
//
// Each card is one row of a 2-column grid. The metric values
// are bound to a refreshable source so polling the active tab
// re-renders the dashboard without rebuilding the widget tree.
type tileCachePanel struct {
	fyne.Container

	// Bound labels: each card renders one of these.
	tilesLabel  *widget.Label
	hitsLabel   *widget.Label
	missLabel   *widget.Label
	evictLabel  *widget.Label
	intrinsicLabel *widget.Label
	budgetLabel *widget.Label
	imageLabel  *widget.Label
	glyphLabel  *widget.Label

	// Static informational note.
	noteLabel *widget.Label
}

func newTileCachePanelContent(activeTab func() *TabContext) fyne.CanvasObject {
	tilesBinding := binding.NewString()
	hitsBinding := binding.NewString()
	missBinding := binding.NewString()
	evictBinding := binding.NewString()
	intrinsicBinding := binding.NewString()
	budgetBinding := binding.NewString()
	imageBinding := binding.NewString()
	glyphBinding := binding.NewString()

	tilesLabel := widget.NewLabelWithData(tilesBinding)
	hitsLabel := widget.NewLabelWithData(hitsBinding)
	missLabel := widget.NewLabelWithData(missBinding)
	evictLabel := widget.NewLabelWithData(evictBinding)
	intrinsicLabel := widget.NewLabelWithData(intrinsicBinding)
	budgetLabel := widget.NewLabelWithData(budgetBinding)
	imageLabel := widget.NewLabelWithData(imageBinding)
	glyphLabel := widget.NewLabelWithData(glyphBinding)

	p := &tileCachePanel{
		tilesLabel:     tilesLabel,
		hitsLabel:      hitsLabel,
		missLabel:      missLabel,
		evictLabel:     evictLabel,
		intrinsicLabel: intrinsicLabel,
		budgetLabel:    budgetLabel,
		imageLabel:     imageLabel,
		glyphLabel:     glyphLabel,
		noteLabel:      widget.NewLabel(""),
	}

	// Stat card helper: title (bold) above the bound value widget.
	makeCard := func(title string, value fyne.CanvasObject) fyne.CanvasObject {
		header := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		return container.NewBorder(header, nil, nil, nil, value)
	}

	// Two sections: cache counters (left) and memory budget (right).
	counters := container.NewGridWithColumns(2,
		makeCard("Tiles Built", tilesLabel),
		makeCard("Cache Hits", hitsLabel),
		makeCard("Cache Misses", missLabel),
		makeCard("Cache Evictions", evictLabel),
		makeCard("Intrinsic Sizes", intrinsicLabel),
		makeCard("Image Cache", imageLabel),
		makeCard("Glyph Cache", glyphLabel),
		makeCard("Memory Budget", budgetLabel),
	)

	// Notes panel: human-readable summary of which caches are
	// wired up and which are infrastructure-only. Lets a user
	// tell at a glance whether the tile cache is actively
	// engaged on the current document.
	p.noteLabel.Wrapping = fyne.TextWrapWord
	notesPanel := container.NewBorder(
		widget.NewLabelWithStyle("Cache Infrastructure", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		container.NewScroll(p.noteLabel),
	)

	refresh := func() {
		ctx := activeTab()
		if ctx == nil {
			setAllToUnknown(tilesBinding, hitsBinding, missBinding, evictBinding,
				intrinsicBinding, imageBinding, glyphBinding, budgetBinding)
			p.noteLabel.SetText("No active tab.")
			return
		}

		// Render tree status: is the renderer available? If yes,
		// how many nodes are in the current tree? This is the
		// primary indicator that tile building is happening.
		var (
			tileCount         int64 = -1
			hit, miss, tiles int64 = -1, -1, -1
		)
		if ctx.Renderer != nil {
			tileCount = int64(ctx.Renderer.GetLayoutNodeCount())
			if sm := snapshotMetrics(ctx); sm != nil {
				hit = int64(sm.Counters.CacheHits)
				miss = int64(sm.Counters.CacheMisses)
				tiles = int64(sm.Counters.TileCount)
			}
		}

		_ = tilesBinding.Set(formatCount(tileCount, "nodes"))
		_ = hitsBinding.Set(formatCount(hit, "hits"))
		_ = missBinding.Set(formatCount(miss, "misses"))
		_ = evictBinding.Set(formatCount(tiles, "tiles"))
		_ = intrinsicBinding.Set(formatCount(tileCount, "nodes"))

		// Memory budget: read the manager's per-component
		// limits so the user can see headroom on each cache.
		if ctx.Memory != nil {
			stats := ctx.Memory.Stats()
			_ = budgetBinding.Set(formatBudget(stats))
			_ = imageBinding.Set(formatLimit("Image", stats.Limits[memory.ComponentImage]))
			_ = glyphBinding.Set(formatLimit("Glyph", stats.Limits[memory.ComponentGlyph]))
		} else {
			_ = budgetBinding.Set("n/a")
			_ = imageBinding.Set("n/a")
			_ = glyphBinding.Set("n/a")
		}

		// Static note: which caches are wired up vs ready
		// but inactive. Kept short so the user can scan it.
		var b strings.Builder
		b.WriteString("TileCache:        ")
		if ctx.Renderer != nil {
			b.WriteString("active\n")
		} else {
			b.WriteString("inactive\n")
		}
		b.WriteString("GlyphCache:       available (internal/renderer/frame/cache/cache.go)\n")
		b.WriteString("ImageCache:       available (internal/renderer/frame/cache/cache.go)\n")
		b.WriteString("IntrinsicSizeCache: available (internal/renderer/intrinsic_size_cache.go)\n")
		b.WriteString("\nNote: Tile caching infrastructure is present in the\n")
		b.WriteString("codebase but is activated by the compositor rendering\n")
		b.WriteString("path. Tile counts reflect the layout tree node count\n")
		b.WriteString("rather than rasterized tile rects in the current\n")
		b.WriteString("rendering path.\n")
		p.noteLabel.SetText(b.String())
	}

	refreshBtn := widget.NewButton("Refresh", refresh)

	topBar := container.NewBorder(nil, nil, refreshBtn, nil,
		widget.NewLabelWithStyle("Tile Cache Inspector", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	body := container.NewVSplit(counters, notesPanel)

	p.Container = *container.NewBorder(topBar, nil, nil, nil, body)

	// Refresh once at construction so the panel shows current
	// values the moment the user opens it.
	refresh()
	return &p.Container
}

// setAllToUnknown sets every bound metric to a placeholder
// string. Used when the active tab context is missing so the
// panel shows "n/a" instead of stale values from a prior tab.
func setAllToUnknown(bindings ...binding.String) {
	for _, b := range bindings {
		_ = b.Set("n/a")
	}
}

// formatCount renders a counter as a human-readable string. The
// value of -1 means "unknown / not available" and renders as "n/a".
func formatCount(v int64, unit string) string {
	if v < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d %s", v, unit)
}

// formatLimit renders a per-component memory limit. A zero limit
// means "no bound"; the user sees "unbounded" instead of "0 B".
func formatLimit(name string, limit uint64) string {
	if limit == 0 {
		return name + ": unbounded"
	}
	return fmt.Sprintf("%s: %s", name, formatBytes(int64(limit)))
}

// formatBudget renders the entire memory.Stats blob as a compact
// multi-line summary covering the components relevant to the
// tile cache: tile, image cache, glyph cache.
func formatBudget(stats memory.Stats) string {
	var b strings.Builder
	b.WriteString("Per-component limits:\n")
	for _, comp := range []struct {
		name string
		key  memory.Component
	}{
		{"  Tile", memory.ComponentTile},
		{"  ImageCache", memory.ComponentImage},
		{"  GlyphCache", memory.ComponentGlyph},
		{"  PageCache", memory.ComponentPageCache},
		{"  LayoutIntrinsicSize", memory.ComponentLayoutIntrinsicSize},
	} {
		limit, ok := stats.Limits[comp.key]
		if !ok {
			fmt.Fprintf(&b, "%s: not configured\n", comp.name)
			continue
		}
		if limit == 0 {
			fmt.Fprintf(&b, "%s: unbounded\n", comp.name)
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", comp.name, formatBytes(int64(limit)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// snapshotMetrics pulls a metrics snapshot from the active tab
// context when one is available. Returns nil if no metrics
// recorder is wired so the caller can show "n/a".
func snapshotMetrics(ctx *TabContext) *metricsSnapshot {
	if ctx.MetricsRecorder == nil {
		return nil
	}
	s := ctx.MetricsRecorder.Snapshot()
	return &metricsSnapshot{Counters: s.Counters}
}

// metricsSnapshot wraps the metrics.Counters value with named
// fields the panel reads. We import the metrics package here
// rather than mirroring the struct because the canonical names
// must stay in sync with what the recorder writes.
type metricsSnapshot struct {
	Counters metrics.Counters
}