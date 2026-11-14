package engine

import (
	"github.com/vyquocvu/goosie/internal/dom"
)

func (s *Session) FocusedSelect() *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.focusedSelectLocked()
}

func (s *Session) FocusedSelectValue() string {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	sel := s.focusedSelectLocked()
	if sel == nil {
		return ""
	}
	return controlValue(sel)
}

func (s *Session) focusedSelectLocked() *dom.Node {
	n := s.focus
	if n == nil || !n.Element() || n.Data != "select" || dom.IsFormControlDisabled(n) {
		return nil
	}
	return n
}

func selectOptionNodes(sel *dom.Node) []*dom.Node {
	return dom.SelectOptions(sel)
}

func (s *Session) MoveSelect(action EditAction) bool {
	switch action {
	case EditUp, EditDown, EditHome, EditEnd:
	default:
		return false
	}

	s.opMu.Lock()
	consumed, events := s.moveSelectLocked(action)
	s.opMu.Unlock()
	dispatchTrustedEvents(events)
	return consumed
}

func (s *Session) moveSelectLocked(action EditAction) (bool, []trustedEvent) {
	sel := s.focusedSelectLocked()
	if sel == nil {
		return false, nil
	}

	opts := selectOptionNodes(sel)
	if len(opts) == 0 {
		return true, nil
	}
	cur := -1
	dom.NormalizeSelectSelectedness(sel)
	for i, o := range opts {
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
			return true, nil
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
		return true, nil
	}

	setSelectedOption(opts, target)
	return true, []trustedEvent{
		{sel, dom.NewTrustedEvent("input", true, true)},
		{sel, dom.NewTrustedEvent("change", true, true)},
	}
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

func setSelectedOption(opts []*dom.Node, target int) {
	for i, o := range opts {
		o.SetOptionSelected(i == target, true)
	}
}
