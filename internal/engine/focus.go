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

// clickFocusable reports whether a click on n should give it focus. It is wider
// than controlEditable: a <select> has no text to edit, but Tab and
// element.focus() already reach it, and refusing the click would leave the
// keyboard selection path unreachable.
func clickFocusable(n *dom.Node) bool {
	if n == nil || !n.Element() || n.HasAttribute("disabled") {
		return false
	}
	if controlEditable(n) {
		return true
	}
	return n.DataAtom == dom.AtomSelect
}

// controlAtMatching walks the arena in reverse paint order for the box
// containing the point, then walks that box's ancestors for an element the
// caller's predicate admits.
func (s *Session) controlAtMatching(x, y float32, admit func(*dom.Node) bool) *dom.Node {
	if s.Arena == nil {
		return nil
	}
	objs := s.Arena.Objects
	for i := len(objs) - 1; i >= 1; i-- {
		obj := &objs[i]
		if obj.Style != nil && obj.Style.PointerEvents == "none" {
			continue
		}
		x0, y0, x1, y1 := obj.BorderRect()
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			continue
		}
		for cur := &objs[i]; cur != nil; cur = parentObj(objs, cur) {
			if cur.Style != nil && cur.Style.PointerEvents == "none" {
				break
			}
			if admit(cur.Node) {
				return cur.Node
			}
		}
	}
	return nil
}

// controlAt finds the editable control at a point. Hover uses it to decide
// where the text cursor belongs, so it stays narrower than click-to-focus.
func (s *Session) controlAt(x, y float32) *dom.Node {
	return s.controlAtMatching(x, y, controlEditable)
}

// HasControlAt reports whether an editable control is laid out at the
// document-space point. Hover uses it to ask for the text cursor.
func (s *Session) HasControlAt(x, y float32) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.controlAt(x, y) != nil
}

// FocusControl focuses the control at the document point, blurring any current
// focus. Clicking empty space blurs; clicking the already-focused control
// changes nothing. Returns whether focus state changed. Any focus change ends
// composition: the marked text belonged to the control that had focus.
// Fires "blur" and "focus" events on the affected controls, and a "change"
// event on the blurred control if its value was modified during focus.
func (s *Session) FocusControl(x, y float32) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	n := s.controlAtMatching(x, y, clickFocusable)
	return s.setFocusedNodeLocked(n)
}

// FocusNode handles an element.focus() call from the document's JavaScript
// runtime. Programmatic focus works for form controls and elements with a
// valid tabindex, even when they are not editable controls.
func (s *Session) FocusNode(n *dom.Node) bool {
	if !programmaticallyFocusable(n) {
		return false
	}
	s.opMu.Lock()
	changed := s.setFocusedNodeLocked(n)
	if changed && s.Styles != nil {
		s.focusStylePending = true
	}
	s.opMu.Unlock()
	return changed
}

// BlurNode handles HTMLElement.blur() for the currently focused element.
func (s *Session) BlurNode(n *dom.Node) bool {
	s.opMu.Lock()
	if n == nil || s.focus != n {
		s.opMu.Unlock()
		return false
	}
	changed := s.setFocusedNodeLocked(nil)
	if changed && s.Styles != nil {
		s.focusStylePending = true
	}
	s.opMu.Unlock()
	return changed
}

func (s *Session) setFocusedNodeLocked(n *dom.Node) bool {
	if n == s.focus {
		return false
	}
	prev := s.focus
	// Fire blur and change on the old control.
	if prev != nil {
		css.SetFocus(prev, false)
		css.SetFocusVisible(prev, false)
		dom.DispatchEvent(prev, dom.NewEvent("blur", false, false))
		if controlEditable(prev) && s.controlValue(prev) != s.focusValue {
			dom.DispatchEvent(prev, dom.NewEvent("change", true, true))
		}
	}
	s.focus = n
	s.focusState.Store(n)
	s.marked = ""
	if controlEditable(n) {
		s.caret = len([]rune(s.controlValue(n)))
		s.focusValue = s.controlValue(n)
		css.SetFocus(n, true)
		css.SetFocusVisible(n, focusVisibleByDefault(n))
		dom.DispatchEvent(n, dom.NewEvent("focus", false, false))
	} else {
		s.caret = 0
		s.focusValue = ""
		if n != nil {
			css.SetFocus(n, true)
			css.SetFocusVisible(n, focusVisibleByDefault(n))
			dom.DispatchEvent(n, dom.NewEvent("focus", false, false))
		}
	}
	return true
}

