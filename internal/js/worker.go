package js

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// Web Workers API implementation.

// WorkerTask represents a unit of work executed in a worker.
type WorkerTask func(data any) any

// WorkerRunner executes worker scripts. The engine provides the concrete
// implementation; internal/js defines the interface so it does not import
// platform-specific code.
type WorkerRunner interface {
	Run(scriptURL string, onData func(any), onError func(error)) error
	PostMessage(data any) error
	Terminate() error
}

// WorkerDialer creates worker runners. The engine wires the concrete dialer
// at pipeline assembly time.
type WorkerDialer interface {
	CreateWorker(scriptURL string) (WorkerRunner, error)
}

type workerTaskDrainer interface {
	DrainTasks()
}

// workerObj holds the internal state of one Worker object.
type workerObj struct {
	mu         sync.Mutex
	runner     WorkerRunner
	url        string
	terminated bool
	pending    []any
	errors     []string
	listeners  []workerEventListener
	obj        *goja.Object
	r          *Runtime
}

type workerEventListener struct {
	typ      string
	callback goja.Value
}

// workerMessage is a pending message to be drained.
type workerMessage struct {
	worker *workerObj
	data   any
	isErr  bool
	errMsg string
}

const workerStateKey = "__workerState__"

type workerState struct {
	mu          sync.Mutex
	dialer      WorkerDialer
	workers     []*workerObj
	shared      map[string]*sharedWorkerEntry
	sharedOrder []*localSharedWorkerRunner
	messages    []workerMessage
	closed      bool
}

type sharedWorkerEntry struct {
	ready  chan struct{}
	runner *localSharedWorkerRunner
	err    error
	refs   int
}

const maxWorkersPerRuntime = 32

func setWorkerState(r *Runtime, s *workerState) {
	_ = r.vm.Set(workerStateKey, s)
}

