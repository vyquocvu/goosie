package renderer

import (
	"context"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/vyquocvu/goosie/internal/css"
	imageloader "github.com/vyquocvu/goosie/internal/image"
	"github.com/vyquocvu/goosie/internal/net"
	"log"
)

// Renderer is the main HTML renderer that coordinates parsing, layout, and rendering
type Renderer struct {
	layoutEngine   *LayoutEngine
	canvasRenderer *CanvasRenderer
	imageLoader    imageloader.Loader
	stylesheet     *css.StyleSheet
	fetcher        *net.Fetcher

	// Cached trees for performance
	currentRenderTree *RenderNode
	currentLayoutTree *LayoutBox
	treeMu            sync.RWMutex

	// Navigation callback for link clicks
	onNavigate func(url string)

	// Current page URL for resolving relative links
	currentURL   string
	currentURLMu sync.RWMutex

	// Inspect callback for element inspection
	onInspect func(node *RenderNode, layout *LayoutBox)

	// Refresh callback for UI updates
	onRefresh func()

	// Testing mode to bypass Fyne's main thread requirement for callbacks
	testingMode bool

	// headless skips fyne.Do marshalling when no UI event loop is running
	headless bool

	// Mutex protects stylesheet during concurrent CSS loading
	stylesheetMu sync.RWMutex

	// dirty tracks whether style/layout needs recomputation.
	// Set by MarkDirty(), SetSize(), and mutation paths.
	// Cleared by Refresh() after recomputation.
	dirty bool

	// csp holds the parsed Content-Security-Policy for style-src enforcement.
	csp   *net.CSPPolicy
	cspMu sync.RWMutex

	Logger  *slog.Logger
	metrics *RenderMetrics
}

// NewRenderer creates a new HTML renderer
func NewRenderer(width, height float32) *Renderer {
	imageLoader := imageloader.NewLoader(100) // Cache up to 100 images
	canvasRenderer := NewCanvasRenderer(width, height)
	canvasRenderer.imageLoader = imageLoader

	return &Renderer{
		layoutEngine:   NewLayoutEngine(width, height),
		canvasRenderer: canvasRenderer,
		imageLoader:    imageLoader,
		fetcher:        net.NewFetcher(),
		Logger:         slog.Default(),
		metrics:        NewRenderMetrics(),
	}
}

// Metrics returns the render metrics
func (r *Renderer) Metrics() *RenderMetrics {
	return r.metrics
}

// SetLogger sets the structured logger for the Renderer and its CanvasRenderer
func (r *Renderer) SetLogger(l *slog.Logger) {
	if l == nil {
		r.Logger = slog.Default()
	} else {
		r.Logger = l
	}
	r.canvasRenderer.SetLogger(r.Logger)
}

