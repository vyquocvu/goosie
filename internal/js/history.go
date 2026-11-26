package js

import (
	"net/url"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

const MaxHistoryStateBytes = 256 << 10

type historyEntry struct {
	state []byte
	title string
	url   string
}

type historyObj struct {
	r       *Runtime
	mu      sync.Mutex
	entries []historyEntry
	index   int
	doc     *dom.Document
	obj     *goja.Object
}

func (r *Runtime) setupHistory() {
	urls := r.opts.HistoryURLs
	states := r.opts.HistoryStates
	if len(urls) == 0 && r.opts.URL != "" {
		urls = []string{r.opts.URL}
		states = [][]byte{nil}
	}
	hist := &historyObj{
		r:       r,
		entries: make([]historyEntry, 0, len(urls)),
		index:   -1,
		doc:     r.domDoc,
	}
	for i, raw := range urls {
		var state []byte
		if i < len(states) {
			state = append([]byte(nil), states[i]...)
		}
		hist.entries = append(hist.entries, historyEntry{state: state, url: raw})
	}
	if len(hist.entries) > 0 {
		index := r.opts.HistoryIndex
		if index < 0 {
			index = 0
		}
		if index >= len(hist.entries) {
			index = len(hist.entries) - 1
		}
		hist.index = index
	}
	obj := r.vm.NewObject()
	hist.obj = obj
	r.history = hist

	_ = obj.Set("pushState", func(call goja.FunctionCall) goja.Value {
		hist.changeState(call, false)
		return goja.Undefined()
	})
	_ = obj.Set("replaceState", func(call goja.FunctionCall) goja.Value {
		hist.changeState(call, true)
		return goja.Undefined()
	})
	_ = obj.Set("back", func(call goja.FunctionCall) goja.Value {
		hist.goTo(-1)
		return goja.Undefined()
	})
	_ = obj.Set("forward", func(call goja.FunctionCall) goja.Value {
		hist.goTo(1)
		return goja.Undefined()
	})
	_ = obj.Set("go", func(call goja.FunctionCall) goja.Value {
		delta := 0
		if len(call.Arguments) > 0 {
			delta = int(call.Arguments[0].ToInteger())
		}
		hist.goTo(delta)
		return goja.Undefined()
	})

	hist.refreshProperties()
	_ = r.vm.Set("history", obj)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("history", obj)
		}
	}
}

func (h *historyObj) refreshProperties() {
	_ = h.obj.Set("length", len(h.entries))
	_ = h.obj.Set("state", h.stateValueLocked())
}

func (h *historyObj) stateValueLocked() goja.Value {
	if h.index < 0 || h.index >= len(h.entries) {
		return goja.Null()
	}
	return historyStateValue(h.r, h.entries[h.index].state)
}

func (h *historyObj) changeState(call goja.FunctionCall, replace bool) {
	var state goja.Value = goja.Null()
	if len(call.Arguments) > 0 {
		state = call.Arguments[0]
	}
	title := ""
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) {
		title = call.Arguments[1].String()
	}
	reference := ""
	if len(call.Arguments) > 2 && !goja.IsUndefined(call.Arguments[2]) {
		reference = call.Arguments[2].String()
	}
	target, ok, crossOrigin := h.r.historyTargetURL(reference)
	if !ok {
		if crossOrigin {
			panic(h.r.navigationError("SecurityError", "Failed to execute '"+historyMethod(replace)+"' on 'History': The provided value is not a valid URL or is cross-origin."))
		}
		panic(h.r.navigationError("SyntaxError", "Failed to execute '"+historyMethod(replace)+"' on 'History': The provided value is not a valid URL."))
	}
	data, err := snapshotHistoryState(h.r, state)
	if err != nil {
		panic(h.r.dataCloneError("state could not be cloned"))
	}

	h.mu.Lock()
	if replace {
		if h.index >= 0 && h.index < len(h.entries) {
			h.entries[h.index] = historyEntry{state: data, title: title, url: target}
		}
	} else {
		if h.index < len(h.entries)-1 {
			h.entries = h.entries[:h.index+1]
		}
		h.entries = append(h.entries, historyEntry{state: data, title: title, url: target})
		h.index = len(h.entries) - 1
	}
	stateValue := historyStateValue(h.r, data)
	_ = h.obj.Set("state", stateValue)
	_ = h.obj.Set("length", len(h.entries))
	h.mu.Unlock()

	h.r.setLocation(target)
	kind := NavigationHistoryPush
	if replace {
		kind = NavigationHistoryReplace
	}
	h.r.enqueueNavigation(NavigationRequest{Kind: kind, URL: target, State: append([]byte(nil), data...)})
}

func historyMethod(replace bool) string {
	if replace {
		return "replaceState"
	}
	return "pushState"
}

