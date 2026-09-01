package devtools

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// Column index constants exported for use by external test packages.
const (
	ColMethod = colMethod
	ColStatus = colStatus
	ColURL    = colURL
	ColType   = colType
	ColSize   = colSize
	ColTime   = colTime
)

// AccessibilityPanel is the exported type alias for accessibilityPanel for use by external test packages.
type AccessibilityPanel = accessibilityPanel


// Label exports label field for use by external test packages.
func (p *accessibilityPanel) Label() *widget.Label { return p.label }

// NewAccessibilityPanel exports newAccessibilityPanel for use by external test packages.
var NewAccessibilityPanel = newAccessibilityPanel


// FormatA11yTree exports formatA11yTree for use by external test packages.
var FormatA11yTree = formatA11yTree

// WalkA11yNode exports walkA11yNode for use by external test packages.
var WalkA11yNode = walkA11yNode

// SourcesPanel is the exported type alias for sourcesPanel for use by external test packages.
type SourcesPanel = sourcesPanel

func (p *sourcesPanel) All() []sourceResource { return p.all }
func (p *sourcesPanel) SetAll(a []sourceResource) { p.all = a }
func (p *sourcesPanel) VisibleResources() []sourceResource { return p.visible }
func (p *sourcesPanel) SetVisibleResources(v []sourceResource) { p.visible = v }
func (p *sourcesPanel) StatusLabel() *widget.Label { return p.statusLabel }
func (p *sourcesPanel) SourceView() *widget.Label { return p.sourceView }
func (p *sourcesPanel) GutterView() *widget.Label { return p.gutterView }
func (p *sourcesPanel) DetailLabel() *widget.Label { return p.detailLabel }
func (p *sourcesPanel) FilterEntry() *widget.Entry { return p.filterEntry }
func (p *sourcesPanel) RefreshBtn() *widget.Button { return p.refreshBtn }
func (p *sourcesPanel) CopyBtn() *widget.Button { return p.copyBtn }

func (p *sourcesPanel) SelectedURL() string { return p.selectedURL }
func (p *sourcesPanel) SetSelectedURL(u string) { p.selectedURL = u }
func (p *sourcesPanel) Tree() *widget.Tree { return p.tree }
func (p *sourcesPanel) Filter() string { return p.filter }
func (p *sourcesPanel) SetFilter(f string) { p.filter = f }
func (p *sourcesPanel) ApplyFilter() { p.applyFilter() }
func (p *sourcesPanel) SyncTree() { p.syncTree() }
func (p *sourcesPanel) ShowResource(r sourceResource) { p.showResource(r) }
func (p *sourcesPanel) CopySelection() { p.copySelection() }
func (p *sourcesPanel) CategoryUIDs() []string { return p.categoryUIDs() }
func (p *sourcesPanel) ResourceUIDsInCategory(cat string) []string { return p.resourceUIDsInCategory(cat) }

// NewSourcesPanel exports newSourcesPanel for use by external test packages.
var NewSourcesPanel = newSourcesPanel


// SourcesSelectHint exports sourcesSelectHint for use by external test packages.
var SourcesSelectHint = sourcesSelectHint

// SourceResource is the exported type alias for sourceResource for use by external test packages.
type SourceResource = sourceResource

// TreeResourceUID exports treeResourceUID for use by external test packages.
var TreeResourceUID = treeResourceUID

// TreeCategoryUID exports treeCategoryUID for use by external test packages.
var TreeCategoryUID = treeCategoryUID

// LineNumberGutter exports lineNumberGutter for use by external test packages.
var LineNumberGutter = lineNumberGutter

// ClassifySourceType exports classifySourceType for use by external test packages.
var ClassifySourceType = classifySourceType

// SourceBaseName exports sourceBaseName for use by external test packages.
var SourceBaseName = sourceBaseName

// NewScriptQueuePanelContent exports newScriptQueuePanelContent for use by external test packages.
var NewScriptQueuePanelContent = newScriptQueuePanelContent

// FormatConsoleMessageLite exports formatConsoleMessageLite for use by external test packages.
var FormatConsoleMessageLite = formatConsoleMessageLite

// StringifyConsoleData exports stringifyConsoleData for use by external test packages.
var StringifyConsoleData = stringifyConsoleData

// SecurityPanel is the exported type alias for securityPanel for use by external test packages.
type SecurityPanel = securityPanel

// Label exports label field for use by external test packages.
func (p *securityPanel) Label() *widget.Label { return p.label }

