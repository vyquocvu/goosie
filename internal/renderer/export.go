package renderer

import (
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/vyquocvu/goosie/internal/css"
	imageloader "github.com/vyquocvu/goosie/internal/image"
	"github.com/vyquocvu/goosie/internal/net"
)

// CurrentRenderTree exports currentRenderTree field for use by external test packages.
func (r *Renderer) CurrentRenderTree() *RenderNode { return r.currentRenderTree }

// SetCurrentRenderTree sets currentRenderTree field for use by external test packages.
func (r *Renderer) SetCurrentRenderTree(node *RenderNode) { r.currentRenderTree = node }

// CurrentLayoutTree exports currentLayoutTree field for use by external test packages.
func (r *Renderer) CurrentLayoutTree() *LayoutBox { return r.currentLayoutTree }

// SetCurrentLayoutTree sets currentLayoutTree field for use by external test packages.
func (r *Renderer) SetCurrentLayoutTree(box *LayoutBox) { r.currentLayoutTree = box }

// FindLayoutBoxForNode exports findLayoutBoxForNode for use by external test packages.
var FindLayoutBoxForNode = findLayoutBoxForNode

// FindRenderNodeByIDRoot exports findRenderNodeByIDRoot for use by external test packages.
var FindRenderNodeByIDRoot = findRenderNodeByIDRoot

// ParseLength exports parseLength for use by external test packages.
var ParseLength = parseLength

// ParseBoxShorthand exports parseBoxShorthand for use by external test packages.
var ParseBoxShorthand = parseBoxShorthand

// EvalCalcExpr exports evalCalcExpr for use by external test packages.
var EvalCalcExpr = evalCalcExpr

// ResolveVarTokens exports resolveVarTokens for use by external test packages.
var ResolveVarTokens = resolveVarTokens

// CollectFixed exports collectFixed for use by external test packages.
var CollectFixed = collectFixed

// MergeInlineAndExternalCSS exports mergeInlineAndExternalCSS for use by external test packages.
var MergeInlineAndExternalCSS = mergeInlineAndExternalCSS

// ApplyLinkColor exports applyLinkColor for use by external test packages.
var ApplyLinkColor = applyLinkColor

// LinkColorTheme is the exported type alias for linkColorTheme for use by external test packages.
type LinkColorTheme = linkColorTheme

// ParseColor exports parseColor for use by external test packages.
var ParseColor = parseColor

// NewTappableHyperlink exports newTappableHyperlink for use by external test packages.
var NewTappableHyperlink = newTappableHyperlink

// NewInspectableContainer exports newInspectableContainer for use by external test packages.
func NewInspectableContainer(content fyne.CanvasObject, cr *CanvasRenderer) *InspectableContainer {
	return newInspectableContainer(content, cr)
}

// GlobalTableColumnCache exports globalTableColumnCache for use by external test packages.
var GlobalTableColumnCache = globalTableColumnCache

// LayoutEngine exports layoutEngine field for use by external test packages.
func (r *Renderer) LayoutEngine() *LayoutEngine { return r.layoutEngine }

// CanvasRenderer exports canvasRenderer field for use by external test packages.
func (r *Renderer) CanvasRenderer() *CanvasRenderer { return r.canvasRenderer }

// TestingMode exports testingMode field for use by external test packages.
func (r *Renderer) TestingMode() bool { return r.testingMode }

// ImageLoader exports imageLoader field for use by external test packages.
func (r *Renderer) ImageLoader() imageloader.Loader { return r.imageLoader }

// ChunkedDisplay exports chunkedDisplay field for use by external test packages.
func (r *Renderer) ChunkedDisplay() *ChunkedDisplayList { return r.chunkedDisplay }

// TreeMu exports treeMu field for use by external test packages.
func (r *Renderer) TreeMu() *sync.RWMutex { return &r.treeMu }

// Stylesheet exports stylesheet field for use by external test packages.
func (r *Renderer) Stylesheet() *css.StyleSheet {
	r.stylesheetMu.RLock()
	defer r.stylesheetMu.RUnlock()
	return r.stylesheet
}

// SetStylesheet sets stylesheet field for use by external test packages.
func (r *Renderer) SetStylesheet(s *css.StyleSheet) {
	r.stylesheetMu.Lock()
	defer r.stylesheetMu.Unlock()
	r.stylesheet = s
}

// StylesheetMu exports stylesheetMu field for use by external test packages.
func (r *Renderer) StylesheetMu() *sync.RWMutex { return &r.stylesheetMu }

// Incremental exports incremental field for use by external test packages.
func (r *Renderer) Incremental() *IncrementalLayoutEngine { return r.incremental }

// Inspectable exports inspectable field for use by external test packages.
func (cr *CanvasRenderer) Inspectable() *InspectableContainer { return cr.inspectable }

// SetInspectable sets inspectable field for use by external test packages.
func (cr *CanvasRenderer) SetInspectable(obj *InspectableContainer) { cr.inspectable = obj }

