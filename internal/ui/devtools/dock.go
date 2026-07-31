package devtools

import (
	"fmt"
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

// memoryProvider abstracts access to memory manager stats.
// The concrete implementation is *memory.Manager.
type memoryProvider interface {
	Stats() memory.Stats
}

// jsRuntimeProvider abstracts access to JS runtime statistics.
// The concrete implementation is *js.Runtime.
type jsRuntimeProvider interface {
	ActiveTimersCount() int
	GetConsoleMessages() []js.ConsoleMessage
	GetJavaScriptErrors() []string
	RunningScriptCount() int
}

// SecurityInfo holds TLS certificate details for the Security panel.
type SecurityInfo struct {
	Scheme    string // "https" or "http"
	Subject   string // TLS certificate subject
	Issuer    string // TLS certificate issuer
	NotBefore string // certificate validity start
	NotAfter  string // certificate validity end
}

type A11yNode struct {
	Role        string
	Name        string
	Tag         string
	Description string
	Children    []*A11yNode
}

type accessibilityProvider interface {
	GetAccessibilityTree() []*A11yNode
}

type TabContext struct {
	Memory          memoryProvider
	Renderer        rendererProvider
	JSRuntime       jsRuntimeProvider
	RawSource       string
	RequestLog      requestLogProvider
	Storage         storageProvider
	CurrentURL      string
	SecuritySummary string
	SecurityInfo    SecurityInfo
	Settings        settingsProvider
	MetricsRecorder metricsProvider
	SourceCache     sourceCacheProvider
	RenderStats     map[string]time.Duration // render timing percentiles
	Accessibility   accessibilityProvider
}

// sourceCacheProvider exposes cached response bodies to the Sources panel so
// sub-resource content can be displayed without issuing new network requests.
type sourceCacheProvider interface {
	CachedBody(rawURL string) (string, bool)
}

type settingsProvider interface {
	GetHomepage() string
	GetDefaultSearchEngine() string
	GetEnableJavaScript() bool
	GetEnableImages() bool
	// SetHomepage updates the homepage URL. Implementations must
	// persist the value so it survives a browser restart.
	SetHomepage(url string)
	// SetDefaultSearchEngine updates the default search engine.
	SetDefaultSearchEngine(url string)
	// SetEnableJavaScript toggles JavaScript execution.
	SetEnableJavaScript(enabled bool)
	// SetEnableImages toggles image loading.
	SetEnableImages(enabled bool)
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

// TimingPhase represents one phase of a network request's lifecycle.
type TimingPhase struct {
	Name     string        `json:"name"`
	Duration time.Duration `json:"duration"`
}

// standard phase names used in waterfall rendering
const (
	PhaseDNS      = "DNS"
	PhaseConnect  = "Connect"
	PhaseTLS      = "TLS"
	PhaseRequest  = "Request"
	PhaseResponse = "Response"
	PhaseDownload = "Download"
)

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

	TimingPhases []TimingPhase `json:"timing_phases,omitempty"`
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

// EnsureTabs populates the dock with every registered DevTools tab if it is
// currently empty. Safe to call multiple times; callers wire Elements/Console
// content after invoking this in headless contexts.
func (d *Dock) EnsureTabs() {
	if len(d.tabs.Items) > 0 {
		return
	}
	d.addAllTabs()
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
	d.addTab("Accessibility", newAccessibilityPanel(d.activeTab))
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

// ActiveTabContext returns the current tab context from the dock's active
// tab callback, or nil if none is registered.
func (d *Dock) ActiveTabContext() *TabContext {
	if d == nil || d.activeTab == nil {
		return nil
	}
	return d.activeTab()
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
	return newSourcesPanel(activeTab)
}

func newDisplayListPanel(activeTab func() *TabContext) fyne.CanvasObject {
	return newDisplayListPanelContent(activeTab)
}

func newScriptQueuePanel(activeTab func() *TabContext) fyne.CanvasObject {
	return newScriptQueuePanelContent(activeTab)
}

func newTileCachePanel(activeTab func() *TabContext) fyne.CanvasObject {
	return newTileCachePanelContent(activeTab)
}

func newStoragePanel(activeTab func() *TabContext) fyne.CanvasObject {
	return newStoragePanelContent(activeTab)
}