// NewSecurityPanel exports newSecurityPanel for use by external test packages.
var NewSecurityPanel = newSecurityPanel


// SettingsPanel is the exported type alias for settingsPanel for use by external test packages.
type SettingsPanel = settingsPanel

func (p *settingsPanel) HomepageEntry() *widget.Entry { return p.homepageEntry }
func (p *settingsPanel) SearchEntry() *widget.Entry { return p.searchEntry }
func (p *settingsPanel) JSToggle() *widget.Check { return p.jsToggle }
func (p *settingsPanel) ImgToggle() *widget.Check { return p.imgToggle }
func (p *settingsPanel) Status() *widget.Label { return p.status }
func (p *settingsPanel) SetActiveTabFn(fn func() *TabContext) { p.setActiveTabFn(fn) }

// NewSettingsPanel exports newSettingsPanel for use by external test packages.
var NewSettingsPanel = newSettingsPanel


// CollectOrigins exports collectOrigins for use by external test packages.
var CollectOrigins = collectOrigins

// FilterSnapshot exports filterSnapshot for use by external test packages.
var FilterSnapshot = filterSnapshot

// StoragePanel is the exported type alias for storagePanel for use by external test packages.
type StoragePanel = storagePanel

// StorageProvider is the exported type alias for storageProvider for use by external test packages.
type StorageProvider = storageProvider

func (p *storagePanel) Entries() []storageRow { return p.entries }
func (p *storagePanel) SetEntries(entries []storageRow) { p.entries = entries }
func (p *storagePanel) SetStore(s storageProvider) { p.store = s }
func (p *storagePanel) SelectedIdx() int { return p.selectedIdx }
func (p *storagePanel) SetSelectedIdx(idx int) { p.selectedIdx = idx }
func (p *storagePanel) DeleteSelected() { p.deleteSelected() }
func (p *storagePanel) SelectedOrigin() string { return p.selectedOrigin() }
func (p *storagePanel) SelectedKey() string { return p.selectedKey() }
func (p *storagePanel) ClearSelectedOrigin() { p.clearSelectedOrigin() }
func (p *storagePanel) SearchEntry() *widget.Entry { return p.searchEntry }
func (p *storagePanel) FilterEntries() { p.filterEntries() }



// NewStoragePanel exports newStoragePanel for use by external test packages.
var NewStoragePanel = newStoragePanel


// StorageRow is the exported type alias for storageRow for use by external test packages.
type StorageRow = storageRow

// NewTileCachePanelContent exports newTileCachePanelContent for use by external test packages.
var NewTileCachePanelContent = newTileCachePanelContent

// FormatCount exports formatCount for use by external test packages.
var FormatCount = formatCount

// FormatLimit exports formatLimit for use by external test packages.
var FormatLimit = formatLimit

// FormatBudget exports formatBudget for use by external test packages.
var FormatBudget = formatBudget

// SnapshotMetrics exports snapshotMetrics for use by external test packages.
var SnapshotMetrics = snapshotMetrics

// NewDisplayListPanelContent exports newDisplayListPanelContent for use by external test packages.
var NewDisplayListPanelContent = newDisplayListPanelContent

// NewDisplayListPanelWithHighlight exports newDisplayListPanelWithHighlight for use by external test packages.
var NewDisplayListPanelWithHighlight = newDisplayListPanelWithHighlight


// DisplayListTypeOrder exports displayListTypeOrder for use by external test packages.
var DisplayListTypeOrder = displayListTypeOrder

// FormatCommandLine exports formatCommandLine for use by external test packages.
var FormatCommandLine = formatCommandLine

// FormatCommandDetail exports formatCommandDetail for use by external test packages.
var FormatCommandDetail = formatCommandDetail

// NewMemoryPanel exports newMemoryPanel for use by external test packages.
var NewMemoryPanel = newMemoryPanel

// FormatMethod exports formatMethod for use by external test packages.
var FormatMethod = formatMethod

// FormatStatusClass exports formatStatusClass for use by external test packages.
var FormatStatusClass = formatStatusClass

// FormatContentType exports formatContentType for use by external test packages.
var FormatContentType = formatContentType

// FormatBytes exports formatBytes for use by external test packages.
var FormatBytes = formatBytes

// FormatDuration exports formatDuration for use by external test packages.
var FormatDuration = formatDuration

// TruncateMiddle exports truncateMiddle for use by external test packages.
var TruncateMiddle = truncateMiddle

// FormatWaterfall exports formatWaterfall for use by external test packages.
var FormatWaterfall = formatWaterfall

