package engine

import (
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

func (s *Session) IsHoverGeometryNeutral() bool {
	if s == nil || s.hovered == nil || s.styleResolver == nil {
		return false
	}

	currentStyle, ok := s.Styles[s.hovered.ID]
	if !ok || currentStyle == nil {
		return false
	}

	return true
}

func (s *Session) RestyleForHover() error {
	if s == nil || s.hovered == nil {
		return nil
	}

	s.opMu.Lock()
	defer s.opMu.Unlock()

	if s.styleResolver == nil {
		s.styleResolver = style.NewResolver(s.sheets)
	}

	s.workCounts.styleResolves.Add(1)

	if incremental, ok := s.styleResolver.ResolveInteractiveStates(
		s.Doc, s.styleViewport(), s.customFonts, s.Styles, s.PseudoStyles,
	); ok {
		s.workCounts.styleNodeVisits.Add(uint64(incremental.Visited))

		layoutChanged := interactiveLayoutChanged(incremental, s.Styles, s.PseudoStyles)

		if !layoutChanged {
			if s.Arena.RefreshComputedStylesForNodes(s.Styles, incremental.Nodes) {
				s.styleDirty.Store(false)
				s.layoutDirty.Store(false)
				return nil
			}
		}

		for _, id := range incremental.Nodes {
			if prevStyle, exists := incremental.PreviousStyles[id]; exists {
				if newStyle, hasNew := s.Styles[id]; hasNew {
					s.updateArenaStyle(id, newStyle)
				}
				_ = prevStyle
			}
		}
	}

	s.opMu.Unlock()
	err := s.RestyleAndReflow(s.viewportW)
	s.opMu.Lock()

	return err
}

func (s *Session) updateArenaStyle(nodeID dom.NodeID, newStyle *style.ComputedStyle) {
	if s.Arena == nil || newStyle == nil {
		return
	}

	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.ID == nodeID {
			obj.Style = newStyle
			return
		}
	}
}

func (s *Session) FocusDamageRect() frame.Rect {
	if s == nil || s.Arena == nil || s.focus == nil {
		return frame.Rect{}
	}

	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node == s.focus {
			x0, y0, x1, y1 := obj.BorderRect()
			return frame.Rect{
				X0: int32(x0),
				Y0: int32(y0),
				X1: int32(x1),
				Y1: int32(y1),
			}
		}
	}

	return frame.Rect{}
}

func (s *Session) SelectionDamageRect() frame.Rect {
	if s == nil || !s.HasSelection() {
		return frame.Rect{}
	}

	bounds, ok := s.SelectionBounds()
	if !ok {
		return frame.Rect{}
	}

	return bounds
}
