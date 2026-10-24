package js

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// XHR readyState constants per the spec.
const (
	xhrUnsent = iota
	xhrOpened
	xhrHeadersReceived
	xhrLoading
	xhrDone
)

// MaxConcurrentXHR bounds in-flight XMLHttpRequests per document, mirroring
// MaxConcurrentFetches: a page that sprays beacons cannot pile goroutines
// without bound.
const MaxConcurrentXHR = 6

// xhrStateKey is the hidden VM property holding the per-Runtime XHR bookkeeping.
// internal/js can only import internal/dom (archtest), so per-subsystem state
// rides on the VM exactly like fetchState does.
const xhrStateKey = "__xhrState__"

type xhrBookkeeping struct {
	client HTTPFetcher
	mu     sync.Mutex
	active int
}

func setXHRState(r *Runtime, st *xhrBookkeeping) {
	_ = r.vm.Set(xhrStateKey, st)
}

func xhrOf(r *Runtime) *xhrBookkeeping {
	v := r.vm.Get(xhrStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	st, ok := v.Export().(*xhrBookkeeping)
	if !ok {
		return nil
	}
	return st
}

// xhrObject is one request's plain-Go state. The mutex guards everything the
// network goroutine and the VM thread share; goja values are only touched on
// the VM thread (inside queueMicrotask jobs or synchronous methods).
type xhrObject struct {
	r   *Runtime
	obj *goja.Object

	mu           sync.Mutex
	method       string
	url          string
	reqHeaders   map[string]string
	opened       bool
	sent         bool
	settled      bool // completion/abort/timeout claimed; late arrivals no-op
	aborted      bool
	timedOut     bool
	state        int
	status       int
	statusText   string
	respHeaders  map[string]string
	body         []byte
	responseType string
	timeoutMS    int64
	listeners    map[string][]goja.Value
	timer        *time.Timer
}

// setupXHR installs the XMLHttpRequest constructor. The transport reuses the
// fetch HTTPFetcher; without one, send() still exists and fails each request
// through the error event, so feature-detecting pages never see a missing
// constructor.
func (r *Runtime) setupXHR(client HTTPFetcher) {
	setXHRState(r, &xhrBookkeeping{client: client})

	ctor := func(call goja.ConstructorCall) *goja.Object {
		return r.constructXHR(call)
	}
	ctorObj := r.vm.ToValue(ctor).(*goja.Object)
	setXHRConsts(ctorObj)
	_ = r.vm.Set("XMLHttpRequest", ctorObj)
}

func setXHRConsts(obj *goja.Object) {
	_ = obj.Set("UNSENT", xhrUnsent)
	_ = obj.Set("OPENED", xhrOpened)
	_ = obj.Set("HEADERS_RECEIVED", xhrHeadersReceived)
	_ = obj.Set("LOADING", xhrLoading)
	_ = obj.Set("DONE", xhrDone)
}

func getXHR(v goja.Value) *xhrObject {
	obj, ok := v.(*goja.Object)
	if !ok {
		return nil
	}
	x, ok := obj.Get("__xhr__").Export().(*xhrObject)
	if !ok {
		return nil
	}
	return x
}

func (r *Runtime) constructXHR(call goja.ConstructorCall) *goja.Object {
	x := &xhrObject{
		r:           r,
		reqHeaders:  make(map[string]string),
		respHeaders: make(map[string]string),
		listeners:   make(map[string][]goja.Value),
	}
	obj := call.This
	x.obj = obj
	setXHRConsts(obj)
	_ = obj.Set("__xhr__", x)

	_ = obj.DefineAccessorProperty("readyState",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			x.mu.Lock()
			defer x.mu.Unlock()
			return r.vm.ToValue(x.state)
		}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = obj.DefineAccessorProperty("status",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			x.mu.Lock()
			defer x.mu.Unlock()
			return r.vm.ToValue(x.status)
		}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = obj.DefineAccessorProperty("statusText",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			x.mu.Lock()
			defer x.mu.Unlock()
			return r.vm.ToValue(x.statusText)
		}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = obj.DefineAccessorProperty("responseText",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			x.mu.Lock()
			defer x.mu.Unlock()
			return r.vm.ToValue(string(x.body))
		}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = obj.DefineAccessorProperty("response",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value { return r.xhrResponse(x) }),
		goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = obj.DefineAccessorProperty("responseURL",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			x.mu.Lock()
			defer x.mu.Unlock()
			if x.state == xhrUnsent {
				return r.vm.ToValue("")
			}
			return r.vm.ToValue(x.url)
		}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = obj.DefineAccessorProperty("timeout",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			x.mu.Lock()
			defer x.mu.Unlock()
			return r.vm.ToValue(x.timeoutMS)
		}),
		r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) > 0 {
				ms := call.Arguments[0].ToInteger()
				if ms < 0 {
					ms = 0
				}
				x.mu.Lock()
				x.timeoutMS = ms
				x.mu.Unlock()
			}
			return goja.Undefined()
		}), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = obj.DefineAccessorProperty("withCredentials",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(false)
		}),
		r.vm.ToValue(func(goja.FunctionCall) goja.Value { return goja.Undefined() }),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	_ = obj.Set("open", func(call goja.FunctionCall) goja.Value { return r.xhrOpen(x, call) })
	_ = obj.Set("setRequestHeader", func(call goja.FunctionCall) goja.Value { return r.xhrSetHeader(x, call) })
	_ = obj.Set("send", func(call goja.FunctionCall) goja.Value { return r.xhrSend(x, call) })
	_ = obj.Set("abort", func(call goja.FunctionCall) goja.Value { return r.xhrAbort(x) })
	_ = obj.Set("getResponseHeader", func(call goja.FunctionCall) goja.Value { return r.xhrGetHeader(x, call) })
	_ = obj.Set("getAllResponseHeaders", func(call goja.FunctionCall) goja.Value { return r.xhrGetAllHeaders(x) })
	_ = obj.Set("overrideMimeType", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = obj.Set("addEventListener", func(call goja.FunctionCall) goja.Value { return r.xhrAddListener(x, call) })
	_ = obj.Set("removeEventListener", func(call goja.FunctionCall) goja.Value { return r.xhrRemoveListener(x, call) })

	// responseType validates the enum on write; anything else is ignored.
	_ = obj.DefineAccessorProperty("responseType",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			x.mu.Lock()
			defer x.mu.Unlock()
			return r.vm.ToValue(x.responseType)
		}),
		r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) > 0 {
				switch call.Arguments[0].String() {
				case "", "text", "json":
					x.mu.Lock()
					x.responseType = call.Arguments[0].String()
					x.mu.Unlock()
				}
			}
			return goja.Undefined()
		}), goja.FLAG_FALSE, goja.FLAG_TRUE)

	// upload has the listener surface but never fires progress: the whole
	// body hands to the transport at once, so there is nothing to report.
	upload := r.vm.NewObject()
	_ = upload.Set("addEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = upload.Set("removeEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = obj.Set("upload", upload)

	return obj
}

