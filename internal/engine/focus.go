package engine

import (
	"strconv"

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

// FocusControl focuses the control at the document point, blurring any current
// focus. Clicking empty space blurs; clicking the already-focused control
// changes nothing. Returns whether focus state changed. Any focus change ends
// composition: the marked text belonged to the control that had focus.
func (s *Session) FocusControl(x, y float32) bool {
	n := s.controlAt(x, y)
	if n == s.focus {
		return false
	}
	s.focus = n
	s.marked = ""
	if n != nil {
		s.caret = len([]rune(s.controlValue(n)))
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
		return insert(string(r))
	case EditEnter:
		if s.focus.DataAtom != dom.AtomTextarea {
			return false
		}
		return insert("\n")
	case EditBackspace:
		if s.caret == 0 || len(runes) == 0 {
			return false
		}
		out := append([]rune{}, runes[:s.caret-1]...)
		out = append(out, runes[s.caret:]...)
		s.caret--
		s.setControlValue(s.focus, string(out))
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