// RenderHTML renders HTML content and returns a Fyne canvas object
func (r *Renderer) RenderHTML(ctx context.Context, htmlContent string) (fyne.CanvasObject, error) {
	start := time.Now()
	defer func() { r.metrics.RecordRenderHTML(time.Since(start)) }()

	globalTableColumnCache.Clear()
	recorder := metrics.RecorderFromContext(ctx)
	if recorder != nil {
		recorder.BeginPhase(metrics.PhaseParse)
	}

	// Parse HTML
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		if recorder != nil {
			recorder.EndPhase(metrics.PhaseParse)
		}
		return nil, err
	}

	if recorder != nil {
		recorder.AddCounters(metrics.Counters{
			NodeCount: countHTMLNodes(doc),
		})
	}

	// Extract and parse CSS from <style> tags
	r.stylesheetMu.Lock()
	r.stylesheet = extractAndParseCSS(doc)
	r.stylesheetMu.Unlock()

	if recorder != nil && r.stylesheet != nil {
		rules, selectors := countRulesAndSelectors(r.stylesheet)
		recorder.AddCounters(metrics.Counters{
			RuleCount:     rules,
			SelectorCount: selectors,
		})
	}

	// Find html element
	htmlNode := findHTMLNode(doc)
	if htmlNode == nil {
		// No html found, use the entire document
		htmlNode = doc
	}

	// Build render tree
	renderTree := BuildRenderTree(htmlNode)

	if recorder != nil {
		recorder.EndPhase(metrics.PhaseParse)
	}

	if renderTree == nil {
		// Return empty container if no content
		return r.canvasRenderer.Render(nil), nil
	}

	r.treeMu.RLock()
	width, height := r.layoutEngine.canvasWidth, r.layoutEngine.canvasHeight
	r.treeMu.RUnlock()

	r.treeMu.Lock()

	if recorder != nil {
		recorder.BeginPhase(metrics.PhaseStyle)
	}
	// Apply styles
	renderTreeCopy := renderTree.Clone()
	r.stylesheetMu.RLock()
	if r.stylesheet != nil {
		styleManager := NewStyleManagerWithViewport(r.stylesheet, width, height)
		styleManager.ApplyStyles(renderTreeCopy)
	}
	r.stylesheetMu.RUnlock()
	if recorder != nil {
		recorder.EndPhase(metrics.PhaseStyle)
	}

	if recorder != nil {
		recorder.BeginPhase(metrics.PhaseLayout)
	}
	// Perform layout
	layoutEngine := NewLayoutEngine(width, height)
	layoutStart := time.Now()
	layoutTree := layoutEngine.ComputeLayout(renderTreeCopy)
	r.metrics.RecordComputeLayout(time.Since(layoutStart))
	renderTree = renderTreeCopy
	if recorder != nil {
		recorder.EndPhase(metrics.PhaseLayout)
	}

	if recorder != nil {
		boxes, fragments := countBoxesAndFragments(layoutTree)
		recorder.AddCounters(metrics.Counters{
			BoxCount:      boxes,
			FragmentCount: fragments,
		})
	}

	// Cache trees for viewport updates
	r.currentRenderTree = renderTree
	r.currentLayoutTree = layoutTree
	r.dirty = false
	r.treeMu.Unlock()

	// Load external CSS asynchronously (synchronously in testing mode)
	if r.testingMode {
		r.loadExternalCSS(ctx, doc)
		// Re-read current render tree and layout tree since Refresh() re-cloned them
		r.treeMu.RLock()
		renderTree = r.currentRenderTree
		layoutTree = r.currentLayoutTree
		r.treeMu.RUnlock()
	} else {
		go r.loadExternalCSS(ctx, doc)
	}

	// Pass navigation callback to canvas renderer
	r.treeMu.RLock()
	onNav := r.onNavigate
	r.treeMu.RUnlock()
	r.canvasRenderer.SetNavigationCallback(onNav, r.currentURLRead())

	if recorder != nil {
		recorder.BeginPhase(metrics.PhaseRaster)
	}
	// Render to canvas with viewport optimization
	viewportStart := time.Now()
	canvasObject := r.canvasRenderer.RenderWithViewport(renderTree, layoutTree)
	r.metrics.RecordRenderWithViewport(time.Since(viewportStart))
	if recorder != nil {
		recorder.EndPhase(metrics.PhaseRaster)
	}

	if recorder != nil {
		// Read cached display list command count
		r.canvasRenderer.mu.RLock()
		if r.canvasRenderer.cachedDisplayList != nil {
			recorder.AddCounters(metrics.Counters{
				DisplayItemCount: len(r.canvasRenderer.cachedDisplayList.Commands),
			})
		}
		r.canvasRenderer.mu.RUnlock()

		// Count images from renderTree
		images := countImages(renderTree)
		recorder.AddCounters(metrics.Counters{
			ImageCount: images,
		})
	}

	r.imageLoader.SetOnLoadCallback(r.onImageLoaded)
	r.loadImages(renderTree)

	return canvasObject, nil
}

// ExternalCSS is one externally-fetched stylesheet delivered to the
// renderer. Source is the raw CSS body; URL is the resolved URL used
// for diagnostics and CSP error messages. Callers are expected to
// have already CSP-checked and resolved the URL against the document.
type ExternalCSS struct {
	URL    string
	Source []byte
}

// RenderParsed renders a pre-parsed HTML node with the supplied
// external stylesheets and returns a Fyne canvas object. It is the
// snapshot entry point introduced by M3 of the resource pipeline
// plan: callers (e.g. cmd/browser after the documentloader coordinator
// has fetched all stylesheets) hand in a fully assembled set of CSS
// sources in document order, and the renderer blocks only on the
// render itself — no async CSS loading happens here.
//
// Behavior:
//   - Inline <style> tag contents are extracted from doc and merged
//     with external in source order (inline first, external appended
//     in the order provided).
//   - r.loadExternalCSS is NOT called. The caller is responsible for
//     any further external CSS loading.
//   - On a parse failure of doc, the error is returned.
//
// RenderHTML remains the legacy entry point for direct callers (tests,
// demos, headless renderer); it continues to call loadExternalCSS
// asynchronously and is unchanged in observable behavior.
func (r *Renderer) RenderParsed(ctx context.Context, doc *html.Node, externalCSS []ExternalCSS) (fyne.CanvasObject, error) {
	start := time.Now()
	defer func() { r.metrics.RecordRenderHTML(time.Since(start)) }()

	globalTableColumnCache.Clear()
	recorder := metrics.RecorderFromContext(ctx)
	if recorder != nil {
		recorder.BeginPhase(metrics.PhaseParse)
	}

	if recorder != nil {
		recorder.AddCounters(metrics.Counters{
			NodeCount: countHTMLNodes(doc),
		})
	}

	// Assemble stylesheet: inline + external (in source order).
	inline := extractAndParseCSS(doc)
	assembled := mergeInlineAndExternalCSS(inline, externalCSS)

	r.stylesheetMu.Lock()
	r.stylesheet = assembled
	r.stylesheetMu.Unlock()

	if recorder != nil && r.stylesheet != nil {
		rules, selectors := countRulesAndSelectors(r.stylesheet)
		recorder.AddCounters(metrics.Counters{
			RuleCount:     rules,
			SelectorCount: selectors,
		})
	}

	if recorder != nil {
		recorder.EndPhase(metrics.PhaseParse)
	}

	return r.renderParsedInner(ctx, doc, recorder), nil
}

