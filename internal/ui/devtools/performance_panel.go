package devtools

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/engine/metrics"
)

type performancePanel struct {
	fyne.Container
	label   *widget.Label
	content *fyne.Container
}

func newPerformancePanel(activeTab func() *TabContext) fyne.CanvasObject {
	p := &performancePanel{
		label:   widget.NewLabel("No performance data available."),
		content: container.NewVBox(),
	}
	p.label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		if activeTab != nil {
			ctx := activeTab()
			if ctx != nil {
				p.RefreshFrom(ctx)
			}
		}
	})

	topBar := container.NewBorder(nil, nil, refreshBtn,
		widget.NewLabelWithStyle("Performance", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	scrollContent := container.NewVScroll(p.content)
	outer := container.NewBorder(topBar, nil, nil, nil, scrollContent)
	p.Container = *outer
	return p
}

func (p *performancePanel) RefreshFrom(ctx *TabContext) {
	if ctx == nil || ctx.MetricsRecorder == nil {
		p.label.SetText("No performance data available.")
		return
	}
	m := ctx.MetricsRecorder.Snapshot()
	p.renderMetrics(m, ctx.RenderStats)
}

func (p *performancePanel) renderMetrics(m metrics.Metrics, renderStats map[string]time.Duration) {
	var b strings.Builder

	if m.URL != "" && m.URL != "live" {
		b.WriteString(fmt.Sprintf("Navigation #%d — %s\n\n", m.NavID, m.URL))
	} else {
		b.WriteString(fmt.Sprintf("Navigation #%d\n\n", m.NavID))
	}

	total := m.TotalDuration()
	b.WriteString(fmt.Sprintf("Total: %s\n\n", formatDuration(time.Duration(total))))

	// Phase timings as a bar chart
	if len(m.Timings) > 0 {
		b.WriteString("Phase Timings\n")
		b.WriteString("─────────────\n")
		maxDuration := time.Duration(0)
		for _, t := range m.Timings {
			d := t.Duration()
			if d > maxDuration {
				maxDuration = d
			}
		}
		maxBar := 30.0
		for _, t := range m.Timings {
			d := t.Duration()
			bars := int((float64(d) / float64(maxDuration)) * maxBar)
			if bars < 1 && d > 0 {
				bars = 1
			}
			bar := strings.Repeat("█", bars)
			pct := 0.0
			if total > 0 {
				pct = (float64(d) / float64(total)) * 100
			}
			b.WriteString(fmt.Sprintf("  %-14s %s %8s (%5.1f%%)\n",
				humanPhaseLabel(t.Phase.String()),
				bar,
				formatDuration(d),
				pct,
			))
		}
		b.WriteString("\n")
	}

	// Counters
	b.WriteString("Counters\n")
	b.WriteString("────────\n")
	b.WriteString(fmt.Sprintf("  DOM Nodes:          %d\n", m.Counters.NodeCount))
	b.WriteString(fmt.Sprintf("  CSS Rules:          %d\n", m.Counters.RuleCount))
	b.WriteString(fmt.Sprintf("  Selectors:          %d\n", m.Counters.SelectorCount))
	b.WriteString(fmt.Sprintf("  Layout Boxes:       %d\n", m.Counters.BoxCount))
	b.WriteString(fmt.Sprintf("  Display Items:      %d\n", m.Counters.DisplayItemCount))
	b.WriteString(fmt.Sprintf("  Images Decoded:     %d\n", m.Counters.ImageCount))
	b.WriteString(fmt.Sprintf("  Tiles:              %d\n", m.Counters.TileCount))
	b.WriteString(fmt.Sprintf("  Bytes Downloaded:   %s\n", formatBytes(m.Counters.BytesDownloaded)))
	b.WriteString(fmt.Sprintf("  Decoded Image Bytes: %s\n", formatBytes(m.Counters.DecodedImageBytes)))
	b.WriteString(fmt.Sprintf("  Cache Hits:         %d\n", m.Counters.CacheHits))
	b.WriteString(fmt.Sprintf("  Cache Misses:       %d\n", m.Counters.CacheMisses))
	b.WriteString(fmt.Sprintf("  Script Errors:      %d\n", m.Counters.ScriptErrors))

	// Render timing percentiles
	if len(renderStats) > 0 {
		b.WriteString("\nRender Timing\n")
		b.WriteString("─────────────\n")
		for _, key := range []string{"RenderHTML_p50", "RenderHTML_p95", "RenderHTML_p99",
			"ComputeLayout_p50", "ComputeLayout_p95", "ComputeLayout_p99",
			"RenderWithViewport_p50", "RenderWithViewport_p95", "RenderWithViewport_p99"} {
			if d, ok := renderStats[key]; ok {
				label := strings.ReplaceAll(key, "_", " ")
				b.WriteString(fmt.Sprintf("  %-24s %s\n", label, formatDuration(d)))
			}
		}
	}

	p.label.SetText(b.String())
	p.label.TextStyle.Monospace = true
	p.label.Refresh()
}

func humanPhaseLabel(s string) string {
	switch s {
	case "dns_resolve":
		return "DNS Resolve"
	case "first_byte":
		return "First Byte"
	case "body_read":
		return "Body Read"
	default:
		if len(s) > 0 {
			return strings.ToUpper(s[:1]) + s[1:]
		}
		return s
	}
}
