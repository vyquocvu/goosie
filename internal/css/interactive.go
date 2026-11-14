package css

import (
	"strings"
	"sync"

	"github.com/vyquocvu/goosie/internal/dom"
)

var (
	hoverState sync.Map

	focusState sync.Map

	focusVisibleState sync.Map

	activeState sync.Map
)

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

func IsHovered(n *dom.Node) bool {
	if n == nil {
		return false
	}
	v, ok := hoverState.Load(n)
	return ok && v.(bool)
}

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

func IsFocused(n *dom.Node) bool {
	if n == nil {
		return false
	}
	v, ok := focusState.Load(n)
	return ok && v.(bool)
}

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

func IsFocusVisible(n *dom.Node) bool {
	if !IsFocused(n) {
		return false
	}
	v, ok := focusVisibleState.Load(n)
	if ok && v.(bool) {
		return true
	}
	if n.Element() && n.Data == "input" {
		return !strings.EqualFold(strings.TrimSpace(n.GetAttribute("type")), "hidden")
	}
	return false
}

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

func IsActive(n *dom.Node) bool {
	if n == nil {
		return false
	}
	v, ok := activeState.Load(n)
	return ok && v.(bool)
}

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