// renderParsedInner runs style, layout, raster, and image loading for a
// pre-assembled document and stylesheet. Shared helper used by
// RenderParsed and by tests that want to drive these phases directly
// without going through RenderHTML.
func (r *Renderer) renderParsedInner(ctx context.Context, doc *html.Node, recorder *metrics.Recorder) fyne.CanvasObject {
	// Find html element
	htmlNode := findHTMLNode(doc)
	if htmlNode == nil {
		// No html found, use the entire document
		htmlNode = doc
	}

	// Build render tree
	renderTree := BuildRenderTree(htmlNode)
	if renderTree == nil {
		// Return empty container if no content
		return r.canvasRenderer.Render(nil)
	}

	r.treeMu.RLock()
	width, height := r.layoutEngine.canvasWidth, r.layoutEngine.canvasHeight
	r.treeMu.RUnlock()

	r.treeMu.Lock()

	if recorder != nil {
		recorder.BeginPhase(metrics.PhaseStyle)
	}
	// Apply styles
	renderTreeCopy := renderTree.Clone()
	r.stylesheetMu.RLock()
	if r.stylesheet != nil {
		styleManager := NewStyleManagerWithViewport(r.stylesheet, width, height)
		styleManager.ApplyStyles(renderTreeCopy)
	}
	r.stylesheetMu.RUnlock()
	if recorder != nil {
		recorder.EndPhase(metrics.PhaseStyle)
	}

	if recorder != nil {
		recorder.BeginPhase(metrics.PhaseLayout)
	}
	// Perform layout
	layoutEngine := NewLayoutEngine(width, height)
	layoutStart := time.Now()
	layoutTree := layoutEngine.ComputeLayout(renderTreeCopy)
	r.metrics.RecordComputeLayout(time.Since(layoutStart))
	renderTree = renderTreeCopy
	if recorder != nil {
		recorder.EndPhase(metrics.PhaseLayout)

		boxes, fragments := countBoxesAndFragments(layoutTree)
		recorder.AddCounters(metrics.Counters{
			BoxCount:      boxes,
			FragmentCount: fragments,
		})
	}

	// Cache trees for viewport updates
	r.currentRenderTree = renderTree
	r.currentLayoutTree = layoutTree
	r.dirty = false
	r.treeMu.Unlock()

	// Pass navigation callback to canvas renderer
	r.treeMu.RLock()
	onNav := r.onNavigate
	r.treeMu.RUnlock()
	r.canvasRenderer.SetNavigationCallback(onNav, r.currentURLRead())

	if recorder != nil {
		recorder.BeginPhase(metrics.PhaseRaster)
	}
	// Render to canvas with viewport optimization
	viewportStart := time.Now()
	canvasObject := r.canvasRenderer.RenderWithViewport(renderTree, layoutTree)
	r.metrics.RecordRenderWithViewport(time.Since(viewportStart))
	if recorder != nil {
		recorder.EndPhase(metrics.PhaseRaster)

		// Read cached display list command count
		r.canvasRenderer.mu.RLock()
		if r.canvasRenderer.cachedDisplayList != nil {
			recorder.AddCounters(metrics.Counters{
				DisplayItemCount: len(r.canvasRenderer.cachedDisplayList.Commands),
			})
		}
		r.canvasRenderer.mu.RUnlock()

		// Count images from renderTree
		images := countImages(renderTree)
		recorder.AddCounters(metrics.Counters{
			ImageCount: images,
		})
	}

	r.imageLoader.SetOnLoadCallback(r.onImageLoaded)
	r.loadImages(renderTree)

	_ = ctx // reserved for future per-frame recorder hooks
	return canvasObject
}