// xhrResponse implements the response getter: text for ""/"text", parsed
// JSON (or null) for "json".
func (r *Runtime) xhrResponse(x *xhrObject) goja.Value {
	x.mu.Lock()
	rt, body := x.responseType, append([]byte(nil), x.body...)
	x.mu.Unlock()
	switch rt {
	case "json":
		if len(body) == 0 {
			return goja.Null()
		}
		jsonVal := r.vm.Get("JSON")
		if jsonObj, ok := jsonVal.(*goja.Object); ok {
			if parse, ok := goja.AssertFunction(jsonObj.Get("parse")); ok {
				if v, err := parse(goja.Undefined(), r.vm.ToValue(string(body))); err == nil {
					return v
				}
			}
		}
		return goja.Null()
	default:
		return r.vm.ToValue(string(body))
	}
}

func xhrInvalidState(r *Runtime, what string) goja.Value {
	panic(namedError(r, "InvalidStateError", what))
}

func (r *Runtime) xhrOpen(x *xhrObject, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		panic(namedError(r, "SyntaxError", "XMLHttpRequest.open requires method and url"))
	}
	method := strings.ToUpper(call.Arguments[0].String())
	rawURL := call.Arguments[1].String()
	async := true
	if len(call.Arguments) >= 3 {
		async = call.Arguments[2].ToBoolean()
	}
	// Synchronous open would block the VM thread on the network with no
	// interruptible bound; no shipped page may do that. Async is the only
	// mode; refusing loudly beats hanging silently.
	if !async {
		panic(namedError(r, "InvalidAccessError", "XMLHttpRequest synchronous open is not supported"))
	}
	if _, err := url.Parse(rawURL); err != nil {
		panic(namedError(r, "SyntaxError", "XMLHttpRequest.open: invalid url"))
	}
	x.mu.Lock()
	x.method = method
	x.url = r.resolveFetchURL(rawURL)
	x.reqHeaders = make(map[string]string)
	x.opened = true
	x.sent = false
	x.settled = false
	x.aborted = false
	x.timedOut = false
	x.state = xhrOpened
	x.status = 0
	x.statusText = ""
	x.respHeaders = make(map[string]string)
	x.body = nil
	if x.timer != nil {
		x.timer.Stop()
		x.timer = nil
	}
	x.mu.Unlock()
	return goja.Undefined()
}

