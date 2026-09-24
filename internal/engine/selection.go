package engine

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

// selPos is one end of a selection: an index into the tree-order word list
// plus a rune offset inside that word. Positions index the current arena, so
// they are only meaningful until the next Reflow rebuilds it.
type selPos struct {
	word int
	rune int
}

// The selection state. selActive is true from press until reflow or a new
// press; anchor is where the drag started, head where it currently reaches.
// Both live on Session next to focus state, which Reflow also keeps while
// selection dies, because word positions cannot survive an arena rebuild.

// selWords returns the laid-out text word boxes in document order, skipping
// whitespace-only runs the same way Find skips them when joining text.
func (s *Session) selWords() []*layout.Object {
	if s.Arena == nil {
		return nil
	}
	objs := s.Arena.Objects
	var words []*layout.Object
	var walk func(layout.ObjectID)
	walk = func(id layout.ObjectID) {
		if id == 0 || int(id) >= len(objs) {
			return
		}
		obj := &objs[id]
		if obj.Node != nil && obj.Node.Type == dom.NodeText && strings.TrimSpace(obj.Node.DataContent) != "" {
			if x0, y0, x1, y1 := obj.BorderRect(); x0 < x1 && y0 < y1 {
				words = append(words, obj)
			}
		}
		for k := obj.FirstKid; k != 0; k = objs[k].NextSibling {
			walk(k)
		}
	}
	walk(1)
	return words
}

// HasTextAt reports whether a laid-out word box contains the document-space
// point. Hover uses it to ask for the text cursor over selectable text.
func (s *Session) HasTextAt(x, y float32) bool {
	for _, w := range s.selWords() {
		x0, y0, x1, y1 := w.BorderRect()
		if x >= x0 && x < x1 && y >= y0 && y < y1 {
			return true
		}
	}
	return false
}

// posAt maps a document-space point to a selection position. It picks the
// word whose vertical band contains y (or the nearest band when y falls
// outside every word, e.g. past the end of the document), expands to the
// whole visual line, and splits horizontally by rune.
func (s *Session) posAt(words []*layout.Object, x, y float32) selPos {
	if len(words) == 0 {
		return selPos{}
	}
	base := -1
	for i, w := range words {
		_, y0, _, y1 := w.BorderRect()
		if y >= y0 && y < y1 {
			base = i
			break
		}
	}
	if base < 0 {
		base = nearestBand(words, y)
	}
	_, by0, _, by1 := words[base].BorderRect()
	lineStart, lineEnd := base, base
	for lineStart > 0 {
		_, y0, _, y1 := words[lineStart-1].BorderRect()
		if y1 <= by0 || y0 >= by1 {
			break
		}
		lineStart--
	}
	for lineEnd < len(words)-1 {
		_, y0, _, y1 := words[lineEnd+1].BorderRect()
		if y1 <= by0 || y0 >= by1 {
			break
		}
		lineEnd++
	}
	if x0, _, _, _ := words[lineStart].BorderRect(); x < x0 {
		return selPos{word: lineStart, rune: 0}
	}
	if _, _, x1, _ := words[lineEnd].BorderRect(); x >= x1 {
		return selPos{word: lineEnd, rune: runeLen(words[lineEnd])}
	}
	for i := lineStart; i <= lineEnd; i++ {
		x0, _, x1, _ := words[i].BorderRect()
		if x < x1 {
			if x >= x0 {
				return selPos{word: i, rune: s.runeOffset(words[i], x)}
			}
			return selPos{word: i, rune: 0}
		}
	}
	return selPos{word: lineEnd, rune: runeLen(words[lineEnd])}
}

// nearestBand returns the word whose vertical band is closest to y.
func nearestBand(words []*layout.Object, y float32) int {
	best, bestDist := 0, float32(0)
	for i, w := range words {
		_, y0, _, y1 := w.BorderRect()
		d := float32(0)
		if y < y0 {
			d = y0 - y
		} else if y >= y1 {
			d = y - y1
		}
		if i == 0 || d < bestDist {
			best, bestDist = i, d
		}
	}
	return best
}

func runeLen(w *layout.Object) int {
	if w.Node == nil {
		return 0
	}
	return len([]rune(w.Node.DataContent))
}

