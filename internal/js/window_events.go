package js

import (
	"reflect"

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
	typ        string
	fn         goja.Callable
	fnVal      goja.Value
	capture    bool
	once       bool
	passive    bool
	signal     *goja.Object
	handler    bool // callback came from an on* IDL attribute
	content    string
	hasContent bool
	compiled   bool
}

// setupWindowEvents installs the EventTarget API on window and seeds the
// lifecycle state. Called from setupWindow, before any script runs.
func (r *Runtime) setupWindowEvents() {
	win := r.vm.GlobalObject()
	_ = win.DefineAccessorProperty("crossOriginIsolated", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.crossOriginIsolated)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	for _, typ := range globalEventHandlerTypes {
		installWindowEventHandlerAttribute(r, win, typ)
	}
	for _, typ := range windowEventHandlerTypes {
		installWindowEventHandlerAttribute(r, win, typ)
	}
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
	r.setupWindowPostMessage(win)

	// document.readyState is a read-only accessor: `document.readyState = "x"`
	// is ignored rather than throwing, which is what a non-strict page expects
	// and what keeps a `try { } catch { }`-less boot script alive.
	_ = r.docObj().DefineAccessorProperty("readyState", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.readyState())
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.setReadyState(ReadyLoading)
}

func installWindowEventHandlerAttribute(r *Runtime, win *goja.Object, typ string) {
	eventType := typ
	_ = win.DefineAccessorProperty("on"+eventType, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		r.winMu.Lock()
		for _, l := range r.winListeners {
			if l.typ == eventType && l.handler && (l.fnVal != nil || l.hasContent) {
				if l.hasContent && !l.compiled {
					r.compileWindowContentEventHandler(l)
				}
				value := l.fnVal
				r.winMu.Unlock()
				if value != nil {
					return value
				}
				return goja.Null()
			}
		}
		r.winMu.Unlock()
		return goja.Null()
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		fn, callable := goja.AssertFunction(value)
		r.winMu.Lock()
		for i, l := range r.winListeners {
			if l.typ != eventType || !l.handler || (l.fnVal == nil && !l.hasContent) {
				continue
			}
			if goja.IsNull(value) || goja.IsUndefined(value) || !callable {
				r.winListeners = append(r.winListeners[:i], r.winListeners[i+1:]...)
			} else {
				l.fn = fn
				l.fnVal = value
				l.content, l.hasContent, l.compiled = "", false, true
			}
			r.winMu.Unlock()
			return goja.Undefined()
		}
		if callable && !goja.IsNull(value) && !goja.IsUndefined(value) {
			r.winListeners = append(r.winListeners, &windowListener{
				typ: eventType, fn: fn, fnVal: value, handler: true,
			})
		}
		r.winMu.Unlock()
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// syncWindowContentEventHandler maps body/frameset content attributes into
// Window's event-handler slots while retaining the listener's original order.
func (r *Runtime) syncWindowContentEventHandler(node *dom.Node, typ, source string, present bool) {
	if node == nil || (node.Data != "body" && node.Data != "frameset") {
		return
	}
	r.winMu.Lock()
	defer r.winMu.Unlock()
	for i, listener := range r.winListeners {
		if listener.typ != typ || !listener.handler || (listener.fnVal == nil && !listener.hasContent) {
			continue
		}
		if !present {
			r.winListeners = append(r.winListeners[:i], r.winListeners[i+1:]...)
		} else {
			listener.fn, listener.fnVal = nil, nil
			listener.content, listener.hasContent, listener.compiled = source, true, false
		}
		return
	}
	if present {
		r.winListeners = append(r.winListeners, &windowListener{
			typ: typ, handler: true, content: source, hasContent: true,
		})
	}
}

func (r *Runtime) compileWindowContentEventHandler(listener *windowListener) {
	if listener == nil || listener.compiled || !listener.hasContent {
		return
	}
	listener.compiled = true
	if r.opts.ScriptingDisabled || (r.opts.CSP != nil && !r.opts.CSP.AllowsInlineScript()) {
		return
	}
	params := "event"
	if listener.typ == "error" {
		params = "event, source, lineno, colno, error"
	}
	source := "(function on" + listener.typ + "(" + params + ") {\n" + listener.content + "\n})"
	program, err := goja.Compile(r.opts.URL+"#event-handler", source, false)
	if err != nil {
		return
	}
	value, err := r.vm.RunProgram(program)
	if err != nil {
		return
	}
	if fn, ok := goja.AssertFunction(value); ok {
		listener.fn, listener.fnVal = fn, value
	}
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
	capture, once, passive, signalValue := windowListenerOptions(r.vm, call.Arguments[2:], defaultPassiveEventType(typ))
	var signal *goja.Object
	if signalValue != nil && !goja.IsUndefined(signalValue) {
		var ok bool
		signal, ok = signalValue.(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("EventListenerOptions.signal must be an AbortSignal"))
		}
		if _, callable := goja.AssertFunction(signal.Get("__registerAbortAlgorithm__")); !callable {
			panic(r.vm.NewTypeError("EventListenerOptions.signal must be an AbortSignal"))
		}
		if signal.Get("aborted").ToBoolean() {
			return goja.Undefined()
		}
	}
	r.winMu.Lock()
	defer r.winMu.Unlock()
	for _, l := range r.winListeners {
		if l.typ == typ && l.capture == capture && l.fnVal == call.Arguments[1] {
			return goja.Undefined()
		}
	}
	listener := &windowListener{
		typ: typ, fn: fn, fnVal: call.Arguments[1], capture: capture, once: once, passive: passive, signal: signal,
	}
	r.winListeners = append(r.winListeners, listener)
	if signal != nil {
		register, _ := goja.AssertFunction(signal.Get("__registerAbortAlgorithm__"))
		cleanup := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			r.removeWindowListener(listener)
			return goja.Undefined()
		})
		_, _ = register(signal, cleanup)
	}
	return goja.Undefined()
}

