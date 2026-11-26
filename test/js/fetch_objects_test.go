package js_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestFetchHeadersAndRequestConstructors(t *testing.T) {
	r := newFetchRuntime(t, &mockFetcher{})
	if err := r.Run(`
		var constructorShape = [
			typeof Headers,
			typeof Request,
			new Headers() instanceof Headers,
			new Request('/request') instanceof Request,
			new Request('/request').method,
			new Request('/request').url,
			new Request('/request').credentials,
			typeof new Request('/request').signal.throwIfAborted,
			new Request('/request').body === null,
			new Request('/request').bodyUsed
		].join('|');
		var headerShape = '';
		var headers = new Headers([['X-A', '1'], ['x-a', '2']]);
		headers.append('X-B', '3');
		headers.set('x-b', '4');
		headerShape = [headers.get('X-A'), headers.get('x-b'), headers.has('X-B'), headers.keys().next().value, JSON.stringify([...headers])].join('|');
		var headerForeach = '';
		var headerTrim = new Headers({'X-Space': '  value  '}).get('x-space');
		headers.forEach(function(value, key, owner) { headerForeach += key + '=' + value + ':' + (owner === headers) + ';'; });
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("constructorShape"); !ok || value.Str != "function|function|true|true|GET|https://example.test/request|same-origin|function|true|false" {
		t.Fatalf("constructor shape = %+v", value)
	}
	if value, ok := r.Global("headerShape"); !ok || value.Str != "1, 2|4|true|x-a|[[\"x-a\",\"1, 2\"],[\"x-b\",\"4\"]]" {
		t.Fatalf("header shape = %+v", value)
	}
	if value, ok := r.Global("headerForeach"); !ok || value.Str != "x-a=1, 2:true;x-b=4:true;" {
		t.Fatalf("header forEach = %+v", value)
	}
	if value, ok := r.Global("headerTrim"); !ok || value.Str != "value" {
		t.Fatalf("trimmed header = %+v", value)
	}
}

func TestFetchObjectReceiverErrors(t *testing.T) {
	r := newFetchRuntime(t, &mockFetcher{})
	if err := r.Run(`
		var receiverErrors = [];
		var checks = [
			function() { Headers(); },
			function() { Headers.prototype.get.call({}, 'x'); },
			function() { Headers.prototype.append.call({}, 'x', 'y'); },
			function() { Request(); },
			function() { Object.getOwnPropertyDescriptor(Request.prototype, 'method').get.call({}); },
			function() { Request.prototype.clone.call({}); },
			function() { Object.getOwnPropertyDescriptor(Response.prototype, 'bodyUsed').get.call({}); },
			function() { Response.prototype.clone.call({}); },
			function() { Response.prototype.text.call({}); }
		];
		for (var i = 0; i < checks.length; i++) {
			try { checks[i](); receiverErrors.push('none'); }
			catch (error) { receiverErrors.push(error instanceof TypeError); }
		}
		receiverErrors = receiverErrors.join('|');
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	value, ok := r.Global("receiverErrors")
	if !ok || value.Str != "true|true|true|true|true|true|true|true|true" {
		t.Fatalf("receiver errors = %+v, want TypeErrors", value)
	}
}

func TestFetchRequestPreservesMethodHeadersAndBody(t *testing.T) {
	m := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/request": {Status: 200, Body: []byte("ok"), OK: true},
	}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var requestFetchText = '';
		var requestFetchBodyUsed = '';
		var request = new Request('/request', {method: 'post', body: 'payload', headers: {'X-Test': 'yes'}});
		var requestBodyBeforeFetch = request.bodyUsed;
		fetch(request).then(function(response) {
			response.text().then(function(text) { requestFetchText = text; });
		});
		requestFetchBodyUsed = request.bodyUsed;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	call := m.lastCall()
	if call.Method != "POST" || string(call.Body) != "payload" || js.GetHeader(call.Headers, "X-Test") != "yes" {
		t.Fatalf("fetch(Request) call = %+v", call)
	}
	if value, ok := r.Global("requestBodyBeforeFetch"); !ok || value.Bool {
		t.Fatalf("request bodyUsed before fetch = %+v, want false", value)
	}
	if value, ok := r.Global("requestFetchBodyUsed"); !ok || !value.Bool {
		t.Fatalf("request bodyUsed after fetch = %+v, want true", value)
	}
	if value, ok := r.Global("requestFetchText"); !ok || value.Str != "ok" {
		t.Fatalf("fetch(Request) text = %+v", value)
	}
}