// runeOffset converts an x inside a word's box to a rune index, walking the
// same 26.6 fixed-point pen the paint builder places glyphs with, so the caret
// boundary lands where the glyph edge is. A click in the left part of a rune
// selects before it.
func (s *Session) runeOffset(w *layout.Object, x float32) int {
	runes := []rune(w.Node.DataContent)
	x0, _, _, _ := w.BorderRect()
	if x <= x0 {
		return 0
	}
	rel := x - x0
	fontSize := int32(16)
	letterSpacing, wordSpacing := int32(0), int32(0)
	var slot frame.FontSlot
	var m layout.Metrics
	if w.Style != nil {
		fontSize = int32(w.Style.FontSize)
		letterSpacing = int32(w.Style.LetterSpacing)
		wordSpacing = int32(w.Style.WordSpacing)
		slot = w.Style.FontSlot()
		m = s.metrics
	}
	pen := int32(0)
	for i, r := range runes {
		if i > 0 {
			pen += letterSpacing * 64
		}
		if m != nil {
			pen += m.GlyphAdvanceFixed(fontSize, r, slot)
		} else {
			pen += int32(fontSize) / 2 * 64
		}
		if r == ' ' {
			pen += wordSpacing * 64
		}
		next := (pen + 32) >> 6
		if rel < float32(next) {
			return i
		}
	}
	return len(runes)
}

// SelectAt starts a selection gesture at a document-space point, replacing any
// selection in progress. It always reports a change: a press may clear an
// existing highlight even when it lands on the same spot.
func (s *Session) SelectAt(x, y float32) bool {
	p := s.posAt(s.selWords(), x, y)
	s.selAnchor = p
	s.selHead = p
	s.selActive = true
	return true
}

// SelectTo extends the active selection to a document-space point. It reports
// whether the head moved, so a caller can skip repaints when a drag twitches
// within one rune boundary.
func (s *Session) SelectTo(x, y float32) bool {
	if !s.selActive {
		return false
	}
	p := s.posAt(s.selWords(), x, y)
	if p == s.selHead {
		return false
	}
	s.selHead = p
	return true
}

// HasSelection reports whether a non-empty span is selected.
func (s *Session) HasSelection() bool {
	_, _, ok := s.orderedSel()
	return ok
}

// orderedSel returns the selection ends in reading order. ok is false when no
// selection is active or the ends collapsed to the same point.
func (s *Session) orderedSel() (from, to selPos, ok bool) {
	if !s.selActive {
		return selPos{}, selPos{}, false
	}
	from, to = s.selAnchor, s.selHead
	if from.word > to.word || (from.word == to.word && from.rune > to.rune) {
		from, to = to, from
	}
	return from, to, from != to
}

// selCoverage resolves the selection against the current arena's word list.
// A head past the end of the words (a stale position after a partial arena
// change) reads as no selection rather than a panic.
func (s *Session) selCoverage() (words []*layout.Object, from, to selPos, ok bool) {
	from, to, ok = s.orderedSel()
	if !ok {
		return nil, selPos{}, selPos{}, false
	}
	words = s.selWords()
	if to.word >= len(words) {
		return nil, selPos{}, selPos{}, false
	}
	return words, from, to, true
}

// SelectionText joins the selected words, separated by spaces within a line
// and newlines where the band gap says a line break intervened.
func (s *Session) SelectionText() string {
	words, from, to, ok := s.selCoverage()
	if !ok {
		return ""
	}
	var b strings.Builder
	prev := -1
	for i := from.word; i <= to.word; i++ {
		w := words[i]
		runes := []rune(w.Node.DataContent)
		start, end := 0, len(runes)
		if i == from.word {
			start = from.rune
		}
		if i == to.word {
			end = to.rune
		}
		if start >= end {
			continue
		}
		if prev >= 0 {
			_, py0, _, py1 := words[prev].BorderRect()
			_, wy0, _, wy1 := words[i].BorderRect()
			if wy0 >= py1 || wy1 <= py0 {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(string(runes[start:end]))
		prev = i
	}
	return b.String()
}

// selectionSpans returns the paint-side highlight rects, one per selected
// word box, with edge words clipped to the selected rune range.
func (s *Session) selectionSpans() []paint.SelSpan {
	words, from, to, ok := s.selCoverage()
	if !ok {
		return nil
	}
	spans := make([]paint.SelSpan, 0, to.word-from.word+1)
	for i := from.word; i <= to.word; i++ {
		start, end := 0, runeLen(words[i])
		if i == from.word {
			start = from.rune
		}
		if i == to.word {
			end = to.rune
		}
		if start >= end {
			continue
		}
		spans = append(spans, paint.SelSpan{Obj: words[i], Start: start, End: end})
	}
	return spans
}