// mergeInlineAndExternalCSS combines inline <style> rules with external
// stylesheets in source order. The external list is the order returned
// by the documentloader coordinator. Each external entry is parsed
// independently; parse failures are skipped silently and reported via
// the renderer's logger when one is configured.
//
// shouldAttemptParseExternalCSS is the same gate loadExternalCSS uses
// to refuse non-CSS bodies (e.g. an HTML 404 page). Keeping the gate
// here ensures RenderParsed and loadExternalCSS behave consistently
// when fed the same source bytes.
func mergeInlineAndExternalCSS(inline *css.StyleSheet, external []ExternalCSS) *css.StyleSheet {
	if inline == nil {
		inline = &css.StyleSheet{}
	}
	for _, e := range external {
		body := string(e.Source)
		if !shouldAttemptParseExternalCSS(body) {
			continue
		}
		parser := css.NewParser(body)
		sheet, err := parser.Parse()
		if err != nil {
			continue
		}
		inline.Rules = append(inline.Rules, sheet.Rules...)
	}
	return inline
}

// SetViewport updates the viewport for optimized rendering during scroll
func (r *Renderer) SetViewport(y, height float32) {
	r.canvasRenderer.SetViewport(y, height)
}

// UpdateViewport re-renders with the current viewport (for scroll updates)
func (r *Renderer) UpdateViewport() fyne.CanvasObject {
	r.treeMu.RLock()
	defer r.treeMu.RUnlock()
	if r.currentRenderTree == nil || r.currentLayoutTree == nil {
		return container.NewVBox()
	}
	return r.canvasRenderer.RenderWithViewport(r.currentRenderTree, r.currentLayoutTree)
}

// GetContentHeight returns the total height of the rendered content
func (r *Renderer) GetContentHeight() float32 {
	r.treeMu.RLock()
	defer r.treeMu.RUnlock()
	if r.currentLayoutTree == nil {
		return 0
	}
	return r.currentLayoutTree.Box.Height
}

// RenderHTMLBody renders just the body content of an HTML document
func (r *Renderer) RenderHTMLBody(htmlContent string) (fyne.CanvasObject, error) {
	globalTableColumnCache.Clear()
	// Use html.ParseFragment to handle content that is expected to be inside a <body> tag.
	// This avoids wrapping the content in an extra <html><body>...</body></html> structure.
	nodes, err := html.ParseFragment(strings.NewReader(htmlContent), &html.Node{
		Type:     html.ElementNode,
		Data:     "body",
		DataAtom: atom.Body,
	})
	if err != nil {
		return nil, err
	}

	// Create a new root node to hold the parsed fragment.
	root := &html.Node{
		Type:     html.ElementNode,
		Data:     "body",
		DataAtom: atom.Body,
	}
	for _, node := range nodes {
		root.AppendChild(node)
	}

	// Build the render tree from the fragment.
	renderTree := BuildRenderTree(root)
	if renderTree == nil {
		return r.canvasRenderer.Render(nil), nil
	}

	r.treeMu.RLock()
	width, height := r.layoutEngine.canvasWidth, r.layoutEngine.canvasHeight
	r.treeMu.RUnlock()

	r.treeMu.Lock()
	// Apply styles
	renderTreeCopy := renderTree.Clone()
	r.stylesheetMu.RLock()
	styleManager := NewStyleManagerWithViewport(r.stylesheet, width, height)
	styleManager.ApplyStyles(renderTreeCopy)
	r.stylesheetMu.RUnlock()

	// Perform layout.
	layoutEngine := NewLayoutEngine(width, height)
	layoutTree := layoutEngine.ComputeLayout(renderTreeCopy)
	renderTree = renderTreeCopy

	// Cache trees for viewport updates.
	r.currentRenderTree = renderTree
	r.currentLayoutTree = layoutTree
	r.dirty = false
	r.treeMu.Unlock()

	// Pass navigation callback to canvas renderer.
	r.treeMu.RLock()
	onNav := r.onNavigate
	r.treeMu.RUnlock()
	r.canvasRenderer.SetNavigationCallback(onNav, r.currentURLRead())

	// Render to canvas with viewport optimization.
	canvasObject := r.canvasRenderer.RenderWithViewport(renderTree, layoutTree)

	return canvasObject, nil
}

// findBodyNode finds the body element in an HTML document
func findBodyNode(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}

	if node.Type == html.ElementNode && node.Data == "body" {
		return node
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findBodyNode(child); found != nil {
			return found
		}
	}

	return nil
}

// findHTMLNode finds the html element in an HTML document
func findHTMLNode(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}

	if node.Type == html.ElementNode && node.Data == "html" {
		return node
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLNode(child); found != nil {
			return found
		}
	}

	return nil
}

// SetSize updates the renderer dimensions
func (r *Renderer) SetSize(width, height float32) {
	r.treeMu.Lock()
	defer r.treeMu.Unlock()
	r.layoutEngine.canvasWidth = width
	r.layoutEngine.canvasHeight = height
	r.canvasRenderer.canvasWidth = width
	r.canvasRenderer.canvasHeight = height
	r.dirty = true
}

