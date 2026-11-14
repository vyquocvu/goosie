package js

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

const (
	MaxConcurrentFetches = 6

	MaxFetchBodyBytes = 1 << 20

	FetchTimeout = 30 * time.Second
)

var ErrFetchNotAvailable = fmt.Errorf("fetch is not available")

var ErrTooManyConcurrentFetches = fmt.Errorf("too many concurrent fetches")

type HTTPFetcher interface {
	Fetch(ctx context.Context, url string, method string, headers map[string]string, body []byte) (*FetchResponse, error)
}

type FetchOptions struct {
	Method       string
	Headers      map[string]string
	Body         string
	BodyBytes    []byte
	HasBodyBytes bool
	Credentials  string
}

type FetchResponse struct {
	Status     int
	StatusText string
	Headers    map[string]string
	Body       []byte
	URL        string
	OK         bool
}

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

type fetchCallback struct {
	fn       goja.Value
	isReject bool
	arg      interface{}
	chain    *thenableObj
}

type thenableObj struct {
	r               *Runtime
	obj             *goja.Object
	mu              sync.Mutex
	resolved        bool
	rejected        bool
	result          interface{}
	callbackPending bool

	thenCB  goja.Value
	catchCB goja.Value

	chain *thenableObj
}

type fetchState struct {
	client    HTTPFetcher
	ctx       context.Context
	mu        sync.Mutex
	active    int
	callbacks []fetchCallback
}

type fetchAbort struct {
	mu      sync.Mutex
	aborted bool
	reason  goja.Value
	done    chan struct{}
}

func newFetchAbort() *fetchAbort {
	return &fetchAbort{done: make(chan struct{})}
}

func (a *fetchAbort) fire(reason goja.Value) {
	a.mu.Lock()
	if a.aborted {
		a.mu.Unlock()
		return
	}
	a.aborted = true
	a.reason = reason
	a.mu.Unlock()
	close(a.done)
}

func (a *fetchAbort) fired() (bool, goja.Value) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.aborted, a.reason
}

func (a *fetchAbort) reasonValue() goja.Value {
	_, reason := a.fired()
	return reason
}

func (r *Runtime) setupFetch(client HTTPFetcher) {
	if client == nil {
		r.installFetchWrapper()
		return
	}

	fs := &fetchState{client: client, ctx: context.Background()}
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

func (r *Runtime) parseFetchOpts(val goja.Value) (FetchOptions, error) {
	var opts FetchOptions
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return opts, nil
	}
	obj, ok := val.(*goja.Object)
	if !ok {
		return opts, nil
	}
	if v := obj.Get("method"); v != nil && !goja.IsUndefined(v) {
		opts.Method = strings.ToUpper(v.String())
	}
	if opts.Method == "" {
		opts.Method = "GET"
	}
	if v := obj.Get("body"); v != nil && !goja.IsUndefined(v) {
		if body, ok := v.(*goja.Object); ok {
			if list := r.formDataValues[body]; list != nil {
				boundary, data, err := r.encodeMultipartFormData(list, "UTF-8")
				if err != nil {
					return opts, err
				}
				opts.BodyBytes, opts.HasBodyBytes = data, true
				if opts.Headers == nil {
					opts.Headers = make(map[string]string)
				}
				if GetHeader(opts.Headers, "Content-Type") == "" {
					opts.Headers["Content-Type"] = "multipart/form-data; boundary=" + boundary
				}
			} else {
				opts.Body = v.String()
			}
		} else {
			opts.Body = v.String()
		}
	}
	if v := obj.Get("credentials"); v != nil && !goja.IsUndefined(v) {
		opts.Credentials = strings.ToLower(v.String())
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
	return opts, nil
}

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

const fetchSignalInvalidMessage = "Failed to execute 'fetch' on 'Window': Failed to read the 'signal' property from 'RequestInit': Failed to convert value to 'AbortSignal'."

func (r *Runtime) followFetchSignal(init goja.Value) (*fetchAbort, goja.Value) {
	if init == nil || goja.IsUndefined(init) || goja.IsNull(init) {
		return nil, nil
	}
	initObj, ok := init.(*goja.Object)
	if !ok {
		return nil, nil
	}
	val := initObj.Get("signal")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, nil
	}
	signal, ok := val.(*goja.Object)
	if !ok {
		return nil, r.invalidFetchSignalError()
	}
	register, ok := goja.AssertFunction(signal.Get("__registerAbortAlgorithm__"))
	if !ok {
		return nil, r.invalidFetchSignalError()
	}

	control := newFetchAbort()
	reason := func() goja.Value {
		if v := signal.Get("reason"); v != nil {
			return v
		}
		return r.newAbortErrorDOMException()
	}
	if aborted := signal.Get("aborted"); aborted != nil && aborted.ToBoolean() {
		control.fire(reason())
		return control, nil
	}
	_, _ = register(signal, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		control.fire(reason())
		return goja.Undefined()
	}))
	return control, nil
}

