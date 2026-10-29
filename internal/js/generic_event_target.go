package js

import (
	"reflect"
	"strconv"

	"github.com/dop251/goja"
)

// genericEventListener is the listener record used by ordinary EventTarget
// instances. DOM nodes keep their richer propagation records in internal/dom.
type genericEventListener struct {
	typ     string
	value   goja.Value
	fn      goja.Callable
	capture bool
	once    bool
	passive bool
	realm   *Runtime
}

func (r *Runtime) defineEventTargetConstructor() {
	r.defineInterfaceToStringTag("EventTarget", r.eventTargetProto)
	ctor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		if r.genericEventTargets == nil {
			r.genericEventTargets = make(map[*goja.Object][]*genericEventListener)
		}
		r.genericEventTargets[call.This] = nil
		return call.This
	}).(*goja.Object)
	if !ok {
		return
	}
	_ = r.vm.Set("__eventTargetConstructor__", ctor)
	_, _ = r.vm.RunString(`Object.defineProperty(__eventTargetConstructor__, "name", { value: "EventTarget", configurable: true })`)
	_ = r.vm.Set("__eventTargetConstructor__", nil)
	_ = ctor.Set("prototype", r.eventTargetProto)
	_ = r.eventTargetProto.Set("constructor", ctor)
	r.ifaceCtors["EventTarget"] = ctor
	_ = r.vm.Set("EventTarget", ctor)
	_ = r.eventTargetProto.DefineDataProperty("addEventListener", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.addGenericEventListener(call)
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.eventTargetProto.DefineDataProperty("removeEventListener", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.jsRemoveGenericEventListener(call)
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.eventTargetProto.DefineDataProperty("dispatchEvent", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.dispatchGenericEvent(call)
	}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
}

func (r *Runtime) initializeGenericEventTarget(target *goja.Object) {
	if r.genericEventTargets == nil {
		r.genericEventTargets = make(map[*goja.Object][]*genericEventListener)
	}
	r.genericEventTargets[target] = nil
	if r.eventTargetProto != nil {
		_ = target.SetPrototype(r.eventTargetProto)
	}
}

func (r *Runtime) jsCreateLegacyEvent(interfaceName string) goja.Value {
	switch interfaceName {
	case "BeforeUnloadEvent", "CompositionEvent", "CustomEvent", "DeviceMotionEvent", "DeviceOrientationEvent",
		"DragEvent", "Event", "Events", "FocusEvent", "HashChangeEvent", "HTMLEvents", "KeyboardEvent",
		"MessageEvent", "MouseEvent", "MouseEvents", "StorageEvent", "SVGEvents", "TextEvent", "UIEvent", "UIEvents":
	default:
		panic(r.domError("NotSupportedError", "The requested event interface is not supported"))
	}
	event := r.vm.NewObject()
	if ctor, ok := r.vm.Get("Event").(*goja.Object); ok {
		if proto, ok := ctor.Get("prototype").(*goja.Object); ok {
			_ = event.SetPrototype(proto)
		}
	}
	_ = event.Set("type", "")
	_ = event.Set("bubbles", false)
	_ = event.Set("cancelable", false)
	_ = event.Set("defaultPrevented", false)
	_ = event.Set("target", goja.Null())
	_ = event.Set("currentTarget", goja.Null())
	_ = event.Set("eventPhase", 0)
	_ = event.Set("srcElement", goja.Null())
	_ = event.Set("isTrusted", false)
	_ = event.Set("__propagationStopped", false)
	_ = event.Set("__immediateStopped", false)
	_ = event.Set("__passiveListener", false)
	_ = event.Set("__dispatching", false)
	_ = event.Set("__initialized", false)
	_ = event.Set("__goosieEvent", true)
	_ = event.Set("__composedPath", r.vm.NewArray())
	_ = event.Set("preventDefault", func() {
		if event.Get("cancelable").ToBoolean() && !event.Get("__passiveListener").ToBoolean() {
			_ = event.Set("defaultPrevented", true)
		}
	})
	_ = event.Set("stopPropagation", func() { _ = event.Set("__propagationStopped", true) })
	_ = event.Set("stopImmediatePropagation", func() { _ = event.Set("__propagationStopped", true); _ = event.Set("__immediateStopped", true) })
	_ = event.DefineAccessorProperty("cancelBubble", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(event.Get("__propagationStopped").ToBoolean())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if call.Argument(0).ToBoolean() {
			_ = event.Set("__propagationStopped", true)
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = event.Set("initEvent", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'initEvent': 1 argument required"))
		}
		if event.Get("__dispatching").ToBoolean() {
			return goja.Undefined()
		}
		if len(call.Arguments) > 0 {
			_ = event.Set("type", call.Arguments[0].String())
		}
		_ = event.Set("bubbles", len(call.Arguments) > 1 && call.Arguments[1].ToBoolean())
		_ = event.Set("cancelable", len(call.Arguments) > 2 && call.Arguments[2].ToBoolean())
		_ = event.Set("defaultPrevented", false)
		_ = event.Set("__propagationStopped", false)
		_ = event.Set("__immediateStopped", false)
		_ = event.Set("__initialized", true)
		return goja.Undefined()
	})
	_ = event.Set("initCustomEvent", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'initCustomEvent': 1 argument required"))
		}
		if event.Get("__dispatching").ToBoolean() {
			return goja.Undefined()
		}
		if len(call.Arguments) > 0 {
			_ = event.Set("type", call.Arguments[0].String())
		}
		if len(call.Arguments) > 1 {
			_ = event.Set("bubbles", call.Arguments[1].ToBoolean())
		}
		if len(call.Arguments) > 2 {
			_ = event.Set("cancelable", call.Arguments[2].ToBoolean())
		}
		if len(call.Arguments) > 3 {
			_ = event.Set("detail", call.Arguments[3])
		} else {
			_ = event.Set("detail", goja.Null())
		}
		_ = event.Set("defaultPrevented", false)
		_ = event.Set("__propagationStopped", false)
		_ = event.Set("__immediateStopped", false)
		_ = event.Set("__initialized", true)
		return goja.Undefined()
	})
	_ = event.DefineAccessorProperty("returnValue", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(!event.Get("defaultPrevented").ToBoolean())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if !call.Argument(0).ToBoolean() {
			if fn, ok := goja.AssertFunction(event.Get("preventDefault")); ok {
				_, _ = fn(event)
			}
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = event.Set("composedPath", func() goja.Value {
		if !event.Get("__dispatching").ToBoolean() {
			return r.vm.NewArray()
		}
		path, _ := event.Get("__composedPath").(*goja.Object)
		if path == nil {
			return r.vm.NewArray()
		}
		length := path.Get("length").ToInteger()
		values := make([]interface{}, length)
		for i := int64(0); i < length; i++ {
			values[i] = path.Get(strconv.FormatInt(i, 10))
		}
		return r.vm.NewArray(values...)
	})
	return event
}

func genericListenerOptions(vm *goja.Runtime, args []goja.Value) (capture, once, passive bool, signal goja.Value, hasSignal bool) {
	if len(args) == 0 || args[0] == nil || goja.IsUndefined(args[0]) || goja.IsNull(args[0]) {
		return
	}
	if args[0].ExportType().Kind() == reflect.Bool {
		capture = args[0].ToBoolean()
		return
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
	if value := options.Get("signal"); value != nil && !goja.IsUndefined(value) {
		signal, hasSignal = value, true
	}
	return
}

func (r *Runtime) addGenericEventListener(call goja.FunctionCall) {
	target, ok := call.This.(*goja.Object)
	if !ok {
		return
	}
	if channel := broadcastChannelForTarget(r, target); channel != nil {
		addBroadcastEventListener(r, channel, call.Arguments)
		return
	}
	if len(call.Arguments) < 2 {
		return
	}
	value := call.Arguments[1]
	fn, callable := goja.AssertFunction(value)
	listenerRealm := r
	if object, ok := value.(*goja.Object); ok {
		if callable {
			if realm := r.iframeImportedFunctionRealms[object]; realm != nil {
				listenerRealm = realm
			} else {
				listenerRealm = r.runtimeForFunctionObject(object)
			}
		} else {
			listenerRealm = r.importedObjectRealm(object)
		}
	}
	typ := call.Arguments[0].String()
	capture, once, passive, signalValue, hasSignal := genericListenerOptions(r.vm, call.Arguments[2:])
	var signal *goja.Object
	if hasSignal {
		var valid bool
		signal, valid = signalValue.(*goja.Object)
		if !valid || goja.IsNull(signalValue) || goja.IsUndefined(signalValue) {
			panic(r.vm.NewTypeError("EventListenerOptions.signal must be an AbortSignal"))
		}
		if _, callable := goja.AssertFunction(signal.Get("__registerAbortAlgorithm__")); !callable {
			panic(r.vm.NewTypeError("EventListenerOptions.signal must be an AbortSignal"))
		}
		if signal.Get("aborted").ToBoolean() {
			return
		}
	}
	if !callable {
		if goja.IsNull(value) || goja.IsUndefined(value) {
			return
		}
		if _, object := value.(*goja.Object); !object {
			return
		}
	}
	listeners := r.genericEventTargets[target]
	for _, listener := range listeners {
		if listener.typ == typ && listener.value == value && listener.capture == capture {
			return
		}
	}
	listener := &genericEventListener{typ: typ, value: value, fn: fn, capture: capture, once: once, passive: passive, realm: listenerRealm}
	r.genericEventTargets[target] = append(listeners, listener)
	if signal != nil {
		register, _ := goja.AssertFunction(signal.Get("__registerAbortAlgorithm__"))
		cleanup := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			r.removeGenericListener(target, listener)
			return goja.Undefined()
		})
		_, _ = register(signal, cleanup)
	}
}

