package js_test

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// Mock HTTPFetcher.
// ---------------------------------------------------------------------------

// mockFetcher is a test double for js.HTTPFetcher. It returns pre-configured
// responses keyed by URL, or a fixed error. It records every call so tests
// can assert on the method, headers, and body the fetch passed through.
type mockFetcher struct {
	mu        sync.Mutex
	responses map[string]*js.FetchResponse
	err       error
	calls     []mockCall
}

type mockCall struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    []byte
}

func (m *mockFetcher) Fetch(url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, mockCall{URL: url, Method: method, Headers: headers, Body: body})
	if m.err != nil {
		return nil, m.err
	}
	resp, ok := m.responses[url]
	if !ok {
		return &js.FetchResponse{Status: 404, StatusText: "Not Found"}, nil
	}
	return resp, nil
}

func (m *mockFetcher) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockFetcher) lastCall() mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return mockCall{}
	}
	return m.calls[len(m.calls)-1]
}

type delayedErrorFetcher struct{}

func (delayedErrorFetcher) Fetch(string, string, map[string]string, []byte) (*js.FetchResponse, error) {
	time.Sleep(300 * time.Millisecond)
	return nil, fmt.Errorf("delayed network down")
}

// blockingFetcher blocks on a gate channel inside Fetch, allowing tests to
// control exactly when requests complete. Used by the concurrent-limit test.
type blockingFetcher struct {
	gate   chan struct{} // closed to unblock all Fetch calls
	active int           // atomic-ish, protected by mu
	mu     sync.Mutex
	resp   *js.FetchResponse
}

func (b *blockingFetcher) Fetch(url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	b.mu.Lock()
	b.active++
	b.mu.Unlock()
	<-b.gate
	return b.resp, nil
}

// ---------------------------------------------------------------------------
// Test helpers.
// ---------------------------------------------------------------------------

// newFetchRuntime creates a Runtime with the given HTTPFetcher wired through
// Options.HTTPClient, so that fetch() is available to scripts.
func newFetchRuntime(t *testing.T, fetcher js.HTTPFetcher) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		URL:        "https://example.test/",
		HTTPClient: fetcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func TestResponseConstructorConsumesBlobAndBufferBodies(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	if err := r.Run(`
		var responseCtorWorks = false;
		var responseBlobText = '';
		var responseBufferText = '';
		var responseBodyWasUsed = false;
		var responseFromBlob = new Response(new Blob(['hello'], {type: 'text/x-test'}));
		responseCtorWorks = responseFromBlob instanceof Response && responseFromBlob.status === 200 && responseFromBlob.ok;
		responseFromBlob.blob().then(function(blob) {
			blob.text().then(function(text) { responseBlobText = text; });
		});
		responseBodyWasUsed = responseFromBlob.bodyUsed;
		new Response(new Uint8Array([65, 66])).arrayBuffer().then(function(buffer) {
			responseBufferText = String.fromCharCode.apply(null, new Uint8Array(buffer));
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("responseCtorWorks"); !ok || !value.Bool {
		t.Fatalf("Response constructor state = %+v, want true", value)
	}
	if value, ok := r.Global("responseBlobText"); !ok || value.Str != "hello" {
		t.Fatalf("Response blob text = %+v, want hello", value)
	}
	if value, ok := r.Global("responseBufferText"); !ok || value.Str != "AB" {
		t.Fatalf("Response arrayBuffer text = %+v, want AB", value)
	}
	if value, ok := r.Global("responseBodyWasUsed"); !ok || !value.Bool {
		t.Fatalf("Response bodyUsed = %+v, want true", value)
	}
}

func TestResponseIsNotStructuredCloneable(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	if err := r.Run(`
		var responseCloneError = '';
		var responseCloneErrorIsDOMException = false;
		var responseClonePromiseError = '';
		var responseClonePromiseErrorIsDOMException = false;
	try {
			new MessageChannel().port1.postMessage(new Response('body'));
		} catch (error) {
			responseCloneError = error.name;
			responseCloneErrorIsDOMException = error instanceof DOMException;
		}
		new Promise(function() {
			new MessageChannel().port1.postMessage(new Response('body'));
		}).catch(function(error) {
			responseClonePromiseError = error.name || String(error);
			responseClonePromiseErrorIsDOMException = error instanceof DOMException;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("responseCloneError"); !ok || value.Str != "DataCloneError" {
		t.Fatalf("postMessage(Response) error = %+v, want DataCloneError", value)
	}
	if value, ok := r.Global("responseCloneErrorIsDOMException"); !ok || !value.Bool {
		t.Fatalf("postMessage(Response) thrown value = %+v, want DOMException", value)
	}
	if value, ok := r.Global("responseClonePromiseError"); !ok || value.Str != "DataCloneError" {
		t.Fatalf("promise rejection from postMessage(Response) = %+v, want DataCloneError", value)
	}
	if value, ok := r.Global("responseClonePromiseErrorIsDOMException"); !ok || !value.Bool {
		t.Fatalf("promise rejection type = %+v, want DOMException", value)
	}
}

// drainFetch waits for in-flight fetches to finish, then invokes their queued
// callbacks on the runtime goroutine.
func drainFetch(t *testing.T, r *js.Runtime) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for pending := r.PendingFetches(); pending > 0; pending = r.PendingFetches() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for fetches; pending=%d", pending)
		}
		time.Sleep(time.Millisecond)
	}
	r.DrainFetchCallbacks()
}

