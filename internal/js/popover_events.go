package js

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

// PopoverInvoker returns the current invoker associated with an open popover.
// Layout uses this relationship as the implicit anchor for position-area:auto.
func (r *Runtime) PopoverInvoker(node *dom.Node) *dom.Node {
	if r == nil || node == nil || !node.IsPopoverOpen || r.popoverInvokers == nil {
		return nil
	}
	source := r.popoverInvokers[node]
	if source == nil || !source.IsConnected() || source.Doc != node.Doc {
		return nil
	}
	return source
}

func (r *Runtime) preparePopoverAttributeChange(node *dom.Node, name, newValue string, present bool) {
	if node == nil || node.Namespace != dom.NSHTML || !strings.EqualFold(name, "popover") || !node.IsPopoverOpen {
		return
	}
	oldMode := popoverMode(node.GetAttribute("popover"), true)
	newMode := popoverMode(newValue, present)
	if oldMode != newMode {
		r.setPopoverOpen(node, false)
	}
}

func popoverMode(value string, present bool) string {
	if !present {
		return "none"
	}
	switch strings.ToLower(value) {
	case "", "auto":
		return "auto"
	case "hint":
		return "hint"
	default:
		return "manual"
	}
}

func (r *Runtime) isFullyActiveDocument(doc *dom.Document) bool {
	if r == nil {
		return false
	}
	// Documents with no browsing context (for example, synthetic documents
	// created by DOM APIs) are not made inactive by iframe lifecycle changes.
	// Only enforce the fully-active rule for this runtime's document or a
	// document tracked as an iframe browsing context.
	if doc == nil {
		return true
	}
	if doc == r.domDoc {
		return r.isFullyActiveBrowsingContext()
	}
	frame := r.iframeForDocument(doc)
	if frame == nil {
		return true
	}
	if r.iframeDocuments[frame] != doc || !frame.IsConnected() {
		return false
	}
	if child := r.iframeChildRuntimes[frame]; child != nil {
		return child.isFullyActiveBrowsingContext()
	}
	return true
}

func (r *Runtime) isFullyActiveBrowsingContext() bool {
	if r == nil {
		return false
	}
	// Top-level realms without a Document, including dedicated and shared
	// workers, are not nested browsing contexts and remain active.
	if r.iframeParent == nil {
		return true
	}
	if r.domDoc == nil {
		return false
	}
	frame := r.iframeParentFrame
	parent := r.iframeParent
	return frame != nil && frame.IsConnected() && !parent.iframeClosedContexts[frame] &&
		parent.iframeChildRuntimes[frame] == r && parent.iframeDocuments[frame] == r.domDoc &&
		parent.isFullyActiveBrowsingContext()
}

type popoverToggleTask struct {
	oldState string
	newState string
	source   *dom.Node
}

func (r *Runtime) firePopoverBeforeToggle(node *dom.Node, opening bool) bool {
	oldState, newState := "open", "closed"
	if opening {
		oldState, newState = "closed", "open"
	}
	event := dom.NewToggleEvent("beforetoggle", false, opening, oldState, newState)
	if r.popoverTransitionSources != nil {
		event.Source = r.popoverTransitionSources[node]
	}
	r.popoverBeforeToggleDepth++
	defer func() { r.popoverBeforeToggleDepth-- }()
	return dom.DispatchEvent(node, &event.Event)
}

func (r *Runtime) queuePopoverToggle(node *dom.Node, oldState, newState string, separate bool, source *dom.Node) {
	if r.popoverToggleTasks == nil {
		r.popoverToggleTasks = make(map[*dom.Node][]*popoverToggleTask)
	}
	if tasks := r.popoverToggleTasks[node]; len(tasks) != 0 && !separate {
		tasks[len(tasks)-1].newState = newState
		return
	}
	pending := &popoverToggleTask{oldState: oldState, newState: newState, source: source}
	r.popoverToggleTasks[node] = append(r.popoverToggleTasks[node], pending)
	r.QueueTask(func() {
		tasks := r.popoverToggleTasks[node]
		index := -1
		for i, task := range tasks {
			if task == pending {
				index = i
				break
			}
		}
		if index < 0 {
			return
		}
		tasks = append(tasks[:index], tasks[index+1:]...)
		if len(tasks) == 0 {
			delete(r.popoverToggleTasks, node)
		} else {
			r.popoverToggleTasks[node] = tasks
		}
		event := dom.NewToggleEvent("toggle", false, false, pending.oldState, pending.newState)
		event.Source = pending.source
		dom.DispatchEvent(node, &event.Event)
	})
}

func (r *Runtime) setPopoverOpen(node *dom.Node, opening bool) bool {
	return r.setPopoverOpenFrom(node, opening, nil)
}

