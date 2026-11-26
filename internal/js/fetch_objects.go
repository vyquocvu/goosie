package js

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/dop251/goja"
)

type fetchHeaderEntry struct {
	name  string
	value string
}

type fetchHeadersState struct {
	entries []fetchHeaderEntry
	guard   string
}

type fetchBodyState struct {
	data         []byte
	present      bool
	used         bool
	streamLocked bool
	stream       *goja.Object
}

type fetchRequestState struct {
	url           string
	method        string
	headers       *fetchHeadersState
	headersObject *goja.Object
	body          *fetchBodyState
	credentials   string
	signal        goja.Value
}

type fetchResponseState struct {
	status        int
	statusText    string
	url           string
	headers       *fetchHeadersState
	headersObject *goja.Object
	body          *fetchBodyState
}

func (r *Runtime) ensureFetchObjectState() *fetchState {
	fs := fetchOf(r)
	if fs == nil {
		fs = &fetchState{ctx: context.Background()}
		setFetchState(r, fs)
	}
	if fs.headers == nil {
		fs.headers = make(map[*goja.Object]*fetchHeadersState)
	}
	if fs.requests == nil {
		fs.requests = make(map[*goja.Object]*fetchRequestState)
	}
	if fs.responses == nil {
		fs.responses = make(map[*FetchResponse]*goja.Object)
	}
	if fs.responseBodies == nil {
		fs.responseBodies = make(map[*goja.Object]*fetchResponseState)
	}
	if fs.headersProto == nil {
		fs.headersProto = r.vm.NewObject()
	}
	if fs.requestProto == nil {
		fs.requestProto = r.vm.NewObject()
	}
	return fs
}

func (r *Runtime) setupFetchObjects() {
	fs := r.ensureFetchObjectState()
	r.installHeadersConstructor(fs.headersProto)
	r.installRequestConstructor(fs.requestProto)
}

func (r *Runtime) installHeadersConstructor(proto *goja.Object) {
	_ = r.vm.Set("__goosieHeadersInit", func(call goja.FunctionCall) goja.Value {
		state := &fetchHeadersState{guard: "none"}
		if err := r.loadHeadersInit(state, call.Argument(1)); err != nil {
			panic(r.vm.NewTypeError(err.Error()))
		}
		obj, ok := call.Argument(0).(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("Failed to construct 'Headers'"))
		}
		r.ensureFetchObjectState().headers[obj] = state
		return obj
	})
	if _, err := r.vm.RunString(`
		globalThis.Headers = (function(init) {
			function Headers(value) {
				if (!(this instanceof Headers)) throw new TypeError("Failed to construct 'Headers': Please use the 'new' operator");
				init(this, value);
			}
			return Headers;
		})(__goosieHeadersInit);
	`); err != nil {
		panic(err)
	}
	ctor := r.vm.Get("Headers").ToObject(r.vm)
	proto = ctor.Get("prototype").ToObject(r.vm)
	r.ensureFetchObjectState().headersProto = proto
	_ = proto.Set("constructor", ctor)

	_ = proto.Set("append", func(call goja.FunctionCall) goja.Value {
		state := r.headersMethodReceiver(call, "append")
		if len(call.Arguments) < 2 {
			panic(r.vm.NewTypeError("Failed to execute 'append' on 'Headers': 2 arguments required"))
		}
		r.appendFetchHeader(state, call.Argument(0).String(), call.Argument(1).String())
		return goja.Undefined()
	})
	_ = proto.Set("delete", func(call goja.FunctionCall) goja.Value {
		state := r.headersMethodReceiver(call, "delete")
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("Failed to execute 'delete' on 'Headers': 1 argument required"))
		}
		r.deleteFetchHeader(state, call.Argument(0).String())
		return goja.Undefined()
	})
	_ = proto.Set("get", func(call goja.FunctionCall) goja.Value {
		state := r.headersMethodReceiver(call, "get")
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("Failed to execute 'get' on 'Headers': 1 argument required"))
		}
		if !state.has(call.Argument(0).String()) {
			return goja.Null()
		}
		return r.vm.ToValue(state.get(call.Argument(0).String()))
	})
	_ = proto.Set("has", func(call goja.FunctionCall) goja.Value {
		state := r.headersMethodReceiver(call, "has")
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("Failed to execute 'has' on 'Headers': 1 argument required"))
		}
		return r.vm.ToValue(state.has(call.Argument(0).String()))
	})
	_ = proto.Set("set", func(call goja.FunctionCall) goja.Value {
		state := r.headersMethodReceiver(call, "set")
		if len(call.Arguments) < 2 {
			panic(r.vm.NewTypeError("Failed to execute 'set' on 'Headers': 2 arguments required"))
		}
		r.setFetchHeader(state, call.Argument(0).String(), call.Argument(1).String())
		return goja.Undefined()
	})
	_ = proto.Set("getSetCookie", func(call goja.FunctionCall) goja.Value {
		state := r.headersMethodReceiver(call, "getSetCookie")
		values := make([]string, 0)
		for _, entry := range state.entries {
			if entry.name == "set-cookie" {
				values = append(values, entry.value)
			}
		}
		return r.vm.ToValue(values)
	})
	_ = proto.Set("forEach", func(call goja.FunctionCall) goja.Value {
		state := r.headersMethodReceiver(call, "forEach")
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("Failed to execute 'forEach' on 'Headers': 1 argument required"))
		}
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(r.vm.NewTypeError("Failed to execute 'forEach' on 'Headers': parameter 1 is not a function"))
		}
		var thisArg goja.Value = goja.Undefined()
		if len(call.Arguments) > 1 {
			thisArg = call.Argument(1)
		}
		for _, entry := range state.entries {
			_, _ = fn(thisArg, r.vm.ToValue(entry.value), r.vm.ToValue(entry.name), call.This)
		}
		return goja.Undefined()
	})
	_ = proto.Set("keys", func(call goja.FunctionCall) goja.Value {
		state := r.headersMethodReceiver(call, "keys")
		return r.fetchHeaderIterator("keys", state)
	})
	_ = proto.Set("values", func(call goja.FunctionCall) goja.Value {
		state := r.headersMethodReceiver(call, "values")
		return r.fetchHeaderIterator("values", state)
	})
	_ = proto.Set("entries", func(call goja.FunctionCall) goja.Value {
		state := r.headersMethodReceiver(call, "entries")
		return r.fetchHeaderIterator("entries", state)
	})
	_ = proto.DefineDataPropertySymbol(goja.SymIterator, proto.Get("entries"), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	r.defineInterfaceToStringTag("Headers", proto)
	_ = r.vm.Set("Headers", ctor)
	if win, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = win.Set("Headers", ctor)
	}
}