func (r *Runtime) jsRemoveGenericEventListener(call goja.FunctionCall) {
	target, ok := call.This.(*goja.Object)
	if !ok {
		return
	}
	if channel := broadcastChannelForTarget(r, target); channel != nil {
		removeBroadcastEventListener(r, channel, call.Arguments)
		return
	}
	if len(call.Arguments) < 2 {
		return
	}
	capture := false
	if len(call.Arguments) > 2 && !goja.IsNull(call.Arguments[2]) && !goja.IsUndefined(call.Arguments[2]) {
		if call.Arguments[2].ExportType().Kind() == reflect.Bool {
			capture = call.Arguments[2].ToBoolean()
		} else {
			if value := call.Arguments[2].ToObject(r.vm).Get("capture"); value != nil && !goja.IsUndefined(value) {
				capture = value.ToBoolean()
			}
		}
	}
	typ, value := call.Arguments[0].String(), call.Arguments[1]
	for _, listener := range r.genericEventTargets[target] {
		if listener.typ == typ && listener.value == value && listener.capture == capture {
			r.removeGenericListener(target, listener)
			return
		}
	}
}

func (r *Runtime) removeGenericListener(target *goja.Object, wanted *genericEventListener) {
	listeners := r.genericEventTargets[target]
	for i, listener := range listeners {
		if listener == wanted {
			listeners = append(listeners[:i], listeners[i+1:]...)
			r.genericEventTargets[target] = listeners
			return
		}
	}
}

