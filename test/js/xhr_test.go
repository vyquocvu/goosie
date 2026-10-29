package js_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// XMLHttpRequest is the transport dynamic pages still use for beacons and
// data: Google's impression logger constructs one per ping. A missing XHR
// takes those subsystems down with "XMLHttpRequest is not defined".
func TestXHRConstructorExists(t *testing.T) {
	r := newFetchRuntime(t, &mockFetcher{responses: map[string]*js.FetchResponse{}})
	src := `
var xhr = new XMLHttpRequest();
var out = (typeof XMLHttpRequest) + "/" + xhr.readyState + "/" + xhr.UNSENT + "/" + XMLHttpRequest.DONE + "/" + xhr.status + "/" + (typeof xhr.open) + "/" + (typeof xhr.send);`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "function/0/0/4/0/function/function" {
		t.Fatalf("xhr shape = %+v", v)
	}
}

func TestXHRGetFlow(t *testing.T) {
	fetcher := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/data": {Status: 200, StatusText: "OK", Body: []byte("hello")},
	}}
	r := newFetchRuntime(t, fetcher)
	src := `
var log = [];
var xhr = new XMLHttpRequest();
xhr.onreadystatechange = function() { log.push(xhr.readyState); };
xhr.onload = function() { log.push("load"); };
xhr.onloadend = function() { log.push("loadend"); };
xhr.open("GET", "/data");
xhr.send();
var out = "sent:" + xhr.readyState;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "sent:1" {
		t.Fatalf("after send readyState = %+v, want sent:1", v)
	}
	time.Sleep(100 * time.Millisecond)
	r.Tick()
	if err := r.Run(`var result = xhr.status + "/" + xhr.responseText + "/" + log.join(",");`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("result"); !ok || v.Str != "200/hello/1,2,3,4,load,loadend" {
		t.Fatalf("xhr flow = %+v, want 200/hello/1,2,3,4,load,loadend", v)
	}
}

func TestXHRResponseXMLHTMLDocument(t *testing.T) {
	fetcher := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/fragment": {
			Status: 200, StatusText: "OK",
			Headers: map[string]string{"Content-Type": "text/html; charset=utf-8"},
			Body:    []byte(`<!doctype html><html><head><title>XHR</title></head><body><p id="inside">ok</p></body></html>`),
		},
	}}
	r, err := js.New(js.Options{
		URL: "https://example.test/", DOM: domtest.Parse(`<!doctype html><html><head></head><body></body></html>`), HTTPClient: fetcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	if err := r.Run(`
		class XHRElement extends HTMLElement { connectedCallback() { window.xhrConnected = true; } }
		customElements.define('x-xhr-element', XHRElement);
		var xhr = new XMLHttpRequest();
		xhr.open('GET', '/fragment');
		xhr.send();
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	r.Tick()
	if err := r.Run(`
		var xhrDocument = xhr.responseXML;
		var result = xhrDocument.contentType + '/' + xhrDocument.querySelector('#inside').textContent + '/' +
			(xhr.responseXML === xhrDocument) + '/' + (typeof xhrDocument.adoptNode);
		var unresolved = xhrDocument.createElement('x-xhr-element');
		xhrDocument.adoptNode(unresolved);
		document.body.appendChild(unresolved);
		result += '/' + (unresolved instanceof XHRElement) + '/' + window.xhrConnected;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("result"); !ok || v.Str != "text/html/ok/true/function/true/true" {
		t.Fatalf("XHR responseXML = %+v", v)
	}
}

func TestXHRPostEcho(t *testing.T) {
	fetcher := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/submit": {Status: 200, StatusText: "OK", Body: []byte("ok")},
	}}
	r := newFetchRuntime(t, fetcher)
	src := `
var xhr = new XMLHttpRequest();
xhr.open("POST", "https://example.test/submit");
xhr.setRequestHeader("Content-Type", "text/plain");
xhr.send("payload");`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	r.Tick()
	call := fetcher.lastCall()
	if call.Method != "POST" || string(call.Body) != "payload" {
		t.Fatalf("xhr passthrough = %+v", call)
	}
	if call.Headers["Content-Type"] != "text/plain" {
		t.Fatalf("xhr headers = %+v", call.Headers)
	}
}

func TestXHRNetworkError(t *testing.T) {
	fetcher := &mockFetcher{err: errors.New("boom")}
	r := newFetchRuntime(t, fetcher)
	src := `
var fired = [];
var xhr = new XMLHttpRequest();
xhr.addEventListener("error", function() { fired.push("error"); });
xhr.addEventListener("loadend", function() { fired.push("loadend"); });
xhr.onload = function() { fired.push("load-nope"); };
xhr.open("GET", "https://example.test/x");
xhr.send();`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	r.Tick()
	if err := r.Run(`var result = fired.join(",") + "/" + xhr.status + "/" + xhr.readyState;`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("result"); !ok || v.Str != "error,loadend/0/4" {
		t.Fatalf("xhr error flow = %+v, want error,loadend/0/4", v)
	}
}

func TestXHRNoClientFailsClean(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	src := `
var fired = [];
var xhr = new XMLHttpRequest();
xhr.addEventListener("error", function() { fired.push("error"); });
xhr.addEventListener("loadend", function() { fired.push("loadend"); });
xhr.open("GET", "https://example.test/x");
xhr.send();
var out = "sent:" + xhr.readyState;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	r.Tick()
	if err := r.Run(`var result = out + "/" + fired.join(",");`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("result"); !ok || v.Str != "sent:1/error,loadend" {
		t.Fatalf("no-client xhr = %+v, want sent:1/error,loadend", v)
	}
}

func TestXHRResponseHeaders(t *testing.T) {
	fetcher := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/h": {Status: 200, StatusText: "OK", Headers: map[string]string{"Content-Type": "text/html", "X-Custom": "v"}, Body: []byte("")},
	}}
	r := newFetchRuntime(t, fetcher)
	src := `
var xhr = new XMLHttpRequest();
xhr.open("GET", "https://example.test/h");
xhr.send();`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	r.Tick()
	if err := r.Run(`var result = xhr.getResponseHeader("content-type") + "/" + xhr.getResponseHeader("X-CUSTOM") + "/" + xhr.getResponseHeader("nope");`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("result"); !ok || v.Str != "text/html/v/null" {
		t.Fatalf("xhr getResponseHeader = %+v", v)
	}
	if err := r.Run(`var all = xhr.getAllResponseHeaders();`, "inline"); err != nil {
		t.Fatal(err)
	}
	v, _ := r.Global("all")
	if !strings.Contains(v.Str, "content-type: text/html") {
		t.Fatalf("xhr getAllResponseHeaders = %q", v.Str)
	}
}

