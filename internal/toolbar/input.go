package toolbar

import "unicode"

func (s *State) InsertRune(r rune) {
	if s.Focus != FocusAddress {
		return
	}
	s.deleteSelection()
	runes := []rune(s.Input)
	if s.Cursor < 0 || s.Cursor > len(runes) {
		s.Cursor = len(runes)
	}
	before := runes[:s.Cursor]
	after := runes[s.Cursor:]
	s.Input = string(append(before, append([]rune{r}, after...)...))
	s.Cursor++
}

func (s *State) DeleteBackward() {
	if s.Focus != FocusAddress {
		return
	}
	if s.hasSelection() {
		s.deleteSelection()
		return
	}
	if s.Cursor <= 0 {
		return
	}
	runes := []rune(s.Input)
	before := runes[:s.Cursor-1]
	after := runes[s.Cursor:]
	s.Input = string(append(before, after...))
	s.Cursor--
}

func (s *State) DeleteForward() {
	if s.Focus != FocusAddress {
		return
	}
	if s.hasSelection() {
		s.deleteSelection()
		return
	}
	runes := []rune(s.Input)
	if s.Cursor < 0 || s.Cursor >= len(runes) {
		return
	}
	before := runes[:s.Cursor]
	after := runes[s.Cursor+1:]
	s.Input = string(append(before, after...))
}

func (s *State) MoveCursorLeft() {
	s.moveCursorLeft(false)
}

func (s *State) MoveCursorRight() {
	s.moveCursorRight(false)
}

func (s *State) MoveCursorStart() {
	s.moveCursorStart(false)
}

func (s *State) MoveCursorEnd() {
	s.moveCursorEnd(false)
}

func (s *State) SelectAll() {
	if s.Focus != FocusAddress {
		return
	}
	n := len([]rune(s.Input))
	s.SelStart = 0
	s.SelEnd = n
	s.Cursor = n
}

func (s *State) Copy() {
	if !s.hasSelection() {
		return
	}
	runes := []rune(s.Input)
	sMin := s.selMin()
	sMax := s.selMax()
	if sMin < 0 {
		sMin = 0
	}
	if sMax > len(runes) {
		sMax = len(runes)
	}
	if s.Clipboard != nil {
		s.Clipboard.Write(string(runes[sMin:sMax]))
	}
}

func (s *State) Paste() {
	if s.Focus != FocusAddress || s.Clipboard == nil {
		return
	}
	text := s.Clipboard.Read()
	if text == "" {
		return
	}
	s.deleteSelection()
	runes := []rune(s.Input)
	if s.Cursor < 0 || s.Cursor > len(runes) {
		s.Cursor = len(runes)
	}
	pasted := []rune(text)
	before := runes[:s.Cursor]
	after := runes[s.Cursor:]
	s.Input = string(append(before, append(pasted, after...)...))
	s.Cursor += len(pasted)
}

func (s *State) Cut() {
	if !s.hasSelection() {
		return
	}
	s.Copy()
	s.deleteSelection()
}

func (s *State) deleteSelection() {
	if !s.hasSelection() {
		return
	}
	runes := []rune(s.Input)
	sMin := s.selMin()
	sMax := s.selMax()
	if sMin < 0 {
		sMin = 0
	}
	if sMax > len(runes) {
		sMax = len(runes)
	}
	before := runes[:sMin]
	after := runes[sMax:]
	s.Input = string(append(before, after...))
	s.Cursor = sMin
	s.clearSelection()
}

func (s *State) moveCursorLeft(shift bool) {
	if s.Cursor <= 0 {
		if !shift {
			s.clearSelection()
		}
		return
	}
	if !shift {
		if s.hasSelection() {
			s.Cursor = s.selMin()
			s.clearSelection()
			return
		}
		s.Cursor--
		s.clearSelection()
		return
	}
	if !s.hasSelection() {
		s.SelStart = s.Cursor
		s.SelEnd = s.Cursor
	}
	s.Cursor--
	s.SelEnd = s.Cursor
}

func (s *State) moveCursorRight(shift bool) {
	n := len([]rune(s.Input))
	if s.Cursor >= n {
		if !shift {
			s.clearSelection()
		}
		return
	}
	if !shift {
		if s.hasSelection() {
			s.Cursor = s.selMax()
			s.clearSelection()
			return
		}
		s.Cursor++
		s.clearSelection()
		return
	}
	if !s.hasSelection() {
		s.SelStart = s.Cursor
		s.SelEnd = s.Cursor
	}
	s.Cursor++
	s.SelEnd = s.Cursor
}

func (s *State) moveCursorStart(shift bool) {
	if !shift {
		if s.hasSelection() {
			s.Cursor = s.selMin()
			s.clearSelection()
			return
		}
		s.Cursor = 0
		s.clearSelection()
		return
	}
	if !s.hasSelection() {
		s.SelStart = s.Cursor
		s.SelEnd = s.Cursor
	}
	s.Cursor = 0
	s.SelEnd = s.Cursor
}

func (s *State) moveCursorEnd(shift bool) {
	n := len([]rune(s.Input))
	if !shift {
		if s.hasSelection() {
			s.Cursor = s.selMax()
			s.clearSelection()
			return
		}
		s.Cursor = n
		s.clearSelection()
		return
	}
	if !s.hasSelection() {
		s.SelStart = s.Cursor
		s.SelEnd = s.Cursor
	}
	s.Cursor = n
	s.SelEnd = s.Cursor
}

func (s *State) moveWordLeft(shift bool) {
	runes := []rune(s.Input)
	pos := s.Cursor
	if pos <= 0 {
		if !shift {
			s.clearSelection()
		}
		return
	}
	pos = prevWordBoundary(runes, pos)
	if !shift {
		s.Cursor = pos
		s.clearSelection()
		return
	}
	if !s.hasSelection() {
		s.SelStart = s.Cursor
		s.SelEnd = s.Cursor
	}
	s.Cursor = pos
	s.SelEnd = s.Cursor
}

func (s *State) moveWordRight(shift bool) {
	runes := []rune(s.Input)
	pos := s.Cursor
	n := len(runes)
	if pos >= n {
		if !shift {
			s.clearSelection()
		}
		return
	}
	pos = nextWordBoundary(runes, pos)
	if !shift {
		s.Cursor = pos
		s.clearSelection()
		return
	}
	if !s.hasSelection() {
		s.SelStart = s.Cursor
		s.SelEnd = s.Cursor
	}
	s.Cursor = pos
	s.SelEnd = s.Cursor
}

func (s *State) deleteWordBackward() {
	if s.hasSelection() {
		s.deleteSelection()
		return
	}
	runes := []rune(s.Input)
	if s.Cursor <= 0 {
		return
	}
	pos := prevWordBoundary(runes, s.Cursor)
	before := runes[:pos]
	after := runes[s.Cursor:]
	s.Input = string(append(before, after...))
	s.Cursor = pos
}

func prevWordBoundary(runes []rune, pos int) int {
	if pos <= 0 {
		return 0
	}
	i := pos - 1
	for i > 0 && unicode.IsSpace(runes[i]) {
		i--
	}
	if i >= 0 && isWordChar(runes[i]) {
		for i > 0 && isWordChar(runes[i-1]) {
			i--
		}
	}
	if i < 0 {
		return 0
	}
	return i
}

func nextWordBoundary(runes []rune, pos int) int {
	n := len(runes)
	if pos >= n {
		return n
	}
	i := pos
	if isWordChar(runes[i]) {
		for i < n && isWordChar(runes[i]) {
			i++
		}
	}
	for i < n && unicode.IsSpace(runes[i]) {
		i++
	}
	return i
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