func (r *Runtime) invalidFetchSignalError() goja.Value {
	if ctor, ok := goja.AssertConstructor(r.vm.Get("TypeError")); ok {
		if err, newErr := ctor(nil, r.vm.ToValue(fetchSignalInvalidMessage)); newErr == nil {
			return err
		}
	}
	return r.vm.ToValue(fetchSignalInvalidMessage)
}

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

	opts := FetchOptions{Method: "GET"}
	var abort *fetchAbort
	if len(call.Arguments) >= 2 {
		var err error
		opts, err = r.parseFetchOpts(call.Arguments[1])
		if err != nil {
			return r.makeRejectThenable(err.Error())
		}
		var invalid goja.Value
		abort, invalid = r.followFetchSignal(call.Arguments[1])
		if invalid != nil {
			return r.rejectThenableWithValue(invalid)
		}
	}

	bodyLen := len(opts.Body)
	if opts.HasBodyBytes {
		bodyLen = len(opts.BodyBytes)
	}
	if bodyLen > MaxFetchBodyBytes {
		return r.makeRejectThenable("request body exceeds limit")
	}

	if abort != nil {
		if fired, reason := abort.fired(); fired {
			return r.rejectThenableWithValue(reason)
		}
	}

	fs.mu.Lock()
	if fs.active >= MaxConcurrentFetches {
		fs.mu.Unlock()
		return r.makeRejectThenable("too many concurrent fetches")
	}
	fs.active++
	fs.mu.Unlock()

	budget := r.subresourceBudget()

	t := r.newThenable()

	go r.runFetch(fs, resolvedURL, opts, t, budget, abort)

	return t.obj
}

func (r *Runtime) runFetch(fs *fetchState, fetchURL string, opts FetchOptions, t *thenableObj, budget SubresourceBudget, abort *fetchAbort) {
	defer func() {
		fs.mu.Lock()
		fs.active--
		fs.mu.Unlock()
	}()

	withCredentials := opts.Credentials == "include"

	documentURL := r.opts.URL
	if IsCORSRequest(documentURL, fetchURL) {
		origin := getOrigin(documentURL)
		if opts.Headers == nil {
			opts.Headers = make(map[string]string)
		}
		opts.Headers["Origin"] = origin

		if NeedsPreflight(opts.Method, opts.Headers) {
			preflightHeaders := buildPreflightHeaders(origin, opts.Method, opts.Headers)
			preflightResp, err, aborted := callFetcher(fs.ctx, abort, func(ctx context.Context) (*FetchResponse, error) {
				return fs.client.Fetch(ctx, fetchURL, "OPTIONS", preflightHeaders, nil)
			})
			if aborted {
				fs.rejectFetch(t, abort.reasonValue())
				return
			}
			if err != nil {
				fs.rejectFetch(t, "preflight failed: "+err.Error())
				return
			}
			if !CheckPreflightResponse(origin, opts.Method, opts.Headers, preflightResp.Headers, withCredentials) {
				fs.rejectFetch(t, "preflight CORS check failed")
				return
			}
		}
	}

	body := []byte(opts.Body)
	if opts.HasBodyBytes {
		body = opts.BodyBytes
	}
	resp, err, aborted := callFetcher(fs.ctx, abort, func(ctx context.Context) (*FetchResponse, error) {
		return fs.client.Fetch(ctx, fetchURL, opts.Method, opts.Headers, body)
	})
	if aborted {
		fs.rejectFetch(t, abort.reasonValue())
		return
	}

	if err == nil && budget != nil && !budget.Take(len(resp.Body)) {
		err = fmt.Errorf("fetch response exceeds the document subresource budget")
		resp = nil
	}

	if err == nil && IsCORSRequest(documentURL, fetchURL) {
		origin := getOrigin(documentURL)
		if !CheckCORSResponse(origin, resp.Headers, withCredentials) {
			fs.rejectFetch(t, "CORS check failed: Access-Control-Allow-Origin header missing or mismatched")
			return
		}
	}

	if err != nil {
		fs.rejectFetch(t, err.Error())
		return
	}

	fs.resolveFetch(t, resp)
}

func callFetcher(baseCtx context.Context, abort *fetchAbort, fn func(context.Context) (*FetchResponse, error)) (resp *FetchResponse, err error, aborted bool) {
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	if abort == nil {
		resp, err = fn(ctx)
		return resp, err, false
	}

	go func() {
		<-abort.done
		cancel()
	}()

	type result struct {
		resp *FetchResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		resp, err := fn(ctx)
		ch <- result{resp: resp, err: err}
	}()
	select {
	case <-abort.done:
		return nil, nil, true
	case res := <-ch:
		if fired, _ := abort.fired(); fired {
			return nil, nil, true
		}
		return res.resp, res.err, false
	}
}