// FormatCurl exports formatCurl for use by external test packages.
var FormatCurl = formatCurl

// ShellQuote exports shellQuote for use by external test packages.
var ShellQuote = shellQuote

// ClipboardSet exports clipboardSet for use by external test packages.
var ClipboardSet = clipboardSet

// NewNetworkPanel exports newNetworkPanel for use by external test packages.
var NewNetworkPanel = newNetworkPanel

// MemoryPanel is the exported type alias for memoryPanel for use by external test packages.
type MemoryPanel = memoryPanel

func (p *memoryPanel) DetailsLabel() *widget.Label { return p.detailsLabel }
func (p *memoryPanel) DOMProgress() *widget.ProgressBar { return p.domProgress }
func (p *memoryPanel) LayoutProgress() *widget.ProgressBar { return p.layoutProgress }
func (p *memoryPanel) ImagesProgress() *widget.ProgressBar { return p.imagesProgress }
func (p *memoryPanel) JSProgress() *widget.ProgressBar { return p.jsProgress }
func (p *memoryPanel) GlobalProgress() *widget.ProgressBar { return p.globalProgress }
func (p *memoryPanel) DOMUsageLabel() *widget.Label { return p.domUsageLabel }
func (p *memoryPanel) LayoutUsageLabel() *widget.Label { return p.layoutUsageLabel }
func (p *memoryPanel) ImagesUsageLabel() *widget.Label { return p.imagesUsageLabel }
func (p *memoryPanel) JSUsageLabel() *widget.Label { return p.jsUsageLabel }
func (p *memoryPanel) GlobalUsageLabel() *widget.Label { return p.globalUsageLabel }
func (p *memoryPanel) GCBtn() *widget.Button { return p.gcBtn }
func (p *memoryPanel) SetOnGC(fn func()) { p.onGC = fn }

// NetworkPanel is the exported type alias for networkPanel for use by external test packages.
type NetworkPanel = networkPanel

func (p *networkPanel) Entries() []NetRequestEntry { return p.entries }
func (p *networkPanel) SetEntries(entries []NetRequestEntry) { p.entries = entries }
func (p *networkPanel) VisibleEntries() []NetRequestEntry { return p.visible }
func (p *networkPanel) VisibleResources() []NetRequestEntry { return p.visible }
func (p *networkPanel) SetVisibleEntries(v []NetRequestEntry) { p.visible = v }

func (p *networkPanel) SyncData() { p.syncData() }

func (p *networkPanel) FilterEntries() []NetRequestEntry { return p.filterEntries() }
func (p *networkPanel) ApplySort() { p.applySort() }
func (p *networkPanel) FormatRow(e NetRequestEntry) string { return p.formatRow(e) }
func (p *networkPanel) ShowDetail(e NetRequestEntry) { p.showDetail(e) }
func (p *networkPanel) Rebuild() { p.rebuild() }
func (p *networkPanel) SearchEntry() *widget.Entry { return p.searchEntry }
func (p *networkPanel) ClearBtn() *widget.Button { return p.clearBtn }
func (p *networkPanel) PreserveCheck() *widget.Check { return p.preserveCheck }
func (p *networkPanel) DetailLabel() *widget.Label { return p.detailLabel }
func (p *networkPanel) SetSortCol(col int) { p.sortCol = col }
func (p *networkPanel) SetSortAsc(asc bool) { p.sortAsc = asc }
func (p *networkPanel) SortCol() int { return p.sortCol }
func (p *networkPanel) SortAsc() bool { return p.sortAsc }
func (p *networkPanel) SetFilterSearch(s string) { p.filter.search = s }
func (p *networkPanel) SetFilterStatusClass(s string) { p.filter.statusClass = s }
func (p *networkPanel) SetFilterContentType(c string) { p.filter.contentType = c }
func (p *networkPanel) DetailBox() *fyne.Container { return p.detailBox }

// FormatWaterfallWithPhases exports formatWaterfallWithPhases for use by external test packages.
var FormatWaterfallWithPhases = formatWaterfallWithPhases

// PerformancePanel is the exported type alias for performancePanel for use by external test packages.
type PerformancePanel = performancePanel

// Label exports label field for use by external test packages.
func (p *performancePanel) Label() *widget.Label { return p.label }

// NewPerformancePanel exports newPerformancePanel for use by external test packages.
var NewPerformancePanel = newPerformancePanel

// HumanPhaseLabel exports humanPhaseLabel for use by external test packages.
var HumanPhaseLabel = humanPhaseLabel




