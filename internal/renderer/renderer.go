package renderer

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"log/slog"
	"net/url"
	"strings"

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
	incremental    *IncrementalLayoutEngine
	canvasRenderer *CanvasRenderer
	chunkedDisplay *ChunkedDisplayList
	imageLoader    imageloader.Loader
	stylesheet     *css.StyleSheet
	fetcher        *net.Fetcher

	// Cached trees for performance
	currentRenderTree *RenderNode
	currentLayoutTree *LayoutBox
	treeMu            sync.RWMutex

	// buildSeq is a monotonic build counter. buildFrame hands off its
	// trees only when its sequence is still the newest, so a slower build
	// for an older render intent cannot clobber a newer build's trees.
	buildSeq atomic.Uint64
	// lastRecorder is the navigation recorder of the most recent build,
	// stashed under treeMu so PresentFrame can time the raster phase on
	// the same navigation the built trees belong to.
	lastRecorder *metrics.Recorder

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

	// imageBatcher collapses image-loaded callbacks into one flush per
	// window so an image-heavy page performs one style+layout+present
	// cycle per burst instead of one per completed image (PR7).
	imageBatcher *ImageLoadBatcher

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

	r := &Renderer{
		layoutEngine:   NewLayoutEngine(width, height),
		incremental:    NewIncrementalLayoutEngine(width, height),
		canvasRenderer: canvasRenderer,
		chunkedDisplay: NewChunkedDisplayList(NewDisplayCommandList(), NewPaintChunkList()),
		imageLoader:    imageLoader,
		fetcher:        net.NewFetcher(),
		Logger:         slog.Default(),
		metrics:        NewRenderMetrics(),
	}
	// The canvas is owned by this renderer, so its image-load callback
	// delegates to the renderer's batched owner (PR12): the loader's
	// single callback slot always lands on the PR7 batch path even when
	// SetWindow runs after a present.
	canvasRenderer.renderer = r
	r.imageBatcher = NewImageLoadBatcher(16*time.Millisecond, r.flushImageBatch)
	return r
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

// RenderHTML renders HTML content and returns a Fyne canvas object. It is
// the legacy single-phase entry point: it builds the engine state (parse,
// style, layout) and then presents the frame. Callers running off the
// Fyne main thread (navigation, mutations) should prefer the two-phase
// BuildHTML + PresentFrame pair so the heavy engine phases do not block
// the UI thread.
func (r *Renderer) RenderHTML(ctx context.Context, htmlContent string) (fyne.CanvasObject, error) {
	if err := r.BuildHTML(ctx, htmlContent); err != nil {
		return nil, err
	}
	return r.PresentFrame(), nil
}

// BuildHTML performs the engine phases of rendering — HTML parse,
// stylesheet assembly, style resolution, and layout — and caches the
// resulting trees for PresentFrame. It performs no Fyne work and is safe
// to call off the UI thread: this is the PR4 "build" half of the render
// split. Legacy external-CSS loading is kicked off here exactly as
// RenderHTML did (asynchronous in normal mode, synchronous under
// testingMode).
func (r *Renderer) BuildHTML(ctx context.Context, htmlContent string) error {
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
		return err
	}

	if recorder != nil {
		recorder.AddCounters(metrics.Counters{
			NodeCount: countHTMLNodes(doc),
		})
	}

	// Extract and parse CSS from <style> tags. The sheet is captured
	// locally so a concurrent build cannot overwrite the stylesheet
	// between parse and style application (see buildFrame).
	sheet := extractAndParseCSS(doc)
	r.stylesheetMu.Lock()
	r.stylesheet = sheet
	r.stylesheetMu.Unlock()

	if recorder != nil && sheet != nil {
		rules, selectors := countRulesAndSelectors(sheet)
		recorder.AddCounters(metrics.Counters{
			RuleCount:     rules,
			SelectorCount: selectors,
		})
	}

	if recorder != nil {
		recorder.EndPhase(metrics.PhaseParse)
	}

	if err := r.buildFrame(ctx, doc, sheet, recorder); err != nil {
		return err
	}

	// Load external CSS asynchronously (synchronously in testing mode),
	// preserving the legacy RenderHTML behavior.
	if r.testingMode {
		r.loadExternalCSS(ctx, doc)
	} else {
		go r.loadExternalCSS(ctx, doc)
	}

	return nil
}