func workerStateOf(r *Runtime) *workerState {
	v := r.vm.Get(workerStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	s, ok := v.Export().(*workerState)
	if !ok {
		return nil
	}
	return s
}

// setupWorker installs the Worker constructor on the global scope.
func (r *Runtime) setupWorker(dialer WorkerDialer) {
	if dialer == nil {
		dialer = &localWorkerDialer{runtime: r}
	}
	state := &workerState{
		dialer: dialer,
		shared: make(map[string]*sharedWorkerEntry),
	}
	setWorkerState(r, state)

	constructor := func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("Worker requires a script URL argument"))
		}

		scriptURL := call.Arguments[0].String()

		w := &workerObj{
			url: scriptURL,
			r:   r,
		}

		obj := call.This
		w.obj = obj

		// Event handlers (initially null).
		_ = obj.Set("onmessage", goja.Null())
		_ = obj.Set("onerror", goja.Null())
		_ = obj.Set("onmessageerror", goja.Null())
		_ = obj.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 || goja.IsNull(call.Argument(1)) || goja.IsUndefined(call.Argument(1)) {
				return goja.Undefined()
			}
			listener := call.Argument(1)
			if _, ok := goja.AssertFunction(listener); !ok {
				if object, ok := listener.(*goja.Object); !ok || object.Get("handleEvent") == nil {
					return goja.Undefined()
				}
			}
			typ := call.Argument(0).String()
			w.mu.Lock()
			for _, current := range w.listeners {
				if current.typ == typ && current.callback.StrictEquals(listener) {
					w.mu.Unlock()
					return goja.Undefined()
				}
			}
			w.listeners = append(w.listeners, workerEventListener{typ: typ, callback: listener})
			w.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			typ, listener := call.Argument(0).String(), call.Argument(1)
			w.mu.Lock()
			for i, current := range w.listeners {
				if current.typ == typ && current.callback.StrictEquals(listener) {
					w.listeners = append(w.listeners[:i], w.listeners[i+1:]...)
					break
				}
			}
			w.mu.Unlock()
			return goja.Undefined()
		})

		// postMessage(data) method.
		_ = obj.Set("postMessage", func(call goja.FunctionCall) goja.Value {
			w.mu.Lock()
			if w.terminated {
				w.mu.Unlock()
				return goja.Undefined()
			}
			runner := w.runner
			if runner == nil {
				var data any
				if len(call.Arguments) != 0 {
					data = call.Arguments[0].Export()
				}
				w.pending = append(w.pending, data)
				w.mu.Unlock()
				return goja.Undefined()
			}
			w.mu.Unlock()

			var data any
			if len(call.Arguments) >= 1 {
				data = call.Arguments[0].Export()
			}

			if err := runner.PostMessage(data); err != nil {
				w.mu.Lock()
				w.errors = append(w.errors, err.Error())
				w.mu.Unlock()
			}

			return goja.Undefined()
		})

		// terminate() method.
		_ = obj.Set("terminate", func(call goja.FunctionCall) goja.Value {
			w.mu.Lock()
			if w.terminated {
				w.mu.Unlock()
				return goja.Undefined()
			}
			w.terminated = true
			runner := w.runner
			w.mu.Unlock()

			if runner != nil {
				_ = runner.Terminate()
			}

			return goja.Undefined()
		})

		// Start the worker asynchronously.
		state.mu.Lock()
		if state.closed || len(state.workers) >= maxWorkersPerRuntime {
			state.mu.Unlock()
			panic(r.vm.NewGoError(errors.New("Worker limit exceeded")))
		}
		state.workers = append(state.workers, w)
		state.mu.Unlock()

		go func() {
			if state.dialer == nil {
				state.mu.Lock()
				state.messages = append(state.messages, workerMessage{
					worker: w,
					isErr:  true,
					errMsg: "no worker dialer",
				})
				state.mu.Unlock()
				return
			}

			runner, err := state.dialer.CreateWorker(scriptURL)
			if err != nil {
				state.mu.Lock()
				state.messages = append(state.messages, workerMessage{
					worker: w,
					isErr:  true,
					errMsg: err.Error(),
				})
				state.mu.Unlock()
				return
			}
			state.mu.Lock()
			closed := state.closed
			state.mu.Unlock()
			w.mu.Lock()
			terminated := w.terminated
			w.mu.Unlock()
			if closed || terminated {
				_ = runner.Terminate()
				return
			}

			w.mu.Lock()
			w.runner = runner
			pending := append([]any(nil), w.pending...)
			w.pending = nil
			w.mu.Unlock()
			for _, data := range pending {
				if err := runner.PostMessage(data); err != nil {
					state.mu.Lock()
					state.messages = append(state.messages, workerMessage{worker: w, isErr: true, errMsg: err.Error()})
					state.mu.Unlock()
				}
			}

			// Set up message callbacks.
			if err := runner.Run(scriptURL,
				func(data any) {
					state.mu.Lock()
					state.messages = append(state.messages, workerMessage{
						worker: w,
						data:   data,
					})
					state.mu.Unlock()
				},
				func(err error) {
					state.mu.Lock()
					state.messages = append(state.messages, workerMessage{
						worker: w,
						isErr:  true,
						errMsg: err.Error(),
					})
					state.mu.Unlock()
				},
			); err != nil {
				state.mu.Lock()
				state.messages = append(state.messages, workerMessage{worker: w, isErr: true, errMsg: err.Error()})
				state.mu.Unlock()
			}
		}()

		return nil
	}

	ctor := r.vm.ToValue(constructor)
	_ = r.vm.Set("Worker", ctor)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("Worker", ctor)
		}
	}
	r.setupSharedWorker(state)
}

type localSharedWorkerRunner struct {
	mu     sync.Mutex
	key    string
	url    string
	runner *localWorkerRunner
	closed bool
}

func (r *Runtime) setupSharedWorker(state *workerState) {
	constructor := func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("SharedWorker requires a script URL argument"))
		}
		scriptURL := r.resolveURLString(call.Arguments[0].String())
		name := ""
		if len(call.Arguments) > 1 && !goja.IsNull(call.Arguments[1]) && !goja.IsUndefined(call.Arguments[1]) {
			if options, ok := call.Arguments[1].(*goja.Object); ok {
				nameValue := options.Get("name")
				if nameValue != nil && !goja.IsUndefined(nameValue) {
					name = nameValue.String()
				}
			} else {
				name = call.Arguments[1].String()
			}
		}
		origin := runtimeBroadcastOrigin(r)
		if origin == "null" {
			origin = runtimeBroadcastOpaqueKey(r)
		}
		key := runtimeBroadcastTopLevelSite(r) + "\x00" + origin + "\x00" + scriptURL + "\x00" + name

		state.mu.Lock()
		if state.closed {
			state.mu.Unlock()
			panic(r.vm.NewGoError(errors.New("SharedWorker runtime is closed")))
		}
		entry := state.shared[key]
		state.mu.Unlock()
		if entry == nil {
			entry = r.acquireSharedWorker(state, key, scriptURL)
			state.mu.Lock()
			if state.closed {
				state.mu.Unlock()
				r.releaseSharedWorker(entry)
				panic(r.vm.NewGoError(errors.New("SharedWorker runtime is closed")))
			}
			state.shared[key] = entry
			state.sharedOrder = append(state.sharedOrder, entry.runner)
			state.mu.Unlock()
		}

		port := r.newSharedWorkerPort(entry.runner)
		obj := call.This
		_ = obj.Set("port", port)
		_ = obj.Set("onerror", goja.Null())
		return nil
	}
	ctor := r.vm.ToValue(constructor)
	_ = r.vm.Set("SharedWorker", ctor)
	if win, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = win.Set("SharedWorker", ctor)
	}
}