// SetImageLoader sets the image loader for the renderer
func (r *Renderer) SetImageLoader(loader imageloader.Loader) {
	r.imageLoader = loader
	r.canvasRenderer.SetImageLoader(loader)
}

// SetNavigationCallback sets the callback for link clicks
func (r *Renderer) SetNavigationCallback(callback func(url string)) {
	r.onNavigate = callback
}

// SetCurrentURL sets the current page URL for resolving relative links
func (r *Renderer) SetCurrentURL(url string) {
	r.currentURLMu.Lock()
	r.currentURL = url
	r.currentURLMu.Unlock()
}

// SetCSP sets the Content-Security-Policy for style-src enforcement on
// external stylesheets. Pass nil to clear the policy.
func (r *Renderer) SetCSP(p *net.CSPPolicy) {
	r.cspMu.Lock()
	defer r.cspMu.Unlock()
	r.csp = p
}

// ResolveURL resolves a relative or absolute URL against the current page URL
func (r *Renderer) ResolveURL(href string) string {
	return r.resolveURL(href)
}

// SetWindow sets the Fyne window for the renderer
func (r *Renderer) SetWindow(w fyne.Window) {
	r.canvasRenderer.SetWindow(w)
}

// SetInspectCallback sets the callback for element inspection
func (r *Renderer) SetInspectCallback(callback func(node *RenderNode, layout *LayoutBox)) {
	r.onInspect = callback
	r.canvasRenderer.SetInspectCallback(callback, r)
}

// SetContextMenuCallback forwards to the underlying canvas renderer.
// See CanvasRenderer.SetContextMenuCallback for details.
func (r *Renderer) SetContextMenuCallback(callback func(node *RenderNode, layout *LayoutBox, abs fyne.Position)) {
	if r.canvasRenderer == nil {
		return
	}
	r.canvasRenderer.SetContextMenuCallback(callback)
}

// SetHighlightNode sets the node to highlight in the viewport
func (r *Renderer) SetHighlightNode(node *RenderNode) {
	r.canvasRenderer.SetHighlightNode(node)
}

// GetLayoutBox returns the computed layout box associated with the given node
func (r *Renderer) GetLayoutBox(node *RenderNode) *LayoutBox {
	r.treeMu.RLock()
	defer r.treeMu.RUnlock()
	if node == nil || r.currentLayoutTree == nil {
		return nil
	}
	return findLayoutBoxForNode(r.currentLayoutTree, node.ID)
}

// GetRoot returns the current render tree root
func (r *Renderer) GetRoot() *RenderNode {
	r.treeMu.RLock()
	defer r.treeMu.RUnlock()
	return r.currentRenderTree
}

// Refresh re-calculates styles and layout only when dirty, then triggers a refresh.
// When the renderer is clean (no mutations since last render), style and layout
// recomputation is skipped to avoid unnecessary work.
func (r *Renderer) Refresh() {
	r.treeMu.Lock()
	if r.currentRenderTree == nil {
		r.treeMu.Unlock()
		return
	}

	if r.dirty {
		width, height := r.layoutEngine.canvasWidth, r.layoutEngine.canvasHeight

		// Apply styles (in case attributes changed)
		r.stylesheetMu.RLock()
		styleManager := NewStyleManagerWithViewport(r.stylesheet, width, height)
		r.stylesheetMu.RUnlock()
		renderTreeCopy := r.currentRenderTree.Clone()
		styleManager.ApplyStyles(renderTreeCopy)

		// Perform layout
		layoutEngine := NewLayoutEngine(width, height)
		r.currentLayoutTree = layoutEngine.ComputeLayout(renderTreeCopy)
		r.currentRenderTree = renderTreeCopy

		// Clear canvas cache
		r.canvasRenderer.ClearCache()
		r.canvasRenderer.cachedRenderRoot = nil
		r.canvasRenderer.cachedLayoutRoot = nil

		r.dirty = false
	}
	r.treeMu.Unlock()

	// Trigger refresh callback
	if r.onRefresh != nil {
		r.onRefresh()
	}
}

// MarkDirty marks the renderer as needing style/layout recomputation.
// The next call to Refresh() will recompute styles and layout.
func (r *Renderer) MarkDirty() {
	r.treeMu.Lock()
	defer r.treeMu.Unlock()
	r.dirty = true
}

// IsDirty reports whether the renderer needs style/layout recomputation.
func (r *Renderer) IsDirty() bool {
	r.treeMu.RLock()
	defer r.treeMu.RUnlock()
	return r.dirty
}

// SetRefreshCallback sets the callback for refresh events
func (r *Renderer) SetRefreshCallback(callback func()) {
	r.onRefresh = callback
}

