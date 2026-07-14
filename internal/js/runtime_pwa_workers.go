package js

import (
	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// ---------------------------------------------------------------------------
// WebSocket / Web Worker / ServiceWorker runtime stubs (M12.1)
//
// The engine does not implement the WebSocket, Web Worker, or
// ServiceWorker APIs. Pages that use them must still report the usage
// to the fallback layer so it can mark the page for compatibility.
// The stubs below:
//
//   1. Report the corresponding dom.UnsupportedFeatureKind via
//      reportRuntimeUnsupportedFeature (deduplicated per Runtime).
//   2. Return no-op object stubs with method placeholders so chained
//      property/method access (e.g. ws.close(), w.postMessage())
//      does not blow up the page's JS execution.
//
// These stubs run regardless of capability policy — the detection is
// the signal we want to surface, not the access denial.
// ---------------------------------------------------------------------------

// setupWebSocketAPI installs a stub WebSocket constructor on the
// global object. Constructing WebSocket via `new WebSocket(url)`
// reports FeatureWebSocket and returns a stub object whose methods
// (close, send, addEventListener) are no-ops.
func (r *Runtime) setupWebSocketAPI() {
	r.vm.Set("WebSocket", func(call goja.ConstructorCall) *goja.Object {
		r.reportRuntimeUnsupportedFeature(dom.FeatureWebSocket)
		obj := r.vm.NewObject()
		// readyState: 3 = CLOSED. The stub is always closed.
		obj.Set("readyState", int64(3))
		obj.Set("close", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		obj.Set("send", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		obj.Set("addEventListener", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		obj.Set("removeEventListener", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		obj.Set("dispatchEvent", func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(false)
		})
		return obj
	})
}

// setupWorkerAPI installs a stub Worker constructor on the global
// object. Constructing Worker via `new Worker(url)` reports
// FeatureWebWorker and returns a stub object whose methods
// (postMessage, terminate, addEventListener) are no-ops.
func (r *Runtime) setupWorkerAPI() {
	r.vm.Set("Worker", func(call goja.ConstructorCall) *goja.Object {
		r.reportRuntimeUnsupportedFeature(dom.FeatureWebWorker)
		obj := r.vm.NewObject()
		obj.Set("postMessage", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		obj.Set("terminate", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		obj.Set("addEventListener", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		obj.Set("removeEventListener", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		obj.Set("dispatchEvent", func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(false)
		})
		return obj
	})
}

// setupServiceWorkerAPI installs a stub navigator.serviceWorker
// object. Calling register(url) reports FeatureServiceWorker and
// returns a rejected promise. getRegistration returns null and
// getRegistrations returns an empty array — the standard patterns
// for an "unavailable service worker" environment.
func (r *Runtime) setupServiceWorkerAPI() {
	nav := r.vm.Get("navigator")
	if nav == nil || goja.IsUndefined(nav) {
		// setupServiceWorkerAPI must run after setupNavigatorAPI,
		// which always creates navigator. If we got here without
		// navigator, the caller misconfigured the setup order.
		return
	}
	navObj := nav.ToObject(r.vm)

	sw := r.vm.NewObject()

	sw.Set("register", func(call goja.FunctionCall) goja.Value {
		r.reportRuntimeUnsupportedFeature(dom.FeatureServiceWorker)
		// Return a rejected Promise. The Goja runtime provides
		// NewPromise for creating pending/rejected promises in
		// Go. We resolve the reject branch with a TypeError
		// that scripts can catch.
		promise, _, reject := r.vm.NewPromise()
		reject(r.vm.NewTypeError("ServiceWorker not supported"))
		return r.vm.ToValue(promise)
	})

	sw.Set("getRegistration", func(call goja.FunctionCall) goja.Value {
		// No registration present — return null per the spec.
		return goja.Null()
	})

	sw.Set("getRegistrations", func(call goja.FunctionCall) goja.Value {
		// No registrations present — return an empty array.
		return r.vm.NewArray()
	})

	navObj.Set("serviceWorker", sw)
}
