package js

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

type bcEvent struct {
	dataJSON string
	origin   string
}

type broadcastDelivery struct {
	channel *bcObj
	event   bcEvent
}

type bcObj struct {
	mu             sync.Mutex
	name           string
	key            string
	origin         string
	closed         bool
	obj            *goja.Object
	r              *Runtime
	hub            *BroadcastHub
	handlers       map[string]goja.Value
	listeners      []bcListener
	nextListenerID uint64
}

type bcListener struct {
	id       uint64
	typ      string
	callback goja.Value
	capture  bool
	once     bool
	passive  bool
	handler  bool
}

func (ch *bcObj) appendListener(listener bcListener) uint64 {
	ch.nextListenerID++
	listener.id = ch.nextListenerID
	ch.listeners = append(ch.listeners, listener)
	return listener.id
}

func (ch *bcObj) removeListenerID(id uint64) {
	ch.mu.Lock()
	for i, listener := range ch.listeners {
		if listener.id == id {
			if listener.handler {
				ch.handlers[listener.typ] = goja.Null()
			}
			ch.listeners = append(ch.listeners[:i], ch.listeners[i+1:]...)
			break
		}
	}
	ch.mu.Unlock()
}

func (ch *bcObj) setHandler(typ string, value goja.Value) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	previous := ch.handlers[typ]
	if previous != nil && previous.StrictEquals(value) {
		return
	}
	for i := range ch.listeners {
		listener := &ch.listeners[i]
		if listener.typ != typ || !listener.handler {
			continue
		}
		if goja.IsNull(value) || goja.IsUndefined(value) {
			ch.listeners = append(ch.listeners[:i], ch.listeners[i+1:]...)
			ch.handlers[typ] = goja.Null()
			return
		}
		listener.callback = value
		ch.handlers[typ] = value
		return
	}
	ch.handlers[typ] = value
	if goja.IsNull(value) || goja.IsUndefined(value) {
		return
	}
	ch.appendListener(bcListener{typ: typ, callback: value, handler: true})
}

func broadcastListenerValue(r *Runtime, value goja.Value) bool {
	if _, ok := goja.AssertFunction(value); ok {
		return true
	}
	object, ok := value.(*goja.Object)
	if !ok {
		return false
	}
	_, ok = goja.AssertFunction(object.Get("handleEvent"))
	return ok
}

func broadcastChannelForTarget(r *Runtime, target *goja.Object) *bcObj {
	if r == nil || target == nil {
		return nil
	}
	state := broadcastStateOf(r)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	channel := state.byObject[target]
	state.mu.Unlock()
	return channel
}

func addBroadcastEventListener(r *Runtime, channel *bcObj, args []goja.Value) {
	if r == nil || channel == nil || len(args) < 2 || goja.IsNull(args[1]) || goja.IsUndefined(args[1]) {
		return
	}
	typ, callback := args[0].String(), args[1]
	capture, once, passive, signalValue, hasSignal := genericListenerOptions(r.vm, args[2:])
	var signal *goja.Object
	if hasSignal {
		var valid bool
		signal, valid = signalValue.(*goja.Object)
		if !valid || goja.IsNull(signalValue) || goja.IsUndefined(signalValue) {
			panic(r.vm.NewTypeError("EventListenerOptions.signal must be an AbortSignal"))
		}
		if _, valid := goja.AssertFunction(signal.Get("__registerAbortAlgorithm__")); !valid {
			panic(r.vm.NewTypeError("EventListenerOptions.signal must be an AbortSignal"))
		}
		if signal.Get("aborted").ToBoolean() {
			return
		}
	}

	channel.mu.Lock()
	for _, listener := range channel.listeners {
		if listener.typ == typ && listener.capture == capture && listener.callback.StrictEquals(callback) {
			channel.mu.Unlock()
			return
		}
	}
	id := channel.appendListener(bcListener{typ: typ, callback: callback, capture: capture, once: once, passive: passive})
	channel.mu.Unlock()

	if signal != nil {
		register, _ := goja.AssertFunction(signal.Get("__registerAbortAlgorithm__"))
		cleanup := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			channel.removeListenerID(id)
			return goja.Undefined()
		})
		_, _ = register(signal, cleanup)
	}
}

