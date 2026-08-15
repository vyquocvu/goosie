package renderer

import (
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
	cr := NewCanvasRenderer(400, 400)
	r := NewRenderer(400, 400)
	cr.renderer = r

	var posted []MouseInput
	cr.SetMouseInputCallback(func(m MouseInput) { posted = append(posted, m) })

	// Legacy direct dispatch must NOT fire while a poster is wired.
	directInspect := 0
	directMenu := 0
	directNav := ""
	cr.SetInspectCallback(func(_ *RenderNode, _ *LayoutBox) { directInspect++ }, r)
	cr.SetContextMenuCallback(func(_ *RenderNode, _ *LayoutBox, _ fyne.Position) { directMenu++ })
	cr.SetNavigationCallback(func(url string) { directNav = url }, "http://base/")

	ic := newInspectableContainer(nil, cr)

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
	link := newTappableHyperlink("home", "http://base/", cr.onNavigate, cr, cr.dlBuildGen)
	link.Tapped(nil)

	assert.Len(t, posted, 4, "every canvas mouse event should be posted")
	if len(posted) != 4 {
		t.Fatalf("posted = %v", posted)
	}
	assert.Equal(t, MouseInput{Kind: MouseInputMove, X: 10, Y: 20}, posted[0])
	assert.Equal(t, MouseInput{Kind: MouseInputClick, Button: 1, X: 30, Y: 40}, posted[1])
	assert.Equal(t, MouseInput{Kind: MouseInputClick, Button: 2, X: 50, Y: 60, AbsX: 150, AbsY: 160}, posted[2])
	assert.Equal(t, MouseInput{Kind: MouseInputLinkTap, URL: "http://base/"}, posted[3])

	assert.Equal(t, 0, directInspect, "poster path must not dispatch inspect directly")
	assert.Equal(t, 0, directMenu, "poster path must not dispatch the context menu directly")
	assert.Equal(t, "", directNav, "poster path must not navigate directly")
}

// TestCanvasRenderer_MouseInputFallbackDirectDispatch guards the
// no-poster path: without a wired poster the widgets keep the legacy
// direct dispatch, so renderer-only owners and tests keep working.
func TestCanvasRenderer_MouseInputFallbackDirectDispatch(t *testing.T) {
	cr := NewCanvasRenderer(200, 200)
	r := NewRenderer(200, 200)
	cr.inspectable = newInspectableContainer(nil, cr)

	// Build a hit-testable tree: a div spanning the whole area.
	div := NewRenderNode(NodeTypeElement)
	div.ID = 1
	div.TagName = "div"
	divBox := NewLayoutBox(div.ID)
	divBox.Box = Rect{X: 0, Y: 0, Width: 100, Height: 30}
	r.currentRenderTree = div
	r.currentLayoutTree = divBox

	type call struct {
		node *RenderNode
		box  *LayoutBox
		abs  fyne.Position
	}
	var got call
	cr.SetContextMenuCallback(func(n *RenderNode, l *LayoutBox, abs fyne.Position) {
		got = call{node: n, box: l, abs: abs}
	})
	cr.SetInspectCallback(func(_ *RenderNode, _ *LayoutBox) {}, r)

	// No SetMouseInputCallback call: direct dispatch must still work.
	cr.inspectable.TappedSecondary(&fyne.PointEvent{
		Position:         fyne.NewPos(5, 5),
		AbsolutePosition: fyne.NewPos(105, 55),
	})

	assert.NotNil(t, got.node, "fallback path must still dispatch the context menu")
	if got.node != nil {
		assert.Equal(t, "div", got.node.TagName)
	}
	assert.Equal(t, fyne.NewPos(105, 55), got.abs)
}