// focusVisibleByDefault follows the browser heuristic for pointer or
// programmatic focus: controls that accept text input keep a visible indicator,
// while buttons and other non-text controls show one after keyboard navigation.
func focusVisibleByDefault(n *dom.Node) bool {
	if n == nil || !n.Element() {
		return false
	}
	if n.DataAtom == dom.AtomTextarea {
		return true
	}
	if n.DataAtom == dom.AtomInput {
		switch strings.ToLower(strings.TrimSpace(n.GetAttribute("type"))) {
		case "button", "checkbox", "color", "file", "hidden", "image", "radio", "range", "reset", "submit":
			return false
		default:
			return true
		}
	}
	if n.HasAttribute("contenteditable") {
		switch strings.ToLower(strings.TrimSpace(n.GetAttribute("contenteditable"))) {
		case "", "true", "plaintext-only":
			return true
		}
	}
	return false
}

func programmaticallyFocusable(n *dom.Node) bool {
	if n == nil || !n.Element() || n.HasAttribute("disabled") {
		return false
	}
	if n.HasAttribute("contenteditable") {
		switch strings.ToLower(strings.TrimSpace(n.GetAttribute("contenteditable"))) {
		case "", "true", "plaintext-only":
			return true
		}
	}
	if n.HasAttribute("tabindex") {
		_, err := strconv.Atoi(strings.TrimSpace(n.GetAttribute("tabindex")))
		return err == nil
	}
	switch n.Data {
	case "input":
		return strings.ToLower(n.GetAttribute("type")) != "hidden"
	case "textarea", "select", "button":
		return true
	case "a", "area":
		return n.HasAttribute("href")
	case "iframe", "object":
		return true
	default:
		return false
	}
}

// clearDetachedFocusLocked drops focus when its node is no longer connected to
// this document. DOM removal can happen inside a script before the next style
// pass; retaining that node would leave :focus and :focus-within stale.
func (s *Session) clearDetachedFocusLocked() bool {
	if s.focus == nil || s.Doc == nil {
		return false
	}
	for current := s.focus; current != nil; current = current.Parent {
		if current == &s.Doc.Node {
			return false
		}
	}
	return s.setFocusedNodeLocked(nil)
}

// Focused returns the focused control node, or nil.
func (s *Session) Focused() *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.focus
}

// SetMarked replaces the IME composition preview shown at the focused
// control's caret. The preview is painted but is not the value until
// CommitText runs; an empty string ends composition. Returns whether the
// preview changed, which is when the caller must repaint.
func (s *Session) SetMarked(text string) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if !controlEditable(s.focus) || s.marked == text {
		return false
	}
	s.marked = text
	return true
}

// Marked returns the current composition text, empty when not composing.
func (s *Session) Marked() string {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.marked
}

// CommitText ends composition by inserting text at the focused control's
// caret, respecting maxlength like any other insert. Returns whether the
// value changed, which is when the caller must reflow.
func (s *Session) CommitText(text string) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
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

// CloseTopmostModalDialog finds and closes the topmost open modal dialog element.
// It fires a cancelable "cancel" event; if not canceled, it removes the open attribute,
// clears the modal flag, and fires a "close" event.
func (s *Session) CloseTopmostModalDialog() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.closeTopmostModalDialogLocked()
}

func (s *Session) closeTopmostModalDialogLocked() bool {
	if s.Doc == nil {
		return false
	}
	var topmost *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() && n.Data == "dialog" && n.IsModal && n.HasAttribute("open") {
			topmost = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&s.Doc.Node)
	if topmost == nil {
		return false
	}
	cancelEv := dom.NewEvent("cancel", false, true)
	if !dom.DispatchEvent(topmost, cancelEv) {
		return false // preventDefault was called
	}
	topmost.RemoveAttribute("open")
	topmost.IsModal = false
	dom.DispatchEvent(topmost, dom.NewEvent("close", false, false))
	return true
}

