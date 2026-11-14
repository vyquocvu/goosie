package js_test

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

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

func (m *mockFetcher) Fetch(ctx context.Context, url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
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

func (delayedErrorFetcher) Fetch(ctx context.Context, url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	time.Sleep(300 * time.Millisecond)
	return nil, fmt.Errorf("delayed network down")
}

type blockingFetcher struct {
	gate   chan struct{}
	active int
	mu     sync.Mutex
	resp   *js.FetchResponse
}

func (b *blockingFetcher) Fetch(ctx context.Context, url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	b.mu.Lock()
	b.active++
	b.mu.Unlock()
	<-b.gate
	return b.resp, nil
}

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

func TestHTTPFetcherInterface(t *testing.T) {
	var _ js.HTTPFetcher = &mockFetcher{}
}

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

	resp, err := m.Fetch(context.Background(), "https://example.test/api", "GET", nil, nil)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("Status = %d, want 200", resp.Status)
	}
	if string(resp.Body) != "data" {
		t.Errorf("Body = %q, want %q", resp.Body, "data")
	}

	resp, err = m.Fetch(context.Background(), "https://example.test/missing", "GET", nil, nil)
	if err != nil {
		t.Fatalf("Fetch unknown URL: %v", err)
	}
	if resp.Status != 404 {
		t.Errorf("unknown URL Status = %d, want 404", resp.Status)
	}
}

func TestMockFetcherReturnsError(t *testing.T) {
	m := &mockFetcher{err: fmt.Errorf("network down")}
	_, err := m.Fetch(context.Background(), "https://example.test/", "GET", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "network down") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "network down")
	}
}

func TestMockFetcherRecordsCalls(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{},
	}
	hdrs := map[string]string{"X-Test": "yes"}
	_, _ = m.Fetch(context.Background(), "https://example.test/a", "POST", hdrs, []byte("body"))
	_, _ = m.Fetch(context.Background(), "https://example.test/b", "GET", nil, nil)

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

// qc:BR-119
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

func TestFetchCatchAttachedAfterSettlement(t *testing.T) {
	m := &mockFetcher{err: fmt.Errorf("network down")}
	r := newFetchRuntime(t, m)
	if err := r.Run(`var lateFetch = fetch('/x'); var lateCatch = '';`, "inline"); err != nil {
		t.Fatal(err)
	}
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

// qc:BR-119
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

func TestFetchConcurrentLimit(t *testing.T) {
	bf := &blockingFetcher{
		gate: make(chan struct{}),
		resp: &js.FetchResponse{Status: 200, OK: true},
	}
	r := newFetchRuntime(t, bf)

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

	time.Sleep(50 * time.Millisecond)

	close(bf.gate)
	drainFetch(t, r)

	if v, ok := r.Global("fetchConcReject"); !ok || !strings.Contains(v.Str, "too many concurrent fetches") {
		t.Errorf("fetchConcReject = %+v, want %q", v, "too many concurrent fetches")
	}
}

func TestFetchWithNilClientRejects(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })

	err = r.Run(`fetch('/x').catch(function(e) {});`, "inline")
	if err == nil {
		t.Fatal("expected error when fetch is not defined")
	}
	if !strings.Contains(err.Error(), "fetch") {
		t.Errorf("error = %q, want it to mention fetch", err.Error())
	}
}

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

// qc:BR-119
func TestFetchSameOriginSettlesOnce(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/once": {
				Status: 200, StatusText: "OK", Body: []byte("settled"),
				URL: "https://example.test/once", OK: true,
			},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		var resolveCount = 0;
		var rejectCount = 0;
		var onceBody = '';
		fetch('/once').then(function(resp) {
			resolveCount++;
			resp.text().then(function(body) { onceBody = body; });
		}, function() {
			rejectCount++;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)
	r.DrainFetchCallbacks()

	if v, ok := r.Global("resolveCount"); !ok || v.Num != 1 {
		t.Errorf("resolveCount = %+v, want 1", v)
	}
	if v, ok := r.Global("rejectCount"); !ok || v.Num != 0 {
		t.Errorf("rejectCount = %+v, want 0", v)
	}
	if v, ok := r.Global("onceBody"); !ok || v.Str != "settled" {
		t.Errorf("onceBody = %+v, want %q", v, "settled")
	}
}

