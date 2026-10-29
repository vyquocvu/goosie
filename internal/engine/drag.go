package engine

import "github.com/vyquocvu/goosie/internal/dom"

type testDriverDragCandidate struct {
	source    *dom.Node
	x, y      float32
	started   bool
	attempted bool
}

// dragState is one in-flight drag-and-drop operation.
//
// The state lives on the Session (guarded by opMu like every other
// interaction field): the drag data store is shared by every event in the
// sequence, and enter/leave tracking needs the previous target to decide
// which dragleave to fire.
type dragState struct {
	source *dom.Node         // draggable element the drag started on
	data   *dom.DataTransfer // one store shared by every event in this drag
	over   *dom.Node         // element the last dragenter/dragover fired on
	dropOK bool              // the last dragover on `over` allowed a drop
	x, y   float32           // last drag position, for CancelDrag's events
}

// StartDrag initiates a drag-and-drop operation at (x, y).
//
// It hit-tests the layout arena for the element under the point and requires
// it to be draggable (explicit draggable="true", or the img / href-link
// defaults). A new DataTransfer is created for the operation and a cancelable
// dragstart fires on the source; when a listener cancels it, no drag begins
// and StartDrag reports false. Only one drag may run per session.
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
		return false // dragstart canceled: no drag operation begins
	}
	// Drop-target tracking starts empty. The first dragenter therefore has a
	// null relatedTarget; the source only receives drag/dragend events unless
	// it is independently entered as a drop target.
	s.drag = &dragState{source: source, data: dt, x: x, y: y}
	return true
}

// DraggableElementAt returns the draggable element under a document-space
// point, or nil when the point is not over a draggable element. Hosts use this
// to distinguish a pending pointer gesture from ordinary text selection and
// activation without starting the drag before the platform threshold is met.
func (s *Session) DraggableElementAt(x, y float32) *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	target := s.dragTargetAt(x, y)
	if !dom.IsDraggable(target) {
		return nil
	}
	return target
}

// TestDriverPointerAction maps the WPT adapter's pointer actions to the same
// drag lifecycle as native input. It returns true once the action belongs to
// an active drag so the adapter can suppress the later synthetic click.
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

// CancelTestDriverDrag cancels a drag created by the WPT pointer adapter or
// discards a pending candidate before dragstart.
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

// DragTo moves the active drag to (x, y), firing the movement sequence in the
// HTML spec's order: dragleave on the element just left, dragenter on the
// element entered, dragover on the element under the cursor, and drag on the
// source. Calling it with no active drag is a no-op.
//
// A drop over the final target is allowed only when dragover selected an
// operation permitted by the source's effectAllowed value.
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

// EndDrag finishes the active drag at (x, y).
//
// The final position participates in enter/leave tracking like a DragTo, so
// calling EndDrag directly after StartDrag still produces a well-formed
// sequence. When the element under the point has a non-none current drag
// operation, drop fires on it; then
// dragend fires on the source either way and the drag state is released.
// Calling it with no active drag is a no-op.
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

// CancelDrag aborts the active drag (the Escape path): dragleave fires on the
// current target, dragend on the source, and no drop ever fires. Calling it
// with no active drag is a no-op.
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

// Dragging reports whether a drag-and-drop operation is in flight.
func (s *Session) Dragging() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.drag != nil
}

// dragTargetAt returns the drop-target element under the point: the deepest
// hit-tested node lifted to its nearest element ancestor, so enter/leave
// tracking compares stable element identities even when the hit lands on a
// text node.
func (s *Session) dragTargetAt(x, y float32) *dom.Node {
	n := s.nodeAt(x, y)
	for n != nil && !n.Element() {
		n = n.Parent
	}
	return n
}

// dispatchDrag fires one drag event with the shared data store on target and
// reports whether the event was not canceled. Coordinates are document
// coordinates; screen coordinates use the same values because the headless
// engine has no window offset.
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
