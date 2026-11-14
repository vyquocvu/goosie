package js

import (
	"reflect"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

const (
	ReadyLoading     = "loading"
	ReadyInteractive = "interactive"
	ReadyComplete    = "complete"
)

type windowListener struct {
	typ        string
	fn         goja.Callable
	fnVal      goja.Value
	capture    bool
	once       bool
	passive    bool
	signal     *goja.Object
	handler    bool
	content    string
	hasContent bool
	compiled   bool
}

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

func (r *Runtime) fireWindowListeners(ev *dom.Event, capture bool) {
	r.winMu.Lock()
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

func (r *Runtime) dispatchWithWindowStep(target *dom.Node, ev *dom.Event) bool {
	if target == nil {
		return true
	}
	path := dom.EventPath(target)
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

func (r *Runtime) FireDocumentLifecycle() bool {
	return r.FireDocumentLoad()
}

func (r *Runtime) mutationStamp() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mutations
}