func (r *Runtime) acquireSharedWorker(state *workerState, key, scriptURL string) *sharedWorkerEntry {
	hub := defaultHub
	if broadcastState := broadcastStateOf(r); broadcastState != nil && broadcastState.hub != nil {
		hub = broadcastState.hub
	}
	hub.mu.Lock()
	entry := hub.sharedWorkers[key]
	creator := entry == nil
	if creator {
		entry = &sharedWorkerEntry{ready: make(chan struct{}), refs: 1}
		hub.sharedWorkers[key] = entry
	} else {
		entry.refs++
	}
	hub.mu.Unlock()

	if creator {
		localDialer, ok := state.dialer.(*localWorkerDialer)
		if !ok || localDialer == nil {
			entry.err = errors.New("SharedWorker is unsupported by this worker backend")
		} else {
			shared := &localSharedWorkerRunner{key: key, url: scriptURL, runner: &localWorkerRunner{parent: r, scriptURL: scriptURL}}
			var startErr error
			if err := shared.runner.Run(scriptURL, nil, func(err error) { startErr = err }); err != nil {
				startErr = err
			}
			if startErr != nil {
				_ = shared.runner.Terminate()
				entry.err = startErr
			} else {
				entry.runner = shared
			}
		}
		hub.mu.Lock()
		if entry.err != nil && hub.sharedWorkers[key] == entry {
			delete(hub.sharedWorkers, key)
		}
		close(entry.ready)
		hub.mu.Unlock()
	} else {
		<-entry.ready
	}
	if entry.err != nil {
		r.releaseSharedWorker(entry)
		panic(r.vm.NewGoError(entry.err))
	}
	return entry
}

func (r *Runtime) releaseSharedWorker(entry *sharedWorkerEntry) {
	if entry == nil {
		return
	}
	hub := defaultHub
	if broadcastState := broadcastStateOf(r); broadcastState != nil && broadcastState.hub != nil {
		hub = broadcastState.hub
	}
	hub.mu.Lock()
	if entry.refs > 0 {
		entry.refs--
	}
	last := entry.refs == 0
	if last {
		for key, current := range hub.sharedWorkers {
			if current == entry {
				delete(hub.sharedWorkers, key)
				break
			}
		}
	}
	hub.mu.Unlock()
	if last && entry.runner != nil {
		_ = entry.runner.Terminate()
	}
}

func (r *Runtime) newSharedWorkerPort(shared *localSharedWorkerRunner) *goja.Object {
	channelStateValue := r.vm.Get(messageChannelStateKey)
	if channelStateValue == nil || goja.IsUndefined(channelStateValue) {
		panic(r.vm.NewTypeError("SharedWorker requires MessagePort support"))
	}
	channelState, ok := channelStateValue.Export().(*messageChannelState)
	if !ok || channelState == nil {
		panic(r.vm.NewTypeError("SharedWorker requires MessagePort support"))
	}
	pageEndpoint, workerEndpoint := &messagePortEndpoint{}, &messagePortEndpoint{}
	pageEndpoint.peer, workerEndpoint.peer = workerEndpoint, pageEndpoint
	pagePort := r.newMessagePort(channelState, pageEndpoint)
	workerPort := r.newMessagePort(channelState, workerEndpoint)
	workerPorts, err := r.transferPortsTo(shared.runner.runtime, r.vm.ToValue(r.vm.NewArray(workerPort)))
	if err != nil || len(workerPorts) != 1 {
		panic(r.vm.NewGoError(errors.New("failed to connect SharedWorker MessagePort")))
	}
	shared.mu.Lock()
	closed := shared.closed
	runner := shared.runner
	shared.mu.Unlock()
	if closed || runner == nil {
		panic(r.vm.NewGoError(errors.New("SharedWorker is closed")))
	}
	if err := runner.connect(workerPorts[0].(*goja.Object)); err != nil {
		panic(r.vm.NewGoError(err))
	}
	return pagePort
}

