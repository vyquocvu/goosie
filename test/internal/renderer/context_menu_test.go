package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

// TestSetContextMenuCallback_RoundTrip verifies that a callback
// passed to SetContextMenuCallback is stored on the CanvasRenderer
// and a nil callback is also accepted (which represents the
// "disabled" state).
func TestSetContextMenuCallback_RoundTrip(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)

	called := 0
	callback := func(_ *renderer.RenderNode, _ *renderer.LayoutBox, _ fyne.Position) {
		called++
	}

	cr.SetContextMenuCallback(callback)
	cr.CanvasRendererMu().RLock()
	stored := cr.OnContextMenu()
	cr.CanvasRendererMu().RUnlock()
	assert.NotNil(t, stored, "callback should be stored")

	// Disabling the callback with nil must also work.
	cr.SetContextMenuCallback(nil)
	cr.CanvasRendererMu().RLock()
	stored = cr.OnContextMenu()
	cr.CanvasRendererMu().RUnlock()
	assert.Nil(t, stored, "callback should be cleared after nil")
	_ = called // silence unused
}

// TestInspectableContainer_TappedSecondary_NoCallback guards against
// the no-callback-attached path: TappedSecondary must return early
// without touching the renderer (which may be nil).
func TestInspectableContainer_TappedSecondary_NoCallback(t *testing.T) {
	cr := renderer.NewCanvasRenderer(400, 400)
	// Intentionally do NOT call SetInspectCallback / SetContextMenuCallback.
	ic := renderer.NewInspectableContainer(nil, cr)

	// No panic is the success criterion here.
	assert.NotPanics(t, func() {
		ic.TappedSecondary(&fyne.PointEvent{
			Position:         fyne.NewPos(10, 10),
			AbsolutePosition: fyne.NewPos(20, 20),
		})
	})
}

// TestInspectableContainer_TappedSecondary_InvokesCallback builds a
// very small layout tree, performs a hit test at (5,5), and confirms
// that the callback fires with the correct node. We exercise the
// callback through the TappedSecondary entry point rather than
// directly.
func TestInspectableContainer_TappedSecondary_InvokesCallback(t *testing.T) {
	cr := renderer.NewCanvasRenderer(200, 200)

	r := renderer.NewRenderer(200, 200)
	cr.SetInspectable(renderer.NewInspectableContainer(nil, cr))

	// Build a hit-testable tree: div containing a paragraph that
	// spans the whole (0,0)-(100,30) area.
	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.ID = 1
	div.TagName = "div"

	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.ID = 2
	p.TagName = "p"
	div.AddChild(p)

	divBox := renderer.NewLayoutBox(div.ID)
	divBox.Box = renderer.Rect{X: 0, Y: 0, Width: 100, Height: 30}

	pBox := renderer.NewLayoutBox(p.ID)
	pBox.Box = renderer.Rect{X: 0, Y: 0, Width: 100, Height: 30}
	divBox.AddChild(pBox)

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
	cr.SetInspectCallback(func(n *renderer.RenderNode, l *renderer.LayoutBox) {}, r)

	// The hit-test traverses down to the deepest matching box.
	cr.Inspectable().TappedSecondary(&fyne.PointEvent{
		Position:         fyne.NewPos(5, 5),
		AbsolutePosition: fyne.NewPos(105, 55),
	})

	assert.NotNil(t, got.node, "callback should have fired with a node")
	if got.node != nil {
		assert.Equal(t, "p", got.node.TagName)
	}
	assert.NotNil(t, got.box, "callback should have fired with a layout box")
	assert.Equal(t, fyne.NewPos(105, 55), got.abs, "absolute position should be passed through")
}
