package engine

import (
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

// A pointer chooses an option in a <select> by pressing the closed control to
// raise its listbox, then pressing a row. The keyboard path (select.go) already
// moved the selection; this is the half that needs the rows to exist on screen
// and under the cursor.
//
// One function owns the listbox geometry: listBoxLocked. Paint draws the rows it
// returns and the pointer path hits the same rows, so a row cannot be drawn
// somewhere the pointer lands on a different one - the same split that
// hitBoxContains keeps for border-radius.
//
// The rules come from scripts/verify-br101-select-popup.js, which measures
// Chromium's in-page listbox (<select size="3">) because the OS-native popup of
// a closed select is invisible to page script. On this host:
//
//	RECTS -> lb [40,60,180,62.5625], a [41,61,178,20.1875], b [41,81.1875,178,20.1875]
//
// so the rows sit 1px inside the control's border box on x, tile it downward
// with no gap, and are exactly as tall as the control font's normal line box
// (3 x 20.1875 + 2 x 1 = 62.5625).
//
// The panel's place in the viewport is the other half, and it splits the same
// way: the rows must be where the user can see them. The panel never covers more
// than the viewport, it opens above the control when the whole list does not fit
// below, and a list longer than the space on either side shows a window of rows
// whose top line follows the selection.
// scripts/verify-br101-listbox-viewport.js measures the window rule on the
// in-page listbox (its scrollTop/rowH is exactly max(0, selected-visible+1)) and
// proves the popup itself is unobservable from a headless host, so the side
// choice and the clamp are this engine's rule, recorded as such in
// docs/qa-open-defects.json rather than claimed as Chromium's.

// listboxInset is the gap between the panel's outer edge and its rows, in CSS
// px - the same 1px Chromium measures between a listbox's border box and its
// first row.
const listboxInset = float32(1)

// selectPopupNode is the race-safe read of the control whose listbox is showing,
// for input paths that must decide before touching the rows. Every write to the
// field happens under opMu.
func (s *Session) selectPopupNode() *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.selectPopup
}

// SelectPopupOpen reports whether a pointer has raised a <select> listbox.
func (s *Session) SelectPopupOpen() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.selectPopup != nil
}

// SelectPopupRows returns the option rows the open listbox shows, in document
// CSS px, each row carrying the option it is showing. It is the list the pointer
// hits and the list paint draws; nil when no listbox is open.
func (s *Session) SelectPopupRows() []paint.ListBoxRow {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	lb := s.listBoxLocked(s.selectPopup)
	if lb == nil {
		return nil
	}
	return lb.Rows
}

// CloseSelectPopup dismisses an open listbox without changing the selection. It
// reports whether one was open, so a host can tell Escape from an unhandled key.
func (s *Session) CloseSelectPopup() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.closeSelectPopupLocked()
}

func (s *Session) closeSelectPopupLocked() bool {
	if s.selectPopup == nil {
		return false
	}
	s.selectPopup = nil
	return true
}

// openableSelect resolves the node a press landed on to the closed-box <select>
// whose listbox that press should raise: the control itself or an ancestor of
// the hit node, enabled, holding at least one option, and drawn as a closed box
// rather than an inline listbox (a <select multiple> or size>1 already shows its
// rows, so raising a second copy of them would be a defect).
func (s *Session) openableSelect(hit *dom.Node) *dom.Node {
	for n := hit; n != nil; n = n.Parent {
		if !n.Element() || n.Data != "select" {
			continue
		}
		if dom.IsFormControlDisabled(n) || !dom.SelectIsPopupStyle(n) {
			return nil
		}
		if len(dom.SelectOptions(n)) == 0 {
			return nil
		}
		return n
	}
	return nil
}

// openSelectPopup raises the listbox for a control a pointer just pressed, and
// takes focus the way a press on a form control does (CLOSED-CLICK reports the
// select as activeElement after a press that leaves the value alone).
func (s *Session) openSelectPopup(sel *dom.Node, x, y float32) {
	s.FocusControl(x, y)
	s.opMu.Lock()
	s.selectPopup = sel
	s.opMu.Unlock()
}