func (r *Runtime) xhrSetHeader(x *xhrObject, call goja.FunctionCall) goja.Value {
	x.mu.Lock()
	opened, sent := x.opened, x.sent
	x.mu.Unlock()
	if !opened || sent {
		return xhrInvalidState(r, "XMLHttpRequest.setRequestHeader: not open or already sent")
	}
	if len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	name := strings.TrimSpace(call.Arguments[0].String())
	if name == "" {
		panic(namedError(r, "SyntaxError", "XMLHttpRequest.setRequestHeader: empty header name"))
	}
	x.mu.Lock()
	x.reqHeaders[name] = call.Arguments[1].String()
	x.mu.Unlock()
	return goja.Undefined()
}

func (r *Runtime) xhrSend(x *xhrObject, call goja.FunctionCall) goja.Value {
	x.mu.Lock()
	if !x.opened {
		x.mu.Unlock()
		return xhrInvalidState(r, "XMLHttpRequest.send: open() has not been called")
	}
	if x.sent {
		x.mu.Unlock()
		return xhrInvalidState(r, "XMLHttpRequest.send: already sent")
	}
	x.sent = true
	x.settled = false
	x.state = xhrOpened
	method, reqURL := x.method, x.url
	headers := make(map[string]string, len(x.reqHeaders)+1)
	for k, v := range x.reqHeaders {
		headers[k] = v
	}
	timeoutMS := x.timeoutMS
	x.mu.Unlock()

	var body []byte
	if len(call.Arguments) > 0 {
		if b := call.Arguments[0]; b != nil && !goja.IsUndefined(b) && !goja.IsNull(b) {
			body = []byte(b.String())
		}
	}
	if len(body) > MaxFetchBodyBytes {
		return xhrInvalidState(r, "XMLHttpRequest.send: request body exceeds limit")
	}

	st := xhrOf(r)
	if st == nil || st.client == nil {
		// No transport: fail through the error event, never by throwing.
		// Pages that feature-detect with try/catch around send still work;
		// pages that listen for error get the event they expect.
		r.fireXHR(x, "readystatechange", 0, 0)
		r.queueMicrotask(func() error {
			r.completeXHRError(x)
			return nil
		})
		return goja.Undefined()
	}

	st.mu.Lock()
	if st.active >= MaxConcurrentXHR {
		st.mu.Unlock()
		r.fireXHR(x, "readystatechange", 0, 0)
		r.queueMicrotask(func() error {
			r.completeXHRError(x)
			return nil
		})
		return goja.Undefined()
	}
	st.active++
	st.mu.Unlock()

	r.fireXHR(x, "readystatechange", 0, 0)

	if timeoutMS > 0 {
		x.mu.Lock()
		x.timer = time.AfterFunc(time.Duration(timeoutMS)*time.Millisecond, func() {
			x.mu.Lock()
			if x.settled {
				x.mu.Unlock()
				return
			}
			x.settled = true
			x.timedOut = true
			x.mu.Unlock()
			st.mu.Lock()
			st.active--
			st.mu.Unlock()
			r.queueMicrotask(func() error {
				r.completeXHRTimeout(x)
				return nil
			})
		})
		x.mu.Unlock()
	}

	// Capture the shared subresource budget on the VM thread so the request
	// goroutine never locks r.mu off-thread. nil leaves the request ungated.
	budget := r.subresourceBudget()

	go func() {
		if IsCORSRequest(r.opts.URL, reqURL) {
			headers["Origin"] = getOrigin(r.opts.URL)
		}
		resp, err := st.client.Fetch(reqURL, method, headers, body)
		// Charge the response body to the document's shared subresource
		// budget, mirroring fetch: a body that does not fit fails the request
		// cleanly (XHR error) rather than succeeding past the cap.
		if err == nil && budget != nil && !budget.Take(len(resp.Body)) {
			err = fmt.Errorf("XHR response exceeds the document subresource budget")
			resp = nil
		}
		st.mu.Lock()
		st.active--
		st.mu.Unlock()
		x.mu.Lock()
		if x.timer != nil {
			x.timer.Stop()
			x.timer = nil
		}
		if x.settled {
			// Aborted or timed out while the bytes were coming back.
			x.mu.Unlock()
			return
		}
		x.settled = true
		x.mu.Unlock()
		if err != nil {
			r.queueMicrotask(func() error {
				r.completeXHRError(x)
				return nil
			})
			return
		}
		r.queueMicrotask(func() error {
			r.completeXHRLoad(x, resp)
			return nil
		})
	}()

	return goja.Undefined()
}

// xhrAbort implements abort(): in-flight requests settle here synchronously
// (abort + loadend fire on this thread); anything else just resets.
func (r *Runtime) xhrAbort(x *xhrObject) goja.Value {
	x.mu.Lock()
	if !x.sent || x.settled {
		x.mu.Unlock()
		return goja.Undefined()
	}
	x.settled = true
	x.aborted = true
	if x.timer != nil {
		x.timer.Stop()
		x.timer = nil
	}
	x.state = xhrDone
	x.status = 0
	x.statusText = ""
	x.body = nil
	x.mu.Unlock()
	r.fireXHR(x, "readystatechange", 0, 0)
	r.fireXHR(x, "abort", 0, 0)
	r.fireXHR(x, "loadend", 0, 0)
	return goja.Undefined()
}

