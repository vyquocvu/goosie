package engine

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func (s *Session) applyImplicitPopoverAnchors(a *layout.Arena) {
	if a == nil {
		return
	}
	for i := range a.Objects {
		obj := &a.Objects[i]
		if obj.Node == nil || obj.Style == nil || !obj.Node.HasAttribute("popover") {
			continue
		}
		obj.Style.PositionAnchorNodeID = 0
		if s.jsRT != nil {
			if source := s.jsRT.PopoverInvoker(obj.Node); source != nil {
				obj.Style.PositionAnchorNodeID = source.ID
			}
		}
	}
}

func (s *Session) Refresh(plan Plan, authorCSS []string, viewportW float32) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if plan.FullDoc == false && len(plan.Subtree) == 0 && plan.StyleObjects == 0 {
		return false
	}
	s.recordViewportWidth(viewportW)

	if plan.FullDoc {
		var sheets []*css.Stylesheet
		for _, src := range authorCSS {
			sheets = append(sheets, css.ParseWithOptions(src, s.mediaOptions()))
		}
		s.sheets = sheets
		s.styleResolver = style.NewResolver(sheets)
		s.workCounts.styleResolves.Add(1)
		s.Styles = s.styleResolver.ResolveViewport(s.Doc, s.styleViewport(), s.customFonts)
		s.workCounts.styleNodeVisits.Add(uint64(len(s.Styles)))
		s.PseudoStyles = s.styleResolver.ResolvePseudoElements(s.Doc, s.Styles, s.customFonts)
		s.workCounts.layoutBuilds.Add(1)
		s.Arena = layout.Build(s.Doc, s.Styles, s.PseudoStyles)
		layout.Block(s.Arena, layout.ObjectID(1), viewportW, s.viewportH)
		layout.Inline(s.Arena, layout.ObjectID(1))
		s.applyImplicitPopoverAnchors(s.Arena)
		layout.Positioning(s.Arena, layout.ObjectID(1), viewportW, s.viewportH)
		s.layoutDirty.Store(false)
		return true
	}

	if len(plan.Subtree) > 0 || plan.StyleObjects > 0 {
		var sheets []*css.Stylesheet
		for _, src := range authorCSS {
			sheets = append(sheets, css.ParseWithOptions(src, s.mediaOptions()))
		}
		s.sheets = sheets
		s.styleResolver = style.NewResolver(sheets)
		s.workCounts.styleResolves.Add(1)
		s.Styles = s.styleResolver.ResolveViewport(s.Doc, s.styleViewport(), s.customFonts)
		s.workCounts.styleNodeVisits.Add(uint64(len(s.Styles)))
		s.PseudoStyles = s.styleResolver.ResolvePseudoElements(s.Doc, s.Styles, s.customFonts)
		s.workCounts.layoutBuilds.Add(1)
		s.Arena = layout.Build(s.Doc, s.Styles, s.PseudoStyles)
		layout.Block(s.Arena, layout.ObjectID(1), viewportW, s.viewportH)
		layout.Inline(s.Arena, layout.ObjectID(1))
		s.applyImplicitPopoverAnchors(s.Arena)
		layout.Positioning(s.Arena, layout.ObjectID(1), viewportW, s.viewportH)
		s.layoutDirty.Store(false)
		return true
	}

	return false
}

func (s *Session) RefreshNode(node *dom.Node, authorCSS []string, viewportW float32) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if node == nil {
		return
	}
	s.recordViewportWidth(viewportW)

	var sheets []*css.Stylesheet
	for _, src := range authorCSS {
		sheets = append(sheets, css.ParseWithOptions(src, s.mediaOptions()))
	}
	s.sheets = sheets
	s.styleResolver = style.NewResolver(sheets)

	s.workCounts.styleResolves.Add(1)
	s.Styles = s.styleResolver.ResolveViewport(s.Doc, s.styleViewport(), s.customFonts)
	s.workCounts.styleNodeVisits.Add(uint64(len(s.Styles)))
	s.PseudoStyles = s.styleResolver.ResolvePseudoElements(s.Doc, s.Styles, s.customFonts)

	s.workCounts.layoutBuilds.Add(1)
	s.Arena = layout.Build(s.Doc, s.Styles, s.PseudoStyles)
	layout.Block(s.Arena, layout.ObjectID(1), viewportW, s.viewportH)
	layout.Inline(s.Arena, layout.ObjectID(1))
	s.applyImplicitPopoverAnchors(s.Arena)
	layout.Positioning(s.Arena, layout.ObjectID(1), viewportW, s.viewportH)
	s.layoutDirty.Store(false)
}