// clickSelectPopup spends a pointer press while a listbox is open. It reports
// whether the press was spent: a press on an enabled row picks that option, and a
// press inside the panel (padding included) or on a disabled row is spent with
// the selection unchanged. A press outside the panel dismisses the listbox and
// returns false, so the same press goes on to whatever is under the cursor: this
// listbox is not modal - the page keeps taking keyboard and pointer input while
// it is up - so swallowing the dismiss would make every control next to a select
// need a second click.
func (s *Session) clickSelectPopup(x, y float32) bool {
	s.opMu.Lock()
	lb := s.listBoxLocked(s.selectPopup)
	s.opMu.Unlock()
	if lb == nil {
		// The control lost its box: there is nothing to pick, so the press simply
		// dismisses the listbox.
		s.CloseSelectPopup()
		return false
	}
	if !rectFContains(lb.Panel, x, y) {
		s.CloseSelectPopup()
		return false
	}
	for _, row := range lb.Rows {
		if !rectFContains(row.Rect, x, y) {
			continue
		}
		if row.Disabled {
			// EVENTS-CLICK-C: Chromium fires nothing at all for a press on a
			// disabled option, and the listbox stays up.
			return true
		}
		s.pickListboxRow(row.Index)
		return true
	}
	s.CloseSelectPopup()
	return true
}

// pickListboxRow makes the control's option at index the selection, reports it
// the way a change made by the user is reported, and closes the listbox.
func (s *Session) pickListboxRow(index int) {
	s.opMu.Lock()
	sel := s.selectPopup
	options := dom.SelectOptions(sel)
	if sel == nil || index < 0 || index >= len(options) {
		s.opMu.Unlock()
		return
	}
	setSelectedOption(options, index)
	rt := s.jsRT
	s.selectPopup = nil
	s.opMu.Unlock()

	if rt != nil {
		rt.SetTransientUserActivation(true)
	}
	// EVENTS-CLICK-A: the value is already settled by mouseup, and input then
	// change fire before the click event reaches the option. This engine models
	// the press as one step, so the order it can honour is input before change,
	// both before the click.
	dom.DispatchEvent(sel, dom.NewTrustedEvent("input", true, false))
	dom.DispatchEvent(sel, dom.NewTrustedEvent("change", true, false))
	dom.DispatchEvent(options[index], dom.NewTrustedEvent("click", true, true))
}

