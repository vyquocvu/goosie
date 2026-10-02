package js

import (
	"sync"
	"time"

	"github.com/dop251/goja"
)

// requestAnimationFrame implementation for smooth animations.
// Schedules a callback to run before the next repaint, typically at 60fps.

// rafCallback is one pending requestAnimationFrame callback.
type rafCallback struct {
	id       int
	callback goja.Value
}

// rafState holds the per-runtime requestAnimationFrame state.
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

// setupRAF installs requestAnimationFrame and cancelAnimationFrame on the global scope.
func (r *Runtime) setupRAF() {
	fs := &rafState{
		callbacks: make(map[int]goja.Value),
		nextID:    1,
	}
	setRAFState(r, fs)

	// requestAnimationFrame(callback) - schedules callback for next frame.
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

	// cancelAnimationFrame(id) - cancels a scheduled callback.
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

	// Install on global scope.
	_ = r.vm.Set("requestAnimationFrame", requestAnimationFrame)
	_ = r.vm.Set("cancelAnimationFrame", cancelAnimationFrame)

	// Also install on window object.
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("requestAnimationFrame", requestAnimationFrame)
			_ = winObj.Set("cancelAnimationFrame", cancelAnimationFrame)
		}
	}
}

// TickRAF invokes all pending requestAnimationFrame callbacks.
// Called by the engine once per frame (typically 60fps).
// The timestamp parameter is the current time in milliseconds.
func (r *Runtime) TickRAF(timestamp time.Time) {
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

	// Convert timestamp to DOMHighResTimeStamp (milliseconds since page load).
	// For simplicity, we use milliseconds since Unix epoch.
	ts := float64(timestamp.UnixNano()) / 1e6

	// Invoke all callbacks with the timestamp.
	for _, cb := range callbacks {
		if f, ok := goja.AssertFunction(cb); ok {
			_, _ = f(nil, r.vm.ToValue(ts))
		}
	}
}

// HasPendingRAF reports whether there are any pending requestAnimationFrame callbacks.
func (r *Runtime) HasPendingRAF() bool {
	fs := rafOf(r)
	if fs == nil {
		return false
	}

	fs.mu.Lock()
	has := len(fs.callbacks) > 0
	fs.mu.Unlock()

	return has
}