func windowListenerOptions(vm *goja.Runtime, args []goja.Value, defaultPassive bool) (capture, once, passive bool, signal goja.Value) {
	passive = defaultPassive
	if len(args) == 0 || args[0] == nil || goja.IsNull(args[0]) || goja.IsUndefined(args[0]) {
		return false, false, passive, nil
	}
	if args[0].ExportType().Kind() == reflect.Bool {
		return args[0].ToBoolean(), false, passive, nil
	}
	options := args[0].ToObject(vm)
	if value := options.Get("capture"); value != nil && !goja.IsUndefined(value) {
		capture = value.ToBoolean()
	}
	if value := options.Get("once"); value != nil && !goja.IsUndefined(value) {
		once = value.ToBoolean()
	}
	if value := options.Get("passive"); value != nil && !goja.IsUndefined(value) {
		passive = value.ToBoolean()
	}
	signal = options.Get("signal")
	return capture, once, passive, signal
}

func defaultPassiveEventType(typ string) bool {
	return dom.IsScrollBlockingEventType(typ)
}

func defaultPassiveNodeTarget(node *dom.Node) bool {
	if node == nil {
		return false
	}
	if node.Type == dom.NodeDocument {
		return true
	}
	if node.Type != dom.NodeElement {
		return false
	}
	if node.Data == "html" && node.Parent != nil && node.Parent.Type == dom.NodeDocument {
		return true
	}
	return node.Data == "body" && node.Parent != nil && node.Parent.Data == "html" &&
		node.Parent.Parent != nil && node.Parent.Parent.Type == dom.NodeDocument
}

func (r *Runtime) removeWindowListener(listener *windowListener) {
	if listener == nil {
		return
	}
	r.winMu.Lock()
	defer r.winMu.Unlock()
	for i, current := range r.winListeners {
		if current == listener {
			r.winListeners = append(r.winListeners[:i], r.winListeners[i+1:]...)
			return
		}
	}
}

