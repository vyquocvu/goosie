package devtools

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/memory"
)

type memoryPanel struct {
	fyne.Container
	activeTab func() *TabContext
	mu        sync.Mutex

	// Graphical progress bars and labels for major sectors
	domProgress      *widget.ProgressBar
	domUsageLabel    *widget.Label
	layoutProgress   *widget.ProgressBar
	layoutUsageLabel *widget.Label
	imagesProgress   *widget.ProgressBar
	imagesUsageLabel *widget.Label
	jsProgress       *widget.ProgressBar
	jsUsageLabel     *widget.Label
	globalProgress   *widget.ProgressBar
	globalUsageLabel *widget.Label

	// Detailed memory dump label
	detailsLabel *widget.Label

	gcBtn      *widget.Button
	refreshBtn *widget.Button

	// Hook for intercepting GC in testing
	onGC func()
}

func newMemoryPanel(activeTab func() *TabContext) fyne.CanvasObject {
	p := &memoryPanel{
		activeTab: activeTab,
	}

	p.refreshBtn = widget.NewButton("Refresh", func() {
		if p.activeTab != nil {
			ctx := p.activeTab()
			if ctx != nil {
				p.RefreshFrom(ctx)
			}
		}
	})

	p.gcBtn = widget.NewButton("Force GC", func() {
		if p.onGC != nil {
			p.onGC()
		} else {
			runtime.GC()
		}
		if p.activeTab != nil {
			ctx := p.activeTab()
			if ctx != nil {
				p.RefreshFrom(ctx)
			}
		}
	})

	// Create sub-widgets
	p.domUsageLabel = widget.NewLabel("DOM: 0 B / unlimited")
	p.domUsageLabel.TextStyle = fyne.TextStyle{Bold: true}
	p.domProgress = widget.NewProgressBar()
	p.domProgress.Min = 0
	p.domProgress.Max = 1.0

	p.layoutUsageLabel = widget.NewLabel("Layout: 0 B / unlimited")
	p.layoutUsageLabel.TextStyle = fyne.TextStyle{Bold: true}
	p.layoutProgress = widget.NewProgressBar()
	p.layoutProgress.Min = 0
	p.layoutProgress.Max = 1.0

	p.imagesUsageLabel = widget.NewLabel("Images & Tiles: 0 B / unlimited")
	p.imagesUsageLabel.TextStyle = fyne.TextStyle{Bold: true}
	p.imagesProgress = widget.NewProgressBar()
	p.imagesProgress.Min = 0
	p.imagesProgress.Max = 1.0

	p.jsUsageLabel = widget.NewLabel("JavaScript: 0 B / unlimited")
	p.jsUsageLabel.TextStyle = fyne.TextStyle{Bold: true}
	p.jsProgress = widget.NewProgressBar()
	p.jsProgress.Min = 0
	p.jsProgress.Max = 1.0

	p.globalUsageLabel = widget.NewLabel("Global Heap: 0 B / unlimited")
	p.globalUsageLabel.TextStyle = fyne.TextStyle{Bold: true}
	p.globalProgress = widget.NewProgressBar()
	p.globalProgress.Min = 0
	p.globalProgress.Max = 1.0

	p.detailsLabel = widget.NewLabel("No detailed memory data.")
	p.detailsLabel.TextStyle.Monospace = true
	p.detailsLabel.Wrapping = fyne.TextWrapWord

	// Build the visual sectors container
	sectors := container.NewVBox(
		widget.NewLabelWithStyle("Global Allocation Status", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		p.globalUsageLabel,
		p.globalProgress,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Memory Sectors", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		p.domUsageLabel,
		p.domProgress,
		p.layoutUsageLabel,
		p.layoutProgress,
		p.imagesUsageLabel,
		p.imagesProgress,
		p.jsUsageLabel,
		p.jsProgress,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Component Breakdown", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		p.detailsLabel,
	)

	topBar := container.NewBorder(nil, nil,
		container.NewHBox(p.refreshBtn, p.gcBtn), nil,
		widget.NewLabelWithStyle("Memory Manager", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	content := container.NewBorder(topBar, nil, nil, nil, container.NewScroll(sectors))
	p.Container = *content

	// Run initial refresh
	if activeTab != nil {
		p.RefreshFrom(activeTab())
	} else {
		p.RefreshFrom(nil)
	}

	return p
}

func (p *memoryPanel) RefreshFrom(ctx *TabContext) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ctx == nil || ctx.Memory == nil {
		p.detailsLabel.SetText("Memory manager not available.")
		p.domProgress.Hide()
		p.layoutProgress.Hide()
		p.imagesProgress.Hide()
		p.jsProgress.Hide()
		p.globalProgress.Hide()
		return
	}

	stats := ctx.Memory.Stats()

	// Update DOM Memory
	domUsage := stats.Usage[memory.ComponentDOM]
	domLimit := stats.Limits[memory.ComponentDOM]
	p.updateSector(p.domUsageLabel, p.domProgress, "DOM", domUsage, domLimit)

	// Update Layout Memory (sum ComponentLayout and ComponentLayoutIntrinsicSize)
	layoutUsage := stats.Usage[memory.ComponentLayout] + stats.Usage[memory.ComponentLayoutIntrinsicSize]
	layoutLimit := stats.Limits[memory.ComponentLayout]
	p.updateSector(p.layoutUsageLabel, p.layoutProgress, "Layout", layoutUsage, layoutLimit)

	// Update Images & Tiles Memory
	imagesUsage := stats.Usage[memory.ComponentImage] + stats.Usage[memory.ComponentGlyph] + stats.Usage[memory.ComponentTile]
	imagesLimit := stats.Limits[memory.ComponentImage]
	p.updateSector(p.imagesUsageLabel, p.imagesProgress, "Images & Tiles", imagesUsage, imagesLimit)

	// Update JS Memory
	jsUsage := stats.Usage[memory.ComponentScript]
	jsLimit := stats.Limits[memory.ComponentScript]
	p.updateSector(p.jsUsageLabel, p.jsProgress, "JavaScript", jsUsage, jsLimit)

	// Update Global Memory
	p.updateSector(p.globalUsageLabel, p.globalProgress, "Global Heap", stats.TotalUsage, stats.GlobalLimit)

	// Format Component Breakdown
	var b strings.Builder
	for _, comp := range memoryDefaultOrder() {
		usage, hasUsage := stats.Usage[comp]
		limit, hasLimit := stats.Limits[comp]
		if !hasUsage && !hasLimit {
			continue
		}
		usageStr := "0 B"
		if hasUsage {
			usageStr = formatBytes(int64(usage))
		}
		limitStr := "unlimited"
		if hasLimit && limit > 0 {
			limitStr = formatBytes(int64(limit))
		}
		b.WriteString(fmt.Sprintf("  %-24s %s / %s\n", string(comp), usageStr, limitStr))
	}
	p.detailsLabel.SetText(b.String())
}

func (p *memoryPanel) updateSector(label *widget.Label, progress *widget.ProgressBar, name string, usage, limit uint64) {
	usageStr := formatBytes(int64(usage))
	limitStr := "unlimited"
	if limit > 0 {
		limitStr = formatBytes(int64(limit))
	}
	label.SetText(fmt.Sprintf("%s: %s / %s", name, usageStr, limitStr))

	if limit > 0 {
		progress.Show()
		val := float64(usage) / float64(limit)
		if val > 1.0 {
			val = 1.0
		}
		progress.SetValue(val)
	} else {
		progress.Hide()
	}
}
