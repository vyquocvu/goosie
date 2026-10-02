package engine

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
)

// EditAction is one keyboard edit operation applied to the focused control.
type EditAction uint8

const (
	EditRune EditAction = iota
	EditBackspace
	EditLeft
	EditRight
	EditUp
	EditDown
	EditHome
	EditEnd
	EditEnter
	EditEscape
)

// controlEditable reports whether n is a control the engine can focus and edit.
func controlEditable(n *dom.Node) bool {
	return n != nil && n.Element() &&
		(n.DataAtom == dom.AtomInput || n.DataAtom == dom.AtomTextarea)
}

// controlAt walks the arena in reverse paint order for the box containing the
// point, then walks that box's ancestors for a control element.
func (s *Session) controlAt(x, y float32) *dom.Node {
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
		for cur := &objs[i]; cur != nil; cur = parentObj(objs, cur) {
			if controlEditable(cur.Node) {
				return cur.Node
			}
		}
	}
	return nil
}

// HasControlAt reports whether an editable control is laid out at the
// document-space point. Hover uses it to ask for the text cursor.
func (s *Session) HasControlAt(x, y float32) bool {
	return s.controlAt(x, y) != nil
}

// FocusControl focuses the control at the document point, blurring any current
// focus. Clicking empty space blurs; clicking the already-focused control
// changes nothing. Returns whether focus state changed. Any focus change ends
// composition: the marked text belonged to the control that had focus.
// Fires "blur" and "focus" events on the affected controls, and a "change"
// event on the blurred control if its value was modified during focus.
func (s *Session) FocusControl(x, y float32) bool {
	n := s.controlAt(x, y)
	if n == s.focus {
		return false
	}
	prev := s.focus
	// Fire blur and change on the old control.
	if prev != nil {
		css.SetFocus(prev, false)
		dom.DispatchEvent(prev, dom.NewEvent("blur", false, false))
		if s.controlValue(prev) != s.focusValue {
			dom.DispatchEvent(prev, dom.NewEvent("change", true, true))
		}
	}
	s.focus = n
	s.marked = ""
	if n != nil {
		s.caret = len([]rune(s.controlValue(n)))
		s.focusValue = s.controlValue(n)
		css.SetFocus(n, true)
		dom.DispatchEvent(n, dom.NewEvent("focus", false, false))
	} else {
		s.focusValue = ""
	}
	return true
}

// Focused returns the focused control node, or nil.
func (s *Session) Focused() *dom.Node { return s.focus }

// SetMarked replaces the IME composition preview shown at the focused
// control's caret. The preview is painted but is not the value until
// CommitText runs; an empty string ends composition. Returns whether the
// preview changed, which is when the caller must repaint.
func (s *Session) SetMarked(text string) bool {
	if !controlEditable(s.focus) || s.marked == text {
		return false
	}
	s.marked = text
	return true
}

// Marked returns the current composition text, empty when not composing.
func (s *Session) Marked() string { return s.marked }

// CommitText ends composition by inserting text at the focused control's
// caret, respecting maxlength like any other insert. Returns whether the
// value changed, which is when the caller must reflow.
func (s *Session) CommitText(text string) bool {
	if !controlEditable(s.focus) {
		return false
	}
	s.marked = ""
	runes := []rune(s.controlValue(s.focus))
	if s.caret > len(runes) {
		s.caret = len(runes)
	}
	added := []rune(text)
	if max := maxLen(s.focus); max > 0 && len(runes)+len(added) > max {
		return false
	}
	out := make([]rune, 0, len(runes)+len(added))
	out = append(out, runes[:s.caret]...)
	out = append(out, added...)
	out = append(out, runes[s.caret:]...)
	s.caret += len(added)
	s.setControlValue(s.focus, string(out))
	dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
	return true
}

// Edit applies one keyboard edit to the focused control. It returns whether
// the value or focus changed, which is when the caller must reflow;
// caret-only movement returns false but still needs a repaint.
func (s *Session) Edit(action EditAction, r rune) bool {
	if !controlEditable(s.focus) {
		return false
	}
	runes := []rune(s.controlValue(s.focus))
	if s.caret > len(runes) {
		s.caret = len(runes)
	}
	insert := func(text string) bool {
		added := []rune(text)
		if max := maxLen(s.focus); max > 0 && len(runes)+len(added) > max {
			return false
		}
		out := make([]rune, 0, len(runes)+len(added))
		out = append(out, runes[:s.caret]...)
		out = append(out, added...)
		out = append(out, runes[s.caret:]...)
		s.caret += len(added)
		s.setControlValue(s.focus, string(out))
		return true
	}
	switch action {
	case EditRune:
		if r < 0x20 {
			return false
		}
		if insert(string(r)) {
			dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
			return true
		}
		return false
	case EditEnter:
		if s.focus.DataAtom != dom.AtomTextarea {
			return false
		}
		if insert("\n") {
			dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
			return true
		}
		return false
	case EditBackspace:
		if s.caret == 0 || len(runes) == 0 {
			return false
		}
		out := append([]rune{}, runes[:s.caret-1]...)
		out = append(out, runes[s.caret:]...)
		s.caret--
		s.setControlValue(s.focus, string(out))
		dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
		return true
	case EditLeft:
		if s.caret > 0 {
			s.caret--
		}
		return false
	case EditRight:
		if s.caret < len(runes) {
			s.caret++
		}
		return false
	case EditHome:
		s.caret = 0
		return false
	case EditEnd:
		s.caret = len(runes)
		return false
	case EditEscape:
		s.focus = nil
		s.marked = ""
		return true
	}
	return false
}