func (r *Runtime) jsWindowRemoveEventListener(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	typ := call.Arguments[0].String()
	capture := windowListenerCapture(r.vm, call.Arguments[2:])
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

func windowListenerCapture(vm *goja.Runtime, args []goja.Value) bool {
	if len(args) == 0 || args[0] == nil || goja.IsNull(args[0]) || goja.IsUndefined(args[0]) {
		return false
	}
	if args[0].ExportType().Kind() == reflect.Bool {
		return args[0].ToBoolean()
	}
	if value := args[0].ToObject(vm).Get("capture"); value != nil && !goja.IsUndefined(value) {
		return value.ToBoolean()
	}
	return false
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
		r.winMu.Lock()
		if l.once {
			for i, current := range r.winListeners {
				if current == l {
					r.winListeners = append(r.winListeners[:i], r.winListeners[i+1:]...)
					break
				}
			}
		}
		if l.hasContent && !l.compiled {
			r.compileWindowContentEventHandler(l)
		}
		fn := l.fn
		r.winMu.Unlock()
		if fn == nil {
			continue
		}
		ev.SetInPassiveListener(l.passive)
		wrapped := r.wrapWindowEvent(ev, capture)
		var result goja.Value
		var err error
		invoke := func() {
			if l.handler && ev.Type == "error" {
				if errorEvent := ev.AsError(); errorEvent != nil {
					var errorValue goja.Value
					if value, ok := errorEvent.Error.(goja.Value); ok {
						errorValue = value
					} else {
						errorValue = r.vm.ToValue(errorEvent.Error)
					}
					result, err = fn(r.vm.GlobalObject(), r.vm.ToValue(errorEvent.Message), r.vm.ToValue(errorEvent.Filename), r.vm.ToValue(errorEvent.Lineno), r.vm.ToValue(errorEvent.Colno), errorValue)
				} else {
					result, err = fn(r.vm.GlobalObject(), wrapped)
				}
			} else {
				result, err = fn(r.vm.GlobalObject(), wrapped)
			}
			if l.handler && ev.Type == "beforeunload" && err == nil && result != nil && !goja.IsNull(result) && !goja.IsUndefined(result) {
				// HTML coerces an onbeforeunload return value while the legacy
				// current-event value is still installed for this callback.
				_ = result.ToString()
			}
		}
		currentEventIsPreserved := (r.reportingWindowError || r.preserveWindowErrorEvent) && ev.Type == "error" && r.legacyWindowEvent != nil && !goja.IsUndefined(r.legacyWindowEvent)
		if currentEventIsPreserved {
			invoke()
		} else {
			r.withCurrentEvent(wrapped, invoke)
		}
		ev.SetInPassiveListener(false)
		if !l.handler || err != nil || result == nil || goja.IsNull(result) || goja.IsUndefined(result) {
			continue
		}
		boolean, isBoolean := result.Export().(bool)
		switch ev.Type {
		case "error":
			if isBoolean && boolean {
				ev.PreventDefault()
			}
		case "beforeunload":
			ev.PreventDefault()
		default:
			if isBoolean && !boolean {
				ev.PreventDefault()
			}
		}
	}
}

// wrapWindowEvent changes currentTarget for a callback on Window. The DOM
// event's currentTarget slot stores nodes only; Window is a separate event
// target in this implementation, so its wrapper supplies that final path step.
func (r *Runtime) wrapWindowEvent(ev *dom.Event, capture bool) *goja.Object {
	wrapped := r.wrapEvent(ev).(*goja.Object)
	win := r.vm.GlobalObject()
	_ = wrapped.Set("currentTarget", win)
	if capture {
		_ = wrapped.Set("eventPhase", int(dom.CapturingPhase))
	} else {
		_ = wrapped.Set("eventPhase", int(dom.BubblingPhase))
	}
	if ev.Target() == nil {
		_ = wrapped.Set("target", win)
	}
	return wrapped
}

