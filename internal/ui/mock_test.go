package ui

import (
	"context"

	"fyne.io/fyne/v2"
	"github.com/vyquocvu/goosie/internal/css"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
	"golang.org/x/net/html"
)

// MockHTMLRendererComp implements HTMLRenderer with configurable summary.
type MockHTMLRendererComp struct {
	summary             map[string]int
	dirtyOverlayEnabled bool
}

func (m *MockHTMLRendererComp) RenderHTML(ctx context.Context, s string) (fyne.CanvasObject, error) {
	return nil, nil
}
func (m *MockHTMLRendererComp) RenderParsed(ctx context.Context, doc *html.Node, externalCSS []renderer.ExternalCSS) (fyne.CanvasObject, error) {
	return nil, nil
}
func (m *MockHTMLRendererComp) UpdateViewport() fyne.CanvasObject               { return nil }
func (m *MockHTMLRendererComp) SetCurrentURL(url string)                        {}
func (m *MockHTMLRendererComp) ResolveURL(url string) string                    { return url }
func (m *MockHTMLRendererComp) SetWindow(w fyne.Window)                         {}
func (m *MockHTMLRendererComp) SetNavigationCallback(callback func(url string)) {}
func (m *MockHTMLRendererComp) HitTest(x, y float32) (*renderer.RenderNode, *renderer.LayoutBox) {
	return nil, nil
}
func (m *MockHTMLRendererComp) SetInspectCallback(callback func(node *renderer.RenderNode, layout *renderer.LayoutBox)) {
}
func (m *MockHTMLRendererComp) SetContextMenuCallback(callback func(node *renderer.RenderNode, layout *renderer.LayoutBox, abs fyne.Position)) {
}
func (m *MockHTMLRendererComp) GetRoot() *renderer.RenderNode                   { return nil }
func (m *MockHTMLRendererComp) Refresh()                                        {}
func (m *MockHTMLRendererComp) SetRefreshCallback(callback func())              {}
func (m *MockHTMLRendererComp) SetSubmitting(submitting bool)                   {}
func (m *MockHTMLRendererComp) SetCSP(p *goosienet.CSPPolicy)                   {}
func (m *MockHTMLRendererComp) GetDisplayListSummary() map[string]int           { return m.summary }
func (m *MockHTMLRendererComp) GetDisplayListCommands() []renderer.PaintCommand { return nil }
func (m *MockHTMLRendererComp) SetDirtyOverlayEnabled(enabled bool) {
	m.dirtyOverlayEnabled = enabled
}
func (m *MockHTMLRendererComp) DirtyOverlayEnabled() bool         { return m.dirtyOverlayEnabled }
func (m *MockHTMLRendererComp) GetDOMNodeCounts() (int, int, int) { return 0, 0, 0 }
func (m *MockHTMLRendererComp) GetLayoutNodeCount() int           { return 0 }
func (m *MockHTMLRendererComp) GetStyleSheet() *css.StyleSheet    { return nil }
func (m *MockHTMLRendererComp) GetMatchedRules(node *renderer.RenderNode) []css.Rule {
	return nil
}
func (m *MockHTMLRendererComp) SetHighlightNode(node *renderer.RenderNode) {}
func (m *MockHTMLRendererComp) GetLayoutBox(node *renderer.RenderNode) *renderer.LayoutBox {
	return nil
}
func (m *MockHTMLRendererComp) SetHeadless(bool) {}
func (m *MockHTMLRendererComp) SetSize(width, height float32) {}

