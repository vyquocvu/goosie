package css

import (
	"sync"

	"github.com/vyquocvu/goosie/internal/dom"
)

// Interactive state tracking for :hover and :focus pseudo-classes.
// The engine updates these registries when the user interacts with elements,
// and the selector matcher queries them during style resolution.

var (
	// hoverState tracks which nodes are currently hovered.
	hoverState sync.Map // map[*dom.Node]bool

	// focusState tracks which node currently has focus.
	focusState sync.Map // map[*dom.Node]bool
)

// SetHover marks a node as hovered or unhovered.
func SetHover(n *dom.Node, hovered bool) {
	if n == nil {
		return
	}
	if hovered {
		hoverState.Store(n, true)
	} else {
		hoverState.Delete(n)
	}
}

// IsHovered reports whether a node is currently hovered.
func IsHovered(n *dom.Node) bool {
	if n == nil {
		return false
	}
	v, ok := hoverState.Load(n)
	return ok && v.(bool)
}

// SetFocus marks a node as focused or unfocused.
func SetFocus(n *dom.Node, focused bool) {
	if n == nil {
		return
	}
	if focused {
		focusState.Store(n, true)
	} else {
		focusState.Delete(n)
	}
}

// IsFocused reports whether a node currently has focus.
func IsFocused(n *dom.Node) bool {
	if n == nil {
		return false
	}
	v, ok := focusState.Load(n)
	return ok && v.(bool)
}

// ClearInteractiveState removes all hover and focus state.
// Called when the document is unloaded or reset.
func ClearInteractiveState() {
	hoverState.Range(func(key, value interface{}) bool {
		hoverState.Delete(key)
		return true
	})
	focusState.Range(func(key, value interface{}) bool {
		focusState.Delete(key)
		return true
	})
}