func TestXHRAbort(t *testing.T) {
	gate := make(chan struct{})
	fetcher := &blockingFetcher{gate: gate, resp: &js.FetchResponse{Status: 200, Body: []byte("late")}}
	r := newFetchRuntime(t, fetcher)
	src := `
var fired = [];
var xhr = new XMLHttpRequest();
xhr.addEventListener("abort", function() { fired.push("abort"); });
xhr.addEventListener("loadend", function() { fired.push("loadend"); });
xhr.addEventListener("load", function() { fired.push("load-nope"); });
xhr.open("GET", "https://example.test/slow");
xhr.send();
xhr.abort();`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	close(gate)
	time.Sleep(100 * time.Millisecond)
	r.Tick()
	if err := r.Run(`var result = fired.join(",") + "/" + xhr.status;`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("result"); !ok || v.Str != "abort,loadend/0" {
		t.Fatalf("xhr abort = %+v, want abort,loadend/0", v)
	}
}

func TestXHRTimeout(t *testing.T) {
	gate := make(chan struct{})
	defer close(gate)
	fetcher := &blockingFetcher{gate: gate, resp: &js.FetchResponse{Status: 200, Body: []byte("late")}}
	r := newFetchRuntime(t, fetcher)
	src := `
var fired = [];
var xhr = new XMLHttpRequest();
xhr.addEventListener("timeout", function() { fired.push("timeout"); });
xhr.addEventListener("loadend", function() { fired.push("loadend"); });
xhr.addEventListener("load", function() { fired.push("load-nope"); });
xhr.open("GET", "https://example.test/slow");
xhr.timeout = 30;
xhr.send();`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	r.Tick()
	// Let the late response land: it must not fire load after a timeout.
	r.Tick()
	if err := r.Run(`var result = fired.join(",") + "/" + xhr.status;`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("result"); !ok || v.Str != "timeout,loadend/0" {
		t.Fatalf("xhr timeout = %+v, want timeout,loadend/0", v)
	}
}

func TestXHRJSONResponse(t *testing.T) {
	fetcher := &mockFetcher{responses: map[string]*js.FetchResponse{
		"https://example.test/j": {Status: 200, StatusText: "OK", Body: []byte(`{"a":1}`)},
	}}
	r := newFetchRuntime(t, fetcher)
	src := `
var xhr = new XMLHttpRequest();
xhr.open("GET", "https://example.test/j");
xhr.responseType = "json";
xhr.send();`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	r.Tick()
	if err := r.Run(`var result = xhr.response.a;`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("result"); !ok || v.Num != 1 {
		t.Fatalf("xhr json response = %+v, want 1", v)
	}
}

func TestXHRSendWithoutOpenThrows(t *testing.T) {
	r := newFetchRuntime(t, &mockFetcher{})
	if err := r.Run(`var out = "none"; try { new XMLHttpRequest().send(); } catch (e) { out = e.name; }`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "InvalidStateError" {
		t.Fatalf("send-without-open = %+v, want InvalidStateError", v)
	}
}
