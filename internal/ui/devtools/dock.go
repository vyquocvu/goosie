package devtools

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/memory"
	"github.com/vyquocvu/goosie/internal/renderer"
)

type Dock struct {
	container *fyne.Container
	tabs      *container.AppTabs
	visible   bool

	activeTab func() *TabContext
}

type TabContext struct {
	Memory          *memory.Manager
	Renderer        rendererProvider
	JSRuntime       *js.Runtime
	RawSource       string
	RequestLog      requestLogProvider
	Storage         storageProvider
	CurrentURL      string
	SecuritySummary string
	Settings        settingsProvider
	MetricsRecorder metricsProvider
}

type settingsProvider interface {
	GetHomepage() string
	GetDefaultSearchEngine() string
	GetEnableJavaScript() bool
	GetEnableImages() bool
}

type metricsProvider interface {
	Snapshot() metrics.Metrics
}

type storageProvider interface {
	Snapshot() map[string]map[string]string
	Set(origin, key, value string) error
	Remove(origin, key string) error
	Clear(origin string) error
}

type requestLogProvider interface {
	Entries() []NetRequestEntry
}

// NetRequestEntry is a snapshot of one network request.
// Exported so the ui package can wrap goosienet.RequestLogEntry values.
type NetRequestEntry struct {
	Method      string
	URL         string
	Status      int
	ContentType string
	Bytes       int64
	CacheHit    bool
	Error       string
	StartedAt   time.Time
	Duration    time.Duration
}

type rendererProvider interface {
	GetDisplayListSummary() map[string]int
	GetDisplayListCommands() []renderer.PaintCommand
	GetDOMNodeCounts() (total int, elements int, text int)
	GetLayoutNodeCount() int
	GetRoot() *renderer.RenderNode
	DirtyOverlayEnabled() bool
	SetDirtyOverlayEnabled(bool)
	Refresh()
}

func NewDock(activeTab func() *TabContext, headless ...bool) *Dock {
	h := len(headless) > 0 && headless[0]
	d := &Dock{
		activeTab: activeTab,
	}
	d.buildTabs()
	d.tabs = container.NewAppTabs()
	if !h {
		d.addAllTabs()
	}
	return d
}

func (d *Dock) buildTabs() {
}

func (d *Dock) addAllTabs() {
	d.addTab("Elements", newElementsPanel(d.activeTab))
	d.addTab("Console", newConsolePanel(d.activeTab))
	d.addTab("Sources", newSourcePanel(d.activeTab))
	d.addTab("Network", newNetworkPanel(d.activeTab))
	d.addTab("Performance", newPerformancePanel(d.activeTab))
	d.addTab("Memory", newMemoryPanel(d.activeTab))
	d.addTab("Storage", newStoragePanel(d.activeTab))
	d.addTab("Security", newSecurityPanel(d.activeTab))
	d.addTab("Settings", newSettingsPanel(d.activeTab))
	d.addTab("Display List", newDisplayListPanel(d.activeTab))
	d.addTab("Script Queue", newScriptQueuePanel(d.activeTab))
	d.addTab("Tile Cache", newTileCachePanel(d.activeTab))
}

func (d *Dock) addTab(title string, content fyne.CanvasObject) {
	d.tabs.Append(container.NewTabItem(title, content))
}

func (d *Dock) CanvasObject() fyne.CanvasObject {
	if d.container == nil {
		d.container = container.NewMax(d.tabs)
	}
	return d.container
}

func (d *Dock) Visible() bool {
	return d.visible
}

func (d *Dock) Show() {
	d.visible = true
}

func (d *Dock) Hide() {
	d.visible = false
}

func (d *Dock) SelectTab(title string) {
	for _, t := range d.tabs.Items {
		if t.Text == title {
			d.tabs.Select(t)
			return
		}
	}
}

// SetElementsContent replaces the content of the "Elements" tab with the given panel.
func (d *Dock) SetElementsContent(content fyne.CanvasObject) {
	for _, t := range d.tabs.Items {
		if t.Text == "Elements" {
			t.Content = content
			return
		}
	}
}

