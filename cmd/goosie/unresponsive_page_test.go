package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	gonet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/raster"
)

// FR-JS-06: a page whose script exceeds the CPU budget must show a prompt that
// names the two honest outcomes, not silently display (or silently discard) the
// interrupted document.

func newUnresponsiveTestPath(t *testing.T) *framePath {
	t.Helper()
	f := newNavTestPath(t)
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	f.fonts = fonts
	f.config.width = 800
	f.config.height = 600
	f.scriptTimeout = 50 * time.Millisecond
	return f
}

// qc:BR-167
func TestUnresponsivePolicyRoutesPromptActions(t *testing.T) {
	stopURL := "goosie://unresponsive?action=stop&url=" + url.QueryEscape("https://example.test/slow")
	target, policy, ok := unresponsiveScriptPolicy(stopURL)
	if !ok || target != "https://example.test/slow" || !policy.disabled || policy.timeout != 0 {
		t.Fatalf("stop action = (%q, %+v, %v), want target with scripts disabled", target, policy, ok)
	}

	waitURL := "goosie://unresponsive?action=wait&url=" + url.QueryEscape("https://example.test/slow")
	target, policy, ok = unresponsiveScriptPolicy(waitURL)
	if !ok || target != "https://example.test/slow" || policy.disabled || policy.timeout != unresponsiveRetryBudget {
		t.Fatalf("wait action = (%q, %+v, %v), want target with the retry budget", target, policy, ok)
	}

	for _, raw := range []string{
		"goosie://unresponsive?url=https%3A%2F%2Fexample.test%2Fslow",
		"goosie://unresponsive?action=stop&url=javascript%3Aalert(1)",
		"https://example.test/unresponsive?action=stop&url=https%3A%2F%2Fexample.test%2F",
	} {
		if _, _, ok := unresponsiveScriptPolicy(raw); ok {
			t.Errorf("%s was treated as a prompt action", raw)
		}
	}
}

// qc:BR-167
func TestUnresponsivePromptNamesBothActions(t *testing.T) {
	_, body := buildUnresponsivePageHTML("goosie://unresponsive?url=" + url.QueryEscape("https://example.test/slow") + "&count=1")
	for _, want := range []string{
		"Unresponsive Script",
		"https://example.test/slow",
		"Reload without scripts",
		"action=stop",
		"action=wait",
		"30s script limit",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

// qc:BR-167
func TestSlowLoadShowsPromptInsteadOfThePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><p>SLOW PAGE BODY</p><script>while (true) {}</script></body></html>`))
	}))
	defer srv.Close()

	f := newUnresponsiveTestPath(t)
	client := gonet.DefaultClient()
	defer client.Close()
	layer, _, bg, sess, finalURL, err := f.loadDocumentCtx(context.Background(), client, srv.URL, 800, 600, 1, t.TempDir(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil || !sess.HasScriptTimeout() {
		t.Fatal("slow page load did not report a script timeout")
	}
	tab := f.tabMgr.Active()
	f.applyNavResult(navResult{tabID: tab.ID(), serial: tab.Nav.Serial, url: finalURL, layer: layer, bgColor: bg, session: sess})

	if got := tab.Title(); got != "Unresponsive Script" {
		t.Fatalf("tab title = %q, want Unresponsive Script", got)
	}
	prompt := tab.Session()
	if prompt == nil {
		t.Fatal("tab has no prompt session")
	}
	text := prompt.Doc.Node.TextContent()
	if !strings.Contains(text, "Unresponsive Script") {
		t.Fatalf("tab shows %q, want the unresponsive prompt", text)
	}
	if strings.Contains(text, "SLOW PAGE BODY") {
		t.Fatal("the interrupted page was committed instead of the prompt")
	}
}

// qc:BR-167
func TestStopActionReloadsWithoutScripts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><p>STOPPED PAGE BODY</p><script>while (true) {}</script></body></html>`))
	}))
	defer srv.Close()

	f := newUnresponsiveTestPath(t)
	client := gonet.DefaultClient()
	defer client.Close()
	promptURL := "goosie://unresponsive?action=stop&url=" + url.QueryEscape(srv.URL)
	layer, _, _, sess, finalURL, err := f.loadDocumentCtx(context.Background(), client, promptURL, 800, 600, 1, t.TempDir(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if layer == nil {
		t.Fatal("stop action produced no layer")
	}
	if finalURL != srv.URL {
		t.Fatalf("final URL = %q, want %q", finalURL, srv.URL)
	}
	if sess.HasScriptTimeout() {
		t.Fatal("stop action still ran scripts")
	}
	if text := sess.Doc.Node.TextContent(); !strings.Contains(text, "STOPPED PAGE BODY") {
		t.Fatalf("page body = %q, want the page rendered without scripts", text)
	}
}

// qc:BR-167
func TestWaitActionReloadsWithRetryBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><p>WAIT PAGE BODY</p></body></html>`))
	}))
	defer srv.Close()

	f := newUnresponsiveTestPath(t)
	client := gonet.DefaultClient()
	defer client.Close()
	promptURL := "goosie://unresponsive?action=wait&url=" + url.QueryEscape(srv.URL)
	_, _, _, sess, finalURL, err := f.loadDocumentCtx(context.Background(), client, promptURL, 800, 600, 1, t.TempDir(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if finalURL != srv.URL {
		t.Fatalf("final URL = %q, want %q", finalURL, srv.URL)
	}
	if text := sess.Doc.Node.TextContent(); !strings.Contains(text, "WAIT PAGE BODY") {
		t.Fatalf("page body = %q, want the retried page", text)
	}
}
