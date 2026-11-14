package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

type InspectHighlight struct {
	Obj *layout.Object
}

func (b *Builder) SetInspectHighlight(h *InspectHighlight) {
	b.inspect = h
}

var InspectHighlightColor = frame.RGB(26, 115, 232)

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
