package js

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// Fetch resource limits.
const (
	// MaxConcurrentFetches bounds the number of in-flight fetch requests per
	// document. A page that fires twenty fetches at once would otherwise open
	// twenty connections; the limit keeps one misbehaving tab from starving
	// the others.
	MaxConcurrentFetches = 6

	// MaxFetchBodyBytes bounds the request body sent through fetch. The
	// response body limit (MaxResponseBytes = 8 MiB) lives in the net
	// package and is enforced by the HTTP client.
	MaxFetchBodyBytes = 1 << 20

	// FetchTimeout bounds one fetch request.
	FetchTimeout = 30 * time.Second
)

// ErrFetchNotAvailable reports that fetch was called without an HTTP client.
var ErrFetchNotAvailable = fmt.Errorf("fetch is not available")

// ErrTooManyConcurrentFetches reports that the per-document fetch limit is
// already reached.
var ErrTooManyConcurrentFetches = fmt.Errorf("too many concurrent fetches")

// HTTPFetcher is the interface the engine's HTTP client satisfies for fetch.
// It is defined here so that internal/js does not import internal/net, which
// the archtest import graph forbids. The engine wires the concrete client in
// at pipeline assembly time.
type HTTPFetcher interface {
	Fetch(url string, method string, headers map[string]string, body []byte) (*FetchResponse, error)
}

// FetchOptions mirrors the JS RequestInit dictionary. Only the fields the
// engine currently supports are present; the rest are silently ignored so a
// script that passes them does not get a confusing error.
type FetchOptions struct {
	Method  string
	Headers map[string]string
	Body    string
}

// FetchResponse is the result of a fetch, exposed to JS as a Response-like
// object. The fields are populated by the HTTP client and copied onto the JS
// response wrapper by makeResponseObject.
type FetchResponse struct {
	Status     int
	StatusText string
	Headers    map[string]string
	Body       []byte
	URL        string
	OK         bool
}

// ---------------------------------------------------------------------------
// Per-Runtime fetch state.
//
// internal/js can only import internal/dom (archtest), so a *Runtime cannot
// carry a *fetchState field without modifying js.go. Instead, the fetchState
// is stored as a hidden property on the per-Runtime goja VM. Each VM belongs
// to exactly one document, so this is per-document state with no process-wide
// mutable globals.
// ---------------------------------------------------------------------------

const fetchStateKey = "__fetchState__"

func setFetchState(r *Runtime, fs *fetchState) {
	_ = r.vm.Set(fetchStateKey, fs)
}

func fetchOf(r *Runtime) *fetchState {
	v := r.vm.Get(fetchStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	fs, ok := v.Export().(*fetchState)
	if !ok {
		return nil
	}
	return fs
}

// ---------------------------------------------------------------------------
// Internal types.
// ---------------------------------------------------------------------------

// fetchCallback is one pending callback waiting for DrainFetchCallbacks to
// invoke it on the main goroutine. The goroutine that completes the HTTP
// request fills in the plain-Go fields; the drain step creates goja values
// and calls the JS function, keeping all VM access on one thread.
type fetchCallback struct {
	fn       goja.Value   // the JS function to call
	isReject bool         // true → call as onRejected, false → onFulfilled
	arg      interface{}  // *FetchResponse for resolve, string for reject
	chain    *thenableObj // non-nil when this callback feeds a chained thenable
}

// thenableObj holds the per-promise callback state. A thenable is the
// callback-based substitute for a Promise in goja, which has no native
// Promise support. Each call to .then() or .catch() returns a new thenable
// so that chaining compiles, and return-value propagation between chained
// calls is handled by propagateToChain.
type thenableObj struct {
	r        *Runtime
	obj      *goja.Object // the JS object with .then() and .catch()
	mu       sync.Mutex   // guards resolved, rejected, result, thenCB, catchCB, chain
	resolved bool
	rejected bool
	result   interface{} // goja.Value once settled

	thenCB  goja.Value
	catchCB goja.Value

	// Chained thenable: set by then/catch to receive the outcome of the
	// callback's return value.
	chain *thenableObj
}

// fetchState is the per-Runtime bookkeeping for the fetch subsystem.
type fetchState struct {
	client    HTTPFetcher
	mu        sync.Mutex
	active    int
	callbacks []fetchCallback
}

// ---------------------------------------------------------------------------
// Setup.
// ---------------------------------------------------------------------------

// setupFetch registers the fetch() function on both the global scope and the
// window object. It replaces the stub installed by setupUnsupportedAPIs. When
// client is nil the function is left in a form that rejects every call with
// "fetch is not available".
func (r *Runtime) setupFetch(client HTTPFetcher) {
	if client == nil {
		r.installFetchWrapper()
		return
	}

	fs := &fetchState{client: client}
	setFetchState(r, fs)

	fetchFn := func(call goja.FunctionCall) goja.Value {
		return r.doFetch(call)
	}

	_ = r.vm.Set("fetch", fetchFn)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("fetch", fetchFn)
		}
	}
}

