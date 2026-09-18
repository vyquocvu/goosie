package engine

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// Refresh applies an invalidation plan to the session, re-styling and
// re-layout only the affected subtrees. Scroll-only plans are a no-op, which
// is how invariant 1 holds: a scroll frame runs no style and no layout.
//
// The method returns true if any work was done, false if the plan was a
// scroll-only update or empty.
func (s *Session) Refresh(plan Plan, authorCSS []string, viewportW float32) bool {
	// Scroll-only or empty plan: no work needed.
	if plan.FullDoc == false && len(plan.Subtree) == 0 && plan.StyleObjects == 0 {
		return false
	}
	s.recordViewportWidth(viewportW)

	// Full document: re-run the entire pipeline.
	if plan.FullDoc {
		var sheets []*css.Stylesheet
		for _, src := range authorCSS {
			sheets = append(sheets, css.Parse(src))
		}
		s.Styles = style.ResolveViewport(s.Doc, sheets, s.styleViewport())
		s.Arena = layout.Build(s.Doc, s.Styles)
		layout.Block(s.Arena, layout.ObjectID(1), viewportW)
		layout.Inline(s.Arena, layout.ObjectID(1))
		layout.Positioning(s.Arena, layout.ObjectID(1), viewportW, s.viewportH)
		return true
	}

	// Partial refresh: re-style and re-layout only affected subtrees.
	// For now, this is a simplified implementation that re-runs the full
	// pipeline if any work is needed. A production implementation would
	// walk only the affected subtrees.
	if len(plan.Subtree) > 0 || plan.StyleObjects > 0 {
		var sheets []*css.Stylesheet
		for _, src := range authorCSS {
			sheets = append(sheets, css.Parse(src))
		}
		s.Styles = style.ResolveViewport(s.Doc, sheets, s.styleViewport())
		s.Arena = layout.Build(s.Doc, s.Styles)
		layout.Block(s.Arena, layout.ObjectID(1), viewportW)
		layout.Inline(s.Arena, layout.ObjectID(1))
		layout.Positioning(s.Arena, layout.ObjectID(1), viewportW, s.viewportH)
		return true
	}

	return false
}

// RefreshNode re-styles and re-layouts a single node and its descendants.
// This is the granular refresh path for hover, style changes, and DOM mutations.
func (s *Session) RefreshNode(node *dom.Node, authorCSS []string, viewportW float32) {
	if node == nil {
		return
	}
	s.recordViewportWidth(viewportW)

	// Re-style this node and its descendants.
	var sheets []*css.Stylesheet
	for _, src := range authorCSS {
		sheets = append(sheets, css.Parse(src))
	}

	// Build a parent chain to compute the node's style context.
	// For now, re-run the full style resolution. A production implementation
	// would only re-style the affected subtree.
	s.Styles = style.ResolveViewport(s.Doc, sheets, s.styleViewport())

	// Rebuild the arena. A production implementation would only re-layout
	// the affected subtree.
	s.Arena = layout.Build(s.Doc, s.Styles)
	layout.Block(s.Arena, layout.ObjectID(1), viewportW)
	layout.Inline(s.Arena, layout.ObjectID(1))
	layout.Positioning(s.Arena, layout.ObjectID(1), viewportW, s.viewportH)
}
