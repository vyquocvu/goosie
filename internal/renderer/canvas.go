package renderer

import (
	"flag"
	"fmt"
	"image/color"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	imageloader "github.com/vyquocvu/goosie/internal/image"
)

// linkColorTheme overrides the hyperlink color of a base theme so anchor
// widgets honor the element's computed color instead of the theme's default
// link blue. All other lookups fall through to the base theme.
type linkColorTheme struct {
	fyne.Theme
	link color.Color
}

func (t linkColorTheme) Color(name fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameHyperlink {
		return t.link
	}
	return t.Theme.Color(name, v)
}

var defaultUALinkColor = color.RGBA{R: 0, G: 0, B: 0xee, A: 0xff}

// applyLinkColor wraps a hyperlink widget in a theme override carrying the
// node's computed color. Fyne's Hyperlink hardcodes ColorNameHyperlink in its
// text segment, so a per-widget theme is the only way to recolor it. Returns
// the original object when the node has no computed color or uses the default link color.
func applyLinkColor(node *RenderNode, obj fyne.CanvasObject) fyne.CanvasObject {
	if node == nil || node.ComputedStyle == nil || node.ComputedStyle.Color == nil || node.ComputedStyle.Color == defaultUALinkColor {
		return obj
	}
	return container.NewThemeOverride(obj, linkColorTheme{Theme: theme.Current(), link: node.ComputedStyle.Color})
}

// NavigationCallback is a function that is called when navigation is requested
type NavigationCallback func(url string)

// CanvasRenderer renders a render tree onto a Fyne canvas
type CanvasRenderer struct {
	canvasWidth  float32
	canvasHeight float32
	defaultSize  float32
	window       fyne.Window

	// Viewport for optimized rendering
	viewportY      float32
	viewportHeight float32

	// Cached display list for performance
	cachedDisplayList *DisplayList
	cachedLayoutRoot  *LayoutBox
	cachedRenderRoot  *RenderNode

	// fontMetrics provides accurate text measurement
	fontMetrics *FontMetrics

	// Navigation callback for link clicks
	onNavigate NavigationCallback

	// Current page URL for resolving relative links
	baseURL string

	// Image loader for loading and caching images
	imageLoader imageloader.Loader

	// OnRefresh is a test hook to signal when a refresh is triggered.
	OnRefresh func()

	// Inspect callback for element inspection
	onInspect func(node *RenderNode, layout *LayoutBox)
	// Context menu callback for right-click. Invoked with the absolute
	// canvas position of the right click so the UI layer can place a
	// popup near the cursor.
	onContextMenu func(node *RenderNode, layout *LayoutBox, abs fyne.Position)
	renderer      *Renderer // Reference to main renderer for hit testing
	// mousePoster routes canvas mouse events to the owning Tab (PR9).
	// When nil, the widgets fall back to their legacy direct dispatch.
	mousePoster mouseInputPoster
	mu          sync.RWMutex

	// Object cache: reuses Fyne canvas objects across frames instead of
	// re-creating them on every scroll/render. Keyed by command index in the
	// display list, valid only within the same dlBuildGen.
	objectCache map[int]fyne.CanvasObject
	dlBuildGen  uint64                // bumped every time the display list is rebuilt
	contentRoot *fyne.Container       // stable root container, reused across renders
	inspectable *InspectableContainer // stable inspect wrapper, reused across renders

	submitting      bool
	submittingForms map[int64]bool

	headless            bool
	dirtyOverlayEnabled bool
	Logger              *slog.Logger
	highlightNode       *RenderNode

	// fps measures the presented-frame rate (see fps.go). One frame is
	// recorded per RenderWithViewport call. fpsOverlayEnabled renders a small
	// on-screen FPS readout when true.
	fps               *FPSCounter
	fpsOverlayEnabled bool

	// frameMetrics is the actionable metrics surface for the
	// performance HUD. It tracks render duration, UI-queue wait,
	// input-to-present latency, and the counters that reveal where
	// work is being collapsed or dropped. See framemetrics.go.
	frameMetrics *FrameMetrics

	// scrollCoalescer collapses OnScrolled bursts into a single
	// pending render. The contract: Schedule() records the latest
	// viewport and TryClaim() drains it. Without coalescing, every
	// OnScrolled tick walks the full display list, builds Fyne
	// objects, and triggers a refresh — easily enough to drive
	// scroll-rate FPS into single digits.
	scrollCoalescer *ScrollCoalescer

	// fpsOverlayText / fpsOverlayBg cache the Fyne canvas objects backing
	// the FPS HUD so we mutate them in place across frames instead of
	// allocating a fresh rectangle+text every scroll tick. Reused only
	// while the overlay stays enabled (CreateRenderer is responsible for
	// rebuilding on first show).
	fpsOverlayText      *canvas.Text
	fpsOverlayBg        *canvas.Rectangle
	fpsOverlayTextCache string // last text we set on fpsOverlayText, to skip refreshes
}

// NewCanvasRenderer creates a new canvas renderer
func NewCanvasRenderer(width, height float32) *CanvasRenderer {
	defaultSize := float32(16.0)
	return &CanvasRenderer{
		canvasWidth:     width,
		canvasHeight:    height,
		defaultSize:     defaultSize,
		viewportY:       0,
		viewportHeight:  height,
		fontMetrics:     NewFontMetrics(defaultSize),
		objectCache:     make(map[int]fyne.CanvasObject),
		dlBuildGen:      1,
		submittingForms: make(map[int64]bool),
		fps:             NewFPSCounter(),
		frameMetrics:    NewFrameMetrics(),
		scrollCoalescer: NewScrollCoalescer(),
		Logger:          slog.Default(),
	}
}

// SetLogger sets the structured logger for the CanvasRenderer
func (cr *CanvasRenderer) SetLogger(l *slog.Logger) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if l == nil {
		cr.Logger = slog.Default()
	} else {
		cr.Logger = l
	}
}

// SetWindow sets the Fyne window for the renderer
func (cr *CanvasRenderer) SetWindow(w fyne.Window) {
	cr.window = w
	if cr.imageLoader != nil {
		cr.imageLoader.SetOnLoadCallback(cr.onImageLoaded)
	}
}

func (cr *CanvasRenderer) SetHeadless(headless bool) {
	cr.headless = headless
}

func (cr *CanvasRenderer) onImageLoaded(source string) {
	// When a Renderer owns this canvas, route the completion through the
	// renderer's batched owner (PR7): N images finishing within one window
	// produce one flush → one style+layout+present instead of one refresh
	// per image. Delegating here (rather than registering a second,
	// competing callback) keeps the loader's single callback slot on the
	// batched path regardless of whether SetWindow ran before or after the
	// present that registered it. A standalone canvas (no owning renderer —
	// direct usage and tests) keeps the legacy per-image refresh below.
	if cr.renderer != nil {
		cr.renderer.onImageLoaded(source)
		return
	}
	fn := func() {
		cr.ClearCache()
		if cr.window != nil {
			cr.window.Canvas().Refresh(cr.window.Content())
		}
		if cr.OnRefresh != nil {
			cr.OnRefresh()
		}
	}

	if cr.headless {
		fn()
		return
	}
	// Always go through fyne.Do so that when SetWindow is called later,
	// the callback already holds the refresh and it runs on the main thread.
	// If window is nil at call time, we queue the refresh anyway — it will
	// be a no-op inside the callback but the cache clear still happens.
	fyne.Do(fn)
}

// SetViewport sets the current viewport for optimized rendering
func (cr *CanvasRenderer) SetViewport(y, height float32) {
	cr.viewportY = y
	cr.viewportHeight = height
}

// ScheduleScroll records a new scroll position. The canvas runs the
// actual render on the next tick of the Fyne presentation loop; the
// coalescer ensures only the latest viewport is rendered even when
// the user is scrolling rapidly.
//
// The returned boolean reports whether a new render was scheduled
// (true) or whether an existing pending render was reused (false).
// Callers can use this to feed FrameMetrics.IncCoalescedScroll.
func (cr *CanvasRenderer) ScheduleScroll(y, height float32) bool {
	return cr.scrollCoalescer.Schedule(ScrollViewport{Y: y, Height: height})
}

// TryClaimScroll returns the latest queued viewport and clears the
// pending flag. The caller is responsible for the actual render.
//
// Returns (ScrollViewport{}, false) if no render is pending.
func (cr *CanvasRenderer) TryClaimScroll() (ScrollViewport, bool) {
	return cr.scrollCoalescer.TryClaim()
}

// FrameMetrics returns the renderer's actionable metrics snapshot.
// Exposed so the DevTools performance panel and on-screen HUD can
// show render duration, UI-queue wait, input-to-present latency,
// and coalesced-event counters.
func (cr *CanvasRenderer) FrameMetrics() FrameMetricsSnapshot {
	return cr.frameMetrics.Snapshot()
}

// RecordInputToPresent records the time from a user-input event
// (scroll, mutation) to the next presented frame. Owners call this
// immediately before triggering a render.
func (cr *CanvasRenderer) RecordInputToPresent(d time.Duration) {
	cr.frameMetrics.RecordInputToPresent(d)
}

// RecordUIQueueWait records how long a piece of work waited on the
// Fyne main thread. High values here are a direct signal of UI
// contention.
func (cr *CanvasRenderer) RecordUIQueueWait(d time.Duration) {
	cr.frameMetrics.RecordUIQueueWait(d)
}

// RecordCoalescedMutations records how many JS mutations were
// collapsed into a single render. See FrameMetrics.
func (cr *CanvasRenderer) RecordCoalescedMutations(n int) {
	cr.frameMetrics.IncCoalescedMutations(n)
}

// RecordCoalescedScroll records how many scroll events were
// collapsed into a single render. See FrameMetrics.
func (cr *CanvasRenderer) RecordCoalescedScroll(n int) {
	cr.frameMetrics.IncCoalescedScroll(n)
}

// RecordCoalescedImages records how many image-loaded callbacks were
// collapsed into a single render. See FrameMetrics.
func (cr *CanvasRenderer) RecordCoalescedImages(n int) {
	cr.frameMetrics.IncCoalescedImages(n)
}

