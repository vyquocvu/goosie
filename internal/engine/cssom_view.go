package engine

import (
	"math"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// layoutBoxInfo returns the CSSOM View geometry for one DOM node from the
// session's layout arena. It reads the border-box position and size the arena
// holds and translates them into the shape internal/js needs without letting
// that package import internal/layout (the archtest table forbids it).
func (s *Session) layoutBoxInfo(n *dom.Node) (js.LayoutBoxInfo, bool) {
	if s == nil || n == nil {
		return js.LayoutBoxInfo{}, false
	}
	s.refreshLayoutForCSSOMRead()
	s.opMu.Lock()
	arena := s.Arena
	computed := s.Styles[n.ID]
	s.opMu.Unlock()
	id, ok := arena.ForNode(n.ID)
	if !ok {
		return js.LayoutBoxInfo{}, false
	}
	obj := arena.Get(id)
	if obj == nil {
		return js.LayoutBoxInfo{}, false
	}
	// The border-box position is the object's origin; the border-box size
	// includes content, padding and border on each axis.
	bx0, by0, bx1, by1 := obj.BorderRect()
	if (bx0 == bx1 || by0 == by1) && computed != nil && computed.Display == style.DisplayInline {
		// Inline elements can have no standalone arena box even though their
		// text descendants occupy line fragments. CSSOM View measures such an
		// element by the union of its client rects, so use the rendered
		// descendant fragments when the element's own box is empty.
		if x0, y0, x1, y1, found := descendantFragmentBounds(arena, n); found {
			bx0, by0, bx1, by1 = x0, y0, x1, y1
		} else if hasRenderedText(n) {
			// The inline formatter can fold a short text-only inline into its
			// parent's line box without retaining a node-indexed text fragment.
			// In that case, anchor hit testing to the nearest rendered ancestor
			// box so element-origin pointer actions still land in the content.
			for parent := n.Parent; parent != nil; parent = parent.Parent {
				if parent.Type == dom.NodeDocument {
					break
				}
				if parentID, ok := arena.ForNode(parent.ID); ok {
					if parentBox := arena.Get(parentID); parentBox != nil {
						x0, y0, x1, y1 := parentBox.BorderRect()
						if x0 < x1 && y0 < y1 {
							bx0, by0, bx1, by1 = x0, y0, x1, y1
							break
						}
					}
				}
			}
		}
	}
	info := js.LayoutBoxInfo{
		X:             float64(bx0),
		Y:             float64(by0),
		Width:         float64(bx1 - bx0),
		Height:        float64(by1 - by0),
		BorderTop:     float64(obj.BorderTop),
		BorderRight:   float64(obj.BorderRight),
		BorderBottom:  float64(obj.BorderBottom),
		BorderLeft:    float64(obj.BorderLeft),
		PaddingTop:    float64(obj.PaddingTop),
		PaddingRight:  float64(obj.PaddingRight),
		PaddingBottom: float64(obj.PaddingBottom),
		PaddingLeft:   float64(obj.PaddingLeft),
		MarginTop:     float64(obj.MarginTop),
		MarginRight:   float64(obj.MarginRight),
		MarginBottom:  float64(obj.MarginBottom),
		MarginLeft:    float64(obj.MarginLeft),
	}
	// Scrollable area: the content box extended by whatever overflows it, plus
	// the container's own trailing padding, and the scrollability the page may
	// write an offset against.
	info.ScrollWidth, info.ScrollHeight = scrollableOverflow(arena, obj)
	info.ScrollContainer = computed != nil &&
		(computed.OverflowX != style.OverflowVisible || computed.OverflowY != style.OverflowVisible)
	return info, true
}

// scrollableOverflow measures the area one laid-out box can scroll to. It starts
// at the content box, extends to the furthest margin edge of any descendant, and
// adds the box's own trailing padding. Two kinds of descendant stop contributing
// at their own border box: one that clips or scrolls, because its content belongs
// to that box's scrollable area and not this one's, and a fixed-position box,
// because it is positioned against the viewport rather than this container.
//
// Measured against Chromium with test/wpt/resources/element_scroll_probe.py:
// a 100x100 scroller over a 200x200 child is 200, the same child with 60px of
// right and bottom margin is 160, trailing padding of 40/60 makes it 240x260, an
// absolutely positioned child counts, a fixed one does not, and a nested
// scroller leaves the outer box at 100.
func scrollableOverflow(arena *layout.Arena, root *layout.Object) (float64, float64) {
	contentX, contentY, contentX1, contentY1 := root.ContentRect()
	extentX, extentY := contentX1-contentX, contentY1-contentY
	var walk func(layout.ObjectID)
	walk = func(id layout.ObjectID) {
		for child := id; child != 0; {
			obj := arena.Get(child)
			if obj == nil {
				break
			}
			next := obj.NextSibling
			if obj.Style != nil && obj.Style.Position == style.PositionFixed {
				child = next
				continue
			}
			// Only the trailing edges can extend the area: content that sticks out
			// to the left or above the content box is not scrollable to.
			_, _, x1, y1 := obj.BorderRect()
			if x1+obj.MarginRight-contentX > extentX {
				extentX = x1 + obj.MarginRight - contentX
			}
			if y1+obj.MarginBottom-contentY > extentY {
				extentY = y1 + obj.MarginBottom - contentY
			}
			if obj.Style == nil || (obj.Style.OverflowX == style.OverflowVisible && obj.Style.OverflowY == style.OverflowVisible) {
				walk(obj.FirstKid)
			}
			child = next
		}
	}
	walk(root.FirstKid)
	if extentX < 0 {
		extentX = 0
	}
	if extentY < 0 {
		extentY = 0
	}
	// The scrollable area is rounded up, and a box cannot scroll into its own
	// leading padding, so only trailing padding extends it.
	return math.Ceil(float64(extentX + root.PaddingRight)), math.Ceil(float64(extentY + root.PaddingBottom))
}

func hasRenderedText(root *dom.Node) bool {
	if root == nil {
		return false
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		// Whitespace-only light-DOM text can remain between a declarative shadow
		// root's host tags. It must not make an empty inline host inherit the
		// entire ancestor box for hit testing.
		if child.Text() && strings.TrimSpace(child.DataContent) != "" {
			return true
		}
		if hasRenderedText(child) {
			return true
		}
	}
	return false
}

func descendantFragmentBounds(arena *layout.Arena, root *dom.Node) (x0, y0, x1, y1 float32, found bool) {
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node != root {
			if id, ok := arena.ForNode(node.ID); ok {
				if obj := arena.Get(id); obj != nil {
					a, b, c, d := obj.BorderRect()
					if a < c && b < d {
						if !found {
							x0, y0, x1, y1, found = a, b, c, d, true
						} else {
							if a < x0 {
								x0 = a
							}
							if b < y0 {
								y0 = b
							}
							if c > x1 {
								x1 = c
							}
							if d > y1 {
								y1 = d
							}
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		walk(child)
	}
	return
}

// refreshLayoutForCSSOMRead performs the synchronous style and layout flush
// required by offset geometry and getBoundingClientRect during script
// execution. NewSession runs scripts before its first presentation layout, so
// without this path an early read would report zero for every element.
func (s *Session) refreshLayoutForCSSOMRead() {
	if s == nil || !s.layoutDirty.Load() {
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if !s.layoutDirty.Load() || s.Doc == nil {
		return
	}
	if s.styleDirty.Swap(false) && s.styleResolver != nil {
		s.Styles = s.styleResolver.ResolveViewport(s.Doc, s.styleViewport(), s.customFonts)
		s.workCounts.styleResolves.Add(1)
		s.workCounts.styleNodeVisits.Add(uint64(len(s.Styles)))
		s.PseudoStyles = s.styleResolver.ResolvePseudoElements(s.Doc, s.Styles, s.customFonts)
	}
	candidate := layout.Build(s.Doc, s.Styles, s.PseudoStyles)
	candidate.Metrics = s.metrics
	candidate.NaturalSizes = s.natural
	layout.Block(candidate, layout.ObjectID(1), s.viewportW, s.viewportH)
	layout.Inline(candidate, layout.ObjectID(1))
	s.applyImplicitPopoverAnchors(candidate)
	layout.Positioning(candidate, layout.ObjectID(1), s.viewportW, s.viewportH)
	s.Arena = candidate
	s.workCounts.layoutBuilds.Add(1)
	s.layoutDirty.Store(false)
}

// offsetParentNode returns the nearest positioned ancestor for n, matching the
// CSSOM View spec: the closest ancestor whose position is not static. The
// document root and the body element answer nil (which the JS binding turns
// into null), as the spec requires.
func (s *Session) offsetParentNode(n *dom.Node) *dom.Node {
	if s == nil || n == nil {
		return nil
	}
	s.opMu.Lock()
	styles := s.Styles
	s.opMu.Unlock()
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == dom.NodeDocument {
			return nil
		}
		if !p.Element() {
			continue
		}
		cs := styles[p.ID]
		if cs == nil {
			continue
		}
		if cs.Position != style.PositionStatic {
			return p
		}
		// The body element is the offsetParent of its children when no
		// intermediate ancestor is positioned, unless body itself is
		// positioned.
		if p.Data == "body" {
			return p
		}
	}
	return nil
}

// installLayoutProviders wires the session's layout arena readers into the JS
// runtime so getBoundingClientRect, offset properties and scroll properties
// answer from the real geometry. Called alongside SetComputedStyleProvider.
func (s *Session) installLayoutProviders() {
	if s.jsRT == nil {
		return
	}
	s.jsRT.SetLayoutGeometryProvider(s.layoutBoxInfo)
	s.jsRT.SetOffsetParentProvider(s.offsetParentNode)
}

// nodeLayoutBox is a test helper that returns the border-box rect of one node
// from the arena. It is unused in production; tests reach it through the
// engine package.
func nodeLayoutBox(a *layout.Arena, n *dom.Node) (x, y, w, h float64, ok bool) {
	if a == nil || n == nil {
		return
	}
	id, found := a.ForNode(n.ID)
	if !found {
		return
	}
	obj := a.Get(id)
	if obj == nil {
		return
	}
	x0, y0, x1, y1 := obj.BorderRect()
	return float64(x0), float64(y0), float64(x1 - x0), float64(y1 - y0), true
}