// maxLen reads the maxlength attribute; 0 means unlimited.
func maxLen(n *dom.Node) int {
	if v := n.GetAttribute("maxlength"); v != "" {
		if m, err := strconv.Atoi(v); err == nil && m > 0 {
			return m
		}
	}
	return 0
}

// controlValue reads the editable value: the value attribute for input, the
// first text child for textarea.
func (s *Session) controlValue(n *dom.Node) string {
	if n.DataAtom == dom.AtomTextarea {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Text() {
				return c.DataContent
			}
		}
		return ""
	}
	return n.GetAttribute("value")
}

func (s *Session) setControlValue(n *dom.Node, v string) {
	if n.DataAtom == dom.AtomTextarea {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Text() {
				c.DataContent = v
				return
			}
		}
		n.AppendChild(&dom.Node{Type: dom.NodeText, DataContent: v})
		return
	}
	n.SetAttribute("value", v)
}

// IsChecked reports whether a checkbox or radio input is checked.
// The checked state is tracked via the "checked" boolean attribute:
// presence means checked, absence means unchecked.
func IsChecked(n *dom.Node) bool {
	if n == nil {
		return false
	}
	return n.HasAttribute("checked")
}

// SetChecked sets or removes the checked attribute on a node.
func SetChecked(n *dom.Node, checked bool) {
	if n == nil {
		return
	}
	if checked {
		n.SetAttribute("checked", "")
	} else {
		n.RemoveAttribute("checked")
	}
}

// IsActivatable reports whether a node is a clickable control
// (button, submit, reset, etc.).
func IsActivatable(n *dom.Node) bool {
	if n == nil || !n.Element() {
		return false
	}
	switch n.Data {
	case "button":
		return true
	case "input":
		typ := strings.ToLower(n.GetAttribute("type"))
		switch typ {
		case "submit", "reset", "button", "image":
			return true
		}
	}
	return false
}

// SelectedOption returns the currently selected <option> node within a <select>.
// If no option has the "selected" attribute, the first <option> child is returned.
// Returns nil if selectNode is nil or has no <option> children.
func SelectedOption(selectNode *dom.Node) *dom.Node {
	if selectNode == nil {
		return nil
	}
	var first *dom.Node
	for c := selectNode.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() && c.Data == "option" {
			if first == nil {
				first = c
			}
			if c.HasAttribute("selected") {
				return c
			}
		}
	}
	return first
}

// ToggleControl toggles the checked state of a checkbox or radio input.
// For radio buttons, it also unchecks other radios in the same group
// (same parent form or document). Returns true if the control was toggled.
func (s *Session) ToggleControl(n *dom.Node) bool {
	if n == nil || !n.Element() {
		return false
	}
	typ := strings.ToLower(n.GetAttribute("type"))
	switch typ {
	case "checkbox":
		SetChecked(n, !IsChecked(n))
		dom.DispatchEvent(n, dom.NewEvent("change", true, true))
		dom.DispatchEvent(n, dom.NewEvent("input", true, true))
		return true
	case "radio":
		// Uncheck all radios in the same group (same name, same form).
		name := n.GetAttribute("name")
		// Walk up to find the form ancestor.
		form := n.Parent
		for form != nil && form.Data != "form" {
			form = form.Parent
		}
		root := &s.Doc.Node
		if form != nil {
			root = form
		}
		// Uncheck siblings with the same name.
		uncheckRadioGroup(root, name, n)
		n.SetAttribute("checked", "")
		dom.DispatchEvent(n, dom.NewEvent("change", true, true))
		dom.DispatchEvent(n, dom.NewEvent("input", true, true))
		return true
	}
	return false
}