// ---------------------------------------------------------------------------
// Standalone tests: verify exported types without needing the full wiring.
// ---------------------------------------------------------------------------

// TestFetchResponseTypeFields verifies that FetchResponse can be constructed
// with all its fields. This is a compile-time contract test: if the struct
// changes incompatibly, this file will not compile.
func TestFetchResponseTypeFields(t *testing.T) {
	resp := &js.FetchResponse{
		Status:     200,
		StatusText: "OK",
		Headers:    map[string]string{"Content-Type": "text/plain"},
		Body:       []byte("hello"),
		URL:        "https://example.test/api",
		OK:         true,
	}
	if resp.Status != 200 {
		t.Errorf("Status = %d, want 200", resp.Status)
	}
	if resp.StatusText != "OK" {
		t.Errorf("StatusText = %q, want %q", resp.StatusText, "OK")
	}
	if resp.Headers["Content-Type"] != "text/plain" {
		t.Errorf("Headers = %v, want Content-Type=text/plain", resp.Headers)
	}
	if string(resp.Body) != "hello" {
		t.Errorf("Body = %q, want %q", resp.Body, "hello")
	}
	if resp.URL != "https://example.test/api" {
		t.Errorf("URL = %q, want %q", resp.URL, "https://example.test/api")
	}
	if !resp.OK {
		t.Error("OK = false, want true")
	}
}

// TestFetchOptionsTypeFields verifies that FetchOptions can be constructed
// with all its fields.
func TestFetchOptionsTypeFields(t *testing.T) {
	opts := js.FetchOptions{
		Method:  "POST",
		Headers: map[string]string{"Accept": "application/json"},
		Body:    `{"key":"value"}`,
	}
	if opts.Method != "POST" {
		t.Errorf("Method = %q, want %q", opts.Method, "POST")
	}
	if opts.Headers["Accept"] != "application/json" {
		t.Errorf("Headers = %v, want Accept=application/json", opts.Headers)
	}
	if opts.Body != `{"key":"value"}` {
		t.Errorf("Body = %q, want %q", opts.Body, `{"key":"value"}`)
	}
}

// TestHTTPFetcherInterface verifies that mockFetcher satisfies the
// js.HTTPFetcher interface. This is a compile-time check: if the interface
// changes, this assignment will not compile.
func TestHTTPFetcherInterface(t *testing.T) {
	var _ js.HTTPFetcher = &mockFetcher{}
}