func (r *Runtime) headersMethodReceiver(call goja.FunctionCall, method string) *fetchHeadersState {
	obj, ok := call.This.(*goja.Object)
	if ok {
		if state := r.ensureFetchObjectState().headers[obj]; state != nil {
			return state
		}
	}
	panic(r.vm.NewTypeError("Failed to execute '" + method + "' on 'Headers': Illegal invocation"))
}

func (r *Runtime) headersReceiver(value goja.Value) *fetchHeadersState {
	obj, ok := value.(*goja.Object)
	if !ok {
		return nil
	}
	return r.ensureFetchObjectState().headers[obj]
}

func (r *Runtime) newHeadersObject(state *fetchHeadersState) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(r.ensureFetchObjectState().headersProto)
	r.ensureFetchObjectState().headers[obj] = state
	return obj
}

func (r *Runtime) headersFromInit(value goja.Value, guard string) (*goja.Object, error) {
	state := &fetchHeadersState{guard: guard}
	if err := r.loadHeadersInit(state, value); err != nil {
		return nil, err
	}
	return r.newHeadersObject(state), nil
}

func (r *Runtime) loadHeadersInit(state *fetchHeadersState, value goja.Value) error {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	obj, ok := value.(*goja.Object)
	if !ok {
		return errors.New("HeadersInit must be an object")
	}
	if source := r.headersReceiver(value); source != nil {
		for _, entry := range source.entries {
			r.appendFetchHeader(state, entry.name, entry.value)
		}
		return nil
	}
	if obj.ClassName() == "Array" {
		length := int(obj.Get("length").ToInteger())
		for i := 0; i < length; i++ {
			pairValue := obj.Get(fmt.Sprintf("%d", i))
			pair, ok := pairValue.(*goja.Object)
			if !ok || pair.ClassName() != "Array" || pair.Get("length").ToInteger() < 2 {
				return errors.New("Headers sequence entries must contain a name and value")
			}
			r.appendFetchHeader(state, pair.Get("0").String(), pair.Get("1").String())
		}
		return nil
	}
	if symbol, ok := r.vm.Get("Symbol").(*goja.Object).Get("iterator").(*goja.Symbol); ok {
		if iteratorValue := obj.GetSymbol(symbol); iteratorValue != nil && !goja.IsUndefined(iteratorValue) {
			iteratorFn, ok := goja.AssertFunction(iteratorValue)
			if !ok {
				return errors.New("HeadersInit iterator is not callable")
			}
			iteratorValue, err := iteratorFn(obj)
			if err != nil {
				return err
			}
			iterator, ok := iteratorValue.(*goja.Object)
			if !ok {
				return errors.New("HeadersInit iterator is not an object")
			}
			nextFn, ok := goja.AssertFunction(iterator.Get("next"))
			if !ok {
				return errors.New("HeadersInit iterator has no next method")
			}
			for {
				nextValue, err := nextFn(iterator)
				if err != nil {
					return err
				}
				next, ok := nextValue.(*goja.Object)
				if !ok {
					return errors.New("HeadersInit iterator result is not an object")
				}
				if next.Get("done").ToBoolean() {
					return nil
				}
				pair, ok := next.Get("value").(*goja.Object)
				if !ok || pair.ClassName() != "Array" || pair.Get("length").ToInteger() < 2 {
					return errors.New("Headers sequence entries must contain a name and value")
				}
				r.appendFetchHeader(state, pair.Get("0").String(), pair.Get("1").String())
			}
		}
	}
	for _, key := range obj.Keys() {
		r.appendFetchHeader(state, key, obj.Get(key).String())
	}
	return nil
}