// HitTest finds the element at the given coordinates (relative to the content)
// Returns the render node and layout box if found, or nil if not found
func (r *Renderer) HitTest(x, y float32) (*RenderNode, *LayoutBox) {
	r.treeMu.RLock()
	defer r.treeMu.RUnlock()
	if r.currentLayoutTree == nil || r.currentRenderTree == nil {
		return nil, nil
	}

	// Find the deepest layout box that contains the point
	layoutBox := r.hitTestLayout(r.currentLayoutTree, x, y)
	if layoutBox == nil {
		return nil, nil
	}

	// Find the corresponding render node
	renderNode := r.findRenderNodeByID(r.currentRenderTree, layoutBox.NodeID)
	return renderNode, layoutBox
}

// hitTestLayout recursively finds the deepest layout box containing the point
func (r *Renderer) hitTestLayout(layoutBox *LayoutBox, x, y float32) *LayoutBox {
	if layoutBox == nil {
		return nil
	}

	// Check if this box contains the point
	if !layoutBox.Contains(x, y) {
		return nil
	}

	// Check children (in reverse order to get the topmost element)
	for i := len(layoutBox.Children) - 1; i >= 0; i-- {
		child := layoutBox.Children[i]
		if result := r.hitTestLayout(child, x, y); result != nil {
			return result
		}
	}

	// This is the deepest box containing the point
	return layoutBox
}

// findRenderNodeByID finds a render node by its ID
func (r *Renderer) findRenderNodeByID(node *RenderNode, id int64) *RenderNode {
	if node == nil {
		return nil
	}

	if node.ID == id {
		return node
	}

	for _, child := range node.Children {
		if result := r.findRenderNodeByID(child, id); result != nil {
			return result
		}
	}

	return nil
}

func (r *Renderer) loadImages(node *RenderNode) {
	if node.TagName == "img" {
		if src, ok := node.GetAttribute("src"); ok {
			// Resolve relative URLs before loading
			resolvedSrc := r.resolveURL(src)
			go func() {
				img, err := r.imageLoader.Load(resolvedSrc)
				if err == nil {
					// Need to lock when updating the node directly since we might be cloning/re-rendering
					r.treeMu.Lock()
					node.ImageData = img
					r.treeMu.Unlock()
					// Trigger refresh when image is loaded
					r.onImageLoaded(resolvedSrc)
				}
			}()
		}
	}
	for _, child := range node.Children {
		r.loadImages(child)
	}
}

// currentURLRead returns the current page URL, safe for concurrent access.
func (r *Renderer) currentURLRead() string {
	r.currentURLMu.RLock()
	defer r.currentURLMu.RUnlock()
	return r.currentURL
}

// resolveURL resolves a relative or absolute URL against the current page URL
func (r *Renderer) resolveURL(href string) string {
	// If href is already absolute, return as-is
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}

	curURL := r.currentURLRead()

	if strings.HasPrefix(href, "//") {
		scheme := "https:"
		if curURL != "" {
			if parsed, err := url.Parse(curURL); err == nil && parsed.Scheme != "" {
				scheme = parsed.Scheme + ":"
			}
		}
		return scheme + href
	}

	// If no current URL, return href as-is
	if curURL == "" {
		return href
	}

	// Parse current URL
	baseURL, err := url.Parse(curURL)
	if err != nil {
		return href
	}

	// Parse relative href
	relURL, err := url.Parse(href)
	if err != nil {
		return href
	}

	// Resolve relative URL against base
	resolved := baseURL.ResolveReference(relURL)
	return resolved.String()
}

func (r *Renderer) SetTestingMode(mode bool) {
	r.testingMode = mode
}

func (r *Renderer) SetHeadless(mode bool) {
	r.headless = mode
	if r.canvasRenderer != nil {
		r.canvasRenderer.SetHeadless(mode)
	}
}

func (r *Renderer) onImageLoaded(src string) {
	r.treeMu.Lock()
	r.updateNodeImageData(r.currentRenderTree, src)
	r.treeMu.Unlock()

	r.canvasRenderer.InvalidateObjectCache()
	r.MarkDirty()
	r.Refresh()
}

func (r *Renderer) updateNodeImageData(node *RenderNode, src string) {
	if node == nil {
		return
	}
	if node.TagName == "img" {
		if nodeSrc, ok := node.GetAttribute("src"); ok {
			resolvedSrc := r.resolveURL(nodeSrc)
			if resolvedSrc == src {
				cache := r.imageLoader.GetCache()
				if cache != nil {
					if cached := cache.Get(src); cached != nil {
						node.ImageData = cached
					}
				} else {
					if img, err := r.imageLoader.Load(src); err == nil {
						node.ImageData = img
					}
				}
			}
		}
	}
	for _, child := range node.Children {
		r.updateNodeImageData(child, src)
	}
}

func shouldAttemptParseExternalCSS(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "<") {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "not found") && !strings.Contains(trimmed, "{") && !strings.HasPrefix(trimmed, "@") {
		return false
	}
	return true
}