// Edit applies one keyboard edit to the focused control. It returns whether
// the value or focus changed, which is when the caller must reflow;
// caret-only movement returns false but still needs a repaint.
func (s *Session) Edit(action EditAction, r rune) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if action == EditEscape {
		if s.closeTopmostAutoPopoverLocked() {
			return true
		}
		closedDialog := s.closeTopmostModalDialogLocked()
		if s.focus != nil {
			prev := s.focus
			s.focus = nil
			s.focusState.Store(nil)
			s.marked = ""
			css.SetFocus(prev, false)
			css.SetFocusVisible(prev, false)
			dom.DispatchEvent(prev, dom.NewEvent("blur", false, false))
			return true
		}
		return closedDialog
	}
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
		if s.focus.DataAtom == dom.AtomTextarea {
			if insert("\n") {
				dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
				return true
			}
			return false
		}
		if s.focus.DataAtom == dom.AtomInput {
			if form := FindForm(s.focus); form != nil {
				s.submitFormLocked(form, FirstSubmitButton(form))
				return true
			}
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
		s.focusState.Store(nil)
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

// InsertText inserts text into the currently focused editable control at the caret
// position, respecting maxlength and firing an "input" event.
func (s *Session) InsertText(text string) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if !controlEditable(s.focus) || text == "" {
		return false
	}
	runes := []rune(s.controlValue(s.focus))
	if s.caret > len(runes) {
		s.caret = len(runes)
	}
	added := []rune(text)
	if max := maxLen(s.focus); max > 0 {
		avail := max - len(runes)
		if avail <= 0 {
			return false
		}
		if len(added) > avail {
			added = added[:avail]
		}
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

// FocusedText returns the current value of the focused control, or empty if none.
func (s *Session) FocusedText() string {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.focus == nil {
		return ""
	}
	return s.controlValue(s.focus)
}

// FocusedEditable reports whether the currently focused control is editable.
func (s *Session) FocusedEditable() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return controlEditable(s.focus)
}

// SetFocusedText replaces the value of the focused editable control. It is the
// programmatic equivalent of selecting all and typing, used by accessibility
// clients that set a text field's AXValue. Like ordinary typing it respects
// maxlength, dispatches input only when the value changes, and leaves focus in
// the control with the caret at the end.
func (s *Session) SetFocusedText(value string) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if !controlEditable(s.focus) {
		return false
	}
	runes := []rune(value)
	if max := maxLen(s.focus); max > 0 && len(runes) > max {
		return false
	}
	old := s.controlValue(s.focus)
	s.marked = ""
	s.caret = len(runes)
	if old == value {
		return false
	}
	s.setControlValue(s.focus, value)
	dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
	return true
}

// SelectAllControl positions the caret at the end of the focused control.
func (s *Session) SelectAllControl() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if !controlEditable(s.focus) {
		return false
	}
	runes := []rune(s.controlValue(s.focus))
	s.caret = len(runes)
	return true
}

