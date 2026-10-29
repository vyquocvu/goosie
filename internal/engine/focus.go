package engine

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
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
	EditDelete
	EditPageUp
	EditPageDown
)

// controlEditable reports whether n is a control the engine can focus and edit.
func controlEditable(n *dom.Node) bool {
	return n != nil && n.Element() &&
		(n.DataAtom == dom.AtomInput || n.DataAtom == dom.AtomTextarea)
}

func textSelectableControl(n *dom.Node) bool {
	if n == nil {
		return false
	}
	if n.DataAtom == dom.AtomTextarea {
		return true
	}
	if n.DataAtom != dom.AtomInput {
		return false
	}
	switch strings.ToLower(n.GetAttribute("type")) {
	case "email", "date", "month", "week", "time", "datetime-local", "number", "range", "color", "checkbox", "radio", "file", "hidden", "submit", "image", "reset", "button":
		return false
	default:
		return true
	}
}

// clickFocusable reports whether a click on n should give it focus. It is wider
// than controlEditable: a <select> has no text to edit, but Tab and
// element.focus() already reach it, and refusing the click would leave the
// keyboard selection path unreachable.
func clickFocusable(n *dom.Node) bool {
	if n == nil || !n.Element() || dom.IsFormControlDisabled(n) {
		return false
	}
	if dom.IsContentEditable(n) {
		return true
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
// focus. Clicking empty space blurs; clicking a text control can also move its
// caret without changing focus. Returns whether focus or caret state changed.
// Any focus change ends composition: the marked text belonged to the control
// that had focus.
// Fires "blur" and "focus" events on the affected controls, and a "change"
// event on the blurred control if its value was modified during focus.
func (s *Session) FocusControl(x, y float32) bool {
	s.opMu.Lock()
	n := s.controlAtMatching(x, y, clickFocusable)
	if modal := topmostModalDialog(s.Doc); modal != nil && !nodeWithin(n, modal) {
		s.opMu.Unlock()
		return false
	}
	s.opMu.Unlock()
	changed := s.changeFocusedNode(n)
	s.opMu.Lock()
	caretChanged := false
	if s.focus == n {
		caretChanged = s.placeTextControlCaretLocked(n, x)
		if changed && !caretChanged && textSelectableControl(n) {
			s.syncControlSelectionLocked()
		}
	}
	s.opMu.Unlock()
	return changed || caretChanged
}

// placeTextControlCaretLocked maps a pointer's x coordinate to the nearest
// rune boundary in a single-line text input. The input paint path uses the same
// glyph advances from the content-box origin. Textarea line wrapping and
// horizontal scrolling need separate geometry and are intentionally not
// approximated here.
func (s *Session) placeTextControlCaretLocked(n *dom.Node, x float32) bool {
	if n == nil || n.DataAtom != dom.AtomInput || !textSelectableControl(n) {
		return false
	}
	obj := s.objectFor(n)
	if obj == nil {
		return false
	}
	x0, _, _, _ := obj.ContentRect()
	runes := []rune(s.controlValue(n))
	fontSize := int32(16)
	var slot frame.FontSlot
	if obj.Style != nil {
		fontSize = int32(obj.Style.FontSize)
		slot = obj.Style.FontSlot()
	}
	if fontSize <= 0 {
		fontSize = 16
	}
	var pen int32
	caret := 0
	if x > x0 {
		rel := x - x0
		for i, r := range runes {
			advance := fontSize / 2 * 64
			if s.metrics != nil {
				advance = s.metrics.GlyphAdvanceFixed(fontSize, r, slot)
			}
			pen += advance
			if rel < float32((pen+32)>>6) {
				// Split each glyph at its midpoint, matching ordinary text
				// hit testing instead of always placing before the glyph.
				prev := pen - advance
				if rel >= float32((prev+advance/2+32)>>6) {
					caret = i + 1
				} else {
					caret = i
				}
				goto placed
			}
			caret = i + 1
		}
	}
placed:
	wasSelected := s.controlSelectionActive
	changed := caret != s.caret || wasSelected
	s.caret = caret
	s.controlSelectionActive = false
	s.controlSelectionAnchor = caret
	s.syncControlSelectionLocked()
	return changed
}

// FocusNode handles an element.focus() call from the document's JavaScript
// runtime. Programmatic focus works for form controls and elements with a
// valid tabindex, even when they are not editable controls.
func (s *Session) FocusNode(n *dom.Node) bool {
	return s.focusNode(n, true)
}

// RestoreFocusNode performs the focus restoration step used when a dialog
// closes. Restoring focus must not scroll the previously focused node.
func (s *Session) RestoreFocusNode(n *dom.Node) bool {
	return s.focusNode(n, false)
}

func (s *Session) focusNode(n *dom.Node, scroll bool) bool {
	focusTarget := n
	if n != nil && n.ShadowRoot != nil && n.ShadowRoot.DelegatesFocus {
		if delegate := shadowFocusDelegate(n.ShadowRoot); delegate != nil {
			focusTarget = delegate
		}
	}
	if !programmaticallyFocusable(focusTarget) && !(focusTarget != nil && s.Doc != nil && focusTarget == s.Doc.Body) && !(focusTarget != nil && focusTarget.Element() && focusTarget.Data == "dialog" && focusTarget.HasAttribute("open")) {
		return false
	}
	if modal := topmostModalDialog(s.Doc); modal != nil && !nodeWithin(focusTarget, modal) && !(focusTarget != nil && s.Doc != nil && focusTarget == s.Doc.Body && modal.HasAttribute("inert")) {
		return false
	}
	if scroll {
		// Script can append and focus a control in the same task. Flush pending
		// layout before computing the focus-induced scroll adjustment.
		s.refreshLayoutForCSSOMRead()
	}
	changed := s.changeFocusedNode(focusTarget)
	s.opMu.Lock()
	rt := s.jsRT
	if changed && s.Styles != nil {
		s.focusStylePending = true
	}
	if changed && controlEditable(focusTarget) {
		s.syncControlSelectionLocked()
	}
	if changed && scroll {
		s.scrollFocusedNodeIntoViewLocked(focusTarget)
	}
	s.opMu.Unlock()
	if changed && rt != nil && dom.IsContentEditable(focusTarget) {
		rt.EnsureContentEditableSelection(focusTarget)
	}
	return changed
}

// fixupDisabledFocus runs during a rendering update after DOM mutations. A
// control that became disabled through a fieldset must stop owning focus.
func (s *Session) fixupDisabledFocus() {
	s.opMu.Lock()
	focused := s.focus
	disabled := focused != nil && dom.IsFormControlDisabled(focused)
	s.opMu.Unlock()
	if disabled {
		s.BlurNode(focused)
	}
}

// BlurNode handles HTMLElement.blur() for the currently focused element.
func (s *Session) BlurNode(n *dom.Node) bool {
	s.opMu.Lock()
	focused := s.focus
	if n == nil || focused != n && !shadowTreeContains(n.ShadowRoot, focused) {
		s.opMu.Unlock()
		return false
	}
	s.opMu.Unlock()
	changed := s.changeFocusedNode(nil)
	s.opMu.Lock()
	if changed && s.Styles != nil {
		s.focusStylePending = true
	}
	s.opMu.Unlock()
	return changed
}

func shadowFocusDelegate(root *dom.ShadowRoot) *dom.Node {
	if root == nil || !root.DelegatesFocus {
		return nil
	}
	var find func(*dom.Node) *dom.Node
	find = func(node *dom.Node) *dom.Node {
		for current := node; current != nil; current = current.NextSibling {
			if current.Element() && programmaticallyFocusable(current) {
				return current
			}
			if current.ShadowRoot != nil && current.ShadowRoot.DelegatesFocus {
				if nested := shadowFocusDelegate(current.ShadowRoot); nested != nil {
					return nested
				}
			}
			if current.FirstChild != nil {
				if nested := find(current.FirstChild); nested != nil {
					return nested
				}
			}
		}
		return nil
	}
	return find(root.FirstChild)
}

func shadowTreeContains(root *dom.ShadowRoot, target *dom.Node) bool {
	if root == nil || target == nil {
		return false
	}
	var contains func(*dom.Node) bool
	contains = func(node *dom.Node) bool {
		for current := node; current != nil; current = current.NextSibling {
			if current == target {
				return true
			}
			if contains(current.FirstChild) {
				return true
			}
			if current.ShadowRoot != nil && contains(current.ShadowRoot.FirstChild) {
				return true
			}
		}
		return false
	}
	return contains(root.FirstChild)
}

func (s *Session) delegatesFocusHost(target *dom.Node) *dom.Node {
	if s.Doc == nil || target == nil {
		return nil
	}
	var find func(*dom.Node) *dom.Node
	find = func(node *dom.Node) *dom.Node {
		for current := node; current != nil; current = current.NextSibling {
			if current.ShadowRoot != nil && shadowTreeContains(current.ShadowRoot, target) {
				if nested := find(current.ShadowRoot.FirstChild); nested != nil {
					return nested
				}
				if current.ShadowRoot.DelegatesFocus {
					return current
				}
			}
			if nested := find(current.FirstChild); nested != nil {
				return nested
			}
		}
		return nil
	}
	return find(&s.Doc.Node)
}

// changeFocusedNode ends an active composition before the old host blurs. The
// event is dispatched without opMu held because page listeners can reenter the
// engine; if they change focus themselves, their focus decision wins.
func (s *Session) changeFocusedNode(n *dom.Node) bool {
	s.opMu.Lock()
	if n == s.focus {
		s.opMu.Unlock()
		return false
	}
	prev := s.focus
	// A held Space activation ends when its control loses focus, even if the
	// host action sequence has not delivered keyup yet.
	s.clearKeyboardActiveLocked()
	composing := s.imeComposing
	oldMarked := s.marked
	markedStart := s.markedOffset
	markedNode := s.markedNode
	markedHasNode := s.markedHasNode
	rt := s.jsRT
	if composing {
		s.imeComposing = false
		s.marked = ""
	}
	s.opMu.Unlock()
	if composing && controlEditable(prev) && oldMarked != "" {
		if rt != nil {
			rt.DispatchTrustedCompositionEvent(prev, "compositionupdate", "")
			_ = rt.DispatchTrustedBeforeInputEvent(prev, nil, "deleteCompositionText", true, false)
		}
		s.opMu.Lock()
		if s.focus == prev {
			s.replaceControlCompositionLocked(prev, markedStart, oldMarked, "")
		}
		s.opMu.Unlock()
		if rt != nil {
			rt.DispatchTrustedInput(prev, nil, "deleteCompositionText", true)
		}
	}
	if composing && dom.IsContentEditable(prev) && oldMarked != "" && markedHasNode && rt != nil {
		rt.DispatchTrustedCompositionEvent(prev, "compositionupdate", "")
		_ = rt.DispatchTrustedBeforeInputEvent(prev, nil, "deleteCompositionText", true, false)
		if rt.ReplaceCompositionText(prev, markedNode, markedStart, oldMarked, "") {
			rt.DispatchTrustedInput(prev, nil, "deleteCompositionText", true)
		}
	}
	if composing && prev != nil && rt != nil {
		rt.DispatchTrustedCompositionEvent(prev, "compositionend", "")
	}
	s.opMu.Lock()
	if s.focus != prev {
		s.opMu.Unlock()
		return false
	}
	changed := s.setFocusedNodeLocked(n)
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
		previousHost := s.delegatesFocusHost(prev)
		css.SetFocus(prev, false)
		css.SetFocusVisible(prev, false)
		if previousHost != nil {
			css.SetFocus(previousHost, false)
			css.SetFocusVisible(previousHost, false)
		}
		dom.DispatchEvent(prev, dom.NewEvent("blur", false, false))
		if previousHost != nil {
			dom.DispatchEvent(previousHost, dom.NewEvent("blur", false, false))
		}
		dom.DispatchEvent(prev, dom.NewEvent("focusout", true, false))
		if previousHost != nil {
			dom.DispatchEvent(previousHost, dom.NewEvent("focusout", true, false))
		}
		if controlEditable(prev) && s.focusUserChanged && s.controlValue(prev) != s.focusValue {
			dom.DispatchEvent(prev, dom.NewEvent("change", true, true))
		}
	}
	s.focus = n
	s.focusState.Store(n)
	s.marked = ""
	s.markedHasNode = false
	s.imeComposing = false
	s.controlSelectionActive = false
	s.focusUserChanged = false
	nextHost := s.delegatesFocusHost(n)
	if controlEditable(n) {
		s.caret = len([]rune(s.controlValue(n)))
		s.controlSelectionActive = false
		s.controlSelectionAnchor = s.caret
		s.focusValue = s.controlValue(n)
		css.SetFocus(n, true)
		css.SetFocusVisible(n, focusVisibleByDefault(n))
		if nextHost != nil {
			css.SetFocus(nextHost, true)
			css.SetFocusVisible(nextHost, focusVisibleByDefault(nextHost))
		}
		dom.DispatchEvent(n, dom.NewEvent("focus", false, false))
		if nextHost != nil {
			dom.DispatchEvent(nextHost, dom.NewEvent("focus", false, false))
		}
		dom.DispatchEvent(n, dom.NewEvent("focusin", true, false))
		if nextHost != nil {
			dom.DispatchEvent(nextHost, dom.NewEvent("focusin", true, false))
		}
	} else {
		s.caret = 0
		s.controlSelectionAnchor = 0
		s.focusValue = ""
		if n != nil {
			css.SetFocus(n, true)
			css.SetFocusVisible(n, focusVisibleByDefault(n))
			if nextHost != nil {
				css.SetFocus(nextHost, true)
				css.SetFocusVisible(nextHost, focusVisibleByDefault(nextHost))
			}
			dom.DispatchEvent(n, dom.NewEvent("focus", false, false))
			if nextHost != nil {
				dom.DispatchEvent(nextHost, dom.NewEvent("focus", false, false))
			}
			dom.DispatchEvent(n, dom.NewEvent("focusin", true, false))
			if nextHost != nil {
				dom.DispatchEvent(nextHost, dom.NewEvent("focusin", true, false))
			}
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
		return !strings.EqualFold(strings.TrimSpace(n.GetAttribute("type")), "hidden")
	}
	if dom.IsContentEditable(n) {
		return true
	}
	return false
}

func programmaticallyFocusable(n *dom.Node) bool {
	if n == nil || !n.Element() || dom.IsFormControlDisabled(n) {
		return false
	}
	if dom.IsContentEditable(n) {
		return true
	}
	if n.HasAttribute("tabindex") {
		_, err := strconv.Atoi(strings.TrimSpace(n.GetAttribute("tabindex")))
		return err == nil
	}
	switch n.Data {
	case "input":
		return strings.ToLower(n.GetAttribute("type")) != "hidden"
	case "textarea", "select", "button", "summary":
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

// MarkUserInput records a text-control edit performed by a host input adapter.
// It lets adapters distinguish a simulated user edit from script assignments
// when deciding whether blur should dispatch change.
func (s *Session) MarkUserInput(n *dom.Node) bool {
	if s == nil || n == nil || (n.Data != "input" && n.Data != "textarea") {
		return false
	}
	s.opMu.Lock()
	n.ValueChangedByUser = true
	focused := s.focus == n
	if focused {
		s.focusUserChanged = true
	}
	s.opMu.Unlock()
	return focused
}

// SetMarked updates the active IME composition at the focused control's caret
// or contenteditable Selection. Text controls and contenteditable Text nodes
// update live when the active passage can be represented; an empty string
// cancels the active composition.
func (s *Session) SetMarked(text string) bool {
	s.opMu.Lock()
	host := s.focus
	if !controlEditable(host) && !dom.IsContentEditable(host) {
		s.opMu.Unlock()
		return false
	}
	if controlEditable(host) {
		if s.marked == text {
			s.opMu.Unlock()
			return false
		}
		wasComposing := s.imeComposing
		oldMarked := s.marked
		start := s.caret
		if wasComposing {
			start = s.markedOffset
		}
		s.imeComposing = text != ""
		s.marked = text
		s.markedNode = 0
		s.markedHasNode = false
		s.markedOffset = start
		rt := s.jsRT
		s.opMu.Unlock()
		if rt != nil {
			if !wasComposing && text != "" {
				if !rt.DispatchTrustedCompositionEvent(host, "compositionstart", "") {
					s.opMu.Lock()
					if s.focus == host && s.marked == text {
						s.marked = ""
						s.imeComposing = false
					}
					s.opMu.Unlock()
					return false
				}
			}
			if text != "" || wasComposing {
				rt.DispatchTrustedCompositionEvent(host, "compositionupdate", text)
			}
		}
		if text != "" || wasComposing {
			inputType := "insertCompositionText"
			var data *string = &text
			if text == "" {
				inputType = "deleteCompositionText"
				data = nil
			}
			if rt != nil {
				_ = rt.DispatchTrustedBeforeInputEvent(host, data, inputType, true, false)
			}
			s.opMu.Lock()
			current := s.focus == host && s.marked == text && s.imeComposing == (text != "")
			if current {
				s.replaceControlCompositionLocked(host, start, oldMarked, text)
			}
			s.opMu.Unlock()
			if current && rt != nil {
				rt.DispatchTrustedInput(host, data, inputType, true)
			}
			if current && text == "" && wasComposing && rt != nil {
				rt.DispatchTrustedCompositionEvent(host, "compositionend", "")
			}
		}
		return true
	}
	wasComposing := s.imeComposing
	oldMarked := s.marked
	nodeID := s.markedNode
	hasNode := s.markedHasNode
	offset := s.markedOffset
	rt := s.jsRT
	s.opMu.Unlock()
	if !wasComposing && rt != nil {
		if node, at, ok := rt.SelectionTextCaret(host); ok {
			nodeID, offset = node.ID, at
			hasNode = true
		}
	}
	s.opMu.Lock()
	if s.focus != host || s.imeComposing != wasComposing || s.marked != oldMarked || (s.marked == text && s.markedNode == nodeID && s.markedOffset == offset) {
		s.opMu.Unlock()
		return false
	}
	if text == "" {
		s.imeComposing = false
	} else {
		s.imeComposing = true
	}
	s.marked = text
	s.markedNode = nodeID
	s.markedHasNode = hasNode
	s.markedOffset = offset
	rt = s.jsRT
	s.opMu.Unlock()
	if rt != nil {
		if !wasComposing && text != "" {
			if !rt.DispatchTrustedCompositionEvent(host, "compositionstart", "") {
				s.opMu.Lock()
				if s.focus == host && s.marked == text {
					s.marked = ""
					s.markedNode = 0
					s.imeComposing = false
					s.markedHasNode = false
				}
				s.opMu.Unlock()
				return false
			}
		}
		if text != "" || wasComposing {
			rt.DispatchTrustedCompositionEvent(host, "compositionupdate", text)
		}
	}
	if rt != nil && hasNode && (text != "" || wasComposing) {
		inputType := "insertCompositionText"
		var data *string = &text
		if text == "" {
			inputType = "deleteCompositionText"
			data = nil
		}
		_ = rt.DispatchTrustedBeforeInputEvent(host, data, inputType, true, false)
		if rt.ReplaceCompositionText(host, nodeID, offset, oldMarked, text) {
			rt.DispatchTrustedInput(host, data, inputType, true)
		} else {
			s.opMu.Lock()
			if s.focus == host && s.marked == text {
				s.markedHasNode = false
			}
			s.opMu.Unlock()
		}
	}
	if text == "" && wasComposing && rt != nil {
		rt.DispatchTrustedCompositionEvent(host, "compositionend", "")
	}
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
	return s.commitTextWithInputType(text, "")
}

// PasteText inserts plain clipboard text using the paste-specific editing
// input type. Editable content receives the text/plain DataTransfer payload.
func (s *Session) PasteText(text string) bool {
	return s.PasteData(text, "")
}

// PasteData inserts clipboard formats into the focused editable target.
// Contenteditable hosts prefer text/html when present; text controls consume
// only text/plain.
func (s *Session) PasteData(text, html string) bool {
	s.opMu.Lock()
	host := s.focus
	contentEditable := dom.IsContentEditable(host)
	textControl := textSelectableControl(host)
	rt := s.jsRT
	s.opMu.Unlock()
	if !contentEditable && !textControl || text == "" && (html == "" || !contentEditable) || len(html) > 1<<20 {
		return false
	}
	clipboardData := dom.NewDataTransfer()
	if text != "" {
		clipboardData.SetData("text/plain", text)
	}
	if html != "" {
		clipboardData.SetData("text/html", html)
	}
	clipboardData.SetMode(dom.DataTransferReadOnly)
	clipboardAllowed := dispatchTrustedClipboardEvent(rt, host, "paste", clipboardData)
	clipboardData.Disassociate()
	if !clipboardAllowed {
		return false
	}
	if contentEditable {
		transfer := dom.NewDataTransfer()
		if text != "" {
			transfer.SetData("text/plain", text)
		}
		if html != "" {
			transfer.SetData("text/html", html)
		}
		transfer.SetMode(dom.DataTransferReadOnly)
		if html != "" {
			return s.insertContentEditableHTMLWithDataTransfer(html, transfer)
		}
		return s.insertContentEditableTextWithDataTransfer(text, "insertFromPaste", transfer)
	}
	return s.commitTextWithInputType(text, "insertFromPaste")
}

// CopySelection dispatches a trusted copy event for the selected text and
// returns the text/plain payload to the host clipboard.
func (s *Session) CopySelection() string {
	text, _ := s.CopySelectionData()
	return text
}

// CopySelectionData dispatches copy and returns both supported clipboard
// formats for the host clipboard.
func (s *Session) CopySelectionData() (string, string) {
	host, rt, text := s.clipboardSelection()
	if host == nil || text == "" {
		return "", ""
	}
	html := s.clipboardSelectionHTML(host, rt)
	data := dom.NewDataTransfer()
	data.SetData("text/plain", text)
	if html != "" {
		data.SetData("text/html", html)
	}
	data.SetMode(dom.DataTransferReadWrite)
	dispatchTrustedClipboardEvent(rt, host, "copy", data)
	plainResult, htmlResult := data.GetData("text/plain"), data.GetData("text/html")
	data.Disassociate()
	return plainResult, htmlResult
}

// CutSelection dispatches a trusted cut event, then the deleteByCut editing
// operation when the clipboard event was not canceled. It returns clipboard
// text and whether the selected content was deleted.
func (s *Session) CutSelection() (string, bool) {
	text, _, changed := s.CutSelectionData()
	return text, changed
}

// CutSelectionData dispatches cut and returns the formats supplied by the
// event handlers along with whether the selected content was deleted.
func (s *Session) CutSelectionData() (string, string, bool) {
	host, rt, text := s.clipboardSelection()
	if host == nil || text == "" {
		return "", "", false
	}
	html := s.clipboardSelectionHTML(host, rt)
	data := dom.NewDataTransfer()
	data.SetData("text/plain", text)
	if html != "" {
		data.SetData("text/html", html)
	}
	data.SetMode(dom.DataTransferReadWrite)
	accepted := dispatchTrustedClipboardEvent(rt, host, "cut", data)
	clipboardText, clipboardHTML := data.GetData("text/plain"), data.GetData("text/html")
	data.Disassociate()
	if !accepted {
		return clipboardText, clipboardHTML, false
	}
	if controlEditable(host) && textSelectableControl(host) {
		return clipboardText, clipboardHTML, s.deleteTextControlContentWithType(false, "deleteByCut")
	}
	if !dom.IsContentEditable(host) || rt == nil {
		return clipboardText, clipboardHTML, false
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteByCut") {
		return clipboardText, clipboardHTML, false
	}
	script := `(function(){var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||sel.isCollapsed)return false;return document.execCommand("delete",false,null)})()`
	result, err := rt.EvaluateUserInput(script, "deleteByCut")
	return clipboardText, clipboardHTML, err == nil && result == "true"
}

func (s *Session) clipboardSelectionHTML(host *dom.Node, rt *js.Runtime) string {
	if host == nil || rt == nil || !dom.IsContentEditable(host) {
		return ""
	}
	value, err := rt.Evaluate(`(function(){var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.rangeCount||!sel.anchorNode||!sel.focusNode||!host.contains(sel.anchorNode)||!host.contains(sel.focusNode))return "";var range=sel.getRangeAt(0),common=range.commonAncestorContainer,holder=document.createElement("div");holder.appendChild(range.cloneContents());var selected=holder.firstChild;if(common.nodeType===3){for(var ancestor=common.parentNode;ancestor&&ancestor!==host;ancestor=ancestor.parentNode){var wrapper=ancestor.cloneNode(false);wrapper.appendChild(selected);selected=wrapper}holder.textContent="";if(selected)holder.appendChild(selected)}return holder.innerHTML})()`)
	if err != nil {
		return ""
	}
	return value
}

func (s *Session) clipboardSelection() (*dom.Node, *js.Runtime, string) {
	s.opMu.Lock()
	s.syncControlSelectionFromRuntimeLocked()
	host, rt := s.focus, s.jsRT
	if textSelectableControl(host) {
		value := []rune(s.controlValue(host))
		start, end, selected := s.controlSelectionBounds(len(value))
		s.opMu.Unlock()
		if selected {
			return host, rt, string(value[start:end])
		}
		return nil, rt, ""
	}
	contentEditable := dom.IsContentEditable(host)
	body := (*dom.Node)(nil)
	if s.Doc != nil {
		body = s.Doc.Body
	}
	s.opMu.Unlock()
	if contentEditable && rt != nil {
		value, err := rt.Evaluate(`(function(){var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.rangeCount||!sel.anchorNode||!sel.focusNode||!host.contains(sel.anchorNode)||!host.contains(sel.focusNode))return "";return sel.toString()})()`)
		if err == nil && value != "" {
			return host, rt, value
		}
		return nil, rt, ""
	}
	if s.HasSelection() {
		return body, rt, s.SelectionText()
	}
	return nil, rt, ""
}

func (s *Session) commitTextWithInputType(text, requestedInputType string) bool {
	s.opMu.Lock()
	s.syncControlSelectionFromRuntimeLocked()
	if dom.IsContentEditable(s.focus) {
		host := s.focus
		rt := s.jsRT
		composing := s.imeComposing
		marked := s.marked
		markedNode := s.markedNode
		markedHasNode := s.markedHasNode
		markedOffset := s.markedOffset
		s.marked = ""
		s.markedHasNode = false
		s.imeComposing = false
		s.opMu.Unlock()
		if composing && rt != nil && markedHasNode {
			if text == "" {
				rt.DispatchTrustedCompositionEvent(host, "compositionupdate", "")
				_ = rt.DispatchTrustedBeforeInputEvent(host, nil, "deleteCompositionText", true, false)
				changed := rt.ReplaceCompositionText(host, markedNode, markedOffset, marked, "")
				if changed {
					rt.DispatchTrustedInput(host, nil, "deleteCompositionText", true)
				}
				rt.DispatchTrustedCompositionEvent(host, "compositionend", "")
				return changed
			}
			if !rt.DispatchTrustedBeforeInputEvent(host, &text, "insertFromComposition", true, true) {
				rt.DispatchTrustedCompositionEvent(host, "compositionupdate", "")
				_ = rt.DispatchTrustedBeforeInputEvent(host, nil, "deleteCompositionText", true, false)
				if rt.ReplaceCompositionText(host, markedNode, markedOffset, marked, "") {
					rt.DispatchTrustedInput(host, nil, "deleteCompositionText", true)
				}
				rt.DispatchTrustedCompositionEvent(host, "compositionend", "")
				return false
			}
			changed := rt.ReplaceCompositionText(host, markedNode, markedOffset, marked, text)
			if changed {
				rt.DispatchTrustedInput(host, &text, "insertFromComposition", true)
				rt.DispatchTrustedCompositionEvent(host, "compositionend", text)
			}
			return changed
		}
		inputType := "insertText"
		if composing {
			inputType = "insertFromComposition"
			if rt != nil {
				rt.DispatchTrustedCompositionEvent(host, "compositionend", text)
			}
		}
		return s.insertContentEditableTextWithType(text, inputType)
	}
	if !controlEditable(s.focus) {
		s.opMu.Unlock()
		return false
	}
	host := s.focus
	rt := s.jsRT
	composing := s.imeComposing
	marked := s.marked
	markedStart := s.markedOffset
	added := []rune(text)
	runes := []rune(s.controlValue(host))
	start := s.caret
	if start < 0 {
		start = 0
	}
	if start > len(runes) {
		start = len(runes)
	}
	end := start
	if composing {
		start = markedStart
		if start < 0 {
			start = 0
		}
		if start > len(runes) {
			start = len(runes)
		}
		end = start + len([]rune(marked))
		if end > len(runes) {
			end = len(runes)
		}
	} else if selectedStart, selectedEnd, selected := s.controlSelectionBounds(len(runes)); selected {
		start, end = selectedStart, selectedEnd
	}
	baseLength := utf16Length(string(runes[:start]) + string(runes[end:]))
	if max, hasMax := maxLen(host); hasMax && baseLength+utf16Length(text) > max {
		if composing {
			s.replaceControlCompositionLocked(host, markedStart, marked, "")
		}
		s.marked = ""
		s.markedHasNode = false
		s.imeComposing = false
		s.opMu.Unlock()
		if composing && rt != nil {
			rt.DispatchTrustedCompositionEvent(host, "compositionend", "")
		}
		return false
	}
	s.opMu.Unlock()

	inputType := "insertText"
	if composing {
		inputType = "insertFromComposition"
	} else if requestedInputType != "" {
		inputType = requestedInputType
	}
	if text != "" && !dispatchTrustedBeforeInput(rt, host, &text, inputType, composing, true) {
		s.opMu.Lock()
		if s.focus == host {
			if composing {
				s.replaceControlCompositionLocked(host, markedStart, marked, "")
			}
			s.marked = ""
			s.markedHasNode = false
			s.imeComposing = false
		}
		s.opMu.Unlock()
		if composing {
			rt.DispatchTrustedCompositionEvent(host, "compositionend", "")
		}
		return false
	}

	s.opMu.Lock()
	if s.focus != host {
		s.opMu.Unlock()
		return false
	}
	runes = []rune(s.controlValue(host))
	start = s.caret
	if composing {
		start = markedStart
	}
	if start < 0 {
		start = 0
	}
	if start > len(runes) {
		start = len(runes)
	}
	end = start
	if composing {
		end += len([]rune(marked))
		if end > len(runes) {
			end = len(runes)
		}
	} else if selectedStart, selectedEnd, selected := s.controlSelectionBounds(len(runes)); selected {
		start, end = selectedStart, selectedEnd
	}
	if s.caret > len(runes) {
		s.caret = len(runes)
	}
	if max, hasMax := maxLen(host); hasMax && utf16Length(string(runes[:start])+text+string(runes[end:])) > max {
		if composing {
			s.replaceControlCompositionLocked(host, markedStart, marked, "")
		}
		s.marked = ""
		s.markedHasNode = false
		s.imeComposing = false
		s.opMu.Unlock()
		if composing && rt != nil {
			rt.DispatchTrustedCompositionEvent(host, "compositionend", "")
		}
		return false
	}
	out := make([]rune, 0, len(runes)-(end-start)+len(added))
	out = append(out, runes[:start]...)
	out = append(out, added...)
	out = append(out, runes[end:]...)
	s.caret = start + len(added)
	s.setControlValue(s.focus, string(out))
	s.controlSelectionActive = false
	s.syncControlSelectionLocked()
	s.marked = ""
	s.markedHasNode = false
	s.imeComposing = false
	s.opMu.Unlock()
	if text != "" {
		dispatchTrustedInput(rt, host, &text, inputType, composing)
	}
	if composing && rt != nil {
		rt.DispatchTrustedCompositionEvent(host, "compositionend", text)
	}
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
	s.restoreDialogFocusLocked(topmost)
	s.queueDialogClose(topmost)
	return true
}

func (s *Session) restoreDialogFocusLocked(dialog *dom.Node) {
	if s == nil || dialog == nil {
		return
	}
	var previous *dom.Node
	var hasReturnFocus bool
	if s.jsRT != nil {
		previous, hasReturnFocus = s.jsRT.TakeDialogFocusReturn(dialog)
	}
	if !hasReturnFocus {
		return
	}
	if previous == nil || !previous.IsConnected() {
		if s.Doc != nil {
			previous = s.Doc.Body
			if previous == nil {
				previous = s.Doc.HTML
			}
		}
	}
	if previous != nil {
		s.setFocusedNodeLocked(previous)
	}
}

// Edit applies one keyboard edit to the focused control. It returns whether
// the value or focus changed, which is when the caller must reflow;
// caret-only movement returns false but still needs a repaint.
func (s *Session) Edit(action EditAction, r rune) bool {
	return s.EditWithShift(action, r, false)
}

// EditWithShift applies a keyboard edit while preserving or extending the
// focused text control's selection when Shift is held.
func (s *Session) EditWithShift(action EditAction, r rune, shift bool) bool {
	if action == EditRune || action == EditEnter || action == EditBackspace || action == EditDelete || action == EditLeft || action == EditRight || action == EditHome || action == EditEnd {
		// Contenteditable uses the document Selection rather than the form
		// control caret maintained below. Run its insertion command outside the
		// session lock because script event handlers can call back into Session.
		s.opMu.Lock()
		editable := dom.IsContentEditable(s.focus)
		s.opMu.Unlock()
		if editable {
			if action == EditLeft || action == EditRight || action == EditHome || action == EditEnd {
				s.moveContentEditableCaret(action)
				return false // caret movement does not change document geometry
			}
			if action == EditBackspace {
				return s.deleteContentEditable(false)
			}
			if action == EditDelete {
				return s.deleteContentEditable(true)
			}
			if action == EditEnter {
				inputType := "insertParagraph"
				if shift {
					inputType = "insertLineBreak"
				}
				return s.insertContentEditableTextWithType("\n", inputType)
			}
			if r < 0x20 {
				return false
			}
			return s.insertContentEditableText(string(r))
		}
	}
	// Route ordinary text-control edits through Input Events before mutating
	// the value. Event handlers run JavaScript and may call back into Session,
	// so the edit path must not hold opMu while dispatching beforeinput.
	if action == EditRune || action == EditBackspace || action == EditDelete || action == EditEnter {
		s.opMu.Lock()
		s.syncControlSelectionFromRuntimeLocked()
		host := s.focus
		textControl := textSelectableControl(host)
		textarea := host != nil && host.DataAtom == dom.AtomTextarea
		s.opMu.Unlock()
		if textControl {
			switch action {
			case EditRune:
				if r >= 0x20 {
					return s.CommitText(string(r))
				}
			case EditEnter:
				if textarea {
					return s.commitTextWithInputType("\n", "insertLineBreak")
				}
			case EditBackspace:
				return s.deleteTextControlContent(false)
			case EditDelete:
				return s.deleteTextControlContent(true)
			}
		}
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.syncControlSelectionFromRuntimeLocked()
	if action == EditEscape {
		if s.closeTopmostAutoPopoverLocked() {
			return true
		}
		closedDialog := s.closeTopmostModalDialogLocked()
		if s.focus != nil {
			prev := s.focus
			if s.imeComposing && controlEditable(prev) {
				s.replaceControlCompositionLocked(prev, s.markedOffset, s.marked, "")
			}
			s.focus = nil
			s.focusState.Store(nil)
			s.marked = ""
			s.markedHasNode = false
			s.imeComposing = false
			css.SetFocus(prev, false)
			css.SetFocusVisible(prev, false)
			dom.DispatchEvent(prev, dom.NewEvent("blur", false, false))
			return true
		}
		return closedDialog
	}
	if isRangeInput(s.focus) {
		return s.adjustRangeLocked(s.focus, action)
	}
	if !controlEditable(s.focus) {
		return false
	}
	runes := []rune(s.controlValue(s.focus))
	if s.caret > len(runes) {
		s.caret = len(runes)
	}
	selectionStart, selectionEnd, hasSelection := s.controlSelectionBounds(len(runes))
	insert := func(text string) bool {
		added := []rune(text)
		start, end := s.caret, s.caret
		if hasSelection {
			start, end = selectionStart, selectionEnd
		}
		candidate := string(runes[:start]) + text + string(runes[end:])
		if max, hasMax := maxLen(s.focus); hasMax && utf16Length(candidate) > max {
			return false
		}
		out := make([]rune, 0, len(runes)+len(added))
		out = append(out, runes[:start]...)
		out = append(out, added...)
		out = append(out, runes[end:]...)
		s.caret = start + len(added)
		s.controlSelectionActive = false
		s.setControlValue(s.focus, string(out))
		s.syncControlSelectionLocked()
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
		if hasSelection {
			out := append([]rune{}, runes[:selectionStart]...)
			out = append(out, runes[selectionEnd:]...)
			s.caret = selectionStart
			s.controlSelectionActive = false
			s.setControlValue(s.focus, string(out))
			s.syncControlSelectionLocked()
			dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
			return true
		}
		if s.caret == 0 || len(runes) == 0 {
			return false
		}
		out := append([]rune{}, runes[:s.caret-1]...)
		out = append(out, runes[s.caret:]...)
		s.caret--
		s.controlSelectionActive = false
		s.setControlValue(s.focus, string(out))
		s.syncControlSelectionLocked()
		dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
		return true
	case EditDelete:
		if hasSelection {
			out := append([]rune{}, runes[:selectionStart]...)
			out = append(out, runes[selectionEnd:]...)
			s.caret = selectionStart
			s.controlSelectionActive = false
			s.setControlValue(s.focus, string(out))
			s.syncControlSelectionLocked()
			dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
			return true
		}
		if s.caret >= len(runes) {
			return false
		}
		out := append([]rune{}, runes[:s.caret]...)
		out = append(out, runes[s.caret+1:]...)
		s.controlSelectionActive = false
		s.setControlValue(s.focus, string(out))
		s.syncControlSelectionLocked()
		dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
		return true
	case EditLeft:
		if shift {
			if !s.controlSelectionActive {
				s.controlSelectionAnchor = s.caret
				s.controlSelectionActive = true
			}
			if s.caret > 0 {
				s.caret--
			}
			if s.caret == s.controlSelectionAnchor {
				s.controlSelectionActive = false
			}
		} else if hasSelection {
			s.caret = selectionStart
			s.controlSelectionActive = false
		} else if s.caret > 0 {
			s.caret--
		}
		if !shift {
			s.controlSelectionAnchor = s.caret
		}
		s.syncControlSelectionLocked()
		return false
	case EditRight:
		if shift {
			if !s.controlSelectionActive {
				s.controlSelectionAnchor = s.caret
				s.controlSelectionActive = true
			}
			if s.caret < len(runes) {
				s.caret++
			}
			if s.caret == s.controlSelectionAnchor {
				s.controlSelectionActive = false
			}
		} else if hasSelection {
			s.caret = selectionEnd
			s.controlSelectionActive = false
		} else if s.caret < len(runes) {
			s.caret++
		}
		if !shift {
			s.controlSelectionAnchor = s.caret
		}
		s.syncControlSelectionLocked()
		return false
	case EditHome:
		if shift {
			if !s.controlSelectionActive {
				s.controlSelectionAnchor = s.caret
				s.controlSelectionActive = true
			}
			s.caret = 0
			s.controlSelectionActive = s.caret != s.controlSelectionAnchor
		} else {
			s.caret = 0
			s.controlSelectionActive = false
			s.controlSelectionAnchor = s.caret
		}
		s.syncControlSelectionLocked()
		return false
	case EditEnd:
		if shift {
			if !s.controlSelectionActive {
				s.controlSelectionAnchor = s.caret
				s.controlSelectionActive = true
			}
			s.caret = len(runes)
			s.controlSelectionActive = s.caret != s.controlSelectionAnchor
		} else {
			s.caret = len(runes)
			s.controlSelectionActive = false
			s.controlSelectionAnchor = s.caret
		}
		s.syncControlSelectionLocked()
		return false
	case EditEscape:
		s.focus = nil
		s.focusState.Store(nil)
		s.marked = ""
		s.imeComposing = false
		return true
	}
	return false
}

func (s *Session) deleteTextControlContent(forward bool) bool {
	return s.deleteTextControlContentWithType(forward, "")
}

func (s *Session) deleteTextControlContentWithType(forward bool, requestedInputType string) bool {
	s.opMu.Lock()
	s.syncControlSelectionFromRuntimeLocked()
	host := s.focus
	if !textSelectableControl(host) {
		s.opMu.Unlock()
		return false
	}
	rt := s.jsRT
	value := []rune(s.controlValue(host))
	caret := s.caret
	if caret < 0 {
		caret = 0
	}
	if caret > len(value) {
		caret = len(value)
	}
	_, _, selected := s.controlSelectionBounds(len(value))
	if !selected && ((forward && caret == len(value)) || (!forward && caret == 0)) {
		s.opMu.Unlock()
		return false
	}
	inputType := requestedInputType
	if inputType == "" {
		inputType = "deleteContentBackward"
		if forward {
			inputType = "deleteContentForward"
		}
	}
	s.opMu.Unlock()

	if !dispatchTrustedBeforeInput(rt, host, nil, inputType, false, true) {
		return false
	}

	s.opMu.Lock()
	if s.focus != host || !textSelectableControl(host) {
		s.opMu.Unlock()
		return false
	}
	runes := []rune(s.controlValue(host))
	if s.caret < 0 {
		s.caret = 0
	}
	if s.caret > len(runes) {
		s.caret = len(runes)
	}
	start, end, selected := s.controlSelectionBounds(len(runes))
	if !selected {
		start, end = s.caret, s.caret
		if forward {
			if end >= len(runes) {
				s.opMu.Unlock()
				return false
			}
			end++
		} else {
			if start <= 0 {
				s.opMu.Unlock()
				return false
			}
			start--
		}
	}
	out := make([]rune, 0, len(runes)-(end-start))
	out = append(out, runes[:start]...)
	out = append(out, runes[end:]...)
	s.setControlValue(host, string(out))
	s.caret = start
	s.controlSelectionActive = false
	s.syncControlSelectionLocked()
	s.opMu.Unlock()

	dispatchTrustedInput(rt, host, nil, inputType, false)
	return true
}

func dispatchTrustedBeforeInput(rt *js.Runtime, host *dom.Node, data *string, inputType string, composing, cancelable bool) bool {
	if rt != nil {
		return rt.DispatchTrustedBeforeInputEvent(host, data, inputType, composing, cancelable)
	}
	event := dom.NewTrustedInputEvent("beforeinput", true, cancelable, data, inputType, composing)
	return dom.DispatchEvent(host, &event.Event)
}

func dispatchTrustedInput(rt *js.Runtime, host *dom.Node, data *string, inputType string, composing bool) {
	if rt != nil {
		rt.DispatchTrustedInput(host, data, inputType, composing)
		return
	}
	event := dom.NewTrustedInputEvent("input", true, false, data, inputType, composing)
	dom.DispatchEvent(host, &event.Event)
}

func dispatchTrustedClipboardEvent(rt *js.Runtime, host *dom.Node, typ string, data *dom.DataTransfer) bool {
	if rt != nil {
		return rt.DispatchTrustedClipboardEvent(host, typ, data)
	}
	event := dom.NewTrustedClipboardEvent(typ, true, true, data)
	return dom.DispatchEvent(host, &event.Event)
}

func (s *Session) controlSelectionBounds(length int) (int, int, bool) {
	if !s.controlSelectionActive {
		return 0, 0, false
	}
	anchor := s.controlSelectionAnchor
	if anchor < 0 {
		anchor = 0
	}
	if anchor > length {
		anchor = length
	}
	caret := s.caret
	if caret < 0 {
		caret = 0
	}
	if caret > length {
		caret = length
	}
	if anchor == caret {
		return 0, 0, false
	}
	if anchor < caret {
		return anchor, caret, true
	}
	return caret, anchor, true
}

// syncControlSelectionFromRuntimeLocked imports a script-updated text-control
// selection before a host editing action reads the engine's rune-indexed state.
// Callers hold opMu; runtime selection access is a read of its current DOM state.
func (s *Session) syncControlSelectionFromRuntimeLocked() {
	if s.jsRT == nil || !textSelectableControl(s.focus) {
		return
	}
	startOffset, endOffset, direction, ok := s.jsRT.TextControlSelection(s.focus)
	if !ok {
		return
	}
	value := s.controlValue(s.focus)
	start := runeIndexForUTF16Offset(value, startOffset)
	end := runeIndexForUTF16Offset(value, endOffset)
	if start == end {
		s.caret = end
		s.controlSelectionAnchor = end
		s.controlSelectionActive = false
		return
	}
	s.controlSelectionActive = true
	if direction == "backward" {
		s.controlSelectionAnchor = end
		s.caret = start
	} else {
		s.controlSelectionAnchor = start
		s.caret = end
	}
}

func runeIndexForUTF16Offset(value string, offset int) int {
	if offset <= 0 {
		return 0
	}
	units, runes := 0, 0
	for _, current := range value {
		width := 1
		if current > 0xFFFF {
			width = 2
		}
		if offset < units+width {
			return runes
		}
		units += width
		runes++
		if units >= offset {
			return runes
		}
	}
	return runes
}

func (s *Session) syncControlSelectionLocked() {
	if s.jsRT == nil || s.focus == nil || !controlEditable(s.focus) {
		return
	}
	runes := []rune(s.controlValue(s.focus))
	caret := s.caret
	if caret < 0 {
		caret = 0
	}
	if caret > len(runes) {
		caret = len(runes)
	}
	start, end, selected := s.controlSelectionBounds(len(runes))
	if !selected {
		start, end = caret, caret
	}
	startOffset := utf16Length(string(runes[:start]))
	endOffset := utf16Length(string(runes[:end]))
	direction := "none"
	if selected {
		if caret < s.controlSelectionAnchor {
			direction = "backward"
		} else {
			direction = "forward"
		}
	}
	s.jsRT.SetTextControlSelection(s.focus, startOffset, endOffset, direction)
}

func isRangeInput(n *dom.Node) bool {
	return n != nil && n.DataAtom == dom.AtomInput && strings.EqualFold(n.GetAttribute("type"), "range")
}

// adjustRangeLocked applies the keyboard value steps used by a range input.
// The caller holds Session.opMu.
func (s *Session) adjustRangeLocked(n *dom.Node, action EditAction) bool {
	if n == nil || dom.IsFormControlDisabled(n) {
		return false
	}
	minimum, hasMinimum := parseRangeNumber(n.GetAttribute("min"))
	if !hasMinimum {
		minimum = 0
	}
	maximum, hasMaximum := parseRangeNumber(n.GetAttribute("max"))
	if !hasMaximum {
		maximum = 100
	}
	if maximum < minimum {
		maximum = minimum
	}
	value, valid := parseRangeNumber(n.InputControlValue())
	if !valid {
		value = minimum + (maximum-minimum)/2
	}
	step := float64(1)
	if !strings.EqualFold(n.GetAttribute("step"), "any") {
		if candidate, ok := parseRangeNumber(n.GetAttribute("step")); ok && n.HasAttribute("step") && candidate > 0 {
			step = candidate
		}
	}
	switch action {
	case EditLeft, EditDown:
		value -= step
	case EditRight, EditUp:
		value += step
	case EditPageDown:
		value -= step * 10
	case EditPageUp:
		value += step * 10
	case EditHome:
		value = minimum
	case EditEnd:
		value = maximum
	default:
		return false
	}
	if value < minimum {
		value = minimum
	}
	if value > maximum {
		value = maximum
	}
	next := strconv.FormatFloat(value, 'f', -1, 64)
	if s.jsRT != nil {
		next = s.jsRT.SanitizeInputValue(n, next)
	}
	if next == n.InputControlValue() {
		return false
	}
	s.setControlValue(n, next)
	rt := s.jsRT
	s.opMu.Unlock()
	if rt != nil {
		rt.DispatchTrustedInput(n, nil, "", false)
	} else {
		event := dom.NewTrustedInputEvent("input", true, false, nil, "", false)
		dom.DispatchEvent(n, &event.Event)
	}
	s.opMu.Lock()
	return true
}

func parseRangeNumber(value string) (float64, bool) {
	number, err := strconv.ParseFloat(value, 64)
	return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
}

// setRangeValueAtLocked maps a pointer x coordinate to the range control's
// horizontal value domain. The caller holds Session.opMu.
func (s *Session) setRangeValueAtLocked(n *dom.Node, x float32) bool {
	if n == nil || dom.IsFormControlDisabled(n) || s.Arena == nil {
		return false
	}
	objectID, ok := s.Arena.ForNode(n.ID)
	if !ok {
		return false
	}
	obj := s.Arena.Get(objectID)
	x0, _, x1, _ := obj.ContentRect()
	left, right := x0+7, x1-7
	if right <= left {
		left, right = x0, x1
	}
	fraction := float64(0.5)
	if right > left {
		fraction = float64(x-left) / float64(right-left)
	}
	if fraction < 0 {
		fraction = 0
	} else if fraction > 1 {
		fraction = 1
	}
	if obj.Style != nil && obj.Style.Direction == "rtl" {
		fraction = 1 - fraction
	}
	minimum, hasMinimum := parseRangeNumber(n.GetAttribute("min"))
	if !hasMinimum {
		minimum = 0
	}
	maximum, hasMaximum := parseRangeNumber(n.GetAttribute("max"))
	if !hasMaximum {
		maximum = 100
	}
	if maximum < minimum {
		maximum = minimum
	}
	value := minimum + (maximum-minimum)*fraction
	if !strings.EqualFold(n.GetAttribute("step"), "any") {
		step := float64(1)
		if parsed, ok := parseRangeNumber(n.GetAttribute("step")); n.HasAttribute("step") && ok && parsed > 0 {
			step = parsed
		}
		base := minimum
		if !hasMinimum {
			if parsed, ok := parseRangeNumber(n.GetAttribute("value")); n.HasAttribute("value") && ok {
				base = parsed
			} else {
				base = 0
			}
		}
		quotient := (value - base) / step
		lower := math.Floor(quotient)
		if quotient-lower >= 0.5 {
			lower++
		}
		value = base + lower*step
		if value < minimum {
			value = minimum
		} else if value > maximum {
			value = maximum
		}
	}
	next := strconv.FormatFloat(value, 'f', -1, 64)
	if s.jsRT != nil {
		next = s.jsRT.SanitizeInputValue(n, next)
	}
	if next == n.InputControlValue() {
		return false
	}
	s.setControlValue(n, next)
	return true
}

// BeginRangePointerDragAt starts the pointer interaction for a range input.
// Value changes are committed during movement; click and change are dispatched
// when the pointer is released. It returns true when the point hit a range.
func (s *Session) BeginRangePointerDragAt(x, y float32) bool {
	n := s.nodeAt(x, y)
	if !isRangeInput(n) || dom.IsFormControlDisabled(n) {
		return false
	}
	s.FocusControl(x, y)
	s.opMu.Lock()
	s.rangePointerNode = n
	s.rangePointerMoved = false
	changed := s.setRangeValueAtLocked(n, x)
	if changed {
		s.rangePointerMoved = true
	}
	rt := s.jsRT
	s.opMu.Unlock()
	if changed {
		dispatchRangeInput(rt, n)
	}
	return true
}

// MoveRangePointerDragAt updates the value during an active range drag.
func (s *Session) MoveRangePointerDragAt(x, y float32) bool {
	s.opMu.Lock()
	n := s.rangePointerNode
	if n == nil {
		s.opMu.Unlock()
		return false
	}
	changed := s.setRangeValueAtLocked(n, x)
	if changed {
		s.rangePointerMoved = true
	}
	rt := s.jsRT
	s.opMu.Unlock()
	if changed {
		dispatchRangeInput(rt, n)
	}
	return true
}

// EndRangePointerDragAt completes the click gesture and emits change once if
// any drag position changed the value. A release over another element is not
// a click on the range input.
func (s *Session) EndRangePointerDragAt(x, y float32) bool {
	s.opMu.Lock()
	n := s.rangePointerNode
	changed := s.rangePointerMoved
	s.rangePointerNode = nil
	s.rangePointerMoved = false
	s.opMu.Unlock()
	if n == nil {
		return false
	}
	if s.nodeAt(x, y) == n && dom.DispatchEvent(n, dom.NewTrustedEvent("click", true, true)) && changed {
		dom.DispatchEvent(n, dom.NewTrustedEvent("change", true, false))
	}
	return true
}

func dispatchRangeInput(rt *js.Runtime, n *dom.Node) {
	if rt != nil {
		rt.DispatchTrustedInput(n, nil, "", false)
	} else {
		event := dom.NewTrustedInputEvent("input", true, false, nil, "", false)
		dom.DispatchEvent(n, &event.Event)
	}
}

func (s *Session) moveContentEditableCaret(action EditAction) {
	s.opMu.Lock()
	rt := s.jsRT
	host := s.focus
	focused := dom.IsContentEditable(host)
	s.opMu.Unlock()
	if rt == nil || !focused {
		return
	}
	direction := "right"
	switch action {
	case EditLeft:
		direction = "left"
	case EditHome:
		direction = "home"
	case EditEnd:
		direction = "end"
	}
	_ = rt.MoveSelectionCaret(host, direction)
}

func (s *Session) deleteContentEditable(forward bool) bool {
	return s.deleteContentEditableWithDeletionMode(forward, false, false)
}

func (s *Session) deleteContentEditableByWord(forward bool) bool {
	return s.deleteContentEditableWithDeletionMode(forward, true, false)
}

func (s *Session) deleteContentEditableSoftLineForward() bool {
	return s.deleteContentEditableWithDeletionMode(true, false, true)
}

func (s *Session) deleteContentEditableWithDeletionMode(forward, byWord, softLine bool) bool {
	s.opMu.Lock()
	rt := s.jsRT
	host := s.focus
	focused := dom.IsContentEditable(host)
	s.opMu.Unlock()
	if rt == nil || !focused {
		return false
	}
	inputType := "deleteContentBackward"
	if forward {
		inputType = "deleteContentForward"
	}
	if softLine && forward {
		inputType = "deleteSoftLineForward"
	}
	deleteWord := false
	if byWord {
		collapsed, err := rt.Evaluate(`(function(){var selection=getSelection();return !selection||selection.isCollapsed?"true":"false"})()`)
		deleteWord = err == nil && collapsed == "true"
		if deleteWord && forward {
			inputType = "deleteWordForward"
		} else if deleteWord {
			inputType = "deleteWordBackward"
		}
	}
	if !forward {
		// A caret immediately before the first visible character can sit after
		// leading collapsible whitespace in the underlying text node. Browsers
		// remove that invisible prefix without dispatching Input Events; word
		// delete shortcuts share this boundary.
		atRenderedStart, boundaryErr := rt.Evaluate(`(function(){
			var selection=getSelection();if(!selection||!selection.isCollapsed)return false;
			var node=selection.anchorNode,offset=selection.anchorOffset;
			if(!node||node.nodeType!==3||offset<0||offset>node.data.length)return false;
			if(offset===0)return false;
			if(!/^[\t\n\f\r ]*$/.test(node.data.slice(0,offset)))return false;
			if(offset===node.data.length||/^[\t\n\f\r ]/.test(node.data.slice(offset)))return false;
			var whiteSpace="normal";try{whiteSpace=String(getComputedStyle(node.parentNode).whiteSpace||"normal").toLowerCase()}catch(_){}
			if(/^(pre|pre-wrap|break-spaces)$/.test(whiteSpace)||String(node.parentNode.localName||"").toLowerCase()==="pre")return false;
			var host=document.activeElement;if(!host||!host.isContentEditable)return false;
			var prefix=document.createRange();prefix.setStart(host,0);prefix.setEnd(node,offset);
			var prefixText=prefix.toString();return prefixText===node.data.slice(0,offset)&&/^[\t\n\f\r ]+$/.test(prefixText)?"true":"false";
		})()`)
		if boundaryErr == nil && atRenderedStart == "true" {
			removed, removeErr := rt.EvaluateUserInput(`(function(){var selection=getSelection(),node=selection&&selection.anchorNode,offset=selection&&selection.anchorOffset;if(!node||node.nodeType!==3||offset<=0||offset>node.data.length||!/^[\t\n\f\r ]+$/.test(node.data.slice(0,offset)))return false;node.data=node.data.slice(offset);selection.collapse(node,0);return true})()`, inputType)
			return removeErr == nil && removed == "true"
		}
	}
	if !rt.DispatchTrustedInputEvent(host, nil, inputType) {
		return false
	}
	if !forward && !deleteWord {
		captionPlaceholder, captionErr := rt.Evaluate(`(function(){var sel=getSelection();if(!sel||!sel.isCollapsed)return false;var caption=sel.anchorNode;if(!caption||caption.nodeType!==1||String(caption.localName||"").toLowerCase()!=="caption"||sel.anchorOffset!==0)return false;var hasBreak=false;for(var child=caption.firstChild;child;child=child.nextSibling){if(child.nodeType===3){if(child.data.trim())return false;continue}if(child.nodeType!==1)return false;if(String(child.localName||"").toLowerCase()==="br"){hasBreak=true;continue}if(child.textContent)return false}return hasBreak})()`)
		if captionErr == nil && captionPlaceholder == "true" {
			return false
		}
	}
	if !deleteWord && !softLine {
		definitionListRootJoinScript := `(function(){
			var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;if(!node||node.nodeType!==3)return false;
			var firstText=function(root){if(root.nodeType===3)return root;for(var child=root.firstChild;child;child=child.nextSibling){var found=firstText(child);if(found)return found}return null};
			var lastText=function(root){if(root.nodeType===3)return root;for(var child=root.lastChild;child;child=child.previousSibling){var found=lastText(child);if(found)return found}return null};
			var item=node.parentNode;while(item&&item!==host&&!/^(li|dt|dd)$/i.test(String(item.localName||"")))item=item.parentNode;if(!item||item===host||!item.parentNode)return false;
			var list=item.parentNode,listName=String(list.localName||"").toLowerCase(),sourceItem,destinationItem,sourceList,destinationList;
			if(` + strconv.FormatBool(forward) + `){
				if(String(item.localName||"").toLowerCase()!=="li"||(listName!=="ul"&&listName!=="ol")||lastText(item)!==node||offset!==node.data.length)return false;
				destinationItem=item;destinationList=list;sourceList=list.nextSibling;while(sourceList&&sourceList.nodeType===3&&!sourceList.data.trim())sourceList=sourceList.nextSibling;
				if(!sourceList||String(sourceList.localName||"").toLowerCase()!=="dl")return false;sourceItem=sourceList.firstElementChild;
			}else{
				if(!/^(dt|dd)$/i.test(String(item.localName||""))||listName!=="dl"||list.firstElementChild!==item||firstText(item)!==node||offset!==0)return false;
				sourceItem=item;sourceList=list;destinationList=list.previousSibling;while(destinationList&&destinationList.nodeType===3&&!destinationList.data.trim())destinationList=destinationList.previousSibling;
				if(!destinationList||!(/^(ul|ol)$/i.test(String(destinationList.localName||""))))return false;destinationItem=destinationList.lastElementChild;
			}
			if(!sourceItem||!destinationItem||!/^(dt|dd)$/i.test(String(sourceItem.localName||""))||String(destinationItem.localName||"").toLowerCase()!=="li"||sourceItem.isContentEditable===false||destinationItem.isContentEditable===false)return false;
			var textNode=lastText(destinationItem);if(!textNode)return false;
			var preserve=function(el){try{return /^(pre|pre-wrap|break-spaces)$/.test(String(getComputedStyle(el).whiteSpace||"normal").toLowerCase())}catch(_){return String(el.localName||"").toLowerCase()==="pre"}};
			var sourceText=firstText(sourceItem);if(!sourceText)return false;
			if(!preserve(sourceItem)&&!preserve(destinationItem)){textNode.data=textNode.data.replace(/[\t\n\f\r ]+$/g,"");sourceText.data=sourceText.data.replace(/^[\t\n\f\r ]+/,"")}
			while(sourceItem.firstChild){var child=sourceItem.firstChild,tail=destinationItem.lastChild;if(tail&&tail.nodeType===3&&child.nodeType===3){tail.data+=child.data;sourceItem.removeChild(child)}else destinationItem.appendChild(child)}
			sourceList.removeChild(sourceItem);var hasDefinition=false;for(var remaining=sourceList.firstElementChild;remaining;remaining=remaining.nextElementSibling)if(/^(dt|dd)$/i.test(String(remaining.localName||""))){hasDefinition=true;break}if(!hasDefinition&&sourceList.parentNode)sourceList.parentNode.removeChild(sourceList);
			var caretText=lastText(destinationItem),range=document.createRange();if(caretText){range.setStart(caretText,caretText.data.length);range.collapse(true);sel.removeAllRanges();sel.addRange(range)}else sel.collapse(destinationItem,destinationItem.childNodes.length);return true
		})()`
		if joined, joinErr := rt.EvaluateUserInput(definitionListRootJoinScript, inputType); joinErr == nil && joined == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		definitionListToListJoinScript := `(function(){
			var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;if(!node||node.nodeType!==3)return false;
			var firstText=function(root){if(root.nodeType===3)return root;for(var child=root.firstChild;child;child=child.nextSibling){var found=firstText(child);if(found)return found}return null};
			var lastText=function(root){if(root.nodeType===3)return root;for(var child=root.lastChild;child;child=child.previousSibling){var found=lastText(child);if(found)return found}return null};
			var item=node.parentNode;while(item&&item!==host&&!/^(li|dt|dd)$/i.test(String(item.localName||"")))item=item.parentNode;if(!item||item===host||!item.parentNode)return false;
			var list=item.parentNode,listName=String(list.localName||"").toLowerCase(),definitionItem,listItem,definitionList,otherList;
			if(` + strconv.FormatBool(forward) + `){
				if(!/^(dt|dd)$/i.test(String(item.localName||""))||listName!=="dl"||list.lastElementChild!==item||lastText(item)!==node||offset!==node.data.length)return false;
				definitionItem=item;definitionList=list;otherList=list.nextSibling;while(otherList&&otherList.nodeType===3&&!otherList.data.trim())otherList=otherList.nextSibling;
				if(!otherList||!/^(ul|ol)$/i.test(String(otherList.localName||"")))return false;listItem=otherList.firstElementChild;
			}else{
				if(String(item.localName||"").toLowerCase()!=="li"||(listName!=="ul"&&listName!=="ol")||list.firstElementChild!==item||firstText(item)!==node||offset!==0)return false;
				listItem=item;otherList=list;definitionList=list.previousSibling;while(definitionList&&definitionList.nodeType===3&&!definitionList.data.trim())definitionList=definitionList.previousSibling;
				if(!definitionList||String(definitionList.localName||"").toLowerCase()!=="dl")return false;definitionItem=definitionList.lastElementChild;
			}
			if(!listItem||String(listItem.localName||"").toLowerCase()!=="li"||!definitionItem||!/^(dt|dd)$/i.test(String(definitionItem.localName||""))||listItem.isContentEditable===false||definitionItem.isContentEditable===false)return false;
			var destinationText=lastText(definitionItem),sourceText=firstText(listItem);if(!destinationText||!sourceText)return false;
			var preserve=function(el){try{return /^(pre|pre-wrap|break-spaces)$/.test(String(getComputedStyle(el).whiteSpace||"normal").toLowerCase())}catch(_){return String(el.localName||"").toLowerCase()==="pre"}};
			if(!preserve(definitionItem)&&!preserve(listItem)){destinationText.data=destinationText.data.replace(/[\t\n\f\r ]+$/g,"");sourceText.data=sourceText.data.replace(/^[\t\n\f\r ]+/,"")}
			while(listItem.firstChild){var child=listItem.firstChild,tail=definitionItem.lastChild;if(tail&&tail.nodeType===3&&child.nodeType===3){tail.data+=child.data;listItem.removeChild(child)}else definitionItem.appendChild(child)}
			otherList.removeChild(listItem);var hasListItem=false;for(var remaining=otherList.firstElementChild;remaining;remaining=remaining.nextElementSibling)if(String(remaining.localName||"").toLowerCase()==="li"){hasListItem=true;break}if(!hasListItem&&otherList.parentNode)otherList.parentNode.removeChild(otherList);
			var caretText=lastText(definitionItem),range=document.createRange();if(caretText){range.setStart(caretText,caretText.data.length);range.collapse(true);sel.removeAllRanges();sel.addRange(range)}else sel.collapse(definitionItem,definitionItem.childNodes.length);return true
		})()`
		if joined, joinErr := rt.EvaluateUserInput(definitionListToListJoinScript, inputType); joinErr == nil && joined == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		definitionListJoinScript := `(function(){
			var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;if(!node||node.nodeType!==3)return false;
			var item=node.parentNode;while(item&&item!==host&&!/^(dt|dd)$/i.test(String(item.localName||"")))item=item.parentNode;
			if(!item||item===host||!item.parentNode||String(item.parentNode.localName||"").toLowerCase()!=="dl")return false;
			var currentList=item.parentNode,firstText=function(root){if(root.nodeType===3)return root;for(var child=root.firstChild;child;child=child.nextSibling){var found=firstText(child);if(found)return found}return null},lastText=function(root){if(root.nodeType===3)return root;for(var child=root.lastChild;child;child=child.previousSibling){var found=lastText(child);if(found)return found}return null};
			var sourceList,sourceItem,destinationItem;
			if(` + strconv.FormatBool(forward) + `){
				if(lastText(item)!==node||offset!==node.data.length||currentList.lastElementChild!==item)return false;
				sourceList=currentList.nextSibling;while(sourceList&&sourceList.nodeType===3&&!sourceList.data.trim())sourceList=sourceList.nextSibling;
				if(!sourceList||String(sourceList.localName||"").toLowerCase()!=="dl")return false;
				sourceItem=sourceList.firstElementChild;destinationItem=item;
			}else{
				if(firstText(item)!==node||offset!==0||currentList.firstElementChild!==item)return false;
				sourceList=currentList.previousSibling;while(sourceList&&sourceList.nodeType===3&&!sourceList.data.trim())sourceList=sourceList.previousSibling;
				if(!sourceList||String(sourceList.localName||"").toLowerCase()!=="dl")return false;
				sourceItem=item;destinationItem=sourceList.lastElementChild;
			}
			if(!sourceItem||!destinationItem||!/^(dt|dd)$/i.test(String(sourceItem.localName||""))||!/^(dt|dd)$/i.test(String(destinationItem.localName||""))||sourceItem.isContentEditable===false||destinationItem.isContentEditable===false)return false;
			var preserve=function(el){try{return /^(pre|pre-wrap|break-spaces)$/.test(String(getComputedStyle(el).whiteSpace||"normal").toLowerCase())}catch(_){return String(el.localName||"").toLowerCase()==="pre"}};
			var destinationText=lastText(destinationItem),sourceText=firstText(sourceItem);if(!destinationText||!sourceText)return false;
			if(!preserve(destinationItem)&&!preserve(sourceItem)){destinationText.data=destinationText.data.replace(/[\t\n\f\r ]+$/g,"");sourceText.data=sourceText.data.replace(/^[\t\n\f\r ]+/,"")}
			while(sourceItem.firstChild){var child=sourceItem.firstChild;if(destinationItem.lastChild&&destinationItem.lastChild.nodeType===3&&child.nodeType===3){destinationItem.lastChild.data+=child.data;sourceItem.removeChild(child)}else destinationItem.appendChild(child)}
			sourceList.removeChild(sourceItem);if(!sourceList.querySelector("dt,dd")&&sourceList.parentNode)sourceList.parentNode.removeChild(sourceList);
			var caretText=lastText(destinationItem),caret=document.createRange();if(!caretText)return false;caret.setStart(caretText,caretText.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret);return true
		})()`
		if joined, joinErr := rt.EvaluateUserInput(definitionListJoinScript, inputType); joinErr == nil && joined == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
	}
	emptyListDeleteScript := `(function(){var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;var item=sel.anchorNode;while(item&&item!==host&&String(item.localName||"").toLowerCase()!=="li")item=item.parentNode;if(!item||item===host||!item.parentNode)return false;var list=item.parentNode,name=String(list.localName||"").toLowerCase();if(name!=="ul"&&name!=="ol")return false;var hasBreak=false;for(var child=item.firstChild;child;child=child.nextSibling){if(child.nodeType===3&&!child.data.trim())continue;if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br"){hasBreak=true;continue}return false}if(!hasBreak||list.childNodes.length!==1)return false;var owner=list.parentNode;if(!owner)return false;var index=Array.prototype.indexOf.call(owner.childNodes,list);owner.removeChild(list);var ownerName=String(owner.localName||"").toLowerCase();if(ownerName==="ul"||ownerName==="ol"){var placeholder=document.createElement("li");placeholder.appendChild(document.createElement("br"));owner.insertBefore(placeholder,owner.childNodes[index]||null)}else if(ownerName==="li"&&!owner.firstChild)owner.appendChild(document.createElement("br"));else if(owner.isContentEditable&&!owner.firstChild)owner.appendChild(document.createElement("br"));sel.collapse(owner,index);return true})()`
	if removed, removeErr := rt.EvaluateUserInput(emptyListDeleteScript, inputType); removeErr == nil && removed == "true" {
		if host.IsConnected() {
			rt.DispatchTrustedInput(host, nil, inputType, false)
		}
		return true
	}
	if forward && !deleteWord && !softLine {
		removeEmptyPlaceholderBeforeNonEditableBlockScript := `(function(){
			var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;if(!node)return false;
			var blocks={address:1,blockquote:1,dd:1,div:1,dl:1,dt:1,li:1,p:1,pre:1};
			var block=node.nodeType===3?node.parentNode:node;while(block&&block!==host&&!blocks[String(block.localName||"").toLowerCase()])block=block.parentNode;
			if(!block||block===host||offset!==0)return false;
			var prefix=document.createRange();prefix.setStart(block,0);prefix.setEnd(node,offset);if(prefix.toString()!=="")return false;
			var previous=block.previousSibling;while(previous&&previous.nodeType===3&&!previous.data.trim())previous=previous.previousSibling;
			if(!previous||previous.nodeType!==1||previous.isContentEditable!==false)return false;
			var empty=previous.previousSibling;while(empty&&empty.nodeType===3&&!empty.data.trim())empty=empty.previousSibling;
			if(!empty||empty.parentNode!==block.parentNode||empty.nodeType!==1||!blocks[String(empty.localName||"").toLowerCase()]||empty.isContentEditable===false)return false;
			var hasBreak=false,onlyPlaceholder=true;var inspect=function(root){for(var child=root.firstChild;child;child=child.nextSibling){if(child.nodeType===3){if(child.data.trim())return false;continue}if(child.nodeType!==1)return false;if(String(child.localName||"").toLowerCase()==="br"){hasBreak=true;continue}if(child.isContentEditable===false||!inspect(child))return false}return true};
			if(!inspect(empty)||!hasBreak||!onlyPlaceholder)return false;
			var parent=empty.parentNode,index=Array.prototype.indexOf.call(parent.childNodes,empty);parent.removeChild(empty);sel.collapse(parent,index);return true
		})()`
		if removed, removeErr := rt.EvaluateUserInput(removeEmptyPlaceholderBeforeNonEditableBlockScript, inputType); removeErr == nil && removed == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
	}
	if forward && !deleteWord && !softLine {
		mergeListItemLineBreakScript := `(function(){var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;var node=sel.anchorNode,offset=sel.anchorOffset,item=node;while(item&&item!==host&&String(item.localName||"").toLowerCase()!=="li")item=item.parentNode;if(!item||item===host||!item.parentNode)return false;var list=item.parentNode,name=String(list.localName||"").toLowerCase();if(name!=="ul"&&name!=="ol")return false;var rest=document.createRange();rest.setStart(node,offset);rest.setEnd(item,item.childNodes.length);if(rest.toString()!=="")return false;var next=item.nextElementSibling;if(!next||String(next.localName||"").toLowerCase()!=="li"||next.isContentEditable===false)return false;var allowed=function(el){for(var c=el.firstChild;c;c=c.nextSibling)if(c.nodeType!==3&&!(c.nodeType===1&&String(c.localName||"").toLowerCase()==="br"))return false;return true};if(!allowed(item)||!allowed(next))return false;var br=null;for(var child=next.firstChild;child;child=child.nextSibling)if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br"){br=child;break}if(!br||br===next.firstChild||!br.nextSibling)return false;var tail=item.lastChild;if(tail&&tail.nodeType===1&&String(tail.localName||"").toLowerCase()==="br")item.removeChild(tail);while(next.firstChild&&next.firstChild!==br){var leading=next.firstChild;if(item.lastChild&&item.lastChild.nodeType===3&&leading.nodeType===3){item.lastChild.data+=leading.data;next.removeChild(leading)}else item.appendChild(leading)}next.removeChild(br);var text=item.lastChild;if(text&&text.nodeType===3){var caret=document.createRange();caret.setStart(text,text.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret)}return true})()`
		if merged, mergeErr := rt.EvaluateUserInput(mergeListItemLineBreakScript, inputType); mergeErr == nil && merged == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		mergeListItemScript := `(function(){var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;var node=sel.anchorNode,offset=sel.anchorOffset,item=node;while(item&&item!==host&&String(item.localName||"").toLowerCase()!=="li")item=item.parentNode;if(!item||item===host||!item.parentNode)return false;var list=item.parentNode,name=String(list.localName||"").toLowerCase();if(name!=="ul"&&name!=="ol")return false;var rest=document.createRange();rest.setStart(node,offset);rest.setEnd(item,item.childNodes.length);if(rest.toString()!=="")return false;var next=item.nextElementSibling;if(!next||String(next.localName||"").toLowerCase()!=="li"||next.isContentEditable===false)return false;var onlyText=function(el){for(var child=el.firstChild;child;child=child.nextSibling){if(child.nodeType===3)continue;if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br")continue;return false}return true};var block=item.childNodes.length===1?item.firstChild:null;if(block&&block.nodeType===1&&String(block.localName||"").toLowerCase()==="p"&&onlyText(block)&&onlyText(next)){var blockTail=block.lastChild;if(blockTail&&blockTail.nodeType===1&&String(blockTail.localName||"").toLowerCase()==="br"){block.removeChild(blockTail);blockTail=block.lastChild}while(next.firstChild){var child=next.firstChild;if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br"){next.removeChild(child);continue}if(blockTail&&blockTail.nodeType===3&&child.nodeType===3){blockTail.data+=child.data;next.removeChild(child)}else{block.appendChild(child);blockTail=child}}list.removeChild(next);var blockText=block.lastChild;if(blockText&&blockText.nodeType===3){var blockCaret=document.createRange();blockCaret.setStart(blockText,blockText.data.length);blockCaret.collapse(true);sel.removeAllRanges();sel.addRange(blockCaret)}return true}var nextBlock=next.childNodes.length===1?next.firstChild:null;if(onlyText(item)&&nextBlock&&nextBlock.nodeType===1&&String(nextBlock.localName||"").toLowerCase()==="p"&&onlyText(nextBlock)){var itemTail=item.lastChild;if(itemTail&&itemTail.nodeType===1&&String(itemTail.localName||"").toLowerCase()==="br"){item.removeChild(itemTail);itemTail=item.lastChild}while(nextBlock.firstChild){var child=nextBlock.firstChild;if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br"){nextBlock.removeChild(child);continue}if(itemTail&&itemTail.nodeType===3&&child.nodeType===3){itemTail.data+=child.data;nextBlock.removeChild(child)}else{item.appendChild(child);itemTail=child}}list.removeChild(next);var itemText=item.lastChild;if(itemText&&itemText.nodeType===3){var itemCaret=document.createRange();itemCaret.setStart(itemText,itemText.data.length);itemCaret.collapse(true);sel.removeAllRanges();sel.addRange(itemCaret)}return true}if(!onlyText(item)||!onlyText(next))return false;var tail=item.lastChild;if(tail&&tail.nodeType===1&&String(tail.localName||"").toLowerCase()==="br")item.removeChild(tail);var head=next.firstChild;if(head&&head.nodeType===1&&String(head.localName||"").toLowerCase()==="br")next.removeChild(head);while(next.firstChild)item.appendChild(next.firstChild);list.removeChild(next);var text=item.lastChild;if(text&&text.nodeType===3){var caret=document.createRange();caret.setStart(text,text.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret)}else sel.collapse(item,item.childNodes.length);return true})()`
		if merged, mergeErr := rt.EvaluateUserInput(mergeListItemScript, inputType); mergeErr == nil && merged == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
	}
	if !forward && !deleteWord {
		joinPreviousLineBreakScript := `(function(){
			var host=document.activeElement,sel=getSelection();
			if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;if(!node||node.nodeType!==3||!node.parentNode)return false;
			if(node.data.slice(0,offset).trim()!=="")return false;
			var parent=node.parentNode,previous=node.previousSibling,trimBeforeBreak=false;
			if(!previous||previous.nodeType!==1||!/^(br|hr)$/i.test(String(previous.localName||"")))return false;
			var br=previous;
			while(br&&br.nodeType===1&&/^(br|hr)$/i.test(String(br.localName||""))){if(String(br.localName||"").toLowerCase()==="hr")trimBeforeBreak=true;var before=br.previousSibling;br.parentNode.removeChild(br);br=before}
			while(br&&br.nodeType===3&&!br.data.trim()){var beforeSpace=br.previousSibling;br.parentNode.removeChild(br);br=beforeSpace}
			node.data=node.data.replace(/^[\t\n\f\r ]+/,"");
			if(br&&br.nodeType===3){if(trimBeforeBreak)br.data=br.data.replace(/[\t\n\f\r ]+$/g,"");br.data+=node.data;parent.removeChild(node);node=br;offset=node.data.length}
			var caret=document.createRange();caret.setStart(node,offset);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret);return true
		})()`
		if joined, joinErr := rt.EvaluateUserInput(joinPreviousLineBreakScript, inputType); joinErr == nil && joined == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		removeEmptyParagraphScript := `(function(){
			var host=document.activeElement,sel=getSelection();
			if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;if(!node)return false;
			var blockNames={address:1,blockquote:1,dd:1,div:1,dt:1,li:1,p:1,pre:1};
			var block=node.nodeType===3?node.parentNode:node;while(block&&block!==host&&!blockNames[String(block.localName||"").toLowerCase()])block=block.parentNode;
			if(!block||block===host||!block.parentNode)return false;
			var prefix=document.createRange();prefix.setStart(block,0);prefix.setEnd(node,offset);if(prefix.toString()!=="")return false;
			var hasPlaceholder=false;
			var empty=function(root){for(var child=root.firstChild;child;child=child.nextSibling){if(child.nodeType===3){if(child.data.trim())return false;continue}if(child.nodeType!==1)return false;var name=String(child.localName||"").toLowerCase();if(name==="br"){hasPlaceholder=true;continue}if(child.isContentEditable===false||!empty(child))return false}return true};
			if(!empty(block)||!hasPlaceholder)return false;
			var previous=block.previousSibling;while(previous&&previous.nodeType===3&&!previous.data.trim())previous=previous.previousSibling;
			if(!previous||previous.nodeType!==1||!blockNames[String(previous.localName||"").toLowerCase()])return false;
			var parent=block.parentNode,index=Array.prototype.indexOf.call(parent.childNodes,block);parent.removeChild(block);
			var caret=document.createRange();if(previous.lastChild&&previous.lastChild.nodeType===3)caret.setStart(previous.lastChild,previous.lastChild.data.length);else caret.setStart(parent,index);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret);return true
		})()`
		if removed, removeErr := rt.EvaluateUserInput(removeEmptyParagraphScript, inputType); removeErr == nil && removed == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		joinListBoundaryScript := `(function(){
			var host=document.activeElement,sel=getSelection();
			if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;
			if(!node||node.nodeType!==3)return false;
			var listName=function(el){return el&&el.nodeType===1&&/^(ul|ol)$/i.test(String(el.localName||""))};
			var blockName=function(el){return el&&el.nodeType===1&&/^(address|blockquote|dd|div|dl|dt|li|p|pre)$/i.test(String(el.localName||""))};
			var firstText=function(root){if(!root)return null;if(root.nodeType===3)return root;for(var c=root.firstChild;c;c=c.nextSibling){var found=firstText(c);if(found)return found}return null};
			var lastText=function(root){if(!root)return null;if(root.nodeType===3)return root;for(var c=root.lastChild;c;c=c.previousSibling){var found=lastText(c);if(found)return found}return null};
			var whiteSpace=function(el){try{return String(getComputedStyle(el).whiteSpace||"normal").toLowerCase()}catch(_){return String(el.localName||"").toLowerCase()==="pre"?"pre":"normal"}};
			var preserve=function(el){return /^(pre|pre-wrap|break-spaces)$/.test(whiteSpace(el))||String(el.localName||"").toLowerCase()==="pre"};
			var item=node.parentNode;while(item&&item!==host&&String(item.localName||"").toLowerCase()!=="li")item=item.parentNode;
			if(item&&item!==host&&item.parentNode&&listName(item.parentNode)){
				var list=item.parentNode;if(list.firstElementChild!==item)return false;
				var prefix=document.createRange();prefix.setStart(item,0);prefix.setEnd(node,offset);if(prefix.toString().trim()!=="")return false;
				var previous=list.previousSibling;while(previous&&previous.nodeType===3&&!previous.data.trim())previous=previous.previousSibling;
				if(!previous||previous.isContentEditable===false||blockName(previous))return false;
				var leading=firstText(item),itemTail=lastText(item),trailing=lastText(previous);if(!leading||!itemTail||!trailing)return false;
				var following=list.nextSibling;while(following&&following.nodeType===3&&!following.data.trim())following=following.nextSibling;
				if(!preserve(item)&&!preserve(previous)){leading.data=leading.data.replace(/^[\t\n\f\r ]+/g,"");itemTail.data=itemTail.data.replace(/[\t\n\f\r ]+$/g,"");trailing.data=trailing.data.replace(/[\t\n\f\r ]+$/g,"");var afterText=firstText(following);if(afterText)afterText.data=afterText.data.replace(/^[\t\n\f\r ]+/,"")}
				while(item.firstChild)list.parentNode.insertBefore(item.firstChild,list);
				list.removeChild(item);if(!list.querySelector("li"))list.parentNode.removeChild(list);
				var caretText=lastText(previous),caret=document.createRange();if(!caretText)return false;caret.setStart(caretText,caretText.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret);return true
			}
			if(node.data.slice(0,offset).trim()!=="")return false;
			var previousList=node.previousSibling;while(previousList&&previousList.nodeType===3&&!previousList.data.trim())previousList=previousList.previousSibling;
			if(!listName(previousList))return false;
			var lastItem=previousList.lastElementChild;if(!lastItem||String(lastItem.localName||"").toLowerCase()!=="li"||lastItem.isContentEditable===false)return false;
			var previousText=lastText(lastItem);if(!previousText)return false;
			if(!preserve(lastItem)){previousText.data=previousText.data.replace(/[\t\n\f\r ]+$/g,"");node.data=node.data.replace(/^[\t\n\f\r ]+/,"")}
			previousText.data+=node.data;if(node.parentNode)node.parentNode.removeChild(node);
			var caret=document.createRange();caret.setStart(previousText,previousText.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret);return true
		})()`
		if joined, joinErr := rt.EvaluateUserInput(joinListBoundaryScript, inputType); joinErr == nil && joined == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		inlineDeleteScript := `(function(){var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;var node=sel.anchorNode,offset=sel.anchorOffset;if(!node||node.nodeType!==3)return false;var parent=node.parentNode,candidate=null,index=-1;if(offset===0&&node.previousSibling&&node.previousSibling.nodeType===1){candidate=node.previousSibling;index=Array.prototype.indexOf.call(parent.childNodes,candidate)}else if(offset===node.data.length&&node.data.length===1){var inline=parent;while(inline&&inline!==host&&inline.parentNode!==host&&/^(span|b|i|u|em|strong|small|code|font|mark|sub|sup|s|strike)$/.test(String(inline.localName||"").toLowerCase())&&inline.textContent===node.data){candidate=inline;parent=inline.parentNode;index=Array.prototype.indexOf.call(parent.childNodes,inline);break}}if(!candidate||index<0||candidate.isContentEditable===false)return false;var name=String(candidate.localName||"").toLowerCase();if(name!=="br"&&name!=="hr"&&name!=="img"&&name!=="span"&&name!=="b"&&name!=="i"&&name!=="u"&&name!=="em"&&name!=="strong"&&name!=="small"&&name!=="code"&&name!=="font"&&name!=="mark"&&name!=="sub"&&name!=="sup"&&name!=="s"&&name!=="strike")return false;parent.removeChild(candidate);sel.collapse(parent,index);return true})()`
		inlineDeleted, inlineErr := rt.EvaluateUserInput(inlineDeleteScript, inputType)
		if inlineErr == nil && inlineDeleted == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		mergeListItemLineBreakScript := `(function(){var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;var node=sel.anchorNode,offset=sel.anchorOffset,item=node;while(item&&item!==host&&String(item.localName||"").toLowerCase()!=="li")item=item.parentNode;if(!item||item===host||!item.parentNode||node.nodeType!==3||offset!==0)return false;var list=item.parentNode,name=String(list.localName||"").toLowerCase();if(name!=="ul"&&name!=="ol")return false;var prefix=document.createRange();prefix.setStart(item,0);prefix.setEnd(node,offset);if(prefix.toString()!=="")return false;var previous=item.previousElementSibling;if(!previous||String(previous.localName||"").toLowerCase()!=="li"||previous.isContentEditable===false)return false;var allowed=function(el){for(var c=el.firstChild;c;c=c.nextSibling)if(c.nodeType!==3&&!(c.nodeType===1&&String(c.localName||"").toLowerCase()==="br"))return false;return true};if(!allowed(item))return false;var br=null;for(var child=item.firstChild;child;child=child.nextSibling)if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br"){br=child;break}if(!br||br===item.firstChild||!br.nextSibling)return false;var target=previous; if(previous.childNodes.length===1&&previous.firstChild.nodeType===1&&String(previous.firstChild.localName||"").toLowerCase()==="p")target=previous.firstChild;if(!allowed(target))return false;var tail=target.lastChild;while(item.firstChild&&item.firstChild!==br){var leading=item.firstChild;if(tail&&tail.nodeType===3&&leading.nodeType===3){tail.data+=leading.data;item.removeChild(leading)}else{target.appendChild(leading);tail=leading}}item.removeChild(br);var text=target.lastChild;if(text&&text.nodeType===3){var caret=document.createRange();caret.setStart(text,text.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret)}return true})()`
		if merged, mergeErr := rt.EvaluateUserInput(mergeListItemLineBreakScript, inputType); mergeErr == nil && merged == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		mergeNestedListParagraphScript := `(function(){var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;var node=sel.anchorNode,offset=sel.anchorOffset,item=node;while(item&&item!==host&&String(item.localName||"").toLowerCase()!=="li")item=item.parentNode;if(!item||item===host||!item.parentNode||offset!==0||node.nodeType!==3)return false;var list=item.parentNode,name=String(list.localName||"").toLowerCase();if(name!=="ul"&&name!=="ol")return false;var prefix=document.createRange();prefix.setStart(item,0);prefix.setEnd(node,offset);if(prefix.toString()!=="")return false;var previous=item.previousElementSibling;if(!previous||String(previous.localName||"").toLowerCase()!=="li"||previous.isContentEditable===false)return false;var paragraph=previous.childNodes.length===1?previous.firstChild:null;if(!paragraph||paragraph.nodeType!==1||String(paragraph.localName||"").toLowerCase()!=="p")return false;var onlyText=function(el){for(var child=el.firstChild;child;child=child.nextSibling){if(child.nodeType===3)continue;if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br")continue;return false}return true};if(!onlyText(item)||!onlyText(paragraph))return false;var target=paragraph.lastChild;while(item.firstChild){var child=item.firstChild;if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br"){item.removeChild(child);continue}if(target&&target.nodeType===3&&child.nodeType===3){target.data+=child.data;item.removeChild(child)}else{paragraph.appendChild(child);target=child}}list.removeChild(item);if(target&&target.nodeType===3){var caret=document.createRange();caret.setStart(target,target.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret)}return true})()`
		if merged, mergeErr := rt.EvaluateUserInput(mergeNestedListParagraphScript, inputType); mergeErr == nil && merged == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		mergeScript := `(function(){var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;var node=sel.anchorNode,offset=sel.anchorOffset,blockNames={address:1,blockquote:1,dd:1,div:1,dl:1,dt:1,li:1,p:1,pre:1};var block=node&&node.nodeType===3?node.parentNode:node;while(block&&block!==host&&!blockNames[String(block.localName||"").toLowerCase()])block=block.parentNode;if(!block||block===host)return false;var emptyItem=block;while(emptyItem&&emptyItem!==host&&String(emptyItem.localName||"").toLowerCase()!=="li")emptyItem=emptyItem.parentNode;if(emptyItem&&emptyItem!==host&&emptyItem.parentNode&&(String(emptyItem.parentNode.localName||"").toLowerCase()==="ul"||String(emptyItem.parentNode.localName||"").toLowerCase()==="ol")){var emptyList=emptyItem.parentNode,onlyItem=true,hasBreak=false;for(var itemChild=emptyItem.firstChild;itemChild;itemChild=itemChild.nextSibling){if(itemChild.nodeType===3&&!itemChild.data.trim())continue;if(itemChild.nodeType===1&&String(itemChild.localName||"").toLowerCase()==="br"){hasBreak=true;continue}onlyItem=false;break}if(onlyItem&&hasBreak&&emptyList.childNodes.length===1){var listOwner=emptyList.parentNode;if(listOwner){var listIndex=Array.prototype.indexOf.call(listOwner.childNodes,emptyList);listOwner.removeChild(emptyList);if(String(listOwner.localName||"").toLowerCase()==="li"&&!listOwner.firstChild)listOwner.appendChild(document.createElement("br"));sel.collapse(listOwner,listIndex);return true}}}var prefix=document.createRange();prefix.setStart(block,0);prefix.setEnd(node,offset);if(prefix.toString().trim()!=="")return false;var previous=null;for(var current=block;current&&current!==host&&!previous;current=current.parentNode){var candidate=current.previousSibling;while(candidate&&candidate.nodeType===3&&!candidate.data.trim())candidate=candidate.previousSibling;if(candidate&&candidate.nodeType===1&&blockNames[String(candidate.localName||"").toLowerCase()])previous=candidate}if(!previous||previous.nodeType!==1||!blockNames[String(previous.localName||"").toLowerCase()])return false;var preserve=function(el){try{return /^(pre|pre-wrap|break-spaces)$/.test(getComputedStyle(el).whiteSpace)}catch(_){return el.localName==="pre"}};var trailing=previous.lastChild;if(trailing&&trailing.nodeType===1&&trailing.localName==="br")previous.removeChild(trailing);var oldText=previous.lastChild;if(oldText&&oldText.nodeType===3&&!preserve(previous))oldText.data=oldText.data.replace(/[ \t\r\n\f]+$/g,"");var firstText=block.firstChild;if(oldText&&oldText.nodeType===3&&firstText&&firstText.nodeType===3&&preserve(block)&&!preserve(previous)){var leadingRun=firstText.data.match(/^[ \t\r\n\f]+/);if(leadingRun){firstText.data=firstText.data.slice(leadingRun[0].length);oldText.data+=String.fromCharCode(160).repeat(Math.max(0,leadingRun[0].length-1))+(leadingRun[0].length>0?" ":"")}}if(firstText&&firstText.nodeType===3&&!preserve(block))firstText.data=firstText.data.replace(/^[ \t\r\n\f]+/g,"");if(oldText&&oldText.nodeType===3){while(block.firstChild&&block.firstChild.nodeType===3){var first=block.firstChild;oldText.data+=first.data;block.removeChild(first)}}while(block.firstChild)previous.appendChild(block.firstChild);var empty=block.parentNode;if(empty)empty.removeChild(block);while(empty&&empty!==host&&!empty.firstChild){var owner=empty.parentNode;if(!owner)break;owner.removeChild(empty);empty=owner}var caret=document.createRange();var text=previous.lastChild;if(text&&text.nodeType===3){caret.setStart(text,text.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret)}return true})()`
		inlineBlockJoinScript := `(function(){
			var host=document.activeElement,sel=getSelection();
			if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;
			var names={address:1,blockquote:1,dd:1,div:1,dl:1,dt:1,li:1,p:1,pre:1};
			var block=node&&node.nodeType===3?node.parentNode:node;
			while(block&&block!==host&&!names[String(block.localName||"").toLowerCase()])block=block.parentNode;
			if(!block||block===host||!block.parentNode)return false;
			var prefix=document.createRange();prefix.setStart(block,0);prefix.setEnd(node,offset);
			if(prefix.toString().trim()!=="")return false;
			var parent=block.parentNode,previous=block.previousSibling;
			while(previous&&previous.nodeType===3&&!previous.data.trim())previous=previous.previousSibling;
			if(!previous||previous.nodeType===1&&names[String(previous.localName||"").toLowerCase()]||previous.isContentEditable===false)return false;
			var firstText=function(root){if(!root)return null;if(root.nodeType===3)return root;for(var c=root.firstChild;c;c=c.nextSibling){var found=firstText(c);if(found)return found}return null};
			var lastText=function(root){if(!root)return null;if(root.nodeType===3)return root;for(var c=root.lastChild;c;c=c.previousSibling){var found=lastText(c);if(found)return found}return null};
			var oldText=lastText(previous),leading=firstText(block);
			if(!oldText||!leading)return false;
			var preserve=function(el){try{return /^(pre|pre-wrap|break-spaces)$/.test(getComputedStyle(el.nodeType===3?el.parentNode:el).whiteSpace)}catch(_){return String(el.localName||"").toLowerCase()==="pre"}};
			if(!preserve(previous))oldText.data=oldText.data.replace(/[ \t\r\n\f]+$/g,"");
			if(!preserve(block))leading.data=leading.data.replace(/^[ \t\r\n\f]+/g,"");
			var lastMoved=null;
			while(block.firstChild){var child=block.firstChild;if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br"){block.removeChild(child);break}parent.insertBefore(child,block);lastMoved=child}
			if(!block.firstChild)parent.removeChild(block);
			var caretText=lastText(lastMoved)||oldText,caret=document.createRange();
			caret.setStart(caretText,caretText.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret);
			return true
		})()`
		if merged, mergeErr := rt.EvaluateUserInput(inlineBlockJoinScript, inputType); mergeErr == nil && merged == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		blockInlineJoinScript := `(function(){
			var host=document.activeElement,sel=getSelection();
			if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;
			var names={address:1,blockquote:1,dd:1,div:1,dl:1,dt:1,li:1,p:1,pre:1};
			var root=node,previous=null;
			while(root&&root!==host&&!previous){
				var candidate=root.previousSibling;
				while(candidate&&candidate.nodeType===3&&!candidate.data.trim())candidate=candidate.previousSibling;
				if(candidate&&candidate.nodeType===1&&names[String(candidate.localName||"").toLowerCase()])previous=candidate;
				else root=root.parentNode;
			}
			if(!root||root===host||!previous||root.nodeType===1&&names[String(root.localName||"").toLowerCase()]||previous.isContentEditable===false)return false;
			var prefix=document.createRange();prefix.setStart(root,0);prefix.setEnd(node,offset);
			if(prefix.toString().trim()!=="")return false;
			var firstText=function(el){if(!el)return null;if(el.nodeType===3)return el;for(var c=el.firstChild;c;c=c.nextSibling){var found=firstText(c);if(found)return found}return null};
			var lastText=function(el){if(!el)return null;if(el.nodeType===3)return el;for(var c=el.lastChild;c;c=c.previousSibling){var found=lastText(c);if(found)return found}return null};
			var leading=firstText(root),trailing=lastText(previous);
			if(!leading||!trailing)return false;
			var preserve=function(el){try{return /^(pre|pre-wrap|break-spaces)$/.test(getComputedStyle(el.nodeType===3?el.parentNode:el).whiteSpace)}catch(_){return String(el.localName||"").toLowerCase()==="pre"}};
			if(!preserve(root)&&!preserve(previous)){
				trailing.data=trailing.data.replace(/[ \t\r\n\f]+$/g,"");
				leading.data=leading.data.replace(/^[ \t\r\n\f]+/g,"");
			}
			var parent=root.parentNode,tail=previous.lastChild;
			if(tail&&tail.nodeType===1&&String(tail.localName||"").toLowerCase()==="br"){previous.removeChild(tail);tail=previous.lastChild}
			if(root.nodeType===3&&tail&&tail.nodeType===3){tail.data+=root.data;parent.removeChild(root)}else previous.appendChild(root);
			var caretText=lastText(previous),caret=document.createRange();
			caret.setStart(caretText,caretText.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret);
			return true
		})()`
		if merged, mergeErr := rt.EvaluateUserInput(blockInlineJoinScript, inputType); mergeErr == nil && merged == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		merged, mergeErr := rt.EvaluateUserInput(mergeScript, inputType)
		if mergeErr == nil && merged == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
	}
	if forward && !deleteWord && !softLine {
		joinTrailingListItemInlineScript := `(function(){
			var host=document.activeElement,sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;if(!node||node.nodeType!==3||offset<0||offset>node.data.length||!/^[\t\n\f\r ]*$/.test(node.data.slice(offset)))return false;
			var item=node.parentNode;while(item&&item!==host&&String(item.localName||"").toLowerCase()!=="li")item=item.parentNode;
			if(!item||item===host||!item.parentNode)return false;var list=item.parentNode,listName=String(list.localName||"").toLowerCase();
			if((listName!=="ul"&&listName!=="ol")||list.lastElementChild!==item||item.isContentEditable===false)return false;
			var lastText=function(root){if(root.nodeType===3)return root;for(var child=root.lastChild;child;child=child.previousSibling){var found=lastText(child);if(found)return found}return null};
			if(lastText(item)!==node)return false;var whiteSpace="normal";try{whiteSpace=String(getComputedStyle(item).whiteSpace||"normal").toLowerCase()}catch(_){}
			if(/^(pre|pre-wrap|break-spaces)$/.test(whiteSpace)||String(item.localName||"").toLowerCase()==="pre")return false;
			var following=list.nextSibling,skipped=[];while(following&&following.nodeType===3&&!following.data.trim()){skipped.push(following);following=following.nextSibling}
			var blocks={address:1,article:1,aside:1,blockquote:1,dd:1,div:1,dl:1,dt:1,fieldset:1,figure:1,footer:1,form:1,hr:1,li:1,main:1,nav:1,ol:1,p:1,pre:1,section:1,table:1,ul:1};
			if(!following||following.nodeType===1&&(blocks[String(following.localName||"").toLowerCase()]||following.isContentEditable===false))return false;
			var isText=following.nodeType===3;if(!isText&&following.nodeType!==1)return false;
			var text=isText?following.data.replace(/^[\t\n\f\r ]+/,""):"";if(isText&&!text)return false;
			var safe=function(root){if(root.nodeType===1&&root.isContentEditable===false)return false;for(var child=root.firstChild;child;child=child.nextSibling)if(child.nodeType===1&&blocks[String(child.localName||"").toLowerCase()]||!safe(child))return false;return true};
			if(following.nodeType===1&&!safe(following))return false;
			node.data=node.data.slice(0,offset).replace(/[\t\n\f\r ]+$/g,"");for(var i=0;i<skipped.length;i++)if(skipped[i].parentNode)skipped[i].parentNode.removeChild(skipped[i]);
			if(isText)following.data=text;var previous=item.lastChild;
			if(previous&&previous.nodeType===3&&following.nodeType===3){previous.data+=following.data;following.parentNode.removeChild(following)}else item.appendChild(following);
			var caretText=lastText(item),caret=document.createRange();if(!caretText)return false;caret.setStart(caretText,caretText.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret);return true
		})()`
		if joined, joinErr := rt.EvaluateUserInput(joinTrailingListItemInlineScript, inputType); joinErr == nil && joined == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		joinFollowingListItemScript := `(function(){
			var host=document.activeElement,sel=getSelection();
			if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;
			if(!node||node.nodeType!==3||offset<0||offset>node.data.length||!/^[\t\n\f\r ]*$/.test(node.data.slice(offset)))return false;
			var whiteSpace="normal";try{whiteSpace=String(getComputedStyle(node.parentNode).whiteSpace||"normal").toLowerCase()}catch(_){}
			if(/^(pre|pre-wrap|break-spaces)$/.test(whiteSpace)||String(node.parentNode.localName||"").toLowerCase()==="pre")return false;
			var cursor=node,list=null,owner=null,separators=[],breaks=0;
			while(cursor&&cursor!==host&&!list){
				var candidate=cursor.nextSibling;
				while(candidate&&candidate.nodeType===3&&!candidate.data.trim()){separators.push(candidate);candidate=candidate.nextSibling}
				if(candidate&&candidate.nodeType===1&&String(candidate.localName||"").toLowerCase()==="br"&&breaks===0){separators.push(candidate);breaks++;candidate=candidate.nextSibling;while(candidate&&candidate.nodeType===3&&!candidate.data.trim()){separators.push(candidate);candidate=candidate.nextSibling}}
				if(candidate&&candidate.nodeType===1&&/^(ul|ol)$/.test(String(candidate.localName||"").toLowerCase())){list=candidate;owner=cursor.parentNode;break}
				if(candidate)break;
				cursor=cursor.parentNode;
			}
			if(!list||!owner||!list.firstElementChild||String(list.firstElementChild.localName||"").toLowerCase()!=="li"||list.firstElementChild.isContentEditable===false)return false;
			var item=list.firstElementChild;
			var safe=function(root){for(var child=root.firstChild;child;child=child.nextSibling){if(child.nodeType===3)continue;if(child.nodeType!==1||/^(ul|ol|address|blockquote|dd|div|dl|dt|li|p|pre|table)$/.test(String(child.localName||"").toLowerCase())||child.isContentEditable===false||!safe(child))return false}return true};
			if(!safe(item))return false;
			var firstText=function(root){if(root.nodeType===3)return root;for(var child=root.firstChild;child;child=child.nextSibling){var found=firstText(child);if(found)return found}return null};
			var lastText=function(root){if(root.nodeType===3)return root;for(var child=root.lastChild;child;child=child.previousSibling){var found=lastText(child);if(found)return found}return null};
			var first=firstText(item),last=lastText(item);if(!first||!last)return false;
			var leading=first.data.match(/^[\t\n\f\r ]+/);if(leading)first.data=first.data.slice(leading[0].length);
			var trailing=last.data.match(/[\t\n\f\r ]+$/);if(trailing)last.data=last.data.slice(0,-trailing[0].length);
			if(!item.textContent)return false;
			var start=node.data.slice(0,offset).replace(/[\t\n\f\r ]+$/,"");node.data=start;
			for(var i=0;i<separators.length;i++)if(separators[i].parentNode)separators[i].parentNode.removeChild(separators[i]);
			var parent=list.parentNode,followingAfterList=list.nextSibling;
			while(item.firstChild)parent.insertBefore(item.firstChild,list);
			list.removeChild(item);
			if(!list.firstChild)parent.removeChild(list);
			if(!list.parentNode){var following=followingAfterList;while(following&&following.nodeType===3&&!following.data.trim()){var next=following.nextSibling;parent.removeChild(following);following=next}if(following&&following.nodeType===3)following.data=following.data.replace(/^[\t\n\f\r ]+/,"")}
			var caretText=last,caret=document.createRange();
			if(!caretText)return false;caret.setStart(caretText,caretText.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret);
			return true
		})()`
		if joined, joinErr := rt.EvaluateUserInput(joinFollowingListItemScript, inputType); joinErr == nil && joined == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
		joinFollowingInlineScript := `(function(){
			var host=document.activeElement,sel=getSelection();
			if(!host||!host.isContentEditable||!sel||!sel.isCollapsed)return false;
			var node=sel.anchorNode,offset=sel.anchorOffset;
			if(!node)return false;
			var names={address:1,article:1,aside:1,blockquote:1,dd:1,div:1,dl:1,dt:1,fieldset:1,figcaption:1,figure:1,footer:1,form:1,h1:1,h2:1,h3:1,h4:1,h5:1,h6:1,header:1,hr:1,li:1,main:1,menu:1,nav:1,ol:1,p:1,pre:1,section:1,table:1,tbody:1,td:1,tfoot:1,th:1,thead:1,tr:1,ul:1};
			var block=node.nodeType===3?node.parentNode:node;
			while(block&&block!==host&&!names[String(block.localName||"").toLowerCase()])block=block.parentNode;
			if(!block||block===host)return false;
			var lastText=function(root){if(!root)return null;if(root.nodeType===3)return root;for(var child=root.lastChild;child;child=child.previousSibling){var found=lastText(child);if(found)return found}return null};
			var currentTrailingSpace=false;
			if(node.nodeType===3&&offset>=0&&offset<=node.data.length){currentTrailingSpace=offset<node.data.length&&/^[\t\n\f\r ]*$/.test(node.data.slice(offset))}
			var atBlockTail=node.nodeType===3&&(offset===node.data.length||currentTrailingSpace)&&lastText(block)===node;
			if(node===block&&node.nodeType===1){var hasTrailingBreak=false,tailOnly=true;for(var i=offset;i<block.childNodes.length;i++){var child=block.childNodes[i];if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br"){hasTrailingBreak=true;continue}if(child.nodeType===3&&!child.data.trim())continue;tailOnly=false;break}atBlockTail=hasTrailingBreak&&tailOnly}
			if(!atBlockTail)return false;
			var whiteSpace="normal";try{whiteSpace=String(getComputedStyle(block).whiteSpace||"normal").toLowerCase()}catch(_){}
			if(/^(pre|pre-wrap|break-spaces)$/.test(whiteSpace)||String(block.localName||"").toLowerCase()==="pre")return false;
			var next=block.nextSibling,skipped=[];
			while(next&&next.nodeType===3&&!next.data.trim()){skipped.push(next);next=next.nextSibling}
			if(!next||next.nodeType===1&&(names[String(next.localName||"").toLowerCase()]||String(next.localName||"").toLowerCase()==="br")||next.isContentEditable===false)return false;
			var isInlineText=next.nodeType===3;
			if(!isInlineText&&next.nodeType!==1)return false;
			var nextText=isInlineText?next.data.replace(/^[\t\n\f\r ]+/,""):"";
			if(isInlineText&&!nextText)return false;
			var safe=function(root){if(root.nodeType===1&&root.isContentEditable===false)return false;for(var child=root.firstChild;child;child=child.nextSibling)if(!safe(child))return false;return true};
			if(next.nodeType===1&&!safe(next))return false;
			if(currentTrailingSpace)node.data=node.data.slice(0,offset);
			var trailing=block.lastChild;
			if(trailing&&trailing.nodeType===1&&String(trailing.localName||"").toLowerCase()==="br"){block.removeChild(trailing)}
			for(var i=0;i<skipped.length;i++)block.parentNode.removeChild(skipped[i]);
			if(isInlineText)next.data=nextText;
			var tail=block.lastChild;
			if(tail&&tail.nodeType===3&&next.nodeType===3){tail.data+=next.data;next.parentNode.removeChild(next)}else block.appendChild(next);
			var caretText=lastText(block),caret=document.createRange();
			if(!caretText)return false;caret.setStart(caretText,caretText.data.length);caret.collapse(true);sel.removeAllRanges();sel.addRange(caret);
			return true
		})()`
		if joined, joinErr := rt.EvaluateUserInput(joinFollowingInlineScript, inputType); joinErr == nil && joined == "true" {
			if host.IsConnected() {
				rt.DispatchTrustedInput(host, nil, inputType, false)
			}
			return true
		}
	}
	backward := `if(n.nodeType===3){if(o===0)return false;var start=o-1;if(start>0){var before=n.data.charCodeAt(start-1),last=n.data.charCodeAt(start);if(before>=0xD800&&before<=0xDBFF&&last>=0xDC00&&last<=0xDFFF)start--}if(/[\t\n\f\r ]/.test(n.data[start]||"")){var whiteSpace="normal";try{whiteSpace=String(getComputedStyle(n.parentNode).whiteSpace||"normal").toLowerCase()}catch(_){};if(whiteSpace==="pre-line"){var crossedLineBreak=false;while(start>0&&/[\t\n\f\r ]/.test(n.data[start-1])){if(n.data[start-1]==="\n"){if(crossedLineBreak)break;crossedLineBreak=true}start--}}else{while(start>0&&/[\t\n\f\r ]/.test(n.data[start-1]))start--}}r.setStart(n,start)}else{if(o===0)return false;var prev=n.childNodes[o-1];if(!prev)return false;if(prev.nodeType===3){var len=prev.data.length;if(!len)return false;var start=len-1;if(start>0){var before=prev.data.charCodeAt(start-1),last=prev.data.charCodeAt(start);if(before>=0xD800&&before<=0xDBFF&&last>=0xDC00&&last<=0xDFFF)start--}r.setStart(prev,start)}else if(prev.nodeType===1&&/^(br|hr|img|input|video|audio|iframe|object|embed)$/i.test(String(prev.localName||""))){r.setStartBefore(prev);r.setEndAfter(prev)}else{r.selectNodeContents(prev)}}`
	forwardDelete := `if(n.nodeType===3){var preLine=false;try{preLine=getComputedStyle(n.parentNode).whiteSpace==="pre-line"}catch(_){};if(o>=n.data.length||/^[\t\n\f\r ]*$/.test(n.data.slice(o))||(preLine&&/[\t\n\f\r ]/.test(n.data[o]||""))){var blockNames={address:1,blockquote:1,dd:1,div:1,dl:1,dt:1,li:1,p:1,pre:1};var block=n.parentNode;while(block&&block!==host&&!blockNames[String(block.localName||"").toLowerCase()])block=block.parentNode;var nextBlock=null,cursor=n;while(cursor&&cursor!==host&&!nextBlock){var candidate=cursor.nextSibling;while(candidate&&candidate.nodeType===3&&!candidate.data.trim())candidate=candidate.nextSibling;if(candidate&&candidate.nodeType===1&&blockNames[String(candidate.localName||"").toLowerCase()])nextBlock=candidate;else cursor=cursor.parentNode}if(nextBlock){var firstText=function(root){if(!root)return null;if(root.nodeType===3)return root;if(root.nodeType===1&&root!==host&&root.getAttribute&&root.getAttribute("contenteditable")==="false")return null;for(var child=root.firstChild;child;child=child.nextSibling){var found=firstText(child);if(found)return found}return null};var nextText=firstText(nextBlock);if(!nextText)return false;var whiteSpace=function(el){try{return String(getComputedStyle(el).whiteSpace||"").toLowerCase()}catch(_){return String(el.localName||"").toLowerCase()==="pre"?"pre":"normal"}};var preserves=function(value,el){return /^(pre|pre-wrap|break-spaces)$/.test(value)||String(el.localName||"").toLowerCase()==="pre"};var startOffset=o,currentWhiteSpace=whiteSpace(block);if(!preserves(currentWhiteSpace,block)&&/^[\t\n\f\r ]*$/.test(n.data.slice(o))){while(startOffset>0&&/[\t\n\f\r ]/.test(n.data[startOffset-1]))startOffset--;r.setStart(n,startOffset)}var endOffset=0,destinationWhiteSpace=whiteSpace(nextBlock);if(!preserves(destinationWhiteSpace,nextBlock)){while(endOffset<nextText.data.length&&/[\t\n\f\r ]/.test(nextText.data[endOffset]))endOffset++}r.setEnd(nextText,endOffset)}else if(preLine&&/[\t\n\f\r ]/.test(n.data[o]||"")){var end=o,sawNewline=false;while(end<n.data.length){var ch=n.data[end];if(ch==="\n"){if(sawNewline)break;sawNewline=true;end++;continue}if(/[\t\f\r ]/.test(ch)){end++;continue}break}r.setEnd(n,end)}else{var cursor=n,next=null;while(cursor&&cursor!==host&&!next){var candidate=cursor.nextSibling;while(candidate&&candidate.nodeType===3&&!candidate.data.length)candidate=candidate.nextSibling;if(candidate&&candidate.nodeType===1&&String(candidate.localName||"").toLowerCase()==="br"){var trailing=true;for(var rest=candidate.nextSibling;rest;rest=rest.nextSibling)if(!(rest.nodeType===1&&String(rest.localName||"").toLowerCase()==="br")){trailing=false;break}if(trailing){cursor=cursor.parentNode;continue}}if(candidate)next=candidate;else cursor=cursor.parentNode}if(!next||next.nodeType===1&&next.isContentEditable===false)return false;if(next.nodeType===3){if(!next.data.length)return false;var end=1;if(next.data.length>1){var first=next.data.charCodeAt(0),second=next.data.charCodeAt(1);if(first>=0xD800&&first<=0xDBFF&&second>=0xDC00&&second<=0xDFFF)end++}r.setEnd(next,end)}else if(next.nodeType===1&&/^(br|hr|img|input|video|audio|iframe|object|embed)$/.test(String(next.localName||"").toLowerCase())){}else{r.selectNodeContents(next)}}}else{var end=o+1;if(end<n.data.length){var first=n.data.charCodeAt(o),nextCode=n.data.charCodeAt(end);if(first>=0xD800&&first<=0xDBFF&&nextCode>=0xDC00&&nextCode<=0xDFFF)end++}r.setEnd(n,end)}}else{var next=n.childNodes[o];if(next){if(next.nodeType===3){if(!next.data.length)return false;var end=1;if(next.data.length>1){var first=next.data.charCodeAt(0),second=next.data.charCodeAt(1);if(first>=0xD800&&first<=0xDBFF&&second>=0xDC00&&second<=0xDFFF)end++}r.setEnd(next,end)}else if(next.nodeType===1&&/^(br|hr|img|input|video|audio|iframe|object|embed)$/.test(String(next.localName||"").toLowerCase())){}else{r.selectNodeContents(next)}}}`
	backwardWord := `if(n.nodeType===3){var texts=[];var collect=function(node){if(!node)return;if(node.nodeType===3){texts.push(node);return}if(node!==host&&node.nodeType===1&&node.hasAttribute("contenteditable")&&node.getAttribute("contenteditable")==="false")return;for(var child=node.firstChild;child;child=child.nextSibling)collect(child)};collect(host);var index=texts.indexOf(n),current=n,start=o;if(index<0)return false;while(true){while(start>0&&!/\s/.test(current.data[start-1]))start--;if(start>0)break;if(index===0)break;var previous=texts[index-1];if(!previous||!previous.data.length)break;var previousOffset=previous.data.length;if(/\s/.test(previous.data[previousOffset-1]))break;index--;current=previous;start=previousOffset}r.setStart(current,start)}else{if(o===0)return false;var prev=n.childNodes[o-1];if(!prev)return false;r.selectNodeContents(prev)}`
	forwardWord := `if(n.nodeType===3){var end=o;while(end<n.data.length&&!/\s/.test(n.data[end]))end++;while(end<n.data.length&&/\s/.test(n.data[end]))end++;r.setEnd(n,end)}else{if(o>=n.childNodes.length)return false;var next=n.childNodes[o];if(!next)return false;r.selectNodeContents(next)}`
	softLineForward := `if(n.nodeType===3){var end=n.data.length;try{var whiteSpace=String(getComputedStyle(n.parentNode).whiteSpace||"");if(/^(pre|pre-wrap|pre-line|break-spaces)$/.test(whiteSpace)){var newline=n.data.indexOf("\n",o);if(newline>=0)end=newline}}catch(_){};r.setEnd(n,end)}else{var block=n.nodeType===1?n:n.parentNode;var endText=null;var find=function(root){for(var child=root.firstChild;child;child=child.nextSibling){if(child.nodeType===1&&String(child.localName||"").toLowerCase()==="br")return true;if(child.nodeType===1&&child.isContentEditable===false)continue;if(find(child))return true;if(child.nodeType===3)endText=child}return false};while(block&&block!==host&&!/^(address|blockquote|dd|div|dl|dt|li|p|pre)$/.test(String(block.localName||"").toLowerCase()))block=block.parentNode;if(!block)return false;find(block);if(!endText)return false;r.setEnd(endText,endText.data.length)}`
	selectAdjacent := backward
	if softLine && forward {
		selectAdjacent = softLineForward
	} else if deleteWord && forward {
		selectAdjacent = forwardWord
	} else if deleteWord {
		selectAdjacent = backwardWord
	} else if forward {
		selectAdjacent = forwardDelete
	}
	script := `(function(){var host=document.activeElement;var sel=getSelection();if(!host||!host.isContentEditable||!sel||!sel.rangeCount)return false;if(sel.rangeCount>1){var cellFor=function(node){for(;node&&node!==host;node=node.parentNode)if(node.nodeType===1&&/^(td|th)$/i.test(String(node.localName||"")))return node;return null};var isCellRange=function(range){if(!range||range.collapsed)return false;var container=range.startContainer;if(container===range.endContainer&&container.nodeType===1&&String(container.localName||"").toLowerCase()==="tr"&&range.endOffset===range.startOffset+1){var selectedCell=container.childNodes[range.startOffset];return !!selectedCell&&selectedCell.nodeType===1&&/^(td|th)$/i.test(String(selectedCell.localName||""))}var startCell=cellFor(range.startContainer);return !!startCell&&cellFor(range.endContainer)===startCell};var tableCells=true;for(var i=0;i<sel.rangeCount;i++)if(!isCellRange(sel.getRangeAt(i))){tableCells=false;break}if(tableCells)return document.execCommand("delete",false,null)}var r=sel.getRangeAt(0).cloneRange();if(r.collapsed){var n=r.startContainer,o=r.startOffset;` + selectAdjacent + `}sel.removeAllRanges();sel.addRange(r);return document.execCommand("delete",false,null)})()`
	v, err := rt.EvaluateUserInput(script, inputType)
	if err != nil {
		return false
	}
	if v == "true" {
		return true
	}
	// Empty editing hosts still report the paired input event for the WPT
	// testdriver deletion action. At a boundary in a nonempty host, leave only
	// beforeinput because no editing operation changed the DOM.
	empty, emptyErr := rt.Evaluate(`(function(){var host=document.activeElement;if(!host||!host.isContentEditable||host.textContent!=="")return false;var visit=function(node){for(var child=node.firstChild;child;child=child.nextSibling){if(child.nodeType===1){if(String(child.localName||"").toLowerCase()!=="br")return false;if(!visit(child))return false}}return true};return visit(host)})()`)
	if emptyErr == nil && empty == "true" && host.IsConnected() {
		rt.DispatchTrustedInput(host, nil, inputType, false)
	}
	return false
}

// maxLen parses maxlength using HTML's non-negative-integer parsing rules.
// The boolean result distinguishes an absent/invalid limit from maxlength=0.
func maxLen(n *dom.Node) (int, bool) {
	return dom.MaxLength(n)
}

func utf16Length(value string) int {
	return dom.UTF16Length(value)
}

// InsertText inserts text into the currently focused editable control at the caret
// position, respecting maxlength and firing an "input" event.
func (s *Session) InsertText(text string) bool {
	s.opMu.Lock()
	if dom.IsContentEditable(s.focus) {
		s.opMu.Unlock()
		return s.insertContentEditableText(text)
	}
	defer s.opMu.Unlock()
	if !controlEditable(s.focus) || text == "" {
		return false
	}
	runes := []rune(s.controlValue(s.focus))
	if s.caret > len(runes) {
		s.caret = len(runes)
	}
	start, end, selected := s.controlSelectionBounds(len(runes))
	if !selected {
		start, end = s.caret, s.caret
	}
	added := []rune(text)
	if max, hasMax := maxLen(s.focus); hasMax {
		base := string(runes[:start]) + string(runes[end:])
		avail := max - utf16Length(base)
		if avail <= 0 {
			return false
		}
		if utf16Length(string(added)) > avail {
			for len(added) > 0 && utf16Length(string(added)) > avail {
				added = added[:len(added)-1]
			}
			if len(added) == 0 {
				return false
			}
		}
	}
	out := make([]rune, 0, len(runes)+len(added))
	out = append(out, runes[:start]...)
	out = append(out, added...)
	out = append(out, runes[end:]...)
	s.caret = start + len(added)
	s.controlSelectionActive = false
	s.setControlValue(s.focus, string(out))
	s.syncControlSelectionLocked()
	dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
	return true
}

// insertContentEditableText routes user text through the document's existing
// Selection-aware insertText command. The runtime call is made without opMu:
// beforeinput/input listeners may synchronously call back into Session.
func (s *Session) insertContentEditableText(text string) bool {
	return s.insertContentEditableTextWithType(text, "insertText")
}

func (s *Session) insertContentEditableTextWithType(text, inputType string) bool {
	return s.insertContentEditableTextWithDataTransfer(text, inputType, nil)
}

func (s *Session) insertContentEditableTextWithDataTransfer(text, inputType string, dataTransfer *dom.DataTransfer) bool {
	if text == "" {
		return false
	}
	s.opMu.Lock()
	rt := s.jsRT
	host := s.focus
	focused := dom.IsContentEditable(host)
	s.opMu.Unlock()
	if rt == nil || !focused {
		return false
	}
	quoted, err := json.Marshal(text)
	if err != nil {
		return false
	}
	// beforeinput is a trusted user-agent event; its cancellation suppresses
	// the default editing command. The command then updates the live Selection.
	accepted := false
	if dataTransfer != nil {
		accepted = rt.DispatchTrustedBeforeInputEventWithDataTransfer(host, nil, inputType, false, true, dataTransfer)
		dataTransfer.Disassociate()
	} else {
		accepted = rt.DispatchTrustedInputEvent(host, &text, inputType)
	}
	if !accepted {
		return false
	}
	beforeText := host.TextContent()
	script := `(function(){var host=document.activeElement;if(!host||!host.isContentEditable)return false;return document.execCommand("insertText",false,` + string(quoted) + `)})()`
	v, evalErr := rt.EvaluateUserInput(script, inputType)
	if evalErr == nil && v != "true" && host.IsConnected() && host.TextContent() == beforeText {
		rt.DispatchTrustedInput(host, &text, inputType, false)
	}
	return evalErr == nil && v == "true"
}

func (s *Session) insertContentEditableHTMLWithDataTransfer(markup string, dataTransfer *dom.DataTransfer) bool {
	if markup == "" || len(markup) > 1<<20 {
		return false
	}
	s.opMu.Lock()
	rt := s.jsRT
	host := s.focus
	focused := dom.IsContentEditable(host)
	s.opMu.Unlock()
	if rt == nil || !focused {
		return false
	}
	if !rt.DispatchTrustedBeforeInputEventWithDataTransfer(host, nil, "insertFromPaste", false, true, dataTransfer) {
		dataTransfer.Disassociate()
		return false
	}
	dataTransfer.Disassociate()
	quoted, err := json.Marshal(markup)
	if err != nil {
		return false
	}
	script := `(function(){var host=document.activeElement;if(!host||!host.isContentEditable)return false;return document.execCommand("insertHTML",false,` + string(quoted) + `)})()`
	result, err := rt.EvaluateUserInput(script, "insertFromPaste")
	return err == nil && result == "true"
}

// FocusedText returns the current value of the focused control, or empty if none.
func (s *Session) FocusedText() string {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.focus == nil {
		return ""
	}
	value := s.controlValue(s.focus)
	if !textSelectableControl(s.focus) || !s.controlSelectionActive {
		return value
	}
	runes := []rune(value)
	start, end, selected := s.controlSelectionBounds(len(runes))
	if !selected {
		return value
	}
	return string(runes[start:end])
}

// FocusedEditable reports whether the currently focused control is editable.
func (s *Session) FocusedEditable() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return controlEditable(s.focus) || dom.IsContentEditable(s.focus)
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
	if max, hasMax := maxLen(s.focus); hasMax && utf16Length(value) > max {
		return false
	}
	old := s.controlValue(s.focus)
	s.marked = ""
	s.markedHasNode = false
	s.caret = len(runes)
	s.controlSelectionActive = false
	s.controlSelectionAnchor = s.caret
	s.syncControlSelectionLocked()
	if old == value {
		return false
	}
	s.setControlValue(s.focus, value)
	dom.DispatchEvent(s.focus, dom.NewEvent("input", true, true))
	return true
}

// SelectAllControl selects the full value of the focused text control.
func (s *Session) SelectAllControl() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if !textSelectableControl(s.focus) {
		return false
	}
	runes := []rune(s.controlValue(s.focus))
	s.controlSelectionAnchor = 0
	s.controlSelectionActive = len(runes) > 0
	s.caret = len(runes)
	s.syncControlSelectionLocked()
	return true
}

// controlValue reads the current form value for a control.
func (s *Session) controlValue(n *dom.Node) string {
	if n.DataAtom == dom.AtomSelect {
		if option := SelectedOption(n); option != nil {
			return optionValue(option)
		}
		return ""
	}
	if n.DataAtom == dom.AtomOption {
		return optionValue(n)
	}
	if n.DataAtom == dom.AtomTextarea {
		return n.InputControlValue()
	}
	return n.InputControlValue()
}

// replaceControlCompositionLocked replaces the previously marked scalar-value
// span in an input or textarea. The caller holds opMu and updates the marked
// state after the corresponding input event sequence has been dispatched.
func (s *Session) replaceControlCompositionLocked(n *dom.Node, start int, oldText, newText string) {
	if !controlEditable(n) {
		return
	}
	runes := []rune(s.controlValue(n))
	if start < 0 {
		start = 0
	}
	if start > len(runes) {
		start = len(runes)
	}
	end := start + len([]rune(oldText))
	if end > len(runes) {
		end = len(runes)
	}
	inserted := []rune(newText)
	out := make([]rune, 0, len(runes)-(end-start)+len(inserted))
	out = append(out, runes[:start]...)
	out = append(out, inserted...)
	out = append(out, runes[end:]...)
	s.setControlValue(n, string(out))
	s.caret = start + len(inserted)
	s.markedOffset = start
}

func (s *Session) setControlValue(n *dom.Node, v string) {
	changed := n != nil && s.controlValue(n) != v
	if changed && n == s.focus {
		s.focusUserChanged = true
	}
	if n.DataAtom == dom.AtomTextarea {
		n.SetInputControlValue(v, true)
		n.ValueChangedByUser = true
		if s.jsRT != nil {
			s.jsRT.RefreshInputPatternMismatch(n)
		}
		return
	}
	n.SetInputControlValue(v, true)
	n.ValueChangedByUser = true
	if s.jsRT != nil {
		s.jsRT.RefreshInputPatternMismatch(n)
	}
}

// IsChecked reports whether a checkbox or radio input is checked.
// The checked state is tracked via the "checked" boolean attribute:
// presence means checked, absence means unchecked.
func IsChecked(n *dom.Node) bool {
	if n == nil {
		return false
	}
	return n.InputChecked()
}

// SetChecked sets or removes the checked attribute on a node.
func SetChecked(n *dom.Node, checked bool) {
	if n == nil {
		return
	}
	n.SetInputChecked(checked, true)
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

// optionValue is an <option>'s form value: its value attribute when present,
// otherwise its text content normalised the way Chromium reports it.
// Submission, the editing path and the accessibility tree all read it here so
// they cannot drift apart.
//
// The two branches are deliberately different, and both are measured rather
// than remembered (scripts/verify-task037-option-value.js, Chromium 1.63 on
// this host):
//
//	<option selected>\n    Charcoal\n  </option>  -> value "Charcoal"
//	<option selected>  Hi   there </option>       -> value "Hi there"
//	<option selected value="  pad  me  ">        -> value "  pad  me  "
//	<option selected value="a\n b\tc">            -> value "a\n b\tc"
//	<option selected value="">                    -> value ""
//
// The value content attribute is used verbatim, including when it is empty (an
// empty attribute is not the absent one: Chromium submits the empty string, not
// the text). Only the text-content fallback is normalised.
func optionValue(n *dom.Node) string {
	if n.HasAttribute("value") {
		return n.GetAttribute("value")
	}
	return optionTextValue(n.TextContent())
}

// optionTextValue is the normalisation Chromium applies to an option's text
// content before it becomes a value or a label: leading and trailing ASCII
// whitespace stripped, every internal run of ASCII whitespace collapsed to a
// single U+0020. Measured by scripts/verify-task037-option-value.js, which also
// shows what must NOT be touched - a U+00A0 on either edge survives verbatim,
// so strings.TrimSpace and unicode.IsSpace are both wrong
// here. Whitespace is ASCII and cannot occur inside a multi-byte UTF-8
// sequence, so the byte scan below is safe on any input.
func optionTextValue(text string) string {
	var sb strings.Builder
	sb.Grow(len(text))
	pendingSpace := false
	for i := 0; i < len(text); i++ {
		if isASCIISpace(text[i]) {
			pendingSpace = true
			continue
		}
		if pendingSpace && sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		pendingSpace = false
		sb.WriteByte(text[i])
	}
	return sb.String()
}

// isASCIISpace reports the bytes Chromium collapses in an option's text content,
// measured by scripts/verify-task037-option-value.js: U+0009, U+000A, U+000C,
// U+000D and U+0020. U+000B (vertical tab) is deliberately absent - the same
// measurement shows "a\v\vb" keeps its vertical tabs while "a\f\fb" collapses,
// so this is HTML's ASCII whitespace set and not CSS's or unicode.IsSpace's.
func isASCIISpace(b byte) bool {
	switch b {
	case '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

// SelectedOption returns the currently selected <option> node within a <select>.
// When several options carry the "selected" attribute the last one in tree order
// wins, which is what Chromium reports for the same markup: for
// `<option value="a" selected><option value="b" selected>` it gives
// `selectedIndex 1`, `value "b"` and FormData `[["c","b"]] while leaving both
// content attributes in the DOM (scripts/verify-task037-option-value.js,
// TIEBREAK). Resolving the tie at read time keeps the serialised document
// untouched. If no option has the "selected" attribute, the first <option> child
// is returned. Returns nil if selectNode is nil or has no <option> children.
func SelectedOption(selectNode *dom.Node) *dom.Node {
	if selectNode == nil {
		return nil
	}
	dom.NormalizeSelectSelectedness(selectNode)
	for _, option := range dom.SelectOptions(selectNode) {
		if option.OptionSelected() {
			return option
		}
	}
	return nil
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
		SetChecked(n, true)
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
		SetChecked(root, false)
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
	s.FocusFrom(nil, false)
}

// FocusFrom advances sequential focus navigation relative to a non-focusable
// interaction target. This lets pointer actions inside static content move
// focus to the next control in tree order.
func (s *Session) FocusFrom(from *dom.Node, backwards bool) {
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
	if from != nil && from.IsConnected() {
		positions := focusTreePositions(s.Doc)
		if at, ok := positions[from]; ok {
			if backwards {
				idx = 0
				for i, control := range controls {
					if position, found := positions[control]; found && position >= at {
						idx = i
						break
					}
				}
			} else {
				idx = len(controls) - 1
				for i, control := range controls {
					if position, found := positions[control]; found && position > at {
						idx = i - 1
						break
					}
				}
			}
		}
	}
	if idx < 0 {
		for i, c := range controls {
			if c == s.focus {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		if groupIndex := radioTabGroupIndex(s.focus, controls); groupIndex >= 0 {
			idx = groupIndex - 1
		}
	}
	step := 1
	if backwards {
		step = -1
	}
	next := controls[(idx+step+len(controls))%len(controls)]
	s.clearKeyboardActiveLocked()
	// Fire blur/change on old focus.
	if s.focus != nil {
		css.SetFocus(s.focus, false)
		css.SetFocusVisible(s.focus, false)
		dom.DispatchEvent(s.focus, dom.NewEvent("blur", false, false))
		dom.DispatchEvent(s.focus, dom.NewEvent("focusout", true, false))
		if s.focusUserChanged && s.controlValue(s.focus) != s.focusValue {
			dom.DispatchEvent(s.focus, dom.NewEvent("change", true, true))
		}
	}
	s.focus = next
	s.focusState.Store(next)
	s.marked = ""
	s.controlSelectionActive = false
	s.caret = len([]rune(s.controlValue(next)))
	s.controlSelectionAnchor = s.caret
	s.syncControlSelectionLocked()
	s.focusValue = s.controlValue(next)
	s.focusUserChanged = false
	css.SetFocus(next, true)
	css.SetFocusVisible(next, true)
	dom.DispatchEvent(next, dom.NewEvent("focus", false, false))
	dom.DispatchEvent(next, dom.NewEvent("focusin", true, false))
	s.scrollFocusedNodeIntoViewLocked(next)
}

func focusTreePositions(doc *dom.Document) map[*dom.Node]int {
	positions := make(map[*dom.Node]int)
	index := 0
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		positions[node] = index
		index++
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.ShadowRoot != nil {
			for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
	}
	if doc != nil {
		walk(&doc.Node)
	}
	return positions
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
	if idx == 0 && controls[0] != s.focus {
		if groupIndex := radioTabGroupIndex(s.focus, controls); groupIndex >= 0 {
			idx = groupIndex + 1
		}
	}
	prev := controls[(idx-1+len(controls))%len(controls)]
	s.clearKeyboardActiveLocked()
	// Fire blur/change on old focus.
	if s.focus != nil {
		css.SetFocus(s.focus, false)
		css.SetFocusVisible(s.focus, false)
		dom.DispatchEvent(s.focus, dom.NewEvent("blur", false, false))
		dom.DispatchEvent(s.focus, dom.NewEvent("focusout", true, false))
		if s.focusUserChanged && s.controlValue(s.focus) != s.focusValue {
			dom.DispatchEvent(s.focus, dom.NewEvent("change", true, true))
		}
	}
	s.focus = prev
	s.focusState.Store(prev)
	s.marked = ""
	s.controlSelectionActive = false
	s.caret = len([]rune(s.controlValue(prev)))
	s.controlSelectionAnchor = s.caret
	s.syncControlSelectionLocked()
	s.focusValue = s.controlValue(prev)
	s.focusUserChanged = false
	css.SetFocus(prev, true)
	css.SetFocusVisible(prev, true)
	dom.DispatchEvent(prev, dom.NewEvent("focus", false, false))
	dom.DispatchEvent(prev, dom.NewEvent("focusin", true, false))
	s.scrollFocusedNodeIntoViewLocked(prev)
}

// scrollFocusedNodeIntoViewLocked applies the viewport scroll adjustment that
// accompanies focusing an element outside the visible vertical range. The
// layout arena keeps document coordinates; the JS runtime owns the document's
// CSSOM View scroll offset.
func (s *Session) scrollFocusedNodeIntoViewLocked(n *dom.Node) {
	if s == nil || s.jsRT == nil || s.viewportH <= 0 {
		return
	}
	obj := s.objectFor(n)
	if obj == nil {
		return
	}
	_, top, _, bottom := obj.BorderRect()
	scrollY := float32(s.jsRT.WindowScrollY())
	if top < scrollY {
		scrollY = top
	} else if bottom > scrollY+s.viewportH {
		scrollY = bottom - s.viewportH
	}
	if scrollY < 0 {
		scrollY = 0
	}
	s.jsRT.SetWindowScrollY(float64(scrollY))
}

// TabbableControls returns sequentially focusable controls in tree order.
// Tabbable elements: input (not hidden, not disabled), textarea, select,
// button, and a[href]. Named radio groups contribute one stop: the checked
// enabled member, or the first enabled member when none is checked.
func TabbableControls(doc *dom.Document) []*dom.Node {
	if doc == nil {
		return nil
	}
	var candidates []*dom.Node
	walkForTabbable(&doc.Node, &candidates)
	if modal := topmostModalDialog(doc); modal != nil {
		withinModal := candidates[:0]
		for _, node := range candidates {
			if nodeWithin(node, modal) {
				withinModal = append(withinModal, node)
			}
		}
		candidates = withinModal
	}
	groups := make(map[radioTabGroupKey][]*dom.Node)
	for _, node := range candidates {
		if !isRadioInput(node) {
			continue
		}
		name := node.GetAttribute("name")
		if name == "" {
			continue
		}
		key := radioTabGroupKey{owner: radioTabFormOwner(node), name: name}
		groups[key] = append(groups[key], node)
	}
	selected := make(map[radioTabGroupKey]*dom.Node, len(groups))
	for key, members := range groups {
		var firstEnabled, checked *dom.Node
		for _, member := range members {
			if !isTabbable(member) {
				continue
			}
			if firstEnabled == nil {
				firstEnabled = member
			}
			if checkedRadio(member) {
				checked = member
				break
			}
		}
		if checked != nil {
			selected[key] = checked
		} else if firstEnabled != nil {
			selected[key] = firstEnabled
		}
	}
	result := make([]*dom.Node, 0, len(candidates))
	for _, node := range candidates {
		if !isRadioInput(node) || node.GetAttribute("name") == "" {
			result = append(result, node)
			continue
		}
		key := radioTabGroupKey{owner: radioTabFormOwner(node), name: node.GetAttribute("name")}
		if selected[key] == node {
			result = append(result, node)
		}
	}
	return result
}

type radioTabGroupKey struct {
	owner *dom.Node
	name  string
}

func isRadioInput(n *dom.Node) bool {
	return n != nil && n.Element() && n.Data == "input" && strings.EqualFold(n.GetAttribute("type"), "radio")
}

func checkedRadio(n *dom.Node) bool {
	return IsChecked(n)
}

func radioTabFormOwner(n *dom.Node) *dom.Node {
	if n.FormOwner != nil {
		return n.FormOwner
	}
	for parent := n.Parent; parent != nil; parent = parent.Parent {
		if parent.Element() && parent.Data == "form" {
			return parent
		}
	}
	return nil
}

func radioTabGroupIndex(n *dom.Node, controls []*dom.Node) int {
	if !isRadioInput(n) || n.GetAttribute("name") == "" {
		return -1
	}
	key := radioTabGroupKey{owner: radioTabFormOwner(n), name: n.GetAttribute("name")}
	for i, candidate := range controls {
		if !isRadioInput(candidate) || candidate.GetAttribute("name") == "" {
			continue
		}
		if (radioTabGroupKey{owner: radioTabFormOwner(candidate), name: candidate.GetAttribute("name")}) == key {
			return i
		}
	}
	return -1
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
	if dom.IsFormControlDisabled(n) {
		return false
	}
	for ancestor := n; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.HasAttribute("inert") {
			return false
		}
	}
	if n.HasAttribute("tabindex") {
		tabIndex, err := strconv.Atoi(strings.TrimSpace(n.GetAttribute("tabindex")))
		return err == nil && tabIndex >= 0
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
	return dom.IsContentEditable(n)
}

func topmostModalDialog(doc *dom.Document) *dom.Node {
	if doc == nil {
		return nil
	}
	var topmost *dom.Node
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() && node.Data == "dialog" && node.IsModal && node.HasAttribute("open") {
			topmost = node
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
	walk(&doc.Node)
	return topmost
}

func nodeWithin(node, ancestor *dom.Node) bool {
	if node == nil || ancestor == nil {
		return false
	}
	for current := node; current != nil; current = current.Parent {
		if current == ancestor {
			return true
		}
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
			if dom.IsFormControlDisabled(obj.Node) {
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
