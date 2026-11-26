package engine

import (
	"fmt"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

func (s *Session) DocumentLayerBounds(painted frame.Rect, scale float32, viewport frame.Size) (frame.Rect, error) {
	if s == nil {
		return frame.Rect{}, fmt.Errorf("missing session")
	}
	if err := validateScale(float64(scale)); err != nil {
		return frame.Rect{}, err
	}
	if err := ValidateViewport(int(viewport.W), int(viewport.H), 1); err != nil {
		return frame.Rect{}, err
	}

	s.opMu.Lock()
	defer s.opMu.Unlock()
	arena := s.Arena
	if err := validateArena(arena); err != nil {
		return frame.Rect{}, err
	}
	root := arena.Get(layout.ObjectID(1))
	marginBox := frame.RectF4(
		root.X-root.MarginLeft,
		root.Y-root.MarginTop,
		root.X+root.BorderW()+root.MarginRight,
		root.Y+root.BorderH()+root.MarginBottom,
	).ToDevice(scale)
	contentX, contentY, _, _ := root.ContentRect()
	overflowW, overflowH := documentScrollableOverflow(arena, root)
	overflow := frame.RectF4(
		contentX,
		contentY,
		contentX+float32(overflowW),
		contentY+float32(overflowH),
	).ToDevice(scale)
	bounds := marginBox.Union(overflow).Union(painted).Union(frame.RectAt(frame.Point{}, viewport))
	if bounds.X0 < 0 {
		bounds.X0 = 0
	}
	if bounds.Y0 < 0 {
		bounds.Y0 = 0
	}
	if err := ValidateExtent(bounds); err != nil {
		return frame.Rect{}, err
	}
	return bounds, nil
}