// OnContextMenu exports onContextMenu field for use by external test packages.
func (cr *CanvasRenderer) OnContextMenu() func(node *RenderNode, layout *LayoutBox, abs fyne.Position) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.onContextMenu
}

// CanvasRendererMu exports mu field for use by external test packages.
func (cr *CanvasRenderer) CanvasRendererMu() *sync.RWMutex { return &cr.mu }

// Renderer exports renderer field for use by external test packages.
func (cr *CanvasRenderer) Renderer() *Renderer { return cr.renderer }

// SetRenderer sets renderer field for use by external test packages.
func (cr *CanvasRenderer) SetRenderer(r *Renderer) { cr.renderer = r }

// DLBuildGen exports dlBuildGen field for use by external test packages.
func (cr *CanvasRenderer) DLBuildGen() uint64 { return cr.dlBuildGen }

// CachedDisplayList exports cachedDisplayList field for use by external test packages.
func (cr *CanvasRenderer) CachedDisplayList() *DisplayList { return cr.cachedDisplayList }

// SetCachedDisplayList sets cachedDisplayList field for use by external test packages.
func (cr *CanvasRenderer) SetCachedDisplayList(dl *DisplayList) { cr.cachedDisplayList = dl }

// BaseURL exports baseURL field for use by external test packages.
func (cr *CanvasRenderer) BaseURL() string { return cr.baseURL }

// SetBaseURL sets baseURL field for use by external test packages.
func (cr *CanvasRenderer) SetBaseURL(u string) { cr.baseURL = u }

// ResolveURL exports resolveURL for use by external test packages.
func (cr *CanvasRenderer) ResolveURL(ref string) string { return cr.resolveURL(ref) }

// CanvasWidth exports canvasWidth field for use by external test packages.
func (cr *CanvasRenderer) CanvasWidth() float32 { return cr.canvasWidth }

// CanvasHeight exports canvasHeight field for use by external test packages.
func (cr *CanvasRenderer) CanvasHeight() float32 { return cr.canvasHeight }

// CanvasWidth exports canvasWidth field for use by external test packages.
func (le *LayoutEngine) CanvasWidth() float32 { return le.canvasWidth }

// CanvasHeight exports canvasHeight field for use by external test packages.
func (le *LayoutEngine) CanvasHeight() float32 { return le.canvasHeight }

// NodeMapMu exports nodeMapMu field for use by external test packages.
func (le *LayoutEngine) NodeMapMu() *sync.RWMutex { return &le.nodeMapMu }

// NodeMap exports nodeMap field for use by external test packages.
func (le *LayoutEngine) NodeMap() map[int64]*LayoutBox { return le.nodeMap }

// DefaultFontSize exports defaultFontSize field for use by external test packages.
func (le *LayoutEngine) DefaultFontSize() float32 { return le.defaultFontSize }

// GetFontSize exports getFontSize for use by external test packages.
func (le *LayoutEngine) GetFontSize(tagName string) float32 { return le.getFontSize(tagName) }

// MinContentSize exports minContentSize for use by external test packages.
func (le *LayoutEngine) MinContentSize(node *RenderNode) float32 { return le.minContentSize(node) }

// Fetcher exports fetcher field for use by external test packages.
func (r *Renderer) Fetcher() *net.Fetcher { return r.fetcher }

// SetFetcher sets fetcher field for use by external test packages.
func (r *Renderer) SetFetcher(f *net.Fetcher) { r.fetcher = f }

// OnNavigate exports onNavigate field for use by external test packages.
func (r *Renderer) OnNavigate() func(string) { return r.onNavigate }

// OnNavigate exports onNavigate field for use by external test packages.
func (cr *CanvasRenderer) OnNavigate() NavigationCallback { return cr.onNavigate }

// IntrinsicSizeEntry is the exported type alias for intrinsicSizeEntry for use by external test packages.
type IntrinsicSizeEntry = intrinsicSizeEntry

// DirtyNodes exports dirtyNodes field for use by external test packages.
func (it *InvalidationTracker) DirtyNodes() map[int64]DirtyFlag { return it.dirtyNodes }

// NewLineBox exports newLineBox for use by external test packages.
func (ile *InlineLayoutEngine) NewLineBox(x, y, availableWidth float32, textAlign string, lineHeight float32) *LineBox {
	return ile.newLineBox(x, y, availableWidth, textAlign, lineHeight)
}

// FinalizeLine exports finalizeLine for use by external test packages.
func (ile *InlineLayoutEngine) FinalizeLine(lb *LineBox) { ile.finalizeLine(lb) }

// FontMetrics exports fontMetrics field for use by external test packages.
func (ile *InlineLayoutEngine) FontMetrics() *FontMetrics { return ile.fontMetrics }

// DefaultFontSize exports defaultFontSize field for use by external test packages.
func (ile *InlineLayoutEngine) DefaultFontSize() float32 { return ile.defaultFontSize }