func TestFetchRequestInitOverrides(t *testing.T) {
	m := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/override": {Status: 200, Body: []byte("ok"), OK: true},
	}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var overrideStatus = 0;
		var overrideSourceUsed = false;
		var oldController = new AbortController();
		oldController.abort('old');
		var request = new Request('/override', {
			method: 'POST', body: 'old', headers: {'X-Old': 'old'}, signal: oldController.signal
		});
		fetch(request, {
			method: 'PUT', body: 'new', headers: {'X-New': 'new'}, signal: new AbortController().signal
		}).then(function(response) { overrideStatus = response.status; });
		overrideSourceUsed = request.bodyUsed;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	call := m.lastCall()
	if call.Method != "PUT" || string(call.Body) != "new" || js.GetHeader(call.Headers, "X-New") != "new" || js.GetHeader(call.Headers, "X-Old") != "" {
		t.Fatalf("fetch(Request, init) call = %+v", call)
	}
	if value, ok := r.Global("overrideStatus"); !ok || value.Num != 200 {
		t.Fatalf("override status = %+v, want 200", value)
	}
	if value, ok := r.Global("overrideSourceUsed"); !ok || value.Bool {
		t.Fatalf("overridden source bodyUsed = %+v, want false", value)
	}
}

func TestFetchRequestConstructorValidation(t *testing.T) {
	r := newFetchRuntime(t, &mockFetcher{})
	if err := r.Run(`
		var requestValidation = [];
		var invalidInits = [
			{credentials: 'invalid'},
			{signal: 'invalid'},
			{method: 'GET', body: 'body'}
		];
		for (var i = 0; i < invalidInits.length; i++) {
			try { new Request('/invalid', invalidInits[i]); requestValidation.push('none'); }
			catch (error) { requestValidation.push(error instanceof TypeError); }
		}
		requestValidation = requestValidation.join('|');
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("requestValidation"); !ok || value.Str != "true|true|true" {
		t.Fatalf("Request constructor validation = %+v", value)
	}
}

func TestFetchRequestConstructorBodyRules(t *testing.T) {
	m := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/body-rules": {Status: 200, OK: true},
	}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var requestBodyShape = '';
		var requestGetBodyError = '';
		var request = new Request('/body-rules', {method: 'POST', body: new Blob(['abc'], {type: 'text/plain'})});
		var clone = request.clone();
		var derived = new Request(request);
		requestBodyShape = [request.headers.get('content-type'), request.bodyUsed, clone.bodyUsed, derived.bodyUsed, derived.method].join('|');
		try { new Request('/body-rules', {method: 'GET', body: 'x'}); }
		catch (error) { requestGetBodyError = error.name; }
		fetch(derived).then(function() {});
		fetch(request).catch(function(error) { requestBodyShape += '|' + error.name; });
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	if value, ok := r.Global("requestBodyShape"); !ok || value.Str != "text/plain|true|false|false|POST|TypeError" {
		t.Fatalf("Request body shape = %+v", value)
	}
	if value, ok := r.Global("requestGetBodyError"); !ok || value.Str != "TypeError" {
		t.Fatalf("GET Request body error = %+v", value)
	}
	if calls := m.callCount(); calls != 1 || string(m.lastCall().Body) != "abc" {
		t.Fatalf("Request body calls = %d last=%+v", calls, m.lastCall())
	}
}

func TestFetchResponseBodyLockRejectsConsumption(t *testing.T) {
	r := newFetchRuntime(t, &mockFetcher{})
	if err := r.Run(`
		var bodyLockShape = '';
		var response = new Response('locked');
		var reader = response.body.getReader();
		var cloneError = 'none';
		try { response.clone(); } catch (error) { cloneError = error.name; }
		response.text().then(function() { bodyLockShape = 'resolved'; }, function(error) {
			bodyLockShape = [cloneError, error.name, response.bodyUsed, response.body.locked].join('|');
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("bodyLockShape"); !ok || value.Str != "TypeError|TypeError|false|true" {
		t.Fatalf("locked Response body shape = %+v", value)
	}
}

func TestFetchRequestSignalAndAbort(t *testing.T) {
	gf := &gatedFetcher{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		resp:    &js.FetchResponse{Status: 200, Body: []byte("late"), OK: true},
	}
	r := newFetchRuntime(t, gf)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gf.release) }) }
	t.Cleanup(release)
	if err := r.Run(`
		var requestAbortState = 'pending';
		var requestController = new AbortController();
		var request = new Request('/slow', {signal: requestController.signal});
		fetch(request).then(
			function() { requestAbortState = 'resolved'; },
			function(error) { requestAbortState = 'rejected:' + error; }
		);
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gf.started:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch(Request) never reached the HTTP client")
	}
	if err := r.Run(`requestController.abort('request-stop');`, "inline"); err != nil {
		t.Fatal(err)
	}
	waitAbortedDrain(t, r)
	if value, ok := r.Global("requestAbortState"); !ok || value.Str != "rejected:request-stop" {
		t.Fatalf("fetch(Request) abort state = %+v", value)
	}
	release()
	drainFetch(t, r)
}

func TestFetchRequestSignalOverrideClearsAbort(t *testing.T) {
	m := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/clear": {Status: 200, Body: []byte("ok"), OK: true},
	}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var clearAbortText = '';
		var controller = new AbortController();
		controller.abort('old');
		var request = new Request('/clear', {signal: controller.signal});
		fetch(request, {signal: null}).then(function(response) {
			response.text().then(function(text) { clearAbortText = text; });
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	if value, ok := r.Global("clearAbortText"); !ok || value.Str != "ok" {
		t.Fatalf("fetch(Request, {signal:null}) = %+v", value)
	}
}

func TestFetchRequestForbiddenHeaders(t *testing.T) {
	m := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/headers": {Status: 200, OK: true},
	}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var forbiddenHeaderShape = '';
		var loose = new Headers({Cookie: 'secret', Origin: 'https://evil.test', 'Sec-Fetch-Site': 'cross-site', 'X-A': '1'});
		var guarded = new Request('/headers', {headers: loose});
		forbiddenHeaderShape = [loose.get('cookie'), guarded.headers.get('cookie'), guarded.headers.get('origin'), guarded.headers.get('sec-fetch-site'), guarded.headers.get('x-a')].join('|');
		fetch(guarded).then(function() {});
		fetch('/headers', {headers: {Cookie: 'plain-secret', Origin: 'https://evil.test', 'X-B': '2'}}).then(function() {});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	if value, ok := r.Global("forbiddenHeaderShape"); !ok || value.Str != "secret||||1" {
		t.Fatalf("forbidden header shape = %+v", value)
	}
	calls := m.calls
	if len(calls) != 2 {
		t.Fatalf("fetch calls = %d, want 2", len(calls))
	}
	for _, call := range calls {
		for _, name := range []string{"Cookie", "cookie", "Origin", "origin", "Sec-Fetch-Site", "sec-fetch-site"} {
			if _, exists := call.Headers[name]; exists {
				t.Errorf("forbidden header %q reached transport: %v", name, call.Headers)
			}
		}
	}
}

func TestFetchRequestCORSAndCredentials(t *testing.T) {
	origin := "https://example.test"
	m := &methodFetcher{responses: map[string]map[string]*js.FetchResponse{
		"https://other.test/cors": {
			"OPTIONS": {
				Status: 204,
				Headers: map[string]string{
					"Access-Control-Allow-Origin":      origin,
					"Access-Control-Allow-Credentials": "true",
					"Access-Control-Allow-Methods":     "POST",
					"Access-Control-Allow-Headers":     "Origin, X-Token",
				},
			},
			"POST": {
				Status: 200, OK: true, Body: []byte("cors-body"),
				Headers: map[string]string{
					"Access-Control-Allow-Origin":      origin,
					"Access-Control-Allow-Credentials": "true",
				},
			},
		},
	}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var requestCorsBody = '';
		var request = new Request('https://other.test/cors', {
			method: 'POST', body: 'cors', credentials: 'include', headers: {'X-Token': 'token'}
		});
		fetch(request).then(function(response) {
			response.text().then(function(text) { requestCorsBody = text; });
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	if value, ok := r.Global("requestCorsBody"); !ok || value.Str != "cors-body" {
		t.Fatalf("fetch(Request) CORS body = %+v, calls=%v", value, m.calls)
	}
	if len(m.calls) != 2 || !strings.HasPrefix(m.calls[0], "OPTIONS ") {
		t.Fatalf("fetch(Request) CORS calls = %v, want preflight then POST", m.calls)
	}
}

func TestFetchRequestCredentialsOverride(t *testing.T) {
	m := &methodFetcher{responses: map[string]map[string]*js.FetchResponse{
		"https://other.test/override-creds": {
			"OPTIONS": {
				Status: 204,
				Headers: map[string]string{
					"Access-Control-Allow-Origin":  "*",
					"Access-Control-Allow-Methods": "POST",
					"Access-Control-Allow-Headers": "Origin, X-Token",
				},
			},
			"POST": {
				Status: 200, OK: true, Body: []byte("omit"),
				Headers: map[string]string{"Access-Control-Allow-Origin": "*"},
			},
		},
	}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var credentialsOverrideBody = '';
		var request = new Request('https://other.test/override-creds', {
			method: 'POST', body: 'body', credentials: 'include', headers: {'X-Token': 'token'}
		});
		fetch(request, {credentials: 'omit'}).then(function(response) {
			response.text().then(function(text) { credentialsOverrideBody = text; });
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	if value, ok := r.Global("credentialsOverrideBody"); !ok || value.Str != "omit" {
		t.Fatalf("credentials override body = %+v, calls=%v", value, m.calls)
	}
}

func TestFetchResponseHeadersGuard(t *testing.T) {
	m := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/response-headers": {Status: 200, OK: true},
	}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var responseHeadersShape = '';
		var constructed = new Response('x');
		constructed.headers.set('X-Test', 'constructed');
		var constructedValue = constructed.headers.get('x-test');
		fetch('/response-headers').then(function(response) {
			var fetchedError = 'none';
			try { response.headers.set('X-Test', 'fetched'); }
			catch (error) { fetchedError = error.name; }
			responseHeadersShape = [constructedValue, fetchedError].join('|');
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	if value, ok := r.Global("responseHeadersShape"); !ok || value.Str != "constructed|TypeError" {
		t.Fatalf("Response headers guard = %+v", value)
	}
}

func TestFetchResponseCloneAndBodyUsed(t *testing.T) {
	r := newFetchRuntime(t, &mockFetcher{})
	if err := r.Run(`
		var cloneShape = '';
		var responseCloneAfterUse = 'none';
		var response = new Response('original');
		var clone = response.clone();
		var beforeClone = [response.bodyUsed, clone.bodyUsed, response.body === response.body, clone.body instanceof ReadableStream].join('|');
		clone.text().then(function(text) {
			var secondUse = 'none';
			var lateClone = 'none';
			try { response.clone(); } catch (error) { responseCloneAfterUse = error.name; }
			try { clone.clone(); } catch (error) { lateClone = error.name; }
			clone.text().then(function() { secondUse = 'none'; }, function(error) { secondUse = error.name; });
			response.text().then(function(original) {
				cloneShape = [beforeClone, text, original, response.bodyUsed, clone.bodyUsed, responseCloneAfterUse, lateClone, secondUse].join('|');
			});
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("cloneShape"); !ok || value.Str != "false|false|true|true|original|original|true|true|none|TypeError|TypeError" {
		t.Fatalf("response clone shape = %+v", value)
	}
}

func TestFetchResponseNullBodyClone(t *testing.T) {
	r := newFetchRuntime(t, &mockFetcher{})
	if err := r.Run(`
		var nullBodyShape = '';
		var response = new Response(null);
		var clone = response.clone();
		clone.text().then(function(text) {
			nullBodyShape = [response.body, clone.body, response.bodyUsed, clone.bodyUsed, text].join('|');
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("nullBodyShape"); !ok || value.Str != "||false|false|" {
		t.Fatalf("null body clone shape = %+v", value)
	}
}

func TestFetchResponseBufferedBodyStreamMarksUsed(t *testing.T) {
	r := newFetchRuntime(t, &mockFetcher{})
	if err := r.Run(`
		var streamShape = '';
		var response = new Response('stream-bytes');
		var reader = response.body.getReader();
		var beforeRead = [response.body.locked, response.bodyUsed].join('|');
		reader.read().then(function(result) {
			var cloneError = 'none';
			try { response.clone(); } catch (error) { cloneError = error.name; }
			var bytes = Array.from(result.value).join(',');
			streamShape = [beforeRead, result.done, bytes, response.bodyUsed, cloneError].join('|');
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("streamShape"); !ok || value.Str != "true|false|false|115,116,114,101,97,109,45,98,121,116,101,115|true|TypeError" {
		t.Fatalf("buffered body stream shape = %+v", value)
	}
}

func TestFetchResponseArrayBufferAndBlob(t *testing.T) {
	m := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/binary": {
			Status: 200, OK: true, Body: []byte{0, 255, 65, 66},
			Headers: map[string]string{"Content-Type": "Text/Plain; Charset=UTF-8"},
		},
	}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var binaryShape = '';
		var binaryError = '';
		fetch('/binary').then(function(response) {
			var bufferResponse = response.clone();
			var blobResponse = response.clone();
			bufferResponse.arrayBuffer().then(function(buffer) {
				var bytes = Array.from(new Uint8Array(buffer)).join(',');
				blobResponse.blob().then(function(blob) {
					blob.arrayBuffer().then(function(blobBuffer) {
						binaryShape = [bytes, blob.type, blob.size, Array.from(new Uint8Array(blobBuffer)).join(','), bufferResponse.bodyUsed, blobResponse.bodyUsed, response.bodyUsed].join('|');
					}).catch(function(error) { binaryError = 'blob-buffer:' + error; });
				}).catch(function(error) { binaryError = 'blob:' + error; });
			}).catch(function(error) { binaryError = 'buffer:' + error; });
		}).catch(function(error) { binaryError = 'fetch:' + error; });
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	r.Tick()
	if value, ok := r.Global("binaryShape"); !ok || value.Str != "0,255,65,66|text/plain; charset=utf-8|4|0,255,65,66|true|true|false" {
		errorValue, _ := r.Global("binaryError")
		t.Fatalf("arrayBuffer/blob shape = %+v error=%+v", value, errorValue)
	}
}

func TestFetchResponseCloneKeepsSubresourceAccounting(t *testing.T) {
	body := []byte("shared-buffer")
	m := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/shared": {Status: 200, OK: true, Body: body},
	}}
	budget := &fakeSubresourceBudget{cap: len(body)}
	r := newFetchRuntime(t, m)
	r.SetSubresourceBudget(budget)
	if err := r.Run(`
		var sharedCloneText = '';
		fetch('/shared').then(function(response) {
			var clone = response.clone();
			clone.text().then(function(text) { sharedCloneText = text; });
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	if value, ok := r.Global("sharedCloneText"); !ok || value.Str != "shared-buffer" {
		t.Fatalf("shared clone text = %+v", value)
	}
	if used := budget.Used(); used != len(body) {
		t.Fatalf("subresource budget used = %d, want %d for the shared immutable buffer", used, len(body))
	}
}

func TestFetchRequestBodyLimit(t *testing.T) {
	m := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/large": {Status: 200, OK: true},
	}}
	r := newFetchRuntime(t, m)
	script := fmt.Sprintf(`
		var largeRequestBodyError = '';
		var request = new Request('/large', {method: 'POST', body: 'x'.repeat(%d)});
		fetch(request).catch(function(error) { largeRequestBodyError = String(error); });
	`, js.MaxFetchBodyBytes+1)
	if err := r.Run(script, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	if value, ok := r.Global("largeRequestBodyError"); !ok || !strings.Contains(value.Str, "request body exceeds limit") {
		t.Fatalf("large Request body error = %+v", value)
	}
	if calls := m.callCount(); calls != 0 {
		t.Fatalf("large Request body issued %d HTTP calls, want 0", calls)
	}
}