// TestMockFetcherReturnsConfiguredResponse verifies the mock itself works:
// it returns the response keyed by URL and 404 for unknown URLs.
func TestMockFetcherReturnsConfiguredResponse(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/api": {
				Status:     200,
				StatusText: "OK",
				Body:       []byte("data"),
				OK:         true,
			},
		},
	}

	resp, err := m.Fetch("https://example.test/api", "GET", nil, nil)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("Status = %d, want 200", resp.Status)
	}
	if string(resp.Body) != "data" {
		t.Errorf("Body = %q, want %q", resp.Body, "data")
	}

	// Unknown URL -> 404.
	resp, err = m.Fetch("https://example.test/missing", "GET", nil, nil)
	if err != nil {
		t.Fatalf("Fetch unknown URL: %v", err)
	}
	if resp.Status != 404 {
		t.Errorf("unknown URL Status = %d, want 404", resp.Status)
	}
}

// TestMockFetcherReturnsError verifies that the mock propagates a fixed
// error for every request.
func TestMockFetcherReturnsError(t *testing.T) {
	m := &mockFetcher{err: fmt.Errorf("network down")}
	_, err := m.Fetch("https://example.test/", "GET", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "network down") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "network down")
	}
}

// TestMockFetcherRecordsCalls verifies that the mock records every call for
// later inspection.
func TestMockFetcherRecordsCalls(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{},
	}
	hdrs := map[string]string{"X-Test": "yes"}
	_, _ = m.Fetch("https://example.test/a", "POST", hdrs, []byte("body"))
	_, _ = m.Fetch("https://example.test/b", "GET", nil, nil)

	if m.callCount() != 2 {
		t.Fatalf("callCount = %d, want 2", m.callCount())
	}
	last := m.lastCall()
	if last.URL != "https://example.test/b" {
		t.Errorf("last call URL = %q, want %q", last.URL, "https://example.test/b")
	}
}