func removeBroadcastEventListener(r *Runtime, channel *bcObj, args []goja.Value) {
	if r == nil || channel == nil || len(args) < 2 {
		return
	}
	typ, callback := args[0].String(), args[1]
	capture := false
	if len(args) > 2 && !goja.IsNull(args[2]) && !goja.IsUndefined(args[2]) {
		if args[2].ExportType().Kind() == reflect.Bool {
			capture = args[2].ToBoolean()
		} else if options, ok := args[2].(*goja.Object); ok {
			if value := options.Get("capture"); value != nil && !goja.IsUndefined(value) {
				capture = value.ToBoolean()
			}
		} else {
			capture = args[2].ToObject(r.vm).Get("capture").ToBoolean()
		}
	}
	channel.mu.Lock()
	defer channel.mu.Unlock()
	for i, listener := range channel.listeners {
		if listener.typ == typ && listener.capture == capture && listener.callback.StrictEquals(callback) {
			if listener.handler {
				channel.handlers[typ] = goja.Null()
			}
			channel.listeners = append(channel.listeners[:i], channel.listeners[i+1:]...)
			return
		}
	}
}

type BroadcastHub struct {
	mu            sync.Mutex
	channels      map[string][]*bcObj
	pending       map[*Runtime][]broadcastDelivery
	sharedWorkers map[string]*sharedWorkerEntry
}

var defaultHub = &BroadcastHub{
	channels:      make(map[string][]*bcObj),
	pending:       make(map[*Runtime][]broadcastDelivery),
	sharedWorkers: make(map[string]*sharedWorkerEntry),
}

func NewBroadcastHub() *BroadcastHub {
	return &BroadcastHub{
		channels:      make(map[string][]*bcObj),
		pending:       make(map[*Runtime][]broadcastDelivery),
		sharedWorkers: make(map[string]*sharedWorkerEntry),
	}
}

func (h *BroadcastHub) register(ch *bcObj) {
	h.mu.Lock()
	h.channels[ch.key] = append(h.channels[ch.key], ch)
	h.mu.Unlock()
}

func (h *BroadcastHub) unregister(ch *bcObj) {
	h.mu.Lock()
	defer h.mu.Unlock()
	slice := h.channels[ch.key]
	for i, c := range slice {
		if c == ch {
			h.channels[ch.key] = append(slice[:i], slice[i+1:]...)
			break
		}
	}
	if len(h.channels[ch.key]) == 0 {
		delete(h.channels, ch.key)
	}
}

func (h *BroadcastHub) broadcast(sender *bcObj, dataJSON string) {
	if sender == nil || !broadcastContextActive(sender.r) {
		return
	}
	h.mu.Lock()
	for _, peer := range h.channels[sender.key] {
		if peer == sender {
			continue
		}
		if !broadcastContextActive(peer.r) {
			continue
		}
		peer.mu.Lock()
		closed := peer.closed
		peer.mu.Unlock()
		if !closed {
			root := broadcastContextRoot(peer.r)
			h.pending[root] = append(h.pending[root], broadcastDelivery{
				channel: peer,
				event:   bcEvent{dataJSON: dataJSON, origin: sender.origin},
			})
		}
	}
	h.mu.Unlock()
}

type broadcastState struct {
	mu       sync.Mutex
	hub      *BroadcastHub
	byObject map[*goja.Object]*bcObj
}

func broadcastContextRoot(r *Runtime) *Runtime {
	for r != nil {
		if r.workerParent != nil {
			r = r.workerParent
			continue
		}
		if r.iframeParent != nil {
			if r.iframeParent.iframeOpenedContexts[r.iframeParentFrame] {
				return r
			}
			r = r.iframeParent
			continue
		}
		return r
	}
	return nil
}

func setBroadcastState(r *Runtime, s *broadcastState) {
	r.broadcastStateMu.Lock()
	r.broadcastState = s
	r.broadcastStateMu.Unlock()
}