// buildFrame applies styles and computes layout for a parsed document,
// caching the resulting trees for PresentFrame. Style and layout run on
// local trees without holding the tree lock, so a background build never
// blocks a concurrent scroll render on the UI thread; only the final
// handoff is atomic. A build that has been superseded by a newer one
// (buildSeq mismatch) or cancelled via ctx skips the handoff so a stale
// frame is never painted.
func (r *Renderer) buildFrame(ctx context.Context, doc *html.Node, sheet *css.StyleSheet, recorder *metrics.Recorder) error {
	// Find html element
	htmlNode := findHTMLNode(doc)
	if htmlNode == nil {
		// No html found, use the entire document
		htmlNode = doc
	}

	// Build render tree
	renderTree := BuildRenderTree(htmlNode)
	if renderTree == nil {
		// Empty document: PresentFrame renders an empty canvas.
		return nil
	}

	r.treeMu.RLock()
	width, height := r.layoutEngine.canvasWidth, r.layoutEngine.canvasHeight
	r.treeMu.RUnlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if recorder != nil {
		recorder.BeginPhase(metrics.PhaseStyle)
	}
	// Apply styles
	renderTreeCopy := renderTree.Clone()
	if sheet != nil {
		styleManager := NewStyleManagerWithViewport(sheet, width, height)
		styleManager.ApplyStyles(renderTreeCopy)
	}
	if recorder != nil {
		recorder.EndPhase(metrics.PhaseStyle)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if recorder != nil {
		recorder.BeginPhase(metrics.PhaseLayout)
	}
	// Perform layout
	layoutEngine := NewLayoutEngine(width, height)
	layoutStart := time.Now()
	layoutTree := layoutEngine.ComputeLayout(renderTreeCopy)
	r.metrics.RecordComputeLayout(time.Since(layoutStart))
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

	// Hand off atomically; skip when a newer build already owns the trees.
	seq := r.buildSeq.Add(1)
	r.treeMu.Lock()
	defer r.treeMu.Unlock()
	if seq != r.buildSeq.Load() {
		return nil
	}
	r.currentRenderTree = renderTreeCopy
	r.currentLayoutTree = layoutTree
	r.dirty = false
	r.lastRecorder = recorder
	return nil
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
// legacy single-phase snapshot entry point introduced by M3 of the
// resource pipeline plan: callers (e.g. cmd/browser after the
// documentloader coordinator has fetched all stylesheets) hand in a
// fully assembled set of CSS sources in document order, and the
// renderer blocks only on the render itself — no async CSS loading
// happens here. Callers running off the Fyne main thread should prefer
// the two-phase BuildParsed + PresentFrame pair.
//
// Behavior:
//   - Inline <style> tag contents are extracted from doc and merged
//     with external in source order (inline first, external appended
//     in the order provided).
//   - r.loadExternalCSS is NOT called. The caller is responsible for
//     any further external CSS loading.
//   - On a parse failure of doc, the error is returned.
func (r *Renderer) RenderParsed(ctx context.Context, doc *html.Node, externalCSS []ExternalCSS) (fyne.CanvasObject, error) {
	if err := r.BuildParsed(ctx, doc, externalCSS); err != nil {
		return nil, err
	}
	return r.PresentFrame(), nil
}

// BuildParsed performs the engine phases — render-tree construction,
// style resolution, and layout — for an already-parsed document with
// pre-assembled external stylesheets, caching the result for
// PresentFrame. It performs no Fyne work and is safe to call off the
// UI thread: this is the PR4 "build" half of the render split used by
// the documentloader coordinator and mutation paths.
func (r *Renderer) BuildParsed(ctx context.Context, doc *html.Node, externalCSS []ExternalCSS) error {
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

	// Assemble stylesheet: inline + external (in source order). The
	// sheet is captured locally so a concurrent build cannot overwrite
	// the stylesheet between parse and style application (buildFrame).
	inline := extractAndParseCSS(doc)
	assembled := mergeInlineAndExternalCSS(inline, externalCSS)

	r.stylesheetMu.Lock()
	r.stylesheet = assembled
	r.stylesheetMu.Unlock()

	if recorder != nil && assembled != nil {
		rules, selectors := countRulesAndSelectors(assembled)
		recorder.AddCounters(metrics.Counters{
			RuleCount:     rules,
			SelectorCount: selectors,
		})
	}

	if recorder != nil {
		recorder.EndPhase(metrics.PhaseParse)
	}

	return r.buildFrame(ctx, doc, assembled, recorder)
}

// PresentFrame builds the Fyne canvas object for the trees cached by the
// most recent BuildHTML/BuildParsed call. It MUST run on the Fyne main
// thread: it constructs and mutates Fyne canvas objects (the PR4
// "present" half of the render split). The heavy engine phases already
// ran in the Build* call, possibly on a worker goroutine.
func (r *Renderer) PresentFrame() fyne.CanvasObject {
	r.treeMu.RLock()
	renderTree := r.currentRenderTree
	layoutTree := r.currentLayoutTree
	onNav := r.onNavigate
	recorder := r.lastRecorder
	r.treeMu.RUnlock()

	r.canvasRenderer.SetNavigationCallback(onNav, r.currentURLRead())

	if renderTree == nil || layoutTree == nil {
		// Return empty container if no content
		return r.canvasRenderer.Render(nil)
	}

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

// SetMouseInputCallback forwards to the underlying canvas renderer.
// See CanvasRenderer.SetMouseInputCallback for details.
func (r *Renderer) SetMouseInputCallback(poster func(MouseInput)) {
	if r.canvasRenderer == nil {
		return
	}
	r.canvasRenderer.SetMouseInputCallback(poster)
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
	if r.imageBatcher != nil {
		r.imageBatcher.Signal(src)
		return
	}
	// No batcher (tests constructing the renderer directly): fall back to
	// an immediate single-image flush.
	r.flushImageBatch([]string{src})
}

// flushImageBatch applies completed image data to the current render tree
// and triggers exactly one style+layout recompute and refresh for the whole
// batch (PR7): 100 images completing within a window produce one render
// request instead of 100. Runs on the batcher's flush goroutine.
func (r *Renderer) flushImageBatch(srcs []string) {
	if len(srcs) == 0 {
		return
	}
	r.treeMu.Lock()
	for _, src := range srcs {
		r.updateNodeImageData(r.currentRenderTree, src)
	}
	r.treeMu.Unlock()

	r.canvasRenderer.InvalidateObjectCache()
	r.MarkDirty()
	r.Refresh()
	r.RecordCoalescedImages(len(srcs))
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

// SetFPSOverlayEnabled enables or disables the on-screen FPS HUD overlay on
// the active viewport. See CanvasRenderer.SetFPSOverlayEnabled.
func (r *Renderer) SetFPSOverlayEnabled(enabled bool) {
	r.canvasRenderer.SetFPSOverlayEnabled(enabled)
}

// FPSOverlayEnabled returns whether the on-screen FPS HUD overlay is enabled.
func (r *Renderer) FPSOverlayEnabled() bool {
	return r.canvasRenderer.FPSOverlayEnabled()
}

// FPSStats returns the current frame-rate statistics measured by the renderer.
func (r *Renderer) FPSStats() FPSStats {
	return r.canvasRenderer.FPSStats()
}

// FrameMetrics returns the renderer's actionable performance metrics
// snapshot. See CanvasRenderer.FrameMetrics and FrameMetricsSnapshot.
func (r *Renderer) FrameMetrics() FrameMetricsSnapshot {
	return r.canvasRenderer.FrameMetrics()
}

// ScheduleScroll records a new scroll position and asks the canvas to
// coalesce it. Returns true when the call scheduled a new render,
// false when an existing render was reused (i.e. another scroll
// event arrived in the same frame and was collapsed into this one).
func (r *Renderer) ScheduleScroll(y, height float32) bool {
	return r.canvasRenderer.ScheduleScroll(y, height)
}

// TryClaimScroll returns the latest queued viewport and clears the
// pending flag. The caller is responsible for the actual render.
func (r *Renderer) TryClaimScroll() (ScrollViewport, bool) {
	return r.canvasRenderer.TryClaimScroll()
}

// RecordInputToPresent records the time from a user-input event
// (scroll, mutation) to the next presented frame.
func (r *Renderer) RecordInputToPresent(d time.Duration) {
	r.canvasRenderer.RecordInputToPresent(d)
}

// RecordUIQueueWait records how long a piece of work waited on the
// Fyne main thread.
func (r *Renderer) RecordUIQueueWait(d time.Duration) {
	r.canvasRenderer.RecordUIQueueWait(d)
}

// RecordCoalescedMutations records how many JS mutations were
// collapsed into a single render. Owners call this from the
// mutation-coalescer callback so the freeze-fix health checks
// and the on-screen HUD can see the actual rate.
func (r *Renderer) RecordCoalescedMutations(n int) {
	r.canvasRenderer.RecordCoalescedMutations(n)
}

// RecordCoalescedScroll records how many scroll events were
// collapsed into a single render.
func (r *Renderer) RecordCoalescedScroll(n int) {
	r.canvasRenderer.RecordCoalescedScroll(n)
}

// RecordCoalescedImages records how many image-loaded callbacks were
// collapsed into a single render (PR7).
func (r *Renderer) RecordCoalescedImages(n int) {
	r.canvasRenderer.RecordCoalescedImages(n)
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