func (s *fetchHeadersState) get(name string) string {
	name = strings.ToLower(name)
	for _, entry := range s.entries {
		if entry.name == name {
			return entry.value
		}
	}
	return ""
}

func (s *fetchHeadersState) has(name string) bool {
	name = strings.ToLower(name)
	for _, entry := range s.entries {
		if entry.name == name {
			return true
		}
	}
	return false
}

func isFetchHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, ch := range name {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' {
			continue
		}
		switch ch {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func isFetchHeaderValue(value string) bool {
	for _, ch := range value {
		if ch == 0 || ch == '\r' || ch == '\n' {
			return false
		}
		if ch < 0x20 && ch != '\t' || ch == 0x7f {
			return false
		}
	}
	return true
}

func isForbiddenRequestHeader(name string) bool {
	switch strings.ToLower(name) {
	case "accept-charset", "accept-encoding", "access-control-request-headers", "access-control-request-method",
		"connection", "content-length", "cookie", "cookie2", "date", "dnt", "expect", "host", "keep-alive",
		"origin", "proxy-connection", "set-cookie", "te", "trailer", "transfer-encoding", "upgrade", "via", "user-agent":
		return true
	}
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "proxy-") || strings.HasPrefix(lower, "sec-")
}

func (r *Runtime) appendFetchHeader(state *fetchHeadersState, name, value string) {
	name = strings.ToLower(name)
	value = strings.Trim(value, " \t")
	if !isFetchHeaderName(name) {
		panic(r.vm.NewTypeError("Invalid header name"))
	}
	if !isFetchHeaderValue(value) {
		panic(r.vm.NewTypeError("Invalid header value"))
	}
	if state.guard == "immutable" {
		panic(r.vm.NewTypeError("Headers are immutable"))
	}
	if state.guard == "request" && isForbiddenRequestHeader(name) {
		return
	}
	if state.guard == "response" && name == "set-cookie" {
		return
	}
	for i := range state.entries {
		if state.entries[i].name == name {
			state.entries[i].value += ", " + value
			return
		}
	}
	state.entries = append(state.entries, fetchHeaderEntry{name: name, value: value})
	sort.SliceStable(state.entries, func(i, j int) bool { return state.entries[i].name < state.entries[j].name })
}

func (r *Runtime) deleteFetchHeader(state *fetchHeadersState, name string) {
	if state.guard == "immutable" {
		panic(r.vm.NewTypeError("Headers are immutable"))
	}
	name = strings.ToLower(name)
	entries := state.entries[:0]
	for _, entry := range state.entries {
		if entry.name != name {
			entries = append(entries, entry)
		}
	}
	state.entries = entries
}

func (r *Runtime) setFetchHeader(state *fetchHeadersState, name, value string) {
	name = strings.ToLower(name)
	value = strings.Trim(value, " \t")
	if !isFetchHeaderName(name) {
		panic(r.vm.NewTypeError("Invalid header name"))
	}
	if !isFetchHeaderValue(value) {
		panic(r.vm.NewTypeError("Invalid header value"))
	}
	if state.guard == "immutable" {
		panic(r.vm.NewTypeError("Headers are immutable"))
	}
	if state.guard == "request" && isForbiddenRequestHeader(name) {
		return
	}
	if state.guard == "response" && name == "set-cookie" {
		return
	}
	entries := state.entries[:0]
	for _, entry := range state.entries {
		if entry.name != name {
			entries = append(entries, entry)
		}
	}
	state.entries = append(entries, fetchHeaderEntry{name: name, value: value})
	sort.SliceStable(state.entries, func(i, j int) bool { return state.entries[i].name < state.entries[j].name })
}

func (s *fetchHeadersState) headerMap() map[string]string {
	if len(s.entries) == 0 {
		return nil
	}
	headers := make(map[string]string, len(s.entries))
	for _, entry := range s.entries {
		headers[entry.name] = entry.value
	}
	return headers
}

func (s *fetchHeadersState) clone(guard string) *fetchHeadersState {
	clone := &fetchHeadersState{guard: guard, entries: make([]fetchHeaderEntry, len(s.entries))}
	copy(clone.entries, s.entries)
	return clone
}