// SetNavigationCallback sets the navigation callback for link clicks
func (cr *CanvasRenderer) SetNavigationCallback(callback NavigationCallback, baseURL string) {
	cr.mu.Lock()
	cr.onNavigate = callback
	cr.baseURL = baseURL
	cr.mu.Unlock()
}

// SetSubmitting updates the submitting status of the canvas renderer
func (cr *CanvasRenderer) SetSubmitting(submitting bool) {
	cr.mu.Lock()
	cr.submitting = submitting
	if !submitting {
		cr.submittingForms = make(map[int64]bool)
	}
	cr.mu.Unlock()
}

// SetImageLoader sets the image loader for the canvas renderer
func (cr *CanvasRenderer) SetImageLoader(loader imageloader.Loader) {
	cr.mu.Lock()
	cr.imageLoader = loader
	cr.mu.Unlock()
	if cr.window != nil {
		loader.SetOnLoadCallback(cr.onImageLoaded)
	}
}

// SetInspectCallback sets the inspect callback for element inspection
func (cr *CanvasRenderer) SetInspectCallback(callback func(node *RenderNode, layout *LayoutBox), renderer *Renderer) {
	cr.onInspect = callback
	cr.renderer = renderer
}

// SetContextMenuCallback wires a callback that the canvas invokes when the
// user right-clicks (secondary tap) on the rendered page. The callback
// receives the hit-tested node, layout box, and absolute position of the
// click so a popup menu can be displayed near the cursor. Passing nil
// disables the context menu callback (the default).
func (cr *CanvasRenderer) SetContextMenuCallback(callback func(node *RenderNode, layout *LayoutBox, abs fyne.Position)) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.onContextMenu = callback
}

// SetMouseInputCallback wires the canvas mouse events to a poster (PR9).
// When a poster is set, the canvas widgets post immutable MouseInput
// values to the owner instead of dispatching inspect/context-menu/
// navigation callbacks directly; the owner routes them through the engine
// event loop and owns hit-testing and dispatch. Passing nil restores the
// legacy direct dispatch.
func (cr *CanvasRenderer) SetMouseInputCallback(poster func(MouseInput)) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.mousePoster = poster
}

// isInViewport checks if a box intersects with the current viewport
func (cr *CanvasRenderer) isInViewport(box Rect) bool {
	if cr.headless {
		return true
	}
	// Add buffer zone above and below viewport for smoother scrolling
	bufferZone := cr.viewportHeight * 0.5
	viewportTop := cr.viewportY - bufferZone
	viewportBottom := cr.viewportY + cr.viewportHeight + bufferZone

	boxBottom := box.Y + box.Height

	// Check if box intersects with viewport
	return boxBottom >= viewportTop && box.Y <= viewportBottom
}

// Render renders the render tree and returns a Fyne container.
// When a cached display list is available (from a prior RenderHTML or
// RenderWithViewport call), Render delegates to RenderWithViewport to
// consume display commands only, avoiding DOM tree traversal.
// Falls back to DOM traversal only when no display list exists (e.g.,
// direct test usage without a layout tree).
func (cr *CanvasRenderer) Render(root *RenderNode) fyne.CanvasObject {
	if root == nil {
		return container.NewWithoutLayout()
	}

	// Use display-list path when cached display list and layout root exist.
	cr.mu.RLock()
	hasCache := cr.cachedDisplayList != nil && cr.cachedLayoutRoot != nil && cr.cachedRenderRoot == root
	cr.mu.RUnlock()

	if hasCache {
		cr.mu.RLock()
		layoutRoot := cr.cachedLayoutRoot
		cr.mu.RUnlock()
		return cr.RenderWithViewport(root, layoutRoot)
	}

	// Fallback: DOM traversal for test/direct usage without layout tree.
	objects := make([]fyne.CanvasObject, 0)
	cr.renderNode(root, &objects)

	return container.NewVBox(objects...)
}

// DisplayListSummary returns a map of command type names to their counts
// from the currently cached display list. Returns nil if no display list
// has been built yet.
func (cr *CanvasRenderer) DisplayListSummary() map[string]int {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	if cr.cachedDisplayList == nil {
		return nil
	}

	summary := make(map[string]int)
	for _, cmd := range cr.cachedDisplayList.Commands {
		summary[cmd.Type.String()]++
	}
	return summary
}

// DisplayListCommands returns a copy of the current display list commands
// for inspection. Returns nil if no display list has been built yet.
func (cr *CanvasRenderer) DisplayListCommands() []PaintCommand {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	if cr.cachedDisplayList == nil {
		return nil
	}

	cmds := make([]PaintCommand, len(cr.cachedDisplayList.Commands))
	for i, cmd := range cr.cachedDisplayList.Commands {
		cmds[i] = *cmd
		cmds[i].Node = nil
	}
	return cmds
}

// renderNode renders a single node and its children
func (cr *CanvasRenderer) renderNode(node *RenderNode, objects *[]fyne.CanvasObject) {
	if node == nil {
		return
	}

	cr.Logger.Debug("renderNode processing", "tag", node.TagName, "type", node.Type)

	// Apply display: none
	if node.ComputedStyle != nil && node.ComputedStyle.Display == "none" {
		return
	}

	if node.Type == NodeTypeText {
		cr.renderTextNode(node, objects)
	} else if node.Type == NodeTypeElement {
		cr.renderElementNode(node, objects)
	}
}

// renderTextNode renders a text node
func (cr *CanvasRenderer) renderTextNode(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := strings.TrimSpace(node.Text)
	if text == "" {
		return
	}

	// Create text widget
	textWidget := widget.NewLabel(text)
	textWidget.Wrapping = fyne.TextWrapWord

	// Get text style from parent if available
	if node.Parent != nil {
		textWidget.TextStyle = cr.fontMetrics.GetTextStyle(node.Parent.TagName)
	}

	*objects = append(*objects, textWidget)
}

// renderElementNode renders an element node
func (cr *CanvasRenderer) renderElementNode(node *RenderNode, objects *[]fyne.CanvasObject) {
	switch node.TagName {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		cr.renderHeading(node, objects)
	case "p":
		cr.renderParagraph(node, objects)
	case "div":
		cr.renderDiv(node, objects)
	case "a":
		cr.renderLink(node, objects)
	case "ul", "ol":
		cr.renderList(node, objects)
	case "li":
		cr.renderListItem(node, objects)
	case "img":
		cr.renderImage(node, objects)
	case "input":
		cr.renderInput(node, objects)
	case "button":
		cr.renderButton(node, objects)
	case "textarea":
		cr.renderTextarea(node, objects)
	case "table":
		cr.renderTable(node, objects)
	case "tbody", "thead", "tfoot", "tr", "td", "th":
		// These elements are handled by the renderTable function.
		// If they were rendered here, it would cause the cell contents to be
		// duplicated and rendered as separate text blocks.
		// By having these cases do nothing, we ensure that only the renderTable
		// function handles the rendering of the table and its contents.
	case "br":
		// Add a spacer for line break
		*objects = append(*objects, widget.NewLabel(""))
	case "code":
		cr.renderCode(node, objects)
	case "pre":
		cr.renderPre(node, objects)
	case "blockquote":
		cr.renderBlockquote(node, objects)
	case "span", "strong", "em", "b", "i":
		// Inline elements - render children
		for _, child := range node.Children {
			cr.renderNode(child, objects)
		}
	default:
		// Generic element - just render children
		for _, child := range node.Children {
			cr.renderNode(child, objects)
		}
	}
}

// renderHeading renders heading elements (h1-h6)
func (cr *CanvasRenderer) renderHeading(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	if text == "" {
		return
	}

	// Apply CSS styles if present
	styledObj := cr.applyStylesToLabel(node, text)

	// If it's a standard label (no CSS), apply heading styles
	if label, ok := styledObj.(*widget.Label); ok {
		label.TextStyle = fyne.TextStyle{Bold: true}
	}

	*objects = append(*objects, styledObj)
}

// renderParagraph renders paragraph elements
func (cr *CanvasRenderer) renderParagraph(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	if text == "" {
		return
	}

	// Apply CSS styles if present
	styledObj := cr.applyStylesToLabel(node, text)
	*objects = append(*objects, styledObj)
}

// renderDiv renders div elements
func (cr *CanvasRenderer) renderDiv(node *RenderNode, objects *[]fyne.CanvasObject) {
	// Render children
	for _, child := range node.Children {
		cr.renderNode(child, objects)
	}
}

// renderLink renders anchor (link) elements
func (cr *CanvasRenderer) renderLink(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	href, hasHref := node.GetAttribute("href")

	// Check if the anchor contains non-text children (e.g. <img>)
	hasNonTextChildren := false
	for _, child := range node.Children {
		if child.Type == NodeTypeElement {
			hasNonTextChildren = true
			break
		}
	}

	// If there is no text and no non-text children, nothing to render.
	if text == "" && !hasNonTextChildren {
		return
	}

	// If the anchor contains images (or other element children) but no text,
	// render the children directly so images are not silently dropped.
	if text == "" && hasNonTextChildren {
		for _, child := range node.Children {
			cr.renderNode(child, objects)
		}
		return
	}

	// Render any element children (e.g. images inside the link) first.
	for _, child := range node.Children {
		if child.Type == NodeTypeElement {
			cr.renderNode(child, objects)
		}
	}

	if hasHref && href != "" {
		// Resolve URL (absolute or relative)
		resolvedURL := cr.resolveURL(href)

		// Link targets are not implemented yet.

		// Parse URL to create a proper Fyne URL object
		parsedURL, err := url.Parse(resolvedURL)
		if err != nil {
			// If URL parsing fails, display as text
			label := widget.NewLabel(text)
			label.Wrapping = fyne.TextWrapWord
			*objects = append(*objects, label)
			return
		}

		// Create a clickable hyperlink widget
		link := widget.NewHyperlink(text, parsedURL)
		link.Wrapping = fyne.TextWrapOff

		// Override the default tap handler to use our navigation callback
		if cr.onNavigate != nil {
			// Create a custom tappable widget
			tappableLink := newTappableHyperlink(text, resolvedURL, cr.onNavigate, cr, cr.dlBuildGen)
			*objects = append(*objects, applyLinkColor(node, tappableLink))
		} else {
			// Fallback to default hyperlink behavior
			*objects = append(*objects, applyLinkColor(node, link))
		}
	} else {
		// No href, just display as text
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapWord
		*objects = append(*objects, label)
	}
}

