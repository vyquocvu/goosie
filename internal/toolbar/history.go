package toolbar

type History struct {
	entries []string
	index   int
}

func NewHistory() *History {
	return &History{entries: []string{}, index: -1}
}

func (h *History) Push(url string) {
	if h.index >= 0 && h.index < len(h.entries)-1 {
		h.entries = h.entries[:h.index+1]
	}
	h.entries = append(h.entries, url)
	h.index = len(h.entries) - 1
}

func (h *History) Back() (string, bool) {
	if !h.CanBack() {
		return "", false
	}
	h.index--
	return h.entries[h.index], true
}

func (h *History) Forward() (string, bool) {
	if !h.CanForward() {
		return "", false
	}
	h.index++
	return h.entries[h.index], true
}

func (h *History) CanBack() bool {
	return h.index > 0
}

func (h *History) CanForward() bool {
	return h.index >= 0 && h.index < len(h.entries)-1
}

func (h *History) Current() string {
	if h.index < 0 || h.index >= len(h.entries) {
		return ""
	}
	return h.entries[h.index]
}

// Entries returns a copy of the URL trail and the current index, for saving.
func (h *History) Entries() ([]string, int) {
	out := make([]string, len(h.entries))
	copy(out, h.entries)
	return out, h.index
}

// RestoreHistory rebuilds a history from saved entries, clamping an
// out-of-range index instead of trusting the state file.
func RestoreHistory(entries []string, index int) *History {
	h := &History{entries: make([]string, len(entries)), index: index}
	copy(h.entries, entries)
	if h.index < -1 {
		h.index = -1
	}
	if h.index > len(h.entries)-1 {
		h.index = len(h.entries) - 1
	}
	return h
}