func (r *Runtime) setPopoverOpenFrom(node *dom.Node, opening bool, source *dom.Node) bool {
	if node == nil || !node.Element() || !node.HasAttribute("popover") || node.IsPopoverOpen == opening {
		return false
	}
	if opening && r.popoverBeforeToggleDepth > 0 {
		panic(r.domError("InvalidStateError", "A popover cannot be shown while another popover's beforetoggle event is being dispatched."))
	}
	if r.popoverTransitionSources == nil {
		r.popoverTransitionSources = make(map[*dom.Node]*dom.Node)
	}
	previousSource, hadPreviousSource := r.popoverTransitionSources[node]
	r.popoverTransitionSources[node] = source
	defer func() {
		if hadPreviousSource {
			r.popoverTransitionSources[node] = previousSource
		} else {
			delete(r.popoverTransitionSources, node)
		}
	}()
	if r.popoverTransitions == nil {
		r.popoverTransitions = make(map[*dom.Node]bool)
	}
	wasTransitioning := r.popoverTransitions[node]
	allowCloseDuringAutofocus := wasTransitioning && !opening && r.popoverFocusTransitions[node]
	if wasTransitioning && !allowCloseDuringAutofocus {
		return false
	}
	r.popoverTransitions[node] = true
	defer func() {
		if !wasTransitioning {
			delete(r.popoverTransitions, node)
		}
	}()

	originalDoc := node.Doc
	originalMode := popoverMode(node.GetAttribute("popover"), true)
	if opening {
		r.assertAutoPopoversCanClose(node)
		if !r.firePopoverBeforeToggle(node, true) {
			return false
		}
		if !node.HasAttribute("popover") || !node.IsConnected() || node.Doc != originalDoc || popoverMode(node.GetAttribute("popover"), true) != originalMode {
			return false
		}
		value := strings.ToLower(node.GetAttribute("popover"))
		if value == "" || value == "auto" || value == "hint" {
			r.closeOpenAutoPopovers(node)
		}
		if !node.HasAttribute("popover") || !node.IsConnected() || node.Doc != originalDoc || popoverMode(node.GetAttribute("popover"), true) != originalMode {
			return false
		}
	} else {
		r.closeDescendantAutoPopovers(node)
		if !r.firePopoverBeforeToggle(node, false) {
			return false
		}
	}
	oldState, newState := "open", "closed"
	if opening {
		oldState, newState = "closed", "open"
	}
	if opening {
		if r.showingPopovers == nil {
			r.showingPopovers = make(map[*dom.Node]bool)
		}
		if r.popoverReentrantClose == nil {
			r.popoverReentrantClose = make(map[*dom.Node]bool)
		}
		r.showingPopovers[node] = true
	}
	node.IsPopoverOpen = opening
	if opening {
		if r.popoverFocusTransitions == nil {
			r.popoverFocusTransitions = make(map[*dom.Node]bool)
		}
		r.popoverFocusTransitions[node] = true
		r.focusAutofocusInPopover(node)
		delete(r.popoverFocusTransitions, node)
		delete(r.showingPopovers, node)
	} else if r.popoverInvokers != nil {
		delete(r.popoverInvokers, node)
	}
	separate := false
	if opening {
		separate = r.popoverReentrantClose[node]
		delete(r.popoverReentrantClose, node)
	} else if r.showingPopovers[node] {
		r.popoverReentrantClose[node] = true
	}
	r.queuePopoverToggle(node, oldState, newState, separate, source)
	r.notifyMutation()
	return true
}

