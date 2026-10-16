package toolbar

import "sync"

// History is one tab's back/forward trail. The UI thread peeks and the
// navigation drain commits, so every method takes the mutex: cursor moves
// used to race between traverseTab and applyNavResult with no lock at all.
type History struct {
	mu      sync.Mutex
	entries []string
	index   int
}

func NewHistory() *History {
	return &History{entries: []string{}, index: -1}
}

func (h *History) Push(url string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index >= 0 && h.index < len(h.entries)-1 {
		h.entries = h.entries[:h.index+1]
	}
	h.entries = append(h.entries, url)
	h.index = len(h.entries) - 1
}

func (h *History) Back() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.backLocked()
}

func (h *History) Forward() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.forwardLocked()
}

// PeekBack reports the back target without moving the cursor: traversal
// commits it later, only if the load succeeds.
func (h *History) PeekBack() (url string, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	url, ok = h.peekBackLocked()
	return url, h.index, ok
}

// PeekForward is the forward PeekBack.
func (h *History) PeekForward() (url string, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	url, ok = h.peekForwardLocked()
	return url, h.index, ok
}

// Step commits a peeked traversal after its load succeeds: the cursor moves
// only if it still sits where the peek left it. Anything else committed
// meanwhile - a typed URL, another traversal - owns the cursor and the stale
// commit is dropped instead of yanking it.
func (h *History) Step(delta, base int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index != base {
		return false
	}
	if delta < 0 {
		_, ok := h.backLocked()
		return ok
	}
	if delta > 0 {
		_, ok := h.forwardLocked()
		return ok
	}
	return false
}

// ReplaceCurrent rewrites the entry under the cursor: a traversal that
// redirected lands its final URL where the requested one stood, so the
// address bar and a later reload name what displayed. The forward trail is
// untouched.
func (h *History) ReplaceCurrent(url string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index < 0 || h.index >= len(h.entries) || url == "" {
		return
	}
	h.entries[h.index] = url
}

func (h *History) backLocked() (string, bool) {
	if h.index <= 0 {
		return "", false
	}
	h.index--
	return h.entries[h.index], true
}

func (h *History) forwardLocked() (string, bool) {
	if h.index < 0 || h.index >= len(h.entries)-1 {
		return "", false
	}
	h.index++
	return h.entries[h.index], true
}

func (h *History) peekBackLocked() (string, bool) {
	if h.index <= 0 {
		return "", false
	}
	return h.entries[h.index-1], true
}

func (h *History) peekForwardLocked() (string, bool) {
	if h.index < 0 || h.index >= len(h.entries)-1 {
		return "", false
	}
	return h.entries[h.index+1], true
}

func (h *History) CanBack() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.index > 0
}

func (h *History) CanForward() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.index >= 0 && h.index < len(h.entries)-1
}

func (h *History) Current() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index < 0 || h.index >= len(h.entries) {
		return ""
	}
	return h.entries[h.index]
}

// Entries returns a copy of the URL trail and the current index, for saving.
func (h *History) Entries() ([]string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
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
