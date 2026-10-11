package js

import (
	"errors"
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

// Promise state constants per the ECMAScript specification.
const (
	promisePending   = "pending"
	promiseFulfilled = "fulfilled"
	promiseRejected  = "rejected"
)

// Hidden property key to store the internal promiseObj on the JS object.
const promiseInternalKey = "__promise__"

// promiseReaction is a callback waiting for a promise to settle.
type promiseReaction struct {
	promise     *promiseObj
	onFulfilled goja.Value
	onRejected  goja.Value
}

// promiseObj holds the internal state of a Promise.
type promiseObj struct {
	r         *Runtime
	obj       *goja.Object
	mu        sync.Mutex
	state     string
	result    goja.Value
	reactions []promiseReaction
}

// setupPromise installs the Promise constructor and related APIs on the
// global scope. This provides a standards-compliant Promise implementation
// that works with async/await patterns common in modern JavaScript.
func (r *Runtime) setupPromise() {
	promiseConstructor := func(call goja.ConstructorCall) *goja.Object {
		return r.constructPromise(call)
	}

	promiseCtorObj := r.vm.ToValue(promiseConstructor).(*goja.Object)

	_ = promiseCtorObj.Set("resolve", func(call goja.FunctionCall) goja.Value {
		return r.promiseResolveValue(call)
	})

	_ = promiseCtorObj.Set("reject", func(call goja.FunctionCall) goja.Value {
		return r.promiseRejectValue(call)
	})

	_ = promiseCtorObj.Set("all", func(call goja.FunctionCall) goja.Value {
		return r.promiseAll(call)
	})

	_ = promiseCtorObj.Set("race", func(call goja.FunctionCall) goja.Value {
		return r.promiseRace(call)
	})

	_ = r.vm.Set("Promise", promiseCtorObj)
}

// constructPromise implements the Promise(executor) constructor.
func (r *Runtime) constructPromise(call goja.ConstructorCall) *goja.Object {
	p := &promiseObj{
		r:     r,
		state: promisePending,
	}

	obj := call.This
	p.obj = obj

	// Store the internal promiseObj on the JS object for later retrieval.
	_ = obj.Set(promiseInternalKey, p)

	_ = obj.Set("then", func(call goja.FunctionCall) goja.Value {
		return r.promiseThen(call, p)
	})

	_ = obj.Set("catch", func(call goja.FunctionCall) goja.Value {
		return r.promiseCatch(call, p)
	})

	_ = obj.Set("finally", func(call goja.FunctionCall) goja.Value {
		return r.promiseFinally(call, p)
	})

	if len(call.Arguments) < 1 {
		return obj
	}

	executor, ok := goja.AssertFunction(call.Arguments[0])
	if !ok {
		return obj
	}

	// The executor runs synchronously during construction, per spec. A throw
	// rejects the promise it was building instead of escaping — except an
	// interrupt, which must keep propagating so Run still reports timeouts
	// that fire mid-executor.
	resolve := r.resolveFunc(p)
	reject := r.rejectFunc(p)
	if _, err := executor(nil, r.vm.ToValue(resolve), r.vm.ToValue(reject)); err != nil {
		var ie *goja.InterruptedError
		if errors.As(err, &ie) {
			panic(err)
		}
		r.rejectPromise(p, r.vm.ToValue(err.Error()))
	}

	return obj
}

// resolveFunc builds the resolve callback handed to executors and thenables:
// settling always enqueues, so user code never runs on a foreign goroutine.
func (r *Runtime) resolveFunc(p *promiseObj) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		var val goja.Value
		if len(call.Arguments) > 0 {
			val = call.Arguments[0]
		} else {
			val = goja.Undefined()
		}
		r.resolvePromise(p, val)
		return goja.Undefined()
	}
}

// rejectFunc builds the matching reject callback.
func (r *Runtime) rejectFunc(p *promiseObj) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		var val goja.Value
		if len(call.Arguments) > 0 {
			val = call.Arguments[0]
		} else {
			val = goja.Undefined()
		}
		r.rejectPromise(p, val)
		return goja.Undefined()
	}
}

// queueMicrotask appends a reaction job. Producers on any goroutine only
// ever append; only the Run/Tick goroutine drains, so user callbacks always
// run on the runtime's own thread even when a network event settled.
func (r *Runtime) queueMicrotask(job func() error) {
	r.mu.Lock()
	r.microtasks = append(r.microtasks, job)
	r.mu.Unlock()
}