func broadcastStateOf(r *Runtime) *broadcastState {
	if r == nil {
		return nil
	}
	r.broadcastStateMu.Lock()
	defer r.broadcastStateMu.Unlock()
	return r.broadcastState
}

func broadcastContextActive(r *Runtime) bool {
	for current := r; current != nil && current.iframeParent != nil; current = current.iframeParent {
		if current.workerClosed {
			return false
		}
		if current.iframeParentFrame != nil {
			if !current.iframeParentFrame.IsConnected() || current.iframeParent.iframeClosedContexts[current.iframeParentFrame] {
				return false
			}
		}
	}
	return r != nil && !r.workerClosed
}

func (r *Runtime) setupBroadcastChannel() {
	if r.eventTargetProto == nil {
		r.eventTargetProto = r.vm.NewObject()
		if r.ifaceCtors == nil {
			r.ifaceCtors = make(map[string]*goja.Object)
		}
		r.defineEventTargetConstructor()
	}
	hub := r.opts.BroadcastHub
	if hub == nil {
		hub = defaultHub
	}

	state := &broadcastState{hub: hub, byObject: make(map[*goja.Object]*bcObj)}
	setBroadcastState(r, state)

	constructor := func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("Failed to construct 'BroadcastChannel': 1 argument required, but only 0 present."))
		}

		name := call.Arguments[0].String()

		ch := &bcObj{
			name:     name,
			r:        r,
			hub:      hub,
			handlers: map[string]goja.Value{"message": goja.Null(), "messageerror": goja.Null()},
		}
		ch.origin = runtimeBroadcastOrigin(r)
		storageOrigin := ch.origin
		if storageOrigin == "null" {
			storageOrigin = runtimeBroadcastOpaqueKey(r)
		}
		storageKey := storageOrigin
		if ch.origin != "null" {
			storageKey = runtimeBroadcastTopLevelSite(r) + "\x00" + storageOrigin
		}
		ch.key = storageKey + "\x00" + name

		obj := call.This
		ch.obj = obj
		if r.genericEventTargets == nil {
			r.genericEventTargets = make(map[*goja.Object][]*genericEventListener)
		}
		r.genericEventTargets[obj] = nil

		hub.register(ch)
		state.mu.Lock()
		state.byObject[obj] = ch
		state.mu.Unlock()

		return nil
	}

	ctor := r.vm.ToValue(constructor)
	if constructorObject, ok := ctor.(*goja.Object); ok {
		if prototype, ok := constructorObject.Get("prototype").(*goja.Object); ok {
			if r.eventTargetProto != nil {
				_ = prototype.SetPrototype(r.eventTargetProto)
			}
			channelForThis := func(this goja.Value) *bcObj {
				object, ok := this.(*goja.Object)
				if !ok {
					return nil
				}
				state.mu.Lock()
				channel := state.byObject[object]
				state.mu.Unlock()
				return channel
			}
			getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				channel := channelForThis(call.This)
				if channel == nil {
					panic(r.vm.NewTypeError("Illegal invocation"))
				}
				return r.vm.ToValue(channel.name)
			})
			_ = prototype.DefineAccessorProperty("name", getter, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
			postMessage := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				channel := channelForThis(call.This)
				if channel == nil {
					panic(r.vm.NewTypeError("Illegal invocation"))
				}
				if len(call.Arguments) < 1 {
					panic(r.vm.NewTypeError("Failed to execute 'postMessage': 1 argument required"))
				}
				channel.mu.Lock()
				if channel.closed {
					channel.mu.Unlock()
					panic(r.domError("InvalidStateError", "The BroadcastChannel is closed"))
				}
				channel.mu.Unlock()

				encoded, err := serializeBroadcastMessage(r, call.Arguments[0])
				if err != nil {
					panic(r.domError("DataCloneError", "The message could not be cloned"))
				}
				channel.hub.broadcast(channel, string(encoded))
				return goja.Undefined()
			})
			_ = prototype.DefineDataProperty("postMessage", postMessage, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
			closeChannel := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				channel := channelForThis(call.This)
				if channel == nil {
					panic(r.vm.NewTypeError("Illegal invocation"))
				}
				channel.mu.Lock()
				if channel.closed {
					channel.mu.Unlock()
					return goja.Undefined()
				}
				channel.closed = true
				channel.mu.Unlock()

				channel.hub.unregister(channel)
				return goja.Undefined()
			})
			_ = prototype.DefineDataProperty("close", closeChannel, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
			for property, typ := range map[string]string{"onmessage": "message", "onmessageerror": "messageerror"} {
				property, typ := property, typ
				getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
					channel := channelForThis(call.This)
					if channel == nil {
						panic(r.vm.NewTypeError("Illegal invocation"))
					}
					channel.mu.Lock()
					defer channel.mu.Unlock()
					return channel.handlers[typ]
				})
				setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
					channel := channelForThis(call.This)
					if channel == nil {
						panic(r.vm.NewTypeError("Illegal invocation"))
					}
					value := call.Argument(0)
					if !broadcastListenerValue(r, value) {
						value = goja.Null()
					}
					channel.setHandler(typ, value)
					return goja.Undefined()
				})
				_ = prototype.DefineAccessorProperty(property, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
			}
		}
	}
	const nativeCtorName = "__goosieNativeBroadcastChannelCtor"
	_ = r.vm.Set(nativeCtorName, ctor)
	wrapperValue, wrapErr := r.vm.RunString(`(function(nativeCtor) {
		function BroadcastChannel(name) {
			if (!new.target) {
				throw new TypeError("Failed to construct 'BroadcastChannel': Please use the 'new' operator.");
			}
			return Reflect.construct(nativeCtor, arguments, new.target);
		}
		BroadcastChannel.prototype = nativeCtor.prototype;
		Object.defineProperty(BroadcastChannel.prototype, 'constructor', {
			value: BroadcastChannel, writable: true, configurable: true
		});
		return BroadcastChannel;
	})(` + nativeCtorName + `)`)
	_ = r.vm.Set(nativeCtorName, goja.Undefined())
	if wrapErr != nil {
		return
	}
	publicCtor, ok := wrapperValue.(*goja.Object)
	if !ok {
		return
	}
	_ = r.vm.Set("BroadcastChannel", publicCtor)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("BroadcastChannel", publicCtor)
		}
	}
}

