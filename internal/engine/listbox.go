package engine

import (
	"math"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

const listboxInset = float32(1)

func (s *Session) selectPopupNode() *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.selectPopup
}

func (s *Session) SelectPopupOpen() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.selectPopup != nil
}

func (s *Session) SelectPopupRows() []paint.ListBoxRow {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	lb := s.listBoxLocked(s.selectPopup)
	if lb == nil {
		return nil
	}
	return lb.Rows
}

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

func (s *Session) openSelectPopup(sel *dom.Node, x, y float32) {
	s.FocusControl(x, y)
	s.opMu.Lock()
	s.selectPopup = sel
	s.opMu.Unlock()
}

func (s *Session) clickSelectPopup(x, y float32) bool {
	s.opMu.Lock()
	lb := s.listBoxLocked(s.selectPopup)
	s.opMu.Unlock()
	if lb == nil {
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
			return true
		}
		s.pickListboxRow(row.Index)
		return true
	}
	s.CloseSelectPopup()
	return true
}

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
	dom.DispatchEvent(sel, dom.NewTrustedEvent("input", true, false))
	dom.DispatchEvent(sel, dom.NewTrustedEvent("change", true, false))
	dom.DispatchEvent(options[index], dom.NewTrustedEvent("click", true, true))
}

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
	visible := len(options)
	above := false
	if vpH > 0 {
		room := func(space float32) int {
			if space <= 2*listboxInset {
				return 0
			}
			return int((space - 2*listboxInset) / rowH)
		}
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

func (s *Session) windowScrollY() float32 {
	if s.jsRT == nil {
		return 0
	}
	return float32(s.jsRT.WindowScrollY())
}

func (s *Session) listboxRowHeight(obj *layout.Object) float32 {
	size := float32(16)
	if obj.Style != nil && obj.Style.FontSize > 0 {
		size = obj.Style.FontSize
	}
	content := math.Floor(float64(1.2)*float64(size)*64) / 64
	return float32(content) + 1
}

func rectFContains(r frame.RectF, x, y float32) bool {
	return x >= r.X0 && x < r.X1 && y >= r.Y0 && y < r.Y1
}
