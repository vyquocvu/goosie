package js

import (
	"fmt"
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// MutationObserver API implementation.

// MutationRecord represents a single DOM mutation.
type MutationRecord struct {
	Type            string // "attributes", "childList", or "characterData"
	Target          *dom.Node
	AddedNodes      []*dom.Node
	RemovedNodes    []*dom.Node
	PreviousSibling *dom.Node
	NextSibling     *dom.Node
	AttributeName   string
	OldValue        string
}

// mutationObserver holds the internal state of one MutationObserver.
type mutationObserver struct {
	mu           sync.Mutex
	callback     goja.Callable
	targets      map[*dom.Node]*mutationOptions
	records      []MutationRecord
	r            *Runtime
	disconnected bool
	obj          *goja.Object
}

type mutationOptions struct {
	attributes            bool
	childList             bool
	characterData         bool
	subtree               bool
	attributeOldValue     bool
	characterDataOldValue bool
	attributeFilter       []string
}

const mutationStateKey = "__mutationState__"

type mutationState struct {
	mu        sync.Mutex
	observers []*mutationObserver
}

func setMutationState(r *Runtime, s *mutationState) {
	_ = r.vm.Set(mutationStateKey, s)
}

func mutationStateOf(r *Runtime) *mutationState {
	v := r.vm.Get(mutationStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	s, ok := v.Export().(*mutationState)
	if !ok {
		return nil
	}
	return s
}

// setupMutationObserver installs the MutationObserver constructor.
func (r *Runtime) setupMutationObserver() {
	state := &mutationState{}
	setMutationState(r, state)

	constructor := func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("MutationObserver requires a callback argument"))
		}

		callback, ok := goja.AssertFunction(call.Arguments[0])
		if !ok {
			panic(r.vm.NewTypeError("MutationObserver callback must be a function"))
		}

		obs := &mutationObserver{
			callback: callback,
			targets:  make(map[*dom.Node]*mutationOptions),
			r:        r,
		}

		obj := call.This
		obs.obj = obj

		// observe(target, options)
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

			opts := &mutationOptions{
				childList: true, // default
			}

			if len(call.Arguments) >= 2 {
				optsArg := call.Arguments[1]
				if optsArg != nil && !goja.IsUndefined(optsArg) && !goja.IsNull(optsArg) {
					if optsObj, ok := optsArg.(*goja.Object); ok {
						if v := optsObj.Get("attributes"); v != nil && !goja.IsUndefined(v) {
							opts.attributes = v.ToBoolean()
						}
						if v := optsObj.Get("childList"); v != nil && !goja.IsUndefined(v) {
							opts.childList = v.ToBoolean()
						}
						if v := optsObj.Get("characterData"); v != nil && !goja.IsUndefined(v) {
							opts.characterData = v.ToBoolean()
						}
						if v := optsObj.Get("subtree"); v != nil && !goja.IsUndefined(v) {
							opts.subtree = v.ToBoolean()
						}
						if v := optsObj.Get("attributeOldValue"); v != nil && !goja.IsUndefined(v) {
							opts.attributeOldValue = v.ToBoolean()
						}
						if v := optsObj.Get("characterDataOldValue"); v != nil && !goja.IsUndefined(v) {
							opts.characterDataOldValue = v.ToBoolean()
						}
					}
				}
			}

			// If attributes or characterData are explicitly set, childList
			// defaults to false unless explicitly set.
			if len(call.Arguments) >= 2 {
				optsArg := call.Arguments[1]
				if optsArg != nil && !goja.IsUndefined(optsArg) {
					if optsObj, ok := optsArg.(*goja.Object); ok {
						childListVal := optsObj.Get("childList")
						if childListVal == nil || goja.IsUndefined(childListVal) {
							if opts.attributes || opts.characterData {
								opts.childList = false
							}
						}
					}
				}
			}

			obs.mu.Lock()
			obs.targets[node] = opts
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

		// disconnect()
		_ = obj.Set("disconnect", func(call goja.FunctionCall) goja.Value {
			obs.mu.Lock()
			obs.disconnected = true
			obs.targets = make(map[*dom.Node]*mutationOptions)
			obs.records = nil
			obs.mu.Unlock()
			return goja.Undefined()
		})

		// takeRecords()
		_ = obj.Set("takeRecords", func(call goja.FunctionCall) goja.Value {
			obs.mu.Lock()
			records := obs.records
			obs.records = nil
			obs.mu.Unlock()

			arr := r.vm.NewArray()
			for i, rec := range records {
				jsRec := r.wrapMutationRecord(rec)
				_ = arr.Set(fmt.Sprintf("%d", i), jsRec)
			}
			return arr
		})

		return nil
	}

	ctor := r.vm.ToValue(constructor)
	_ = r.vm.Set("MutationObserver", ctor)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("MutationObserver", ctor)
		}
	}
}