func TestWorkerFetchUsesCreatorHTTPClient(t *testing.T) {
	fetcher := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/data": {
			Status: 200, StatusText: "OK", Body: []byte("worker-fetched"),
			URL: "https://example.test/data", OK: true,
		},
	}}
	sources := map[string]string{
		"https://example.test/worker.js": `fetch('/data').then(function(response) { return response.text(); }).then(function(text) { postMessage(text); });`,
	}
	r, err := js.New(js.Options{
		URL:        "https://example.test/page.html",
		HTTPClient: fetcher,
		WorkerScriptFetcher: func(scriptURL string) ([]byte, error) {
			source, ok := sources[scriptURL]
			if !ok {
				return nil, fmt.Errorf("unexpected worker script URL %q", scriptURL)
			}
			return []byte(source), nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var workerFetchResult = '';
		var worker = new Worker('/worker.js');
		worker.onmessage = function(event) { workerFetchResult = event.data; };
		worker.postMessage('start');
	`, "worker-fetch.html"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.Tick()
		if result, ok := r.Global("workerFetchResult"); ok && result.Str == "worker-fetched" {
			if fetcher.callCount() != 1 {
				t.Fatalf("worker fetch calls = %d, want 1", fetcher.callCount())
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	result, _ := r.Global("workerFetchResult")
	t.Fatalf("worker fetch did not resolve; result=%+v calls=%d", result, fetcher.callCount())
}

// TestFetchConstants verifies the exported resource-limit constants have the
// values the architecture requires.
func TestFetchConstants(t *testing.T) {
	if js.MaxConcurrentFetches != 6 {
		t.Errorf("MaxConcurrentFetches = %d, want 6", js.MaxConcurrentFetches)
	}
	if js.MaxFetchBodyBytes != 1<<20 {
		t.Errorf("MaxFetchBodyBytes = %d, want %d (1 MiB)", js.MaxFetchBodyBytes, 1<<20)
	}
	if js.FetchTimeout != 30*time.Second {
		t.Errorf("FetchTimeout = %v, want %v", js.FetchTimeout, 30*time.Second)
	}
}

// TestFetchErrorSentinels verifies the exported error values exist and have
// distinct messages.
func TestFetchErrorSentinels(t *testing.T) {
	if js.ErrFetchNotAvailable == nil {
		t.Fatal("ErrFetchNotAvailable is nil")
	}
	if js.ErrTooManyConcurrentFetches == nil {
		t.Fatal("ErrTooManyConcurrentFetches is nil")
	}
	if js.ErrFetchNotAvailable.Error() == js.ErrTooManyConcurrentFetches.Error() {
		t.Error("error sentinels have the same message")
	}
}

// ---------------------------------------------------------------------------
// Integration tests: verify fetch() end-to-end through the JS runtime.
// ---------------------------------------------------------------------------

// TestFetchReturnsResponse verifies that a basic fetch resolves with the
// correct status, headers, and body on the response object.
func TestFetchReturnsResponse(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/api": {
				Status:     200,
				StatusText: "OK",
				Headers:    map[string]string{"Content-Type": "text/plain"},
				Body:       []byte("hello world"),
				URL:        "https://example.test/api",
				OK:         true,
			},
		},
	}
	r := newFetchRuntime(t, m)

	err := r.Run(`
		var fetchStatus = 0;
		var fetchCT = '';
		var fetchOK = false;
		var fetchURL = '';
		fetch('/api').then(function(resp) {
			fetchStatus = resp.status;
			fetchCT = resp.headers.get('content-type');
			fetchOK = resp.ok;
			fetchURL = resp.url;
		});
	`, "inline")
	if err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("fetchStatus"); !ok || v.Num != 200 {
		t.Errorf("fetchStatus = %+v, want 200", v)
	}
	if v, ok := r.Global("fetchCT"); !ok || v.Str != "text/plain" {
		t.Errorf("fetchCT = %+v, want %q", v, "text/plain")
	}
	if v, ok := r.Global("fetchOK"); !ok || !v.Bool {
		t.Errorf("fetchOK = %+v, want true", v)
	}
	if v, ok := r.Global("fetchURL"); !ok || v.Str != "https://example.test/api" {
		t.Errorf("fetchURL = %+v, want %q", v, "https://example.test/api")
	}
}

// TestFetchThenCallback verifies that the .then() callback receives the
// response object and can read its properties.
func TestFetchThenCallback(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/data": {
				Status: 200, StatusText: "OK",
				Body: []byte("content"), OK: true,
			},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		var gotStatus = 0;
		fetch('/data').then(function(r) {
			gotStatus = r.status;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("gotStatus"); !ok || v.Num != 200 {
		t.Errorf("gotStatus = %+v, want 200", v)
	}
}

// TestFetchCatchOnError verifies that .catch() fires when the HTTP client
// returns an error.
func TestFetchCatchOnError(t *testing.T) {
	m := &mockFetcher{err: fmt.Errorf("network down")}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		var gotCatch = '';
		fetch('/x').catch(function(e) {
			gotCatch = e;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("gotCatch"); !ok || !strings.Contains(v.Str, "network down") {
		t.Errorf("gotCatch = %+v, want it to contain %q", v, "network down")
	}
}

// TestFetchCatchAttachedAfterSettlement verifies a fetch rejection is settled
// before .catch() is attached and delivered when the callback queue drains.
func TestFetchCatchAttachedAfterSettlement(t *testing.T) {
	m := &mockFetcher{err: fmt.Errorf("network down")}
	r := newFetchRuntime(t, m)
	if err := r.Run(`var lateFetch = fetch('/x'); var lateCatch = '';`, "inline"); err != nil {
		t.Fatal(err)
	}
	// PendingFetches is only a barrier proving runFetch has settled and queued
	// its result; the handler is deliberately attached after that point.
	deadline := time.Now().Add(2 * time.Second)
	for r.PendingFetches() > 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for fetch settlement")
		}
		runtime.Gosched()
	}

	if err := r.Run(`lateFetch.catch(function(e) { lateCatch = e; });`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("lateCatch"); !ok || v.Str != "" {
		t.Fatalf("lateCatch before drain = %+v, want empty", v)
	}
	r.DrainFetchCallbacks()
	if v, ok := r.Global("lateCatch"); !ok || !strings.Contains(v.Str, "network down") {
		t.Errorf("lateCatch after drain = %+v, want it to contain %q", v, "network down")
	}
}

// TestFetchDrainWaitsForSlowRequest proves drainFetch waits for the request
// goroutine instead of relying on the former 50 ms timing guess.
func TestFetchDrainWaitsForSlowRequest(t *testing.T) {
	r := newFetchRuntime(t, delayedErrorFetcher{})

	if err := r.Run(`
		var gotCatch = '';
		fetch('/x').catch(function(e) {
			gotCatch = e;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("gotCatch"); !ok || !strings.Contains(v.Str, "delayed network down") {
		t.Errorf("gotCatch = %+v, want it to contain %q", v, "delayed network down")
	}
}

// TestFetchResponseText verifies that response.text() is available and
// returns a thenable that resolves with the body string.
func TestFetchResponseText(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/msg": {
				Status: 200, Body: []byte("hello text"), OK: true,
			},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		var fetchText = '';
		var fetchHasText = false;
		fetch('/msg').then(function(r) {
			fetchHasText = typeof r.text === 'function';
			r.text().then(function(t) {
				fetchText = t;
			});
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("fetchHasText"); !ok || !v.Bool {
		t.Errorf("fetchHasText = %+v, want true", v)
	}
	if v, ok := r.Global("fetchText"); !ok || v.Str != "hello text" {
		t.Errorf("fetchText = %+v, want %q", v, "hello text")
	}
}

// TestFetchResponseJSON verifies that response.json() parses a JSON body and
// resolves with the resulting JS object.
func TestFetchResponseJSON(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/api": {
				Status: 200, Body: []byte(`{"name":"goosie"}`), OK: true,
			},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		var fetchJSONName = '';
		fetch('/api').then(function(r) {
			r.json().then(function(j) {
				fetchJSONName = j.name;
			});
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("fetchJSONName"); !ok || v.Str != "goosie" {
		t.Errorf("fetchJSONName = %+v, want %q", v, "goosie")
	}
}

// TestFetchResponseOK verifies that response.ok is true for 200-299 and false
// otherwise.
func TestFetchResponseOK(t *testing.T) {
	tests := []struct {
		path   string
		status int
		ok     bool
	}{
		{"/ok", 200, true},
		{"/red", 301, false},
		{"/err", 500, false},
		{"/none", 404, false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			fullURL := "https://example.test" + tt.path
			m := &mockFetcher{
				responses: map[string]*js.FetchResponse{
					fullURL: {Status: tt.status, OK: tt.ok},
				},
			}
			r := newFetchRuntime(t, m)

			if err := r.Run(`
				var fetchOK = false;
				fetch('`+tt.path+`').then(function(r) {
					fetchOK = r.ok;
				});
			`, "inline"); err != nil {
				t.Fatal(err)
			}
			drainFetch(t, r)

			if v, ok := r.Global("fetchOK"); !ok || v.Bool != tt.ok {
				t.Errorf("fetchOK = %+v, want %v", v, tt.ok)
			}
		})
	}
}

// TestFetchMethodOption verifies that the method option is forwarded to the
// HTTP client.
func TestFetchMethodOption(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/api": {Status: 200, OK: true},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		fetch('/api', {method: 'POST'}).then(function(r) {});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if got := m.lastCall().Method; got != "POST" {
		t.Errorf("method = %q, want %q", got, "POST")
	}
}

// TestFetchHeadersOption verifies that request headers are forwarded to the
// HTTP client.
func TestFetchHeadersOption(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/api": {Status: 200, OK: true},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		fetch('/api', {headers: {"X-Custom": "val"}}).then(function(r) {});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if got := m.lastCall().Headers["X-Custom"]; got != "val" {
		t.Errorf("X-Custom header = %q, want %q", got, "val")
	}
}

// TestFetchBodyOption verifies that the request body is forwarded to the HTTP
// client with POST.
func TestFetchBodyOption(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/api": {Status: 200, OK: true},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		fetch('/api', {method: 'POST', body: 'data'}).then(function(r) {});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if got := string(m.lastCall().Body); got != "data" {
		t.Errorf("body = %q, want %q", got, "data")
	}
}

func TestFetchFormDataEncodesMultipartFieldsAndFiles(t *testing.T) {
	m := &mockFetcher{responses: map[string]*js.FetchResponse{"https://example.test/upload": {Status: 200, OK: true}}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var body = new FormData();
		body.append('line\n"name', 'first\nsecond');
		body.append('upload', new File(['file-bytes'], 'quoted"name.txt', {type: 'text/plain'}));
		fetch('/upload', {method: 'POST', body: body}).then(function() {});
	`, "fetch-formdata-multipart"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	call := m.lastCall()
	contentType := js.GetHeader(call.Headers, "Content-Type")
	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=----GoosieFormBoundary") {
		t.Fatalf("multipart Content-Type = %q", contentType)
	}
	boundary := strings.TrimPrefix(contentType, "multipart/form-data; boundary=")
	body := string(call.Body)
	for _, want := range []string{
		"--" + boundary + "\r\n",
		`name="line%0D%0A%22name"`,
		"first\r\nsecond",
		`filename="quoted%22name.txt"`,
		"Content-Type: text/plain\r\n\r\nfile-bytes\r\n",
		"--" + boundary + "--\r\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("multipart body missing %q: %q", want, body)
		}
	}
}

// TestFetchConcurrentLimit verifies that at most MaxConcurrentFetches (6)
// requests can be in flight simultaneously. The 7th must reject.
func TestFetchConcurrentLimit(t *testing.T) {
	bf := &blockingFetcher{
		gate: make(chan struct{}),
		resp: &js.FetchResponse{Status: 200, OK: true},
	}
	r := newFetchRuntime(t, bf)

	// Launch 6 blocking fetches plus a 7th that should be rejected.
	if err := r.Run(`
		var fetchConcReject = '';
		for (var i = 0; i < 6; i++) {
			fetch('/a' + i).then(function() {});
		}
		fetch('/blocked').catch(function(e) {
			fetchConcReject = e;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	// Give the 6 goroutines time to enter Fetch and block on the gate.
	time.Sleep(50 * time.Millisecond)

	// Unblock the 6 fetches.
	close(bf.gate)
	drainFetch(t, r)

	if v, ok := r.Global("fetchConcReject"); !ok || !strings.Contains(v.Str, "too many concurrent fetches") {
		t.Errorf("fetchConcReject = %+v, want %q", v, "too many concurrent fetches")
	}
}

// TestFetchWithNilClientRejects verifies that fetch without an HTTP client
// is not defined and calling it produces an error.
func TestFetchWithNilClientRejects(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })

	// Without HTTPClient, fetch is not defined at all, so calling it
	// produces a ReferenceError.
	err = r.Run(`fetch('/x').catch(function(e) {});`, "inline")
	if err == nil {
		t.Fatal("expected error when fetch is not defined")
	}
	if !strings.Contains(err.Error(), "fetch") {
		t.Errorf("error = %q, want it to mention fetch", err.Error())
	}
}

// TestFetchURLResolution verifies that relative URLs are resolved against the
// document URL before being passed to the HTTP client.
func TestFetchURLResolution(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/api/data": {Status: 200, OK: true},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		fetch('/api/data').then(function(r) {});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if got := m.lastCall().URL; got != "https://example.test/api/data" {
		t.Errorf("URL = %q, want %q", got, "https://example.test/api/data")
	}
}

// TestFetchTickDrainsCallbacks verifies that fetch callbacks are processed
// when the event loop drains them via Tick.
func TestFetchTickDrainsCallbacks(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/": {Status: 200, Body: []byte("ok"), OK: true},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		var fetchTick = 0;
		fetch('/').then(function(r) {
			fetchTick = r.status;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	r.Tick()

	if v, ok := r.Global("fetchTick"); !ok || v.Num != 200 {
		t.Errorf("fetchTick = %+v, want 200", v)
	}
}

// TestFetchMultipleThenChain verifies that multiple independent fetches
// all resolve correctly.
func TestFetchMultipleThenChain(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/a": {Status: 200, OK: true},
			"https://example.test/b": {Status: 201, OK: true},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		var fetchA = 0;
		var fetchB = 0;
		fetch('/a').then(function(r) {
			fetchA = r.status;
		});
		fetch('/b').then(function(r) {
			fetchB = r.status;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("fetchA"); !ok || v.Num != 200 {
		t.Errorf("fetchA = %+v, want 200", v)
	}
	if v, ok := r.Global("fetchB"); !ok || v.Num != 201 {
		t.Errorf("fetchB = %+v, want 201", v)
	}
}