// uncheckRadioGroup walks the subtree rooted at root and removes the checked
// attribute from every radio input with the given name, except for keep.
func uncheckRadioGroup(root *dom.Node, name string, keep *dom.Node) {
	if root == nil {
		return
	}
	if root.Element() && root.Data == "input" &&
		strings.ToLower(root.GetAttribute("type")) == "radio" &&
		root.GetAttribute("name") == name && root != keep {
		root.RemoveAttribute("checked")
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		uncheckRadioGroup(c, name, keep)
	}
}

// objectFor returns the arena object laid out for n, or nil when the node has
// no box (e.g. display:none) or the arena has not been built.
func (s *Session) objectFor(n *dom.Node) *layout.Object {
	if s.Arena == nil || n == nil {
		return nil
	}
	for i := range s.Arena.Objects {
		if s.Arena.Objects[i].Node == n {
			return &s.Arena.Objects[i]
		}
	}
	return nil
}

// FocusNext cycles focus to the next tabbable control in the document.
// Tabbable controls: input (not hidden), textarea, select, button, a[href].
// Wraps around to the first control when at the end.
func (s *Session) FocusNext() {
	if s.Doc == nil {
		return
	}
	controls := tabbableControls(s.Doc)
	if len(controls) == 0 {
		return
	}
	idx := -1
	for i, c := range controls {
		if c == s.focus {
			idx = i
			break
		}
	}
	next := controls[(idx+1)%len(controls)]
	// Fire blur/change on old focus.
	if s.focus != nil {
		dom.DispatchEvent(s.focus, dom.NewEvent("blur", false, false))
		if s.controlValue(s.focus) != s.focusValue {
			dom.DispatchEvent(s.focus, dom.NewEvent("change", true, true))
		}
	}
	s.focus = next
	s.marked = ""
	s.caret = len([]rune(s.controlValue(next)))
	s.focusValue = s.controlValue(next)
	dom.DispatchEvent(next, dom.NewEvent("focus", false, false))
}

// FocusPrev cycles focus to the previous tabbable control in the document.
// Wraps around to the last control when at the beginning.
func (s *Session) FocusPrev() {
	if s.Doc == nil {
		return
	}
	controls := tabbableControls(s.Doc)
	if len(controls) == 0 {
		return
	}
	idx := 0
	for i, c := range controls {
		if c == s.focus {
			idx = i
			break
		}
	}
	prev := controls[(idx-1+len(controls))%len(controls)]
	// Fire blur/change on old focus.
	if s.focus != nil {
		dom.DispatchEvent(s.focus, dom.NewEvent("blur", false, false))
		if s.controlValue(s.focus) != s.focusValue {
			dom.DispatchEvent(s.focus, dom.NewEvent("change", true, true))
		}
	}
	s.focus = prev
	s.marked = ""
	s.caret = len([]rune(s.controlValue(prev)))
	s.focusValue = s.controlValue(prev)
	dom.DispatchEvent(prev, dom.NewEvent("focus", false, false))
}

// TabbableControls returns all focusable controls in tree order.
// Tabbable elements: input (not hidden, not disabled), textarea, select,
// button, and a[href].
func TabbableControls(doc *dom.Document) []*dom.Node {
	var result []*dom.Node
	walkForTabbable(&doc.Node, &result)
	return result
}

func tabbableControls(doc *dom.Document) []*dom.Node {
	return TabbableControls(doc)
}

func walkForTabbable(n *dom.Node, result *[]*dom.Node) {
	if n.Element() && isTabbable(n) {
		*result = append(*result, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkForTabbable(c, result)
	}
}

func isTabbable(n *dom.Node) bool {
	if !n.Element() {
		return false
	}
	if n.HasAttribute("disabled") {
		return false
	}
	switch n.Data {
	case "input":
		typ := strings.ToLower(n.GetAttribute("type"))
		return typ != "hidden"
	case "textarea", "select", "button":
		return true
	case "a":
		return n.HasAttribute("href")
	}
	return false
}

// SetHover updates the hover state for the element at the given document point.
// It unhoversthe previously hovered element (if any) and hovers the new one.
// Returns whether the hover state changed.
func (s *Session) SetHover(x, y float32) bool {
	if s.Arena == nil {
		return false
	}

	// Find the topmost element at the point.
	var hovered *dom.Node
	objs := s.Arena.Objects
	for i := len(objs) - 1; i >= 1; i-- {
		obj := &objs[i]
		x0, y0, x1, y1 := obj.BorderRect()
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			continue
		}
		if obj.Node != nil && obj.Node.Element() {
			hovered = obj.Node
			break
		}
	}

	if hovered == s.hovered {
		return false
	}

	// Unhover the previous element.
	if s.hovered != nil {
		css.SetHover(s.hovered, false)
	}

	s.hovered = hovered

	// Hover the new element.
	if hovered != nil {
		css.SetHover(hovered, true)
	}

	return true
}