// controlValue reads the current form value for a control.
func (s *Session) controlValue(n *dom.Node) string {
	if n.DataAtom == dom.AtomSelect {
		option := SelectedOption(n)
		if option == nil {
			return ""
		}
		if option.HasAttribute("value") {
			return option.GetAttribute("value")
		}
		return option.TextContent()
	}
	if n.DataAtom == dom.AtomOption {
		if n.HasAttribute("value") {
			return n.GetAttribute("value")
		}
		return n.TextContent()
	}
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
		if !n.HasAttribute("data-default-value") {
			n.SetAttribute("data-default-value", s.controlValue(n))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Text() {
				c.DataContent = v
				return
			}
		}
		n.AppendChild(&dom.Node{Type: dom.NodeText, DataContent: v})
		return
	}
	if !n.HasAttribute("data-default-value") {
		n.SetAttribute("data-default-value", n.GetAttribute("value"))
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
	case "button", "summary":
		return true
	case "a":
		return n.GetAttribute("href") != ""
	case "input":
		typ := strings.ToLower(n.GetAttribute("type"))
		switch typ {
		case "submit", "reset", "button", "image", "checkbox", "radio":
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
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if n == nil || !n.Element() {
		return false
	}
	typ := strings.ToLower(n.GetAttribute("type"))
	switch typ {
	case "checkbox":
		if !n.HasAttribute("data-default-checked") {
			if IsChecked(n) {
				n.SetAttribute("data-default-checked", "true")
			} else {
				n.SetAttribute("data-default-checked", "false")
			}
		}
		SetChecked(n, !IsChecked(n))
		dom.DispatchEvent(n, dom.NewEvent("change", true, true))
		dom.DispatchEvent(n, dom.NewEvent("input", true, true))
		return true
	case "radio":
		if !n.HasAttribute("data-default-checked") {
			if IsChecked(n) {
				n.SetAttribute("data-default-checked", "true")
			} else {
				n.SetAttribute("data-default-checked", "false")
			}
		}
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
	s.opMu.Lock()
	defer s.opMu.Unlock()
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
		css.SetFocus(s.focus, false)
		css.SetFocusVisible(s.focus, false)
		dom.DispatchEvent(s.focus, dom.NewEvent("blur", false, false))
		if s.controlValue(s.focus) != s.focusValue {
			dom.DispatchEvent(s.focus, dom.NewEvent("change", true, true))
		}
	}
	s.focus = next
	s.focusState.Store(next)
	s.marked = ""
	s.caret = len([]rune(s.controlValue(next)))
	s.focusValue = s.controlValue(next)
	css.SetFocus(next, true)
	css.SetFocusVisible(next, true)
	dom.DispatchEvent(next, dom.NewEvent("focus", false, false))
}

// FocusPrev cycles focus to the previous tabbable control in the document.
// Wraps around to the last control when at the beginning.
func (s *Session) FocusPrev() {
	s.opMu.Lock()
	defer s.opMu.Unlock()
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
		css.SetFocus(s.focus, false)
		css.SetFocusVisible(s.focus, false)
		dom.DispatchEvent(s.focus, dom.NewEvent("blur", false, false))
		if s.controlValue(s.focus) != s.focusValue {
			dom.DispatchEvent(s.focus, dom.NewEvent("change", true, true))
		}
	}
	s.focus = prev
	s.focusState.Store(prev)
	s.marked = ""
	s.caret = len([]rune(s.controlValue(prev)))
	s.focusValue = s.controlValue(prev)
	css.SetFocus(prev, true)
	css.SetFocusVisible(prev, true)
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
	s.opMu.Lock()
	defer s.opMu.Unlock()
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

	prev := s.hovered

	// Unhover the previous element and fire mouseleave.
	if prev != nil {
		for current := prev; current != nil && current.Element(); current = current.Parent {
			css.SetHover(current, false)
		}
		dom.DispatchEvent(prev, dom.NewEvent("mouseleave", false, false))
	}

	s.hovered = hovered

	// Hover the new element and fire mouseenter.
	if hovered != nil {
		for current := hovered; current != nil && current.Element(); current = current.Parent {
			css.SetHover(current, true)
		}
		dom.DispatchEvent(hovered, dom.NewEvent("mouseenter", false, false))
	}

	return true
}

// SetActiveAt updates the element currently being activated by the primary
// pointer. Active state follows the hit target while the button is held and is
// cleared on release; ancestors match as they do for :hover.
func (s *Session) SetActiveAt(x, y float32, pressed bool) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	var active *dom.Node
	if pressed && s.Arena != nil {
		objs := s.Arena.Objects
		for i := len(objs) - 1; i >= 1; i-- {
			obj := &objs[i]
			if obj.Style != nil && obj.Style.PointerEvents == "none" {
				continue
			}
			x0, y0, x1, y1 := obj.BorderRect()
			if x < x0 || x >= x1 || y < y0 || y >= y1 || obj.Node == nil || !obj.Node.Element() {
				continue
			}
			if obj.Node.HasAttribute("disabled") {
				break
			}
			active = obj.Node
			break
		}
	}
	if active == s.active {
		return false
	}
	for n := s.active; n != nil && n.Element(); n = n.Parent {
		css.SetActive(n, false)
	}
	s.active = active
	for n := active; n != nil && n.Element(); n = n.Parent {
		css.SetActive(n, true)
	}
	return true
}
