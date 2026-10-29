package js

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

// setDatasetAttribute applies a DOMStringMap write through the element's
// attribute setter. Attribute reactions run for same-value writes, while
// visual invalidation is limited to an actual stored-value change.
func (r *Runtime) setDatasetAttribute(node *dom.Node, name, value string) {
	if node == nil || !node.Element() {
		return
	}
	oldValue, hadOld := node.GetAttributeNS("", name)
	node.SetAttribute(name, value)
	newValue, hasNew := node.GetAttributeNS("", name)
	r.notifyCustomElementAttributeChanged(node, strings.ToLower(name), oldValue, hadOld, newValue, hasNew, "")
	if oldValue != newValue || hadOld != hasNew {
		r.notifyMutation()
	}
}

// deleteDatasetAttribute queues the observed-attribute reaction only when a
// data-* attribute was actually present and removed.
func (r *Runtime) deleteDatasetAttribute(node *dom.Node, name string) {
	if node == nil || !node.Element() {
		return
	}
	oldValue, hadOld := node.GetAttributeNS("", name)
	if !hadOld || !node.RemoveAttributeNS("", name) {
		return
	}
	r.notifyCustomElementAttributeChanged(node, strings.ToLower(name), oldValue, true, "", false, "")
	r.notifyMutation()
}
