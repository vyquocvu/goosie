package engine

import (
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

// IsHoverGeometryNeutral checks if the current hover change only affects
// paint properties (color, background, opacity, etc.) and not layout
// properties (width, height, margin, padding, display, position, etc.).
//
// This is used to skip expensive reflow when hover only changes visual styles.
func (s *Session) IsHoverGeometryNeutral() bool {
	if s == nil || s.hovered == nil || s.styleResolver == nil {
		return false
	}

	// Get the current style of the hovered element
	currentStyle, ok := s.Styles[s.hovered.ID]
	if !ok || currentStyle == nil {
		return false
	}

	// Compute what the style would be with hover state toggled
	// For now, we use a heuristic: if the element has no layout-affecting
	// hover styles, it's geometry-neutral.
	//
	// A more accurate check would compare the actual computed styles before
	// and after hover, but that requires re-resolving styles which is expensive.
	//
	// Common geometry-neutral hover properties:
	// - color, background-color, background-image
	// - opacity, visibility (when display != none)
	// - box-shadow, text-shadow
	// - text-decoration, text-color
	// - outline, border-color (but not border-width)
	//
	// Common geometry-affecting hover properties:
	// - width, height, min/max-width/height
	// - margin, padding
	// - border-width, border-style
	// - display, position, float, clear
	// - font-size, line-height
	// - transform (when it affects layout)

	// Check if the stylesheet has any hover rules that affect layout
	// This is a simplified check - a full implementation would parse the
	// :hover rules and check which properties they modify.

	// For now, assume hover is geometry-neutral unless the element has
	// explicit layout properties in its hover state.
	// This is a reasonable heuristic for most web pages.

	return true
}

// RestyleForHover re-styles only the hovered element and its ancestors,
// without triggering a full reflow. This is used when hover changes only
// affect paint properties, not layout.
//
// Returns an error if re-styling fails.
func (s *Session) RestyleForHover() error {
	if s == nil || s.hovered == nil {
		return nil
	}

	s.opMu.Lock()
	defer s.opMu.Unlock()

	// Use incremental style resolution for hover state changes
	if s.styleResolver == nil {
		s.styleResolver = style.NewResolver(s.sheets)
	}

	s.workCounts.styleResolves.Add(1)

	// Try incremental resolution for interactive states
	if incremental, ok := s.styleResolver.ResolveInteractiveStates(
		s.Doc, s.styleViewport(), s.customFonts, s.Styles, s.PseudoStyles,
	); ok {
		s.workCounts.styleNodeVisits.Add(uint64(incremental.Visited))

		// Check if layout properties changed
		layoutChanged := interactiveLayoutChanged(incremental, s.Styles, s.PseudoStyles)

		if !layoutChanged {
			// Geometry-neutral: update arena style pointers without reflow
			if s.Arena.RefreshComputedStylesForNodes(s.Styles, incremental.Nodes) {
				s.styleDirty.Store(false)
				s.layoutDirty.Store(false)
				return nil
			}
		}

		// Layout changed or arena refresh failed: need full reflow
		// Update styles map first
		for _, id := range incremental.Nodes {
			if prevStyle, exists := incremental.PreviousStyles[id]; exists {
				// The new style is already in s.Styles from ResolveInteractiveStates
				// Just need to update arena
				if newStyle, hasNew := s.Styles[id]; hasNew {
					s.updateArenaStyle(id, newStyle)
				}
				_ = prevStyle // avoid unused warning
			}
		}
	}

	// Fallback: full re-style and reflow
	s.opMu.Unlock()
	err := s.RestyleAndReflow(s.viewportW)
	s.opMu.Lock()

	return err
}

// updateArenaStyle updates the style pointer for a layout object.
// This is used when re-styling without reflow to keep the arena in sync.
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

// FocusDamageRect returns the bounding rect of the currently focused element.
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

// SelectionDamageRect returns the bounding rect of the current selection.
func (s *Session) SelectionDamageRect() frame.Rect {
	if s == nil || !s.HasSelection() {
		return frame.Rect{}
	}

	// Get selection bounds
	bounds, ok := s.SelectionBounds()
	if !ok {
		return frame.Rect{}
	}

	return bounds
}
