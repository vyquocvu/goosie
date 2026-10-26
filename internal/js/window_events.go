package js

import (
	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// The window object as an EventTarget, plus the document lifecycle states.
//
// Why this exists is a count, not an opinion. On the saved vnexpress home page
// (201,234-byte copy, sha1 1cd00f06…, measured offline against the pre-change
// engine; the artifact is named in docs/tasks/TASK-047-DOM-JQUERY-APIS.md §1.2)
// the inline boot scripts register:
//
//	window.addEventListener('DOMContentLoaded', ...)   x4
//	window.addEventListener('load', ...)               x1
//	window.addEventListener('pageshow', ...)           x1
//
// and every one of them threw `Object has no member 'addEventListener'`, which
// is three of the six script errors that page produces. jQuery 3.6 adds its own
// `C.addEventListener("load", B)` next to `E.addEventListener("DOMContentLoaded",
// B)` (C is window, E is document), so a window without listeners cannot reach
// `$(document.ready)` at all - and gov.vn keeps its entire body inside a
// `display:none` form that only that reveal opens.
//
// window is not a *dom.Node, so it cannot live in internal/dom's listener
// registry: that map is keyed by node pointer, and pretending otherwise would
// put a fake node into a tree the parser, style and layout walks all trust.
// The registry therefore belongs to the runtime, and the DOM Standard's window
// step of the propagation path is appended by dispatchWithWindowStep - after
// the document, in bubble order, and not at all once a listener stopped
// propagation.

// Ready-state values, spelled exactly as the standard spells them: pages
// compare these strings, so a typo is a silent never-ready page.
const (
	ReadyLoading     = "loading"
	ReadyInteractive = "interactive"
	ReadyComplete    = "complete"
)

// windowListener is one entry of the window object's listener list. fnVal keeps
// the JS function value the page passed in, which is what makes a second
// registration of the same function a no-op and what lets
// removeEventListener find the entry - neither is possible from a Go closure,
// because every wrapper closure has a distinct identity from the page's point
// of view and an identical code pointer from Go's.
type windowListener struct {
	typ     string
	fn      goja.Callable
	fnVal   goja.Value
	capture bool
}

// setupWindowEvents installs the EventTarget API on window and seeds the
// lifecycle state. Called from setupWindow, before any script runs.
func (r *Runtime) setupWindowEvents() {
	win := r.vm.GlobalObject()
	_ = win.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		r.jsWindowAddEventListener(call)
		return goja.Undefined()
	})
	_ = win.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		r.jsWindowRemoveEventListener(call)
		return goja.Undefined()
	})
	_ = win.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		return r.jsWindowDispatchEvent(call)
	})

	// document.readyState is a read-only accessor: `document.readyState = "x"`
	// is ignored rather than throwing, which is what a non-strict page expects
	// and what keeps a `try { } catch { }`-less boot script alive.
	_ = r.docObj().DefineAccessorProperty("readyState", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.readyState())
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.setReadyState(ReadyLoading)
}

// docObj returns the global document object. setupDocument installs it as a
// global before any DOM binding runs, so the lookup cannot fail in a runtime
// this package built.
func (r *Runtime) docObj() *goja.Object {
	if v := r.vm.Get("document"); v != nil {
		if o, ok := v.(*goja.Object); ok {
			return o
		}
	}
	return r.vm.NewObject()
}

func (r *Runtime) readyState() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ready == "" {
		return ReadyLoading
	}
	return r.ready
}

func (r *Runtime) setReadyState(state string) {
	r.mu.Lock()
	r.ready = state
	r.mu.Unlock()
}

func (r *Runtime) jsWindowAddEventListener(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	typ := call.Arguments[0].String()
	fn, ok := goja.AssertFunction(call.Arguments[1])
	if !ok {
		return goja.Undefined()
	}
	capture := len(call.Arguments) > 2 && call.Arguments[2].ToBoolean()
	r.winMu.Lock()
	defer r.winMu.Unlock()
	for _, l := range r.winListeners {
		if l.typ == typ && l.capture == capture && l.fnVal == call.Arguments[1] {
			return goja.Undefined()
		}
	}
	r.winListeners = append(r.winListeners, &windowListener{
		typ: typ, fn: fn, fnVal: call.Arguments[1], capture: capture,
	})
	return goja.Undefined()
}

func (r *Runtime) jsWindowRemoveEventListener(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	typ := call.Arguments[0].String()
	capture := len(call.Arguments) > 2 && call.Arguments[2].ToBoolean()
	r.winMu.Lock()
	defer r.winMu.Unlock()
	for i, l := range r.winListeners {
		if l.typ == typ && l.capture == capture && l.fnVal == call.Arguments[1] {
			r.winListeners = append(r.winListeners[:i], r.winListeners[i+1:]...)
			break
		}
	}
	return goja.Undefined()
}