// DrainWorkerMessages drains pending worker messages and fires them on the
// main goroutine. Call this from Tick.
func (r *Runtime) DrainWorkerMessages() {
	state := workerStateOf(r)
	if state == nil {
		return
	}

	state.mu.Lock()
	workers := append([]*workerObj(nil), state.workers...)
	sharedWorkers := append([]*localSharedWorkerRunner(nil), state.sharedOrder...)
	state.mu.Unlock()
	for _, worker := range workers {
		worker.mu.Lock()
		runner := worker.runner
		worker.mu.Unlock()
		if drainer, ok := runner.(workerTaskDrainer); ok {
			drainer.DrainTasks()
		}
	}
	for _, shared := range sharedWorkers {
		shared.mu.Lock()
		runner := shared.runner
		active := !shared.closed
		shared.mu.Unlock()
		if active && runner != nil {
			runner.DrainTasks()
		}
	}

	state.mu.Lock()
	msgs := make([]workerMessage, len(state.messages))
	copy(msgs, state.messages)
	state.messages = state.messages[:0]
	state.mu.Unlock()

	for _, msg := range msgs {
		w := msg.worker
		w.mu.Lock()
		obj := w.obj
		terminated := w.terminated
		w.mu.Unlock()

		if obj == nil || terminated {
			continue
		}

		if msg.isErr {
			handler := obj.Get("onerror")
			errObj := r.vm.NewObject()
			_ = errObj.Set("message", msg.errMsg)
			_ = errObj.Set("type", "error")
			dispatchWorkerEvent(r, w, obj, "error", errObj, handler)
		} else {
			handler := obj.Get("onmessage")
			eventObj := r.vm.NewObject()
			_ = eventObj.Set("data", r.vm.ToValue(msg.data))
			_ = eventObj.Set("type", "message")
			_ = eventObj.Set("target", obj)
			_ = eventObj.Set("currentTarget", obj)
			dispatchWorkerEvent(r, w, obj, "message", eventObj, handler)
		}
	}
}

func dispatchWorkerEvent(r *Runtime, worker *workerObj, target *goja.Object, typ string, event *goja.Object, handler goja.Value) {
	worker.mu.Lock()
	listeners := append([]workerEventListener(nil), worker.listeners...)
	worker.mu.Unlock()
	invoke := func(callback goja.Value) {
		if fn, ok := goja.AssertFunction(callback); ok {
			_, _ = r.callBounded(func() (goja.Value, error) { return fn(target, event) })
		} else if object, ok := callback.(*goja.Object); ok {
			if fn, ok := goja.AssertFunction(object.Get("handleEvent")); ok {
				_, _ = r.callBounded(func() (goja.Value, error) { return fn(object, event) })
			}
		}
	}
	for _, listener := range listeners {
		if listener.typ == typ {
			invoke(listener.callback)
		}
	}
	if handler != nil && !goja.IsNull(handler) && !goja.IsUndefined(handler) {
		invoke(handler)
	}
}

type localWorkerDialer struct {
	runtime *Runtime
}

func (d *localWorkerDialer) CreateWorker(scriptURL string) (WorkerRunner, error) {
	if d == nil || d.runtime == nil {
		return nil, errors.New("worker runtime is unavailable")
	}
	return &localWorkerRunner{parent: d.runtime, scriptURL: scriptURL}, nil
}

type localWorkerRunner struct {
	parent     *Runtime
	scriptURL  string
	mu         sync.Mutex
	runMu      sync.Mutex
	runtime    *Runtime
	onData     func(any)
	onError    func(error)
	pending    []any
	terminated bool
	closing    bool
	ready      bool
}

