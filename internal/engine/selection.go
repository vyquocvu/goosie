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

// SelectDirection is one arrow-key step for keyboard selection extension.
type SelectDirection int

const (
	SelectLeft SelectDirection = iota
	SelectRight
	SelectUp
	SelectDown
)

// The selection state. selActive is true from press until reflow or a new
// press; anchor is where the drag started, head where it currently reaches.
// Both live on Session next to focus state, which Reflow also keeps while
// selection dies, because word positions cannot survive an arena rebuild.

// ExtendSelectionByKey moves the selection head one rune left or right, or one
// visual line up or down, while the anchor stays where the selection started.
// With no selection it starts at the beginning of the document, so Shift+Arrow
// works before any pointer press. It reports whether the head moved.
func (s *Session) ExtendSelectionByKey(dir SelectDirection) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	words := s.selWords()
	if len(words) == 0 {
		return false
	}
	head := s.selHead
	if !s.selActive {
		head = selPos{}
	}
	if head.word < 0 || head.word >= len(words) {
		return false
	}
	next := head
	switch dir {
	case SelectLeft:
		if head.rune > 0 {
			next.rune--
		} else if head.word > 0 {
			next.word--
			next.rune = runeLen(words[next.word])
		} else {
			return false
		}
	case SelectRight:
		if head.rune < runeLen(words[head.word]) {
			next.rune++
		} else if head.word+1 < len(words) {
			next.word++
			next.rune = 0
		} else {
			return false
		}
	case SelectUp, SelectDown:
		start, end := lineBounds(words, head.word)
		if dir == SelectDown {
			if end+1 >= len(words) {
				return false
			}
			start, end = lineBounds(words, end+1)
		} else {
			if start == 0 {
				return false
			}
			start, end = lineBounds(words, start-1)
		}
		x := s.runeX(words[head.word], head.rune)
		target := lineWordAtX(words, start, end, x)
		next = selPos{word: target, rune: s.runeOffset(words[target], x)}
	default:
		return false
	}
	if next == head {
		return false
	}
	if !s.selActive {
		s.selAnchor = head
		s.selActive = true
	}
	s.selHead = next
	return true
}

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
			if obj.Style == nil || obj.Style.UserSelect != "none" {
				if x0, y0, x1, y1 := obj.BorderRect(); x0 < x1 && y0 < y1 {
					words = append(words, obj)
				}
			}
		}
		for k := obj.FirstKid; k != 0; k = objs[k].NextSibling {
			walk(k)
		}
	}
	walk(1)
	return words
}

// HasTextAt reports whether a document-space point lies on a selectable text
// line. Hover uses it to ask for the I-beam.
//
// The hit is the line box, not the glyph box. line-height adds leading above
// and below the glyphs and between wrapped lines; testing only BorderRect
// flips the cursor to the arrow in every one of those gaps, and each flip
// asks AppKit to update the pointer over a layer it does not own.
func (s *Session) HasTextAt(x, y float32) bool {
	words := s.selWords()
	for i := 0; i < len(words); {
		x0, y0, x1, y1 := textLineRect(words[i])
		j := i + 1
		for j < len(words) {
			ox0, oy0, ox1, oy1 := textLineRect(words[j])
			if oy0 >= y1 || oy1 <= y0 {
				break
			}
			if ox0 < x0 {
				x0 = ox0
			}
			if ox1 > x1 {
				x1 = ox1
			}
			if oy0 < y0 {
				y0 = oy0
			}
			if oy1 > y1 {
				y1 = oy1
			}
			j++
		}
		if x >= x0 && x < x1 && y >= y0 && y < y1 {
			return true
		}
		i = j
	}
	return false
}

