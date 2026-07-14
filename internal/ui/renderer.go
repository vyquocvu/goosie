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
}