// resolveURL resolves a relative or absolute URL against the base URL
func (cr *CanvasRenderer) resolveURL(href string) string {
	// If href is already absolute, return as-is
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}

	// If no base URL, return href as-is
	if cr.baseURL == "" {
		return href
	}

	// Parse base URL
	baseURL, err := url.Parse(cr.baseURL)
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

// TappableHyperlink is a custom hyperlink widget that can trigger navigation callbacks.
// It extends widget.Hyperlink, inheriting keyboard navigation support (Tab focus, Enter activation).
type TappableHyperlink struct {
	widget.Hyperlink
	url        string
	onNavigate NavigationCallback
	cr         *CanvasRenderer
	gen        uint64
}

// newTappableHyperlink creates a new tappable hyperlink
func newTappableHyperlink(text, urlStr string, onNavigate NavigationCallback, cr *CanvasRenderer, gen uint64) *TappableHyperlink {
	parsedURL := urlParse(urlStr)
	link := &TappableHyperlink{
		url:        urlStr,
		onNavigate: onNavigate,
		cr:         cr,
		gen:        gen,
	}
	link.ExtendBaseWidget(link)
	link.Text = text
	link.URL = parsedURL
	link.Wrapping = fyne.TextWrapOff
	return link
}

// Tapped handles tap events on the hyperlink. With a mouse-input poster
// wired (PR9) it posts an immutable LinkTap event into the engine loop
// instead of dispatching navigation directly; the drain resolves the URL
// on the owner. Without a poster it keeps the legacy direct dispatch.
func (t *TappableHyperlink) Tapped(_ *fyne.PointEvent) {
	if t.cr != nil {
		t.cr.mu.Lock()
		if t.cr.dlBuildGen != t.gen || t.cr.submitting {
			t.cr.mu.Unlock()
			return
		}
		t.cr.mu.Unlock()
		if t.cr.postMouseInput(MouseInput{Kind: MouseInputLinkTap, URL: t.url}) {
			return
		}
	}
	if t.onNavigate != nil {
		t.onNavigate(t.url)
	}
}

// urlParse is a helper that returns nil on parse error
func urlParse(urlStr string) *url.URL {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}
	return parsed
}

// InspectableContainer is a container that handles mouse events for element inspection
type InspectableContainer struct {
	widget.BaseWidget
	container      *fyne.Container
	canvasRenderer *CanvasRenderer

	lastHitNodeID int64
	lastHitTest   time.Time
}

// newInspectableContainer creates a new inspectable container
func newInspectableContainer(content fyne.CanvasObject, cr *CanvasRenderer) *InspectableContainer {
	ic := &InspectableContainer{
		canvasRenderer: cr,
	}
	ic.container = container.NewWithoutLayout(content)
	ic.ExtendBaseWidget(ic)
	return ic
}

// CreateRenderer creates a renderer for the inspectable container
func (ic *InspectableContainer) CreateRenderer() fyne.WidgetRenderer {
	return &inspectableContainerRenderer{
		container: ic.container,
		objects:   []fyne.CanvasObject{ic.container},
	}
}

// MouseIn handles mouse enter events
func (ic *InspectableContainer) MouseIn(*fyne.PointEvent) {
	// Mouse enter - could show hover state
}

// MouseOut handles mouse leave events
func (ic *InspectableContainer) MouseOut() {
	// Mouse leave - could clear hover state
}

// MouseMoved handles mouse movement for hover inspection. With a mouse-input
// poster wired (PR9) it posts an immutable Move event into the engine loop
// (the loop's latest-wins slot collapses the ~60fps burst); the drain owns
// hit-test throttling and the inspect dispatch. Without a poster it keeps
// the legacy direct dispatch below.
func (ic *InspectableContainer) MouseMoved(event *fyne.PointEvent) {
	cr := ic.canvasRenderer
	if cr == nil {
		return
	}
	if cr.postMouseInput(MouseInput{
		Kind: MouseInputMove,
		X:    event.Position.X,
		Y:    event.Position.Y,
	}) {
		return
	}
	if cr.onInspect == nil || cr.renderer == nil {
		return
	}

	// Throttle hit tests to avoid freezing the UI on complex pages.
	// MouseMoved fires at display refresh rate (~60fps), and each hit test
	// does a full recursive traversal of the layout tree.
	if time.Since(ic.lastHitTest) < 80*time.Millisecond {
		return
	}
	ic.lastHitTest = time.Now()

	// Get the scroll offset if we're in a scroll container
	scrollY := cr.viewportY

	// Convert mouse position to content coordinates
	contentX := event.Position.X
	contentY := event.Position.Y + scrollY

	// Perform hit test
	node, layout := cr.renderer.HitTest(contentX, contentY)
	if node != nil && layout != nil {
		// Only trigger callback if hovering a different element
		if node.ID != ic.lastHitNodeID {
			ic.lastHitNodeID = node.ID
			if cr.onInspect != nil {
				cr.onInspect(node, layout)
			}
		}
	} else {
		ic.lastHitNodeID = 0
	}
}

// MouseDown handles mouse click events for element selection. With a
// mouse-input poster wired (PR9) it posts an immutable Click event (button
// 1) into the engine loop's ordered FIFO; the drain hit-tests and selects.
// Without a poster it keeps the legacy direct dispatch below.
func (ic *InspectableContainer) MouseDown(event *fyne.PointEvent) {
	cr := ic.canvasRenderer
	if cr == nil {
		return
	}
	if cr.postMouseInput(MouseInput{
		Kind:   MouseInputClick,
		Button: 1,
		X:      event.Position.X,
		Y:      event.Position.Y,
	}) {
		return
	}
	if cr.onInspect == nil || cr.renderer == nil {
		return
	}

	// Get the scroll offset
	scrollY := cr.viewportY

	// Convert mouse position to content coordinates
	contentX := event.Position.X
	contentY := event.Position.Y + scrollY

	// Perform hit test
	node, layout := cr.renderer.HitTest(contentX, contentY)
	if node != nil && layout != nil {
		// Call inspect callback on click (to select element)
		if cr.onInspect != nil {
			cr.onInspect(node, layout)
		}
	}
}

// TappedSecondary handles right-click (secondary tap) events on the rendered
// page. It performs a hit-test at the cursor and, when a context menu
// callback is registered, forwards the result along with the absolute cursor
// position so the UI layer can show a dev-tools context menu.
//
// If no node is hit (e.g. the user clicks outside any layout box) and a
// callback is registered we still notify it with nil node/layout so the
// UI can decide whether to show a page-level menu.
func (ic *InspectableContainer) TappedSecondary(event *fyne.PointEvent) {
	cr := ic.canvasRenderer
	if cr == nil || cr.renderer == nil {
		return
	}
	if cr.postMouseInput(MouseInput{
		Kind:   MouseInputClick,
		Button: 2,
		X:      event.Position.X,
		Y:      event.Position.Y,
		AbsX:   event.AbsolutePosition.X,
		AbsY:   event.AbsolutePosition.Y,
	}) {
		return
	}

	cr.mu.RLock()
	cb := cr.onContextMenu
	cr.mu.RUnlock()
	if cb == nil {
		return
	}

	// Convert mouse position to content coordinates (scroll-aware).
	scrollY := cr.viewportY
	contentX := event.Position.X
	contentY := event.Position.Y + scrollY

	node, layout := cr.renderer.HitTest(contentX, contentY)
	cb(node, layout, event.AbsolutePosition)
}

// inspectableContainerRenderer is the renderer for the inspectable container
type inspectableContainerRenderer struct {
	container *fyne.Container
	objects   []fyne.CanvasObject
}

func (r *inspectableContainerRenderer) Layout(size fyne.Size) {
	r.container.Resize(size)
}

func (r *inspectableContainerRenderer) MinSize() fyne.Size {
	return r.container.MinSize()
}

func (r *inspectableContainerRenderer) Refresh() {
	r.container.Refresh()
}

func (r *inspectableContainerRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *inspectableContainerRenderer) Destroy() {}

// renderList renders ul/ol elements
func (cr *CanvasRenderer) renderList(node *RenderNode, objects *[]fyne.CanvasObject) {
	// Render list items
	for _, child := range node.Children {
		if child.TagName == "li" {
			cr.renderListItem(child, objects)
		}
	}
}

// renderListItem renders li elements
func (cr *CanvasRenderer) renderListItem(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	if text == "" {
		return
	}

	// Add bullet point
	label := widget.NewLabel("• " + text)
	label.Wrapping = fyne.TextWrapWord

	*objects = append(*objects, label)
}

// renderCode renders code elements with monospace styling
func (cr *CanvasRenderer) renderCode(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	if text == "" {
		return
	}

	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	label.TextStyle = fyne.TextStyle{Monospace: true}

	*objects = append(*objects, label)
}

// renderPre renders pre elements with monospace styling and preserved whitespace
func (cr *CanvasRenderer) renderPre(node *RenderNode, objects *[]fyne.CanvasObject) {
	// For pre elements, we want to preserve whitespace and newlines
	// Extract text without trimming
	text := cr.extractTextPreserveWhitespace(node)
	if text == "" {
		return
	}

	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapOff // Pre elements typically don't wrap
	label.TextStyle = fyne.TextStyle{Monospace: true}

	*objects = append(*objects, label)
}

// renderBlockquote renders blockquote elements
func (cr *CanvasRenderer) renderBlockquote(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	if text == "" {
		return
	}

	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord

	*objects = append(*objects, label)
}