// listBoxLocked is the only place a listbox's geometry is computed. It returns
// nil when there is nothing to show: no control, no box, no options, or a
// control that stopped being a closed-box select.
//
// Callers must hold opMu: the row list reads the option tree, which
// setSelectedOption mutates under the same lock.
func (s *Session) listBoxLocked(sel *dom.Node) *paint.ListBox {
	if sel == nil || s.Arena == nil {
		return nil
	}
	obj := s.objectFor(sel)
	if obj == nil || obj.Style == nil {
		return nil
	}
	options := dom.SelectOptions(sel)
	if len(options) == 0 {
		return nil
	}
	x0, y0, x1, y1 := obj.BorderRect()
	box := frame.RectF4(x0, y0, x1, y1)
	if box.Empty() {
		return nil
	}
	rowH := s.listboxRowHeight(obj)
	if rowH <= 0 {
		return nil
	}
	panelW := box.X1 - box.X0
	_, vpH := s.Viewport()
	scrollY := s.windowScrollY()
	// The panel may only cover the viewport the user is looking at: a row the
	// pointer can hit must be a row that is on screen, and rows the panel cannot
	// show are a window, not an overflow. The row-window rule is the one
	// scripts/verify-br101-listbox-viewport.js measures on Chromium's in-page
	// listbox (scrollTop/rowH is exactly max(0, selected-visible+1)); the side
	// choice and the clamp are this engine's rule, because a closed select's
	// native popup is invisible to every headless measurement - see the same
	// script's POPUP-PIXELS line and GOOSIE-LISTBOX-001.
	visible := len(options)
	above := false
	if vpH > 0 {
		room := func(space float32) int {
			if space <= 2*listboxInset {
				return 0
			}
			return int((space - 2*listboxInset) / rowH)
		}
		// Room under the control only counts while its bottom edge is on screen:
		// a panel below a control that has scrolled away is not visible either.
		belowSpace := vpH - (box.Y1 - scrollY)
		if box.Y1-scrollY <= 0 {
			belowSpace = 0
		}
		below, up := room(belowSpace), room(box.Y0-scrollY)
		switch {
		case below >= len(options):
			visible = len(options)
		case up >= len(options):
			visible, above = len(options), true
		case up > below:
			visible, above = max(up, 1), true
		default:
			visible = max(below, 1)
		}
	}
	panelH := 2*listboxInset + float32(visible)*rowH
	var panel frame.RectF
	if above {
		panel = frame.RectF4(box.X0, box.Y0-panelH, box.X0+panelW, box.Y0)
	} else {
		panel = frame.RectF4(box.X0, box.Y1, box.X0+panelW, box.Y1+panelH)
	}

	selected := sel.SelectedOption()
	first := 0
	if visible < len(options) {
		// The window starts at the smallest offset that shows the selection, which
		// parks it on the last line: the anchor's ROW-WINDOW lines give exactly
		// max(0, selected-visible+1) as the top visible row. selIndex is a real
		// index into options, so that expression only needs a floor at zero.
		selIndex := -1
		for i, option := range options {
			if option == selected {
				selIndex = i
				break
			}
		}
		if selIndex >= 0 {
			first = selIndex - visible + 1
		}
		first = max(first, 0)
	}

	rows := make([]paint.ListBoxRow, 0, visible)
	top := panel.Y0 + listboxInset
	for j := 0; j < visible; j++ {
		i := first + j
		option := options[i]
		rect := frame.RectF4(panel.X0+listboxInset, top+float32(j)*rowH, panel.X1-listboxInset, top+float32(j+1)*rowH)
		rows = append(rows, paint.ListBoxRow{
			Rect:     rect,
			Index:    i,
			Label:    option.OptionLabel(),
			Selected: selected != nil && option == selected,
			Disabled: option.OptionDisabled(),
		})
	}
	return &paint.ListBox{Obj: obj, Panel: panel, Rows: rows}
}

// windowScrollY is the document's vertical scroll offset, which the JS runtime's
// CSSOM View state owns. It is 0 when the session runs without scripting: an
// engine with no runtime has never moved its viewport.
//
// Callers must hold opMu.
func (s *Session) windowScrollY() float32 {
	if s.jsRT == nil {
		return 0
	}
	return float32(s.jsRT.WindowScrollY())
}

// listboxRowHeight is the height one option's row takes in CSS px: the control
// font's own normal line box, which is what Chromium's listbox rows measure
// (20.1875px for its 16px default font). Layout asks the same face for the
// height a line box takes with line-height: normal, so a row here is as tall as
// the text inside it.
func (s *Session) listboxRowHeight(obj *layout.Object) float32 {
	size := float32(16)
	var slot frame.FontSlot
	if obj.Style != nil {
		size = obj.Style.FontSize
		slot = obj.Style.FontSlot()
	}
	if size <= 0 {
		return 0
	}
	if s.metrics != nil {
		// LineMetrics answers in device pixels for the size it is given, so
		// asking for the CSS size returns CSS heights.
		a, d, h := s.metrics.LineMetrics(int32(size), slot)
		switch {
		case h > 0:
			return float32(h)
		case a+d > 0:
			return float32(a + d)
		}
	}
	return size * 1.25
}

func rectFContains(r frame.RectF, x, y float32) bool {
	return x >= r.X0 && x < r.X1 && y >= r.Y0 && y < r.Y1
}