func (r *Runtime) fetchHeaderIterator(method string, state *fetchHeadersState) goja.Value {
	values := make([]goja.Value, 0, len(state.entries))
	for _, entry := range state.entries {
		switch method {
		case "keys":
			values = append(values, r.vm.ToValue(entry.name))
		case "values":
			values = append(values, r.vm.ToValue(entry.value))
		default:
			pair := r.vm.NewArray()
			_ = pair.Set("0", entry.name)
			_ = pair.Set("1", entry.value)
			values = append(values, pair)
		}
	}
	iterator := r.vm.NewObject()
	index := 0
	_ = iterator.Set("next", func(goja.FunctionCall) goja.Value {
		if index >= len(values) {
			return r.fetchReadResult(goja.Undefined(), true)
		}
		value := values[index]
		index++
		return r.fetchReadResult(value, false)
	})
	_ = iterator.DefineDataPropertySymbol(goja.SymIterator, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return iterator
	}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	return iterator
}

func (r *Runtime) installRequestConstructor(proto *goja.Object) {
	_ = r.vm.Set("__goosieRequestInit", func(call goja.FunctionCall) goja.Value {
		state, err := r.requestInit(call.Argument(1), call.Argument(2))
		if err != nil {
			panic(r.vm.NewTypeError(err.Error()))
		}
		obj, ok := call.Argument(0).(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("Failed to construct 'Request'"))
		}
		r.ensureFetchObjectState().requests[obj] = state
		return obj
	})
	if _, err := r.vm.RunString(`
		globalThis.Request = (function(init) {
			function Request(input, init) {
				if (!(this instanceof Request)) throw new TypeError("Failed to construct 'Request': Please use the 'new' operator");
				if (arguments.length < 1) throw new TypeError("Failed to construct 'Request': 1 argument required");
				__goosieRequestInit(this, input, init);
			}
			return Request;
		})(__goosieRequestInit);
	`); err != nil {
		panic(err)
	}
	ctor := r.vm.Get("Request").ToObject(r.vm)
	proto = ctor.Get("prototype").ToObject(r.vm)
	r.ensureFetchObjectState().requestProto = proto
	_ = proto.Set("constructor", ctor)
	_ = proto.DefineAccessorProperty("method", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.requestMethodReceiver(call, "method").method)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("url", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.requestMethodReceiver(call, "url").url)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("headers", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		state := r.requestMethodReceiver(call, "headers")
		if state.headersObject == nil {
			state.headersObject = r.newHeadersObject(state.headers)
		}
		return state.headersObject
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("credentials", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.requestMethodReceiver(call, "credentials").credentials)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("signal", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.requestMethodReceiver(call, "signal").signal
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("body", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		body := r.requestMethodReceiver(call, "body").body
		if body == nil || !body.present {
			return goja.Null()
		}
		return r.fetchBodyStream(body)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("bodyUsed", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		body := r.requestMethodReceiver(call, "bodyUsed").body
		return r.vm.ToValue(body != nil && body.present && body.used)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.Set("clone", func(call goja.FunctionCall) goja.Value {
		state := r.requestMethodReceiver(call, "clone")
		if state.body != nil && state.body.present && (state.body.used || state.body.streamLocked) {
			panic(r.vm.NewTypeError("Failed to execute 'clone' on 'Request': Request body has already been consumed"))
		}
		return r.newRequestObject(state.clone())
	})
	r.defineInterfaceToStringTag("Request", proto)
	_ = r.vm.Set("Request", ctor)
	if win, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = win.Set("Request", ctor)
	}
}

func (r *Runtime) requestMethodReceiver(call goja.FunctionCall, method string) *fetchRequestState {
	obj, ok := call.This.(*goja.Object)
	if ok {
		if state := r.ensureFetchObjectState().requests[obj]; state != nil {
			return state
		}
	}
	panic(r.vm.NewTypeError("Failed to execute '" + method + "' on 'Request': Illegal invocation"))
}

func (r *Runtime) requestReceiver(value goja.Value) *fetchRequestState {
	obj, ok := value.(*goja.Object)
	if !ok {
		return nil
	}
	return r.ensureFetchObjectState().requests[obj]
}

func (r *Runtime) newRequestObject(state *fetchRequestState) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(r.ensureFetchObjectState().requestProto)
	r.ensureFetchObjectState().requests[obj] = state
	return obj
}

func (s *fetchRequestState) clone() *fetchRequestState {
	clone := &fetchRequestState{
		url:         s.url,
		method:      s.method,
		headers:     s.headers.clone("request"),
		credentials: s.credentials,
		signal:      s.signal,
	}
	if s.body != nil && s.body.present {
		clone.body = &fetchBodyState{data: s.body.data, present: true}
	}
	return clone
}

func (r *Runtime) requestInit(input, init goja.Value) (*fetchRequestState, error) {
	var source *fetchRequestState
	rawURL := input.String()
	if candidate := r.requestReceiver(input); candidate != nil {
		source = candidate
		rawURL = source.url
	}
	resolvedURL := r.resolveFetchURL(rawURL)
	parsedURL, err := url.Parse(resolvedURL)
	if err != nil || parsedURL.Scheme == "" {
		return nil, errors.New("Failed to parse URL from " + rawURL)
	}

	state := &fetchRequestState{url: resolvedURL, method: "GET", credentials: "same-origin"}
	state.signal = r.newAbortSignal(false, goja.Undefined())
	state.headers = &fetchHeadersState{guard: "request"}
	var inheritedBody *fetchBodyState
	if source != nil {
		state.method = source.method
		state.headers = source.headers.clone("request")
		state.credentials = source.credentials
		state.signal = source.signal
		if source.body != nil && source.body.present {
			if source.body.used || source.body.streamLocked {
				return nil, errors.New("Cannot construct a Request with a Request object that has already been used")
			}
			inheritedBody = source.body
			state.body = &fetchBodyState{data: inheritedBody.data, present: true}
		}
	}

	initObj, ok := init.(*goja.Object)
	if !ok {
		initObj = nil
	}
	bodyOverridden := false
	if initObj != nil {
		if value := initObj.Get("method"); value != nil && !goja.IsUndefined(value) {
			state.method = strings.ToUpper(value.String())
			if state.method == "" {
				state.method = "GET"
			}
		}
		if value := initObj.Get("headers"); value != nil && !goja.IsUndefined(value) {
			headers, err := r.headersFromInit(value, "request")
			if err != nil {
				return nil, err
			}
			state.headers = r.headersReceiver(headers)
		}
		if value := initObj.Get("credentials"); value != nil && !goja.IsUndefined(value) {
			state.credentials = strings.ToLower(value.String())
		}
		if value := initObj.Get("signal"); value != nil && !goja.IsUndefined(value) {
			if goja.IsNull(value) {
				state.signal = r.newAbortSignal(false, goja.Undefined())
			} else {
				signal, err := r.validateFetchSignal(value)
				if err != nil {
					return nil, err
				}
				state.signal = signal
			}
		}
		if value := initObj.Get("body"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
			body, mime, err := r.fetchBodyFromInit(value)
			if err != nil {
				return nil, err
			}
			state.body = body
			bodyOverridden = true
			if mime != "" && !state.headers.has("content-type") {
				r.appendFetchHeader(state.headers, "content-type", mime)
			}
		}
	}
	switch state.credentials {
	case "omit", "same-origin", "include":
	default:
		return nil, errors.New("Request credentials must be one of omit, same-origin, or include")
	}
	if state.body != nil && state.body.present && (state.method == "GET" || state.method == "HEAD") {
		return nil, errors.New("Request with GET/HEAD method cannot have body")
	}
	if inheritedBody != nil && !bodyOverridden {
		inheritedBody.used = true
		inheritedBody.streamLocked = true
	}
	return state, nil
}

func (r *Runtime) fetchBodyFromInit(value goja.Value) (*fetchBodyState, string, error) {
	data, mime, present, err := r.fetchBodyBytes(value)
	if err != nil {
		return nil, "", err
	}
	return &fetchBodyState{data: data, present: present}, mime, nil
}

func (r *Runtime) fetchBodyBytes(value goja.Value) ([]byte, string, bool, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, "", false, nil
	}
	if object, ok := value.(*goja.Object); ok {
		if file := r.fileValues[object]; file != nil {
			return bytes.Clone(file.data), file.typeValue, true, nil
		}
		if list := r.formDataValues[object]; list != nil {
			boundary, data, err := r.encodeMultipartFormData(list, "UTF-8")
			if err != nil {
				return nil, "", false, err
			}
			return data, "multipart/form-data; boundary=" + boundary, true, nil
		}
		if r.isURLSearchParams(value) {
			return []byte(value.String()), "application/x-www-form-urlencoded;charset=UTF-8", true, nil
		}
		if buffer, ok := object.Export().(goja.ArrayBuffer); ok {
			if buffer.Detached() {
				return nil, "", false, errors.New("Request body buffer is detached")
			}
			return bytes.Clone(buffer.Bytes()), "", true, nil
		}
		if bufferValue := object.Get("buffer"); bufferValue != nil && !goja.IsUndefined(bufferValue) {
			if buffer, ok := bufferValue.Export().(goja.ArrayBuffer); ok {
				if buffer.Detached() {
					return nil, "", false, errors.New("Request body buffer is detached")
				}
				offset := int(object.Get("byteOffset").ToInteger())
				length := int(object.Get("byteLength").ToInteger())
				data := buffer.Bytes()
				if offset < 0 || length < 0 || offset > len(data) || length > len(data)-offset {
					return nil, "", false, errors.New("Request body view is out of bounds")
				}
				return bytes.Clone(data[offset : offset+length]), "", true, nil
			}
		}
	}
	return []byte(value.String()), "text/plain;charset=UTF-8", true, nil
}