// renderImage renders img elements
func (cr *CanvasRenderer) renderImage(node *RenderNode, objects *[]fyne.CanvasObject) {
	alt, hasAlt := node.GetAttribute("alt")
	src, hasSrc := node.GetAttribute("src")

	// Check visibility and opacity from styles
	isHidden := false
	opacity := float64(1.0)
	if node.ComputedStyle != nil {
		if node.ComputedStyle.Visibility == "hidden" {
			isHidden = true
		}
		opacity = float64(node.ComputedStyle.Opacity)
	} else {
		cr.Logger.Debug("ComputedStyle is nil", "tag", node.TagName)
	}

	if node.ComputedStyle != nil {
		cr.Logger.Debug("renderImage", "tag", node.TagName, "visibility", node.ComputedStyle.Visibility, "isHidden", isHidden, "opacity", opacity)
	} else {
		cr.Logger.Debug("renderImage", "tag", node.TagName, "visibility", "unknown", "isHidden", isHidden, "opacity", opacity)
	}

	// If hidden, render transparent placeholder
	if isHidden {
		rect := canvas.NewRectangle(color.Transparent)
		w, h := cr.imageAttrSize(node)
		rect.SetMinSize(fyne.NewSize(w, h))
		*objects = append(*objects, rect)
		return
	}

	if !hasSrc || src == "" {
		// No source - show alt text or placeholder
		displayText := "[Image"
		if hasAlt {
			displayText += ": " + alt
		}
		displayText += "]"
		label := widget.NewLabel(displayText)
		label.Wrapping = fyne.TextWrapWord
		*objects = append(*objects, label)
		return
	}

	// Resolve relative URLs
	resolvedSrc := cr.resolveURL(src)

	// Try to load the image if loader is available
	if cr.imageLoader != nil {
		imageData, err := cr.imageLoader.Load(resolvedSrc)

		cr.Logger.Debug("renderImage load", "tag", node.TagName, "state", imageData, "err", err)
		if imageData != nil {
			cr.Logger.Debug("renderImage state", "tag", node.TagName, "state", imageData.State)
		}

		if err != nil {
			// Image failed to load - show error with alt text
			displayText := "[Image Load Error"
			if hasAlt {
				displayText += ": " + alt
			}
			displayText += "]"
			label := widget.NewLabel(displayText)
			label.Wrapping = fyne.TextWrapWord

			// Create a placeholder rectangle
			rect := canvas.NewRectangle(color.RGBA{R: 200, G: 200, B: 200, A: 255})
			w, h := cr.imageAttrSize(node)
			rect.SetMinSize(fyne.NewSize(w, h))

			*objects = append(*objects, container.NewVBox(rect, label))
			return
		}

		if imageData != nil {
			switch imageData.State {
			case imageloader.StateLoaded:
				// Image loaded successfully - render it
				if cr.headless {
					w, h := cr.imageAttrSize(node)
					rect := canvas.NewRectangle(color.RGBA{R: 76, G: 175, B: 80, A: 255}) // Material Green
					rect.SetMinSize(fyne.NewSize(w, h))
					rect.Resize(fyne.NewSize(w, h))
					*objects = append(*objects, rect)
					return
				}

				img := canvas.NewImageFromImage(imageData.Image)
				img.FillMode = canvas.ImageFillOriginal
				img.SetMinSize(fyne.NewSize(float32(imageData.Width), float32(imageData.Height)))

				// Apply opacity
				// Use Translucency (0 = opaque, 1 = transparent)
				img.Translucency = 1.0 - opacity

				// Add alt text below the image if available
				if hasAlt && alt != "" {
					altLabel := widget.NewLabel(alt)
					altLabel.Wrapping = fyne.TextWrapWord
					*objects = append(*objects, container.NewVBox(img, altLabel))
				} else {
					*objects = append(*objects, img)
				}
				return

			case imageloader.StateError:
				// Image failed to load - show error with alt text
				displayText := "[Image Load Failed"
				if hasAlt {
					displayText += ": " + alt
				}
				displayText += "]"
				label := widget.NewLabel(displayText)
				label.Wrapping = fyne.TextWrapWord

				// Show a gray rectangle as placeholder
				rect := canvas.NewRectangle(color.RGBA{R: 200, G: 200, B: 200, A: 255})
				w, h := cr.imageAttrSize(node)
				rect.SetMinSize(fyne.NewSize(w, h))

				*objects = append(*objects, container.NewVBox(rect, label))
				return

			case imageloader.StateLoading:
				// Image is loading - show loading placeholder
				displayText := "[Loading Image"
				if hasAlt {
					displayText += ": " + alt
				}
				displayText += "]"
				label := widget.NewLabel(displayText)
				label.Wrapping = fyne.TextWrapWord

				// Show a gray rectangle as loading indicator
				rect := canvas.NewRectangle(color.RGBA{R: 200, G: 200, B: 200, A: 255})
				w, h := cr.imageAttrSize(node)
				rect.SetMinSize(fyne.NewSize(w, h))

				*objects = append(*objects, container.NewVBox(rect, label))
				return
			}
		}
	}

	// Fallback: Show placeholder if loader is not available or something went wrong
	displayText := "[Image: " + src
	if hasAlt {
		displayText += " - " + alt
	}
	displayText += "]"

	label := widget.NewLabel(displayText)
	label.Wrapping = fyne.TextWrapWord

	rect := canvas.NewRectangle(color.RGBA{R: 200, G: 200, B: 200, A: 255})
	w, h := cr.imageAttrSize(node)
	rect.SetMinSize(fyne.NewSize(w, h))
	*objects = append(*objects, container.NewVBox(rect, label))
}

// imageAttrSize returns width and height from an img node's HTML attributes,
// falling back to 100x100 if not present or unparseable.
func (cr *CanvasRenderer) imageAttrSize(node *RenderNode) (float32, float32) {
	w := float32(100)
	h := float32(100)
	if node == nil {
		return w, h
	}
	if wAttr, ok := node.GetAttribute("width"); ok && wAttr != "" {
		if v, err := strconv.ParseFloat(wAttr, 32); err == nil && v > 0 {
			w = float32(v)
		}
	}
	if hAttr, ok := node.GetAttribute("height"); ok && hAttr != "" {
		if v, err := strconv.ParseFloat(hAttr, 32); err == nil && v > 0 {
			h = float32(v)
		}
	}
	return w, h
}

// extractText extracts all text content from a node and its children
func (cr *CanvasRenderer) extractText(node *RenderNode) string {
	var text strings.Builder
	cr.extractTextRecursive(node, &text)
	return strings.TrimSpace(text.String())
}

// extractTextRecursive recursively extracts text from a node tree
func (cr *CanvasRenderer) extractTextRecursive(node *RenderNode, builder *strings.Builder) {
	if node == nil {
		return
	}

	if node.Type == NodeTypeText {
		text := strings.TrimSpace(node.Text)
		if text != "" {
			if builder.Len() > 0 {
				builder.WriteString(" ")
			}
			builder.WriteString(text)
		}
	}

	for _, child := range node.Children {
		cr.extractTextRecursive(child, builder)
	}
}

// extractTextPreserveWhitespace extracts text content while preserving whitespace and newlines
// This is used for <pre> elements where whitespace formatting is significant
func (cr *CanvasRenderer) extractTextPreserveWhitespace(node *RenderNode) string {
	var text strings.Builder
	cr.extractTextPreserveWhitespaceRecursive(node, &text)
	return text.String()
}

// extractTextPreserveWhitespaceRecursive recursively extracts text without trimming whitespace
func (cr *CanvasRenderer) extractTextPreserveWhitespaceRecursive(node *RenderNode, builder *strings.Builder) {
	if node == nil {
		return
	}

	if node.Type == NodeTypeText {
		// Don't trim whitespace for pre elements
		builder.WriteString(node.Text)
	}

	for _, child := range node.Children {
		cr.extractTextPreserveWhitespaceRecursive(child, builder)
	}
}

// getFontSize returns font size for an element type (delegates to fontMetrics)
func (cr *CanvasRenderer) getFontSize(tagName string) float32 {
	return cr.fontMetrics.GetFontSize(tagName)
}

// getTextStyle returns text style for an element type (delegates to fontMetrics)
func (cr *CanvasRenderer) getTextStyle(tagName string) fyne.TextStyle {
	return cr.fontMetrics.GetTextStyle(tagName)
}

