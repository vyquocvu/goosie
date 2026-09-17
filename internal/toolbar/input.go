package toolbar

func (s *State) InsertRune(r rune) {
	if s.Focus != FocusAddress {
		return
	}
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
	if s.Focus != FocusAddress || s.Cursor <= 0 {
		return
	}
	runes := []rune(s.Input)
	before := runes[:s.Cursor-1]
	after := runes[s.Cursor:]
	s.Input = string(append(before, after...))
	s.Cursor--
}

func (s *State) MoveCursorLeft() {
	if s.Cursor > 0 {
		s.Cursor--
	}
}

func (s *State) MoveCursorRight() {
	if s.Cursor < len([]rune(s.Input)) {
		s.Cursor++
	}
}

func (s *State) MoveCursorStart() {
	s.Cursor = 0
}

func (s *State) MoveCursorEnd() {
	s.Cursor = len([]rune(s.Input))
}

func (s *State) SelectAll() {
	s.Cursor = len([]rune(s.Input))
}