func (r *Runtime) historyTargetURL(reference string) (string, bool, bool) {
	current, base := r.locationState()
	if current == "" && r.opts.URL != "" {
		current = r.opts.URL
		if base == nil {
			if parsed, err := url.Parse(current); err == nil {
				base = parsed
			}
		}
	}
	if strings.TrimSpace(reference) == "" {
		if current == "" {
			return "", false, false
		}
		return current, true, false
	}
	target, ok := resolveLocationURL(base, reference)
	if !ok {
		return "", false, false
	}
	if current == "" {
		return target, isSameOrigin(target, target), false
	}
	if !isSameOrigin(target, current) {
		return "", false, true
	}
	return target, true, false
}

func snapshotHistoryState(r *Runtime, value goja.Value) ([]byte, error) {
	if value == nil || goja.IsNull(value) {
		return nil, nil
	}
	data, err := serializeBroadcastMessage(r, value)
	if err != nil || len(data) > MaxHistoryStateBytes {
		return nil, errBroadcastDataClone
	}
	return data, nil
}

func historyStateValue(r *Runtime, data []byte) goja.Value {
	if len(data) == 0 {
		return goja.Null()
	}
	value, err := deserializeBroadcastMessage(r, data)
	if err != nil {
		return goja.Null()
	}
	return value
}

func (h *historyObj) goTo(delta int) {
	if h.r.navigationSinkAvailable() {
		if delta == 0 {
			h.r.enqueueNavigation(NavigationRequest{Kind: NavigationReload})
		} else {
			h.r.enqueueNavigation(NavigationRequest{Kind: NavigationHistoryTraverse, Delta: delta})
		}
		return
	}
	if delta == 0 {
		return
	}
	h.mu.Lock()
	target := h.index + delta
	if target < 0 || target >= len(h.entries) {
		h.mu.Unlock()
		return
	}
	h.index = target
	entry := h.entries[target]
	stateValue := historyStateValue(h.r, entry.state)
	_ = h.obj.Set("state", stateValue)
	h.mu.Unlock()
	if entry.url != "" {
		h.r.setLocation(entry.url)
	}
	h.r.firePopState(stateValue)
}

func (r *Runtime) firePopState(state goja.Value) {
	event := dom.NewTrustedPopStateEvent("popstate", state)
	r.fireWindowListeners(&event.Event, true)
	if !event.Event.PropagationStopped() {
		r.fireWindowListeners(&event.Event, false)
	}
}

func (r *Runtime) SetHistorySnapshot(urls []string, states [][]byte, index int) {
	if r == nil || r.history == nil {
		return
	}
	h := r.history
	entries := make([]historyEntry, len(urls))
	for i, raw := range urls {
		if i < len(states) {
			entries[i].state = append([]byte(nil), states[i]...)
		}
		entries[i].url = raw
	}
	if index < 0 {
		index = -1
	}
	if len(entries) == 0 {
		index = -1
	} else if index >= len(entries) {
		index = len(entries) - 1
	}
	h.mu.Lock()
	h.entries = entries
	h.index = index
	h.refreshProperties()
	currentURL := ""
	if index >= 0 && index < len(entries) {
		currentURL = entries[index].url
	}
	h.mu.Unlock()
	if currentURL != "" {
		r.setLocation(currentURL)
	}
}

func (r *Runtime) ApplyHistoryEntry(target string, state []byte, index, length, delta int) {
	if r == nil || r.history == nil {
		return
	}
	h := r.history
	oldURL, _ := r.locationState()
	h.mu.Lock()
	if index < 0 {
		index = 0
	}
	if index >= len(h.entries) {
		index = len(h.entries) - 1
	}
	if index >= 0 {
		h.entries[index].url = target
		h.entries[index].state = append([]byte(nil), state...)
	}
	h.index = index
	stateValue := h.stateValueLocked()
	_ = h.obj.Set("state", stateValue)
	_ = h.obj.Set("length", length)
	h.mu.Unlock()
	if target != "" {
		r.setLocation(target)
	}
	if delta != 0 {
		h.r.firePopState(stateValue)
	}
	if delta != 0 && fragmentChanged(oldURL, target) {
		if parsed, err := url.Parse(target); err == nil {
			r.revealFragmentTarget(parsed.EscapedFragment())
		}
		event := dom.NewHashChangeEvent("hashchange", false, false, oldURL, target)
		r.fireWindowListeners(&event.Event, true)
		if !event.Event.PropagationStopped() {
			r.fireWindowListeners(&event.Event, false)
		}
	}
}

func fragmentChanged(oldURL, newURL string) bool {
	a, errA := url.Parse(oldURL)
	b, errB := url.Parse(newURL)
	if errA != nil || errB != nil {
		return false
	}
	return a.EscapedFragment() != b.EscapedFragment()
}
