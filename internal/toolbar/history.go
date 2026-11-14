package toolbar

import (
	"strings"
	"sync"
)

type Request struct {
	Method      string
	ContentType string
	Body        string
}

func (r Request) IsPost() bool {
	return strings.EqualFold(strings.TrimSpace(r.Method), "POST")
}

type entry struct {
	url     string
	request Request
	scroll  int32
}

type History struct {
	mu      sync.Mutex
	entries []entry
	index   int
}

func NewHistory() *History {
	return &History{entries: []entry{}, index: -1}
}

const MaxHistoryEntries = 50

func (h *History) Push(url string) {
	h.PushRequest(url, Request{})
}

func (h *History) PushRequest(url string, req Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index >= 0 && h.index < len(h.entries)-1 {
		h.entries = h.entries[:h.index+1]
	}
	h.entries = append(h.entries, entry{url: url, request: req})
	h.index = len(h.entries) - 1
	if drop := len(h.entries) - MaxHistoryEntries; drop > 0 {
		copy(h.entries, h.entries[drop:])
		h.entries = h.entries[:MaxHistoryEntries]
		h.index -= drop
	}
}

func (h *History) Back() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.backLocked()
	return e.url, ok
}

func (h *History) Forward() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.forwardLocked()
	return e.url, ok
}

func (h *History) PeekBack() (url string, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekBackLocked()
	return e.url, h.index, ok
}

func (h *History) PeekForward() (url string, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekForwardLocked()
	return e.url, h.index, ok
}

func (h *History) PeekBackRequest() (url string, req Request, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekBackLocked()
	return e.url, e.request, h.index, ok
}

func (h *History) PeekForwardRequest() (url string, req Request, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekForwardLocked()
	return e.url, e.request, h.index, ok
}

func (h *History) SetCurrentScroll(y int32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index >= 0 && h.index < len(h.entries) {
		h.entries[h.index].scroll = y
	}
}

func (h *History) PeekBackState() (url string, req Request, scroll int32, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekBackLocked()
	return e.url, e.request, e.scroll, h.index, ok
}

func (h *History) PeekForwardState() (url string, req Request, scroll int32, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekForwardLocked()
	return e.url, e.request, e.scroll, h.index, ok
}

func (h *History) CurrentRequest() (url string, req Request, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index < 0 || h.index >= len(h.entries) {
		return "", Request{}, false
	}
	e := h.entries[h.index]
	return e.url, e.request, true
}

func (h *History) ClearCurrentRequest() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index >= 0 && h.index < len(h.entries) {
		h.entries[h.index].request = Request{}
	}
}

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

func (h *History) ReplaceCurrent(url string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index < 0 || h.index >= len(h.entries) || url == "" {
		return
	}
	h.entries[h.index].url = url
}

func (h *History) backLocked() (entry, bool) {
	if h.index <= 0 {
		return entry{}, false
	}
	h.index--
	return h.entries[h.index], true
}

func (h *History) forwardLocked() (entry, bool) {
	if h.index < 0 || h.index >= len(h.entries)-1 {
		return entry{}, false
	}
	h.index++
	return h.entries[h.index], true
}

func (h *History) peekBackLocked() (entry, bool) {
	if h.index <= 0 {
		return entry{}, false
	}
	return h.entries[h.index-1], true
}

func (h *History) peekForwardLocked() (entry, bool) {
	if h.index < 0 || h.index >= len(h.entries)-1 {
		return entry{}, false
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
	return h.entries[h.index].url
}

func (h *History) Entries() ([]string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.entries))
	for i, e := range h.entries {
		out[i] = e.url
	}
	return out, h.index
}

func RestoreHistory(entries []string, index int) *History {
	if drop := len(entries) - MaxHistoryEntries; drop > 0 {
		entries = entries[drop:]
		index -= drop
	}
	h := &History{entries: make([]entry, len(entries)), index: index}
	for i, url := range entries {
		h.entries[i] = entry{url: url}
	}
	if h.index < -1 {
		h.index = -1
	}
	if h.index > len(h.entries)-1 {
		h.index = len(h.entries) - 1
	}
	return h
}
