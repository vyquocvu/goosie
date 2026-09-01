// Package ui provides the InteractiveRasterCanvas widget.
//
// M1.2: Implements a single-surface Fyne widget that displays an *image.RGBA
// raster surface and handles all pointer/keyboard interaction. This replaces
// the tree of thousands of Fyne widgets with a single pixel buffer that Goosie
// fully controls. Fyne only acts as the window/display layer; Goosie owns 100%
// of the graphics pipeline and mouse/keyboard interaction.
package ui

import (
	"image"
	"image/draw"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// rasterHitTester is the subset of HTMLRenderer that the raster canvas
// needs for pointer interaction. Using an interface avoids a circular
// import and lets tests supply a mock.

// ---------------------------------------------------------------------------
// InteractiveRasterCanvas — single-surface raster widget
// ---------------------------------------------------------------------------

// InteractiveRasterCanvas is a Fyne custom widget that displays a single
// *image.RGBA pixel buffer and handles all pointer/keyboard interaction.
// It replaces the tree of thousands of Fyne widgets with one pixel surface
// that Goosie fully controls.
//
// Key features:
//   - Displays a single *image.RGBA raster surface
//   - Handles Tapped (left click), TappedSecondary (right click), MouseMoved
//   - Connects to renderer.HitTest for cursor changes and navigation
//   - Implements "Invisible Focus Proxy" for keyboard/IME input
type InteractiveRasterCanvas struct {
	widget.BaseWidget

	// hitTester provides HitTest for pointer interaction. This is the
	// HTMLRenderer (or a test mock) — not a concrete *renderer.Renderer.
	hitTester interface {
		HitTest(x, y float32) (*renderer.RenderNode, *renderer.LayoutBox)
	}

	// frame is the current pixel buffer to display.
	frame *image.RGBA

	// canvas is the Fyne canvas image that renders the frame.
	canvas *canvas.Image

	// focusProxy is an invisible entry widget that captures keyboard/IME
	// input without requiring Goosie to implement its own input method editor.
	focusProxy *widget.Entry

	// scrollOffset is the current scroll position in content coordinates.
	scrollOffset float32

	// lastCursor tracks the last cursor type to avoid redundant updates.
	lastCursor desktop.Cursor

	// mu protects frame and scrollOffset during concurrent updates.
	mu sync.RWMutex

	// onNavigate is called when a link is clicked.
	onNavigate func(url string)

	// onInspect is called when an element is inspected (e.g., right-click).
	onInspect func(node *renderer.RenderNode, layout *renderer.LayoutBox)

	// onContextMenu is called for right-click context menu.
	onContextMenu func(node *renderer.RenderNode, layout *renderer.LayoutBox, pos fyne.Position)
}

// NewInteractiveRasterCanvas creates a new interactive raster canvas with the
// given renderer. The renderer provides HitTest and frame data.
func NewInteractiveRasterCanvas(hitTester interface {
	HitTest(x, y float32) (*renderer.RenderNode, *renderer.LayoutBox)
}) *InteractiveRasterCanvas {
	irc := &InteractiveRasterCanvas{
		hitTester: hitTester,
		canvas:   canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1))),
		focusProxy: &widget.Entry{
			// Invisible: zero-size, no placeholder, no text
			PlaceHolder: "",
		},
		lastCursor: desktop.DefaultCursor,
	}

	// Configure the focus proxy to be invisible but functional
	// The focus proxy is a zero-size entry that captures keyboard input
	// without being visible in the UI

	// Wire up the focus proxy's key input handler
	irc.focusProxy.OnChanged = func(s string) {
		// Forward typed text to the renderer's event loop
		if hitTester != nil && len(s) > 0 {
			// TODO: Route to event loop for form input
			// For now, clear the proxy to avoid accumulation
			irc.focusProxy.SetText("")
		}
	}

	irc.ExtendBaseWidget(irc)
	return irc
}

// CreateRenderer returns the Fyne renderer for this widget.
func (irc *InteractiveRasterCanvas) CreateRenderer() fyne.WidgetRenderer {
	return &rasterCanvasRenderer{
		canvas:     irc.canvas,
		focusProxy: irc.focusProxy,
	}
}

// SetFrame updates the pixel buffer to display. This is called by the
// renderer when a new frame is ready.
func (irc *InteractiveRasterCanvas) SetFrame(frame *image.RGBA) {
	irc.mu.Lock()
	irc.frame = frame
	irc.mu.Unlock()

	// Update the canvas image
	irc.canvas.Image = frame
	irc.canvas.Refresh()
}

// SetScrollOffset updates the scroll position. HitTest uses this to convert
// widget coordinates to content coordinates.
func (irc *InteractiveRasterCanvas) SetScrollOffset(offset float32) {
	irc.mu.Lock()
	irc.scrollOffset = offset
	irc.mu.Unlock()
}

// SetNavigateCallback sets the callback for link navigation.
func (irc *InteractiveRasterCanvas) SetNavigateCallback(fn func(url string)) {
	irc.onNavigate = fn
}

// SetInspectCallback sets the callback for element inspection.
func (irc *InteractiveRasterCanvas) SetInspectCallback(fn func(node *renderer.RenderNode, layout *renderer.LayoutBox)) {
	irc.onInspect = fn
}

// SetContextMenuCallback sets the callback for right-click context menu.
func (irc *InteractiveRasterCanvas) SetContextMenuCallback(fn func(node *renderer.RenderNode, layout *renderer.LayoutBox, pos fyne.Position)) {
	irc.onContextMenu = fn
}

// ---------------------------------------------------------------------------
// Pointer interaction handlers
// ---------------------------------------------------------------------------