// loadExternalCSS finds and loads external stylesheets
func (r *Renderer) loadExternalCSS(ctx context.Context, doc *html.Node) {
	links := extractExternalLinks(doc)
	log.Printf("loadExternalCSS found links: %v", links)

	// Read CSP policy and current URL for CSP source matching.
	r.cspMu.RLock()
	csp := r.csp
	r.cspMu.RUnlock()
	r.currentURLMu.RLock()
	currentURL := r.currentURL
	r.currentURLMu.RUnlock()

	var baseURL *url.URL
	if currentURL != "" {
		baseURL, _ = url.Parse(currentURL)
	}

	for _, href := range links {
		if ctx.Err() != nil {
			return
		}
		// Resolve URL
		resolvedURL := r.ResolveURL(href)
		log.Printf("loadExternalCSS fetching resolved URL: %s", resolvedURL)

		// Check CSP style-src before fetching.
		if csp != nil {
			if err := csp.AllowStyle(resolvedURL, baseURL); err != nil {
				log.Printf("loadExternalCSS: CSP blocked stylesheet %s: %v", resolvedURL, err)
				if r.Logger != nil {
					r.Logger.Warn("CSP blocked stylesheet", "url", resolvedURL, "err", err)
				}
				continue
			}
		}

		// Fetch CSS
		content, err := r.fetcher.FetchWithContext(ctx, resolvedURL, nil)
		if err != nil {
			log.Printf("loadExternalCSS: Failed to fetch CSS %s: %v", resolvedURL, err)
			if r.Logger != nil {
				r.Logger.Warn("Failed to fetch CSS", "url", resolvedURL, "err", err)
			}
			continue
		}
		log.Printf("loadExternalCSS: Successfully fetched CSS %s, length=%d", resolvedURL, len(content))
		if !shouldAttemptParseExternalCSS(content) {
			log.Printf("loadExternalCSS: Skipping parse for CSS %s (shouldAttemptParseExternalCSS returned false)", resolvedURL)
			continue
		}

		if ctx.Err() != nil {
			return
		}

		// Parse CSS
		parser := css.NewParser(content)
		stylesheet, err := parser.Parse()
		if err != nil {
			log.Printf("loadExternalCSS: Failed to parse CSS %s: %v", resolvedURL, err)
			if r.Logger != nil {
				r.Logger.Warn("Failed to parse CSS", "url", resolvedURL, "err", err)
			}
			continue
		}
		log.Printf("loadExternalCSS: Successfully parsed CSS %s, rules count=%d", resolvedURL, len(stylesheet.Rules))

		// Append rules to current stylesheet safely
		// Note: This simple append assumes r.stylesheet is safe to modify or we are lucky.
		// Since we are inside a goroutine and Refresh reads it, we should ideally lock.
		// But for now, we'll update it inside fyne.Do to be safe with UI refresh.

		updateCSS := func() {
			r.stylesheetMu.Lock()
			if r.stylesheet == nil {
				r.stylesheet = stylesheet
			} else {
				newStylesheet := &css.StyleSheet{
					Rules:   append(append([]css.Rule(nil), r.stylesheet.Rules...), stylesheet.Rules...),
					AtRules: append(append([]css.AtRule(nil), r.stylesheet.AtRules...), stylesheet.AtRules...),
				}
				r.stylesheet = newStylesheet
				// r.stylesheet.AtRules = append(r.stylesheet.AtRules, stylesheet.AtRules...)
			}
			r.stylesheetMu.Unlock()

			if recorder := metrics.RecorderFromContext(ctx); recorder != nil {
				rules, selectors := countRulesAndSelectors(stylesheet)
				recorder.AddCounters(metrics.Counters{
					RuleCount:     rules,
					SelectorCount: selectors,
				})
			}

			// Mark dirty so Refresh() recomputes style and layout with new CSS.
			r.MarkDirty()
			r.Refresh()
		}

		if r.testingMode {
			updateCSS()
		} else {
			updateCSS()
		}
	}
}