func (r *Runtime) dispatchGenericEvent(call goja.FunctionCall) goja.Value {
	target, ok := call.This.(*goja.Object)
	if !ok || len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'dispatchEvent': 1 argument required"))
	}
	if channel := broadcastChannelForTarget(r, target); channel != nil {
		return dispatchBroadcastEventCall(r, channel, call.Arguments)
	}
	event, ok := call.Arguments[0].(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("parameter 1 is not of type 'Event'"))
	}
	if value := event.Get("__dispatching"); value != nil && value.ToBoolean() {
		panic(r.domError("InvalidStateError", "The event is already being dispatched"))
	}
	if value := event.Get("__initialized"); value != nil && !value.ToBoolean() {
		panic(r.domError("InvalidStateError", "The event has not been initialized"))
	}
	typValue := event.Get("type")
	if typValue == nil || goja.IsUndefined(typValue) {
		panic(r.vm.NewTypeError("parameter 1 is not of type 'Event'"))
	}
	_ = event.Set("__dispatching", true)
	defer func() {
		_ = event.Set("__dispatching", false)
		_ = event.Set("currentTarget", goja.Null())
		_ = event.Set("eventPhase", 0)
		_ = event.Set("__composedPath", r.vm.NewArray())
	}()
	_ = event.Set("target", target)
	_ = event.Set("currentTarget", target)
	_ = event.Set("eventPhase", 2)
	_ = event.Set("__composedPath", r.vm.NewArray(target))
	listeners := append([]*genericEventListener(nil), r.genericEventTargets[target]...)
	for _, listener := range listeners {
		// DOM's inner invoke skips callbacks when the event target's global is
		// no longer fully active. Recheck before every listener because an
		// earlier callback can remove the iframe containing this target.
		if !r.isFullyActiveBrowsingContext() {
			break
		}
		if listener.typ != typValue.String() || !r.genericListenerPresent(target, listener) {
			continue
		}
		if listener.once {
			r.removeGenericListener(target, listener)
		}
		if listener.realm != nil && !listener.realm.isFullyActiveBrowsingContext() {
			continue
		}
		_ = event.Set("__passiveListener", listener.passive)
		var callbackErr error
		r.withEventListenerInvocation(func() {
			r.withCurrentEvent(event, func() {
				if listener.fn != nil {
					_, callbackErr = listener.fn(target, event)
				} else {
					handler := listener.value.ToObject(r.vm).Get("handleEvent")
					if fn, ok := goja.AssertFunction(handler); ok {
						_, callbackErr = fn(listener.value, event)
					}
				}
			})
		})
		listener.realm.reportWindowExceptionFrom(r, callbackErr)
		_ = event.Set("__passiveListener", false)
		if value := event.Get("__immediateStopped"); value != nil && value.ToBoolean() {
			break
		}
	}
	defaultPrevented := event.Get("defaultPrevented")
	return r.vm.ToValue(defaultPrevented == nil || !defaultPrevented.ToBoolean())
}

func (r *Runtime) genericListenerPresent(target *goja.Object, wanted *genericEventListener) bool {
	for _, listener := range r.genericEventTargets[target] {
		if listener == wanted {
			return true
		}
	}
	return false
}