// dispatchWithWindowStep dispatches an event whose target is a node along the
// document path, adding Window's capture step before the node path and its
// bubble step after the document. dom.DispatchEvent stops at the document
// because that is the top of the node tree.
func (r *Runtime) dispatchWithWindowStep(target *dom.Node, ev *dom.Event) bool {
	if target == nil {
		return true
	}
	// Snapshot the node path before Window capture listeners run. Those
	// listeners are part of this event dispatch and may move or remove target;
	// the path for the in-progress event must retain the original ancestors.
	path := dom.EventPath(target)
	// Only this Runtime's associated Document has this Window's event-target
	// step. A connected tree inside a cloned or otherwise detached Document
	// must not leak events to the Runtime's Window.
	includeWindow := false
	if r.domDoc != nil {
		for current := target; current != nil; current = current.Parent {
			if current == &r.domDoc.Node {
				includeWindow = true
				break
			}
		}
	}
	defer ev.ClearPropagationFlags()
	ev.SetTarget(target)
	if r.activeEventObjects == nil {
		r.activeEventObjects = make(map[*dom.Event]goja.Value)
	}
	ownedEventObject := r.activeEventObjects[ev] == nil
	if ownedEventObject {
		r.activeEventObjects[ev] = r.wrapEvent(ev)
	}
	wrapped, _ := r.activeEventObjects[ev].(*goja.Object)
	if wrapped != nil {
		_ = wrapped.Set("__dispatching", true)
		wrappedPath := make([]interface{}, 0, len(path)+1)
		for _, current := range path {
			wrappedPath = append(wrappedPath, r.wrapNode(current))
		}
		if includeWindow {
			wrappedPath = append(wrappedPath, r.vm.GlobalObject())
		}
		_ = wrapped.Set("__composedPath", r.vm.NewArray(wrappedPath...))
	}
	defer func() {
		if wrapped != nil {
			_ = wrapped.Set("__dispatching", false)
			_ = wrapped.Set("__composedPath", r.vm.NewArray())
			_ = wrapped.Set("currentTarget", goja.Null())
			_ = wrapped.Set("eventPhase", 0)
			if ev.AsInput() != nil {
				_ = wrapped.Set("__targetRanges", r.vm.NewArray())
			}
		}
		if ownedEventObject {
			delete(r.activeEventObjects, ev)
		}
	}()
	if includeWindow && !ev.PropagationStopped() {
		r.fireWindowListeners(ev, true)
		if ev.PropagationStopped() {
			return !ev.DefaultPrevented()
		}
	}
	if target.Type == dom.NodeDocument {
		ok := dom.DispatchEventWithPathWhile(target, ev, path, func() bool { return r.isFullyActiveDocument(target.Doc) })
		if !ev.PropagationStopped() {
			r.fireWindowListeners(ev, false)
		}
		return ok
	}
	if shadow := r.shadowRootContainingNode(target); shadow != nil && !ev.PropagationStopped() {
		r.fireShadowRootEvent(shadow, ev)
	}
	// The element path runs between Window's capture and bubble steps.
	ok := dom.DispatchEventWithPathWhile(target, ev, path, func() bool { return r.isFullyActiveDocument(target.Doc) })
	if includeWindow && ev.Bubbles() && !ev.PropagationStopped() {
		r.fireWindowListeners(ev, false)
	}
	return ok
}

func (r *Runtime) jsWindowDispatchEvent(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return r.vm.ToValue(false)
	}
	eventObj := call.Arguments[0]
	object := eventObj.ToObject(r.vm)
	if value := object.Get("__dispatching"); value != nil && value.ToBoolean() {
		panic(r.domError("InvalidStateError", "The event is already being dispatched"))
	}
	if value := object.Get("__initialized"); value != nil && !value.ToBoolean() {
		panic(r.domError("InvalidStateError", "The event has not been initialized"))
	}
	if typ := object.Get("type"); typ == nil || goja.IsUndefined(typ) {
		panic(r.vm.NewTypeError("parameter 1 is not of type 'Event'"))
	}
	if trusted := object.Get("__isTrusted"); trusted != nil && !goja.IsUndefined(trusted) {
		_ = object.Set("__isTrusted", false)
	} else {
		_ = object.Set("isTrusted", false)
	}
	_ = object.Set("__dispatching", true)
	defer func() { _ = object.Set("__dispatching", false) }()
	typ := ""
	if t := object.Get("type"); t != nil && !goja.IsUndefined(t) {
		typ = t.String()
	}
	if typ == "error" && r.legacyWindowEvent != nil && !goja.IsUndefined(r.legacyWindowEvent) {
		previousPreserve := r.preserveWindowErrorEvent
		previousContext := r.iframeEventContext
		r.preserveWindowErrorEvent = true
		r.iframeEventContext = object
		defer func() {
			r.preserveWindowErrorEvent = previousPreserve
			r.iframeEventContext = previousContext
		}()
	}
	readBool := func(name string, fallback bool) bool {
		value := object.Get(name)
		if value == nil || goja.IsUndefined(value) {
			return fallback
		}
		return value.ToBoolean()
	}
	readString := func(name string) string {
		value := object.Get(name)
		if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
			return ""
		}
		return value.String()
	}
	bubbles, cancelable := readBool("bubbles", false), readBool("cancelable", false)
	marker := object.Get("__goosieEventInterface")
	markerName := ""
	if marker != nil && !goja.IsUndefined(marker) && !goja.IsNull(marker) {
		markerName = marker.String()
	}
	var ev *dom.Event
	if markerName == "ErrorEvent" {
		readUint32 := func(name string) uint32 {
			value := object.Get(name)
			if value == nil || goja.IsUndefined(value) || goja.IsNull(value) || value.ToInteger() < 0 {
				return 0
			}
			return uint32(value.ToInteger())
		}
		errorValue := object.Get("error")
		if errorValue == nil {
			errorValue = goja.Null()
		}
		errorEvent := dom.NewErrorEvent(typ, bubbles, cancelable, readString("message"), readString("filename"), readUint32("lineno"), readUint32("colno"), errorValue)
		ev = &errorEvent.Event
	} else if markerName == "MessageEvent" {
		data := object.Get("data")
		if data == nil {
			data = goja.Null()
		}
		source := object.Get("source")
		if source == nil {
			source = goja.Null()
		}
		message := dom.NewMessageEvent(typ, bubbles, cancelable, data, readString("origin"), readString("lastEventId"), source, eventPorts(object.Get("ports"), r.vm))
		ev = &message.Event
	} else {
		ev = dom.NewEvent(typ, bubbles, cancelable)
	}
	if value := object.Get("__propagationStopped"); value != nil && value.ToBoolean() {
		ev.StopPropagation()
	}
	defer ev.ClearPropagationFlags()
	// window is the target, so the path is just [window]: the document is not
	// an ancestor of window in the standard's event path, and a browser does
	// not deliver `window.dispatchEvent(new Event("x"))` to document listeners.
	if r.isFullyActiveBrowsingContext() && !ev.PropagationStopped() {
		r.fireWindowListeners(ev, true)
		if r.isFullyActiveBrowsingContext() && !ev.PropagationStopped() {
			r.fireWindowListeners(ev, false)
		}
	}
	_ = object.Set("defaultPrevented", ev.DefaultPrevented())
	_ = object.Set("__propagationStopped", false)
	_ = object.Set("__immediateStopped", false)
	return r.vm.ToValue(!ev.DefaultPrevented())
}

