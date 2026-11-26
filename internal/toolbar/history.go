package toolbar

import (
	"strings"
	"sync"
)

const MaxHistoryStateBytes = 256 << 10

type Request struct {
	Method      string
	ContentType string
	Body        string
}

func (r Request) IsPost() bool {
	return strings.EqualFold(strings.TrimSpace(r.Method), "POST")
}

type entry struct {
	url          string
	request      Request
	scroll       int32
	state        []byte
	sameDocument bool
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

func cloneHistoryState(state []byte) []byte {
	if len(state) == 0 {
		return nil
	}
	if len(state) > MaxHistoryStateBytes {
		return nil
	}
	return append([]byte(nil), state...)
}

func (h *History) Push(url string) {
	h.PushRequest(url, Request{})
}

func (h *History) PushRequest(url string, req Request) {
	h.PushState(url, req, nil, 0, false)
}

func (h *History) PushState(url string, req Request, state []byte, scroll int32, sameDocument bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index >= 0 && h.index < len(h.entries)-1 {
		h.entries = h.entries[:h.index+1]
	}
	h.entries = append(h.entries, entry{
		url:          url,
		request:      req,
		scroll:       scroll,
		state:        cloneHistoryState(state),
		sameDocument: sameDocument,
	})
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
	url, req, scroll, _, _, base, ok = h.PeekBackEntry()
	return url, req, scroll, base, ok
}

func (h *History) PeekForwardState() (url string, req Request, scroll int32, base int, ok bool) {
	url, req, scroll, _, _, base, ok = h.PeekForwardEntry()
	return url, req, scroll, base, ok
}

func (h *History) PeekEntry(delta int) (url string, req Request, scroll int32, state []byte, sameDocument bool, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	base = h.index
	target := h.index + delta
	if target < 0 || target >= len(h.entries) {
		return "", Request{}, 0, nil, false, base, false
	}
	e := h.entries[target]
	return e.url, e.request, e.scroll, cloneHistoryState(e.state), e.sameDocument, base, true
}

func (h *History) PeekBackEntry() (url string, req Request, scroll int32, state []byte, sameDocument bool, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekBackLocked()
	return e.url, e.request, e.scroll, cloneHistoryState(e.state), e.sameDocument, h.index, ok
}

func (h *History) PeekForwardEntry() (url string, req Request, scroll int32, state []byte, sameDocument bool, base int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.peekForwardLocked()
	return e.url, e.request, e.scroll, cloneHistoryState(e.state), e.sameDocument, h.index, ok
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

func (h *History) CurrentState() ([]byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index < 0 || h.index >= len(h.entries) {
		return nil, false
	}
	return cloneHistoryState(h.entries[h.index].state), true
}

func (h *History) ClearCurrentRequest() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index >= 0 && h.index < len(h.entries) {
		h.entries[h.index].request = Request{}
	}
}

func (h *History) SameDocumentTraversal(delta int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if delta == 0 || h.index < 0 || h.index >= len(h.entries) {
		return false
	}
	target := h.index + delta
	if target < 0 || target >= len(h.entries) {
		return false
	}
	if delta > 0 {
		for i := h.index + 1; i <= target; i++ {
			if !h.entries[i].sameDocument {
				return false
			}
		}
		return true
	}
	for i := target + 1; i <= h.index; i++ {
		if !h.entries[i].sameDocument {
			return false
		}
	}
	return true
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

func (h *History) Go(delta, base int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index != base {
		return false
	}
	target := h.index + delta
	if target < 0 || target >= len(h.entries) {
		return false
	}
	h.index = target
	return true
}

func (h *History) ReplaceCurrent(url string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index < 0 || h.index >= len(h.entries) || url == "" {
		return
	}
	h.entries[h.index].url = url
}

func (h *History) ReplaceCurrentRequest(url string, req Request) {
	h.ReplaceCurrentState(url, req, nil, false)
}

func (h *History) ReplaceCurrentState(url string, req Request, state []byte, sameDocument bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index < 0 || h.index >= len(h.entries) || url == "" {
		return
	}
	h.entries = h.entries[:h.index+1]
	h.entries[h.index].url = url
	h.entries[h.index].request = req
	h.entries[h.index].state = cloneHistoryState(state)
	h.entries[h.index].sameDocument = sameDocument
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

func (h *History) Clone() *History {
	h.mu.Lock()
	defer h.mu.Unlock()
	clone := &History{entries: make([]entry, len(h.entries)), index: h.index}
	for i, e := range h.entries {
		clone.entries[i] = entry{
			url:          e.url,
			request:      e.request,
			scroll:       e.scroll,
			state:        cloneHistoryState(e.state),
			sameDocument: e.sameDocument,
		}
	}
	return clone
}

func (h *History) Entries() ([]string, int) {
	urls, _, _, index := h.Snapshot()
	return urls, index
}

func (h *History) Snapshot() (urls []string, states [][]byte, sameDocument []bool, index int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	urls = make([]string, len(h.entries))
	states = make([][]byte, len(h.entries))
	sameDocument = make([]bool, len(h.entries))
	for i, e := range h.entries {
		urls[i] = e.url
		states[i] = cloneHistoryState(e.state)
		sameDocument[i] = e.sameDocument
	}
	return urls, states, sameDocument, h.index
}

func RestoreHistory(entries []string, index int) *History {
	return RestoreHistoryWithStates(entries, nil, nil, index)
}

func RestoreHistoryWithStates(entries []string, states [][]byte, sameDocument []bool, index int) *History {
	if drop := len(entries) - MaxHistoryEntries; drop > 0 {
		entries = entries[drop:]
		if index >= drop {
			index -= drop
		}
		if len(states) >= drop {
			states = states[drop:]
		}
		if len(sameDocument) >= drop {
			sameDocument = sameDocument[drop:]
		}
	}
	h := &History{entries: make([]entry, len(entries)), index: index}
	for i, raw := range entries {
		h.entries[i].url = raw
		if i < len(states) {
			h.entries[i].state = cloneHistoryState(states[i])
		}
		if i < len(sameDocument) {
			h.entries[i].sameDocument = sameDocument[i]
		}
	}
	if h.index < -1 {
		h.index = -1
	}
	if h.index > len(h.entries)-1 {
		h.index = len(h.entries) - 1
	}
	return h
}
