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
