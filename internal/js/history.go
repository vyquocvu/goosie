package js

import (
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type historyEntry struct {
	state goja.Value
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
	hist := &historyObj{
		r:       r,
		entries: make([]historyEntry, 0),
		index:   -1,
		doc:     r.domDoc,
	}

	if r.opts.URL != "" {
		hist.entries = append(hist.entries, historyEntry{
			state: goja.Null(),
			title: "",
			url:   r.opts.URL,
		})
		hist.index = 0
	}

	obj := r.vm.NewObject()

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
		if hist.index < len(hist.entries)-1 {
			hist.entries = hist.entries[:hist.index+1]
		}
		hist.entries = append(hist.entries, historyEntry{
			state: state,
			title: title,
			url:   url,
		})
		hist.index = len(hist.entries) - 1
		_ = hist.obj.Set("state", state)
		_ = hist.obj.Set("length", len(hist.entries))
		hist.mu.Unlock()

		if url != "" {
			r.updateLocation(url)
		}

		return goja.Undefined()
	})

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
		_ = hist.obj.Set("state", state)
		hist.mu.Unlock()

		if url != "" {
			r.updateLocation(url)
		}

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
		var delta int
		if len(call.Arguments) >= 1 {
			delta = int(call.Arguments[0].ToInteger())
		} else {
			delta = 0
		}
		hist.goTo(delta)
		return goja.Undefined()
	})

	_ = obj.Set("length", len(hist.entries))

	_ = obj.Set("state", goja.Null())

	_ = r.vm.Set("history", obj)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("history", obj)
		}
	}

	hist.obj = obj
}

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
	_ = h.obj.Set("state", entry.state)
	h.mu.Unlock()

	if entry.url != "" {
		h.r.updateLocation(entry.url)
	}

	if newIndex != oldIndex && h.doc != nil {
		ev := dom.NewTrustedEvent("popstate", false, false)
		dom.DispatchEvent(&h.doc.Node, ev)
	}
}

func (r *Runtime) updateLocation(url string) {
	r.setLocation(url)
}
