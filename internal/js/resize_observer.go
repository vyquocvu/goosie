package js

import (
	"sort"
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type resizeBox struct {
	x, y, width, height float64
}

type resizeEntry struct {
	target         *dom.Node
	contentRect    resizeBox
	borderBoxSize  [2]float64
	contentBoxSize [2]float64
}

type resizeObserver struct {
	mu           sync.Mutex
	callback     goja.Callable
	targets      map[*dom.Node]bool
	lastSizes    map[*dom.Node][2]float64
	r            *Runtime
	disconnected bool
	obj          *goja.Object
}

const resizeStateKey = "__resizeState__"

type resizeState struct {
	mu           sync.Mutex
	observers    []*resizeObserver
	rects        map[*dom.Node][2]float64
	watched      map[*dom.Node]int
	notifyQueued bool
}

func setResizeState(r *Runtime, state *resizeState) {
	_ = r.vm.Set(resizeStateKey, state)
}

func resizeStateOf(r *Runtime) *resizeState {
	value := r.vm.Get(resizeStateKey)
	if value == nil || goja.IsUndefined(value) {
		return nil
	}
	state, ok := value.Export().(*resizeState)
	if !ok {
		return nil
	}
	return state
}

func (r *Runtime) setupResizeObserver() {
	state := &resizeState{
		rects:   make(map[*dom.Node][2]float64),
		watched: make(map[*dom.Node]int),
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
			callback:  callback,
			targets:   make(map[*dom.Node]bool),
			lastSizes: make(map[*dom.Node][2]float64),
			r:         r,
		}
		obs.obj = call.This
		_ = obs.obj.Set("observe", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(r.vm.NewTypeError("observe requires a target argument"))
			}
			target, ok := call.Arguments[0].(*goja.Object)
			if !ok {
				panic(r.vm.NewTypeError("observe target must be a Node"))
			}
			nidValue := target.Get("__nid__")
			if nidValue == nil || goja.IsUndefined(nidValue) {
				panic(r.vm.NewTypeError("observe target must be a Node"))
			}
			r.mu.Lock()
			node := r.nodeRegistry[int(nidValue.ToInteger())]
			r.mu.Unlock()
			if node == nil {
				panic(r.vm.NewTypeError("observe target is not a valid Node"))
			}
			obs.mu.Lock()
			alreadyObserved := obs.targets[node]
			obs.targets[node] = true
			obs.disconnected = false
			obs.mu.Unlock()
			state.mu.Lock()
			if !alreadyObserved {
				state.watched[node]++
			}
			found := false
			for _, registered := range state.observers {
				if registered == obs {
					found = true
					break
				}
			}
			if !found {
				state.observers = append(state.observers, obs)
			}
			state.mu.Unlock()
			if r.scriptExecutionDepth == 0 {
				r.DrainResizeObservers()
			}
			return goja.Undefined()
		})
		_ = obs.obj.Set("unobserve", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Undefined()
			}
			target, ok := call.Arguments[0].(*goja.Object)
			if !ok {
				return goja.Undefined()
			}
			nidValue := target.Get("__nid__")
			if nidValue == nil || goja.IsUndefined(nidValue) {
				return goja.Undefined()
			}
			r.mu.Lock()
			node := r.nodeRegistry[int(nidValue.ToInteger())]
			r.mu.Unlock()
			if node == nil {
				return goja.Undefined()
			}
			obs.mu.Lock()
			removed := obs.targets[node]
			delete(obs.targets, node)
			delete(obs.lastSizes, node)
			obs.mu.Unlock()
			if removed {
				releaseResizeTarget(state, node)
			}
			return goja.Undefined()
		})
		_ = obs.obj.Set("disconnect", func(call goja.FunctionCall) goja.Value {
			obs.mu.Lock()
			obs.disconnected = true
			targets := obs.targets
			obs.targets = make(map[*dom.Node]bool)
			obs.lastSizes = make(map[*dom.Node][2]float64)
			obs.mu.Unlock()
			state.mu.Lock()
			for target := range targets {
				decrementResizeTargetLocked(state, target)
			}
			for i, registered := range state.observers {
				if registered == obs {
					state.observers = append(state.observers[:i], state.observers[i+1:]...)
					break
				}
			}
			state.mu.Unlock()
			return goja.Undefined()
		})
		return nil
	}
	ctor := r.vm.ToValue(constructor)
	_ = r.vm.Set("ResizeObserver", ctor)
	if window := r.vm.Get("window"); window != nil {
		if windowObject, ok := window.(*goja.Object); ok {
			_ = windowObject.Set("ResizeObserver", ctor)
		}
	}
}