// CollapseWhiteSpace exports collapseWhiteSpace for use by external test packages.
func (ile *InlineLayoutEngine) CollapseWhiteSpace(text string) string { return ile.collapseWhiteSpace(text) }

// CollapseWhiteSpacePreserveNewlines exports collapseWhiteSpacePreserveNewlines for use by external test packages.
func (ile *InlineLayoutEngine) CollapseWhiteSpacePreserveNewlines(text string) string { return ile.collapseWhiteSpacePreserveNewlines(text) }

// SplitTextForWrapping exports splitTextForWrapping for use by external test packages.
func (ile *InlineLayoutEngine) SplitTextForWrapping(text string, mode WhiteSpaceMode) []string {
	return ile.splitTextForWrapping(text, mode)
}

// ProcessWhiteSpace exports processWhiteSpace for use by external test packages.
func (ile *InlineLayoutEngine) ProcessWhiteSpace(text string, mode WhiteSpaceMode) string { return ile.processWhiteSpace(text, mode) }

// IsInlineBlock exports isInlineBlock for use by external test packages.
func (ile *InlineLayoutEngine) IsInlineBlock(node *RenderNode) bool { return ile.isInlineBlock(node) }

// GetFontSizeForNode exports getFontSizeForNode for use by external test packages.
func (ile *InlineLayoutEngine) GetFontSizeForNode(node *RenderNode) float32 { return ile.getFontSizeForNode(node) }


// String returns string representation of WhiteSpaceMode.
func (m WhiteSpaceMode) String() string {
	switch m {
	case WhiteSpaceNormal:
		return "normal"
	case WhiteSpaceNoWrap:
		return "nowrap"
	case WhiteSpacePre:
		return "pre"
	case WhiteSpacePreWrap:
		return "pre-wrap"
	case WhiteSpacePreLine:
		return "pre-line"
	default:
		return "unknown"
	}
}

// ParseFlexShorthand exports parseFlexShorthand for use by external test packages.
var ParseFlexShorthand = parseFlexShorthand

// SplitIntoWords exports splitIntoWords for use by external test packages.
var SplitIntoWords = splitIntoWords

// ConvertPaintCommands exports convertPaintCommands for use by external test packages.
var ConvertPaintCommands = convertPaintCommands

// FPSFromInterval exports fpsFromInterval for use by external test packages.
var FPSFromInterval = fpsFromInterval

// DefaultFontSize exports defaultFontSize field for use by external test packages.
func (fm *FontMetrics) DefaultFontSize() float32 { return fm.defaultFontSize }

// FPSOverlayText exports fpsOverlayText field for use by external test packages.
func (cr *CanvasRenderer) FPSOverlayText() *canvas.Text { return cr.fpsOverlayText }

// FPSOverlayBg exports fpsOverlayBg field for use by external test packages.
func (cr *CanvasRenderer) FPSOverlayBg() *canvas.Rectangle { return cr.fpsOverlayBg }

// FPSOverlayTextCache exports fpsOverlayTextCache field for use by external test packages.
func (cr *CanvasRenderer) FPSOverlayTextCache() string { return cr.fpsOverlayTextCache }

// BuildFPSOverlay exports buildFPSOverlay for use by external test packages.
func (cr *CanvasRenderer) BuildFPSOverlay() []fyne.CanvasObject { return cr.buildFPSOverlay() }

// OnImageLoaded exports onImageLoaded for use by external test packages.
func (cr *CanvasRenderer) OnImageLoaded(url string) { cr.onImageLoaded(url) }

// MaxSamples exports maxSamples field for use by external test packages.
func (c *FPSCounter) MaxSamples() int { return c.maxSamples }

// SetMaxSamples sets maxSamples field for use by external test packages.
func (c *FPSCounter) SetMaxSamples(n int) { c.maxSamples = n }

// Target exports target field for use by external test packages.
func (c *FPSCounter) Target() time.Duration { return c.target }

// SetTarget sets target field for use by external test packages.
func (c *FPSCounter) SetTarget(d time.Duration) { c.target = d }

// Samples exports samples field for use by external test packages.
func (c *FPSCounter) Samples() []time.Time { return c.samples }

// Invalidation exports invalidation field for use by external test packages.
func (ile *IncrementalLayoutEngine) Invalidation() *InvalidationTracker { return ile.invalidation }


// NewLinkColorTheme creates a LinkColorTheme with given base theme and link color.
func NewLinkColorTheme(base fyne.Theme, link color.Color) LinkColorTheme {
	return linkColorTheme{Theme: base, link: link}
}


// LinkColor returns link color of LinkColorTheme.
func (t LinkColorTheme) LinkColor() color.Color { return t.link }

// NewPaintChunk creates a PaintChunk with given fields.
func NewPaintChunk(owner LayoutID, start, end int, bounds RectF, dirty bool) PaintChunk {
	return PaintChunk{Owner: owner, Start: start, End: end, Bounds: bounds, dirty: dirty}
}
