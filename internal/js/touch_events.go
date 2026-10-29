package js

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// installTouchEvents exposes the TouchEvent, Touch and TouchList interfaces.
// WPT's dom/events corpus needs them twice over: synthetic-events-cancelable.html
// asserts `TouchEvent in self` for all four touch types and constructs one, and
// the non-cancelable-when-passive files read the lists off a generated gesture.
//
// Touch lists are read-only array-likes: length plus item(), indexable, and never
// writable, so a page cannot fake a contact into or out of a live gesture.
func (r *Runtime) installTouchEvents() {
	isTrustedGetter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return call.This.ToObject(r.vm).Get("__isTrusted")
	})
	readBool := func(obj *goja.Object, name string) bool {
		if obj == nil {
			return false
		}
		value := obj.Get(name)
		return value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) && value.ToBoolean()
	}
	readNumber := func(obj *goja.Object, name string) float64 {
		if obj == nil {
			return 0
		}
		value := obj.Get(name)
		if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
			return 0
		}
		return value.ToFloat()
	}

	_ = r.vm.Set("Touch", r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) == 0 || call.Arguments[0] == nil || goja.IsUndefined(call.Arguments[0]) || goja.IsNull(call.Arguments[0]) {
			panic(r.vm.NewTypeError("Failed to construct 'Touch': 1 argument required"))
		}
		init := call.Arguments[0].ToObject(r.vm)
		target := init.Get("target")
		if target == nil || goja.IsUndefined(target) || goja.IsNull(target) {
			panic(r.vm.NewTypeError("Failed to construct 'Touch': parameter 1 is not of type 'EventTarget'"))
		}
		r.writeTouchFields(call.This, int(readNumber(init, "identifier")),
			readNumber(init, "clientX"), readNumber(init, "clientY"), target)
		return call.This
	}))

	_ = r.vm.Set("TouchList", r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		items := make([]*goja.Object, 0, len(call.Arguments))
		for _, argument := range call.Arguments {
			if argument == nil || goja.IsUndefined(argument) || goja.IsNull(argument) {
				continue
			}
			items = append(items, argument.ToObject(r.vm))
		}
		r.writeTouchListFields(call.This, items)
		return call.This
	}))

	_ = r.vm.Set("TouchEvent", r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) == 0 || call.Arguments[0] == nil {
			panic(r.vm.NewTypeError("Failed to construct 'TouchEvent': 1 argument required"))
		}
		obj := call.This
		_ = obj.Set("type", call.Arguments[0].String())
		bubbles, cancelable := false, false
		var init *goja.Object
		if len(call.Arguments) > 1 {
			if value := call.Arguments[1]; value != nil && !goja.IsNull(value) && !goja.IsUndefined(value) {
				init = value.ToObject(r.vm)
				bubbles = readBool(init, "bubbles")
				cancelable = readBool(init, "cancelable")
			}
		}
		_ = obj.Set("bubbles", bubbles)
		_ = obj.Set("cancelable", cancelable)
		_ = obj.Set("defaultPrevented", false)
		_ = obj.Set("__isTrusted", false)
		_ = obj.DefineAccessorProperty("isTrusted", isTrustedGetter, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
		_ = obj.Set("__dispatching", false)
		_ = obj.Set("__initialized", true)
		_ = obj.Set("__goosieEvent", true)
		_ = obj.Set("__goosieEventInterface", "TouchEvent")
		_ = obj.Set("detail", int64(0))
		_ = obj.Set("view", goja.Null())
		for _, key := range []string{"altKey", "ctrlKey", "shiftKey", "metaKey"} {
			_ = obj.Set(key, readBool(init, key))
		}
		for _, key := range []string{"touches", "targetTouches", "changedTouches"} {
			var items []*goja.Object
			if init != nil {
				if value := init.Get(key); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
					items = touchItems(value.ToObject(r.vm))
				}
			}
			list := r.vm.NewObject()
			r.writeTouchListFields(list, items)
			_ = obj.Set(key, list)
		}
		_ = obj.Set("preventDefault", func() {
			if cancelable {
				_ = obj.Set("defaultPrevented", true)
			}
		})
		_ = obj.Set("stopPropagation", func() {})
		_ = obj.Set("stopImmediatePropagation", func() {})
		return obj
	}))

	// TouchEvent derives from UIEvent which derives from Event; Touch and
	// TouchList stand on their own. The toStringTag brands are what
	// Object.prototype.toString.call reports, so a constructed event and a
	// dispatched one read identically.
	_, _ = r.vm.RunString(`
		for (const name of ["TouchEvent", "Touch", "TouchList"]) {
			Object.defineProperty(globalThis[name].prototype, Symbol.toStringTag, { value: name, configurable: true });
		}
		Object.setPrototypeOf(TouchEvent.prototype, typeof UIEvent === "function" ? UIEvent.prototype : Event.prototype);
	`)
}

