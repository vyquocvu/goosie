package js

import (
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

// Service Workers API implementation.

// ServiceWorker states.
const (
	SWParsed     = "parsed"
	SWInstalling = "installing"
	SWInstalled  = "installed"
	SWActivating = "activating"
	SWActivated  = "activated"
	SWRedundant  = "redundant"
)

// ServiceWorkerRegistration holds the state of a service worker registration.
type serviceWorkerRegistration struct {
	mu           sync.Mutex
	scope        string
	scriptURL    string
	state        string
	activeWorker *serviceWorkerObj
	installing   *serviceWorkerObj
	waiting      *serviceWorkerObj
	obj          *goja.Object
	r            *Runtime
}

// serviceWorkerObj holds the internal state of one ServiceWorker object.
type serviceWorkerObj struct {
	mu        sync.Mutex
	state     string
	scriptURL string
	obj       *goja.Object
	r         *Runtime
}

const swStateKey = "__swState__"

type swState struct {
	mu            sync.Mutex
	registrations map[string]*serviceWorkerRegistration
	// pending activations queued by background goroutines; drained on the
	// VM thread where goja objects may be touched.
	pending []swActivation
}

// swActivation records one install→activate completion. The background
// goroutine fills plain-Go state and appends this; DrainServiceWorkerEvents
// performs the goja Sets on the main goroutine.
type swActivation struct {
	reg    *serviceWorkerRegistration
	sw     *serviceWorkerObj
	regObj *goja.Object
	swObj  *goja.Object
}

func setSWState(r *Runtime, s *swState) {
	_ = r.vm.Set(swStateKey, s)
}

func swStateOf(r *Runtime) *swState {
	v := r.vm.Get(swStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	s, ok := v.Export().(*swState)
	if !ok {
		return nil
	}
	return s
}

// setupServiceWorker installs the ServiceWorker API.
func (r *Runtime) setupServiceWorker() {
	state := &swState{
		registrations: make(map[string]*serviceWorkerRegistration),
	}
	setSWState(r, state)

	// navigator.serviceWorker.register(scriptURL, options)
	navigator := r.vm.Get("navigator")
	if navigator == nil || goja.IsUndefined(navigator) {
		navigatorObj := r.vm.NewObject()
		_ = r.vm.Set("navigator", navigatorObj)
		navigator = navigatorObj
	}

	navigatorObj, ok := navigator.(*goja.Object)
	if !ok {
		return
	}

	swContainer := r.vm.NewObject()

	// register(scriptURL, options)
	_ = swContainer.Set("register", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("register requires a script URL argument"))
		}

		scriptURL := call.Arguments[0].String()
		scope := "/"
		if len(call.Arguments) >= 2 {
			opts := call.Arguments[1]
			if opts != nil && !goja.IsUndefined(opts) {
				if optsObj, ok := opts.(*goja.Object); ok {
					if s := optsObj.Get("scope"); s != nil && !goja.IsUndefined(s) {
						scope = s.String()
					}
				}
			}
		}

		reg := &serviceWorkerRegistration{
			scope:     scope,
			scriptURL: scriptURL,
			state:     SWParsed,
			r:         r,
		}

		state.mu.Lock()
		state.registrations[scope] = reg
		state.mu.Unlock()

		// Create registration JS object.
		regObj := r.vm.NewObject()
		reg.obj = regObj
		_ = regObj.Set("scope", scope)
		_ = regObj.Set("active", goja.Null())
		_ = regObj.Set("installing", goja.Null())
		_ = regObj.Set("waiting", goja.Null())

		// unregister()
		_ = regObj.Set("unregister", func(call goja.FunctionCall) goja.Value {
			state.mu.Lock()
			delete(state.registrations, scope)
			state.mu.Unlock()
			return r.vm.ToValue(true)
		})

		// update()
		_ = regObj.Set("update", func(call goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})

		// Create service worker object.
		sw := &serviceWorkerObj{
			state:     SWInstalling,
			scriptURL: scriptURL,
			r:         r,
		}

		swObj := r.vm.NewObject()
		sw.obj = swObj
		_ = swObj.Set("scriptURL", scriptURL)
		_ = swObj.Set("state", SWInstalling)

		reg.mu.Lock()
		reg.installing = sw
		reg.mu.Unlock()
		_ = regObj.Set("installing", swObj)

		// Simulate install → activate lifecycle. The background goroutine
		// touches only plain-Go fields and queues an activation; all goja
		// Sets happen in DrainServiceWorkerEvents on the VM thread.
		go func() {
			sw.mu.Lock()
			sw.state = SWInstalled
			sw.mu.Unlock()

			sw.mu.Lock()
			sw.state = SWActivating
			sw.mu.Unlock()

			sw.mu.Lock()
			sw.state = SWActivated
			sw.mu.Unlock()

			reg.mu.Lock()
			reg.activeWorker = sw
			reg.installing = nil
			reg.mu.Unlock()

			state.mu.Lock()
			state.pending = append(state.pending, swActivation{
				reg:    reg,
				sw:     sw,
				regObj: regObj,
				swObj:  swObj,
			})
			state.mu.Unlock()
		}()

		return regObj
	})

	// getRegistration(scope)
	_ = swContainer.Set("getRegistration", func(call goja.FunctionCall) goja.Value {
		scope := "/"
		if len(call.Arguments) >= 1 {
			scope = call.Arguments[0].String()
		}

		state.mu.Lock()
		reg, ok := state.registrations[scope]
		state.mu.Unlock()

		if !ok || reg.obj == nil {
			return goja.Null()
		}
		return reg.obj
	})

	// getRegistrations()
	_ = swContainer.Set("getRegistrations", func(call goja.FunctionCall) goja.Value {
		state.mu.Lock()
		regs := make([]*serviceWorkerRegistration, 0, len(state.registrations))
		for _, reg := range state.registrations {
			regs = append(regs, reg)
		}
		state.mu.Unlock()

		arr := r.vm.NewArray()
		for i, reg := range regs {
			if reg.obj != nil {
				_ = arr.Set(fmt.Sprintf("%d", i), reg.obj)
			}
		}
		return arr
	})

	// ready property (returns a promise-like)
	_ = swContainer.Set("ready", goja.Null())

	_ = navigatorObj.Set("serviceWorker", swContainer)

	// Also install on window.navigator.
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			nav := winObj.Get("navigator")
			if nav == nil || goja.IsUndefined(nav) {
				navObj := r.vm.NewObject()
				_ = winObj.Set("navigator", navObj)
				nav = navObj
			}
			if navObj, ok := nav.(*goja.Object); ok {
				_ = navObj.Set("serviceWorker", swContainer)
			}
		}
	}
}

// DrainServiceWorkerEvents applies pending install→activate transitions on
// the VM thread. Call this from Tick; background goroutines never touch goja.
func (r *Runtime) DrainServiceWorkerEvents() {
	state := swStateOf(r)
	if state == nil {
		return
	}

	state.mu.Lock()
	pending := make([]swActivation, len(state.pending))
	copy(pending, state.pending)
	state.pending = state.pending[:0]
	state.mu.Unlock()

	for _, act := range pending {
		if act.swObj != nil {
			act.sw.mu.Lock()
			st := act.sw.state
			act.sw.mu.Unlock()
			_ = act.swObj.Set("state", st)
		}
		if act.regObj != nil && act.swObj != nil {
			_ = act.regObj.Set("active", act.swObj)
			_ = act.regObj.Set("installing", goja.Null())
		}
	}
}