func (r *Runtime) isURLSearchParams(value goja.Value) bool {
	constructor := r.vm.Get("URLSearchParams")
	if constructor == nil || goja.IsUndefined(constructor) {
		return false
	}
	prototype := constructor.ToObject(r.vm).Get("prototype").ToObject(r.vm)
	for current := value.ToObject(r.vm).Prototype(); current != nil; current = current.Prototype() {
		if current == prototype {
			return true
		}
	}
	return false
}

func (r *Runtime) validateFetchSignal(value goja.Value) (*goja.Object, error) {
	signal, ok := value.(*goja.Object)
	if !ok {
		return nil, errors.New(fetchSignalInvalidMessage)
	}
	if _, ok := goja.AssertFunction(signal.Get("__registerAbortAlgorithm__")); !ok {
		return nil, errors.New(fetchSignalInvalidMessage)
	}
	return signal, nil
}

func (r *Runtime) followFetchSignalValue(value goja.Value) (*fetchAbort, goja.Value) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, nil
	}
	signal, err := r.validateFetchSignal(value)
	if err != nil {
		return nil, r.invalidFetchSignalError()
	}
	control := newFetchAbort()
	reason := func() goja.Value {
		if v := signal.Get("reason"); v != nil {
			return v
		}
		return r.newAbortErrorDOMException()
	}
	if signal.Get("aborted").ToBoolean() {
		control.fire(reason())
		return control, nil
	}
	register, _ := goja.AssertFunction(signal.Get("__registerAbortAlgorithm__"))
	_, _ = register(signal, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		control.fire(reason())
		return goja.Undefined()
	}))
	return control, nil
}

