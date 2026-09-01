package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/vyquocvu/goosie/internal/engine/eventloop"
	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/renderer"
)


// ContentScroll exports contentScroll field for use by external test packages.
func (t *Tab) ContentScroll() *container.Scroll { return t.contentScroll }

// SetContentScroll sets contentScroll field for use by external test packages.
func (t *Tab) SetContentScroll(s *container.Scroll) { t.contentScroll = s }

// HTMLRenderer exports htmlRenderer field for use by external test packages.
func (t *Tab) HTMLRenderer() HTMLRenderer { return t.htmlRenderer }

// SetHTMLRenderer sets htmlRenderer field for use by external test packages.
func (t *Tab) SetHTMLRenderer(r HTMLRenderer) { t.htmlRenderer = r }

// RefreshTabContent exports refreshTabContent for use by external test packages.
var RefreshTabContent = refreshTabContent

// FilterSelect exports filterSelect field for use by external test packages.
func (p *ConsolePanel) FilterSelect() *widget.Select { return p.filterSelect }

// GetFilteredMessageCount exports getFilteredMessageCount for use by external test packages.
func (p *ConsolePanel) GetFilteredMessageCount() int { return p.getFilteredMessageCount() }

// GetFilteredMessage exports getFilteredMessage for use by external test packages.
func (p *ConsolePanel) GetFilteredMessage(i int) *js.ConsoleMessage { return p.getFilteredMessage(i) }

// CommandEntry exports commandEntry field for use by external test packages.
func (p *ConsolePanel) CommandEntry() *ConsoleEntry { return p.commandEntry }

// History exports history field for use by external test packages.
func (e *ConsoleEntry) History() []string { return e.history }

// BuildMenu exports buildMenu for use by external test packages.
func (m *DevToolsContextMenu) BuildMenu(node *renderer.RenderNode, layout *renderer.LayoutBox) *fyne.Menu {
	return m.buildMenu(node, layout)
}


// Tree exports tree field for use by external test packages.
func (p *InspectPanel) Tree() *widget.Tree { return p.tree }

// ExpandAncestors exports expandAncestors for use by external test packages.
func (p *InspectPanel) ExpandAncestors(node *renderer.RenderNode) { p.expandAncestors(node) }


// SelectedNode exports selectedNode field for use by external test packages.
func (p *InspectPanel) SelectedNode() *renderer.RenderNode { return p.selectedNode }

// RefreshRenderer exports refreshRenderer for use by external test packages.
func (p *InspectPanel) RefreshRenderer() { p.refreshRenderer() }

// HasPhaseTimings exports hasPhaseTimings for use by external test packages.
func (p *InspectPanel) HasPhaseTimings() bool { return p.hasPhaseTimings() }

// LastMetrics exports lastMetrics field for use by external test packages.
func (p *InspectPanel) LastMetrics() metrics.Metrics { return p.lastMetrics }

// NodeMap exports nodeMap field for use by external test packages.
func (p *InspectPanel) NodeMap() map[string]*renderer.RenderNode { return p.nodeMap }

// BreadcrumbsBar exports breadcrumbsBar field for use by external test packages.
func (p *InspectPanel) BreadcrumbsBar() *fyne.Container { return p.breadcrumbsBar }

// StylesContainer exports stylesContainer field for use by external test packages.
func (p *InspectPanel) StylesContainer() *fyne.Container { return p.stylesContainer }

// StylesFilter exports stylesFilter field for use by external test packages.
func (p *InspectPanel) StylesFilter() *widget.Entry { return p.stylesFilter }

// CurrentIndex exports currentIndex field for use by external test packages.
func (s *BrowserState) CurrentIndex() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentIndex
}

// History exports history field for use by external test packages.
func (s *BrowserState) History() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.history
}

// NewBrowserInternal exports newBrowserInternal for use by external test packages.
var NewBrowserInternal = newBrowserInternal

// MainGoroutineID exports &mainGoroutineID for use by external test packages.
var MainGoroutineID = &mainGoroutineID

// CaptureMainGoroutineID exports captureMainGoroutineID for use by external test packages.
var CaptureMainGoroutineID = captureMainGoroutineID

// SaveImageAsPNG exports saveImageAsPNG for use by external test packages.
var SaveImageAsPNG = saveImageAsPNG

// FormatCounterValue exports formatCounterValue for use by external test packages.
var FormatCounterValue = formatCounterValue

// FPSButton exports fpsButton field for use by external test packages.
func (b *Browser) FPSButton() *widget.Button { return b.fpsButton }

// FPSBar exports fpsBar field for use by external test packages.
func (b *Browser) FPSBar() *FPSBar { return b.fpsBar }

// ToggleFPSOverlay exports toggleFPSOverlay for use by external test packages.
func (b *Browser) ToggleFPSOverlay() { b.toggleFPSOverlay() }

