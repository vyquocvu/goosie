package js

import (
	"errors"
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

const (
	promisePending   = "pending"
	promiseFulfilled = "fulfilled"
	promiseRejected  = "rejected"
)

const promiseInternalKey = "__promise__"

type promiseReaction struct {
	promise     *promiseObj
	onFulfilled goja.Value
	onRejected  goja.Value
}

type promiseObj struct {
	r         *Runtime
	obj       *goja.Object
	mu        sync.Mutex
	state     string
	result    goja.Value
	reactions []promiseReaction
}

func (r *Runtime) setupPromise() {
	promiseConstructor := func(call goja.ConstructorCall) *goja.Object {
		return r.constructPromise(call)
	}

	promiseCtorObj := r.vm.ToValue(promiseConstructor).(*goja.Object)
	if prototype, ok := promiseCtorObj.Get("prototype").(*goja.Object); ok {
		r.promiseProto = prototype
		_ = prototype.Set("constructor", promiseCtorObj)
		_ = r.vm.Set("__promiseConstructor__", promiseCtorObj)
		_, _ = r.vm.RunString(`Object.defineProperty(__promiseConstructor__, "name", { value: "Promise", configurable: true })`)
		_ = r.vm.Set("__promiseConstructor__", nil)
	}

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

	_ = promiseCtorObj.Set("withResolvers", func(call goja.FunctionCall) goja.Value {
		return r.promiseWithResolvers(call)
	})
	_ = r.vm.Set("__promiseConstructorWithResolvers__", promiseCtorObj)
	_ = r.vm.Set("__promiseWithResolversMethod__", promiseCtorObj.Get("withResolvers"))
	_, _ = r.vm.RunString(`
		Object.defineProperty(__promiseWithResolversMethod__, "name", {
			value: "withResolvers",
			configurable: true
		});
		Object.defineProperty(__promiseConstructorWithResolvers__, "withResolvers", {
			enumerable: false,
			configurable: true,
			writable: true
		});
	`)
	_ = r.vm.Set("__promiseWithResolversMethod__", nil)
	_ = r.vm.Set("__promiseConstructorWithResolvers__", nil)

	_ = r.vm.Set("Promise", promiseCtorObj)
}

func (r *Runtime) promiseWithResolvers(call goja.FunctionCall) goja.Value {
	constructor, ok := goja.AssertConstructor(call.This)
	if !ok {
		panic(r.vm.NewTypeError("Promise.withResolvers called on a non-constructor"))
	}
	var resolve, reject goja.Value
	called := false
	var executorErr error
	executor := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if called {
			executorErr = errors.New("Promise capability executor called more than once")
			panic(r.vm.NewTypeError(executorErr.Error()))
		}
		called = true
		resolve = call.Argument(0)
		reject = call.Argument(1)
		return goja.Undefined()
	})
	promise, err := constructor(nil, executor)
	if executorErr != nil {
		panic(r.vm.NewTypeError(executorErr.Error()))
	}
	if err != nil {
		panic(err)
	}
	if !called {
		panic(r.vm.NewTypeError("Promise constructor did not initialize its capability"))
	}
	if _, ok := goja.AssertFunction(resolve); !ok {
		panic(r.vm.NewTypeError("Promise capability resolve is not callable"))
	}
	if _, ok := goja.AssertFunction(reject); !ok {
		panic(r.vm.NewTypeError("Promise capability reject is not callable"))
	}
	result := r.vm.NewObject()
	_ = result.Set("promise", promise)
	_ = result.Set("resolve", resolve)
	_ = result.Set("reject", reject)
	return result
}

func (r *Runtime) constructPromise(call goja.ConstructorCall) *goja.Object {
	p := &promiseObj{
		r:     r,
		state: promisePending,
	}

	obj := call.This
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

	if len(call.Arguments) < 1 {
		return obj
	}

	executor, ok := goja.AssertFunction(call.Arguments[0])
	if !ok {
		return obj
	}

	resolve := r.resolveFunc(p)
	reject := r.rejectFunc(p)
	if _, err := executor(nil, r.vm.ToValue(resolve), r.vm.ToValue(reject)); err != nil {
		var ie *goja.InterruptedError
		if errors.As(err, &ie) {
			panic(err)
		}
		var rejected goja.Value = r.vm.ToValue(err.Error())
		var exception *goja.Exception
		if errors.As(err, &exception) {
			rejected = exception.Value()
		}
		r.rejectPromise(p, rejected)
	}

	return obj
}

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

func (r *Runtime) queueMicrotask(job func() error) {
	r.mu.Lock()
	r.microtasks = append(r.microtasks, job)
	r.mu.Unlock()
}

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

	r.scriptExecutionDepth++
	var ret goja.Value
	var err error
	func() {
		defer func() { r.scriptExecutionDepth-- }()
		ret, err = f(nil, val)
	}()
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

func (r *Runtime) newPromiseObj() *promiseObj {
	p := &promiseObj{
		r:     r,
		state: promisePending,
	}

	obj := r.vm.NewObject()
	if r.promiseProto != nil {
		obj.SetPrototype(r.promiseProto)
	}
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

func (r *Runtime) promiseResolveValue(call goja.FunctionCall) goja.Value {
	var val goja.Value
	if len(call.Arguments) > 0 {
		val = call.Arguments[0]
	} else {
		val = goja.Undefined()
	}

	if obj, ok := val.(*goja.Object); ok {
		if getInternalPromise(obj) != nil {
			return val
		}
	}

	p := r.newPromiseObj()
	r.resolvePromise(p, val)
	return p.obj
}

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