// extractExternalLinks finds all <link rel="stylesheet"> tags and returns their hrefs
func extractExternalLinks(node *html.Node) []string {
	var links []string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "link" {
			isStylesheet := false
			href := ""
			for _, attr := range n.Attr {
				if attr.Key == "rel" && strings.Contains(strings.ToLower(attr.Val), "stylesheet") {
					isStylesheet = true
				}
				if attr.Key == "href" {
					href = attr.Val
				}
			}
			if isStylesheet && href != "" {
				links = append(links, href)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(node)
	return links
}

// extractAndParseCSS finds all <style> tags, extracts their content, and parses it.
func extractAndParseCSS(node *html.Node) *css.StyleSheet {
	var cssContent string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "style" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					cssContent += c.Data
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(node)

	if cssContent == "" {
		return &css.StyleSheet{}
	}

	parser := css.NewParser(cssContent)
	stylesheet, err := parser.Parse()
	if err != nil {
		// For now, we'll just ignore CSS parsing errors.
		// A more robust solution would involve logging or displaying an error.
		return &css.StyleSheet{}
	}
	return stylesheet
}

// Helper functions for counting metrics

func countHTMLNodes(n *html.Node) int {
	if n == nil {
		return 0
	}
	count := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		count += countHTMLNodes(c)
	}
	return count
}

func countRulesAndSelectors(ss *css.StyleSheet) (rules int, selectors int) {
	if ss == nil {
		return 0, 0
	}

	var countRules func([]css.Rule)
	countRules = func(rls []css.Rule) {
		rules += len(rls)
		for _, r := range rls {
			selectors += len(r.Selectors)
		}
	}

	countRules(ss.Rules)

	var countAtRules func([]css.AtRule)
	countAtRules = func(atRls []css.AtRule) {
		for _, ar := range atRls {
			countRules(ar.Rules)
			countAtRules(ar.AtRules)
		}
	}

	countAtRules(ss.AtRules)

	return rules, selectors
}

func countBoxesAndFragments(box *LayoutBox) (boxes int, fragments int) {
	if box == nil {
		return 0, 0
	}
	boxes = 1
	for _, lineBox := range box.LineBoxes {
		fragments += len(lineBox.InlineBoxes)
	}
	for _, child := range box.Children {
		cBox, cFrag := countBoxesAndFragments(child)
		boxes += cBox
		fragments += cFrag
	}
	return boxes, fragments
}

func countRenderNodes(n *RenderNode, total, elements, text *int) {
	if n == nil {
		return
	}
	*total++
	switch n.Type {
	case NodeTypeElement:
		*elements++
	case NodeTypeText:
		*text++
	}
	for _, child := range n.Children {
		countRenderNodes(child, total, elements, text)
	}
}

func countLayoutBoxes(box *LayoutBox) int {
	if box == nil {
		return 0
	}
	count := 1
	for _, child := range box.Children {
		count += countLayoutBoxes(child)
	}
	return count
}

func countImages(node *RenderNode) int {
	if node == nil {
		return 0
	}
	count := 0
	if node.TagName == "img" {
		count = 1
	}
	for _, child := range node.Children {
		count += countImages(child)
	}
	return count
}

// SetSubmitting delegates the submitting status to CanvasRenderer
func (r *Renderer) SetSubmitting(submitting bool) {
	r.canvasRenderer.SetSubmitting(submitting)
}

// GetDisplayListSummary returns command type counts from the canvas renderer's
// cached display list. Returns nil when no display list has been built.
func (r *Renderer) GetDisplayListSummary() map[string]int {
	return r.canvasRenderer.DisplayListSummary()
}

// GetDisplayListCommands returns a copy of the current display list commands
// for inspection. Returns nil when no display list has been built.
func (r *Renderer) GetDisplayListCommands() []PaintCommand {
	return r.canvasRenderer.DisplayListCommands()
}

// SetDirtyOverlayEnabled enables or disables the dirty-region overlay visualization.
func (r *Renderer) SetDirtyOverlayEnabled(enabled bool) {
	r.canvasRenderer.SetDirtyOverlayEnabled(enabled)
}

// DirtyOverlayEnabled returns whether the dirty-region overlay is enabled.
func (r *Renderer) DirtyOverlayEnabled() bool {
	return r.canvasRenderer.DirtyOverlayEnabled()
}

// GetDOMNodeCounts returns the total, element, and text node counts
// from the current render tree.
func (r *Renderer) GetDOMNodeCounts() (total int, elements int, text int) {
	r.treeMu.RLock()
	defer r.treeMu.RUnlock()
	countRenderNodes(r.currentRenderTree, &total, &elements, &text)
	return
}

// GetLayoutNodeCount returns the number of layout boxes in the
// current layout tree.
func (r *Renderer) GetLayoutNodeCount() int {
	r.treeMu.RLock()
	defer r.treeMu.RUnlock()
	return countLayoutBoxes(r.currentLayoutTree)
}

// GetStyleSheet returns the current stylesheet.
func (r *Renderer) GetStyleSheet() *css.StyleSheet {
	r.stylesheetMu.RLock()
	defer r.stylesheetMu.RUnlock()
	return r.stylesheet
}

// GetMatchedRules returns all CSS rules matching the given node, sorted by specificity.
func (r *Renderer) GetMatchedRules(node *RenderNode) []css.Rule {
	r.stylesheetMu.RLock()
	defer r.stylesheetMu.RUnlock()
	sm := NewStyleManager(r.stylesheet)
	return sm.MatchRules(node)
}