func runtimeBroadcastOpaqueKey(r *Runtime) string {
	if r != nil && r.workerParent != nil {
		if u := r.documentURL(); u != nil && strings.EqualFold(u.Scheme, "blob") {
			return runtimeBroadcastOpaqueKey(r.workerParent)
		}
	}
	return fmt.Sprintf("opaque-runtime:%p", r)
}

func broadcastEventInstance(r *Runtime, event *goja.Object) bool {
	if r == nil || event == nil {
		return false
	}
	if brand := event.Get("__goosieEvent"); brand != nil && brand.ToBoolean() {
		return true
	}
	if interfaceBrand := event.Get("__goosieEventInterface"); interfaceBrand != nil && !goja.IsUndefined(interfaceBrand) && !goja.IsNull(interfaceBrand) && interfaceBrand.String() != "" {
		return true
	}
	constructor, ok := r.vm.Get("Event").(*goja.Object)
	if !ok {
		return false
	}
	eventPrototype, ok := constructor.Get("prototype").(*goja.Object)
	if !ok {
		return false
	}
	for prototype := event.Prototype(); prototype != nil; prototype = prototype.Prototype() {
		if prototype == eventPrototype {
			return true
		}
	}
	return false
}

func dispatchBroadcastEventCall(r *Runtime, channel *bcObj, args []goja.Value) goja.Value {
	if len(args) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'dispatchEvent': 1 argument required"))
	}
	event, ok := args[0].(*goja.Object)
	if !ok || event.Get("type") == nil || goja.IsUndefined(event.Get("type")) || !broadcastEventInstance(r, event) {
		panic(r.vm.NewTypeError("parameter 1 is not of type 'Event'"))
	}
	brand := event.Get("__goosieEvent")
	interfaceBrand := event.Get("__goosieEventInterface")
	if (brand == nil || !brand.ToBoolean()) && (interfaceBrand == nil || goja.IsUndefined(interfaceBrand) || goja.IsNull(interfaceBrand)) {
		panic(r.vm.NewTypeError("parameter 1 is not of type 'Event'"))
	}
	dispatching := event.Get("__dispatching")
	initialized := event.Get("__initialized")
	if (dispatching != nil && dispatching.ToBoolean()) || (initialized != nil && !initialized.ToBoolean()) {
		panic(r.domError("InvalidStateError", "The event is already being dispatched or was not initialized"))
	}
	return r.vm.ToValue(dispatchBroadcastEvent(r, channel, event))
}

