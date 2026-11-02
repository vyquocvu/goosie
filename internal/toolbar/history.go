package toolbar

import (
	"strings"
	"sync"
)

// Request is the method and payload that produced a history entry. The zero
// value is a GET; only a form submission fills the body. Requests live in
// memory with the tab trail and are not written to the session file, so a
// restored entry always starts as a GET.
type Request struct {
	Method      string
	ContentType string
	Body        string
}

// IsPost reports whether replaying the entry needs the request body. Matching
// is case-insensitive because the method arrives from author markup.
func (r Request) IsPost() bool {
	return strings.EqualFold(strings.TrimSpace(r.Method), "POST")
}

type entry struct {
	url     string
	request Request
	// scroll is the viewport offset this page was left at. A traversal
	// restores it, so Back returns the reader to where they were reading
	// instead of to the top.
	scroll int32
}

// History is one tab's back/forward trail. The UI thread peeks and the
// navigation drain commits, so every method takes the mutex: cursor moves
// used to race between traverseTab and applyNavResult with no lock at all.
type History struct {
	mu      sync.Mutex
	entries []entry
	index   int
}

func NewHistory() *History {
	return &History{entries: []entry{}, index: -1}
}

// MaxHistoryEntries caps one tab's back/forward trail. A session that visits
// more pages drops its oldest visits; the newest stay reachable.
const MaxHistoryEntries = 50

func (h *History) Push(url string) {
	h.PushRequest(url, Request{})
}

// PushRequest records an entry together with the request that produced it, so
// a later traversal or reload can tell a POST result apart from a GET.
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

// PeekBack reports the back target without moving the cursor: traversal
// commits it later, only if the load succeeds.
func (h *History) PeekBack() (url string, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekBackLocked()
	return e.url, h.index, ok
}

// PeekForward is the forward PeekBack.
func (h *History) PeekForward() (url string, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekForwardLocked()
	return e.url, h.index, ok
}

// PeekBackRequest is PeekBack with the request that produced the target, so a
// caller can ask before replaying a POST.
func (h *History) PeekBackRequest() (url string, req Request, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekBackLocked()
	return e.url, e.request, h.index, ok
}

// PeekForwardRequest is the forward PeekBackRequest.
func (h *History) PeekForwardRequest() (url string, req Request, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekForwardLocked()
	return e.url, e.request, h.index, ok
}

// SetCurrentScroll records where the reader left the page under the cursor.
// Navigation captures it before leaving; traversals read the target entry's
// value through PeekBackState / PeekForwardState.
func (h *History) SetCurrentScroll(y int32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index >= 0 && h.index < len(h.entries) {
		h.entries[h.index].scroll = y
	}
}

// PeekBackState is PeekBackRequest plus the offset the target page was left
// at: a Back that lands on the same URL must also land on the same scroll.
func (h *History) PeekBackState() (url string, req Request, scroll int32, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekBackLocked()
	return e.url, e.request, e.scroll, h.index, ok
}

// PeekForwardState is the forward PeekBackState.
func (h *History) PeekForwardState() (url string, req Request, scroll int32, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekForwardLocked()
	return e.url, e.request, e.scroll, h.index, ok
}

// CurrentRequest reports the entry under the cursor and how it was produced.
func (h *History) CurrentRequest() (url string, req Request, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index < 0 || h.index >= len(h.entries) {
		return "", Request{}, false
	}
	e := h.entries[h.index]
	return e.url, e.request, true
}

// ClearCurrentRequest demotes the entry under the cursor to a GET. A POST that
// redirected lands a response the next replay must fetch without the body.
func (h *History) ClearCurrentRequest() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index >= 0 && h.index < len(h.entries) {
		h.entries[h.index].request = Request{}
	}
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

// Entries returns a copy of the URL trail and the current index, for saving.
func (h *History) Entries() ([]string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.entries))
	for i, e := range h.entries {
		out[i] = e.url
	}
	return out, h.index
}

// RestoreHistory rebuilds a history from saved entries, clamping an
// out-of-range index instead of trusting the state file. Session files store
// only URLs, so every restored entry starts as a GET.
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
