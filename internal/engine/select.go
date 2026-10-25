package engine

import (
	"github.com/vyquocvu/goosie/internal/dom"
)

// Keyboard selection for <select>. The DOM stays the single source of truth:
// the chosen <option> carries the "selected" attribute, which is what
// controlValue, SelectedOption, paintSelect and the form collection already
// read. Nothing here keeps a parallel selection index that could drift.

// FocusedSelect returns the focused <select> element, or nil when focus is
// elsewhere, on a disabled select, or on nothing at all.
func (s *Session) FocusedSelect() *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.focusedSelectLocked()
}

// FocusedSelectValue returns the value a submit would carry for the focused
// select. Callers that are not on the input goroutine must read the selection
// through this instead of walking the option nodes themselves: MoveSelect
// clears and sets "selected" attributes, so an option list is only a coherent
// snapshot while the operation lock is held. Paint gets the same guarantee from
// PaintChecked, which takes that lock.
func (s *Session) FocusedSelectValue() string {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	sel := s.focusedSelectLocked()
	if sel == nil {
		return ""
	}
	return controlValue(sel)
}

// focusedSelectLocked is the caller-locked form, used by MoveSelect, which
// already holds opMu and must not re-enter the non-reentrant mutex.
func (s *Session) focusedSelectLocked() *dom.Node {
	n := s.focus
	if n == nil || !n.Element() || n.Data != "select" || n.HasAttribute("disabled") {
		return nil
	}
	return n
}

// selectOptionNodes returns the option children of a select in tree order.
// Options nested inside an <optgroup> are not reached: the engine has no
// optgroup support yet, so a keyboard user cannot land on those options.
func selectOptionNodes(sel *dom.Node) []*dom.Node {
	var opts []*dom.Node
	if sel == nil {
		return opts
	}
	for c := sel.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() && c.Data == "option" {
			opts = append(opts, c)
		}
	}
	return opts
}

// MoveSelect shifts the selected option of the focused select. It reports
// whether the key was consumed by the select: while a select has focus the
// up/down/home/end keys always belong to it, even when the selection cannot
// move, so the shell must not fall through to page scrolling.
//
// A move clears "selected" from every option before setting it on the target,
// so a document that authored two selected options collapses to the single
// selection the rest of the engine assumes. Disabled options are skipped, and
// the selection stops at the ends instead of wrapping, matching a
// single-select control.
func (s *Session) MoveSelect(action EditAction) bool {
	switch action {
	case EditUp, EditDown, EditHome, EditEnd:
	default:
		return false
	}

	s.opMu.Lock()
	defer s.opMu.Unlock()

	sel := s.focusedSelectLocked()
	if sel == nil {
		return false
	}

	opts := selectOptionNodes(sel)
	if len(opts) == 0 {
		return true
	}
	cur := -1
	for i, o := range opts {
		if o.HasAttribute("selected") {
			cur = i
			break
		}
	}
	if cur < 0 {
		for i, o := range opts {
			if !o.HasAttribute("disabled") {
				cur = i
				break
			}
		}
		if cur < 0 {
			return true
		}
	}

	target := cur
	switch action {
	case EditUp:
		target = prevEnabledOption(opts, cur)
	case EditDown:
		target = nextEnabledOption(opts, cur)
	case EditHome:
		target = firstEnabledOption(opts)
	case EditEnd:
		target = lastEnabledOption(opts)
	}
	if target < 0 || target == cur {
		return true
	}

	setSelectedOption(opts, target)
	// input before change: a page listening to both sees the value already
	// settled by the time change fires.
	dom.DispatchEvent(sel, dom.NewEvent("input", true, true))
	dom.DispatchEvent(sel, dom.NewEvent("change", true, true))
	return true
}

func nextEnabledOption(opts []*dom.Node, from int) int {
	for i := from + 1; i < len(opts); i++ {
		if !opts[i].HasAttribute("disabled") {
			return i
		}
	}
	return from
}

func prevEnabledOption(opts []*dom.Node, from int) int {
	for i := from - 1; i >= 0; i-- {
		if !opts[i].HasAttribute("disabled") {
			return i
		}
	}
	return from
}

func firstEnabledOption(opts []*dom.Node) int {
	for i, o := range opts {
		if !o.HasAttribute("disabled") {
			return i
		}
	}
	return -1
}

func lastEnabledOption(opts []*dom.Node) int {
	for i := len(opts) - 1; i >= 0; i-- {
		if !opts[i].HasAttribute("disabled") {
			return i
		}
	}
	return -1
}

// setSelectedOption makes opts[target] the only selected option and drops the
// attribute from every sibling, so the serialized DOM cannot end up with two
// selected options for a single-select control.
func setSelectedOption(opts []*dom.Node, target int) {
	for _, o := range opts {
		o.RemoveAttribute("selected")
	}
	opts[target].SetAttribute("selected", "")
}
