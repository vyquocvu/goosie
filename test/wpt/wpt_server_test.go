package wpt

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerMapsLegacyWebIDLParserResource(t *testing.T) {
	root := t.TempDir()
	resourcePath := filepath.Join(root, "resources", "webidl2", "lib")
	if err := os.MkdirAll(resourcePath, 0o755); err != nil {
		t.Fatal(err)
	}
	const bundle = "globalThis.WebIDL2 = { parse: function() {} };"
	if err := os.WriteFile(filepath.Join(resourcePath, "webidl2.js"), []byte(bundle), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(root)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	response, err := http.Get(server.URL() + "/resources/WebIDLParser.js")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), bundle) {
		t.Fatalf("legacy WebIDLParser resource: status=%d body contains bundle=%t", response.StatusCode, strings.Contains(string(body), bundle))
	}
}

func TestServerExpandsGetParametersInSubTests(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "forms"), 0o755); err != nil {
		t.Fatal(err)
	}
	const source = `<form action="{{GET[action]}}">{{GET[content]}}</form>`
	if err := os.WriteFile(filepath.Join(root, "forms", "submit.sub.html"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(root)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	response, err := http.Get(server.URL() + "/forms/submit.sub.html?action=target%2Fpage.html%3Fx%3D1%26y%3D2&content=%3Cscript%3Ebad%3C%2Fscript%3E")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("response status = %d, want 200", response.StatusCode)
	}
	const want = `<form action="target/page.html?x=1&amp;y=2">&lt;script&gt;bad&lt;/script&gt;</form>`
	if got := string(body); got != want {
		t.Fatalf("expanded .sub.html = %q, want %q", got, want)
	}
}

func TestServerExpandsHostAndPortInSubTests(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "domparsing"), 0o755); err != nil {
		t.Fatal(err)
	}
	const source = `http://{{host}}:{{ports[http][0]}}/domparsing/path`
	if err := os.WriteFile(filepath.Join(root, "domparsing", "host.sub.html"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(root)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	response, err := http.Get(server.URL() + "/domparsing/host.sub.html")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := "http://wpt.localhost:" + fmt.Sprint(server.Port()) + "/domparsing/path"
	if got := string(body); got != want {
		t.Fatalf("expanded .sub.html origin = %q, want %q", got, want)
	}
}

func TestServerKeepsWPTRemoteOriginCrossOriginButReachable(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "common")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const source = "var ORIGINAL_HOST = '{{host}}';\nvar REMOTE_HOST = (ORIGINAL_HOST === 'localhost') ? '127.0.0.1' : ('www1.' + ORIGINAL_HOST);\nvar REMOTE_ORIGIN = 'http://' + REMOTE_HOST + ':{{ports[http][0]}}';\nvar HTTPS_NOTSAMESITE_ORIGIN = 'https://{{hosts[alt][]}}:{{ports[https][0]}}';\nvar ALT_HOST = '{{hosts[alt][]}}';"
	if err := os.WriteFile(filepath.Join(dir, "get-host-info.sub.js"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(root)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	response, err := http.Get(server.URL() + "/common/get-host-info.sub.js")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("var ORIGINAL_HOST = 'wpt.localhost';\nvar REMOTE_HOST = (ORIGINAL_HOST === 'localhost') ? '127.0.0.1' : ('www1.' + ORIGINAL_HOST);\nvar REMOTE_ORIGIN = 'http://' + REMOTE_HOST + ':%d';\nvar HTTPS_NOTSAMESITE_ORIGIN = 'https://wpt-other.localhost:%d';\nvar ALT_HOST = 'wpt-other.localhost';", server.Port(), server.Port())
	if got := string(body); got != want {
		t.Fatalf("expanded remote origin = %q, want %q", got, want)
	}
}

func TestServerDispatcherQueuesCrossContextMessages(t *testing.T) {
	server := NewServer(t.TempDir())
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	const endpoint = "/common/dispatcher/dispatcher.py?uuid=dispatcher-regression"
	client := &http.Client{}
	request := func(method, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, server.URL()+endpoint, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", "https://frame.wpt.localhost")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	readBody := func(response *http.Response) string {
		t.Helper()
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	initial := request(http.MethodGet, "")
	if got := readBody(initial); initial.StatusCode != http.StatusOK || got != "not ready" {
		t.Fatalf("empty dispatcher queue: status=%d body=%q", initial.StatusCode, got)
	}
	posted := request(http.MethodPost, "first message")
	if got := readBody(posted); posted.StatusCode != http.StatusOK || got != "done" {
		t.Fatalf("dispatcher POST: status=%d body=%q", posted.StatusCode, got)
	}
	if got := posted.Header.Get("Access-Control-Allow-Origin"); got != "https://frame.wpt.localhost" {
		t.Fatalf("dispatcher CORS origin = %q", got)
	}
	if got := posted.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("dispatcher CORS credentials = %q", got)
	}
	message := request(http.MethodGet, "")
	if got := readBody(message); message.StatusCode != http.StatusOK || got != "first message" {
		t.Fatalf("dispatcher GET: status=%d body=%q", message.StatusCode, got)
	}
	if got := readBody(request(http.MethodGet, "")); got != "not ready" {
		t.Fatalf("dispatcher queue was not consumed")
	}
}

func TestServerEchoesWPTRequestBodies(t *testing.T) {
	server := NewServer(t.TempDir())
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	requestBody := []byte{'A', '\r', '\n', 'B', '\n', 0, '\\'}
	for _, test := range []struct {
		path string
		want []byte
	}{
		{path: "/fetch/api/resources/echo-content.py", want: requestBody},
		{path: "/FileAPI/file/resources/echo-content-escaped.py", want: []byte("A\r\nB\\x0a\\x00\\\\")},
	} {
		request, err := http.NewRequest(http.MethodPost, server.URL()+test.path, bytes.NewReader(requestBody))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "multipart/form-data; boundary=test-boundary")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusOK || !bytes.Equal(body, test.want) {
			t.Errorf("POST %s: status=%d body=%q, want %q", test.path, response.StatusCode, body, test.want)
		}
		if response.Header.Get("X-Request-Method") != http.MethodPost || response.Header.Get("X-Request-Content-Type") != "multipart/form-data; boundary=test-boundary" {
			t.Errorf("POST %s echo headers: %+v", test.path, response.Header)
		}
	}
}

func TestServerFormEchoReturnsHexadecimalBodyBytes(t *testing.T) {
	server := NewServer(t.TempDir())
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	requestBody := []byte{'A', '\r', '\n', 'B', '\n', 0, '\\'}
	request, err := http.NewRequest(http.MethodPost, server.URL()+"/html/semantics/forms/form-submission-0/form-echo.py", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "text/plain")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK || string(body) != "41 0d 0a 42 0a 00 5c" {
		t.Fatalf("form echo: status=%d body=%q", response.StatusCode, body)
	}
	if response.Header.Get("Content-Type") != "text/plain" || response.Header.Get("X-Request-Content-Type") != "text/plain" {
		t.Fatalf("form echo headers: %+v", response.Header)
	}
}
