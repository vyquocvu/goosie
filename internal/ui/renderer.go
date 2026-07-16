package ui

import (
	"context"
	"fyne.io/fyne/v2"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
	"golang.org/x/net/html"
)

type HTMLRenderer interface {
	RenderHTML(ctx context.Context, htmlContent string) (fyne.CanvasObject, error)
	// RenderParsed renders a pre-parsed HTML node with the supplied
	// external stylesheets. It is the M3 snapshot entry point: the
	// caller has already fetched CSS via the documentloader
	// coordinator and the renderer does not perform further network
	// I/O for stylesheets.
	RenderParsed(ctx context.Context, doc *html.Node, externalCSS []renderer.ExternalCSS) (fyne.CanvasObject, error)
	UpdateViewport() fyne.CanvasObject
	SetCurrentURL(url string)
	ResolveURL(url string) string
	SetWindow(w fyne.Window)
	SetHeadless(headless bool)
	SetNavigationCallback(callback func(url string))
	HitTest(x, y float32) (*renderer.RenderNode, *renderer.LayoutBox)
	SetInspectCallback(callback func(node *renderer.RenderNode, layout *renderer.LayoutBox))

	// SetContextMenuCallback wires a callback invoked when the user
	// right-clicks (secondary tap) on the rendered page. The callback
	// receives the hit-tested node, layout box, and absolute position of
	// the cursor so the UI layer can show a dev-tools context menu. Passing
	// nil disables the context menu.
	SetContextMenuCallback(callback func(node *renderer.RenderNode, layout *renderer.LayoutBox, abs fyne.Position))
	GetRoot() *renderer.RenderNode
	Refresh()
	SetRefreshCallback(callback func())
	SetSubmitting(submitting bool)
	SetCSP(p *net.CSPPolicy)

	// GetDisplayListSummary returns a map of paint command type names to
	// their counts from the current display list. Returns nil when no
	// display list has been built.
	GetDisplayListSummary() map[string]int

	// GetDisplayListCommands returns a copy of the current display list
	// commands for individual inspection. Returns nil when no display
	// list has been built.
	GetDisplayListCommands() []renderer.PaintCommand

	// SetDirtyOverlayEnabled enables or disables the dirty-region overlay
	// visualization. When enabled, semi-transparent colored rectangles are
	// rendered over each paint command to show repaint regions.
	SetDirtyOverlayEnabled(enabled bool)

	// DirtyOverlayEnabled returns whether the dirty-region overlay is enabled.
	DirtyOverlayEnabled() bool

	// SetSize updates the renderer's canvas dimensions and marks it for
	// re-layout. Callers should call Refresh() after SetSize to recompute
	// style/layout and trigger a re-render.
	SetSize(width, height float32)

	// GetDOMNodeCounts returns the total, element, and text node counts
	// from the current render tree.
	GetDOMNodeCounts() (total int, elements int, text int)

	// GetLayoutNodeCount returns the number of layout boxes in the
	// current layout tree.
	GetLayoutNodeCount() int

	// GetStyleSheet returns the current stylesheet.
	GetStyleSheet() *css.StyleSheet

	// GetMatchedRules returns all CSS rules matching the given node, sorted by specificity.
	GetMatchedRules(node *renderer.RenderNode) []css.Rule
	// SetHighlightNode sets the node to highlight in the viewport.
	SetHighlightNode(node *renderer.RenderNode)
	// GetLayoutBox returns the computed layout box associated with the given node.
	GetLayoutBox(node *renderer.RenderNode) *renderer.LayoutBox
}
