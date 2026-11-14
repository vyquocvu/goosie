package js

import (
	"sync"
	"time"

	"github.com/dop251/goja"
)

type rafCallback struct {
	id       int
	callback goja.Value
}

type rafState struct {
	mu        sync.Mutex
	callbacks map[int]goja.Value
	nextID    int
}

const rafStateKey = "__rafState__"

func setRAFState(r *Runtime, fs *rafState) {
	_ = r.vm.Set(rafStateKey, fs)
}

func rafOf(r *Runtime) *rafState {
	v := r.vm.Get(rafStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	fs, ok := v.Export().(*rafState)
	if !ok {
		return nil
	}
	return fs
}

func (r *Runtime) setupRAF() {
	fs := &rafState{
		callbacks: make(map[int]goja.Value),
		nextID:    1,
	}
	setRAFState(r, fs)

	requestAnimationFrame := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return r.vm.ToValue(0)
		}

		callback := call.Arguments[0]
		if _, ok := goja.AssertFunction(callback); !ok {
			return r.vm.ToValue(0)
		}

		fs.mu.Lock()
		id := fs.nextID
		fs.nextID++
		fs.callbacks[id] = callback
		fs.mu.Unlock()

		return r.vm.ToValue(id)
	}

	cancelAnimationFrame := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}

		id := int(call.Arguments[0].ToInteger())

		fs.mu.Lock()
		delete(fs.callbacks, id)
		fs.mu.Unlock()

		return goja.Undefined()
	}

	_ = r.vm.Set("requestAnimationFrame", requestAnimationFrame)
	_ = r.vm.Set("cancelAnimationFrame", cancelAnimationFrame)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("requestAnimationFrame", requestAnimationFrame)
			_ = winObj.Set("cancelAnimationFrame", cancelAnimationFrame)
		}
	}
}

func (r *Runtime) TickRAF(timestamp time.Time) {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return
	}
	fs := rafOf(r)
	if fs == nil {
		return
	}

	fs.mu.Lock()
	callbacks := make(map[int]goja.Value, len(fs.callbacks))
	for id, cb := range fs.callbacks {
		callbacks[id] = cb
	}
	fs.callbacks = make(map[int]goja.Value)
	fs.mu.Unlock()

	ts := float64(timestamp.UnixNano()) / 1e6

	for _, cb := range callbacks {
		if f, ok := goja.AssertFunction(cb); ok {
			if _, err := r.callBounded(func() (goja.Value, error) { return f(nil, r.vm.ToValue(ts)) }); err != nil {
				return
			}
		}
	}

	r.TickAnimations()
}

func (r *Runtime) HasPendingRAF() bool {
	fs := rafOf(r)
	if fs != nil {
		fs.mu.Lock()
		has := len(fs.callbacks) > 0
		fs.mu.Unlock()
		if has {
			return true
		}
	}

	reg := getAnimRegistry(r)
	if reg != nil {
		for _, anim := range reg.anims {
			if anim.playState == "running" || anim.playState == "paused" {
				return true
			}
		}
	}

	return false
}