func (fs *fetchState) rejectFetch(t *thenableObj, msg interface{}) {
	t.settleFetch(nil, msg, true)
	fs.mu.Lock()
	fs.callbacks = append(fs.callbacks, fetchCallback{
		isReject: true,
		arg:      msg,
		chain:    t,
	})
	fs.mu.Unlock()
}

func (fs *fetchState) resolveFetch(t *thenableObj, resp *FetchResponse) {
	t.settleFetch(resp, nil, false)
	fs.mu.Lock()
	fs.callbacks = append(fs.callbacks, fetchCallback{
		isReject: false,
		arg:      resp,
		chain:    t,
	})
	fs.mu.Unlock()
}

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

		if resolved && onOK != nil {
			t.mu.Lock()
			pending := t.callbackPending
			t.mu.Unlock()
			if !pending {
				r.invokeResolvedCallback(onOK, false, result, chain)
			}
		} else if rejected && onErr != nil {
			t.mu.Lock()
			pending := t.callbackPending
			t.mu.Unlock()
			if !pending {
				r.invokeResolvedCallback(onErr, true, result, chain)
			}
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
			t.mu.Lock()
			pending := t.callbackPending
			t.mu.Unlock()
			if !pending {
				r.invokeResolvedCallback(onErr, true, result, chain)
			}
		}

		return chain.obj
	})

	t.obj = obj
	return t
}

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

func (r *Runtime) makeResponseObject(resp *FetchResponse) *goja.Object {
	obj := r.vm.NewObject()
	if r.responseProto != nil {
		_ = obj.SetPrototype(r.responseProto)
	}

	_ = obj.Set("status", resp.Status)
	_ = obj.Set("statusText", resp.StatusText)
	_ = obj.Set("ok", resp.OK)
	_ = obj.Set("url", resp.URL)
	bodyStr := string(resp.Body)

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

	_ = obj.Set("text", func(call goja.FunctionCall) goja.Value {
		t := r.newThenable()
		t.resolve(r.vm.ToValue(bodyStr))
		return t.obj
	})

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

func (r *Runtime) setupResponse() {
	prototype := r.vm.NewObject()
	ctor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		var body goja.Value = goja.Undefined()
		if len(call.Arguments) > 0 {
			body = call.Argument(0)
		}
		bodyBytes, mime, err := r.responseBodyInit(body)
		if err != nil {
			panic(r.vm.NewTypeError(err.Error()))
		}
		status, statusText := 200, "OK"
		if len(call.Arguments) > 1 && !goja.IsNull(call.Argument(1)) && !goja.IsUndefined(call.Argument(1)) {
			init := call.Argument(1).ToObject(r.vm)
			if value := init.Get("status"); value != nil && !goja.IsUndefined(value) {
				status = int(value.ToInteger())
			}
			if value := init.Get("statusText"); value != nil && !goja.IsUndefined(value) {
				statusText = value.String()
			}
		}
		obj := call.This
		var used bool
		_ = obj.DefineAccessorProperty("bodyUsed", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(used)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
		_ = obj.DefineAccessorProperty("status", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(status)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
		_ = obj.DefineAccessorProperty("statusText", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(statusText)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
		_ = obj.DefineAccessorProperty("ok", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(status >= 200 && status <= 299)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
		_ = obj.DefineAccessorProperty("type", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue("default")
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
		_ = obj.DefineAccessorProperty("url", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue("")
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
		headers := r.vm.NewObject()
		_ = headers.Set("get", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return goja.Null()
			}
			if strings.EqualFold(call.Argument(0).String(), "content-type") && mime != "" {
				return r.vm.ToValue(mime)
			}
			return goja.Null()
		})
		_ = obj.Set("headers", headers)
		consume := func() {
			if used {
				panic(r.vm.NewTypeError("Response body has already been consumed"))
			}
			used = true
		}
		_ = obj.Set("arrayBuffer", func(goja.FunctionCall) goja.Value {
			consume()
			thenable := r.newThenable()
			thenable.resolve(r.vm.ToValue(r.vm.NewArrayBuffer(bytes.Clone(bodyBytes))))
			return thenable.obj
		})
		_ = obj.Set("blob", func(goja.FunctionCall) goja.Value {
			consume()
			thenable := r.newThenable()
			thenable.resolve(r.vm.ToValue(r.makeBlob(nil, bytes.Clone(bodyBytes), mime, false, "", 0)))
			return thenable.obj
		})
		_ = obj.Set("text", func(goja.FunctionCall) goja.Value {
			consume()
			thenable := r.newThenable()
			thenable.resolve(r.vm.ToValue(decodeBlobText(bodyBytes)))
			return thenable.obj
		})
		_ = obj.Set("json", func(goja.FunctionCall) goja.Value {
			consume()
			thenable := r.newThenable()
			var parsed any
			if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
				thenable.reject(r.vm.ToValue("JSON parse error: " + err.Error()))
			} else {
				thenable.resolve(r.vm.ToValue(parsed))
			}
			return thenable.obj
		})
		return obj
	}).(*goja.Object)
	if !ok {
		return
	}
	r.responseProto = prototype
	_ = ctor.Set("prototype", prototype)
	_ = prototype.Set("constructor", ctor)
	r.defineInterfaceToStringTag("Response", prototype)
	_ = r.vm.Set("Response", ctor)
	if win, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = win.Set("Response", ctor)
	}
}

func (r *Runtime) responseBodyInit(value goja.Value) ([]byte, string, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, "", nil
	}
	if object, ok := value.(*goja.Object); ok {
		if blob := r.fileValues[object]; blob != nil {
			return bytes.Clone(blob.data), blob.typeValue, nil
		}
		if buffer, ok := object.Export().(goja.ArrayBuffer); ok {
			if buffer.Detached() {
				return nil, "", errors.New("Response body buffer is detached")
			}
			return bytes.Clone(buffer.Bytes()), "", nil
		}
		if bufferValue := object.Get("buffer"); bufferValue != nil && !goja.IsUndefined(bufferValue) {
			if buffer, ok := bufferValue.Export().(goja.ArrayBuffer); ok {
				if buffer.Detached() {
					return nil, "", errors.New("Response body buffer is detached")
				}
				offset := int(object.Get("byteOffset").ToInteger())
				length := int(object.Get("byteLength").ToInteger())
				data := buffer.Bytes()
				if offset < 0 || length < 0 || offset > len(data) || length > len(data)-offset {
					return nil, "", errors.New("Response body view is out of bounds")
				}
				return bytes.Clone(data[offset : offset+length]), "", nil
			}
		}
	}
	return []byte(value.String()), "", nil
}