func (w *localWorkerRunner) Run(scriptURL string, onData func(any), onError func(error)) error {
	resolvedURL := w.parent.resolveURLString(scriptURL)
	sourceBytes, ok := w.parent.objectURLSource(resolvedURL)
	if !ok {
		if decoded, isDataURL, err := decodeDataWorkerURL(resolvedURL); isDataURL {
			if err != nil {
				return err
			}
			sourceBytes = []byte(decoded)
		} else if w.parent.opts.WorkerScriptFetcher == nil {
			return errors.New("worker script URL is not a supported Blob URL")
		} else {
			var err error
			sourceBytes, err = w.parent.opts.WorkerScriptFetcher(resolvedURL)
			if err != nil {
				return err
			}
		}
	}
	if len(sourceBytes) > MaxScriptBytes {
		return ErrTooLarge
	}
	workerOpts := Options{
		URL: resolvedURL, WorkerScriptFetcher: w.parent.opts.WorkerScriptFetcher,
		HTTPClient: w.parent.opts.HTTPClient,
	}
	if state := broadcastStateOf(w.parent); state != nil {
		workerOpts.BroadcastHub = state.hub
	}
	workerRuntime, err := New(workerOpts)
	if err != nil {
		return err
	}
	workerRuntime.workerParent = w.parent
	w.mu.Lock()
	if w.terminated {
		w.mu.Unlock()
		_ = workerRuntime.Close()
		return nil
	}
	w.runtime, w.onData, w.onError = workerRuntime, onData, onError
	w.mu.Unlock()
	global := workerRuntime.vm.GlobalObject()
	_ = workerRuntime.vm.Set("self", global)
	_ = global.Set("postMessage", func(call goja.FunctionCall) goja.Value {
		var data any
		if len(call.Arguments) != 0 {
			data = call.Argument(0).Export()
		}
		w.mu.Lock()
		callback, terminated := w.onData, w.terminated
		w.mu.Unlock()
		if callback != nil && !terminated {
			callback(data)
		}
		return goja.Undefined()
	})
	_ = global.Set("close", func(goja.FunctionCall) goja.Value {
		w.mu.Lock()
		w.closing = true
		w.mu.Unlock()
		workerRuntime.workerClosed = true
		return goja.Undefined()
	})
	_ = global.Set("importScripts", func(call goja.FunctionCall) goja.Value {
		for _, arg := range call.Arguments {
			importURL := workerRuntime.resolveURLString(arg.String())
			var imported []byte
			if blob, found := workerRuntime.objectURLSource(importURL); found {
				imported = blob
			} else if workerRuntime.opts.WorkerScriptFetcher != nil {
				var fetchErr error
				imported, fetchErr = workerRuntime.opts.WorkerScriptFetcher(importURL)
				if fetchErr != nil {
					panic(workerRuntime.vm.NewGoError(fetchErr))
				}
			} else {
				panic(workerRuntime.vm.NewTypeError("importScripts requires a worker script fetcher"))
			}
			if len(imported) > MaxScriptBytes {
				panic(workerRuntime.vm.NewGoError(ErrTooLarge))
			}
			if err := workerRuntime.Run(string(imported), importURL); err != nil {
				panic(workerRuntime.vm.NewGoError(err))
			}
		}
		return goja.Undefined()
	})
	w.runMu.Lock()
	if err := workerRuntime.Run(string(sourceBytes), scriptURL); err != nil {
		if onError != nil {
			onError(err)
		}
	}
	w.mu.Lock()
	closing := w.closing
	w.mu.Unlock()
	if !closing {
		workerRuntime.Tick()
	}
	w.mu.Lock()
	closing, terminated := w.closing, w.terminated
	w.ready = !closing && !terminated
	w.mu.Unlock()
	w.runMu.Unlock()
	if closing || terminated {
		_ = workerRuntime.Close()
	}
	return nil
}

func (w *localWorkerRunner) connect(port *goja.Object) error {
	w.runMu.Lock()
	defer w.runMu.Unlock()
	w.mu.Lock()
	workerRuntime := w.runtime
	active := w.ready && !w.terminated && !w.closing
	w.mu.Unlock()
	if !active || workerRuntime == nil {
		return errors.New("SharedWorker global scope is unavailable")
	}
	constructor, ok := goja.AssertConstructor(workerRuntime.vm.Get("MessageEvent"))
	if !ok {
		return errors.New("SharedWorker global scope does not support MessageEvent")
	}
	init := workerRuntime.vm.NewObject()
	_ = init.Set("ports", workerRuntime.vm.NewArray(port))
	event, err := constructor(nil, workerRuntime.vm.ToValue("connect"), init)
	if err != nil {
		return err
	}
	dispatch, ok := goja.AssertFunction(workerRuntime.vm.Get("dispatchEvent"))
	if !ok {
		return errors.New("SharedWorker global scope does not support event dispatch")
	}
	global := workerRuntime.vm.GlobalObject()
	if _, err := dispatch(global, event); err != nil {
		return err
	}
	if handler := global.Get("onconnect"); handler != nil && !goja.IsNull(handler) && !goja.IsUndefined(handler) {
		if fn, ok := goja.AssertFunction(handler); ok {
			if _, err := workerRuntime.callBounded(func() (goja.Value, error) { return fn(global, event) }); err != nil {
				return err
			}
		}
	}
	workerRuntime.DrainMessagePortEvents()
	return workerRuntime.drainMicrotasks()
}