// drainMicrotasks runs queued jobs FIFO until empty: one microtask
// checkpoint. A job that settles further promises only enqueues, so chains
// cannot grow the Go stack no matter how deep they run. The first error
// stops the drain; only interrupts produce errors.
func (r *Runtime) drainMicrotasks() error {
	for i := 0; ; i++ {
		if i >= MaxMicrotasksPerCheckpoint {
			return ErrInterrupted
		}
		r.mu.Lock()
		if len(r.microtasks) == 0 {
			r.mu.Unlock()
			return nil
		}
		job := r.microtasks[0]
		r.microtasks[0] = nil
		r.microtasks = r.microtasks[1:]
		r.mu.Unlock()
		if err := job(); err != nil {
			return err
		}
	}
}

// resolvePromise settles a promise as fulfilled. Everything past the enqueue
// runs on the drain goroutine: thenable getters and assimilation call user
// code, which must never run on a foreign (e.g. network) thread or inline
// inside whatever settled.
func (r *Runtime) resolvePromise(p *promiseObj, val goja.Value) {
	r.queueMicrotask(func() error {
		p.mu.Lock()
		if p.state != promisePending {
			p.mu.Unlock()
			return nil
		}
		if obj, ok := val.(*goja.Object); ok {
			p.mu.Unlock()
			thenVal := obj.Get("then")
			if thenVal != nil {
				if f, ok := goja.AssertFunction(thenVal); ok {
					_, err := f(obj, r.vm.ToValue(r.resolveFunc(p)), r.vm.ToValue(r.rejectFunc(p)))
					if err != nil {
						var ie *goja.InterruptedError
						if errors.As(err, &ie) {
							return ErrInterrupted
						}
						r.rejectPromise(p, r.vm.ToValue(err.Error()))
					}
					return nil
				}
			}
			p.mu.Lock()
			if p.state != promisePending {
				// The getter settled us while reading `then`.
				p.mu.Unlock()
				return nil
			}
		}
		p.state = promiseFulfilled
		p.result = val
		reactions := p.reactions
		p.reactions = nil
		p.mu.Unlock()

		for _, reaction := range reactions {
			if err := r.runReaction(reaction, false, val); err != nil {
				return err
			}
		}
		return nil
	})
}

// rejectPromise settles a promise as rejected, same queuing as resolve.
func (r *Runtime) rejectPromise(p *promiseObj, val goja.Value) {
	r.queueMicrotask(func() error {
		p.mu.Lock()
		if p.state != promisePending {
			p.mu.Unlock()
			return nil
		}
		p.state = promiseRejected
		p.result = val
		reactions := p.reactions
		p.reactions = nil
		p.mu.Unlock()

		for _, reaction := range reactions {
			if err := r.runReaction(reaction, true, val); err != nil {
				return err
			}
		}
		return nil
	})
}

// runReaction invokes one reaction callback with the promise's result,
// settling the chained promise with whatever it returns. It reports only
// interrupts; ordinary callback failures become rejections of the chained
// promise. It runs on the drain goroutine, never inline in a settler.
func (r *Runtime) runReaction(reaction promiseReaction, isRejected bool, val goja.Value) error {
	var callback goja.Value
	if isRejected {
		callback = reaction.onRejected
	} else {
		callback = reaction.onFulfilled
	}

	if callback == nil || goja.IsUndefined(callback) {
		if isRejected {
			r.rejectPromise(reaction.promise, val)
		} else {
			r.resolvePromise(reaction.promise, val)
		}
		return nil
	}

	f, ok := goja.AssertFunction(callback)
	if !ok {
		if isRejected {
			r.rejectPromise(reaction.promise, val)
		} else {
			r.resolvePromise(reaction.promise, val)
		}
		return nil
	}

	ret, err := f(nil, val)
	if err != nil {
		var ie *goja.InterruptedError
		if errors.As(err, &ie) {
			return ErrInterrupted
		}
		r.rejectPromise(reaction.promise, r.vm.ToValue(err.Error()))
		return nil
	}

	r.resolvePromise(reaction.promise, ret)
	return nil
}

