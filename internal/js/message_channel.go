package js

import (
	"strconv"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// messagePortEndpoint is one side of a channel. Transferring a port replaces
// its wrapper while preserving the endpoint and its entanglement.
type messagePortEndpoint struct {
	peer  *messagePortEndpoint
	owner *messagePort
}

type messagePortMessage struct {
	data                  goja.Value
	ports                 []*goja.Object
	includeUserActivation bool
}

type messagePortListener struct {
	callback goja.Value
	capture  bool
}

type messagePort struct {
	mu        sync.Mutex
	endpoint  *messagePortEndpoint
	obj       *goja.Object
	r         *Runtime
	started   bool
	closed    bool
	onmessage goja.Value
	queue     []messagePortMessage
	listeners []messagePortListener
}

type messageChannelState struct {
	mu    sync.Mutex
	ports []*messagePort
	proto *goja.Object
	ctor  *goja.Object
}

const messageChannelStateKey = "__messageChannelState__"

func (r *Runtime) setupMessageChannel() {
	state := &messageChannelState{proto: r.vm.NewObject()}
	_ = state.proto.DefineAccessorProperty("onmessage", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		port := messagePortFromObject(r.vm, call.This)
		if port == nil {
			return goja.Null()
		}
		port.mu.Lock()
		defer port.mu.Unlock()
		if port.onmessage == nil {
			return goja.Null()
		}
		return port.onmessage
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		port := messagePortFromObject(r.vm, call.This)
		if port == nil {
			return goja.Undefined()
		}
		value := call.Argument(0)
		port.mu.Lock()
		if goja.IsNull(value) || goja.IsUndefined(value) {
			port.onmessage = goja.Null()
		} else {
			port.onmessage = value
			if _, ok := goja.AssertFunction(value); ok {
				port.started = true
			}
		}
		port.mu.Unlock()
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.vm.Set(messageChannelStateKey, state)
	portCtor := r.vm.ToValue(func(goja.ConstructorCall) *goja.Object {
		panic(r.vm.NewTypeError("Illegal constructor"))
	})
	state.ctor, _ = portCtor.(*goja.Object)
	_ = state.ctor.Set("prototype", state.proto)
	_ = state.proto.Set("constructor", portCtor)
	_ = r.vm.Set("__goosieMessagePortPrototype__", state.proto)
	_, _ = r.vm.RunString(`Object.defineProperty(__goosieMessagePortPrototype__, Symbol.toStringTag, { value: "MessagePort", configurable: true });`)
	_ = r.vm.Set("__goosieMessagePortPrototype__", nil)
	ctor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) != 0 {
			panic(r.vm.NewTypeError("MessageChannel does not accept arguments"))
		}
		a, b := &messagePortEndpoint{}, &messagePortEndpoint{}
		a.peer, b.peer = b, a
		obj := call.This
		_ = obj.Set("port1", r.newMessagePort(state, a))
		_ = obj.Set("port2", r.newMessagePort(state, b))
		return nil
	})
	_ = state.proto.Set("constructor", ctor)
	_ = r.vm.Set("MessagePort", portCtor)
	_ = r.vm.Set("MessageChannel", ctor)
	if win, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = win.Set("MessageChannel", ctor)
		_ = win.Set("MessagePort", r.vm.Get("MessagePort"))
	}
}

