package js

import (
	"github.com/vyquocvu/goosie/internal/dom"
)

type detailsToggleTask struct {
	oldState string
	newState string
	source   *dom.Node
	timerID  int
}

// SetDetailsOpen applies the open IDL/default-action transition, including the
// exclusive name group and the queued toggle event.
func (r *Runtime) SetDetailsOpen(node *dom.Node, open bool, source *dom.Node) {
	if r == nil || node == nil || !node.Element() || node.Data != "details" || node.HasAttribute("open") == open {
		return
	}
	wasOpen := node.HasAttribute("open")
	if open {
		node.SetAttribute("open", "")
	} else {
		node.RemoveAttribute("open")
	}
	r.detailsOpenChanged(node, wasOpen, open, source)
	r.notifyMutation()
}

func (r *Runtime) detailsOpenChanged(node *dom.Node, wasOpen, isOpen bool, source *dom.Node) {
	if node == nil || wasOpen == isOpen {
		return
	}
	oldState, newState := "closed", "open"
	if wasOpen {
		oldState, newState = "open", "closed"
	}
	r.queueDetailsToggle(node, oldState, newState, source)
	if isOpen {
		r.closeOtherNamedDetails(node)
	}
}

func (r *Runtime) detailsNameChanged(node *dom.Node) {
	if node != nil && node.Element() && node.Data == "details" && node.HasAttribute("open") {
		name := node.GetAttribute("name")
		if name == "" {
			return
		}
		walk, root := dom.DetailsTreeScope(node)
		conflict := false
		if root != nil {
			walk(root, func(other *dom.Node) {
				if other != node && other.Element() && other.Data == "details" && other.HasAttribute("open") && other.GetAttribute("name") == name {
					conflict = true
				}
			})
		}
		if conflict {
			node.RemoveAttribute("open")
			r.queueDetailsToggle(node, "open", "closed", nil)
			r.notifyMutation()
		}
	}
}

func (r *Runtime) queueInitialDetailsToggleEvents() {
	if r == nil {
		return
	}
	r.queueInitialDetailsToggleEventsForDocument(r.domDoc)
}

func (r *Runtime) queueInitialDetailsToggleEventsForDocument(doc *dom.Document) {
	if r == nil || doc == nil {
		return
	}
	if r.detailsInitialToggleDocs == nil {
		r.detailsInitialToggleDocs = make(map[*dom.Document]bool)
	}
	if r.detailsInitialToggleDocs[doc] {
		return
	}
	r.detailsInitialToggleDocs[doc] = true
	var scan func(*dom.Node)
	scan = func(root *dom.Node) {
		seen := make(map[string]bool)
		var walk func(*dom.Node)
		walk = func(parent *dom.Node) {
			for node := parent; node != nil; node = node.NextSibling {
				if node.Element() && node.Data == "details" && node.HasAttribute("open") {
					name := node.GetAttribute("name")
					if name != "" && seen[name] {
						node.RemoveAttribute("open")
					} else {
						if name != "" {
							seen[name] = true
						}
						r.queueInitialDetailsToggleForNode(node)
					}
				}
				if node.ShadowRoot != nil {
					scan(node.ShadowRoot.FirstChild)
				}
				walk(node.FirstChild)
			}
		}
		walk(root)
	}
	scan(&doc.Node)
}

func (r *Runtime) queueInitialDetailsToggleForNode(node *dom.Node) {
	if r == nil || node == nil || !node.Element() || node.Data != "details" || !node.HasAttribute("open") {
		return
	}
	if r.detailsInitialToggleNodes == nil {
		r.detailsInitialToggleNodes = make(map[*dom.Node]bool)
	}
	if r.detailsInitialToggleNodes[node] {
		return
	}
	r.detailsInitialToggleNodes[node] = true
	r.queueDetailsToggle(node, "closed", "open", nil)
}

func (r *Runtime) queueDetailsToggle(node *dom.Node, oldState, newState string, source *dom.Node) {
	if r.detailsToggleTasks == nil {
		r.detailsToggleTasks = make(map[*dom.Node]*detailsToggleTask)
	}
	if pending := r.detailsToggleTasks[node]; pending != nil {
		// A later state change coalesces the old/new states but queues the
		// element's toggle notification at the tail of the task source. This
		// preserves the ordering observed when parser-created open members are
		// subsequently displaced by a script mutation in the same task.
		delete(r.timers, pending.timerID)
		pending = &detailsToggleTask{oldState: pending.oldState, newState: newState, source: source}
		r.detailsToggleTasks[node] = pending
		r.queueDetailsToggleTask(node, pending)
		return
	}
	pending := &detailsToggleTask{oldState: oldState, newState: newState, source: source}
	r.detailsToggleTasks[node] = pending
	r.queueDetailsToggleTask(node, pending)
}

func (r *Runtime) queueDetailsToggleTask(node *dom.Node, pending *detailsToggleTask) {
	pending.timerID = r.QueueTask(func() {
		if r.detailsToggleTasks[node] != pending {
			return
		}
		delete(r.detailsToggleTasks, node)
		event := dom.NewTrustedToggleEvent("toggle", false, false, pending.oldState, pending.newState)
		event.Source = pending.source
		dom.DispatchEvent(node, &event.Event)
	})
}

func (r *Runtime) closeOtherNamedDetails(node *dom.Node) {
	for _, other := range dom.CloseOtherNamedDetails(node) {
		r.queueDetailsToggle(other, "open", "closed", nil)
	}
}

func (r *Runtime) normalizeInsertedDetailsGroup(node *dom.Node) {
	name := node.GetAttribute("name")
	if name == "" {
		r.queueDetailsToggle(node, "closed", "open", nil)
		return
	}
	walk, root := dom.DetailsTreeScope(node)
	var open []*dom.Node
	if root != nil {
		walk(root, func(other *dom.Node) {
			if other.Element() && other.Data == "details" && other.HasAttribute("open") && other.GetAttribute("name") == name {
				open = append(open, other)
			}
		})
	}
	if len(open) == 0 {
		r.queueDetailsToggle(node, "closed", "open", nil)
		return
	}
	if open[0] != node {
		node.RemoveAttribute("open")
		r.queueDetailsToggle(node, "open", "closed", nil)
		r.notifyMutation()
		return
	}
	r.queueDetailsToggle(node, "closed", "open", nil)
	for _, other := range open[1:] {
		other.RemoveAttribute("open")
		r.queueDetailsToggle(other, "open", "closed", nil)
		r.notifyMutation()
	}
}

func (r *Runtime) normalizeDOMInsertedDetailsGroup(node *dom.Node) {
	name := node.GetAttribute("name")
	if name == "" {
		r.queueDetailsToggle(node, "closed", "open", nil)
		return
	}
	walk, root := dom.DetailsTreeScope(node)
	hasOpenPeer := false
	if root != nil {
		walk(root, func(other *dom.Node) {
			if other != node && other.Element() && other.Data == "details" && other.HasAttribute("open") && other.GetAttribute("name") == name {
				hasOpenPeer = true
			}
		})
	}
	if hasOpenPeer {
		node.RemoveAttribute("open")
		r.queueDetailsToggle(node, "open", "closed", nil)
		r.notifyMutation()
		return
	}
	r.queueDetailsToggle(node, "closed", "open", nil)
}