// RenderWithViewport renders the render tree with viewport culling, object
// caching, and spatial Y-band indexing for high-performance scroll/redraw.
// Objects are reused across frames so Fyne doesn't allocate new canvas objects
// on every scroll tick — matching Chrome/WebKit's retain-and-recycle approach.
//
// Threading contract: this method MUST be called on the Fyne main goroutine.
// It mutates Fyne canvas objects (canvas.Text, canvas.Rectangle, widget.*,
// container.Objects) and triggers container Refresh; doing so off-thread
// trips Fyne's async.EnsureMain guard and the app appears not-responding
// because the queued function never runs while the main goroutine is
// blocked waiting on it. The UI layer (ui.Tab.RenderHTML/RenderParsedContent)
// marshals callers onto the main thread via ui.RunOnMainThread before
// invoking this method. Direct callers (tests, the headless renderer) are
// either single-threaded or explicitly headless and don't enforce the
// contract.
func (cr *CanvasRenderer) RenderWithViewport(root *RenderNode, layoutRoot *LayoutBox) fyne.CanvasObject {
	if root == nil || layoutRoot == nil {
		return container.NewWithoutLayout()
	}

	cr.mu.Lock()
	defer cr.mu.Unlock()

	// Time the render path. The deferred call records the duration
	// into FrameMetrics so the HUD can show where time is going.
	renderStart := time.Now()
	defer func() {
		cr.frameMetrics.ObserveFrame(time.Since(renderStart))
	}()

	// Each viewport paint is one presented frame for the FPS meter.
	cr.fps.RecordFrame()

	// Build or reuse display list
	var displayList *DisplayList
	dlChanged := false

	if cr.cachedDisplayList != nil && cr.cachedRenderRoot == root && cr.cachedLayoutRoot == layoutRoot {
		displayList = cr.cachedDisplayList
	} else {
		dlb := NewDisplayListBuilder()
		displayList = dlb.Build(layoutRoot, root)
		SortByZIndex(displayList)
		cr.cachedDisplayList = displayList
		cr.cachedRenderRoot = root
		cr.cachedLayoutRoot = layoutRoot
		dlChanged = true
	}

	// Invalidate object cache on display list rebuild
	if dlChanged {
		cr.dlBuildGen++
		cr.objectCache = make(map[int]fyne.CanvasObject)
		cr.submittingForms = make(map[int64]bool)
	}

	// Object stack for clipped hierarchy
	objectStack := [][]fyne.CanvasObject{make([]fyne.CanvasObject, 0)}
	type ClipInfo struct {
		Box      Rect
		Overflow string
	}
	clipStack := []ClipInfo{{Box: Rect{X: 0, Y: 0, Width: cr.canvasWidth, Height: cr.canvasHeight}, Overflow: "visible"}}
	getCurrentList := func() *[]fyne.CanvasObject {
		return &objectStack[len(objectStack)-1]
	}

	// Determine viewport limits for culling
	viewportTop := cr.viewportY - cr.viewportHeight*0.5
	viewportBottom := cr.viewportY + cr.viewportHeight*1.5 // extra buffer

	for cmdIdx := 0; cmdIdx < len(displayList.Commands); cmdIdx++ {
		cmd := displayList.Commands[cmdIdx]

		// Viewport culling check (do not cull PushClip/PopClip)
		if cmd.Type != PushClip && cmd.Type != PopClip {
			cmdBottom := cmd.Box.Y + cmd.Box.Height
			if cmdBottom < viewportTop || cmd.Box.Y > viewportBottom {
				continue
			}
		}

		// Handle Clip Commands
		if cmd.Type == PushClip {
			objectStack = append(objectStack, make([]fyne.CanvasObject, 0))
			clipStack = append(clipStack, ClipInfo{Box: cmd.Box, Overflow: cmd.ClipOverflow})
			continue
		} else if cmd.Type == PopClip {
			if len(objectStack) <= 1 {
				continue
			}
			poppedObjects := objectStack[len(objectStack)-1]
			objectStack = objectStack[:len(objectStack)-1]
			clipInfo := clipStack[len(clipStack)-1]
			clipStack = clipStack[:len(clipStack)-1]
			if len(poppedObjects) == 0 {
				continue
			}

			contentObj := container.NewWithoutLayout(poppedObjects...)
			maxX, maxY := float32(0), float32(0)
			for _, obj := range poppedObjects {
				pos := obj.Position()
				size := obj.Size()
				if right := pos.X + size.Width; right > maxX {
					maxX = right
				}
				if bottom := pos.Y + size.Height; bottom > maxY {
					maxY = bottom
				}
			}
			contentObj.Resize(fyne.NewSize(maxX, maxY))

			var clipped fyne.CanvasObject
			if clipInfo.Overflow == "hidden" {
				clipped = container.NewWithoutLayout(poppedObjects...)
				clipped.Resize(fyne.NewSize(clipInfo.Box.Width, clipInfo.Box.Height))
				clipped.Move(fyne.NewPos(clipInfo.Box.X, clipInfo.Box.Y))
			} else {
				scroll := container.NewScroll(contentObj)
				scroll.Resize(fyne.NewSize(clipInfo.Box.Width, clipInfo.Box.Height))
				scroll.Move(fyne.NewPos(clipInfo.Box.X, clipInfo.Box.Y))
				clipped = scroll
			}
			*getCurrentList() = append(*getCurrentList(), clipped)
			continue
		}

		// Leaf commands were already culled by the loop above (the outer
		// bounds check is identical to isInViewport), so every command
		// reaching here is visible; no second culling pass is needed.

		// Object cache: reuse Fyne objects across frames. The cache is keyed
		// by command index and invalidated when the display list is rebuilt.
		// This eliminates allocation/GC pressure during scrolling.
		obj, ok := cr.objectCache[cmdIdx]
		if !ok {
			obj = cr.createCanvasObject(cmd)
			if obj == nil {
				continue
			}
			cr.objectCache[cmdIdx] = obj
		}

		// Set position relative to the current clip container
		currentClip := clipStack[len(clipStack)-1]
		relX := cmd.Box.X - currentClip.Box.X
		relY := cmd.Box.Y - currentClip.Box.Y
		if obj.Position().X != relX || obj.Position().Y != relY {
			obj.Move(fyne.NewPos(relX, relY))
		}

		*getCurrentList() = append(*getCurrentList(), obj)
	}

	rootObjects := objectStack[0]

	// Add dirty-region overlay rectangles when enabled. Each visible command
	// (excluding PushClip/PopClip) gets a semi-transparent overlay colored by
	// its command type, showing which document areas are repainted.
	if cr.dirtyOverlayEnabled && cr.cachedDisplayList != nil {
		for _, cmd := range cr.cachedDisplayList.Commands {
			if cmd.Type == PushClip || cmd.Type == PopClip {
				continue
			}
			if !cr.isInViewport(cmd.Box) {
				continue
			}
			overlayColor := CommandTypeToOverlayColor(cmd.Type)
			overlay := canvas.NewRectangle(overlayColor)
			overlay.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
			overlay.Move(fyne.NewPos(cmd.Box.X, cmd.Box.Y))
			rootObjects = append(rootObjects, overlay)
		}
	}

	// Add highlight overlay if a node is selected in the inspector
	if cr.highlightNode != nil {
		highlightBox := findLayoutBoxForNode(layoutRoot, cr.highlightNode.ID)
		if highlightBox != nil {
			overlayColor := color.RGBA{R: 14, G: 116, B: 235, A: 64}
			strokeColor := color.RGBA{R: 14, G: 116, B: 235, A: 200}

			overlay := canvas.NewRectangle(overlayColor)
			overlay.StrokeColor = strokeColor
			overlay.StrokeWidth = 1.5

			overlay.Resize(fyne.NewSize(highlightBox.Box.Width, highlightBox.Box.Height))
			overlay.Move(fyne.NewPos(highlightBox.Box.X, highlightBox.Box.Y))
			rootObjects = append(rootObjects, overlay)
		}
	}

	// Add live FPS HUD when enabled. The readout is drawn over the top-left
	// of the viewport and refreshed on every frame so it reflects the current
	// measurement.
	if cr.fpsOverlayEnabled {
		textObj := cr.buildFPSOverlay()
		if textObj != nil {
			rootObjects = append(rootObjects, textObj...)
		}
	}

	// Reuse background rectangle across frames
	var viewportBg *canvas.Rectangle
	if cached, ok := cr.objectCache[-1]; ok {
		if bg, ok2 := cached.(*canvas.Rectangle); ok2 {
			viewportBg = bg
		}
	}
	if viewportBg == nil && flag.Lookup("test.v") == nil {
		viewportBg = canvas.NewRectangle(color.White)
		cr.objectCache[-1] = viewportBg
	}
	if viewportBg != nil {
		contentHeight := cr.canvasHeight
		if layoutRoot != nil && layoutRoot.Box.Height > contentHeight {
			contentHeight = layoutRoot.Box.Height
		}
		viewportBg.Resize(fyne.NewSize(cr.canvasWidth, contentHeight))
		viewportBg.SetMinSize(fyne.NewSize(cr.canvasWidth, contentHeight))
		viewportBg.Move(fyne.NewPos(0, 0))
		rootObjects = append([]fyne.CanvasObject{viewportBg}, rootObjects...)
	}

	// Reuse stable root container or create one
	if cr.contentRoot != nil {
		cr.contentRoot.Objects = rootObjects
		// Direct refresh: this function is contractually on the Fyne
		// main goroutine. The previous fyne.Do() re-queued the
		// refresh onto the very thread already executing us, which
		// both deadlocked the caller's doAndWait and stacked up
		// refreshes behind any blocking work we had just done.
		// Headless mode skips Refresh entirely because there is no
		// Fyne event loop to drive; tests rely on this.
		if !cr.headless {
			cr.contentRoot.Refresh()
		}
	} else {
		cr.contentRoot = container.NewWithoutLayout(rootObjects...)
	}

	if cr.onInspect != nil && cr.renderer != nil {
		if cr.inspectable != nil {
			if len(cr.inspectable.container.Objects) == 0 || cr.inspectable.container.Objects[0] != cr.contentRoot {
				cr.inspectable.container.RemoveAll()
				cr.inspectable.container.Add(cr.contentRoot)
			}
		} else {
			cr.inspectable = newInspectableContainer(cr.contentRoot, cr)
		}
		return cr.inspectable
	}

	return cr.contentRoot
}