// promiseThen implements promise.then(onFulfilled, onRejected).
func (r *Runtime) promiseThen(call goja.FunctionCall, p *promiseObj) goja.Value {
	var onFulfilled, onRejected goja.Value
	if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Arguments[0]) {
		onFulfilled = call.Arguments[0]
	}
	if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Arguments[1]) {
		onRejected = call.Arguments[1]
	}

	chained := r.newPromiseObj()

	reaction := promiseReaction{
		promise:     chained,
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
	}

	p.mu.Lock()
	if p.state == promisePending {
		p.reactions = append(p.reactions, reaction)
		p.mu.Unlock()
	} else {
		// Already settled: the reaction still queues rather than running
		// inline, so attach order always wins over settle order.
		p.reactions = append(p.reactions, reaction)
		p.mu.Unlock()
		r.queueMicrotask(func() error {
			p.mu.Lock()
			reactions := p.reactions
			p.reactions = nil
			st := p.state
			res := p.result
			p.mu.Unlock()
			if st == promisePending {
				return nil
			}
			for _, rt := range reactions {
				if err := r.runReaction(rt, st == promiseRejected, res); err != nil {
					return err
				}
			}
			return nil
		})
	}

	return chained.obj
}

// promiseCatch implements promise.catch(onRejected).
func (r *Runtime) promiseCatch(call goja.FunctionCall, p *promiseObj) goja.Value {
	var onRejected goja.Value
	if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Arguments[0]) {
		onRejected = call.Arguments[0]
	}

	return r.promiseThen(goja.FunctionCall{
		This:      p.obj,
		Arguments: []goja.Value{goja.Undefined(), onRejected},
	}, p)
}

// promiseFinally implements promise.finally(onFinally).
func (r *Runtime) promiseFinally(call goja.FunctionCall, p *promiseObj) goja.Value {
	var onFinally goja.Value
	if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Arguments[0]) {
		onFinally = call.Arguments[0]
	}

	if onFinally == nil || goja.IsUndefined(onFinally) {
		return r.promiseThen(call, p)
	}

	finallyFn, ok := goja.AssertFunction(onFinally)
	if !ok {
		return r.promiseThen(call, p)
	}

	wrappedFulfilled := func(call goja.FunctionCall) goja.Value {
		_, _ = finallyFn(nil)
		if len(call.Arguments) > 0 {
			return call.Arguments[0]
		}
		return goja.Undefined()
	}

	wrappedRejected := func(call goja.FunctionCall) goja.Value {
		_, _ = finallyFn(nil)
		if len(call.Arguments) > 0 {
			return call.Arguments[0]
		}
		return goja.Undefined()
	}

	return r.promiseThen(goja.FunctionCall{
		This:      p.obj,
		Arguments: []goja.Value{r.vm.ToValue(wrappedFulfilled), r.vm.ToValue(wrappedRejected)},
	}, p)
}

// newPromiseObj creates a new pending promise object.
func (r *Runtime) newPromiseObj() *promiseObj {
	p := &promiseObj{
		r:     r,
		state: promisePending,
	}

	obj := r.vm.NewObject()
	p.obj = obj
	_ = obj.Set(promiseInternalKey, p)

	_ = obj.Set("then", func(call goja.FunctionCall) goja.Value {
		return r.promiseThen(call, p)
	})

	_ = obj.Set("catch", func(call goja.FunctionCall) goja.Value {
		return r.promiseCatch(call, p)
	})

	_ = obj.Set("finally", func(call goja.FunctionCall) goja.Value {
		return r.promiseFinally(call, p)
	})

	return p
}