func (r *Runtime) closeDescendantAutoPopovers(root *dom.Node) {
	if r.domDoc == nil || root == nil {
		return
	}
	var closing []*dom.Node
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node != root && node.Element() && node.IsPopoverOpen {
			mode := popoverMode(node.GetAttribute("popover"), true)
			if (mode == "auto" || mode == "hint") && r.popoverStackAncestor(node, root) {
				closing = append(closing, node)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.ShadowRoot != nil {
			for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
	}
	walk(&r.domDoc.Node)
	for i := len(closing) - 1; i >= 0; i-- {
		popover := closing[i]
		r.setPopoverOpen(popover, false)
	}
}

func (r *Runtime) closeAllAutoAndHintPopovers() {
	if r.domDoc == nil {
		return
	}
	var open []*dom.Node
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() && node.IsPopoverOpen {
			mode := popoverMode(node.GetAttribute("popover"), node.HasAttribute("popover"))
			if mode == "auto" || mode == "hint" {
				open = append(open, node)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.ShadowRoot != nil {
			for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
	}
	walk(&r.domDoc.Node)
	for _, popover := range open {
		if r.popoverTransitions[popover] {
			panic(r.domError("InvalidStateError", "The popover is already being shown or hidden."))
		}
	}
	for i := len(open) - 1; i >= 0; i-- {
		r.setPopoverOpen(open[i], false)
	}
}

func (r *Runtime) popoversToClose(except *dom.Node) []*dom.Node {
	if r.domDoc == nil {
		return nil
	}
	openingMode := popoverMode(except.GetAttribute("popover"), except.HasAttribute("popover"))
	var closing []*dom.Node
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node != except && node.Element() && node.IsPopoverOpen {
			mode := popoverMode(node.GetAttribute("popover"), true)
			shouldClose := openingMode == "auto" && (mode == "auto" || mode == "hint") || openingMode == "hint" && mode == "hint"
			if shouldClose && !r.popoverStackAncestor(except, node) {
				closing = append(closing, node)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.ShadowRoot != nil {
			for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
	}
	walk(&r.domDoc.Node)
	return closing
}

func (r *Runtime) assertAutoPopoversCanClose(except *dom.Node) {
	for _, popover := range r.popoversToClose(except) {
		if r.popoverTransitions[popover] {
			panic(r.domError("InvalidStateError", "The popover is already being shown or hidden."))
		}
	}
}

func (r *Runtime) focusAutofocusInPopover(popover *dom.Node) {
	if popover == nil || r.opts.OnFocusChange == nil {
		return
	}
	var autofocus *dom.Node
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil || autofocus != nil {
			return
		}
		if node != popover && node.Element() && node.HasAttribute("autofocus") {
			autofocus = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(popover)
	if autofocus != nil {
		r.opts.OnFocusChange(autofocus, true)
	}
}

// closePopoverSubtree removes disconnected popovers from the open state
// without firing beforetoggle. The follow-up toggle event remains queued.
func (r *Runtime) closePopoverSubtree(root *dom.Node) {
	if root == nil {
		return
	}
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() && node.IsPopoverOpen {
			node.IsPopoverOpen = false
			r.queuePopoverToggle(node, "open", "closed", false, nil)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
}

func popoverAncestorOrSelf(node, ancestor *dom.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if current == ancestor {
			return true
		}
	}
	return false
}

func (r *Runtime) handleTestDriverPointerUp(target *dom.Node, pointerID string, button int) bool {
	if target == nil || r.domDoc == nil || r.testDriverPointerDown == nil {
		return false
	}
	down := r.testDriverPointerDown[pointerID]
	delete(r.testDriverPointerDown, pointerID)
	if down == nil && len(r.testDriverPointerDown) == 1 {
		// WebDriver creates a fresh pointer source identifier for separate
		// Actions objects. Preserve the single held pointer across that boundary.
		for id, pendingDown := range r.testDriverPointerDown {
			down = pendingDown
			delete(r.testDriverPointerDown, id)
		}
	}
	if down == nil {
		return false
	}
	if button != 0 {
		return false
	}
	if down.Doc != target.Doc || !down.IsConnected() || !target.IsConnected() {
		return false
	}
	var open []*dom.Node
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() && node.IsPopoverOpen {
			mode := popoverMode(node.GetAttribute("popover"), true)
			if mode == "auto" || mode == "hint" {
				open = append(open, node)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.ShadowRoot != nil {
			for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
	}
	walk(&r.domDoc.Node)
	dismissed := false
	for i := len(open) - 1; i >= 0; i-- {
		popover := open[i]
		if r.insidePopoverOrInvoker(down, popover) || r.insidePopoverOrInvoker(target, popover) {
			continue
		}
		dismissed = r.setPopoverOpen(popover, false) || dismissed
	}
	return dismissed
}

func (r *Runtime) insidePopoverOrInvoker(node, popover *dom.Node) bool {
	if node == nil || popover == nil {
		return false
	}
	id := popover.GetAttribute("id")
	for current := node; current != nil; {
		if current == popover {
			return true
		}
		if r.popoverInvokers[popover] == current {
			return true
		}
		if id != "" && current.Element() && current.GetAttribute("popovertarget") == id {
			return true
		}
		if id != "" && current.Element() && current.GetAttribute("commandfor") == id {
			if _, ok := popoverCommandAction(current.GetAttribute("command")); ok {
				formActionTakesPrecedence := r.formOwner(current) != nil && (isSubmitButton(current) || isResetButton(current))
				if !formActionTakesPrecedence {
					return true
				}
			}
		}
		if current.Parent != nil {
			current = current.Parent
			continue
		}
		current, _ = current.ShadowHost()
	}
	return false
}

func (r *Runtime) popoverStackAncestor(popover, ancestor *dom.Node) bool {
	seen := make(map[*dom.Node]struct{})
	for current := popover; current != nil; {
		if _, exists := seen[current]; exists {
			return false
		}
		seen[current] = struct{}{}
		if popoverAncestorOrSelf(current, ancestor) {
			return true
		}
		if r.popoverInvokers == nil {
			return false
		}
		invoker := r.popoverInvokers[current]
		if invoker == nil {
			return false
		}
		if popoverAncestorOrSelf(invoker, ancestor) {
			return true
		}
		current = nearestOpenAutoPopoverAncestor(invoker)
	}
	return false
}

func nearestOpenAutoPopoverAncestor(node *dom.Node) *dom.Node {
	for current := node; current != nil; current = current.Parent {
		if current.Element() && current.IsPopoverOpen {
			mode := popoverMode(current.GetAttribute("popover"), current.HasAttribute("popover"))
			if mode == "auto" || mode == "hint" {
				return current
			}
		}
	}
	return nil
}