func TestFetchDefaultsToGETWithoutInit(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/default-method": {Status: 200, OK: true},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`fetch('/default-method').then(function() {});`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if got := m.lastCall().Method; got != "GET" {
		t.Errorf("method = %q, want GET", got)
	}
}

type methodFetcher struct {
	responses map[string]map[string]*js.FetchResponse
	mu        sync.Mutex
	calls     []string
}

func (f *methodFetcher) Fetch(ctx context.Context, url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	f.mu.Lock()
	f.calls = append(f.calls, method+" "+url+" "+fmt.Sprint(headers))
	f.mu.Unlock()
	if byMethod, ok := f.responses[url]; ok {
		if resp, ok := byMethod[method]; ok {
			return resp, nil
		}
	}
	return &js.FetchResponse{Status: 404, StatusText: "Not Found"}, nil
}

func corsPreflight(origin string) *js.FetchResponse {
	return &js.FetchResponse{Status: 204, Headers: map[string]string{
		"Access-Control-Allow-Origin":  origin,
		"Access-Control-Allow-Methods": "GET, OPTIONS",
		"Access-Control-Allow-Headers": "Origin",
	}}
}

// qc:BR-120
func TestFetchCrossOriginBlockedResponseDoesNotReachScript(t *testing.T) {
	m := &methodFetcher{responses: map[string]map[string]*js.FetchResponse{
		"https://other.test/secret": {
			"OPTIONS": corsPreflight("https://example.test"),
			"GET": {
				Status: 200, StatusText: "OK", OK: true,
				Body: []byte("TOP-SECRET"),
			},
		},
	}}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		var blockedResolved = false;
		var blockedRejected = '';
		var leaked = '';
		fetch('https://other.test/secret').then(function(resp) {
			blockedResolved = true;
			resp.text().then(function(body) { leaked = body; });
		}, function(err) {
			blockedRejected = String(err);
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("blockedResolved"); !ok || v.Bool {
		t.Errorf("blockedResolved = %+v, want false", v)
	}
	if v, ok := r.Global("leaked"); !ok || v.Str != "" {
		t.Errorf("leaked = %+v, want empty (blocked body must not reach script)", v)
	}
	if v, ok := r.Global("blockedRejected"); !ok || !strings.Contains(v.Str, "CORS") {
		t.Errorf("blockedRejected = %+v, want a CORS error", v)
	}
}

// qc:BR-120
func TestFetchCrossOriginAllowedResponseReachesScript(t *testing.T) {
	m := &methodFetcher{responses: map[string]map[string]*js.FetchResponse{
		"https://other.test/open": {
			"OPTIONS": corsPreflight("https://example.test"),
			"GET": {
				Status: 200, StatusText: "OK", OK: true,
				Headers: map[string]string{"Access-Control-Allow-Origin": "https://example.test"},
				Body:    []byte("OPEN"),
			},
		},
	}}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		var allowedBody = '';
		var allowedResolved = false;
		var allowedStatus = 0;
		var allowedErr = '';
		fetch('https://other.test/open').then(function(resp) {
			allowedResolved = true;
			allowedStatus = resp.status;
			resp.text().then(function(body) { allowedBody = body; });
		}, function(err) {
			allowedErr = String(err);
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("allowedBody"); !ok || v.Str != "OPEN" {
		status, _ := r.Global("allowedStatus")
		resolved, _ := r.Global("allowedResolved")
		errText, _ := r.Global("allowedErr")
		t.Errorf("allowedBody = %+v, want %q (resolved=%v status=%v err=%q calls=%v)", v, "OPEN", resolved.Bool, status.Num, errText.Str, m.calls)
	}
}