// createCanvasObject creates a Fyne object from a paint command
func (cr *CanvasRenderer) createCanvasObject(cmd *PaintCommand) fyne.CanvasObject {
	switch cmd.Type {
	case PaintText:
		if strings.TrimSpace(cmd.Text) == "" {
			return nil
		}

		// List marker prefix for list items
		prefix := ""
		if cmd.Node != nil && cmd.Node.Parent != nil {
			parent := cmd.Node.Parent
			if parent.TagName == "li" && parent.Parent != nil {
				gp := parent.Parent
				if gp.TagName == "ul" {
					prefix = "• "
				} else if gp.TagName == "ol" {
					// Determine index of this li among siblings
					index := 1
					for _, c := range gp.Children {
						if c.TagName == "li" {
							if c == parent {
								break
							}
							index++
						}
					}
					prefix = fmt.Sprintf("%d. ", index)
				}
			}
		}
		textContent := prefix + cmd.Text

		// Determine text color: prefer explicit Color in command, fall back to ComputedStyle, then black
		textColor := color.Color(color.Black)
		if cmd.Color != nil {
			textColor = cmd.Color
		} else if cmd.Node != nil && cmd.Node.ComputedStyle != nil && cmd.Node.ComputedStyle.Color != nil {
			textColor = cmd.Node.ComputedStyle.Color
		}

		textObj := canvas.NewText(textContent, textColor)

		// Apply font size
		if cmd.FontSize > 0 {
			textObj.TextSize = cmd.FontSize
		} else if cmd.Node != nil && cmd.Node.ComputedStyle != nil && cmd.Node.ComputedStyle.FontSize > 0 {
			textObj.TextSize = cmd.Node.ComputedStyle.FontSize
		} else {
			textObj.TextSize = cr.defaultSize
		}

		// Apply text style
		bold := cmd.Bold
		italic := cmd.Italic
		if cmd.Node != nil && cmd.Node.ComputedStyle != nil {
			if cmd.Node.ComputedStyle.FontWeight == "bold" || cmd.Node.ComputedStyle.FontWeight == "700" || cmd.Node.ComputedStyle.FontWeight == "800" || cmd.Node.ComputedStyle.FontWeight == "900" {
				bold = true
			}
		}
		if bold && italic {
			textObj.TextStyle = fyne.TextStyle{Bold: true, Italic: true}
		} else if bold {
			textObj.TextStyle = fyne.TextStyle{Bold: true}
		} else if italic {
			textObj.TextStyle = fyne.TextStyle{Italic: true}
		}

		// Add underline/strikethrough if needed
		if cmd.Underline || cmd.Strikethrough {
			return cr.addDecorations(textObj, cmd)
		}
		return textObj

	case PaintRect:
		rect := canvas.NewRectangle(cmd.FillColor)
		rect.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
		return rect

	case PaintImage:
		// Try to load and render the actual image if loader is available
		if cr.imageLoader != nil && cmd.Node.ImageData != nil {
			imageData := cmd.Node.ImageData

			if imageData != nil {
				switch imageData.State {
				case imageloader.StateLoaded:
					// Image loaded successfully - render it
					if cr.headless {
						// In headless mode, Fyne's software renderer may draw canvas.Image as blank.
						// Render a colored rectangle to easily verify successful loading.
						rect := canvas.NewRectangle(color.RGBA{R: 76, G: 175, B: 80, A: 255}) // Material Green
						rect.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
						rect.SetMinSize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
						return rect
					}

					img := canvas.NewImageFromImage(imageData.Image)
					img.FillMode = canvas.ImageFillStretch
					img.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
					img.SetMinSize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))

					// Apply opacity
					if cmd.Node.ComputedStyle != nil {
						img.Translucency = 1.0 - float64(cmd.Node.ComputedStyle.Opacity)
					}

					return img

				case imageloader.StateError:
					// Image failed to load - show error with alt text
					displayText := "[Image Load Failed"
					if cmd.ImageAlt != "" {
						displayText += ": " + cmd.ImageAlt
					}
					displayText += "]"
					label := widget.NewLabel(displayText)
					label.Wrapping = fyne.TextWrapWord
					label.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
					return label

				case imageloader.StateLoading:
					// Image is loading - show loading placeholder
					displayText := "[Loading Image"
					if cmd.ImageAlt != "" {
						displayText += ": " + cmd.ImageAlt
					}
					displayText += "]"
					label := widget.NewLabel(displayText)
					label.Wrapping = fyne.TextWrapWord

					rect := canvas.NewRectangle(color.RGBA{R: 200, G: 200, B: 200, A: 255})
					rect.SetMinSize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))

					vbox := container.NewVBox(rect, label)
					vbox.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
					return vbox
				}
			}
		}

		// Fallback: Render image placeholder
		displayText := "[Image"
		if cmd.ImageSrc != "" {
			displayText += ": " + cmd.ImageSrc
		}
		if cmd.ImageAlt != "" {
			displayText += " - " + cmd.ImageAlt
		}
		displayText += "]"

		label := widget.NewLabel(displayText)
		label.Wrapping = fyne.TextWrapWord

		rect := canvas.NewRectangle(color.RGBA{R: 200, G: 200, B: 200, A: 255})
		rect.SetMinSize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))

		vbox := container.NewVBox(rect, label)
		vbox.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
		return vbox

	case PaintLink:
		// Render clickable link
		if cmd.LinkText == "" {
			return nil
		}

		// Resolve URL (absolute or relative)
		resolvedURL := cr.resolveURL(cmd.LinkURL)

		// Create a clickable hyperlink widget
		if cr.onNavigate != nil {
			// Create a custom tappable widget
			tappableLink := newTappableHyperlink(cmd.LinkText, resolvedURL, cr.onNavigate, cr, cr.dlBuildGen)
			tappableLink.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
			return applyLinkColor(cmd.Node, tappableLink)
		} else {
			// Fallback to default hyperlink behavior
			parsedURL, err := url.Parse(resolvedURL)
			if err == nil {
				link := widget.NewHyperlink(cmd.LinkText, parsedURL)
				link.Wrapping = fyne.TextWrapOff
				link.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
				return applyLinkColor(cmd.Node, link)
			} else {
				// If URL parsing fails, display as text
				label := widget.NewLabel(cmd.LinkText)
				label.Wrapping = fyne.TextWrapWord
				label.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
				return label
			}
		}

	case PaintBorder:
		// Render borders as lines or rectangles
		// Borders meet at corners without overlapping
		borderContainer := container.NewWithoutLayout()

		// Top border (full width)
		if cmd.BorderTopWidth > 0 && cmd.BorderTopStyle != "" && cmd.BorderTopStyle != "none" {
			addHorizontalBorderSegments(borderContainer, cmd.BorderTopStyle, cmd.BorderTopColor,
				0, 0, cmd.Box.Width, cmd.BorderTopWidth)
		}

		// Right border (height minus top and bottom border widths to avoid overlap)
		if cmd.BorderRightWidth > 0 && cmd.BorderRightStyle != "" && cmd.BorderRightStyle != "none" {
			rightHeight := cmd.Box.Height - cmd.BorderTopWidth - cmd.BorderBottomWidth
			addVerticalBorderSegments(borderContainer, cmd.BorderRightStyle, cmd.BorderRightColor,
				cmd.Box.Width-cmd.BorderRightWidth, cmd.BorderTopWidth, cmd.BorderRightWidth, rightHeight)
		}

		// Bottom border (full width)
		if cmd.BorderBottomWidth > 0 && cmd.BorderBottomStyle != "" && cmd.BorderBottomStyle != "none" {
			addHorizontalBorderSegments(borderContainer, cmd.BorderBottomStyle, cmd.BorderBottomColor,
				0, cmd.Box.Height-cmd.BorderBottomWidth, cmd.Box.Width, cmd.BorderBottomWidth)
		}

		// Left border (height minus top and bottom border widths to avoid overlap)
		if cmd.BorderLeftWidth > 0 && cmd.BorderLeftStyle != "" && cmd.BorderLeftStyle != "none" {
			leftHeight := cmd.Box.Height - cmd.BorderTopWidth - cmd.BorderBottomWidth
			addVerticalBorderSegments(borderContainer, cmd.BorderLeftStyle, cmd.BorderLeftColor,
				0, cmd.BorderTopWidth, cmd.BorderLeftWidth, leftHeight)
		}

		if len(borderContainer.Objects) > 0 {
			borderContainer.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
			return borderContainer
		}
		return nil

	case PaintButton:
		// Render button widget
		if cmd.ButtonText == "" {
			return nil
		}

		gen := cr.dlBuildGen
		isSubmit := true
		if cmd.Node != nil {
			if btnType, ok := cmd.Node.GetAttribute("type"); ok && btnType == "button" {
				isSubmit = false
			}
		}

		button := widget.NewButton(cmd.ButtonText, func() {
			cr.mu.Lock()
			if cr.dlBuildGen != gen || cr.submitting {
				cr.mu.Unlock()
				return
			}
			cr.mu.Unlock()

			if isSubmit {
				formNode := findFormAncestor(cmd.Node)
				if formNode != nil {
					cr.mu.Lock()
					if cr.submittingForms[formNode.ID] {
						cr.mu.Unlock()
						return
					}
					cr.submittingForms[formNode.ID] = true
					cr.mu.Unlock()

					data := cr.collectFormData(formNode)
					method, _ := formNode.GetAttribute("method")
					method = strings.ToUpper(method)
					if method == "" {
						method = "GET"
					}

					if cr.onNavigate != nil {
						action, _ := formNode.GetAttribute("action")
						resolved := cr.resolveURL(action)
						if method == "POST" {
							cr.onNavigate(resolved)
						} else {
							parsed, err := url.Parse(resolved)
							if err == nil {
								query := parsed.Query()
								for k, v := range data {
									query.Set(k, v)
								}
								parsed.RawQuery = query.Encode()
								cr.onNavigate(parsed.String())
							} else {
								cr.onNavigate(resolved)
							}
						}
					}
				}
			}
		})
		button.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
		return button

	case PaintInput:
		if cmd.InputType == "submit" || cmd.InputType == "button" || cmd.InputType == "reset" {
			btnText := cmd.InputValue
			if btnText == "" {
				if cmd.InputType == "submit" {
					btnText = "Submit"
				} else if cmd.InputType == "reset" {
					btnText = "Reset"
				} else {
					btnText = "Button"
				}
			}
			gen := cr.dlBuildGen
			button := widget.NewButton(btnText, func() {
				if cmd.InputType == "submit" {
					cr.mu.Lock()
					if cr.dlBuildGen != gen || cr.submitting {
						cr.mu.Unlock()
						return
					}
					cr.mu.Unlock()

					formNode := findFormAncestor(cmd.Node)
					if formNode != nil {
						cr.mu.Lock()
						if cr.submittingForms[formNode.ID] {
							cr.mu.Unlock()
							return
						}
						cr.submittingForms[formNode.ID] = true
						cr.mu.Unlock()

						data := cr.collectFormData(formNode)
						method, _ := formNode.GetAttribute("method")
						method = strings.ToUpper(method)
						if method == "" {
							method = "GET"
						}

						if cr.onNavigate != nil {
							action, _ := formNode.GetAttribute("action")
							resolved := cr.resolveURL(action)
							if method == "POST" {
								cr.onNavigate(resolved)
							} else {
								parsed, err := url.Parse(resolved)
								if err == nil {
									query := parsed.Query()
									for k, v := range data {
										query.Set(k, v)
									}
									parsed.RawQuery = query.Encode()
									cr.onNavigate(parsed.String())
								} else {
									cr.onNavigate(resolved)
								}
							}
						}
					}
				}
			})
			button.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
			return button
		}

		if cmd.InputType == "checkbox" {
			check := widget.NewCheck("", func(b bool) {})
			if cmd.Node != nil {
				if _, checked := cmd.Node.GetAttribute("checked"); checked {
					check.Checked = true
				}
				if _, disabled := cmd.Node.GetAttribute("disabled"); disabled {
					check.Disable()
				}
			}
			check.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
			return check
		}

		if cmd.InputType == "radio" {
			check := widget.NewCheck("", func(b bool) {})
			if cmd.Node != nil {
				if _, checked := cmd.Node.GetAttribute("checked"); checked {
					check.Checked = true
				}
				if _, disabled := cmd.Node.GetAttribute("disabled"); disabled {
					check.Disable()
				}
			}
			check.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
			return check
		}

		entry := widget.NewEntry()
		if cmd.Placeholder != "" {
			entry.SetPlaceHolder(cmd.Placeholder)
		}
		if cmd.InputValue != "" {
			entry.SetText(cmd.InputValue)
		}
		if cmd.InputType == "password" {
			entry.Password = true
		}
		if cmd.Node != nil {
			if _, disabled := cmd.Node.GetAttribute("disabled"); disabled {
				entry.Disable()
			}
		}
		entry.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
		return entry

	case PaintTextarea:
		entry := widget.NewMultiLineEntry()
		if cmd.Placeholder != "" {
			entry.SetPlaceHolder(cmd.Placeholder)
		}
		if cmd.InputValue != "" {
			entry.SetText(cmd.InputValue)
		}
		if cmd.Node != nil {
			if _, disabled := cmd.Node.GetAttribute("disabled"); disabled {
				entry.Disable()
			}
		}
		entry.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
		return entry
	}

	return nil
}

