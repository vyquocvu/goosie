package js

import (
	"strings"
	"unicode/utf16"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type textControlSelection struct {
	start, end int
	direction  string
}

func (r *Runtime) textSelection(node *dom.Node) textControlSelection {
	if r.textControlSelections == nil {
		r.textControlSelections = make(map[*dom.Node]textControlSelection)
	}
	selection, ok := r.textControlSelections[node]
	if !ok {
		selection.direction = "none"
		return selection
	}
	length := selectionLength(node)
	if selection.start > length {
		selection.start = length
	}
	if selection.end > length {
		selection.end = length
	}
	if selection.end <= selection.start {
		selection.start = selection.end
	}
	if r.textControlSelections[node] != selection {
		r.textControlSelections[node] = selection
		r.queueSelectEvent(node)
	}
	return selection
}

func textSelectionApplies(node *dom.Node) bool {
	if node == nil {
		return false
	}
	if node.Data == "textarea" {
		return true
	}
	if node.Data != "input" {
		return false
	}
	switch strings.ToLower(node.GetAttribute("type")) {
	case "email", "date", "month", "week", "time", "datetime-local", "number", "range", "color", "checkbox", "radio", "file", "hidden", "submit", "image", "reset", "button":
		return false
	default:
		// The missing and invalid states both use the Text state.
		return true
	}
}

func selectionValue(node *dom.Node) string {
	if node == nil {
		return ""
	}
	return node.InputControlValue()
}

func selectionLength(node *dom.Node) int {
	return len(utf16.Encode([]rune(selectionValue(node))))
}

func toUnsignedLong(value goja.Value) int {
	return int(uint32(value.ToInteger()))
}

func (r *Runtime) setTextSelection(node *dom.Node, start, end int, direction string) {
	previous := r.textSelection(node)
	length := selectionLength(node)
	if start > length {
		start = length
	}
	if end > length {
		end = length
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if end <= start {
		start = end
	}
	if direction != "forward" && direction != "backward" {
		direction = "none"
	}
	if r.textControlSelections == nil {
		r.textControlSelections = make(map[*dom.Node]textControlSelection)
	}
	updated := textControlSelection{start: start, end: end, direction: direction}
	if previous == updated {
		return
	}
	r.textControlSelections[node] = updated
	if node.Doc != nil {
		selection := r.selectionForDocument(node.Doc)
		if start < end {
			selection.selectedControl = node
		} else if selection.selectedControl == node {
			selection.selectedControl = nil
		}
	}
	r.queueSelectEvent(node)
}

func (r *Runtime) queueSelectEvent(node *dom.Node) {
	r.QueueTask(func() {
		r.dispatchWithWindowStep(node, dom.NewTrustedEvent("select", true, false))
	})
}

func (r *Runtime) resetTextSelectionAfterValueSet(node *dom.Node, value string) {
	if node == nil || !textSelectionApplies(node) {
		return
	}
	end := len(utf16.Encode([]rune(value)))
	r.setTextSelection(node, end, end, "none")
}

// SetTextControlSelection synchronizes an engine-owned user selection with
// the script-visible selection on an input or textarea.
func (r *Runtime) SetTextControlSelection(node *dom.Node, start, end int, direction string) {
	if r == nil || !textSelectionApplies(node) {
		return
	}
	r.setTextSelection(node, start, end, direction)
}

// TextControlSelection returns the script-visible UTF-16 selection for node.
// The engine uses it when a user action follows a script selection update.
func (r *Runtime) TextControlSelection(node *dom.Node) (start, end int, direction string, ok bool) {
	if r == nil || node == nil {
		return 0, 0, "", false
	}
	selection, ok := r.textControlSelections[node]
	if !ok {
		return 0, 0, "", false
	}
	return selection.start, selection.end, selection.direction, true
}

func (r *Runtime) clampTextControlSelection(node *dom.Node) {
	if r == nil || node == nil || r.textControlSelections == nil {
		return
	}
	if _, ok := r.textControlSelections[node]; ok && textSelectionApplies(node) {
		_ = r.textSelection(node)
	}
}

func (r *Runtime) clampTrackedTextControlSelections() {
	if r == nil {
		return
	}
	for node := range r.textControlSelections {
		r.clampTextControlSelection(node)
	}
}

func (r *Runtime) installTextControlSelection(proto *goja.Object, input bool) {
	get := func(call goja.FunctionCall) *dom.Node { return r.getNode(call.This) }
	for _, name := range []string{"selectionStart", "selectionEnd", "selectionDirection"} {
		name := name
		getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := get(call)
			if node == nil || (input && !textSelectionApplies(node)) {
				return goja.Null()
			}
			selection := r.textSelection(node)
			switch name {
			case "selectionStart":
				return r.vm.ToValue(selection.start)
			case "selectionEnd":
				return r.vm.ToValue(selection.end)
			default:
				return r.vm.ToValue(selection.direction)
			}
		})
		setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := get(call)
			if node == nil {
				return goja.Undefined()
			}
			if input && !textSelectionApplies(node) {
				panic(r.domError("InvalidStateError", "The input element does not support selection."))
			}
			selection := r.textSelection(node)
			switch name {
			case "selectionStart":
				start := toUnsignedLong(call.Argument(0))
				end := selection.end
				if end < start {
					end = start
				}
				r.setTextSelection(node, start, end, selection.direction)
			case "selectionEnd":
				r.setTextSelection(node, selection.start, toUnsignedLong(call.Argument(0)), selection.direction)
			default:
				r.setTextSelection(node, selection.start, selection.end, call.Argument(0).String())
			}
			return goja.Undefined()
		})
		_ = proto.DefineAccessorProperty(name, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}

	_ = proto.DefineDataProperty("setSelectionRange", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := get(call)
		if node == nil {
			return goja.Undefined()
		}
		if input && !textSelectionApplies(node) {
			panic(r.domError("InvalidStateError", "The input element does not support selection."))
		}
		start := toUnsignedLong(call.Argument(0))
		end := start
		if len(call.Arguments) > 1 {
			end = toUnsignedLong(call.Argument(1))
		}
		direction := "none"
		if len(call.Arguments) > 2 {
			direction = call.Argument(2).String()
		}
		r.setTextSelection(node, start, end, direction)
		return goja.Undefined()
	}), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = proto.DefineDataProperty("select", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := get(call)
		if node == nil || (input && !textSelectionApplies(node) && !strings.EqualFold(node.GetAttribute("type"), "email")) {
			return goja.Undefined()
		}
		if node.Doc != nil {
			r.selectionForDocument(node.Doc).selectedControl = node
		}
		if textSelectionApplies(node) {
			r.setTextSelection(node, 0, selectionLength(node), "none")
		}
		return goja.Undefined()
	}), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = proto.DefineDataProperty("setRangeText", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := get(call)
		if node == nil {
			return goja.Undefined()
		}
		if input && !textSelectionApplies(node) {
			panic(r.domError("InvalidStateError", "The input element does not support selection."))
		}
		value := selectionValue(node)
		units := utf16.Encode([]rune(value))
		selection := r.textSelection(node)
		start, end := selection.start, selection.end
		if len(call.Arguments) >= 2 {
			start, end = toUnsignedLong(call.Argument(1)), toUnsignedLong(call.Argument(2))
		}
		if start > end {
			panic(r.domError("IndexSizeError", "The start position is greater than the end position."))
		}
		if start > len(units) {
			start = len(units)
		}
		if end > len(units) {
			end = len(units)
		}
		replacement := call.Argument(0).String()
		inserted := utf16.Encode([]rune(replacement))
		oldStart, oldEnd := selection.start, selection.end
		updated := make([]uint16, 0, len(units)-(end-start)+len(inserted))
		updated = append(updated, units[:start]...)
		updated = append(updated, inserted...)
		updated = append(updated, units[end:]...)
		newEnd := start + len(inserted)
		mode := "preserve"
		if len(call.Arguments) > 3 {
			mode = call.Argument(3).String()
		}
		switch mode {
		case "select":
			oldStart, oldEnd = start, newEnd
		case "start":
			oldStart, oldEnd = start, start
		case "end":
			oldStart, oldEnd = newEnd, newEnd
		case "preserve":
			delta := len(inserted) - (end - start)
			if len(call.Arguments) == 1 {
				if oldStart > end {
					oldStart += delta
				} else if oldStart > start {
					oldStart = start
				}
				if oldEnd > end {
					oldEnd += delta
				} else if oldEnd > start {
					oldEnd = newEnd
				}
			} else {
				if oldStart > start {
					oldStart = start
				}
				if oldEnd > start {
					oldEnd = newEnd
				}
			}
		default:
			panic(r.vm.NewTypeError("The provided value is not a valid selection mode."))
		}
		node.SetInputControlValue(string(utf16.Decode(updated)), true)
		r.setTextSelection(node, oldStart, oldEnd, "none")
		r.notifyMutation()
		return goja.Undefined()
	}), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_TRUE)
}
