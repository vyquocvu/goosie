package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

// TestCanvasRenderer_MouseInputPosterRoutesEvents verifies PR9's routing
// contract: when a mouse-input poster is wired, the canvas widgets post
// immutable MouseInput values (position, button, absolute coords, link
// URL) instead of dispatching the inspect / context-menu / navigation
// callbacks directly.
func TestCanvasRenderer_MouseInputPosterRoutesEvents(t *testing.T) {
	cr := renderer.NewCanvasRenderer(400, 400)
	r := renderer.NewRenderer(400, 400)
	cr.SetRenderer(r)

	var posted []renderer.MouseInput
	cr.SetMouseInputCallback(func(m renderer.MouseInput) { posted = append(posted, m) })

	// Legacy direct dispatch must NOT fire while a poster is wired.
	directInspect := 0
	directMenu := 0
	directNav := ""
	cr.SetInspectCallback(func(_ *renderer.RenderNode, _ *renderer.LayoutBox) { directInspect++ }, r)
	cr.SetContextMenuCallback(func(_ *renderer.RenderNode, _ *renderer.LayoutBox, _ fyne.Position) { directMenu++ })
	cr.SetNavigationCallback(func(url string) { directNav = url }, "http://base/")

	ic := renderer.NewInspectableContainer(nil, cr)

	// Hover move posts a Move event (widget-space position only).
	ic.MouseMoved(&fyne.PointEvent{Position: fyne.NewPos(10, 20)})
	// Left click posts a Click event with button 1.
	ic.MouseDown(&fyne.PointEvent{Position: fyne.NewPos(30, 40)})
	// Right click posts a Click event with button 2 + absolute position.
	ic.TappedSecondary(&fyne.PointEvent{
		Position:         fyne.NewPos(50, 60),
		AbsolutePosition: fyne.NewPos(150, 160),
	})
	// Hyperlink tap posts a LinkTap event carrying the resolved URL.
	link := renderer.NewTappableHyperlink("home", "http://base/", cr.OnNavigate(), cr, cr.DLBuildGen())
	link.Tapped(nil)

	assert.Len(t, posted, 4, "every canvas mouse event should be posted")
	if len(posted) != 4 {
		t.Fatalf("posted = %v", posted)
	}
	assert.Equal(t, renderer.MouseInput{Kind: renderer.MouseInputMove, X: 10, Y: 20}, posted[0])
	assert.Equal(t, renderer.MouseInput{Kind: renderer.MouseInputClick, Button: 1, X: 30, Y: 40}, posted[1])
	assert.Equal(t, renderer.MouseInput{Kind: renderer.MouseInputClick, Button: 2, X: 50, Y: 60, AbsX: 150, AbsY: 160}, posted[2])
	assert.Equal(t, renderer.MouseInput{Kind: renderer.MouseInputLinkTap, URL: "http://base/"}, posted[3])

	assert.Equal(t, 0, directInspect, "poster path must not dispatch inspect directly")
	assert.Equal(t, 0, directMenu, "poster path must not dispatch the context menu directly")
	assert.Equal(t, "", directNav, "poster path must not navigate directly")
}

// TestCanvasRenderer_MouseInputFallbackDirectDispatch guards the
// no-poster path: without a wired poster the widgets keep the legacy
// direct dispatch, so renderer-only owners and tests keep working.
func TestCanvasRenderer_MouseInputFallbackDirectDispatch(t *testing.T) {
	cr := renderer.NewCanvasRenderer(200, 200)
	r := renderer.NewRenderer(200, 200)
	cr.SetInspectable(renderer.NewInspectableContainer(nil, cr))

	// Build a hit-testable tree: a div spanning the whole area.
	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.ID = 1
	div.TagName = "div"
	divBox := renderer.NewLayoutBox(div.ID)
	divBox.Box = renderer.Rect{X: 0, Y: 0, Width: 100, Height: 30}
	r.SetCurrentRenderTree(div)
	r.SetCurrentLayoutTree(divBox)

	type call struct {
		node *renderer.RenderNode
		box  *renderer.LayoutBox
		abs  fyne.Position
	}
	var got call
	cr.SetContextMenuCallback(func(n *renderer.RenderNode, l *renderer.LayoutBox, abs fyne.Position) {
		got = call{node: n, box: l, abs: abs}
	})
	cr.SetInspectCallback(func(_ *renderer.RenderNode, _ *renderer.LayoutBox) {}, r)

	// No SetMouseInputCallback call: direct dispatch must still work.
	cr.Inspectable().TappedSecondary(&fyne.PointEvent{
		Position:         fyne.NewPos(5, 5),
		AbsolutePosition: fyne.NewPos(105, 55),
	})

	assert.NotNil(t, got.node, "fallback path must still dispatch the context menu")
	if got.node != nil {
		assert.Equal(t, "div", got.node.TagName)
	}
	assert.Equal(t, fyne.NewPos(105, 55), got.abs)
}
