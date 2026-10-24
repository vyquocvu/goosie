package css

import (
	"sync"

	"github.com/vyquocvu/goosie/internal/dom"
)

// Interactive state tracking for :hover, :focus, and :focus-visible pseudo-classes.
// The engine updates these registries when the user interacts with elements,
// and the selector matcher queries them during style resolution.

var (
	// hoverState tracks which nodes are currently hovered.
	hoverState sync.Map // map[*dom.Node]bool

	// focusState tracks which node currently has focus.
	focusState sync.Map // map[*dom.Node]bool

	// focusVisibleState tracks which focused nodes should expose a keyboard
	// focus indicator according to the browser's input-modality heuristic.
	focusVisibleState sync.Map // map[*dom.Node]bool

	// activeState tracks the element currently being activated by pointer input.
	activeState sync.Map // map[*dom.Node]bool
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
		focusVisibleState.Delete(n)
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

// SetFocusVisible marks whether the focused node should match :focus-visible.
// Callers choose the value from the input modality and control type.
func SetFocusVisible(n *dom.Node, visible bool) {
	if n == nil {
		return
	}
	if visible {
		focusVisibleState.Store(n, true)
	} else {
		focusVisibleState.Delete(n)
	}
}

// IsFocusVisible reports whether a node is both focused and should display a
// keyboard-style focus indicator.
func IsFocusVisible(n *dom.Node) bool {
	if !IsFocused(n) {
		return false
	}
	v, ok := focusVisibleState.Load(n)
	return ok && v.(bool)
}

// SetActive marks a node as currently being activated by pointer input.
func SetActive(n *dom.Node, active bool) {
	if n == nil {
		return
	}
	if active {
		activeState.Store(n, true)
	} else {
		activeState.Delete(n)
	}
}

// IsActive reports whether a node is currently being activated by pointer input.
func IsActive(n *dom.Node) bool {
	if n == nil {
		return false
	}
	v, ok := activeState.Load(n)
	return ok && v.(bool)
}

// ClearInteractiveState removes all hover, focus, and focus-visible state.
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
	focusVisibleState.Range(func(key, value interface{}) bool {
		focusVisibleState.Delete(key)
		return true
	})
	activeState.Range(func(key, value interface{}) bool {
		activeState.Delete(key)
		return true
	})
}
