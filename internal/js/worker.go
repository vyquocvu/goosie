package js

import (
	"sync"

	"github.com/dop251/goja"
)

// Web Workers API implementation.

// WorkerTask represents a unit of work executed in a worker.
type WorkerTask func(data string) string

// WorkerRunner executes worker scripts. The engine provides the concrete
// implementation; internal/js defines the interface so it does not import
// platform-specific code.
type WorkerRunner interface {
	Run(scriptURL string, onData func(string), onError func(error)) error
	PostMessage(data string) error
	Terminate() error
}

// WorkerDialer creates worker runners. The engine wires the concrete dialer
// at pipeline assembly time.
type WorkerDialer interface {
	CreateWorker(scriptURL string) (WorkerRunner, error)
}

// workerObj holds the internal state of one Worker object.
type workerObj struct {
	mu         sync.Mutex
	runner     WorkerRunner
	url        string
	terminated bool
	messages   []string
	errors     []string
	obj        *goja.Object
	r          *Runtime
}

// workerMessage is a pending message to be drained.
type workerMessage struct {
	worker *workerObj
	data   string
	isErr  bool
	errMsg string
}

const workerStateKey = "__workerState__"

type workerState struct {
	mu       sync.Mutex
	dialer   WorkerDialer
	workers  []*workerObj
	messages []workerMessage
}

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
	state := &workerState{
		dialer: dialer,
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

		// postMessage(data) method.
		_ = obj.Set("postMessage", func(call goja.FunctionCall) goja.Value {
			w.mu.Lock()
			if w.terminated {
				w.mu.Unlock()
				return goja.Undefined()
			}
			runner := w.runner
			w.mu.Unlock()

			if runner == nil {
				return goja.Undefined()
			}

			data := ""
			if len(call.Arguments) >= 1 {
				data = call.Arguments[0].String()
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

			w.mu.Lock()
			w.runner = runner
			w.mu.Unlock()

			// Set up message callbacks.
			_ = runner.Run(scriptURL,
				func(data string) {
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
			)
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
}

// DrainWorkerMessages drains pending worker messages and fires them on the
// main goroutine. Call this from Tick.
func (r *Runtime) DrainWorkerMessages() {
	state := workerStateOf(r)
	if state == nil {
		return
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
		w.mu.Unlock()

		if obj == nil {
			continue
		}

		if msg.isErr {
			handler := obj.Get("onerror")
			if handler == nil || goja.IsNull(handler) || goja.IsUndefined(handler) {
				continue
			}
			fn, ok := goja.AssertFunction(handler)
			if !ok {
				continue
			}
			errObj := r.vm.NewObject()
			_ = errObj.Set("message", msg.errMsg)
			_, _ = fn(obj, errObj)
		} else {
			handler := obj.Get("onmessage")
			if handler == nil || goja.IsNull(handler) || goja.IsUndefined(handler) {
				continue
			}
			fn, ok := goja.AssertFunction(handler)
			if !ok {
				continue
			}
			eventObj := r.vm.NewObject()
			_ = eventObj.Set("data", msg.data)
			_ = eventObj.Set("type", "message")
			_, _ = fn(obj, eventObj)
		}
	}
}