func runtimeBroadcastOrigin(r *Runtime) string {
	if r == nil {
		return "null"
	}
	u := r.documentURL()
	if (u == nil || strings.EqualFold(u.Scheme, "about")) && r.iframeParent != nil {
		return runtimeBroadcastOrigin(r.iframeParent)
	}
	if u == nil {
		return "null"
	}
	if strings.EqualFold(u.Scheme, "blob") {
		inner, err := url.Parse(strings.TrimPrefix(u.String(), u.Scheme+":"))
		if err != nil || inner == nil {
			return "null"
		}
		return serializedBroadcastOrigin(inner)
	}
	return serializedBroadcastOrigin(u)
}

func serializedBroadcastOrigin(u *url.URL) string {
	if u == nil {
		return "null"
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "null"
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "null"
	}
	host, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return "null"
	}
	host = strings.ToLower(host)
	port := u.Port()
	if port == "" || (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		if strings.Contains(host, ":") {
			return scheme + "://[" + host + "]"
		}
		return scheme + "://" + host
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

func runtimeBroadcastTopLevelSite(r *Runtime) string {
	if r == nil {
		return "opaque-top-level"
	}
	if r.workerParent != nil {
		return runtimeBroadcastTopLevelSite(r.workerParent)
	}
	root := r
	for root.iframeParent != nil {
		if root.iframeParent.iframeOpenedContexts[root.iframeParentFrame] {
			break
		}
		root = root.iframeParent
	}
	u := root.documentURL()
	if u == nil {
		return fmt.Sprintf("opaque-top-level:%p", root)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Sprintf("opaque-top-level:%p", root)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Sprintf("opaque-top-level:%p", root)
	}
	host, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return fmt.Sprintf("opaque-top-level:%p", root)
	}
	host = strings.ToLower(host)
	site, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		site = host
	}
	return scheme + "://" + site
}

func (r *Runtime) DrainBroadcastEvents() {
	state := broadcastStateOf(r)
	if state == nil {
		return
	}
	root := broadcastContextRoot(r)
	if root != r {
		if r.workerParent != nil {
			return
		}
		if parent := r.iframeParent; parent != nil && parent.iframeChildRuntimes[r.iframeParentFrame] == r {
			return
		}
	}
	state.hub.mu.Lock()
	pending := append([]broadcastDelivery(nil), state.hub.pending[root]...)
	delete(state.hub.pending, root)
	state.hub.mu.Unlock()
	for _, delivery := range pending {
		fireBCEvent(delivery.channel.r, delivery.channel, delivery.event)
	}
}