// getInternalPromise retrieves the internal promiseObj from a JS object.
func getInternalPromise(obj *goja.Object) *promiseObj {
	v := obj.Get(promiseInternalKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	p, ok := v.Export().(*promiseObj)
	if !ok {
		return nil
	}
	return p
}

// promiseResolveValue implements Promise.resolve(value).
func (r *Runtime) promiseResolveValue(call goja.FunctionCall) goja.Value {
	var val goja.Value
	if len(call.Arguments) > 0 {
		val = call.Arguments[0]
	} else {
		val = goja.Undefined()
	}

	// If the value is already a promise with our internal marker, return it.
	if obj, ok := val.(*goja.Object); ok {
		if getInternalPromise(obj) != nil {
			return val
		}
	}

	p := r.newPromiseObj()
	r.resolvePromise(p, val)
	return p.obj
}

// promiseRejectValue implements Promise.reject(reason).
func (r *Runtime) promiseRejectValue(call goja.FunctionCall) goja.Value {
	var val goja.Value
	if len(call.Arguments) > 0 {
		val = call.Arguments[0]
	} else {
		val = goja.Undefined()
	}

	p := r.newPromiseObj()
	r.rejectPromise(p, val)
	return p.obj
}

// promiseAll implements Promise.all(iterable).
func (r *Runtime) promiseAll(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		p := r.newPromiseObj()
		r.rejectPromise(p, r.vm.ToValue("Promise.all requires an iterable"))
		return p.obj
	}

	iterable := call.Arguments[0]
	arr, ok := iterable.(*goja.Object)
	if !ok {
		p := r.newPromiseObj()
		r.rejectPromise(p, r.vm.ToValue("Promise.all requires an iterable"))
		return p.obj
	}

	lengthVal := arr.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		lengthVal = r.vm.ToValue(0)
	}
	length := int(lengthVal.ToInteger())

	if length == 0 {
		p := r.newPromiseObj()
		r.resolvePromise(p, r.vm.NewArray())
		return p.obj
	}

	p := r.newPromiseObj()
	results := make([]goja.Value, length)
	remaining := length

	for i := 0; i < length; i++ {
		val := arr.Get(fmt.Sprintf("%d", i))
		if val == nil {
			val = goja.Undefined()
		}

		// Wrap value in a resolved promise if it's not already a promise.
		var promiseJSObj *goja.Object
		if obj, ok := val.(*goja.Object); ok && getInternalPromise(obj) != nil {
			promiseJSObj = obj
		} else {
			wrapped := r.promiseResolveValue(goja.FunctionCall{Arguments: []goja.Value{val}})
			promiseJSObj, _ = wrapped.(*goja.Object)
		}

		if promiseJSObj == nil {
			continue
		}

		innerP := getInternalPromise(promiseJSObj)
		if innerP == nil {
			continue
		}

		index := i
		thenFn := func(call goja.FunctionCall) goja.Value {
			results[index] = call.Arguments[0]
			remaining--
			if remaining == 0 {
				resultArr := r.vm.NewArray()
				for j, v := range results {
					_ = resultArr.Set(fmt.Sprintf("%d", j), v)
				}
				r.resolvePromise(p, resultArr)
			}
			return goja.Undefined()
		}

		catchFn := func(call goja.FunctionCall) goja.Value {
			r.rejectPromise(p, call.Arguments[0])
			return goja.Undefined()
		}

		_ = r.promiseThen(goja.FunctionCall{
			This:      promiseJSObj,
			Arguments: []goja.Value{r.vm.ToValue(thenFn), r.vm.ToValue(catchFn)},
		}, innerP)
	}

	return p.obj
}

// promiseRace implements Promise.race(iterable).
func (r *Runtime) promiseRace(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		p := r.newPromiseObj()
		r.rejectPromise(p, r.vm.ToValue("Promise.race requires an iterable"))
		return p.obj
	}

	iterable := call.Arguments[0]
	arr, ok := iterable.(*goja.Object)
	if !ok {
		p := r.newPromiseObj()
		r.rejectPromise(p, r.vm.ToValue("Promise.race requires an iterable"))
		return p.obj
	}

	lengthVal := arr.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		lengthVal = r.vm.ToValue(0)
	}
	length := int(lengthVal.ToInteger())

	if length == 0 {
		return r.newPromiseObj().obj
	}

	p := r.newPromiseObj()

	for i := 0; i < length; i++ {
		val := arr.Get(fmt.Sprintf("%d", i))
		if val == nil {
			val = goja.Undefined()
		}

		var promiseJSObj *goja.Object
		if obj, ok := val.(*goja.Object); ok && getInternalPromise(obj) != nil {
			promiseJSObj = obj
		} else {
			wrapped := r.promiseResolveValue(goja.FunctionCall{Arguments: []goja.Value{val}})
			promiseJSObj, _ = wrapped.(*goja.Object)
		}

		if promiseJSObj == nil {
			continue
		}

		innerP := getInternalPromise(promiseJSObj)
		if innerP == nil {
			continue
		}

		thenFn := func(call goja.FunctionCall) goja.Value {
			r.resolvePromise(p, call.Arguments[0])
			return goja.Undefined()
		}

		catchFn := func(call goja.FunctionCall) goja.Value {
			r.rejectPromise(p, call.Arguments[0])
			return goja.Undefined()
		}

		_ = r.promiseThen(goja.FunctionCall{
			This:      promiseJSObj,
			Arguments: []goja.Value{r.vm.ToValue(thenFn), r.vm.ToValue(catchFn)},
		}, innerP)
	}

	return p.obj
}
