package engine

import (
	"fmt"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/style"
)

// PaintChecked validates geometry and scale before any device-coordinate
// conversion, then checks tile metadata bounds before handing off the list.
func (s *Session) PaintChecked(scale float32) (*paint.List, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := validateScale(float64(scale)); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("missing session")
	}
	if err := validateArena(s.Arena); err != nil {
		return nil, err
	}
	list := s.Paint(scale)
	if err := ValidateExtent(list.Bounds()); err != nil {
		return nil, err
	}
	return list, nil
}

// Paint builds a display list from a trusted session's layout arena. Binaries
// processing untrusted input must use PaintChecked instead.
//
// The list is mutable; the caller freezes it with Publish when the frame is
// ready. scale is the device-pixel ratio: a 2x Retina display passes 2.0, and
// the builder multiplies every CSS-pixel coordinate by it to produce device
// pixels.
func (s *Session) Paint(scale float32) *paint.List {
	s.workCounts.displayListBuilds.Add(1)
	// Size the slab from the box count so a first paint does not grow it
	// repeatedly: each growth copies every DisplayCmd published so far, and a
	// DisplayCmd is a wide value. A box emits a small handful of commands, so
	// 2x the box count covers the common case without over-reserving.
	n := 0
	if s.Arena != nil {
		n = 2 * len(s.Arena.Objects)
	}
	list := paint.NewList(n)
	b := paint.NewBuilder(list, s.Arena, scale, s.metrics)
	if s.jsRT != nil {
		b.SetScrollOffsetProvider(func(obj *layout.Object, prop string) float64 {
			if obj == nil || obj.Node == nil {
				return 0
			}
			return s.jsRT.ElementScrollOffset(obj.Node, prop)
		})
	}
	if s.focus != nil {
		if obj := s.objectFor(s.focus); obj != nil {
			marked := s.marked
			if controlEditable(s.focus) || (dom.IsContentEditable(s.focus) && s.markedHasNode) {
				// Text controls and live contenteditable ranges already include the
				// active IME text in the value or Text node.
				marked = ""
			}
			b.SetFocus(obj, s.caret, marked)
		}
	}
	if spans := s.selectionSpans(); len(spans) > 0 {
		b.SetSelection(spans)
	}
	if len(s.findMarks) > 0 {
		marks := make([]paint.FindMark, 0, len(s.findMarks))
		for _, m := range s.findMarks {
			marks = append(marks, paint.FindMark{
				Obj:    m.obj,
				Rect:   frame.RectF4(m.rect.X0, m.rect.Y0, m.rect.X1, m.rect.Y1),
				Active: m.match == s.findActive,
			})
		}
		b.SetFind(marks)
	}
	b.Build(layout.ObjectID(1))
	return list
}

// BackgroundColor returns the effective canvas background color.
// If html has a non-transparent background, that is used. Otherwise if body
// has a non-transparent background, that is used. If both are transparent,
// white RGB(255, 255, 255) is returned.
//
// A display:none root contributes nothing: with no box the background has
// nothing to propagate from, so html (or body) with display:none reads as
// absent here.
func (s *Session) BackgroundColor() frame.Color {
	if s.Doc != nil {
		if s.Doc.HTML != nil {
			if st, ok := s.Styles[s.Doc.HTML.ID]; ok && st.BackgroundColor.A > 0 && st.Display != style.DisplayNone {
				return frame.RGBA(st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B, st.BackgroundColor.A)
			}
		}
		if s.Doc.Body != nil {
			if st, ok := s.Styles[s.Doc.Body.ID]; ok && st.BackgroundColor.A > 0 && st.Display != style.DisplayNone {
				return frame.RGBA(st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B, st.BackgroundColor.A)
			}
		}
	}
	return frame.RGB(255, 255, 255)
}
