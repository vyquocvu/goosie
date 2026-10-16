package js

import (
	"sync"

	"github.com/dop251/goja"
)

// Web Storage API implementation for localStorage and sessionStorage.
// localStorage persists across sessions, sessionStorage is per-tab.

// MaxStorageBytes is the per-store quota, matching the ~5 MiB browsers
// grant each origin. Keys and values both count, in UTF-8 bytes (close to
// the UTF-16 code units the spec counts, without a second encoding pass).
// Writes past it throw instead of growing: an unbounded store is a memory
// DoS any page reaches with a loop.
const MaxStorageBytes = 5 << 20

// storageObj holds the internal state of a Storage object.
type storageObj struct {
	mu   sync.RWMutex
	data map[string]string
	// Order tracks insertion order for key(index) method.
	order []string
	// used prices stored bytes (keys plus values) against MaxStorageBytes.
	used int64
}

// newStorageObj creates a new empty storage object.
func newStorageObj() *storageObj {
	return &storageObj{
		data:  make(map[string]string),
		order: make([]string, 0),
	}
}

// setupStorage installs localStorage and sessionStorage on the global scope.
func (r *Runtime) setupStorage() {
	// localStorage: persists across sessions (in real browser, backed by disk).
	// For now, we use in-memory storage that persists for the runtime lifetime.
	localStorage := newStorageObj()
	r.installStorage("localStorage", localStorage)

	// sessionStorage: per-tab, cleared when tab closes.
	sessionStorage := newStorageObj()
	r.installStorage("sessionStorage", sessionStorage)
}

// installStorage installs a storage object on the global scope and window.
func (r *Runtime) installStorage(name string, storage *storageObj) {
	obj := r.vm.NewObject()

	// getItem(key) - returns the value or null.
	_ = obj.Set("getItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		key := call.Arguments[0].String()
		storage.mu.RLock()
		val, ok := storage.data[key]
		storage.mu.RUnlock()
		if !ok {
			return goja.Null()
		}
		return r.vm.ToValue(val)
	})

	// setItem(key, value) - stores the key-value pair, throwing a
	// QuotaExceededError-named failure past MaxStorageBytes. The lock is
	// released before throwing: panicking while holding it would wedge the
	// store for every later call.
	_ = obj.Set("setItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()
		value := call.Arguments[1].String()

		storage.mu.Lock()
		old := 0
		_, exists := storage.data[key]
		if exists {
			old = len(key) + len(storage.data[key])
		}
		if storage.used-int64(old)+int64(len(key)+len(value)) > MaxStorageBytes {
			storage.mu.Unlock()
			panic(quotaExceededError(r))
		}
		if !exists {
			storage.order = append(storage.order, key)
		}
		storage.data[key] = value
		storage.used = storage.used - int64(old) + int64(len(key)+len(value))
		storage.mu.Unlock()

		return goja.Undefined()
	})

	// removeItem(key) - removes the key-value pair, freeing its budget.
	_ = obj.Set("removeItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()

		storage.mu.Lock()
		if prev, exists := storage.data[key]; exists {
			storage.used -= int64(len(key) + len(prev))
			delete(storage.data, key)
			// Remove from order.
			for i, k := range storage.order {
				if k == key {
					storage.order = append(storage.order[:i], storage.order[i+1:]...)
					break
				}
			}
		}
		storage.mu.Unlock()

		return goja.Undefined()
	})

	// clear() - removes all key-value pairs, freeing the whole budget.
	_ = obj.Set("clear", func(call goja.FunctionCall) goja.Value {
		storage.mu.Lock()
		storage.data = make(map[string]string)
		storage.order = make([]string, 0)
		storage.used = 0
		storage.mu.Unlock()

		return goja.Undefined()
	})

	// key(index) - returns the key at the given index.
	_ = obj.Set("key", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		index := int(call.Arguments[0].ToInteger())

		storage.mu.RLock()
		if index < 0 || index >= len(storage.order) {
			storage.mu.RUnlock()
			return goja.Null()
		}
		key := storage.order[index]
		storage.mu.RUnlock()

		return r.vm.ToValue(key)
	})

	// length is a live numeric property, not a method: pages read it as a
	// number, and sloppy assignment to it is ignored via the undefined
	// setter rather than replacing the count.
	_ = obj.DefineAccessorProperty("length",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			storage.mu.RLock()
			n := len(storage.data)
			storage.mu.RUnlock()
			return r.vm.ToValue(n)
		}),
		goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)

	// Install on global scope.
	_ = r.vm.Set(name, obj)

	// Also install on window object.
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set(name, obj)
		}
	}
}

// quotaExceededError builds the QuotaExceededError-named failure a
// past-budget setItem throws. Plain error object with the spec name, which
// is what callers assert on; there is no DOMException hierarchy yet.
func quotaExceededError(r *Runtime) goja.Value {
	e := r.vm.NewObject()
	_ = e.Set("name", "QuotaExceededError")
	_ = e.Set("message", "Storage quota exceeded")
	return e
}