// installFetchWrapper replaces the throwing stub with a JS wrapper that
// returns a thenable which immediately rejects with "fetch is not available".
// This is the path taken when no HTTP client was supplied.
func (r *Runtime) installFetchWrapper() {
	_, _ = r.vm.RunString(`
		(function() {
			function fetchNoClient() {
				var t = {};
				t.then = function(onOK, onErr) {
					if (typeof onErr === 'function') {
						setTimeout(function() { onErr("fetch is not available"); }, 0);
					}
					var inner = {};
					inner.then = function() { return inner; };
					inner.catch = function() { return inner; };
					return inner;
				};
				t.catch = function(onErr) {
					return t.then(undefined, onErr);
				};
				return t;
			}
			fetch = fetchNoClient;
			window.fetch = fetchNoClient;
		})();
	`)
}

// ---------------------------------------------------------------------------
// Option parsing and URL resolution.
// ---------------------------------------------------------------------------

// parseFetchOpts extracts a FetchOptions from the optional second argument to
// fetch(url, init). A missing or non-object argument yields the zero value,
// which doFetch interprets as GET with no headers and no body.
func parseFetchOpts(vm *goja.Runtime, val goja.Value) FetchOptions {
	var opts FetchOptions
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return opts
	}
	obj, ok := val.(*goja.Object)
	if !ok {
		return opts
	}
	if v := obj.Get("method"); v != nil && !goja.IsUndefined(v) {
		opts.Method = strings.ToUpper(v.String())
	}
	if opts.Method == "" {
		opts.Method = "GET"
	}
	if v := obj.Get("body"); v != nil && !goja.IsUndefined(v) {
		opts.Body = v.String()
	}
	if v := obj.Get("headers"); v != nil && !goja.IsUndefined(v) {
		if hObj, ok := v.(*goja.Object); ok {
			opts.Headers = make(map[string]string)
			for _, key := range hObj.Keys() {
				hv := hObj.Get(key)
				if hv != nil && !goja.IsUndefined(hv) {
					opts.Headers[key] = hv.String()
				}
			}
		}
	}
	return opts
}

// resolveFetchURL resolves a possibly-relative URL against the document URL
// from Options.URL. Absolute URLs pass through; relative ones are resolved
// with the document URL as the base. An unparseable result falls back to the
// raw input so the HTTP client sees the original string and can return its
// own error.
func (r *Runtime) resolveFetchURL(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" {
		return raw
	}
	base := r.opts.URL
	if base == "" {
		return raw
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return raw
	}
	resolved := baseURL.ResolveReference(&url.URL{Path: raw})
	return resolved.String()
}

// ---------------------------------------------------------------------------
// Fetch execution.
// ---------------------------------------------------------------------------

