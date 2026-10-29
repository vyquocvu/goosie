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
	Type               string // "attributes", "childList", or "characterData"
	Target             *dom.Node
	AddedNodes         []*dom.Node
	RemovedNodes       []*dom.Node
	PreviousSibling    *dom.Node
	NextSibling        *dom.Node
	AttributeName      string
	AttributeNamespace string
	OldValue           *string
}

// mutationObserver holds the internal state of one MutationObserver.
type mutationObserver struct {
	mu            sync.Mutex
	callback      goja.Callable
	callbackRealm *Runtime
	targets       map[*dom.Node]*mutationOptions
	records       []MutationRecord
	r             *Runtime
	root          *Runtime
	disconnected  bool
	obj           *goja.Object
}

type mutationOptions struct {
	attributes            bool
	childList             bool
	characterData         bool
	subtree               bool
	attributeOldValue     bool
	characterDataOldValue bool
	attributeFilter       []string
	attributeFilterSet    bool
}

const mutationStateKey = "__mutationState__"

type mutationState struct {
	mu           sync.Mutex
	observers    []*mutationObserver
	registry     []*mutationObserver
	notifyQueued bool
	recordProto  *goja.Object
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
	state := &mutationState{recordProto: r.vm.NewObject()}
	setMutationState(r, state)
	mutationRecordCtor := r.defineConstructor("MutationRecord", state.recordProto)
	_ = r.vm.Set("MutationRecord", mutationRecordCtor)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("MutationRecord", mutationRecordCtor)
		}
	}

	constructor := func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("MutationObserver requires a callback argument"))
		}

		callback, ok := goja.AssertFunction(call.Arguments[0])
		if !ok {
			panic(r.vm.NewTypeError("MutationObserver callback must be a function"))
		}
		root := r
		for root.iframeParent != nil {
			root = root.iframeParent
		}

		obs := &mutationObserver{
			callback:      callback,
			callbackRealm: r,
			targets:       make(map[*dom.Node]*mutationOptions),
			r:             r,
			root:          root,
		}
		if callbackObject, ok := call.Arguments[0].(*goja.Object); ok {
			if realm := r.iframeImportedFunctionRealms[callbackObject]; realm != nil {
				obs.callbackRealm = realm
			}
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

			node := r.getNode(targetObj)
			if node == nil {
				panic(r.vm.NewTypeError("observe target is not a valid Node"))
			}

			opts := &mutationOptions{}
			attributesSet, characterDataSet := false, false
			attributeOldValueSet, characterDataOldValueSet := false, false

			if len(call.Arguments) >= 2 {
				optsArg := call.Arguments[1]
				if optsArg != nil && !goja.IsUndefined(optsArg) && !goja.IsNull(optsArg) {
					if optsObj, ok := optsArg.(*goja.Object); ok {
						if v := optsObj.Get("attributes"); v != nil && !goja.IsUndefined(v) {
							attributesSet = true
							opts.attributes = v.ToBoolean()
						}
						if v := optsObj.Get("childList"); v != nil && !goja.IsUndefined(v) {
							opts.childList = v.ToBoolean()
						}
						if v := optsObj.Get("characterData"); v != nil && !goja.IsUndefined(v) {
							characterDataSet = true
							opts.characterData = v.ToBoolean()
						}
						if v := optsObj.Get("subtree"); v != nil && !goja.IsUndefined(v) {
							opts.subtree = v.ToBoolean()
						}
						if v := optsObj.Get("attributeOldValue"); v != nil && !goja.IsUndefined(v) {
							attributeOldValueSet = true
							opts.attributeOldValue = v.ToBoolean()
						}
						if v := optsObj.Get("characterDataOldValue"); v != nil && !goja.IsUndefined(v) {
							characterDataOldValueSet = true
							opts.characterDataOldValue = v.ToBoolean()
						}
						if v := optsObj.Get("attributeFilter"); v != nil && !goja.IsUndefined(v) {
							opts.attributeFilterSet = true
							filter, ok := v.(*goja.Object)
							if !ok {
								panic(r.vm.NewTypeError("attributeFilter must be a sequence"))
							}
							length := filter.Get("length").ToInteger()
							for i := int64(0); i < length; i++ {
								opts.attributeFilter = append(opts.attributeFilter, filter.Get(fmt.Sprint(i)).String())
							}
						}
					}
				}
			}
			if !attributesSet && (attributeOldValueSet || opts.attributeFilterSet) {
				opts.attributes = true
			}
			if !characterDataSet && characterDataOldValueSet {
				opts.characterData = true
			}
			if !opts.childList && !opts.attributes && !opts.characterData {
				panic(r.vm.NewTypeError("At least one of childList, attributes, or characterData must be true"))
			}
			if !opts.attributes && (opts.attributeOldValue || opts.attributeFilterSet) {
				panic(r.vm.NewTypeError("attributeOldValue or attributeFilter requires attributes"))
			}
			if !opts.characterData && opts.characterDataOldValue {
				panic(r.vm.NewTypeError("characterDataOldValue requires characterData"))
			}

			obs.mu.Lock()
			obs.disconnected = false
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
			if rootState := mutationStateOf(obs.root); rootState != nil {
				rootState.mu.Lock()
				found = false
				for _, registered := range rootState.registry {
					if registered == obs {
						found = true
						break
					}
				}
				if !found {
					rootState.registry = append(rootState.registry, obs)
				}
				rootState.mu.Unlock()
			}

			return goja.Undefined()
		})

		// disconnect()
		_ = obj.Set("disconnect", func(call goja.FunctionCall) goja.Value {
			obs.mu.Lock()
			obs.disconnected = true
			obs.targets = make(map[*dom.Node]*mutationOptions)
			obs.records = nil
			obs.mu.Unlock()
			if rootState := mutationStateOf(obs.root); rootState != nil {
				rootState.mu.Lock()
				for i, registered := range rootState.registry {
					if registered == obs {
						rootState.registry = append(rootState.registry[:i], rootState.registry[i+1:]...)
						break
					}
				}
				rootState.mu.Unlock()
			}
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
	if ctorObject, ok := ctor.(*goja.Object); ok {
		// Goja's native constructor functions do not always carry an ECMAScript
		// prototype object. MutationObserver is an interface constructor, so
		// provide its Web IDL prototype explicitly; same-origin WindowProxy
		// construction relies on this property when it creates a bridge.
		_ = ctorObject.Set("prototype", r.vm.NewObject())
	}
	_ = r.vm.Set("MutationObserver", ctor)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("MutationObserver", ctor)
		}
	}
}