func (cr *CanvasRenderer) renderCommand(cmd *PaintCommand, objects *[]fyne.CanvasObject) {
	obj := cr.createCanvasObject(cmd)
	if obj != nil {
		*objects = append(*objects, obj)
	}
}

// SetDirtyOverlayEnabled enables or disables the dirty-region overlay visualization.
// When enabled, semi-transparent colored rectangles are rendered over each paint
// command to show which areas are being repainted and what command types they are.
// Callers should call Refresh() after toggling to force a re-render.
func (cr *CanvasRenderer) SetDirtyOverlayEnabled(enabled bool) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if cr.dirtyOverlayEnabled != enabled {
		cr.dirtyOverlayEnabled = enabled
		cr.cachedDisplayList = nil
		cr.objectCache = make(map[int]fyne.CanvasObject)
		cr.dlBuildGen++
	}
}

// SetHighlightNode sets the node to highlight in the viewport.
func (cr *CanvasRenderer) SetHighlightNode(node *RenderNode) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if cr.highlightNode != node {
		cr.highlightNode = node
		cr.cachedDisplayList = nil
		cr.objectCache = make(map[int]fyne.CanvasObject)
		cr.dlBuildGen++
	}
}

// DirtyOverlayEnabled returns whether the dirty-region overlay is enabled.
func (cr *CanvasRenderer) DirtyOverlayEnabled() bool {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.dirtyOverlayEnabled
}

// buildFPSOverlay constructs the on-screen FPS readout objects for the
// current snapshot. It returns a background rectangle plus the text label,
// both positioned in the top-left corner of the viewport. Returns nil when
// there is no measured frame yet.
//
// The renderer is required to run on the Fyne main thread (the UI layer
// marshals RenderParsed/RenderHTML onto the main goroutine before
// invoking the canvas), so we can mutate the cached overlay objects in
// place across frames instead of allocating fresh ones every scroll
// tick. This matters because RenderWithViewport is invoked on every
// OnScrolled tick — historically the per-frame allocations here
// contributed noticeable GC pressure at 60 Hz.
//
// The displayed text is intentionally *actionable*: it shows the
// observed FPS, the worst-case input-to-present latency, the number
// of long frames, and how many scroll/mutation events were coalesced
// in the current window. A "bad FPS" symptom becomes a "lots of long
// frames" symptom, which is what the operator needs to identify the
// real bottleneck.
func (cr *CanvasRenderer) buildFPSOverlay() []fyne.CanvasObject {
	stats := cr.fps.Snapshot()
	if stats.Frames == 0 {
		return nil
	}
	m := cr.frameMetrics.Snapshot()

	const (
		padX  float32 = 8
		padY  float32 = 8
		fontS float32 = 12
	)

	// Three lines, top-left. The first line is the headline FPS; the
	// second is the worst-case latency we observed in the recent
	// window; the third is the long-frame count and the coalesced
	// event totals.
	lines := []string{
		fmt.Sprintf("FPS %.1f", stats.CurrentFPS),
		fmt.Sprintf("i\u2192p %s  q %s",
			formatLatency(m.MaxInputToPresent), formatLatency(m.MaxUIQueueWait)),
		fmt.Sprintf("long %d  coalesced s%d m%d i%d  drop %d",
			m.LongFrames, m.CoalescedScrollEvents, m.CoalescedMutations, m.CoalescedImages, m.StaleFramesDropped),
	}
	text := strings.Join(lines, "\n")

	// Approximate width from the longest line so the background hugs
	// the text closely enough without a text-measure dependency here.
	estW := fontS*0.62*float32(longestLine(lines)) + 2*padX
	estH := fontS*float32(len(lines)) + 2*padY

	// Lazily allocate the overlay objects the first time the HUD is
	// shown. After that we mutate in place — the text object's Text
	// field is reset only when the displayed value actually changed,
	// avoiding a Refresh per scroll tick.
	if cr.fpsOverlayBg == nil {
		cr.fpsOverlayBg = canvas.NewRectangle(color.RGBA{R: 0, G: 0, B: 0, A: 170})
	}
	if cr.fpsOverlayText == nil {
		cr.fpsOverlayText = canvas.NewText(text, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		cr.fpsOverlayText.TextSize = fontS
	}

	bg := cr.fpsOverlayBg
	bg.Resize(fyne.NewSize(estW, estH))
	bg.Move(fyne.NewPos(0, 0))

	textObj := cr.fpsOverlayText
	textObj.Resize(fyne.NewSize(estW, estH-fontS))
	textObj.Move(fyne.NewPos(padX, padY))
	if cr.fpsOverlayTextCache != text {
		textObj.Text = text
		cr.fpsOverlayTextCache = text
		textObj.Refresh()
	}

	return []fyne.CanvasObject{bg, textObj}
}

// longestLine returns the length (in bytes) of the longest string in s.
// Used to size the FPS overlay background without a text-measure call.
func longestLine(s []string) int {
	max := 0
	for _, l := range s {
		if len(l) > max {
			max = len(l)
		}
	}
	return max
}

// formatLatency renders a duration as a short human-readable string
// suitable for the on-screen HUD (e.g. "12ms", "1.2s"). Zero values
// render as "-" to keep the HUD compact.
func formatLatency(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d >= time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

// FPSStats returns the current frame-rate statistics measured by the
// renderer.
func (cr *CanvasRenderer) FPSStats() FPSStats {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.fps.Snapshot()
}

// SetFPSOverlayEnabled enables or disables the on-screen FPS HUD overlay.
// When enabled, each presented frame updates a small readout at the top-left
// of the viewport.
func (cr *CanvasRenderer) SetFPSOverlayEnabled(enabled bool) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if cr.fpsOverlayEnabled == enabled {
		return
	}
	cr.fpsOverlayEnabled = enabled
	if !enabled {
		// Drop the cached overlay objects so the next time the HUD is
		// turned on we start from a clean slate. The container will
		// detach them when it next rebuilds rootObjects.
		cr.fpsOverlayBg = nil
		cr.fpsOverlayText = nil
		cr.fpsOverlayTextCache = ""
	}
	// Invalidate the retained display list so the next render reflects the
	// overlay toggle.
	cr.cachedDisplayList = nil
}

// FPSOverlayEnabled returns whether the on-screen FPS HUD overlay is enabled.
func (cr *CanvasRenderer) FPSOverlayEnabled() bool {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.fpsOverlayEnabled
}

// CommandTypeToOverlayColor returns a semi-transparent color for a paint command
// type, used by the dirty-region overlay visualization.
func CommandTypeToOverlayColor(t PaintCommandType) color.Color {
	switch t {
	case PaintText:
		return color.RGBA{R: 0, G: 0, B: 255, A: 40}
	case PaintRect:
		return color.RGBA{R: 0, G: 128, B: 0, A: 40}
	case PaintImage:
		return color.RGBA{R: 255, G: 255, B: 0, A: 40}
	case PaintLink:
		return color.RGBA{R: 0, G: 255, B: 255, A: 40}
	case PaintBorder:
		return color.RGBA{R: 255, G: 165, B: 0, A: 40}
	case PaintButton:
		return color.RGBA{R: 128, G: 0, B: 128, A: 40}
	case PaintInput:
		return color.RGBA{R: 255, G: 192, B: 203, A: 40}
	case PaintTextarea:
		return color.RGBA{R: 255, G: 0, B: 255, A: 40}
	default:
		return color.RGBA{R: 128, G: 128, B: 128, A: 40}
	}
}

// ClearCache clears the cached display list and object cache to force re-rendering
func (cr *CanvasRenderer) ClearCache() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.cachedDisplayList = nil
	cr.cachedLayoutRoot = nil
	cr.cachedRenderRoot = nil
	cr.objectCache = make(map[int]fyne.CanvasObject)
	cr.dlBuildGen++
}

// InvalidateObjectCache clears only the object cache (keeping the display list)
// so objects are re-created from commands on the next render. Used when backing
// data changes (e.g. images finish loading) without the display list changing.
func (cr *CanvasRenderer) InvalidateObjectCache() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.objectCache = make(map[int]fyne.CanvasObject)
	cr.dlBuildGen++
}

func (cr *CanvasRenderer) renderInput(node *RenderNode, objects *[]fyne.CanvasObject) {
	entry := widget.NewEntry()
	if placeholder, ok := node.GetAttribute("placeholder"); ok {
		entry.SetPlaceHolder(placeholder)
	}
	*objects = append(*objects, entry)
}