// doFetch is the Go side of fetch(url, opts). It validates inputs, enforces
// the concurrent limit, launches the request goroutine, and returns a
// thenable JS object.
func (r *Runtime) doFetch(call goja.FunctionCall) goja.Value {
	fs := fetchOf(r)
	if fs == nil || fs.client == nil {
		return r.makeRejectThenable("fetch is not available")
	}

	if len(call.Arguments) < 1 {
		return r.makeRejectThenable("fetch requires a URL argument")
	}
	rawURL := call.Arguments[0].String()

	resolvedURL := r.resolveFetchURL(rawURL)

	var opts FetchOptions
	if len(call.Arguments) >= 2 {
		opts = parseFetchOpts(r.vm, call.Arguments[1])
	}

	// Enforce request body limit before handing bytes to the network.
	if len(opts.Body) > MaxFetchBodyBytes {
		return r.makeRejectThenable("request body exceeds limit")
	}

	fs.mu.Lock()
	if fs.active >= MaxConcurrentFetches {
		fs.mu.Unlock()
		return r.makeRejectThenable("too many concurrent fetches")
	}
	fs.active++
	fs.mu.Unlock()

	t := r.newThenable()

	go r.runFetch(resolvedURL, opts, t)

	return t.obj
}

// runFetch executes on a dedicated goroutine. It calls the HTTP client and
// queues exactly one callback (resolve or reject) for DrainFetchCallbacks to
// invoke on the main goroutine. All goja interaction happens in the drain
// step, not here.
func (r *Runtime) runFetch(fetchURL string, opts FetchOptions, t *thenableObj) {
	fs := fetchOf(r)
	if fs == nil {
		return
	}
	defer func() {
		fs.mu.Lock()
		fs.active--
		fs.mu.Unlock()
	}()

	resp, err := fs.client.Fetch(fetchURL, opts.Method, opts.Headers, []byte(opts.Body))

	t.mu.Lock()
	thenCB := t.thenCB
	catchCB := t.catchCB
	t.mu.Unlock()

	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err != nil {
		fs.callbacks = append(fs.callbacks, fetchCallback{
			fn:       catchCB,
			isReject: true,
			arg:      err.Error(),
			chain:    t,
		})
		return
	}

	// Per the Fetch spec, non-OK responses still resolve (not reject); the
	// caller inspects response.ok / response.status to decide.
	fs.callbacks = append(fs.callbacks, fetchCallback{
		fn:       thenCB,
		isReject: false,
		arg:      resp,
		chain:    t,
	})
}

// ---------------------------------------------------------------------------
// Thenable construction and chaining.
// ---------------------------------------------------------------------------

// newThenable creates a JS object with .then() and .catch() methods and
// returns the Go-side bookkeeping handle.
func (r *Runtime) newThenable() *thenableObj {
	t := &thenableObj{r: r}
	obj := r.vm.NewObject()

	_ = obj.Set("then", func(call goja.FunctionCall) goja.Value {
		var onOK, onErr goja.Value
		if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Arguments[0]) {
			onOK = call.Arguments[0]
		}
		if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Arguments[1]) {
			onErr = call.Arguments[1]
		}

		chain := r.newThenable()

		t.mu.Lock()
		t.thenCB = onOK
		t.catchCB = onErr
		t.chain = chain
		resolved := t.resolved
		rejected := t.rejected
		result := t.result
		t.mu.Unlock()

		// If the promise already settled before .then() was called, the
		// callback was never queued. Fire it now so late-attached
		// listeners still see the result.
		if resolved && onOK != nil {
			r.invokeResolvedCallback(onOK, false, result, chain)
		} else if rejected && onErr != nil {
			r.invokeResolvedCallback(onErr, true, result, chain)
		}

		return chain.obj
	})

	_ = obj.Set("catch", func(call goja.FunctionCall) goja.Value {
		var onErr goja.Value
		if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Arguments[0]) {
			onErr = call.Arguments[0]
		}

		chain := r.newThenable()

		t.mu.Lock()
		t.catchCB = onErr
		t.chain = chain
		rejected := t.rejected
		result := t.result
		t.mu.Unlock()

		if rejected && onErr != nil {
			r.invokeResolvedCallback(onErr, true, result, chain)
		}

		return chain.obj
	})

	t.obj = obj
	return t
}