func (r *Runtime) wrapMutationRecord(rec MutationRecord) *goja.Object {
	obj := r.vm.NewObject()
	if state := mutationStateOf(r); state != nil && state.recordProto != nil {
		_ = obj.SetPrototype(state.recordProto)
	}
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

	if rec.Type == "attributes" {
		_ = obj.Set("attributeName", rec.AttributeName)
		if rec.AttributeNamespace == "" {
			_ = obj.Set("attributeNamespace", goja.Null())
		} else {
			_ = obj.Set("attributeNamespace", rec.AttributeNamespace)
		}
	} else {
		_ = obj.Set("attributeName", goja.Null())
		_ = obj.Set("attributeNamespace", goja.Null())
	}
	if rec.OldValue != nil {
		_ = obj.Set("oldValue", *rec.OldValue)
	} else {
		_ = obj.Set("oldValue", goja.Null())
	}

	return obj
}

// NotifyMutation records a mutation for all observing MutationObservers.
func (r *Runtime) NotifyMutation(target *dom.Node, mutationType string, attrName string) {
	r.queueMutationRecord(MutationRecord{Type: mutationType, Target: target, AttributeName: attrName})
}

func (r *Runtime) notifyAttributeMutation(target *dom.Node, namespace, name string, oldValue *string) {
	r.queueMutationRecord(MutationRecord{Type: "attributes", Target: target, AttributeName: name, AttributeNamespace: namespace, OldValue: oldValue})
}

type childRemovalSnapshot struct {
	parent, node, previous, next *dom.Node
}

