package js

import (
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// History API implementation for client-side routing.

// historyEntry represents one entry in the session history.
type historyEntry struct {
	state goja.Value
	title string
	url   string
}

// historyObj holds the internal state of the History object.
type historyObj struct {
	r       *Runtime
	mu      sync.Mutex
	entries []historyEntry
	index   int
	doc     *dom.Document
	obj     *goja.Object
}

// setupHistory installs the History API on the global scope.
func (r *Runtime) setupHistory() {
	hist := &historyObj{
		r:       r,
		entries: make([]historyEntry, 0),
		index:   -1,
		doc:     r.domDoc,
	}

	// Add initial entry for the current page.
	if r.opts.URL != "" {
		hist.entries = append(hist.entries, historyEntry{
			state: goja.Null(),
			title: "",
			url:   r.opts.URL,
		})
		hist.index = 0
	}

	obj := r.vm.NewObject()

	// pushState(state, title, url) - adds a new entry to the history.
	_ = obj.Set("pushState", func(call goja.FunctionCall) goja.Value {
		var state goja.Value
		if len(call.Arguments) >= 1 {
			state = call.Arguments[0]
		} else {
			state = goja.Null()
		}

		var title string
		if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Arguments[1]) {
			title = call.Arguments[1].String()
		}

		var url string
		if len(call.Arguments) >= 3 && !goja.IsUndefined(call.Arguments[2]) {
			url = call.Arguments[2].String()
		}

		hist.mu.Lock()
		// Remove any forward history.
		if hist.index < len(hist.entries)-1 {
			hist.entries = hist.entries[:hist.index+1]
		}
		// Add new entry.
		hist.entries = append(hist.entries, historyEntry{
			state: state,
			title: title,
			url:   url,
		})
		hist.index = len(hist.entries) - 1
		// Update state property on the JS object.
		_ = hist.obj.Set("state", state)
		_ = hist.obj.Set("length", len(hist.entries))
		hist.mu.Unlock()

		// Update window.location.href if url was provided.
		if url != "" {
			r.updateLocation(url)
		}

		return goja.Undefined()
	})

	// replaceState(state, title, url) - replaces the current entry.
	_ = obj.Set("replaceState", func(call goja.FunctionCall) goja.Value {
		var state goja.Value
		if len(call.Arguments) >= 1 {
			state = call.Arguments[0]
		} else {
			state = goja.Null()
		}

		var title string
		if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Arguments[1]) {
			title = call.Arguments[1].String()
		}

		var url string
		if len(call.Arguments) >= 3 && !goja.IsUndefined(call.Arguments[2]) {
			url = call.Arguments[2].String()
		}

		hist.mu.Lock()
		if hist.index >= 0 && hist.index < len(hist.entries) {
			hist.entries[hist.index] = historyEntry{
				state: state,
				title: title,
				url:   url,
			}
		}
		// Update state property on the JS object.
		_ = hist.obj.Set("state", state)
		hist.mu.Unlock()

		// Update window.location.href if url was provided.
		if url != "" {
			r.updateLocation(url)
		}

		return goja.Undefined()
	})

	// back() - goes back one entry.
	_ = obj.Set("back", func(call goja.FunctionCall) goja.Value {
		hist.goTo(-1)
		return goja.Undefined()
	})

	// forward() - goes forward one entry.
	_ = obj.Set("forward", func(call goja.FunctionCall) goja.Value {
		hist.goTo(1)
		return goja.Undefined()
	})

	// go(delta) - goes to a specific entry.
	_ = obj.Set("go", func(call goja.FunctionCall) goja.Value {
		var delta int
		if len(call.Arguments) >= 1 {
			delta = int(call.Arguments[0].ToInteger())
		} else {
			delta = 0
		}
		hist.goTo(delta)
		return goja.Undefined()
	})

	// length property - updated by pushState.
	_ = obj.Set("length", len(hist.entries))

	// state property - initially null, updated by pushState/replaceState/goTo.
	_ = obj.Set("state", goja.Null())

	// Install on global scope.
	_ = r.vm.Set("history", obj)

	// Also install on window object.
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("history", obj)
		}
	}

	// Store reference to the history object so we can update state.
	hist.obj = obj
}

// goTo navigates by the given delta in the history.
func (h *historyObj) goTo(delta int) {
	h.mu.Lock()
	newIndex := h.index + delta
	if newIndex < 0 || newIndex >= len(h.entries) {
		h.mu.Unlock()
		return
	}

	oldIndex := h.index
	h.index = newIndex
	entry := h.entries[newIndex]
	// Update state property on the JS object.
	_ = h.obj.Set("state", entry.state)
	h.mu.Unlock()

	// Update window.location.href.
	if entry.url != "" {
		h.r.updateLocation(entry.url)
	}

	// Fire popstate event if we actually moved.
	if newIndex != oldIndex && h.doc != nil {
		ev := dom.NewEvent("popstate", false, false)
		dom.DispatchEvent(&h.doc.Node, ev)
	}
}

// updateLocation updates window.location.href.
func (r *Runtime) updateLocation(url string) {
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			if loc := winObj.Get("location"); loc != nil {
				if locObj, ok := loc.(*goja.Object); ok {
					_ = locObj.Set("href", url)
				}
			}
		}
	}
}