// FireDocumentDOMContentLoaded runs the post-parse DOMContentLoaded step.
// The engine fires the later load step only after pending child browsing
// contexts have completed loading.
func (r *Runtime) FireDocumentDOMContentLoaded() bool {
	if r.domDoc == nil {
		return false
	}
	r.mu.Lock()
	if r.domContentLoadedFired {
		r.mu.Unlock()
		return false
	}
	r.domContentLoadedFired = true
	r.mu.Unlock()
	_ = r.drainMicrotasks()

	before := r.mutationStamp()
	_, _ = r.callBounded(func() (goja.Value, error) {
		r.setReadyState(ReadyInteractive)
		r.dispatchWithWindowStep(&r.domDoc.Node, dom.NewTrustedEvent("DOMContentLoaded", true, false))
		return goja.Undefined(), nil
	})
	return r.mutationStamp() != before
}

// FireDocumentLoad runs the load and pageshow steps after DOMContentLoaded
// and after the embedding browser has completed the document's load-delaying
// work, including child browsing contexts.
func (r *Runtime) FireDocumentLoad() bool {
	if r.domDoc == nil {
		return false
	}
	mutated := r.FireDocumentDOMContentLoaded()
	r.mu.Lock()
	if r.loadFired {
		r.mu.Unlock()
		return mutated
	}
	r.loadFired = true
	r.mu.Unlock()
	_ = r.drainMicrotasks()

	before := r.mutationStamp()
	_, _ = r.callBounded(func() (goja.Value, error) {
		r.setReadyState(ReadyComplete)
		doc := &r.domDoc.Node
		r.dispatchWithWindowStep(doc, dom.NewTrustedEvent("load", false, false))
		r.dispatchWithWindowStep(doc, dom.NewPageTransitionEvent("pageshow", true, true, false))
		return goja.Undefined(), nil
	})
	return mutated || r.mutationStamp() != before
}

// FireDocumentLifecycle runs both document lifecycle phases in order. It is
// used by detached child documents without load-delaying subresources; the
// embedding engine uses the two phase methods so it can wait for iframes.
// readyState changes to "interactive" before DOMContentLoaded and to
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
// whether another style/layout pass is owed. Each phase is idempotent.
func (r *Runtime) FireDocumentLifecycle() bool {
	return r.FireDocumentLoad()
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