// textLineRect is the word's border box grown to its line box. An explicit
// line-height taller than the glyph box leaves leading outside BorderRect;
// half of that difference sits above the glyphs and half below, which is the
// strip the caret and the I-beam share. A normal line-height (LineHeight <= 0)
// already matches the font's own box, so the border rect stands.
func textLineRect(w *layout.Object) (x0, y0, x1, y1 float32) {
	if w == nil {
		return
	}
	x0, y0, x1, y1 = w.BorderRect()
	if w.Style == nil || w.Style.LineHeight <= 0 {
		return
	}
	h := y1 - y0
	if w.Style.LineHeight <= h {
		return
	}
	extra := (w.Style.LineHeight - h) / 2
	y0 -= extra
	y1 += extra
	return
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
	lineStart, lineEnd := lineBounds(words, base)
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

// lineBounds returns the first and last word index sharing idx's vertical
// band: the visual line a selection position sits on.
func lineBounds(words []*layout.Object, idx int) (start, end int) {
	_, by0, _, by1 := words[idx].BorderRect()
	start, end = idx, idx
	for start > 0 {
		_, y0, _, y1 := words[start-1].BorderRect()
		if y1 <= by0 || y0 >= by1 {
			break
		}
		start--
	}
	for end < len(words)-1 {
		_, y0, _, y1 := words[end+1].BorderRect()
		if y1 <= by0 || y0 >= by1 {
			break
		}
		end++
	}
	return start, end
}

// lineWordAtX returns the word in a visual line under x. A point in the gap
// between words, or left of the line, resolves to the next word's start; a
// point past the line resolves to the last word's end.
func lineWordAtX(words []*layout.Object, start, end int, x float32) int {
	for i := start; i <= end; i++ {
		_, _, x1, _ := words[i].BorderRect()
		if x < x1 {
			return i
		}
	}
	return end
}

// runeX is the inverse of runeOffset: the x coordinate after idx runes,
// advancing the pen with the same font metrics the paint builder uses.
// When the word carries a GlyphOrder (bidi-reordered), the logical index is
// mapped through the visual permutation so the caret lands where the glyph
// edge actually sits on screen.
func (s *Session) runeX(w *layout.Object, idx int) float32 {
	runes := []rune(w.Node.DataContent)
	x0, _, _, _ := w.BorderRect()
	if idx <= 0 {
		return x0
	}
	if idx > len(runes) {
		idx = len(runes)
	}
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
	order := w.GlyphOrder
	if order != nil {
		visualSlot := len(order)
		for k, log := range order {
			if log == idx {
				visualSlot = k
				break
			}
		}
		pen := int32(0)
		for k := 0; k < visualSlot; k++ {
			r := runes[order[k]]
			if k > 0 {
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
		}
		return x0 + float32((pen+32)>>6)
	}
	pen := int32(0)
	for i := 0; i < idx; i++ {
		if i > 0 {
			pen += letterSpacing * 64
		}
		if m != nil {
			pen += m.GlyphAdvanceFixed(fontSize, runes[i], slot)
		} else {
			pen += int32(fontSize) / 2 * 64
		}
		if runes[i] == ' ' {
			pen += wordSpacing * 64
		}
	}
	return x0 + float32((pen+32)>>6)
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
// selects before it. When the word carries a GlyphOrder (bidi-reordered), the
// walk iterates visual slots and maps each back to its logical rune, so the
// returned index is the logical offset the rest of the selection code expects.
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
	order := w.GlyphOrder
	if order != nil {
		pen := int32(0)
		for k := range runes {
			r := runes[order[k]]
			if k > 0 {
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
				return order[k]
			}
		}
		return len(runes)
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
// selection in progress. It reports whether anything visible changed: a press
// collapses to an empty point, so it only clears a previous highlight. A press
// with no highlight before and none after changes no pixel and returns false,
// letting the caller skip a repaint (and the full re-raster flash behind it).
func (s *Session) SelectAt(x, y float32) bool {
	had := s.HasSelection()
	p := s.posAt(s.selWords(), x, y)
	s.selAnchor = p
	s.selHead = p
	s.selActive = true
	return had
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
	// IME composition: highlight the marked range in contenteditable.
	if s.imeComposing && s.markedHasNode && s.marked != "" {
		if spans := s.imeMarkedSpans(); len(spans) > 0 {
			return spans
		}
	}
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

// imeMarkedSpans returns the selection spans for the IME marked range in
// contenteditable. The marked text is already in the DOM node; this finds the
// layout object for the node and returns a span covering the marked range.
func (s *Session) imeMarkedSpans() []paint.SelSpan {
	if s.Arena == nil {
		return nil
	}
	// Find the layout object for the marked node.
	var markedObj *layout.Object
	for i := range s.Arena.Objects {
		if s.Arena.Objects[i].Node != nil && s.Arena.Objects[i].Node.ID == s.markedNode {
			markedObj = &s.Arena.Objects[i]
			break
		}
	}
	if markedObj == nil {
		return nil
	}
	// The marked range starts at markedOffset and extends for len(marked) runes.
	start := s.markedOffset
	end := start + len([]rune(s.marked))
	return []paint.SelSpan{{Obj: markedObj, Start: start, End: end}}
}

// SelectAll selects all text in the document. It returns true if there is text to select.
func (s *Session) SelectAll() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	words := s.selWords()
	if len(words) == 0 {
		return false
	}
	s.selAnchor = selPos{word: 0, rune: 0}
	lastWord := len(words) - 1
	s.selHead = selPos{word: lastWord, rune: runeLen(words[lastWord])}
	s.selActive = true
	return true
}

// ClearSelection removes any active document text selection.
func (s *Session) ClearSelection() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if !s.selActive {
		return false
	}
	s.selActive = false
	s.selAnchor = selPos{}
	s.selHead = selPos{}
	return true
}