// fireWindowListeners runs the window step of one event. capture picks the
// group, in registration order, exactly as dom.fireListeners does.
func (r *Runtime) fireWindowListeners(ev *dom.Event, capture bool) {
	r.winMu.Lock()
	// Copy the slice: a listener may add or remove a listener while the pass
	// is running, and the standard dispatches against a snapshot.
	listeners := make([]*windowListener, len(r.winListeners))
	copy(listeners, r.winListeners)
	r.winMu.Unlock()
	for _, l := range listeners {
		if l.typ != ev.Type || l.capture != capture {
			continue
		}
		wrapped := r.wrapEvent(ev)
		_, _ = l.fn(goja.Null(), wrapped)
	}
}

// dispatchWithWindowStep dispatches an event whose target is a node and whose
// propagation path, per the DOM Standard, ends at window one step after the
// document. dom.DispatchEvent stops at the document because that is the top of
// the node tree, so the window step is appended here - before the node tree for
// the capture phase, after it for the bubble phase, and skipped entirely when a
// listener called stopPropagation on the way up.
func (r *Runtime) dispatchWithWindowStep(target *dom.Node, ev *dom.Event) bool {
	if target == nil {
		return true
	}
	if target.Type == dom.NodeDocument {
		r.fireWindowListeners(ev, true)
		ok := dom.DispatchEvent(target, ev)
		if !ev.PropagationStopped() {
			r.fireWindowListeners(ev, false)
		}
		return ok
	}
	// An element target reaches window only by bubbling.
	ok := dom.DispatchEvent(target, ev)
	if ev.Bubbles() && !ev.PropagationStopped() {
		r.fireWindowListeners(ev, false)
	}
	return ok
}

func (r *Runtime) jsWindowDispatchEvent(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return r.vm.ToValue(false)
	}
	eventObj := call.Arguments[0]
	typ := ""
	if t := eventObj.ToObject(r.vm).Get("type"); t != nil && !goja.IsUndefined(t) {
		typ = t.String()
	}
	ev := dom.NewEvent(typ, true, true)
	// window is the target, so the path is just [window]: the document is not
	// an ancestor of window in the standard's event path, and a browser does
	// not deliver `window.dispatchEvent(new Event("x"))` to document listeners.
	r.fireWindowListeners(ev, true)
	r.fireWindowListeners(ev, false)
	return r.vm.ToValue(!ev.DefaultPrevented())
}

// FireDocumentLifecycle runs the two events the HTML standard fires once a
// document is fully parsed: DOMContentLoaded on the document (bubbles, so the
// path ends at window) followed by load on window itself, with the readyState
// transitions pages read in between - "interactive" before DOMContentLoaded and
// "complete" before load. jQuery's boot reads exactly that:
//
//	"complete" === E.readyState || "loading" !== E.readyState && !E.documentElement.doScroll
//	  ? C.setTimeout(S.ready)
//	  : (E.addEventListener("DOMContentLoaded", B), C.addEventListener("load", B))
//
// so readyState must still read "loading" while a document's own scripts run,
// or the library resolves ready inline, before the listeners it is about to
// register exist. The engine calls this after the pipeline has run, so a
// ready handler that reads a computed style or reflows the page sees the load
// cascade rather than an empty map; that is a deliberate deviation from the
// browser's incremental-parse timing, recorded in
// docs/tasks/TASK-047-DOM-JQUERY-APIS.md, and it is the timing that makes
// `$(document).ready(fn)` work at all in a parse-then-run engine.
//
// It reports whether user code mutated the DOM, so the caller can decide
// whether another style/layout pass is owed. It is idempotent: a document
// lifecycle happens once.
func (r *Runtime) FireDocumentLifecycle() bool {
	if r.domDoc == nil {
		return false
	}
	r.mu.Lock()
	if r.lifecycleFired {
		r.mu.Unlock()
		return false
	}
	r.lifecycleFired = true
	r.mu.Unlock()

	before := r.mutationStamp()
	_, _ = r.callBounded(func() (goja.Value, error) {
		r.setReadyState(ReadyInteractive)
		doc := &r.domDoc.Node
		r.dispatchWithWindowStep(doc, dom.NewEvent("DOMContentLoaded", true, false))
		r.setReadyState(ReadyComplete)
		ev := dom.NewEvent("load", false, false)
		r.fireWindowListeners(ev, true)
		r.fireWindowListeners(ev, false)
		return goja.Undefined(), nil
	})
	return r.mutationStamp() != before
}

// mutationStamp counts the DOM writes user code has made. FireDocumentLifecycle
// compares two stamps instead of consulting the invalidation batch, because a
// session that was built without invalidation tracking still has to learn that
// a ready handler changed the tree.
func (r *Runtime) mutationStamp() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mutations
}

// winMu guards the window listener list. Nothing in the current call graph
// reaches it from two goroutines - scripts, timers and the lifecycle pass all
// run on the document's own thread - but the list is read from the accessor
// path and written from page code, so the lock states the rule instead of
// relying on the schedule. The list itself is per-Runtime: no process-global
// listener map exists, which is the archtest rule that keeps two tabs from
// reading each other's handlers.