func (r *Runtime) newMessagePort(state *messageChannelState, endpoint *messagePortEndpoint) *goja.Object {
	p := &messagePort{endpoint: endpoint, r: r}
	obj := r.vm.NewObject()
	obj.SetPrototype(state.proto)
	p.obj = obj
	endpoint.owner = p
	p.onmessage = goja.Null()
	_ = obj.Set("onmessageerror", goja.Null())
	_ = obj.Set("start", func(goja.FunctionCall) goja.Value {
		p.mu.Lock()
		p.started = true
		p.mu.Unlock()
		return goja.Undefined()
	})
	_ = obj.Set("close", func(goja.FunctionCall) goja.Value {
		p.mu.Lock()
		p.closed = true
		p.queue = nil
		p.mu.Unlock()
		return goja.Undefined()
	})
	_ = obj.Set("postMessage", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("postMessage requires a message"))
		}
		data := call.Arguments[0]
		transfer := goja.Value(goja.Undefined())
		includeUserActivation := false
		if len(call.Arguments) > 1 {
			transfer = call.Arguments[1]
			if options, ok := transfer.(*goja.Object); ok {
				value := options.Get("includeUserActivation")
				includeUserActivation = value != nil && !goja.IsUndefined(value) && value.ToBoolean()
			}
		}
		transfers := transferList(r.vm, transfer)
		// Validate the complete list before detaching any entry. The source port
		// itself cannot be transferred through its own postMessage call.
		validated := make(map[*goja.Object]bool, len(transfers))
		for _, value := range transfers {
			obj, isObject := value.(*goja.Object)
			if !isObject || validated[obj] {
				panic(r.dataCloneError("invalid or duplicate MessagePort transfer"))
			}
			validated[obj] = true
			port := messagePortFromObject(r.vm, value)
			if port == nil {
				if isArrayBufferObject(obj) {
					buffer, ok := obj.Export().(goja.ArrayBuffer)
					if ok && !buffer.Detached() {
						continue
					}
					panic(r.dataCloneError("ArrayBuffer is detached or invalid"))
				}
				if canvas := r.offscreenCanvases[obj]; canvas != nil && !canvas.detached {
					continue
				}
				if bitmap := r.imageBitmapObjects[obj]; bitmap != nil && !bitmap.closed {
					continue
				}
				panic(r.dataCloneError("object is not transferable"))
			}
			if port == p || port.r != r {
				panic(r.dataCloneError("invalid MessagePort transfer"))
			}
			port.mu.Lock()
			closed := port.closed
			port.mu.Unlock()
			if closed {
				panic(r.dataCloneError("MessagePort is already closed"))
			}
		}
		// Structured serialization must succeed before transferables are detached.
		// Otherwise a DataCloneError could leave a port closed even though no
		// message was posted. The direct ArrayBuffer transfer case is substituted
		// below and does not need to be cloned here.
		p.mu.Lock()
		preflightEndpoint := p.endpoint
		preflightClosed := p.closed
		p.mu.Unlock()
		var preflightPeer *messagePort
		if !preflightClosed && preflightEndpoint != nil && preflightEndpoint.peer != nil {
			preflightPeer = preflightEndpoint.peer.owner
		}
		var precloned goja.Value
		preclonedReady := false
		stagedTransfers := make(map[*goja.Object]goja.Value, len(transfers))
		if preflightPeer != nil {
			for _, transferValue := range transfers {
				transferObject, ok := transferValue.(*goja.Object)
				if !ok {
					continue
				}
				if isArrayBufferObject(transferObject) {
					buffer, ok := transferObject.Export().(goja.ArrayBuffer)
					if !ok || buffer.Detached() {
						panic(r.dataCloneError("ArrayBuffer is detached or invalid"))
					}
					clonedBuffer, err := cloneArrayBufferToRuntime(r, preflightPeer.r, buffer)
					if err != nil {
						panic(r.dataCloneError(err.Error()))
					}
					stagedTransfers[transferObject] = clonedBuffer
					continue
				}
				if messagePortFromObject(r.vm, transferObject) != nil {
					stateValue := preflightPeer.r.vm.Get(messageChannelStateKey)
					if stateValue == nil || goja.IsUndefined(stateValue) || goja.IsNull(stateValue) {
						panic(r.dataCloneError("target does not support MessagePort"))
					}
					state, ok := stateValue.Export().(*messageChannelState)
					if !ok || state == nil {
						panic(r.dataCloneError("target does not support MessagePort"))
					}
					stagedTransfers[transferObject] = preflightPeer.r.newMessagePort(state, &messagePortEndpoint{})
				}
			}
		}
		if preflightPeer != nil {
			cloned, err := cloneMessageValueWithTransfers(r, preflightPeer.r, data, stagedTransfers)
			if err != nil {
				panic(r.dataCloneError(err.Error()))
			}
			precloned = cloned
			preclonedReady = true
		}
		moved := make([]*goja.Object, 0, len(transfers))
		seen := map[*goja.Object]bool{}
		for _, value := range transfers {
			port := messagePortFromObject(r.vm, value)
			obj, isObject := value.(*goja.Object)
			if !isObject || seen[obj] {
				panic(r.dataCloneError("invalid or duplicate MessagePort transfer"))
			}
			seen[obj] = true
			if port == nil {
				if isArrayBufferObject(obj) {
					buffer, ok := obj.Export().(goja.ArrayBuffer)
					if !ok || buffer.Detached() {
						panic(r.dataCloneError("ArrayBuffer is detached or invalid"))
					}
					if !buffer.Detach() {
						panic(r.dataCloneError("ArrayBuffer is detached or invalid"))
					}
					continue
				}
				if canvas := r.offscreenCanvases[obj]; canvas != nil && !canvas.detached {
					canvas.detached = true
					canvas.context = nil
					canvas.contextObject = nil
					continue
				}
				if bitmap := r.imageBitmapObjects[obj]; bitmap != nil && !bitmap.closed {
					bitmap.closed = true
					bitmap.pixels = nil
					bitmap.width, bitmap.height = 0, 0
					continue
				}
				panic(r.dataCloneError("object is not transferable"))
			}
			if port.r != r {
				panic(r.dataCloneError("MessagePort belongs to another runtime"))
			}
			port.mu.Lock()
			if port.closed {
				port.mu.Unlock()
				panic(r.dataCloneError("MessagePort is already closed"))
			}
			endpoint := port.endpoint
			queue := append([]messagePortMessage(nil), port.queue...)
			started := port.started
			port.closed = true
			port.queue = nil
			port.mu.Unlock()
			var movedPort *goja.Object
			if staged, ok := stagedTransfers[obj].(*goja.Object); ok {
				movedPort = staged
				newPort := messagePortFromObject(preflightPeer.r.vm, movedPort)
				if newPort != nil {
					newPort.endpoint = endpoint
					newPort.queue = queue
					newPort.started = started
					endpoint.owner = newPort
				}
			} else {
				movedPort = r.newMessagePort(state, endpoint)
			}
			moved = append(moved, movedPort)
		}
		p.mu.Lock()
		closed := p.closed
		endpoint := p.endpoint
		p.mu.Unlock()
		if closed || endpoint == nil || endpoint.peer == nil {
			return goja.Undefined()
		}
		peer := endpoint.peer.owner
		if peer == nil {
			return goja.Undefined()
		}
		if preclonedReady {
			data = precloned
		} else {
			// postMessage snapshots its payload when called, even when both ports
			// live in the same runtime. Never queue the sender's object directly:
			// later mutations must not affect the receiver's message data.
			cloned, err := cloneMessageValue(r, peer.r, data)
			if err != nil {
				panic(r.dataCloneError(err.Error()))
			}
			data = cloned
		}
		peer.mu.Lock()
		if !peer.closed {
			peer.queue = append(peer.queue, messagePortMessage{data: data, ports: moved, includeUserActivation: includeUserActivation})
		}
		peer.mu.Unlock()
		return goja.Undefined()
	})
	_ = obj.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 || call.Arguments[0].String() != "message" || goja.IsNull(call.Argument(1)) || goja.IsUndefined(call.Argument(1)) {
			return goja.Undefined()
		}
		capture := false
		if len(call.Arguments) > 2 && !goja.IsNull(call.Argument(2)) && !goja.IsUndefined(call.Argument(2)) {
			if options, ok := call.Argument(2).Export().(bool); ok {
				capture = options
			} else if options, ok := call.Argument(2).(*goja.Object); ok {
				capture = options.Get("capture").ToBoolean()
			}
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, listener := range p.listeners {
			if listener.capture == capture && listener.callback.StrictEquals(call.Argument(1)) {
				return goja.Undefined()
			}
		}
		p.listeners = append(p.listeners, messagePortListener{callback: call.Argument(1), capture: capture})
		return goja.Undefined()
	})
	_ = obj.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 || call.Arguments[0].String() != "message" {
			return goja.Undefined()
		}
		capture := false
		if len(call.Arguments) > 2 && !goja.IsNull(call.Argument(2)) && !goja.IsUndefined(call.Argument(2)) {
			if options, ok := call.Argument(2).Export().(bool); ok {
				capture = options
			} else if options, ok := call.Argument(2).(*goja.Object); ok {
				capture = options.Get("capture").ToBoolean()
			}
		}
		p.mu.Lock()
		for i, listener := range p.listeners {
			if listener.capture == capture && listener.callback.StrictEquals(call.Argument(1)) {
				p.listeners = append(p.listeners[:i], p.listeners[i+1:]...)
				break
			}
		}
		p.mu.Unlock()
		return goja.Undefined()
	})
	_ = obj.Set("toString", func(goja.FunctionCall) goja.Value { return r.vm.ToValue("[object MessagePort]") })
	_ = obj.Set("__goosieMessagePort", true)
	state.mu.Lock()
	state.ports = append(state.ports, p)
	state.mu.Unlock()
	return obj
}