func (r *Runtime) completeXHRLoad(x *xhrObject, resp *FetchResponse) {
	x.mu.Lock()
	x.state = xhrHeadersReceived
	x.status = resp.Status
	x.statusText = resp.StatusText
	hdrs := make(map[string]string, len(resp.Headers))
	for k, v := range resp.Headers {
		hdrs[strings.ToLower(k)] = v
	}
	x.respHeaders = hdrs
	x.mu.Unlock()
	r.fireXHR(x, "readystatechange", 0, 0)

	n := int64(len(resp.Body))
	x.mu.Lock()
	x.state = xhrLoading
	x.body = resp.Body
	x.mu.Unlock()
	r.fireXHR(x, "readystatechange", n, n)
	r.fireXHR(x, "progress", n, n)

	x.mu.Lock()
	x.state = xhrDone
	x.mu.Unlock()
	r.fireXHR(x, "readystatechange", n, n)
	r.fireXHR(x, "load", n, n)
	r.fireXHR(x, "loadend", n, n)
}

func (r *Runtime) completeXHRError(x *xhrObject) {
	x.mu.Lock()
	x.state = xhrDone
	x.status = 0
	x.statusText = ""
	x.body = nil
	x.mu.Unlock()
	r.fireXHR(x, "readystatechange", 0, 0)
	r.fireXHR(x, "error", 0, 0)
	r.fireXHR(x, "loadend", 0, 0)
}

func (r *Runtime) completeXHRTimeout(x *xhrObject) {
	x.mu.Lock()
	x.state = xhrDone
	x.status = 0
	x.statusText = ""
	x.body = nil
	x.mu.Unlock()
	r.fireXHR(x, "readystatechange", 0, 0)
	r.fireXHR(x, "timeout", 0, 0)
	r.fireXHR(x, "loadend", 0, 0)
}

// fireXHR dispatches one event to the on<type> property and every listener
// registered for type. All calls happen on the VM thread; listener errors
// are contained per listener under the runtime bound, as with timers.
func (r *Runtime) fireXHR(x *xhrObject, typ string, loaded, total int64) {
	ev := r.vm.NewObject()
	_ = ev.Set("type", typ)
	_ = ev.Set("target", x.obj)
	_ = ev.Set("loaded", loaded)
	_ = ev.Set("total", total)
	_ = ev.Set("lengthComputable", total > 0)

	if handler := x.obj.Get("on" + typ); handler != nil {
		if fn, ok := goja.AssertFunction(handler); ok {
			_, _ = r.callBounded(func() (goja.Value, error) { return fn(x.obj, ev) })
		}
	}
	x.mu.Lock()
	listeners := append([]goja.Value(nil), x.listeners[typ]...)
	x.mu.Unlock()
	for _, l := range listeners {
		if fn, ok := goja.AssertFunction(l); ok {
			_, _ = r.callBounded(func() (goja.Value, error) { return fn(x.obj, ev) })
		}
	}
}

func (r *Runtime) xhrAddListener(x *xhrObject, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	typ := call.Arguments[0].String()
	fn := call.Arguments[1]
	if _, ok := goja.AssertFunction(fn); !ok {
		return goja.Undefined()
	}
	x.mu.Lock()
	x.listeners[typ] = append(x.listeners[typ], fn)
	x.mu.Unlock()
	return goja.Undefined()
}

func (r *Runtime) xhrRemoveListener(x *xhrObject, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	typ := call.Arguments[0].String()
	fn := call.Arguments[1]
	x.mu.Lock()
	kept := x.listeners[typ][:0]
	for _, l := range x.listeners[typ] {
		if !l.SameAs(fn) {
			kept = append(kept, l)
		}
	}
	x.listeners[typ] = kept
	x.mu.Unlock()
	return goja.Undefined()
}

func (r *Runtime) xhrGetHeader(x *xhrObject, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return goja.Null()
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.state < xhrDone {
		return goja.Null()
	}
	if v, ok := x.respHeaders[strings.ToLower(call.Arguments[0].String())]; ok {
		return r.vm.ToValue(v)
	}
	return goja.Null()
}

func (r *Runtime) xhrGetAllHeaders(x *xhrObject) goja.Value {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.state < xhrDone {
		return r.vm.ToValue("")
	}
	names := make([]string, 0, len(x.respHeaders))
	for k := range x.respHeaders {
		names = append(names, k)
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, k := range names {
		fmt.Fprintf(&sb, "%s: %s\r\n", k, x.respHeaders[k])
	}
	return r.vm.ToValue(sb.String())
}