func (r *Runtime) makeRejectThenable(msg string) goja.Value {
	return r.rejectThenableWithValue(r.vm.ToValue(msg))
}

func (r *Runtime) rejectThenableWithValue(val goja.Value) goja.Value {
	t := r.newThenable()
	t.rejected = true
	t.result = val

	fs := fetchOf(r)
	if fs == nil {
		return t.obj
	}

	fs.mu.Lock()
	fs.callbacks = append(fs.callbacks, fetchCallback{
		fn:       t.catchCB,
		isReject: true,
		arg:      val,
		chain:    t,
	})
	fs.mu.Unlock()

	return t.obj
}

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

func (r *Runtime) PendingFetches() int {
	fs := fetchOf(r)
	if fs == nil {
		return 0
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.active
}

func (r *Runtime) invokeOneCallback(cb fetchCallback) {
	fn := cb.fn
	chain := cb.chain
	if cb.chain != nil {
		cb.chain.mu.Lock()
		settled := cb.chain.resolved || cb.chain.rejected
		if cb.isReject {
			fn = cb.chain.catchCB
		} else {
			fn = cb.chain.thenCB
		}
		chain = cb.chain.chain
		cb.chain.callbackPending = false
		cb.chain.mu.Unlock()
		if !settled {
			return
		}
	}
	if fn == nil || goja.IsUndefined(fn) {
		if cb.chain != nil && !cb.isReject {
			if resp, ok := cb.arg.(*FetchResponse); ok {
				cb.chain.resolve(r.makeResponseObject(resp))
			}
		}
		return
	}
	f, ok := goja.AssertFunction(fn)
	if !ok {
		return
	}

	var arg goja.Value
	if resp, ok := cb.arg.(*FetchResponse); ok {
		arg = r.makeResponseObject(resp)
	} else if v, ok := cb.arg.(goja.Value); ok {
		arg = v
	} else if msg, ok := cb.arg.(string); ok {
		arg = r.vm.ToValue(msg)
	}

	ret, err := r.callBounded(func() (goja.Value, error) { return f(nil, arg) })
	if err != nil {
		return
	}

	if chain != nil {
		r.propagateToChain(ret, chain)
	}
}

func (t *thenableObj) settleFetch(value interface{}, err interface{}, rejected bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resolved = !rejected
	t.rejected = rejected
	if rejected {
		t.result = err
	} else {
		t.result = value
	}
	t.callbackPending = true
}