func fireBCEvent(r *Runtime, channel *bcObj, ev bcEvent) {
	if channel == nil {
		return
	}
	channel.mu.Lock()
	closed := channel.closed
	channel.mu.Unlock()
	if closed || channel.obj == nil || !broadcastContextActive(channel.r) {
		return
	}
	obj := channel.obj
	typ := "message"
	data, err := deserializeBroadcastMessage(r, []byte(ev.dataJSON))
	if err != nil {
		typ = "messageerror"
		data = goja.Null()
	}
	init := r.vm.NewObject()
	_ = init.Set("data", data)
	_ = init.Set("origin", ev.origin)
	eventObj, err := r.vm.New(r.vm.Get("MessageEvent"), r.vm.ToValue(typ), init)
	if err != nil {
		eventObj = r.vm.NewObject()
		_ = eventObj.Set("type", typ)
		_ = eventObj.Set("data", data)
		_ = eventObj.Set("origin", ev.origin)
	}
	_ = eventObj.Set("data", data)
	_ = eventObj.Set("origin", ev.origin)
	_ = eventObj.Set("target", obj)
	_ = eventObj.Set("currentTarget", obj)
	_ = eventObj.Set("isTrusted", true)
	_ = eventObj.Set("timeStamp", time.Now().UnixMilli())
	_ = eventObj.Set("preventDefault", func() {})
	_ = eventObj.Set("stopImmediatePropagation", func() { _ = eventObj.Set("__immediateStopped", true) })
	dispatchBroadcastEvent(r, channel, eventObj)
}

func dispatchBroadcastEvent(r *Runtime, channel *bcObj, eventObj *goja.Object) bool {
	if r == nil || channel == nil || channel.obj == nil || eventObj == nil {
		return true
	}
	obj := channel.obj
	typ := eventObj.Get("type").String()
	_ = eventObj.Set("__dispatching", true)
	_ = eventObj.Set("__immediateStopped", false)
	_ = eventObj.Set("target", obj)
	_ = eventObj.Set("currentTarget", obj)
	_ = eventObj.Set("eventPhase", 2)
	if trusted := eventObj.Get("isTrusted"); trusted == nil || goja.IsUndefined(trusted) {
		_ = eventObj.Set("isTrusted", false)
	}
	_ = eventObj.Set("__composedPath", r.vm.NewArray(obj))
	_ = eventObj.Set("composedPath", func(goja.FunctionCall) goja.Value {
		if !eventObj.Get("__dispatching").ToBoolean() {
			return r.vm.NewArray()
		}
		return r.vm.NewArray(obj)
	})
	defer func() {
		_ = eventObj.Set("__passiveListener", false)
		_ = eventObj.Set("currentTarget", goja.Null())
		_ = eventObj.Set("eventPhase", 0)
		_ = eventObj.Set("__dispatching", false)
		_ = eventObj.Set("__composedPath", r.vm.NewArray())
	}()

	channel.mu.Lock()
	listeners := append([]bcListener(nil), channel.listeners...)
	channel.mu.Unlock()
	for _, listener := range listeners {
		if eventObj.Get("__immediateStopped").ToBoolean() {
			break
		}
		if listener.typ != typ {
			continue
		}
		channel.mu.Lock()
		var active *bcListener
		for i := range channel.listeners {
			if channel.listeners[i].id == listener.id {
				current := channel.listeners[i]
				active = &current
				if current.once {
					channel.listeners = append(channel.listeners[:i], channel.listeners[i+1:]...)
					if current.handler {
						channel.handlers[current.typ] = goja.Null()
					}
				}
				break
			}
		}
		channel.mu.Unlock()
		if active == nil {
			continue
		}
		listener = *active
		_ = eventObj.Set("__passiveListener", listener.passive)
		if fn, ok := goja.AssertFunction(listener.callback); ok {
			_, _ = r.callBounded(func() (goja.Value, error) { return fn(obj, eventObj) })
			_ = eventObj.Set("__passiveListener", false)
			continue
		}
		if listener.callback == nil || goja.IsNull(listener.callback) || goja.IsUndefined(listener.callback) {
			_ = eventObj.Set("__passiveListener", false)
			continue
		}
		listenerObject, ok := listener.callback.(*goja.Object)
		if !ok {
			_ = eventObj.Set("__passiveListener", false)
			continue
		}
		handleEvent := listenerObject.Get("handleEvent")
		if fn, ok := goja.AssertFunction(handleEvent); ok {
			_, _ = r.callBounded(func() (goja.Value, error) { return fn(listener.callback, eventObj) })
		}
		_ = eventObj.Set("__passiveListener", false)
	}
	defaultPrevented := eventObj.Get("defaultPrevented")
	return defaultPrevented == nil || !defaultPrevented.ToBoolean()
}