// Tapped handles left-click events.
func (irc *InteractiveRasterCanvas) Tapped(e *fyne.PointEvent) {
	if irc.hitTester == nil {
		return
	}

	// Convert widget coordinates to content coordinates
	contentX := e.Position.X
	contentY := e.Position.Y + irc.scrollOffset

	// Perform hit test
	node, layout := irc.hitTester.HitTest(contentX, contentY)
	if node == nil {
		return
	}

	// Check if this is a link
	if node.TagName == "a" {
		if href, ok := node.GetAttribute("href"); ok {
			if irc.onNavigate != nil {
				irc.onNavigate(href)
			}
			return
		}
	}

	// For other elements, trigger inspect if callback is set
	if irc.onInspect != nil {
		irc.onInspect(node, layout)
	}
}

// TappedSecondary handles right-click events (context menu).
func (irc *InteractiveRasterCanvas) TappedSecondary(e *fyne.PointEvent) {
	if irc.hitTester == nil {
		return
	}

	// Convert widget coordinates to content coordinates
	contentX := e.Position.X
	contentY := e.Position.Y + irc.scrollOffset

	// Perform hit test
	node, layout := irc.hitTester.HitTest(contentX, contentY)
	if node == nil {
		return
	}

	// Show context menu
	if irc.onContextMenu != nil {
		irc.onContextMenu(node, layout, e.AbsolutePosition)
	}
}

// MouseMoved handles pointer hover events.
func (irc *InteractiveRasterCanvas) MouseMoved(e *desktop.MouseEvent) {
	if irc.hitTester == nil {
		return
	}

	// Convert widget coordinates to content coordinates
	contentX := e.Position.X
	contentY := e.Position.Y + irc.scrollOffset

	// Perform hit test
	node, _ := irc.hitTester.HitTest(contentX, contentY)

	// Determine cursor type based on hit test result
	var newCursor desktop.Cursor
	if node != nil && node.TagName == "a" {
		// Link: pointer cursor
		newCursor = desktop.PointerCursor
	} else {
		// Default: arrow cursor
		newCursor = desktop.DefaultCursor
	}

	// Update cursor if changed
	if newCursor != irc.lastCursor {
		irc.lastCursor = newCursor
		// Fyne doesn't have a direct cursor API, but we can signal the window
		// TODO: Implement cursor change via window.SetCursor()
	}
}

// Dragged handles drag events (for future text selection, etc.).
func (irc *InteractiveRasterCanvas) Dragged(e *fyne.DragEvent) {
	// TODO: Implement text selection, drag-and-drop
}

// Scrolled handles scroll wheel events.
func (irc *InteractiveRasterCanvas) Scrolled(e *fyne.ScrollEvent) {
	// Update scroll offset
	irc.mu.Lock()
	irc.scrollOffset -= e.Scrolled.DY * 50 // 50px per scroll tick
	if irc.scrollOffset < 0 {
		irc.scrollOffset = 0
	}
	irc.mu.Unlock()

	// Trigger a refresh to update the display
	irc.Refresh()
}

// FocusGained is called when the widget gains focus.
func (irc *InteractiveRasterCanvas) FocusGained() {
	// Forward focus to the invisible proxy for keyboard input
	if irc.focusProxy != nil {
		irc.focusProxy.FocusGained()
	}
}

// FocusLost is called when the widget loses focus.
func (irc *InteractiveRasterCanvas) FocusLost() {
	if irc.focusProxy != nil {
		irc.focusProxy.FocusLost()
	}
}

// TypedRune handles typed character input.
func (irc *InteractiveRasterCanvas) TypedRune(r rune) {
	// Forward to focus proxy
	if irc.focusProxy != nil {
		irc.focusProxy.TypedRune(r)
	}
}

// TypedKey handles typed key input.
func (irc *InteractiveRasterCanvas) TypedKey(k *fyne.KeyEvent) {
	// Forward to focus proxy
	if irc.focusProxy != nil {
		irc.focusProxy.TypedKey(k)
	}
}

// ---------------------------------------------------------------------------
// rasterCanvasRenderer — Fyne widget renderer
// ---------------------------------------------------------------------------

// rasterCanvasRenderer is the Fyne renderer for InteractiveRasterCanvas.
type rasterCanvasRenderer struct {
	canvas     *canvas.Image
	focusProxy *widget.Entry
}

// MinSize returns the minimum size of the widget.
func (r *rasterCanvasRenderer) MinSize() fyne.Size {
	return fyne.NewSize(100, 100)
}

// Layout positions the canvas image and focus proxy.
func (r *rasterCanvasRenderer) Layout(size fyne.Size) {
	// Fill the entire widget with the canvas image
	r.canvas.Resize(size)
	r.canvas.Move(fyne.NewPos(0, 0))

	// Position the focus proxy off-screen (invisible but functional)
	r.focusProxy.Resize(fyne.NewSize(0, 0))
	r.focusProxy.Move(fyne.NewPos(-10, -10))
}

// Refresh updates the display.
func (r *rasterCanvasRenderer) Refresh() {
	r.canvas.Refresh()
}

// Objects returns the canvas objects managed by this renderer.
func (r *rasterCanvasRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.canvas, r.focusProxy}
}

// Destroy cleans up resources.
func (r *rasterCanvasRenderer) Destroy() {
	// No resources to clean up
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// blitFrame copies the source image into the destination buffer using
// draw.Draw for efficient pixel copying.
func blitFrame(dst *image.RGBA, src image.Image) {
	if dst == nil || src == nil {
		return
	}
	draw.Draw(dst, dst.Bounds(), src, image.Point{}, draw.Src)
}