func (r *Runtime) fetchBodyStream(body *fetchBodyState) *goja.Object {
	if body.stream != nil {
		return body.stream
	}
	stream := r.vm.NewObject()
	if constructor := r.vm.Get("ReadableStream"); constructor != nil && !goja.IsUndefined(constructor) {
		if prototype := constructor.ToObject(r.vm).Get("prototype"); prototype != nil && !goja.IsUndefined(prototype) {
			_ = stream.SetPrototype(prototype.ToObject(r.vm))
		}
	}
	locked := false
	_ = stream.DefineAccessorProperty("locked", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(body.streamLocked)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = stream.Set("cancel", func(goja.FunctionCall) goja.Value {
		body.used = true
		thenable := r.newThenable()
		thenable.resolve(goja.Undefined())
		return thenable.obj
	})
	_ = stream.Set("getReader", func(call goja.FunctionCall) goja.Value {
		if body.streamLocked {
			panic(r.vm.NewTypeError("ReadableStream is locked"))
		}
		byob := false
		if len(call.Arguments) > 0 && !goja.IsNull(call.Argument(0)) && !goja.IsUndefined(call.Argument(0)) {
			options := call.Argument(0).ToObject(r.vm)
			mode := options.Get("mode")
			if mode != nil && !goja.IsUndefined(mode) {
				if mode.String() != "byob" {
					panic(r.vm.NewTypeError("Unsupported reader mode"))
				}
				byob = true
			}
		}
		body.streamLocked = true
		locked = true
		offset := 0
		reader := r.vm.NewObject()
		_ = reader.Set("read", func(call goja.FunctionCall) goja.Value {
			thenable := r.newThenable()
			if !locked {
				thenable.reject(r.vm.NewTypeError("Reader lock was released"))
				return thenable.obj
			}
			body.used = true
			if byob {
				if len(call.Arguments) == 0 {
					thenable.reject(r.vm.NewTypeError("BYOB read requires a view"))
					return thenable.obj
				}
				view, ok := call.Argument(0).(*goja.Object)
				if !ok || view.Get("byteLength").ToInteger() == 0 {
					thenable.reject(r.vm.NewTypeError("BYOB read requires a non-empty Uint8Array"))
					return thenable.obj
				}
				if offset >= len(body.data) {
					thenable.resolve(r.fetchReadResult(goja.Undefined(), true))
					return thenable.obj
				}
				count := min(int(view.Get("byteLength").ToInteger()), len(body.data)-offset)
				for i := 0; i < count; i++ {
					_ = view.Set(fmt.Sprintf("%d", i), int(body.data[offset+i]))
				}
				offset += count
				subarray, ok := goja.AssertFunction(view.Get("subarray"))
				if !ok {
					thenable.reject(r.vm.NewTypeError("BYOB read view has no subarray method"))
					return thenable.obj
				}
				result, err := subarray(view, r.vm.ToValue(0), r.vm.ToValue(count))
				if err != nil {
					thenable.reject(r.vm.NewGoError(err))
					return thenable.obj
				}
				thenable.resolve(r.fetchReadResult(result, false))
				return thenable.obj
			}
			if offset >= len(body.data) {
				thenable.resolve(r.fetchReadResult(goja.Undefined(), true))
				return thenable.obj
			}
			end := min(offset+65536, len(body.data))
			chunk := bytes.Clone(body.data[offset:end])
			offset = end
			thenable.resolve(r.fetchReadResult(r.newUint8Array(chunk), false))
			return thenable.obj
		})
		_ = reader.Set("cancel", func(goja.FunctionCall) goja.Value {
			body.used = true
			offset = len(body.data)
			thenable := r.newThenable()
			thenable.resolve(goja.Undefined())
			return thenable.obj
		})
		_ = reader.Set("releaseLock", func(goja.FunctionCall) goja.Value {
			if locked {
				locked = false
				body.streamLocked = false
			}
			return goja.Undefined()
		})
		return reader
	})
	body.stream = stream
	return stream
}

func (r *Runtime) fetchReadResult(value goja.Value, done bool) goja.Value {
	result := r.vm.NewObject()
	_ = result.Set("value", value)
	_ = result.Set("done", done)
	return result
}

func (r *Runtime) consumeFetchBody(body *fetchBodyState) ([]byte, error) {
	if body == nil || !body.present {
		return nil, nil
	}
	if body.used {
		return nil, errors.New("Response body has already been consumed")
	}
	if body.streamLocked {
		return nil, errors.New("Response body is locked")
	}
	body.used = true
	body.streamLocked = true
	return body.data, nil
}

func (r *Runtime) setupResponse() {
	r.setupFetchObjects()
	_ = r.vm.Set("__goosieResponseInit", func(call goja.FunctionCall) goja.Value {
		data, mime, present, err := r.fetchBodyBytes(call.Argument(1))
		if err != nil {
			panic(r.vm.NewTypeError(err.Error()))
		}
		status, statusText := 200, "OK"
		headers := make(map[string]string)
		if len(call.Arguments) > 2 && !goja.IsNull(call.Argument(2)) && !goja.IsUndefined(call.Argument(2)) {
			init := call.Argument(2).ToObject(r.vm)
			if value := init.Get("status"); value != nil && !goja.IsUndefined(value) {
				status = int(value.ToInteger())
			}
			if status < 200 || status > 599 {
				panic(r.newRangeError("Response status must be between 200 and 599"))
			}
			if value := init.Get("statusText"); value != nil && !goja.IsUndefined(value) {
				statusText = value.String()
			}
			if value := init.Get("headers"); value != nil && !goja.IsUndefined(value) {
				headersObject, err := r.headersFromInit(value, "response")
				if err != nil {
					panic(r.vm.NewTypeError(err.Error()))
				}
				for key, value := range r.headersReceiver(headersObject).headerMap() {
					headers[key] = value
				}
			}
		}
		if GetHeader(headers, "Content-Type") == "" && mime != "" {
			headers["Content-Type"] = mime
		}
		obj, ok := call.Argument(0).(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("Failed to construct 'Response'"))
		}
		_ = obj.SetPrototype(r.responseProto)
		r.ensureFetchObjectState().responseBodies[obj] = r.newResponseState(status, statusText, "", headers, data, present, "response")
		return obj
	})
	if _, err := r.vm.RunString(`
		globalThis.Response = (function(init) {
			function Response(body, init) {
				if (!(this instanceof Response)) throw new TypeError("Failed to construct 'Response': Please use the 'new' operator");
				__goosieResponseInit(this, body, init);
			}
			return Response;
		})(__goosieResponseInit);
	`); err != nil {
		panic(err)
	}
	ctor := r.vm.Get("Response").ToObject(r.vm)
	prototype := ctor.Get("prototype").ToObject(r.vm)
	r.responseProto = prototype
	_ = prototype.Set("constructor", ctor)
	_ = prototype.DefineAccessorProperty("status", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.responseMethodReceiver(call, "status").status)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = prototype.DefineAccessorProperty("statusText", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.responseMethodReceiver(call, "statusText").statusText)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = prototype.DefineAccessorProperty("ok", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		status := r.responseMethodReceiver(call, "ok").status
		return r.vm.ToValue(status >= 200 && status <= 299)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = prototype.DefineAccessorProperty("type", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.responseMethodReceiver(call, "type")
		return r.vm.ToValue("default")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = prototype.DefineAccessorProperty("url", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.responseMethodReceiver(call, "url").url)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = prototype.DefineAccessorProperty("redirected", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.responseMethodReceiver(call, "redirected")
		return r.vm.ToValue(false)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = prototype.DefineAccessorProperty("headers", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		state := r.responseMethodReceiver(call, "headers")
		if state.headersObject == nil {
			state.headersObject = r.newHeadersObject(state.headers.clone(state.headers.guard))
		}
		return state.headersObject
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = prototype.DefineAccessorProperty("body", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		body := r.responseMethodReceiver(call, "body").body
		if body == nil || !body.present {
			return goja.Null()
		}
		return r.fetchBodyStream(body)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = prototype.DefineAccessorProperty("bodyUsed", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		body := r.responseMethodReceiver(call, "bodyUsed").body
		return r.vm.ToValue(body != nil && body.present && body.used)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = prototype.Set("clone", func(call goja.FunctionCall) goja.Value {
		state := r.responseMethodReceiver(call, "clone")
		if state.body != nil && state.body.present && (state.body.used || state.body.streamLocked) {
			panic(r.vm.NewTypeError("Failed to execute 'clone' on 'Response': Response body has already been consumed"))
		}
		clone := &fetchResponseState{
			status:     state.status,
			statusText: state.statusText,
			url:        state.url,
			headers:    state.headers.clone(state.headers.guard),
		}
		if state.body != nil && state.body.present {
			clone.body = &fetchBodyState{data: state.body.data, present: true}
		}
		return r.responseStateObject(clone)
	})
	_ = prototype.Set("arrayBuffer", func(call goja.FunctionCall) goja.Value {
		state := r.responseMethodReceiver(call, "arrayBuffer")
		data, err := r.consumeFetchBody(state.body)
		if err != nil {
			return r.rejectedFetchThenable(err)
		}
		thenable := r.newThenable()
		thenable.resolve(r.vm.ToValue(r.vm.NewArrayBuffer(bytes.Clone(data))))
		return thenable.obj
	})
	_ = prototype.Set("blob", func(call goja.FunctionCall) goja.Value {
		state := r.responseMethodReceiver(call, "blob")
		data, err := r.consumeFetchBody(state.body)
		if err != nil {
			return r.rejectedFetchThenable(err)
		}
		thenable := r.newThenable()
		thenable.resolve(r.vm.ToValue(r.makeBlob(nil, bytes.Clone(data), normalizeMIMEType(state.headers.get("content-type")), false, "", 0)))
		return thenable.obj
	})
	_ = prototype.Set("text", func(call goja.FunctionCall) goja.Value {
		state := r.responseMethodReceiver(call, "text")
		data, err := r.consumeFetchBody(state.body)
		if err != nil {
			return r.rejectedFetchThenable(err)
		}
		thenable := r.newThenable()
		thenable.resolve(r.vm.ToValue(decodeBlobText(data)))
		return thenable.obj
	})
	_ = prototype.Set("json", func(call goja.FunctionCall) goja.Value {
		state := r.responseMethodReceiver(call, "json")
		data, err := r.consumeFetchBody(state.body)
		if err != nil {
			return r.rejectedFetchThenable(err)
		}
		thenable := r.newThenable()
		var parsed interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			thenable.reject(r.vm.ToValue("JSON parse error: " + err.Error()))
		} else {
			thenable.resolve(r.vm.ToValue(parsed))
		}
		return thenable.obj
	})
	r.defineInterfaceToStringTag("Response", prototype)
	_ = r.vm.Set("Response", ctor)
	if win, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = win.Set("Response", ctor)
	}
}

func (r *Runtime) responseMethodReceiver(call goja.FunctionCall, method string) *fetchResponseState {
	obj, ok := call.This.(*goja.Object)
	if ok {
		if state := r.ensureFetchObjectState().responseBodies[obj]; state != nil {
			return state
		}
	}
	panic(r.vm.NewTypeError("Failed to execute '" + method + "' on 'Response': Illegal invocation"))
}

func (r *Runtime) newResponseState(status int, statusText, responseURL string, headers map[string]string, body []byte, present bool, headersGuard string) *fetchResponseState {
	state := &fetchResponseState{
		status:     status,
		statusText: statusText,
		url:        responseURL,
		headers:    &fetchHeadersState{guard: headersGuard},
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		state.headers.entries = append(state.headers.entries, fetchHeaderEntry{name: strings.ToLower(key), value: headers[key]})
	}
	sort.SliceStable(state.headers.entries, func(i, j int) bool { return state.headers.entries[i].name < state.headers.entries[j].name })
	if present {
		state.body = &fetchBodyState{data: body, present: true}
	}
	return state
}

func (r *Runtime) newResponseObject(status int, statusText, responseURL string, headers map[string]string, body []byte, present bool) *goja.Object {
	return r.responseStateObject(r.newResponseState(status, statusText, responseURL, headers, body, present, "immutable"))
}

func (r *Runtime) responseStateObject(state *fetchResponseState) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(r.responseProto)
	r.ensureFetchObjectState().responseBodies[obj] = state
	return obj
}

func (r *Runtime) rejectedFetchThenable(err error) goja.Value {
	thenable := r.newThenable()
	thenable.reject(r.vm.NewTypeError(err.Error()))
	return thenable.obj
}
