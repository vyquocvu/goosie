package engine

import "github.com/vyquocvu/goosie/internal/dom"

type testDriverDragCandidate struct {
	source    *dom.Node
	x, y      float32
	started   bool
	attempted bool
}

type dragState struct {
	source *dom.Node
	data   *dom.DataTransfer
	over   *dom.Node
	dropOK bool
	x, y   float32
}

func (s *Session) StartDrag(x, y float32) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.Doc == nil || s.drag != nil {
		return false
	}
	source := s.dragTargetAt(x, y)
	if !dom.IsDraggable(source) {
		return false
	}
	dt := dom.NewDragDataTransfer()
	if !s.dispatchDrag("dragstart", source, x, y, dt) {
		return false
	}
	s.drag = &dragState{source: source, data: dt, x: x, y: y}
	return true
}

func (s *Session) DraggableElementAt(x, y float32) *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	target := s.dragTargetAt(x, y)
	if !dom.IsDraggable(target) {
		return nil
	}
	return target
}

func (s *Session) TestDriverPointerAction(action string, x, y float32, button int) bool {
	switch action {
	case "down":
		if button != 0 {
			return false
		}
		source := s.DraggableElementAt(x, y)
		s.opMu.Lock()
		if source == nil || s.drag != nil {
			s.testDriverDrag = nil
		} else {
			s.testDriverDrag = &testDriverDragCandidate{source: source, x: x, y: y}
		}
		s.opMu.Unlock()
		return false
	case "move":
		s.opMu.Lock()
		candidate := s.testDriverDrag
		active := s.drag != nil
		shouldStart := false
		var source *dom.Node
		var startX, startY float32
		if candidate != nil && !candidate.started && !candidate.attempted {
			dx, dy := float64(x-candidate.x), float64(y-candidate.y)
			if dx*dx+dy*dy >= 25 {
				candidate.attempted = true
				shouldStart = true
				source, startX, startY = candidate.source, candidate.x, candidate.y
			}
		} else if candidate != nil && candidate.started {
			active = true
		}
		s.opMu.Unlock()
		if shouldStart {
			started := s.DraggableElementAt(startX, startY) == source && s.StartDrag(startX, startY)
			s.opMu.Lock()
			if s.testDriverDrag == candidate {
				s.testDriverDrag.started = started
			}
			s.opMu.Unlock()
			active = started
		}
		if active {
			s.DragTo(x, y)
		}
		return active
	case "up":
		s.opMu.Lock()
		candidate := s.testDriverDrag
		s.testDriverDrag = nil
		active := candidate != nil && candidate.started
		s.opMu.Unlock()
		if active {
			s.EndDrag(x, y)
		}
		return active
	}
	return false
}

func (s *Session) CancelTestDriverDrag() bool {
	s.opMu.Lock()
	candidate := s.testDriverDrag
	s.testDriverDrag = nil
	active := candidate != nil && candidate.started
	s.opMu.Unlock()
	if active {
		s.CancelDrag()
	}
	return candidate != nil
}

func (s *Session) DragTo(x, y float32) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	d := s.drag
	if d == nil {
		return
	}
	d.x, d.y = x, y
	target := s.dragTargetAt(x, y)
	if target != d.over {
		if d.over != nil {
			s.dispatchDragRelated("dragleave", d.over, target, x, y, d.data)
		}
		if target != nil {
			s.dispatchDragRelated("dragenter", target, d.over, x, y, d.data)
		}
		d.over = target
		d.dropOK = false
	}
	if target != nil {
		s.dispatchDrag("dragover", target, x, y, d.data)
		d.dropOK = d.data.CurrentOperation() != "none"
	} else {
		d.data.SetCurrentOperation("none")
		d.dropOK = false
	}
	s.dispatchDrag("drag", d.source, x, y, d.data)
}

func (s *Session) EndDrag(x, y float32) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	d := s.drag
	if d == nil {
		return
	}
	target := s.dragTargetAt(x, y)
	dropOK := d.dropOK
	if target != d.over {
		if d.over != nil {
			s.dispatchDragRelated("dragleave", d.over, target, x, y, d.data)
		}
		if target != nil {
			s.dispatchDragRelated("dragenter", target, d.over, x, y, d.data)
			s.dispatchDrag("dragover", target, x, y, d.data)
			dropOK = d.data.CurrentOperation() != "none"
		} else {
			d.data.SetCurrentOperation("none")
			dropOK = false
		}
		d.over = target
	}
	if dropOK && target != nil && d.data.CurrentOperation() != "none" {
		s.dispatchDrag("drop", target, x, y, d.data)
	}
	s.dispatchDrag("dragend", d.source, x, y, d.data)
	s.drag = nil
}

func (s *Session) CancelDrag() {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	d := s.drag
	if d == nil {
		return
	}
	if d.over != nil {
		s.dispatchDragRelated("dragleave", d.over, nil, d.x, d.y, d.data)
	}
	d.data.SetCurrentOperation("none")
	s.dispatchDrag("dragend", d.source, d.x, d.y, d.data)
	s.drag = nil
}

func (s *Session) Dragging() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.drag != nil
}

func (s *Session) dragTargetAt(x, y float32) *dom.Node {
	n := s.nodeAt(x, y)
	for n != nil && !n.Element() {
		n = n.Parent
	}
	return n
}

func (s *Session) dispatchDrag(typ string, target *dom.Node, x, y float32, dt *dom.DataTransfer) bool {
	return s.dispatchDragRelated(typ, target, nil, x, y, dt)
}

func (s *Session) dispatchDragRelated(typ string, target, related *dom.Node, x, y float32, dt *dom.DataTransfer) bool {
	mode := dom.DataTransferProtected
	if typ == "dragstart" {
		mode = dom.DataTransferReadWrite
	} else if typ == "drop" {
		mode = dom.DataTransferReadOnly
	}
	view := dt.NewEventView(typ, mode)
	defer func() {
		if typ == "dragstart" {
			dt.StoreEffectAllowed(view.EffectAllowed())
		}
		dt.SetMode(dom.DataTransferProtected)
		view.Disassociate()
	}()
	me := dom.NewMouseEvent(typ, true, true, int32(x), int32(y), int32(x), int32(y), 0)
	me.RelatedTarget = related
	ev := dom.NewTrustedDragEvent(typ, *me, view)
	notCanceled := dom.DispatchEvent(target, ev)
	if typ == "dragover" {
		operation := "none"
		if !notCanceled {
			operation = dom.DragOperation(view.EffectAllowed(), view.DropEffect())
		}
		dt.SetCurrentOperation(operation)
	} else if typ == "drop" {
		if notCanceled {
			dt.SetCurrentOperation("none")
		} else {
			dt.SetCurrentOperation(view.DropEffect())
		}
	}
	return notCanceled
}