// DevToolsButton exports devToolsButton field for use by external test packages.
func (b *Browser) DevToolsButton() *widget.Button { return b.devToolsButton }

// DevToolsVisible exports devToolsVisible field for use by external test packages.
func (b *Browser) DevToolsVisible() bool { return b.devToolsVisible }

// DirtyOverlayButton exports dirtyOverlayButton field for use by external test packages.
func (b *Browser) DirtyOverlayButton() *widget.Button { return b.dirtyOverlayButton }

// ToggleDirtyOverlay exports toggleDirtyOverlay for use by external test packages.
func (b *Browser) ToggleDirtyOverlay() { b.toggleDirtyOverlay() }

// ShowMemoryDialog exports showMemoryDialog for use by external test packages.
func (b *Browser) ShowMemoryDialog() { b.showMemoryDialog() }


// ShowNetworkQueueDialog exports showNetworkQueueDialog for use by external test packages.
func (b *Browser) ShowNetworkQueueDialog() { b.showNetworkQueueDialog() }

// ShowSourceDialog exports showSourceDialog for use by external test packages.
func (b *Browser) ShowSourceDialog() { b.showSourceDialog() }

// Deps exports deps field for use by external test packages.
func (b *Browser) Deps() *BrowserDependencies { return &b.deps }

// State exports state field for use by external test packages.
func (t *Tab) State() *BrowserState { return t.state }

// RootNode exports rootNode field for use by external test packages.
func (p *InspectPanel) RootNode() *renderer.RenderNode { return p.rootNode }

// SetSelectedNode sets selectedNode field for use by external test packages.
func (p *InspectPanel) SetSelectedNode(n *renderer.RenderNode) { p.selectedNode = n }

// BuildCopyItems exports buildCopyItems for use by external test packages.
func (m *DevToolsContextMenu) BuildCopyItems(node *renderer.RenderNode, layout *renderer.LayoutBox) []*fyne.MenuItem {
	return m.buildCopyItems(node, layout)
}

// CSSSelector exports cssSelector for use by external test packages.
var CSSSelector = cssSelector

// RenderOuterHTML exports renderOuterHTML for use by external test packages.
var RenderOuterHTML = renderOuterHTML

// RenderInnerHTML exports renderInnerHTML for use by external test packages.
var RenderInnerHTML = renderInnerHTML

// ExtractText exports extractText for use by external test packages.
var ExtractText = extractText


// EscapeAttr exports escapeAttr for use by external test packages.
var EscapeAttr = escapeAttr

// ShowDisplayListDialog exports showDisplayListDialog for use by external test packages.
func (b *Browser) ShowDisplayListDialog() { b.showDisplayListDialog() }

// DoAndWait exports doAndWait for use by external test packages.
func (b *Browser) DoAndWait(fn func()) { b.doAndWait(fn) }

// SetDevToolsVisible sets devToolsVisible field for use by external test packages.
func (b *Browser) SetDevToolsVisible(v bool) { b.devToolsVisible = v }

// EventLoop exports eventLoop field for use by external test packages.
func (t *Tab) EventLoop() *eventloop.Loop { return t.eventLoop }

// EnsureHTMLRenderer exports ensureHTMLRenderer for use by external test packages.
func (t *Tab) EnsureHTMLRenderer() { t.ensureHTMLRenderer() }

// DrainInputLoop exports drainInputLoop for use by external test packages.
func (t *Tab) DrainInputLoop() { t.drainInputLoop() }

// InspectPanel exports inspectPanel field for use by external test packages.
func (b *Browser) InspectPanel() *InspectPanel { return b.inspectPanel }

// DevToolsMenu exports devToolsMenu field for use by external test packages.
func (b *Browser) DevToolsMenu() *DevToolsContextMenu { return b.devToolsMenu }

// SetLastHoverHit sets lastHoverHit field for use by external test packages.
func (t *Tab) SetLastHoverHit(tm time.Time) { t.lastHoverHit = tm }

// PostCanvasMouseInput exports postCanvasMouseInput for use by external test packages.
func (t *Tab) PostCanvasMouseInput(input renderer.MouseInput) { t.postCanvasMouseInput(input) }

// ExecuteRenderRequest exports executeRenderRequest for use by external test packages.
func (t *Tab) ExecuteRenderRequest() { t.executeRenderRequest() }

// BumpDocumentGeneration exports bumpDocumentGeneration for use by external test packages.
func (t *Tab) BumpDocumentGeneration() { t.bumpDocumentGeneration() }

// DOMTreeNodeWidget is the exported type alias for domTreeNodeWidget for use by external test packages.
type DOMTreeNodeWidget = domTreeNodeWidget

// Text exports text field for use by external test packages.
func (w *domTreeNodeWidget) Text() *canvas.Text { return w.text }