func (w *localSharedWorkerRunner) DrainTasks() {
	w.mu.Lock()
	closed, runner := w.closed, w.runner
	w.mu.Unlock()
	if !closed && runner != nil {
		runner.DrainTasks()
	}
}

func (w *localSharedWorkerRunner) Terminate() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	runner := w.runner
	w.mu.Unlock()
	if runner != nil {
		return runner.Terminate()
	}
	return nil
}

// decodeDataWorkerURL reads a data: URL only after bounding its encoded size,
// then enforces the ordinary worker-script byte limit on the decoded source.
func decodeDataWorkerURL(raw string) (source string, isDataURL bool, err error) {
	comma := strings.IndexByte(raw, ',')
	if comma < 0 || !strings.EqualFold(raw[:comma], "data:") {
		return "", false, nil
	}
	isDataURL = true
	if len(raw) > MaxScriptBytes*4+8192 {
		return "", true, ErrTooLarge
	}
	metadata, payload := raw[len("data:"):comma], raw[comma+1:]
	if strings.HasSuffix(strings.ToLower(metadata), ";base64") {
		decoded, decodeErr := base64.StdEncoding.DecodeString(payload)
		if decodeErr != nil {
			return "", true, decodeErr
		}
		if len(decoded) > MaxScriptBytes {
			return "", true, ErrTooLarge
		}
		return string(decoded), true, nil
	}
	decoded, decodeErr := url.PathUnescape(payload)
	if decodeErr != nil {
		return "", true, decodeErr
	}
	if len(decoded) > MaxScriptBytes {
		return "", true, ErrTooLarge
	}
	return decoded, true, nil
}

func (w *localWorkerRunner) PostMessage(data any) error {
	w.mu.Lock()
	if w.terminated || w.closing {
		w.mu.Unlock()
		return nil
	}
	w.pending = append(w.pending, data)
	w.mu.Unlock()
	return nil
}

func (w *localWorkerRunner) DrainTasks() {
	w.mu.Lock()
	if !w.ready || w.terminated || w.closing {
		w.mu.Unlock()
		return
	}
	pending := append([]any(nil), w.pending...)
	w.pending = nil
	w.mu.Unlock()
	if len(pending) == 0 {
		w.runMu.Lock()
		w.mu.Lock()
		workerRuntime := w.runtime
		active := w.ready && !w.terminated && !w.closing
		w.mu.Unlock()
		if active && workerRuntime != nil {
			workerRuntime.Tick()
		}
		w.runMu.Unlock()
		return
	}
	for _, data := range pending {
		if err := w.deliver(data); err != nil {
			w.mu.Lock()
			onError := w.onError
			w.mu.Unlock()
			if onError != nil {
				onError(err)
			}
		}
	}
}

func (w *localWorkerRunner) deliver(data any) error {
	w.mu.Lock()
	workerRuntime := w.runtime
	terminated, closing := w.terminated, w.closing
	w.mu.Unlock()
	if workerRuntime == nil || terminated || closing {
		return nil
	}
	w.runMu.Lock()
	defer w.runMu.Unlock()
	w.mu.Lock()
	terminated, closing = w.terminated, w.closing
	w.mu.Unlock()
	if terminated || closing {
		return nil
	}
	event := dom.NewTrustedMessageEvent("message", false, false, workerRuntime.vm.ToValue(data), w.parent.windowOrigin(), "", nil, nil)
	workerRuntime.fireWindowListeners(&event.Event, true)
	workerRuntime.fireWindowListeners(&event.Event, false)
	w.mu.Lock()
	closing, terminated = w.closing, w.terminated
	w.mu.Unlock()
	if !closing && !terminated {
		workerRuntime.Tick()
	} else {
		_ = workerRuntime.Close()
	}
	return nil
}

func (w *localWorkerRunner) Terminate() error {
	w.mu.Lock()
	w.terminated = true
	workerRuntime := w.runtime
	w.mu.Unlock()
	if workerRuntime != nil {
		workerRuntime.Interrupt()
		w.runMu.Lock()
		defer w.runMu.Unlock()
		return workerRuntime.Close()
	}
	return nil
}