// SetConsoleContent replaces the content of the "Console" tab with the given panel.
func (d *Dock) SetConsoleContent(content fyne.CanvasObject) {
	for _, t := range d.tabs.Items {
		if t.Text == "Console" {
			t.Content = content
			return
		}
	}
}

func (d *Dock) Refresh() {
	ctx := d.activeTab()
	if ctx == nil {
		return
	}
	for _, item := range d.tabs.Items {
		if p, ok := item.Content.(refreshablePanel); ok {
			p.RefreshFrom(ctx)
		}
	}
}

type refreshablePanel interface {
	RefreshFrom(ctx *TabContext)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return "0ms"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.0fµs", float64(d)/float64(time.Microsecond))
	}
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func memoryDefaultOrder() []memory.Component {
	return []memory.Component{
		memory.ComponentDOM,
		memory.ComponentStyle,
		memory.ComponentLayout,
		memory.ComponentDisplayList,
		memory.ComponentTile,
		memory.ComponentImage,
		memory.ComponentGlyph,
		memory.ComponentScript,
		memory.ComponentNetworkCache,
		memory.ComponentPageCache,
		memory.ComponentLayoutIntrinsicSize,
	}
}

func newElementsPanel(activeTab func() *TabContext) fyne.CanvasObject {
	label := widget.NewLabel("Elements panel — select an element to inspect")
	label.Wrapping = fyne.TextWrapWord
	return container.NewBorder(widget.NewLabelWithStyle("Elements", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil, container.NewScroll(label))
}

func newConsolePanel(activeTab func() *TabContext) fyne.CanvasObject {
	messages := binding.NewStringList()
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Execute JavaScript")

	list := widget.NewListWithData(messages,
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			obj.(*widget.Label).Bind(item.(binding.String))
		},
	)

	clearBtn := widget.NewButton("Clear", func() {
		messages.Set([]string{})
	})

	topBar := container.NewBorder(nil, nil, clearBtn, nil,
		widget.NewLabelWithStyle("Console", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	content := container.NewBorder(container.NewBorder(nil, nil, nil, nil, topBar), entry, nil, nil, list)
	return content
}

func newNetworkPanel(activeTab func() *TabContext) fyne.CanvasObject {
	p := &networkPanel{}
	p.build()
	return p
}

func newSourcePanel(activeTab func() *TabContext) fyne.CanvasObject {
	entry := widget.NewMultiLineEntry()
	entry.Disable()
	entry.Wrapping = fyne.TextWrapOff

	refreshBtn := widget.NewButton("Refresh", func() {
		ctx := activeTab()
		if ctx == nil || ctx.RawSource == "" {
			entry.SetText("No source available — the page may not have loaded yet.")
			return
		}
		entry.SetText(ctx.RawSource)
	})

	topBar := container.NewBorder(nil, nil, refreshBtn, nil,
		widget.NewLabelWithStyle("Page Source", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	return container.NewBorder(topBar, nil, nil, nil, container.NewScroll(entry))
}

func newMemoryPanel(activeTab func() *TabContext) fyne.CanvasObject {
	label := widget.NewLabel("No memory data available yet.")
	label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		ctx := activeTab()
		if ctx == nil || ctx.Memory == nil {
			label.SetText("Memory manager not available.")
			return
		}

		stats := ctx.Memory.Stats()
		var b strings.Builder

		b.WriteString("Global Budget\n")
		b.WriteString(fmt.Sprintf("  Total Usage: %s / %s\n\n",
			formatBytes(int64(stats.TotalUsage)),
			formatBytes(int64(stats.GlobalLimit))))

		b.WriteString("Per-Component Budgets\n")
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
			b.WriteString(fmt.Sprintf("  %-20s  %s / %s\n", string(comp), usageStr, limitStr))
		}
		label.SetText(b.String())
	})

	topBar := container.NewBorder(nil, nil, refreshBtn, nil,
		widget.NewLabelWithStyle("Memory Budget", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	return container.NewBorder(topBar, nil, nil, nil, container.NewScroll(label))
}

func newDisplayListPanel(activeTab func() *TabContext) fyne.CanvasObject {
	label := widget.NewLabel("No display list built yet.")
	label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		ctx := activeTab()
		if ctx == nil || ctx.Renderer == nil {
			label.SetText("No renderer available.")
			return
		}

		summary := ctx.Renderer.GetDisplayListSummary()
		cmds := ctx.Renderer.GetDisplayListCommands()
		if len(cmds) == 0 {
			label.SetText("No display list built yet.")
			return
		}

		total := 0
		for _, c := range summary {
			total += c
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("Total: %d commands\n\n", total))
		for _, name := range displayListTypeOrder() {
			count, ok := summary[name]
			if !ok {
				continue
			}
			b.WriteString(fmt.Sprintf("  %-12s %d\n", name+":", count))
		}

		b.WriteString("\n--- Command Details ---\n")
		for i, cmd := range cmds {
			line := fmt.Sprintf("%d. %s", i+1, cmd.Type.String())
			switch cmd.Type {
			case renderer.PaintText:
				txt := cmd.Text
				if len(txt) > 40 {
					txt = txt[:37] + "..."
				}
				line += fmt.Sprintf("  text=%q  font=%.0f  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
					txt, cmd.FontSize, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintRect:
				line += fmt.Sprintf("  pos=(%.0f,%.0f)  size=(%.0f×%.0f)", cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintImage:
				src := cmd.ImageSrc
				if len(src) > 40 {
					src = src[:37] + "..."
				}
				line += fmt.Sprintf("  src=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)", src, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintLink:
				line += fmt.Sprintf("  url=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)", cmd.LinkURL, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintBorder:
				line += fmt.Sprintf("  pos=(%.0f,%.0f)  size=(%.0f×%.0f)  stroke=%.0f",
					cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height, cmd.StrokeWidth)
			case renderer.PaintButton:
				line += fmt.Sprintf("  text=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
					cmd.ButtonText, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintInput:
				line += fmt.Sprintf("  type=%s  value=%s  placeholder=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
					cmd.InputType, cmd.InputValue, cmd.Placeholder, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintTextarea:
				line += fmt.Sprintf("  value=%s  placeholder=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
					cmd.InputValue, cmd.Placeholder, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PushClip:
				line += fmt.Sprintf("  pos=(%.0f,%.0f)  size=(%.0f×%.0f)  overflow=%s",
					cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height, cmd.ClipOverflow)
			case renderer.PopClip:
			}
			b.WriteString(line + "\n")
		}
		label.SetText(b.String())
	})

	topBar := container.NewBorder(nil, nil, refreshBtn, nil,
		widget.NewLabelWithStyle("Display List Inspector", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	return container.NewBorder(topBar, nil, nil, nil, container.NewScroll(label))
}

func displayListTypeOrder() []string {
	return []string{"Text", "Rect", "Image", "Link", "Border", "Button", "Input", "Textarea", "PushClip", "PopClip"}
}

func newScriptQueuePanel(activeTab func() *TabContext) fyne.CanvasObject {
	label := widget.NewLabel("No JavaScript runtime available.")
	label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		ctx := activeTab()
		if ctx == nil {
			label.SetText("No active tab.")
			return
		}
		rt := ctx.JSRuntime
		if rt == nil {
			label.SetText("No JavaScript runtime available.\n\nJavaScript task queue monitoring requires an active JS runtime on the current tab.")
			return
		}

		var b strings.Builder
		b.WriteString("JavaScript Task Queue\n\n")
		timers := rt.ActiveTimersCount()
		consoleCount := len(rt.GetConsoleMessages())
		errorCount := len(rt.GetJavaScriptErrors())

		b.WriteString(fmt.Sprintf("Active Timers (setTimeout/setInterval): %d\n", timers))
		b.WriteString(fmt.Sprintf("Console Messages:                      %d\n", consoleCount))
		b.WriteString(fmt.Sprintf("JavaScript Errors:                     %d\n", errorCount))
		b.WriteString(fmt.Sprintf("Script Running:                        %s\n", map[bool]string{true: "Yes", false: "No"}[rt.RunningScriptCount() > 0]))
		label.SetText(b.String())
	})

	topBar := container.NewBorder(nil, nil, refreshBtn, nil,
		widget.NewLabelWithStyle("Script Task Queue", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	return container.NewBorder(topBar, nil, nil, nil, container.NewScroll(label))
}

func newTileCachePanel(activeTab func() *TabContext) fyne.CanvasObject {
	label := widget.NewLabel("No tile cache data available.")
	label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		ctx := activeTab()
		if ctx == nil {
			label.SetText("No active tab.")
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
		label.SetText(b.String())
	})

	topBar := container.NewBorder(nil, nil, refreshBtn, nil,
		widget.NewLabelWithStyle("Tile Cache Inspector", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	return container.NewBorder(topBar, nil, nil, nil, container.NewScroll(label))
}

func newStoragePanel(activeTab func() *TabContext) fyne.CanvasObject {
	return newStoragePanelContent(activeTab)
}

type securityPanel struct {
	fyne.Container
	label *widget.Label
}

func newSecurityPanel(activeTab func() *TabContext) fyne.CanvasObject {
	p := &securityPanel{
		label: widget.NewLabel("No security information"),
	}
	p.label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		if activeTab != nil {
			ctx := activeTab()
			if ctx != nil {
				p.refreshFrom(ctx)
			}
		}
	})

	topBar := container.NewBorder(nil, nil, refreshBtn,
		widget.NewLabelWithStyle("Security", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	content := container.NewBorder(topBar, nil, nil, nil, container.NewScroll(p.label))
	p.Container = *content
	return p
}

func (p *securityPanel) RefreshFrom(ctx *TabContext) {
	if ctx != nil {
		p.refreshFrom(ctx)
	}
}

func (p *securityPanel) refreshFrom(ctx *TabContext) {
	var b strings.Builder
	b.WriteString("Current Page\n\n")

	if ctx.CurrentURL != "" {
		b.WriteString(fmt.Sprintf("  URL:     %s\n", ctx.CurrentURL))
		if strings.HasPrefix(ctx.CurrentURL, "https://") {
			b.WriteString("  Protocol: HTTPS (encrypted)\n")
		} else if strings.HasPrefix(ctx.CurrentURL, "http://") {
			b.WriteString("  Protocol: HTTP (unencrypted)\n")
		}
	} else {
		b.WriteString("  No page loaded.\n")
	}

	if ctx.SecuritySummary != "" {
		b.WriteString(fmt.Sprintf("  Summary: %s\n", ctx.SecuritySummary))
	}

	b.WriteString("\nCertificate\n\n")
	b.WriteString("  Certificate chain inspection is available\n")
	b.WriteString("  for HTTPS pages with TLS connections.\n")

	p.label.SetText(b.String())
	p.label.Refresh()
}

type settingsPanel struct {
	fyne.Container
	label *widget.Label
}

func newSettingsPanel(activeTab func() *TabContext) fyne.CanvasObject {
	p := &settingsPanel{
		label: widget.NewLabel("No settings available"),
	}
	p.label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		if activeTab != nil {
			ctx := activeTab()
			if ctx != nil {
				p.refreshFrom(ctx)
			}
		}
	})

	topBar := container.NewBorder(nil, nil, refreshBtn,
		widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	content := container.NewBorder(topBar, nil, nil, nil, container.NewScroll(p.label))
	p.Container = *content
	return p
}

func (p *settingsPanel) RefreshFrom(ctx *TabContext) {
	if ctx != nil {
		p.refreshFrom(ctx)
	}
}

func (p *settingsPanel) refreshFrom(ctx *TabContext) {
	if ctx.Settings == nil {
		p.label.SetText("No settings provider available.")
		return
	}
	s := ctx.Settings
	var b strings.Builder
	b.WriteString("Browser Settings\n\n")
	b.WriteString(fmt.Sprintf("  Homepage:            %s\n", s.GetHomepage()))
	b.WriteString(fmt.Sprintf("  Default Search:     %s\n", s.GetDefaultSearchEngine()))
	b.WriteString(fmt.Sprintf("  JavaScript:         %s\n", map[bool]string{true: "Enabled", false: "Disabled"}[s.GetEnableJavaScript()]))
	b.WriteString(fmt.Sprintf("  Images:             %s\n", map[bool]string{true: "Enabled", false: "Disabled"}[s.GetEnableImages()]))
	b.WriteString("\nChanges are applied immediately and persisted\nto the profile on browser restart.\n")
	p.label.SetText(b.String())
	p.label.Refresh()
}