func isArrayBufferObject(obj *goja.Object) bool {
	if obj == nil {
		return false
	}
	buffer, ok := obj.Export().(goja.ArrayBuffer)
	return ok && !buffer.Shared()
}

func transferList(vm *goja.Runtime, value goja.Value) []goja.Value {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	if obj, ok := value.(*goja.Object); ok {
		if transfer := obj.Get("transfer"); transfer != nil && !goja.IsUndefined(transfer) {
			value = transfer
		}
	}
	obj := value.ToObject(vm)
	length := obj.Get("length")
	if length == nil || goja.IsUndefined(length) {
		return nil
	}
	n := int(length.ToInteger())
	if n < 0 || n > 10000 {
		panic(vm.NewTypeError("invalid transfer list"))
	}
	items := make([]goja.Value, n)
	for i := range items {
		items[i] = obj.Get(strconv.Itoa(i))
	}
	return items
}

func messagePortFromObject(vm *goja.Runtime, value goja.Value) *messagePort {
	obj, ok := value.(*goja.Object)
	if !ok || obj.Get("__goosieMessagePort") == nil {
		return nil
	}
	state, _ := vm.Get(messageChannelStateKey).Export().(*messageChannelState)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, p := range state.ports {
		if p.obj == obj {
			return p
		}
	}
	return nil
}