// writeTouchFields fills one Touch object with a contact's state. The caller owns
// the object - the constructor passes `this`, the list builder passes a fresh one
// - so a Touch built by a page and one delivered in a generated event are the same
// shape.
func (r *Runtime) writeTouchFields(obj *goja.Object, identifier int, clientX, clientY float64, target goja.Value) {
	obj.SetPrototype(r.interfacePrototype("Touch"))
	_ = obj.Set("identifier", int64(identifier))
	_ = obj.Set("clientX", clientX)
	_ = obj.Set("clientY", clientY)
	// TouchInit leaves page and screen coordinates unset and the interface mirrors
	// the client position for both, so a page reading any of the three sees one
	// place.
	_ = obj.Set("pageX", clientX)
	_ = obj.Set("pageY", clientY)
	_ = obj.Set("screenX", clientX)
	_ = obj.Set("screenY", clientY)
	_ = obj.Set("force", 1)
	_ = obj.Set("radiusX", 0)
	_ = obj.Set("radiusY", 0)
	_ = obj.Set("rotationAngle", 0)
	_ = obj.Set("altitudeAngle", 1.5707963267948966)
	if target == nil {
		target = goja.Null()
	}
	_ = obj.Set("target", target)
	_ = obj.Set("__goosieTouch", true)
}

// writeTouchListFields turns items into a TouchList on obj: indexable, with a
// length and item(), and no mutation path.
func (r *Runtime) writeTouchListFields(obj *goja.Object, items []*goja.Object) {
	if obj == nil {
		return
	}
	obj.SetPrototype(r.interfacePrototype("TouchList"))
	if items == nil {
		items = []*goja.Object{}
	}
	for index, item := range items {
		_ = obj.Set(fmt.Sprint(index), item)
	}
	_ = obj.Set("length", int64(len(items)))
	_ = obj.Set("item", func(call goja.FunctionCall) goja.Value {
		index := int64(-1)
		if len(call.Arguments) > 0 && call.Arguments[0] != nil {
			index = call.Arguments[0].ToInteger()
		}
		if index < 0 || index >= int64(len(items)) {
			return goja.Null()
		}
		return items[index]
	})
	_ = obj.Set("__goosieTouchList", true)
}

// newTouchList builds a TouchList from host contacts for a dispatched event.
func (r *Runtime) newTouchList(touches []*dom.Touch) *goja.Object {
	items := make([]*goja.Object, 0, len(touches))
	for _, touch := range touches {
		item := r.vm.NewObject()
		var target goja.Value = goja.Null()
		identifier, clientX, clientY := 0, 0.0, 0.0
		if touch != nil {
			identifier, clientX, clientY = touch.Identifier, touch.ClientX, touch.ClientY
			target = r.wrapNode(touch.Target)
		}
		r.writeTouchFields(item, identifier, clientX, clientY, target)
		items = append(items, item)
	}
	list := r.vm.NewObject()
	// The index properties, length and item() are all written together, so the
	// wrapped contacts have to exist before the list is shaped.
	r.writeTouchListFields(list, items)
	return list
}

// interfacePrototype returns the prototype object of a global constructor, so a
// list or contact built outside `new` still answers `instanceof`. A missing or
// non-object global leaves the value at Object.prototype rather than breaking the
// property reads a page depends on.
func (r *Runtime) interfacePrototype(name string) *goja.Object {
	value := r.vm.Get(name)
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	object, ok := value.(*goja.Object)
	if !ok {
		return nil
	}
	prototype, ok := object.Get("prototype").(*goja.Object)
	if !ok || prototype == nil || goja.IsUndefined(prototype) || goja.IsNull(prototype) {
		return nil
	}
	return prototype
}

// touchItems reads a JS array-like of Touch objects into a slice. The corpus
// passes array literals; a TouchList also has a numeric length, so both work here.
func touchItems(value *goja.Object) []*goja.Object {
	lengthValue := value.Get("length")
	if lengthValue == nil || goja.IsUndefined(lengthValue) || goja.IsNull(lengthValue) {
		return nil
	}
	length := int(lengthValue.ToInteger())
	items := make([]*goja.Object, 0, length)
	for index := 0; index < length; index++ {
		element := value.Get(fmt.Sprint(index))
		if element == nil || goja.IsUndefined(element) || goja.IsNull(element) {
			continue
		}
		if object, ok := element.(*goja.Object); ok {
			items = append(items, object)
		}
	}
	return items
}

// TouchCancelableByListener reports whether the touch listeners a gesture
// starting at target can reach may still cancel the scroll it causes. Only the
// type being dispatched decides - touchstart and touchmove cancel a scroll, so
// each derives its own flag - and a listener registered without options on a
// default-passive target already stores passive=true.
//
// Like a wheel gesture, the answer is one value for the whole dispatch, which
// every listener of it reads.
func (r *Runtime) TouchCancelableByListener(target *dom.Node, typ string) bool {
	matches := func(listenerType string, passive bool) bool {
		return listenerType == typ && !passive
	}
	for node := target; node != nil; node = node.Parent {
		for _, listener := range dom.GetNodeListeners(node) {
			if matches(listener.Type, listener.Passive) {
				return true
			}
		}
	}
	r.winMu.Lock()
	defer r.winMu.Unlock()
	for _, listener := range r.winListeners {
		if matches(listener.typ, listener.passive) {
			return true
		}
	}
	return false
}

// DispatchTrustedTouch delivers one action of a touch gesture raised by the host
// rather than by the page. It derives the event's cancelable flag from the
// listeners this dispatch reaches - which is why the event arrives already built
// with its touch lists - and returns whether the gesture survived, i.e. whether
// the scroll it caused may run.
func (r *Runtime) DispatchTrustedTouch(target *dom.Node, ev *dom.Event) bool {
	if r == nil || ev == nil {
		return true
	}
	if dom.IsScrollBlockingEventType(ev.Type) {
		ev.SetScrollCancelableByListener(r.TouchCancelableByListener(target, ev.Type))
	}
	return r.dispatchWithWindowStep(target, ev)
}
