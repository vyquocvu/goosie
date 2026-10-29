package engine

import (
	"github.com/vyquocvu/goosie/internal/dom"
)

// Keyboard selection for <select>. The DOM option nodes store live selectedness
// separately from their selected content attributes.

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
	if n == nil || !n.Element() || n.Data != "select" || dom.IsFormControlDisabled(n) {
		return nil
	}
	return n
}

// selectOptionNodes returns the options in a select's options list.
func selectOptionNodes(sel *dom.Node) []*dom.Node {
	return dom.SelectOptions(sel)
}

// MoveSelect shifts the selected option of the focused select. It reports
// whether the key was consumed by the select: while a select has focus the
// up/down/home/end keys always belong to it, even when the selection cannot
// move, so the shell must not fall through to page scrolling.
//
// A move clears "selected" from every option before setting it on the target,
// so a document that authored two selected options collapses to one selection -
// the last of them, which is what SelectedOption already reports. Disabled
// options are skipped, and the selection stops at the ends instead of wrapping,
// matching a single-select control.
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
	dom.NormalizeSelectSelectedness(sel)
	for i, o := range opts {
		// The last authored selection wins, which is the tie-break SelectedOption
		// reports and the one Chromium measures (scripts/verify-task037-
		// option-value.js: two selected options give selectedIndex 1, and ArrowDown
		// from that document steps to index 2). Stepping from the first would make
		// the first key press look like a no-op, because the value the control
		// already carries is the last one.
		if o.OptionSelected() {
			cur = i
		}
	}
	if cur < 0 {
		for i, o := range opts {
			if !o.OptionDisabled() {
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
		if !opts[i].OptionDisabled() {
			return i
		}
	}
	return from
}

func prevEnabledOption(opts []*dom.Node, from int) int {
	for i := from - 1; i >= 0; i-- {
		if !opts[i].OptionDisabled() {
			return i
		}
	}
	return from
}

func firstEnabledOption(opts []*dom.Node) int {
	for i, o := range opts {
		if !o.OptionDisabled() {
			return i
		}
	}
	return -1
}

func lastEnabledOption(opts []*dom.Node) int {
	for i := len(opts) - 1; i >= 0; i-- {
		if !opts[i].OptionDisabled() {
			return i
		}
	}
	return -1
}

// setSelectedOption makes opts[target] the only selected option and drops the
// attribute from every sibling, so the serialized DOM cannot end up with two
// selected options for a single-select control.
func setSelectedOption(opts []*dom.Node, target int) {
	for i, o := range opts {
		o.SetOptionSelected(i == target, true)
	}
}