// invokeResolvedCallback fires a callback for a thenable that already settled
// before .then()/.catch() was attached. The response/error is wrapped in a
// fresh JS value on the main goroutine.
func (r *Runtime) invokeResolvedCallback(fn goja.Value, isReject bool, result interface{}, chain *thenableObj) {
	f, ok := goja.AssertFunction(fn)
	if !ok {
		return
	}
	var arg goja.Value
	if resp, ok := result.(*FetchResponse); ok && !isReject {
		arg = r.makeResponseObject(resp)
	} else if msg, ok := result.(string); ok && isReject {
		arg = r.vm.ToValue(msg)
	} else if v, ok := result.(goja.Value); ok {
		arg = v
	}
	ret, _ := f(nil, arg)
	if chain != nil {
		r.propagateToChain(ret, chain)
	}
}

// propagateToChain feeds a callback's return value into the chained thenable.
// If the return is itself a thenable, the chain waits for it; otherwise the
// chain resolves immediately.
func (r *Runtime) propagateToChain(ret goja.Value, chain *thenableObj) {
	if ret == nil || goja.IsUndefined(ret) {
		return
	}
	if obj, ok := ret.(*goja.Object); ok {
		if thenFn := obj.Get("then"); thenFn != nil {
			if f, ok := goja.AssertFunction(thenFn); ok {
				_, _ = f(obj, r.vm.ToValue(func(v goja.Value) {
					chain.resolve(v)
				}), r.vm.ToValue(func(v goja.Value) {
					chain.reject(v)
				}))
				return
			}
		}
	}
	chain.resolve(ret)
}

// resolve marks the thenable as fulfilled and invokes the stored callback if
// one was registered. Called from DrainFetchCallbacks on the main goroutine.
func (t *thenableObj) resolve(val goja.Value) {
	t.resolved = true
	t.result = val
	if t.thenCB != nil {
		if f, ok := goja.AssertFunction(t.thenCB); ok {
			ret, _ := f(nil, val)
			if t.chain != nil {
				t.r.propagateToChain(ret, t.chain)
			}
		}
	}
}

