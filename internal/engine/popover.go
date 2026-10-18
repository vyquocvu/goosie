package engine

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

// ShowPopover opens a popover element if it is not already open.
// Returns true if the popover transitioned from closed to open.
func (s *Session) ShowPopover(n *dom.Node) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.showPopoverLocked(n)
}

func (s *Session) showPopoverLocked(n *dom.Node) bool {
	if n == nil || !n.Element() || !n.HasAttribute("popover") || n.IsPopoverOpen {
		return false
	}
	pType := strings.ToLower(n.GetAttribute("popover"))
	if pType == "" || pType == "auto" {
		s.closeOpenAutoPopoversLocked(n)
	}
	beforeEv := dom.NewEvent("beforetoggle", false, true)
	if !dom.DispatchEvent(n, beforeEv) {
		return false // canceled
	}
	n.IsPopoverOpen = true
	dom.DispatchEvent(n, dom.NewEvent("toggle", false, false))
	return true
}

// HidePopover closes a popover element if it is currently open.
// Returns true if the popover transitioned from open to closed.
func (s *Session) HidePopover(n *dom.Node) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.hidePopoverLocked(n)
}

func (s *Session) hidePopoverLocked(n *dom.Node) bool {
	if n == nil || !n.Element() || !n.HasAttribute("popover") || !n.IsPopoverOpen {
		return false
	}
	beforeEv := dom.NewEvent("beforetoggle", false, true)
	if !dom.DispatchEvent(n, beforeEv) {
		return false // canceled
	}
	n.IsPopoverOpen = false
	dom.DispatchEvent(n, dom.NewEvent("toggle", false, false))
	return true
}

// TogglePopover toggles the open state of a popover element.
// An optional force parameter can force open (true) or closed (false).
func (s *Session) TogglePopover(n *dom.Node, force ...bool) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.togglePopoverLocked(n, force...)
}

func (s *Session) togglePopoverLocked(n *dom.Node, force ...bool) bool {
	if n == nil || !n.Element() || !n.HasAttribute("popover") {
		return false
	}
	if len(force) > 0 {
		if force[0] {
			if !n.IsPopoverOpen {
				return s.showPopoverLocked(n)
			}
			return true
		} else {
			if n.IsPopoverOpen {
				return s.hidePopoverLocked(n)
			}
			return false
		}
	}
	if n.IsPopoverOpen {
		s.hidePopoverLocked(n)
		return false
	}
	return s.showPopoverLocked(n)
}

// CloseTopmostAutoPopover finds and closes the topmost open auto popover element.
func (s *Session) CloseTopmostAutoPopover() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.closeTopmostAutoPopoverLocked()
}

func (s *Session) closeTopmostAutoPopoverLocked() bool {
	if s.Doc == nil {
		return false
	}
	var topmost *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() && n.IsPopoverOpen {
			pType := strings.ToLower(n.GetAttribute("popover"))
			if pType == "" || pType == "auto" {
				topmost = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&s.Doc.Node)
	if topmost == nil {
		return false
	}
	return s.hidePopoverLocked(topmost)
}

func (s *Session) closeOpenAutoPopoversLocked(except *dom.Node) {
	if s.Doc == nil {
		return
	}
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n != except && n.Element() && n.IsPopoverOpen {
			pType := strings.ToLower(n.GetAttribute("popover"))
			if pType == "" || pType == "auto" {
				// Don't close ancestors of except
				if !isDescendantOrSelf(except, n) {
					beforeEv := dom.NewEvent("beforetoggle", false, true)
					if dom.DispatchEvent(n, beforeEv) {
						n.IsPopoverOpen = false
						dom.DispatchEvent(n, dom.NewEvent("toggle", false, false))
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&s.Doc.Node)
}

func (s *Session) handlePopoverTarget(targetID, action string) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.Doc == nil || targetID == "" {
		return
	}
	target := s.Doc.ElementByID(targetID)
	if target == nil || !target.HasAttribute("popover") {
		return
	}
	switch action {
	case "show":
		if !target.IsPopoverOpen {
			s.showPopoverLocked(target)
		}
	case "hide":
		if target.IsPopoverOpen {
			s.hidePopoverLocked(target)
		}
	default: // "toggle" or empty
		s.togglePopoverLocked(target)
	}
}

// dismissAutoPopoversExcept light-dismisses open auto popovers when a click
// occurs outside them.
func (s *Session) dismissAutoPopoversExcept(hitNode *dom.Node) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.Doc == nil {
		return false
	}
	var openPopovers []*dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() && n.IsPopoverOpen {
			pType := strings.ToLower(n.GetAttribute("popover"))
			if pType == "" || pType == "auto" {
				openPopovers = append(openPopovers, n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&s.Doc.Node)
	if len(openPopovers) == 0 {
		return false
	}

	dismissedAny := false
	for _, p := range openPopovers {
		if hitNode != nil && (isDescendantOrSelf(hitNode, p) || isInvokerFor(hitNode, p)) {
			continue
		}
		beforeEv := dom.NewEvent("beforetoggle", false, true)
		if dom.DispatchEvent(p, beforeEv) {
			p.IsPopoverOpen = false
			dom.DispatchEvent(p, dom.NewEvent("toggle", false, false))
			dismissedAny = true
		}
	}
	return dismissedAny
}

// nodeAt returns the deepest DOM node at the given document coordinates, or nil.
func (s *Session) nodeAt(x, y float32) *dom.Node {
	if s.Arena == nil {
		return nil
	}
	objs := s.Arena.Objects
	for i := len(objs) - 1; i >= 1; i-- {
		obj := &objs[i]
		x0, y0, x1, y1 := obj.BorderRect()
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			continue
		}
		if obj.Node != nil {
			return obj.Node
		}
	}
	return nil
}

func isDescendantOrSelf(n, ancestor *dom.Node) bool {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur == ancestor {
			return true
		}
	}
	return false
}

func isInvokerFor(n, popoverNode *dom.Node) bool {
	id := popoverNode.GetAttribute("id")
	if id == "" {
		return false
	}
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Element() && cur.GetAttribute("popovertarget") == id {
			return true
		}
	}
	return false
}