func decrementResizeTargetLocked(state *resizeState, node *dom.Node) {
	count := state.watched[node] - 1
	if count > 0 {
		state.watched[node] = count
		return
	}
	delete(state.watched, node)
	delete(state.rects, node)
}

func releaseResizeTarget(state *resizeState, node *dom.Node) {
	state.mu.Lock()
	decrementResizeTargetLocked(state, node)
	state.mu.Unlock()
}

func (r *Runtime) SetElementSize(node *dom.Node, width, height float64) {
	state := resizeStateOf(r)
	if state == nil || node == nil {
		return
	}
	state.mu.Lock()
	if state.watched[node] > 0 {
		state.rects[node] = [2]float64{width, height}
	}
	state.mu.Unlock()
}

func (r *Runtime) DrainResizeObservers() {
	if r == nil {
		return
	}
	state := resizeStateOf(r)
	if state == nil {
		return
	}
	state.mu.Lock()
	if state.notifyQueued {
		state.mu.Unlock()
		return
	}
	state.notifyQueued = true
	state.mu.Unlock()
	r.queueMicrotask(func() error {
		r.deliverResizeObservers()
		return nil
	})
}

func (r *Runtime) deliverResizeObservers() {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return
	}
	state := resizeStateOf(r)
	if state == nil {
		return
	}
	state.mu.Lock()
	state.notifyQueued = false
	observers := append([]*resizeObserver(nil), state.observers...)
	rects := make(map[*dom.Node][2]float64, len(state.rects))
	for node, size := range state.rects {
		rects[node] = size
	}
	state.mu.Unlock()
	provider := r.layoutGeometryProvider()
	for _, obs := range observers {
		obs.mu.Lock()
		if obs.disconnected || len(obs.targets) == 0 {
			obs.mu.Unlock()
			continue
		}
		targets := make([]*dom.Node, 0, len(obs.targets))
		for target := range obs.targets {
			targets = append(targets, target)
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })
		var entries []resizeEntry
		for _, target := range targets {
			contentRect, borderSize, contentSize, ok := r.resizeBoxForTarget(target, provider, rects)
			if !ok {
				continue
			}
			if last, seen := obs.lastSizes[target]; seen && last == contentSize {
				continue
			}
			obs.lastSizes[target] = contentSize
			entries = append(entries, resizeEntry{
				target:         target,
				contentRect:    contentRect,
				borderBoxSize:  borderSize,
				contentBoxSize: contentSize,
			})
		}
		if len(entries) == 0 {
			obs.mu.Unlock()
			continue
		}
		callback := obs.callback
		obs.mu.Unlock()
		array := r.vm.NewArray()
		for i, entry := range entries {
			_ = array.Set(intToString(i), r.wrapResizeEntry(entry))
		}
		if _, err := callback(obs.obj, array, obs.obj); err != nil {
			r.reportWindowException(err)
		}
	}
}

func (r *Runtime) resizeBoxForTarget(target *dom.Node, provider LayoutGeometryProvider, rects map[*dom.Node][2]float64) (resizeBox, [2]float64, [2]float64, bool) {
	if provider != nil {
		if info, ok := provider(target); ok {
			borderSize := [2]float64{max64(info.Width, 0), max64(info.Height, 0)}
			contentSize := [2]float64{
				max64(info.Width-info.BorderLeft-info.BorderRight-info.PaddingLeft-info.PaddingRight, 0),
				max64(info.Height-info.BorderTop-info.BorderBottom-info.PaddingTop-info.PaddingBottom, 0),
			}
			return resizeBox{
				x: info.PaddingLeft, y: info.PaddingTop,
				width: contentSize[0], height: contentSize[1],
			}, borderSize, contentSize, true
		}
	}
	if size, ok := rects[target]; ok {
		return resizeBox{width: size[0], height: size[1]}, size, size, true
	}
	return resizeBox{}, [2]float64{}, [2]float64{}, false
}

func (r *Runtime) wrapResizeEntry(entry resizeEntry) *goja.Object {
	object := r.vm.NewObject()
	_ = object.Set("target", r.wrapNode(entry.target))
	_ = object.Set("contentRect", r.newDOMRect(entry.contentRect.x, entry.contentRect.y, entry.contentRect.width, entry.contentRect.height))
	_ = object.Set("borderBoxSize", r.vm.NewArray(r.newResizeObserverSize(entry.borderBoxSize)))
	_ = object.Set("contentBoxSize", r.vm.NewArray(r.newResizeObserverSize(entry.contentBoxSize)))
	return object
}

func (r *Runtime) newResizeObserverSize(size [2]float64) *goja.Object {
	object := r.vm.NewObject()
	_ = object.Set("inlineSize", size[0])
	_ = object.Set("blockSize", size[1])
	return object
}
