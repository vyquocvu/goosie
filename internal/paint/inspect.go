package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

// InspectHighlight names the node the inspector is showing. Obj is that node's
// layout box in document CSS pixels; the overlay covers its border rect.
type InspectHighlight struct {
	Obj *layout.Object
}

// SetInspectHighlight marks the node to outline. It paints after the document
// walk, so no ancestor's overflow clips it and no later box covers it. The
// overlay is paint-only: it never writes to the DOM.
func (b *Builder) SetInspectHighlight(h *InspectHighlight) {
	b.inspect = h
}

// InspectHighlightColor is the inspector's own UA blue, chosen to read like the
// element highlight Chromium's DevTools draws. Declared, never assigned, so
// internal/archtest treats it as configuration rather than mutable state.
var InspectHighlightColor = frame.RGB(26, 115, 232)

// inspectHighlightOpacity keeps the page readable under the overlay: the fill
// tints the node without hiding its content, while the border stays solid.
const inspectHighlightOpacity = 0.25

func (b *Builder) paintInspectHighlight() {
	h := b.inspect
	if h == nil || h.Obj == nil || b.scale <= 0 {
		return
	}
	cx0, cy0, cx1, cy1 := h.Obj.BorderRect()
	if cx1 <= cx0 || cy1 <= cy0 {
		return
	}
	r := frame.RectF4(cx0, cy0, cx1, cy1).ToDevice(b.scale)
	if r.Empty() {
		return
	}
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    r,
		Color:   InspectHighlightColor,
		Opacity: inspectHighlightOpacity,
	})
	w := int32(2 * b.scale)
	if w < 1 {
		w = 1
	}
	edge := SideSpec{Width: w, Color: InspectHighlightColor}
	b.list.Append(DisplayCmd{
		Kind:    CmdBorder,
		Rect:    r,
		Border:  BorderSpec{Top: edge, Right: edge, Bottom: edge, Left: edge},
		Opacity: 1,
	})
}
