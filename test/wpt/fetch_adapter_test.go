package wpt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestWPTDocumentURLUsesSecureOriginForHTTPSFiles(t *testing.T) {
	const transport = "http://wpt.localhost:12345/webmessaging/cross-partition.https.tentative.html?run=1"
	if got, want := wptDocumentURL(transport, "webmessaging/cross-partition.https.tentative.html"), "https://wpt.localhost:12345/webmessaging/cross-partition.https.tentative.html?run=1"; got != want {
		t.Fatalf("secure WPT document URL = %q, want %q", got, want)
	}
	if got := wptDocumentURL(transport, "webmessaging/basics.html"); got != transport {
		t.Fatalf("non-secure WPT document URL = %q, want transport URL %q", got, transport)
	}
}

func TestWPTFetchAdapterDrivesFetchThenTextChain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/interfaces/html.idl" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("interface GlobalEventHandlers { attribute EventHandler onclick; };"))
	}))
	defer server.Close()

	runtime, err := js.New(js.Options{
		URL:        server.URL + "/case.html",
		HTTPClient: wptFetchAdapter{ctx: context.Background(), client: server.Client(), baseURL: server.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`
		window.fetchTextResult = 'pending';
		fetch('/interfaces/html.idl').then(function(response) {
			return response.text();
		}).then(function(text) {
			window.fetchTextResult = text;
		});
	`, "fetch-adapter-test"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.Tick()
		if result, ok := runtime.Global("fetchTextResult"); ok && result.Str != "pending" {
			if got := result.Str; got != "interface GlobalEventHandlers { attribute EventHandler onclick; };" {
				t.Fatalf("fetch text = %q", got)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("fetch/text thenable chain did not resolve; pending requests=%d", runtime.PendingFetches())
}

func TestWPTFetchAdapterRejectsOutsideOrigin(t *testing.T) {
	served := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served <- struct{}{}
	}))
	defer server.Close()
	base := httptest.NewServer(http.NotFoundHandler())
	defer base.Close()
	adapter := wptFetchAdapter{ctx: context.Background(), client: base.Client(), baseURL: base.URL}
	if _, err := adapter.Fetch(server.URL+"/resource", http.MethodGet, nil, nil); err == nil {
		t.Fatal("outside-origin fetch was allowed")
	}
	select {
	case <-served:
		t.Fatal("outside-origin request reached the test server")
	default:
	}
}

func TestWPTFetchAdapterMapsConfiguredOriginAlias(t *testing.T) {
	seenHost := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHost <- r.Host
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	local, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port := local.Port()
	base := "http://wpt.localhost:" + port
	target := "http://cross-origin.wpt-other.localhost:" + port + "/resource"
	adapter := wptFetchAdapter{ctx: context.Background(), client: server.Client(), baseURL: base}
	response, err := adapter.Fetch(target, http.MethodGet, nil, nil)
	if err != nil {
		t.Fatalf("Fetch configured WPT alias: %v", err)
	}
	if string(response.Body) != "ok" {
		t.Fatalf("response body = %q, want ok", response.Body)
	}
	select {
	case host := <-seenHost:
		want := fmt.Sprintf("cross-origin.wpt-other.localhost:%s", port)
		if host != want {
			t.Fatalf("server Host = %q, want %q", host, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("configured alias request did not reach the local WPT server")
	}
}