func (r *Runtime) wrapMutationRecord(rec MutationRecord) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.Set("type", rec.Type)
	_ = obj.Set("target", r.wrapNode(rec.Target))

	addedArr := r.vm.NewArray()
	for i, n := range rec.AddedNodes {
		_ = addedArr.Set(fmt.Sprintf("%d", i), r.wrapNode(n))
	}
	_ = obj.Set("addedNodes", addedArr)

	removedArr := r.vm.NewArray()
	for i, n := range rec.RemovedNodes {
		_ = removedArr.Set(fmt.Sprintf("%d", i), r.wrapNode(n))
	}
	_ = obj.Set("removedNodes", removedArr)

	if rec.PreviousSibling != nil {
		_ = obj.Set("previousSibling", r.wrapNode(rec.PreviousSibling))
	} else {
		_ = obj.Set("previousSibling", goja.Null())
	}
	if rec.NextSibling != nil {
		_ = obj.Set("nextSibling", r.wrapNode(rec.NextSibling))
	} else {
		_ = obj.Set("nextSibling", goja.Null())
	}

	if rec.AttributeName != "" {
		_ = obj.Set("attributeName", rec.AttributeName)
	}
	if rec.OldValue != "" {
		_ = obj.Set("oldValue", rec.OldValue)
	} else {
		_ = obj.Set("oldValue", goja.Null())
	}

	return obj
}

// NotifyMutation records a mutation for all observing MutationObservers.
func (r *Runtime) NotifyMutation(target *dom.Node, mutationType string, attrName string) {
	state := mutationStateOf(r)
	if state == nil {
		return
	}

	state.mu.Lock()
	observers := make([]*mutationObserver, len(state.observers))
	copy(observers, state.observers)
	state.mu.Unlock()

	for _, obs := range observers {
		obs.mu.Lock()
		if obs.disconnected {
			obs.mu.Unlock()
			continue
		}

		for node, opts := range obs.targets {
			if nodeMatches(target, node, opts.subtree) {
				rec := MutationRecord{
					Type:   mutationType,
					Target: target,
				}
				if mutationType == "attributes" {
					rec.AttributeName = attrName
				}
				obs.records = append(obs.records, rec)
				break
			}
		}
		obs.mu.Unlock()
	}
}

// DrainMutationObservers fires pending mutation records to callbacks.
func (r *Runtime) DrainMutationObservers() {
	state := mutationStateOf(r)
	if state == nil {
		return
	}

	state.mu.Lock()
	observers := make([]*mutationObserver, len(state.observers))
	copy(observers, state.observers)
	state.mu.Unlock()

	for _, obs := range observers {
		obs.mu.Lock()
		if obs.disconnected || len(obs.records) == 0 {
			obs.mu.Unlock()
			continue
		}
		records := obs.records
		obs.records = nil
		callback := obs.callback
		obs.mu.Unlock()

		arr := r.vm.NewArray()
		for i, rec := range records {
			_ = arr.Set(fmt.Sprintf("%d", i), r.wrapMutationRecord(rec))
		}
		_, _ = callback(goja.Undefined(), arr, obs.obj)
	}
}

func nodeMatches(target, observed *dom.Node, subtree bool) bool {
	if target == observed {
		return true
	}
	if !subtree {
		return false
	}
	for p := target.Parent; p != nil; p = p.Parent {
		if p == observed {
			return true
		}
	}
	return false
}
