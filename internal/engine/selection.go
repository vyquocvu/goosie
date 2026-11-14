package engine

import (
	"sort"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

type selPos struct {
	word int
	rune int
}

type SelectDirection int

const (
	SelectLeft SelectDirection = iota
	SelectRight
	SelectUp
	SelectDown
)

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

func (s *Session) HasTextAt(x, y float32) bool {
	for _, b := range s.textLineBands() {
		if x >= b.x0 && x < b.x1 && y >= b.y0 && y < b.y1 {
			return true
		}
	}
	return false
}

type textBand struct{ x0, y0, x1, y1 float32 }

func (s *Session) textLineBands() []textBand {
	words := s.selWords()
	bands := make([]textBand, 0, len(words))
	for _, w := range words {
		x0, y0, x1, y1 := textLineRect(w)
		bands = append(bands, textBand{x0, y0, x1, y1})
	}
	sort.Slice(bands, func(i, j int) bool { return bands[i].y0 < bands[j].y0 })
	merged := bands[:0]
	for _, b := range bands {
		if n := len(merged); n > 0 {
			last := &merged[n-1]
			if b.y0 < last.y1 && b.y1 > last.y0 && b.x0 <= last.x1 && b.x1 >= last.x0 {
				if b.x0 < last.x0 {
					last.x0 = b.x0
				}
				if b.x1 > last.x1 {
					last.x1 = b.x1
				}
				if b.y0 < last.y0 {
					last.y0 = b.y0
				}
				if b.y1 > last.y1 {
					last.y1 = b.y1
				}
				continue
			}
		}
		merged = append(merged, b)
	}
	return merged
}

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

func lineWordAtX(words []*layout.Object, start, end int, x float32) int {
	for i := start; i <= end; i++ {
		_, _, x1, _ := words[i].BorderRect()
		if x < x1 {
			return i
		}
	}
	return end
}

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

func (s *Session) SelectAt(x, y float32) bool {
	had := s.HasSelection()
	p := s.posAt(s.selWords(), x, y)
	s.selAnchor = p
	s.selHead = p
	s.selActive = true
	return had
}

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

func (s *Session) HasSelection() bool {
	_, _, ok := s.orderedSel()
	return ok
}

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

func (s *Session) selectionSpans() []paint.SelSpan {
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
		spans = append(spans, paint.SelSpan{Key: words[i].Key, Start: start, End: end})
	}
	return spans
}

func (s *Session) imeMarkedSpans() []paint.SelSpan {
	if s.Arena == nil {
		return nil
	}
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
	start := s.markedOffset
	end := start + len([]rune(s.marked))
	return []paint.SelSpan{{Key: markedObj.Key, Start: start, End: end}}
}

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