func snapshotChildRemovals(nodes []*dom.Node) []childRemovalSnapshot {
	var snapshots []childRemovalSnapshot
	for _, node := range nodes {
		if node != nil && node.Parent != nil {
			snapshots = append(snapshots, childRemovalSnapshot{parent: node.Parent, node: node, previous: node.PrevSibling, next: node.NextSibling})
		}
	}
	return snapshots
}

func (r *Runtime) queueChildRemoval(parent, node *dom.Node, previous, next *dom.Node) {
	r.queueMutationRecord(MutationRecord{Type: "childList", Target: parent, RemovedNodes: []*dom.Node{node}, PreviousSibling: previous, NextSibling: next})
}

func (r *Runtime) queueInsertionMutation(parent *dom.Node, inserted []*dom.Node, removals []childRemovalSnapshot) {
	for i := 0; i < len(removals); {
		first := removals[i]
		removed := []*dom.Node{first.node}
		last := first
		j := i + 1
		if first.parent.Type == dom.NodeDocumentFragment {
			for j < len(removals) && removals[j].parent == first.parent {
				removed = append(removed, removals[j].node)
				last = removals[j]
				j++
			}
		}
		r.queueMutationRecord(MutationRecord{Type: "childList", Target: first.parent, RemovedNodes: removed, PreviousSibling: first.previous, NextSibling: last.next})
		i = j
	}
	if len(inserted) == 0 {
		return
	}
	r.queueMutationRecord(MutationRecord{Type: "childList", Target: parent, AddedNodes: append([]*dom.Node(nil), inserted...), PreviousSibling: inserted[0].PrevSibling, NextSibling: inserted[len(inserted)-1].NextSibling})
}

func (r *Runtime) queueMutationRecord(record MutationRecord) {
	if r == nil || record.Target == nil {
		return
	}
	root := r
	for root.iframeParent != nil {
		root = root.iframeParent
	}
	rootState := mutationStateOf(root)
	if rootState == nil {
		return
	}
	rootState.mu.Lock()
	observers := append([]*mutationObserver(nil), rootState.registry...)
	rootState.mu.Unlock()
	for _, obs := range observers {
		if !queueMutationRecordForObserver(obs, record) {
			continue
		}
	}
}

func queueMutationRecordForObserver(obs *mutationObserver, record MutationRecord) bool {
	if obs == nil || obs.r == nil {
		return false
	}
	obs.mu.Lock()
	if obs.disconnected {
		obs.mu.Unlock()
		return false
	}

	var matched, wantOldValue bool
	for node, opts := range obs.targets {
		if !nodeMatches(record.Target, node, opts.subtree) {
			continue
		}
		if record.Type == "childList" && !opts.childList || record.Type == "attributes" && !opts.attributes || record.Type == "characterData" && !opts.characterData {
			continue
		}
		if record.Type == "attributes" && opts.attributeFilterSet {
			if record.AttributeNamespace != "" {
				continue
			}
			found := false
			for _, name := range opts.attributeFilter {
				if name == record.AttributeName {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		matched = true
		if record.Type == "attributes" && opts.attributeOldValue || record.Type == "characterData" && opts.characterDataOldValue {
			wantOldValue = true
		}
	}
	if !matched {
		obs.mu.Unlock()
		return false
	}
	rec := record
	if !wantOldValue {
		rec.OldValue = nil
	}
	obs.records = append(obs.records, rec)
	obs.mu.Unlock()

	state := mutationStateOf(obs.r)
	if state == nil {
		return false
	}
	state.mu.Lock()
	shouldSchedule := !state.notifyQueued
	if shouldSchedule {
		state.notifyQueued = true
	}
	state.mu.Unlock()
	if shouldSchedule {
		obs.r.queueMicrotask(func() error {
			state.mu.Lock()
			state.notifyQueued = false
			state.mu.Unlock()
			obs.r.DrainMutationObservers()
			return nil
		})
	}
	return true
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
		if _, err := callback(obs.obj, arr, obs.obj); err != nil {
			realm := obs.callbackRealm
			if realm == nil {
				realm = r
			}
			realm.reportWindowExceptionFrom(r, err)
		}
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