func (cr *CanvasRenderer) renderTable(node *RenderNode, objects *[]fyne.CanvasObject) {
	data := [][]string{}
	var maxCols int

	// Helper function to extract rows from a node (handles tbody, thead, tfoot)
	var extractRows func(*RenderNode)
	extractRows = func(n *RenderNode) {
		for _, child := range n.Children {
			if child.TagName == "tr" {
				row := []string{}
				for _, td := range child.Children {
					if td.TagName == "td" || td.TagName == "th" {
						row = append(row, cr.extractText(td))
					}
				}
				if len(row) > maxCols {
					maxCols = len(row)
				}
				data = append(data, row)
			} else if child.TagName == "tbody" || child.TagName == "thead" || child.TagName == "tfoot" {
				// Recursively process tbody, thead, tfoot
				extractRows(child)
			}
		}
	}

	extractRows(node)

	if len(data) == 0 || maxCols == 0 {
		return
	}

	table := widget.NewTable(
		func() (int, int) {
			return len(data), maxCols
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(i widget.TableCellID, o fyne.CanvasObject) {
			if i.Row < len(data) && i.Col < len(data[i.Row]) {
				o.(*widget.Label).SetText(data[i.Row][i.Col])
			}
		},
	)

	for i := 0; i < maxCols; i++ {
		table.SetColumnWidth(i, 100)
	}

	*objects = append(*objects, table)
}

func (cr *CanvasRenderer) renderButton(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	button := widget.NewButton(text, func() {})
	*objects = append(*objects, button)
}

func (cr *CanvasRenderer) renderTextarea(node *RenderNode, objects *[]fyne.CanvasObject) {
	entry := widget.NewMultiLineEntry()
	if placeholder, ok := node.GetAttribute("placeholder"); ok {
		entry.SetPlaceHolder(placeholder)
	}
	*objects = append(*objects, entry)
}

// hasCustomStyles checks if a node has CSS styles that require custom rendering
func (cr *CanvasRenderer) hasCustomStyles(node *RenderNode) bool {
	return node != nil && node.ComputedStyle != nil && (node.ComputedStyle.Color != nil ||
		node.ComputedStyle.FontSize > 0 ||
		node.ComputedStyle.FontWeight == "bold")
}

// applyStylesToLabel applies CSS styles from ComputedStyle to a label widget.
// Since Fyne's standard Label widget doesn't support custom colors or font sizes,
// this function creates a styled canvas.Text object when custom styles are present.
// Note: canvas.Text objects don't support text wrapping, which is a known limitation.
func (cr *CanvasRenderer) applyStylesToLabel(node *RenderNode, text string) fyne.CanvasObject {
	if !cr.hasCustomStyles(node) {
		// No custom styles, use selectable text widget
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapWord

		// Apply tag-based styles (bold, italic, etc.)
		if node.Parent != nil {
			label.TextStyle = cr.fontMetrics.GetTextStyle(node.Parent.TagName)
		}

		return label
	}

	// Create a styled canvas.Text object for custom colors/sizes
	// Note: canvas.Text doesn't support selection, but we need it for custom colors
	textObj := canvas.NewText(text, color.Black)
	textObj.TextSize = cr.defaultSize

	// Apply computed styles
	style := node.ComputedStyle

	if style.Color != nil {
		textObj.Color = style.Color
	}

	if style.FontSize > 0 {
		textObj.TextSize = style.FontSize
	}

	if style.FontWeight == "bold" {
		textObj.TextStyle = fyne.TextStyle{Bold: true}
	}

	return textObj
}

// addDecorations adds underline or strikethrough lines to a text object
func (cr *CanvasRenderer) addDecorations(obj fyne.CanvasObject, cmd *PaintCommand) fyne.CanvasObject {
	if !cmd.Underline && !cmd.Strikethrough {
		return obj
	}

	var textColor color.Color = color.Black
	if cmd.Node != nil && cmd.Node.ComputedStyle != nil && cmd.Node.ComputedStyle.Color != nil {
		textColor = cmd.Node.ComputedStyle.Color
	}

	decoContainer := container.NewWithoutLayout(obj)

	// Width and height of the decoration lines
	lineThickness := float32(1.0)
	if cmd.FontSize > 24 {
		lineThickness = 2.0
	}

	if cmd.Underline {
		underline := canvas.NewRectangle(textColor)
		// Position at bottom of text
		// Note: obj.Size() might not be accurate yet if it's a label, but cmd.Box has the dimensions
		underline.Resize(fyne.NewSize(cmd.Box.Width, lineThickness))
		underline.Move(fyne.NewPos(0, cmd.Box.Height-lineThickness))
		decoContainer.Add(underline)
	}

	if cmd.Strikethrough {
		strikethrough := canvas.NewRectangle(textColor)
		// Position at middle of text
		strikethrough.Resize(fyne.NewSize(cmd.Box.Width, lineThickness))
		strikethrough.Move(fyne.NewPos(0, cmd.Box.Height/2))
		decoContainer.Add(strikethrough)
	}

	decoContainer.Resize(fyne.NewSize(cmd.Box.Width, cmd.Box.Height))
	return decoContainer
}

// minDashLength is the minimum length (in pixels) for a dashed border segment.
const minDashLength = float32(6)

// addHorizontalBorderSegments adds horizontal border segments (dashed/dotted/solid) to a container.
// x, y define the top-left corner; totalWidth and height define the area.
func addHorizontalBorderSegments(c *fyne.Container, style string, col color.Color, x, y, totalWidth, height float32) {
	if col == nil {
		col = color.Black
	}
	if style == "solid" || style == "double" || style == "" {
		rect := canvas.NewRectangle(col)
		rect.Resize(fyne.NewSize(totalWidth, height))
		rect.Move(fyne.NewPos(x, y))
		c.Add(rect)
		return
	}

	// Calculate dash and gap lengths
	var dashLen, gapLen float32
	switch style {
	case "dotted":
		dashLen = height // square dots
		gapLen = height
	default: // dashed
		dashLen = height * 3
		if dashLen < minDashLength {
			dashLen = minDashLength
		}
		gapLen = dashLen
	}

	pos := x
	for pos < x+totalWidth {
		w := dashLen
		if pos+w > x+totalWidth {
			w = x + totalWidth - pos
		}
		if w <= 0 {
			break
		}
		seg := canvas.NewRectangle(col)
		seg.Resize(fyne.NewSize(w, height))
		seg.Move(fyne.NewPos(pos, y))
		c.Add(seg)
		pos += dashLen + gapLen
	}
}

// addVerticalBorderSegments adds vertical border segments (dashed/dotted/solid) to a container.
// x, y define the top-left corner; width and totalHeight define the area.
func addVerticalBorderSegments(c *fyne.Container, style string, col color.Color, x, y, width, totalHeight float32) {
	if col == nil {
		col = color.Black
	}
	if style == "solid" || style == "double" || style == "" {
		rect := canvas.NewRectangle(col)
		rect.Resize(fyne.NewSize(width, totalHeight))
		rect.Move(fyne.NewPos(x, y))
		c.Add(rect)
		return
	}

	// Calculate dash and gap lengths
	var dashLen, gapLen float32
	switch style {
	case "dotted":
		dashLen = width // square dots
		gapLen = width
	default: // dashed
		dashLen = width * 3
		if dashLen < minDashLength {
			dashLen = minDashLength
		}
		gapLen = dashLen
	}

	pos := y
	for pos < y+totalHeight {
		h := dashLen
		if pos+h > y+totalHeight {
			h = y + totalHeight - pos
		}
		if h <= 0 {
			break
		}
		seg := canvas.NewRectangle(col)
		seg.Resize(fyne.NewSize(width, h))
		seg.Move(fyne.NewPos(x, pos))
		c.Add(seg)
		pos += dashLen + gapLen
	}
}

// findWidgetByNodeID finds the instantiated Fyne widget for a given Node ID in the object cache
func (cr *CanvasRenderer) findWidgetByNodeID(nodeID int64) fyne.CanvasObject {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	if cr.cachedDisplayList == nil {
		return nil
	}
	for cmdIdx, obj := range cr.objectCache {
		if cmdIdx < 0 || cmdIdx >= len(cr.cachedDisplayList.Commands) {
			continue
		}
		cmd := cr.cachedDisplayList.Commands[cmdIdx]
		if cmd.NodeID == nodeID {
			return obj
		}
	}
	return nil
}

// collectFormData gathers form data from form elements
func (cr *CanvasRenderer) collectFormData(formNode *RenderNode) map[string]string {
	data := make(map[string]string)
	var collect func(*RenderNode)
	collect = func(n *RenderNode) {
		if n == nil {
			return
		}
		if n.Type == NodeTypeElement {
			// Skip disabled inputs
			if _, disabled := n.GetAttribute("disabled"); disabled {
				return
			}
			name, hasName := n.GetAttribute("name")
			if hasName && name != "" {
				// Find Fyne widget for this node
				if widgetObj := cr.findWidgetByNodeID(n.ID); widgetObj != nil {
					if entry, ok := widgetObj.(*widget.Entry); ok {
						data[name] = entry.Text
					} else if check, ok := widgetObj.(*widget.Check); ok {
						if check.Checked {
							if val, ok := n.GetAttribute("value"); ok {
								data[name] = val
							} else {
								data[name] = "on"
							}
						}
					}
				} else {
					// Fallback to value/checked attribute on RenderNode
					if n.TagName == "input" {
						inputType, _ := n.GetAttribute("type")
						if inputType == "checkbox" || inputType == "radio" {
							if _, checked := n.GetAttribute("checked"); checked {
								if val, ok := n.GetAttribute("value"); ok {
									data[name] = val
								} else {
									data[name] = "on"
								}
							}
						} else {
							val, _ := n.GetAttribute("value")
							data[name] = val
						}
					} else if n.TagName == "textarea" {
						data[name] = cr.extractText(n)
					}
				}
			}
		}
		for _, child := range n.Children {
			collect(child)
		}
	}
	collect(formNode)
	return data
}

// findFormAncestor walks up the parent tree to find the containing form element
func findFormAncestor(node *RenderNode) *RenderNode {
	for n := node; n != nil; n = n.Parent {
		if n.TagName == "form" {
			return n
		}
	}
	return nil
}
