package ui

import (
	"context"
	"fyne.io/fyne/v2"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
)

type HTMLRenderer interface {
	RenderHTML(ctx context.Context, htmlContent string) (fyne.CanvasObject, error)
	UpdateViewport() fyne.CanvasObject
	SetCurrentURL(url string)
	ResolveURL(url string) string
	SetWindow(w fyne.Window)
	SetNavigationCallback(callback func(url string))
	HitTest(x, y float32) (*renderer.RenderNode, *renderer.LayoutBox)
	SetInspectCallback(callback func(node *renderer.RenderNode, layout *renderer.LayoutBox))
	GetRoot() *renderer.RenderNode
	Refresh()
	SetRefreshCallback(callback func())
	SetSubmitting(submitting bool)
	SetCSP(p *net.CSPPolicy)

	// GetDisplayListSummary returns a map of paint command type names to
	// their counts from the current display list. Returns nil when no
	// display list has been built.
	GetDisplayListSummary() map[string]int

	// SetDirtyOverlayEnabled enables or disables the dirty-region overlay
	// visualization. When enabled, semi-transparent colored rectangles are
	// rendered over each paint command to show repaint regions.
	SetDirtyOverlayEnabled(enabled bool)

	// DirtyOverlayEnabled returns whether the dirty-region overlay is enabled.
	DirtyOverlayEnabled() bool

	// GetDOMNodeCounts returns the total, element, and text node counts
	// from the current render tree.
	GetDOMNodeCounts() (total int, elements int, text int)

	// GetLayoutNodeCount returns the number of layout boxes in the
	// current layout tree.
	GetLayoutNodeCount() int
}