// DrainMessagePortEvents dispatches queued messages on the runtime thread.
func (r *Runtime) DrainMessagePortEvents() {
	state, _ := r.vm.Get(messageChannelStateKey).Export().(*messageChannelState)
	if state == nil {
		return
	}
	state.mu.Lock()
	ports := append([]*messagePort(nil), state.ports...)
	state.mu.Unlock()
	for _, p := range ports {
		p.mu.Lock()
		if !p.started && p.onmessage != nil && !goja.IsNull(p.onmessage) && !goja.IsUndefined(p.onmessage) {
			p.started = true
		}
		if !p.started || p.closed {
			p.mu.Unlock()
			continue
		}
		messages := append([]messagePortMessage(nil), p.queue...)
		p.queue = nil
		handlers := append([]messagePortListener(nil), p.listeners...)
		p.mu.Unlock()
		for _, msg := range messages {
			portsArray := r.vm.NewArray()
			for i, port := range msg.ports {
				_ = portsArray.Set(strconv.Itoa(i), port)
			}
			init := r.vm.NewObject()
			_ = init.Set("data", msg.data)
			_ = init.Set("origin", "")
			_ = init.Set("lastEventId", "")
			_ = init.Set("ports", portsArray)
			event, err := r.vm.New(r.vm.Get("MessageEvent"), r.vm.ToValue("message"), init)
			if err != nil || event == nil {
				// Keep port delivery working if page code replaces MessageEvent.
				event = r.vm.NewObject()
				_ = event.Set("type", "message")
				_ = event.Set("data", msg.data)
				_ = event.Set("origin", "")
				_ = event.Set("lastEventId", "")
				_ = event.Set("source", goja.Null())
				_ = event.Set("ports", portsArray)
			}
			_ = event.Set("data", msg.data)
			_ = event.Set("isTrusted", true)
			_ = event.Set("target", p.obj)
			_ = event.Set("currentTarget", p.obj)
			_ = event.Set("eventPhase", 2)
			_ = event.Set("timeStamp", time.Now().UnixMilli())
			_ = event.Set("__dispatching", true)
			_ = event.Set("__immediateStopped", false)
			_ = event.Set("__composedPath", r.vm.NewArray(p.obj))
			if msg.includeUserActivation {
				activation := r.vm.NewObject()
				_ = activation.Set("isActive", false)
				_ = activation.Set("hasBeenActive", false)
				_ = event.Set("userActivation", activation)
			} else {
				_ = event.Set("userActivation", goja.Null())
			}
			for _, listener := range handlers {
				if event.Get("__immediateStopped").ToBoolean() {
					break
				}
				if fn, ok := goja.AssertFunction(listener.callback); ok {
					_, _ = r.callBounded(func() (goja.Value, error) { return fn(p.obj, event) })
				} else if object, ok := listener.callback.(*goja.Object); ok {
					if fn, ok := goja.AssertFunction(object.Get("handleEvent")); ok {
						_, _ = r.callBounded(func() (goja.Value, error) { return fn(object, event) })
					}
				}
			}
			if !event.Get("__immediateStopped").ToBoolean() {
				if fn, ok := goja.AssertFunction(p.onmessage); ok {
					_, _ = r.callBounded(func() (goja.Value, error) { return fn(p.obj, event) })
				}
			}
			_ = event.Set("currentTarget", goja.Null())
			_ = event.Set("eventPhase", 0)
			_ = event.Set("__dispatching", false)
			_ = event.Set("__composedPath", r.vm.NewArray())
		}
	}
}
