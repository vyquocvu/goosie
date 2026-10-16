package js

import (
	"fmt"
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// ResizeObserver API implementation.

// resizeEntry represents one observed element's size.
type resizeEntry struct {
	target      *dom.Node
	width       float64
	height      float64
	contentRect map[string]float64
}

// resizeObserver holds the internal state of one ResizeObserver.
type resizeObserver struct {
	mu           sync.Mutex
	callback     goja.Callable
	targets      map[*dom.Node]bool
	entries      []resizeEntry
	r            *Runtime
	disconnected bool
	obj          *goja.Object
}

const resizeStateKey = "__resizeState__"

type resizeState struct {
	mu        sync.Mutex
	observers []*resizeObserver
	rects     map[*dom.Node][2]float64 // width, height
}

func setResizeState(r *Runtime, s *resizeState) {
	_ = r.vm.Set(resizeStateKey, s)
}

func resizeStateOf(r *Runtime) *resizeState {
	v := r.vm.Get(resizeStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	s, ok := v.Export().(*resizeState)
	if !ok {
		return nil
	}
	return s
}

// setupResizeObserver installs the ResizeObserver constructor.
func (r *Runtime) setupResizeObserver() {
	state := &resizeState{
		rects: make(map[*dom.Node][2]float64),
	}
	setResizeState(r, state)

	constructor := func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("ResizeObserver requires a callback argument"))
		}

		callback, ok := goja.AssertFunction(call.Arguments[0])
		if !ok {
			panic(r.vm.NewTypeError("ResizeObserver callback must be a function"))
		}

		obs := &resizeObserver{
			callback: callback,
			targets:  make(map[*dom.Node]bool),
			r:        r,
		}

		obj := call.This
		obs.obj = obj

		// observe(target)
		_ = obj.Set("observe", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(r.vm.NewTypeError("observe requires a target argument"))
			}

			targetArg := call.Arguments[0]
			if targetArg == nil || goja.IsUndefined(targetArg) || goja.IsNull(targetArg) {
				panic(r.vm.NewTypeError("observe target must be a Node"))
			}

			targetObj, ok := targetArg.(*goja.Object)
			if !ok {
				panic(r.vm.NewTypeError("observe target must be a Node"))
			}

			nidVal := targetObj.Get("__nid__")
			if nidVal == nil || goja.IsUndefined(nidVal) {
				panic(r.vm.NewTypeError("observe target must be a Node"))
			}

			nid := int(nidVal.ToInteger())
			node := r.nodeRegistry[nid]
			if node == nil {
				panic(r.vm.NewTypeError("observe target is not a valid Node"))
			}

			obs.mu.Lock()
			obs.targets[node] = true
			obs.mu.Unlock()

			state.mu.Lock()
			found := false
			for _, o := range state.observers {
				if o == obs {
					found = true
					break
				}
			}
			if !found {
				state.observers = append(state.observers, obs)
			}
			state.mu.Unlock()

			return goja.Undefined()
		})

		// unobserve(target)
		_ = obj.Set("unobserve", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Undefined()
			}

			targetArg := call.Arguments[0]
			if targetArg == nil || goja.IsUndefined(targetArg) || goja.IsNull(targetArg) {
				return goja.Undefined()
			}

			targetObj, ok := targetArg.(*goja.Object)
			if !ok {
				return goja.Undefined()
			}

			nidVal := targetObj.Get("__nid__")
			if nidVal == nil || goja.IsUndefined(nidVal) {
				return goja.Undefined()
			}

			nid := int(nidVal.ToInteger())
			node := r.nodeRegistry[nid]
			if node == nil {
				return goja.Undefined()
			}

			obs.mu.Lock()
			delete(obs.targets, node)
			obs.mu.Unlock()

			return goja.Undefined()
		})

		// disconnect()
		_ = obj.Set("disconnect", func(call goja.FunctionCall) goja.Value {
			obs.mu.Lock()
			obs.disconnected = true
			obs.targets = make(map[*dom.Node]bool)
			obs.entries = nil
			obs.mu.Unlock()
			return goja.Undefined()
		})

		return nil
	}

	ctor := r.vm.ToValue(constructor)
	_ = r.vm.Set("ResizeObserver", ctor)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("ResizeObserver", ctor)
		}
	}
}

// SetElementSize records an element's current size for ResizeObserver.
func (r *Runtime) SetElementSize(node *dom.Node, width, height float64) {
	state := resizeStateOf(r)
	if state == nil {
		return
	}

	state.mu.Lock()
	state.rects[node] = [2]float64{width, height}
	state.mu.Unlock()
}

// DrainResizeObservers checks for size changes and fires callbacks.
func (r *Runtime) DrainResizeObservers() {
	state := resizeStateOf(r)
	if state == nil {
		return
	}

	state.mu.Lock()
	observers := make([]*resizeObserver, len(state.observers))
	copy(observers, state.observers)
	rects := make(map[*dom.Node][2]float64)
	for k, v := range state.rects {
		rects[k] = v
	}
	state.mu.Unlock()

	for _, obs := range observers {
		obs.mu.Lock()
		if obs.disconnected {
			obs.mu.Unlock()
			continue
		}

		var entries []resizeEntry
		for target := range obs.targets {
			size, ok := rects[target]
			if !ok {
				continue
			}
			entries = append(entries, resizeEntry{
				target: target,
				width:  size[0],
				height: size[1],
				contentRect: map[string]float64{
					"x": 0, "y": 0,
					"width":  size[0],
					"height": size[1],
					"top":    0,
					"right":  size[0],
					"bottom": size[1],
					"left":   0,
				},
			})
		}

		if len(entries) == 0 {
			obs.mu.Unlock()
			continue
		}

		callback := obs.callback
		obs.mu.Unlock()

		arr := r.vm.NewArray()
		for i, entry := range entries {
			jsEntry := r.wrapResizeEntry(entry)
			_ = arr.Set(fmt.Sprintf("%d", i), jsEntry)
		}
		_, _ = callback(goja.Undefined(), arr, obs.obj)
	}
}

func (r *Runtime) wrapResizeEntry(entry resizeEntry) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.Set("target", r.wrapNode(entry.target))
	_ = obj.Set("contentRect", entry.contentRect)

	borderBox := r.vm.NewObject()
	_ = borderBox.Set("x", 0.0)
	_ = borderBox.Set("y", 0.0)
	_ = borderBox.Set("width", entry.width)
	_ = borderBox.Set("height", entry.height)
	_ = obj.Set("borderBoxSize", borderBox)

	contentBox := r.vm.NewObject()
	_ = contentBox.Set("x", 0.0)
	_ = contentBox.Set("y", 0.0)
	_ = contentBox.Set("width", entry.width)
	_ = contentBox.Set("height", entry.height)
	_ = obj.Set("contentBoxSize", contentBox)

	_ = obj.Set("inlineSize", entry.width)
	_ = obj.Set("blockSize", entry.height)

	return obj
}