// reject marks the thenable as rejected and invokes the stored catch callback
// if one was registered. Called from DrainFetchCallbacks on the main
// goroutine.
func (t *thenableObj) reject(val goja.Value) {
	t.rejected = true
	t.result = val
	if t.catchCB != nil {
		if f, ok := goja.AssertFunction(t.catchCB); ok {
			ret, _ := f(nil, val)
			if t.chain != nil {
				t.r.propagateToChain(ret, t.chain)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Response object.
// ---------------------------------------------------------------------------

// makeResponseObject builds a JS Response-like object from a FetchResponse.
// The object exposes the standard properties (status, statusText, ok, url,
// headers) and the body-consumption methods text() and json(), each of which
// returns a thenable so callers can use .then() uniformly.
func (r *Runtime) makeResponseObject(resp *FetchResponse) *goja.Object {
	obj := r.vm.NewObject()

	_ = obj.Set("status", resp.Status)
	_ = obj.Set("statusText", resp.StatusText)
	_ = obj.Set("ok", resp.OK)
	_ = obj.Set("url", resp.URL)
	bodyStr := string(resp.Body)

	// Headers: expose as a plain object with lowercase keys and a get()
	// method, matching the subset of the Headers interface scripts rely on.
	headersObj := r.vm.NewObject()
	if resp.Headers != nil {
		for k, v := range resp.Headers {
			_ = headersObj.Set(strings.ToLower(k), v)
		}
	}
	_ = headersObj.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		key := strings.ToLower(call.Arguments[0].String())
		v := headersObj.Get(key)
		if v == nil || goja.IsUndefined(v) {
			return goja.Null()
		}
		return v
	})
	_ = obj.Set("headers", headersObj)

	// text() returns a thenable that resolves with the body string.
	_ = obj.Set("text", func(call goja.FunctionCall) goja.Value {
		t := r.newThenable()
		t.resolve(r.vm.ToValue(bodyStr))
		return t.obj
	})

	// json() returns a thenable that resolves with the parsed body or
	// rejects with a syntax error message.
	_ = obj.Set("json", func(call goja.FunctionCall) goja.Value {
		t := r.newThenable()
		var parsed interface{}
		if err := json.Unmarshal([]byte(bodyStr), &parsed); err != nil {
			t.reject(r.vm.ToValue("JSON parse error: " + err.Error()))
		} else {
			t.resolve(r.vm.ToValue(parsed))
		}
		return t.obj
	})

	return obj
}

// ---------------------------------------------------------------------------
// Callback queue and drain.
// ---------------------------------------------------------------------------

// makeRejectThenable creates a thenable that is already rejected with the
// given error message. The catch callback fires on the next DrainFetchCallbacks
// call.
func (r *Runtime) makeRejectThenable(msg string) goja.Value {
	t := r.newThenable()
	// Mark the thenable as rejected immediately so that .catch() calls
	// that arrive before DrainFetchCallbacks can still fire.
	t.rejected = true
	t.result = r.vm.ToValue(msg)

	fs := fetchOf(r)
	if fs == nil {
		// No fetch state at all: the thenable is already settled, so
		// any .catch() attached later will fire via invokeResolvedCallback.
		return t.obj
	}

	fs.mu.Lock()
	fs.callbacks = append(fs.callbacks, fetchCallback{
		fn:       t.catchCB,
		isReject: true,
		arg:      msg,
		chain:    t,
	})
	fs.mu.Unlock()

	return t.obj
}

// DrainFetchCallbacks invokes every pending fetch callback on the current
// goroutine. It must be called from the same goroutine that calls Run and
// Tick, because it creates goja values and calls JS functions.
//
// In wave 2 this will be folded into Tick so that a single call drains both
// timers and fetch callbacks. Until then, hosts call it explicitly after
// allowing time for fetch goroutines to complete.
func (r *Runtime) DrainFetchCallbacks() {
	fs := fetchOf(r)
	if fs == nil {
		return
	}
	for {
		fs.mu.Lock()
		if len(fs.callbacks) == 0 {
			fs.mu.Unlock()
			return
		}
		cbs := make([]fetchCallback, len(fs.callbacks))
		copy(cbs, fs.callbacks)
		fs.callbacks = fs.callbacks[:0]
		fs.mu.Unlock()

		for _, cb := range cbs {
			r.invokeOneCallback(cb)
		}
	}
}

// invokeOneCallback dispatches a single fetch callback. It builds the JS
// argument from the plain-Go data, calls the function, and propagates the
// return value to a chained thenable if one exists.
func (r *Runtime) invokeOneCallback(cb fetchCallback) {
	if cb.fn == nil || goja.IsUndefined(cb.fn) {
		// No callback registered. If there is a chain, propagate the raw
		// value so the chain does not stall.
		if cb.chain != nil && !cb.isReject {
			if resp, ok := cb.arg.(*FetchResponse); ok {
				cb.chain.resolve(r.makeResponseObject(resp))
			}
		}
		return
	}
	f, ok := goja.AssertFunction(cb.fn)
	if !ok {
		return
	}

	var arg goja.Value
	if resp, ok := cb.arg.(*FetchResponse); ok {
		arg = r.makeResponseObject(resp)
	} else if msg, ok := cb.arg.(string); ok {
		arg = r.vm.ToValue(msg)
	}

	ret, _ := f(nil, arg)

	if cb.chain != nil {
		r.propagateToChain(ret, cb.chain)
	}
}
