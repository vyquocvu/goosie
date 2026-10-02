package js

import (
	"sync"

	"github.com/dop251/goja"
)

// Web Storage API implementation for localStorage and sessionStorage.
// localStorage persists across sessions, sessionStorage is per-tab.

// storageObj holds the internal state of a Storage object.
type storageObj struct {
	mu   sync.RWMutex
	data map[string]string
	// Order tracks insertion order for key(index) method.
	order []string
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

	// setItem(key, value) - stores the key-value pair.
	_ = obj.Set("setItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()
		value := call.Arguments[1].String()

		storage.mu.Lock()
		if _, exists := storage.data[key]; !exists {
			storage.order = append(storage.order, key)
		}
		storage.data[key] = value
		storage.mu.Unlock()

		return goja.Undefined()
	})

	// removeItem(key) - removes the key-value pair.
	_ = obj.Set("removeItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()

		storage.mu.Lock()
		if _, exists := storage.data[key]; exists {
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

	// clear() - removes all key-value pairs.
	_ = obj.Set("clear", func(call goja.FunctionCall) goja.Value {
		storage.mu.Lock()
		storage.data = make(map[string]string)
		storage.order = make([]string, 0)
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

	// length property.
	_ = obj.Set("length", func(call goja.FunctionCall) goja.Value {
		storage.mu.RLock()
		length := len(storage.data)
		storage.mu.RUnlock()
		return r.vm.ToValue(length)
	})

	// Install on global scope.
	_ = r.vm.Set(name, obj)

	// Also install on window object.
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set(name, obj)
		}
	}
}
