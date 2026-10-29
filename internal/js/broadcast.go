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

// BroadcastChannel API implementation.
//
// A BroadcastHub is the process-global registry that connects channels with
// the same name and origin across browsing contexts (tabs, windows, iframes).
// Each participating Runtime receives the hub through Options.BroadcastHub;
// otherwise a package-level default is used.

// bcEvent is one pending inbound message queued by a peer's postMessage and
// waiting for DrainBroadcastEvents to fire onmessage on the VM thread.
type bcEvent struct {
	dataJSON string // bounded graph encoding of the structured-clone subset
	origin   string
}

type broadcastDelivery struct {
	channel *bcObj
	event   bcEvent
}

// bcObj holds the internal state of one BroadcastChannel instance.
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

// BroadcastHub connects BroadcastChannel instances that share a name. It is
// safe for concurrent use: postMessage from any goroutine appends to peer
// event queues, and DrainBroadcastEvents on the VM thread reads them.
type BroadcastHub struct {
	mu            sync.Mutex
	channels      map[string][]*bcObj              // keyed by storage key and channel name
	pending       map[*Runtime][]broadcastDelivery // queued by top-level browsing context
	sharedWorkers map[string]*sharedWorkerEntry
}

// defaultHub is the package-level fallback used when Options.BroadcastHub is
// nil. A single process-wide hub means channels created by different runtimes
// (different tabs) with no explicit hub still reach each other, matching the
// browser contract that BroadcastChannel is origin-scoped, not runtime-scoped.
var defaultHub = &BroadcastHub{
	channels:      make(map[string][]*bcObj),
	pending:       make(map[*Runtime][]broadcastDelivery),
	sharedWorkers: make(map[string]*sharedWorkerEntry),
}

// NewBroadcastHub creates a new isolated hub. Hosts that want to scope
// cross-tab messaging to a subset of runtimes (e.g. per origin) pass one of
// these through Options.BroadcastHub; the rest use the package default.
func NewBroadcastHub() *BroadcastHub {
	return &BroadcastHub{
		channels:      make(map[string][]*bcObj),
		pending:       make(map[*Runtime][]broadcastDelivery),
		sharedWorkers: make(map[string]*sharedWorkerEntry),
	}
}

// register adds a channel to its origin and name group.
func (h *BroadcastHub) register(ch *bcObj) {
	h.mu.Lock()
	h.channels[ch.key] = append(h.channels[ch.key], ch)
	h.mu.Unlock()
}

// unregister removes a channel from the hub's name group.
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

// broadcast queues a message for each open peer except the sender. Deliveries
// within one top-level browsing context are appended under one hub lock so
// iframes observe recipient creation order despite having separate Runtimes.
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

const broadcastStateKey = "__broadcastState__"

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
			// window.open contexts are top-level browsing contexts even though
			// this engine hosts them in an iframe-shaped container. Their
			// BroadcastChannel task queue must be pumped and grouped separately
			// from the opener's context.
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
	_ = r.vm.Set(broadcastStateKey, s)
}

func broadcastStateOf(r *Runtime) *broadcastState {
	v := r.vm.Get(broadcastStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	s, ok := v.Export().(*broadcastState)
	if !ok {
		return nil
	}
	return s
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

// setupBroadcastChannel installs the BroadcastChannel constructor on the
// global scope and window object.
func (r *Runtime) setupBroadcastChannel() {
	// Detached and worker-like runtimes may not install the DOM prototype
	// chain, but BroadcastChannel still implements EventTarget in those realms.
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

	// BroadcastChannel constructor.
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
			// Opaque origins never match each other, including two documents
			// that both serialize to "null". A blob worker inherits its
			// creator's opaque origin, though, so it must share the creator's
			// private key rather than using its own Runtime identity.
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

		// Register with the hub and the runtime-local list.
		hub.register(ch)
		state.mu.Lock()
		state.byObject[obj] = ch
		state.mu.Unlock()

		return nil
	}

	ctor := r.vm.ToValue(constructor)
	if constructorObject, ok := ctor.(*goja.Object); ok {
		if prototype, ok := constructorObject.Get("prototype").(*goja.Object); ok {
			// BroadcastChannel inherits the EventTarget interface operations and
			// brand chain. Its listener storage remains channel-specific because
			// inbound messages use the channel's task-queue dispatch path.
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
	// A native Goja constructor can also be called as a function and does not
	// expose that distinction through ConstructorCall.NewTarget. Wrap it in a
	// JavaScript constructor so the Web API's required-new behavior is enforced,
	// while Reflect.construct preserves subclass/newTarget prototype semantics.
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

	// Also install on window.
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

// runtimeBroadcastOrigin returns the origin serialization used by
// MessageEvent.origin. Initial about:blank and srcdoc runtimes inherit their
// creator's origin; URLs with opaque or unsupported origins serialize as
// "null".
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

// runtimeBroadcastTopLevelSite returns the schemeful site of the top-level
// browsing context. BroadcastChannel is keyed by the environment settings
// object's storage key, which includes this site for third-party contexts.
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

// DrainBroadcastEvents drains pending BroadcastChannel messages and fires
// onmessage on the VM thread. Call this from Tick.
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

// fireBCEvent invokes the onmessage handler for one inbound BroadcastChannel
// message. The structured-clone graph is rebuilt in the destination realm.
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
		// Keep delivery alive if a host has replaced or damaged the constructor;
		// the normal runtime path always creates a branded MessageEvent above.
		eventObj = r.vm.NewObject()
		_ = eventObj.Set("type", typ)
		_ = eventObj.Set("data", data)
		_ = eventObj.Set("origin", ev.origin)
	}
	// MessageEvent's constructor defaults its public `data` field to null when
	// init.data is undefined, while a BroadcastChannel message may legitimately
	// carry undefined. Preserve the value delivered by structured clone exactly.
	_ = eventObj.Set("data", data)
	_ = eventObj.Set("origin", ev.origin)
	_ = eventObj.Set("target", obj)
	_ = eventObj.Set("currentTarget", obj)
	_ = eventObj.Set("isTrusted", true)
	_ = eventObj.Set("timeStamp", time.Now().UnixMilli())
	_ = eventObj.Set("preventDefault", func() {}) // BroadcastChannel messages are not cancelable.
	_ = eventObj.Set("stopImmediatePropagation", func() { _ = eventObj.Set("__immediateStopped", true) })
	dispatchBroadcastEvent(r, channel, eventObj)
}

// dispatchBroadcastEvent runs the at-target portion of EventTarget dispatch.
// BroadcastChannel has no ancestors, so there are no capture or bubble phases
// beyond invoking its listener list in registration order.
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
	_ = eventObj.Set("eventPhase", 2) // AT_TARGET while listeners run.
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
		// The snapshot fixes which registrations may run; checking the live
		// listener list lets an earlier callback remove a later one. Reading the
		// current callback also observes an onmessage replacement made mid-turn.
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
