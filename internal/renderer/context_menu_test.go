package renderer

import (
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

// TestSetContextMenuCallback_RoundTrip verifies that a callback
// passed to SetContextMenuCallback is stored on the CanvasRenderer
// and a nil callback is also accepted (which represents the
// "disabled" state).
func TestSetContextMenuCallback_RoundTrip(t *testing.T) {
	cr := NewCanvasRenderer(800, 600)

	called := 0
	callback := func(_ *RenderNode, _ *LayoutBox, _ fyne.Position) {
		called++
	}

	cr.SetContextMenuCallback(callback)
	cr.mu.RLock()
	stored := cr.onContextMenu
	cr.mu.RUnlock()
	assert.NotNil(t, stored, "callback should be stored")

	// Disabling the callback with nil must also work.
	cr.SetContextMenuCallback(nil)
	cr.mu.RLock()
	stored = cr.onContextMenu
	cr.mu.RUnlock()
	assert.Nil(t, stored, "callback should be cleared after nil")
	_ = called // silence unused
}

// TestInspectableContainer_TappedSecondary_NoCallback guards against
// the no-callback-attached path: TappedSecondary must return early
// without touching the renderer (which may be nil).
func TestInspectableContainer_TappedSecondary_NoCallback(t *testing.T) {
	cr := NewCanvasRenderer(400, 400)
	// Intentionally do NOT call SetInspectCallback / SetContextMenuCallback.
	ic := newInspectableContainer(nil, cr)

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
	cr := NewCanvasRenderer(200, 200)

	r := NewRenderer(200, 200)
	cr.inspectable = newInspectableContainer(nil, cr)

	// Build a hit-testable tree: div containing a paragraph that
	// spans the whole (0,0)-(100,30) area.
	div := NewRenderNode(NodeTypeElement)
	div.ID = 1
	div.TagName = "div"

	p := NewRenderNode(NodeTypeElement)
	p.ID = 2
	p.TagName = "p"
	div.AddChild(p)

	divBox := NewLayoutBox(div.ID)
	divBox.Box = Rect{X: 0, Y: 0, Width: 100, Height: 30}

	pBox := NewLayoutBox(p.ID)
	pBox.Box = Rect{X: 0, Y: 0, Width: 100, Height: 30}
	divBox.AddChild(pBox)

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
	cr.SetInspectCallback(func(n *RenderNode, l *LayoutBox) {}, r)

	// The hit-test traverses down to the deepest matching box.
	cr.inspectable.TappedSecondary(&fyne.PointEvent{
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
