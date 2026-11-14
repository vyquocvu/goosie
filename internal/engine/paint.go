package engine

import (
	"fmt"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/style"
)

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

func (s *Session) Paint(scale float32) *paint.List {
	s.workCounts.displayListBuilds.Add(1)
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
			markedLen := len([]rune(s.marked))
			if controlEditable(s.focus) || (dom.IsContentEditable(s.focus) && s.markedHasNode) {
				marked = ""
			}
			b.SetFocus(obj, s.caret, marked, markedLen)
		}
	}
	if spans := s.selectionSpans(); len(spans) > 0 {
		b.SetSelection(spans)
	}
	if len(s.findMarks) > 0 {
		marks := make([]paint.FindMark, 0, len(s.findMarks))
		for _, m := range s.findMarks {
			marks = append(marks, paint.FindMark{
				Key:    m.key,
				Rect:   frame.RectF4(m.rect.X0, m.rect.Y0, m.rect.X1, m.rect.Y1),
				Active: m.match == s.findActive,
			})
		}
		b.SetFind(marks)
	}
	if lb := s.listBoxLocked(s.selectPopup); lb != nil {
		b.SetSelectListBox(lb)
	}
	if s.validationAnchor != nil && s.validationMessage != "" {
		if obj := s.objectFor(s.validationAnchor); obj != nil {
			b.SetValidationBubble(&paint.ValidationBubble{Obj: obj, Message: s.validationMessage})
		}
	}
	if s.inspectNode != nil {
		if obj := s.objectFor(s.inspectNode); obj != nil {
			b.SetInspectHighlight(&paint.InspectHighlight{Obj: obj})
		}
	}
	b.Build(layout.ObjectID(1))
	list.SortStable()
	return list
}

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
