package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestJourneyAnchorTargetBlankOpensHostTab(t *testing.T) {
	var destHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/destination" {
			destHits.Add(1)
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="landed">Landed</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body><h1 id="source">Source</h1><a id="open" href="/destination" target="_blank">open</a></body></html>`)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "source document is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "open")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()

	b.clickElementByID(t, "open")
	b.waitFor(t, "target=_blank opens a second host tab", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/destination")
	})
	b.waitFor(t, "the new tab paints the destination document", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "landed")
		return ok
	})
	b.waitFor(t, "the destination was fetched exactly once", func() bool {
		return destHits.Load() == 1
	})

	b.f.switchTab(sourceID)
	b.waitFor(t, "the source tab keeps its document", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "source")
		return ok && b.activeURL() == sourceURL
	})
}

func TestJourneyWindowOpenOpensHostTab(t *testing.T) {
	var popupHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/popup" {
			popupHits.Add(1)
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="popup-landed">Popup</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body><h1 id="window-source">Source</h1><button id="open-popup" onclick="window.open('/popup', '_blank')">open</button></body></html>`)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/window-source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "window.open source is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "open-popup")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()

	b.clickElementByID(t, "open-popup")
	b.waitFor(t, "window.open opens a second host tab", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/popup")
	})
	b.waitFor(t, "the popup paints its document", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "popup-landed")
		return ok
	})
	b.waitFor(t, "the popup was fetched exactly once", func() bool {
		return popupHits.Load() == 1
	})
}

func TestJourneyNamedTargetReusesHostTab(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/first":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="first-done">First</h1></body></html>`)
		case "/second":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="second-done">Second</h1></body></html>`)
		default:
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="named-source">Source</h1><a id="first-link" href="/first" target="report">first</a><a id="second-link" href="/second" target="report">second</a></body></html>`)
		}
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/named-source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "named target source is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "first-link")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()

	b.clickElementByID(t, "first-link")
	b.waitFor(t, "named target opens a second tab", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/first")
	})
	namedID := b.f.tabMgr.Active().ID()
	b.waitFor(t, "named tab has its name", func() bool {
		return b.f.tabMgr.Active() != nil && b.f.tabMgr.Active().Name() == "report"
	})

	b.f.switchTab(sourceID)
	b.waitFor(t, "source is active again", func() bool { return b.activeURL() == sourceURL })
	b.clickElementByID(t, "second-link")
	b.waitFor(t, "named target reuses the same tab", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() == namedID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/second")
	})
}

func evalTabJS(t *testing.T, b *browserE2E, tabID uint64, expr string) string {
	t.Helper()
	tab := b.f.tabMgr.TabByID(tabID)
	if tab == nil {
		t.Fatalf("tab %d not found", tabID)
	}
	session := tab.Session()
	if session == nil {
		t.Fatalf("tab %d has no session", tabID)
	}
	value, err := session.EvaluateJS(expr)
	if err != nil {
		t.Fatalf("EvaluateJS(%q): %v", expr, err)
	}
	return strings.TrimSpace(value)
}

func TestJourneyPopupPostMessageRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/popup" {
			fmt.Fprint(w, `<!doctype html><html><body><script>
window.__messages = [];
window.__openerName = window.opener === null ? "none" : window.opener.name;
window.addEventListener("message", function(event) {
  window.__messages.push(event.origin + "|" + event.data + "|" + (event.source === window.opener));
  if (event.data === "ping") window.opener.postMessage("pong:" + window.name, "*");
});
</script><h1 id="popup-landed">Popup</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body><script>
window.name = "origin";
window.__messages = [];
window.addEventListener("message", function(event) {
  window.__messages.push(event.origin + "|" + event.data + "|" + event.source.name + "|" + (event.source === window.__popup));
});
</script><button id="open" onclick="window.__popup = window.open('/popup', 'child'); window.__popup.postMessage('ping', '*'); window.__popup.postMessage('secret', 'https://wrong.test');">open</button></body></html>`)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "popup messaging source is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "open")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()

	b.clickElementByID(t, "open")
	b.waitFor(t, "popup opens as a host tab", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/popup")
	})
	popupID := b.f.tabMgr.Active().ID()
	b.waitFor(t, "popup receives the allowed message and rejects the wrong target origin", func() bool {
		return evalTabJS(t, b, popupID, "window.__messages.join(',')") == server.URL+"|ping|true"
	})
	if got := evalTabJS(t, b, popupID, "window.name"); got != "child" {
		t.Fatalf("popup window.name = %q, want %q", got, "child")
	}
	if got := evalTabJS(t, b, popupID, "window.__openerName"); got != "origin" {
		t.Fatalf("popup opener name = %q, want %q", got, "origin")
	}

	b.f.switchTab(sourceID)
	b.waitFor(t, "opener receives the popup reply with source metadata", func() bool {
		return evalTabJS(t, b, sourceID, "window.__messages.join(',')") == server.URL+"|pong:child|child|true"
	})

	evalTabJS(t, b, sourceID, "window.__popup.focus()")
	b.waitFor(t, "popup.focus activates the popup tab", func() bool {
		active := b.f.tabMgr.Active()
		return active != nil && active.ID() == popupID
	})
	b.f.switchTab(sourceID)
	b.waitFor(t, "source is active before closing the popup", func() bool { return b.activeURL() == sourceURL })
	evalTabJS(t, b, sourceID, "window.__popup.close()")
	b.waitFor(t, "popup.close closes the host tab", func() bool {
		return b.f.tabMgr.Count() == 1 && b.f.tabMgr.TabByID(popupID) == nil
	})
	b.waitFor(t, "popup proxy reports closed", func() bool {
		return evalTabJS(t, b, sourceID, "window.__popup.closed") == "true"
	})
}

func TestJourneyPopupNoOpenerHasNullOpener(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/popup-noopener" {
			fmt.Fprint(w, `<!doctype html><html><body><script>
window.__opener = window.opener === null ? "null" : "object";
window.__name = window.name;
</script><h1 id="noopener-landed">No opener</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body><button id="open" onclick="window.__result = window.open('/popup-noopener', 'openerless', 'noopener') === null ? 'null' : 'proxy';">open</button></body></html>`)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "noopener source is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "open")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()

	b.clickElementByID(t, "open")
	b.waitFor(t, "noopener popup opens a host tab", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/popup-noopener")
	})
	popupID := b.f.tabMgr.Active().ID()
	b.waitFor(t, "noopener popup paints", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "noopener-landed")
		return ok
	})
	if got := evalTabJS(t, b, popupID, "window.__opener"); got != "null" {
		t.Fatalf("noopener popup opener = %q, want null", got)
	}
	if got := evalTabJS(t, b, popupID, "window.__name"); got != "openerless" {
		t.Fatalf("noopener popup name = %q, want %q", got, "openerless")
	}
	b.f.switchTab(sourceID)
	if got := evalTabJS(t, b, sourceID, "window.__result"); got != "null" {
		t.Fatalf("window.open with noopener returned %q, want null", got)
	}
}

func TestJourneyBlankPopupInheritsOpenerOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body><button id="open" onclick="window.__popup = window.open(); window.__popup.postMessage('blank-ping', '/');">open</button></body></html>`)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "blank popup source is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "open")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()

	b.clickElementByID(t, "open")
	b.waitFor(t, "window.open with no URL opens a blank host tab", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			b.activeURL() == "about:blank"
	})
	popupID := b.f.tabMgr.Active().ID()
	b.waitFor(t, "blank popup has a live JS runtime", func() bool {
		value, err := b.f.tabMgr.TabByID(popupID).Session().EvaluateJS("1 + 1")
		return err == nil && value == "2"
	})
	evalTabJS(t, b, popupID, "window.__blankMessages = []; window.addEventListener('message', function(event) { window.__blankMessages.push(event.origin + '|' + event.data); });")
	b.f.switchTab(sourceID)
	b.waitFor(t, "blank popup source keeps its document", func() bool { return b.activeURL() == sourceURL })
	evalTabJS(t, b, sourceID, "window.__popup.postMessage('later', '/')")
	b.f.switchTab(popupID)
	b.waitFor(t, "blank popup inherits the opener origin for targetOrigin '/'", func() bool {
		return evalTabJS(t, b, popupID, "window.__blankMessages.join(',')") == server.URL+"|later"
	})
}

func TestJourneyAnchorBaseTargetBlankOpensHostTab(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/landing" {
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="base-landed">Base landed</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><head><base target="_blank"></head><body><h1 id="base-source">Base source</h1><a id="base-open" href="/landing">open</a></body></html>`)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/base-source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "base target source is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "base-open")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()

	b.clickElementByID(t, "base-open")
	b.waitFor(t, "base target=_blank opens a second host tab", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/landing")
	})
}

func TestJourneyPopupMessageCloneTransferAndTargetOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/clone-popup" {
			fmt.Fprint(w, `<!doctype html><html><body><script>
window.__messages = [];
window.addEventListener("message", function(event) {
  var data = event.data;
  if (data && data.kind === "payload") {
    window.__messages.push(data.kind + "|" + (data.err instanceof Error) + "|" + data.err.name + "|" + data.err.message + "|" + data.bytes.length + "|" + data.bytes[0] + "|" + data.bytes.buffer.byteLength);
  }
});
</script><h1 id="clone-landed">Clone</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body><button id="open" onclick="window.__runClone()">open</button><script>
window.__messages = [];
window.addEventListener("message", function(event) { window.__messages.push(String(event.data)); });
window.__runClone = function() {
  var proxy = window.open("/clone-popup", "clone-child");
  var invalid = "ok";
  try { proxy.postMessage("x", "https://a.test/path"); } catch (error) { invalid = error.name; }
  var buffer = new Uint8Array([7, 8, 9]).buffer;
  proxy.postMessage({ kind: "payload", err: new TypeError("boom"), bytes: new Uint8Array(buffer) }, { targetOrigin: "*", transfer: [buffer] });
  window.__openResult = invalid + "|" + buffer.byteLength;
};
</script></body></html>`)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "clone source is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "open")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()

	b.clickElementByID(t, "open")
	b.waitFor(t, "clone popup opens as a host tab", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/clone-popup")
	})
	popupID := b.f.tabMgr.Active().ID()
	if got := evalTabJS(t, b, sourceID, "window.__openResult"); got != "SyntaxError|0" {
		t.Fatalf("postMessage result = %q, want %q", got, "SyntaxError|0")
	}
	b.waitFor(t, "popup receives cloned error and transferred buffer", func() bool {
		return evalTabJS(t, b, popupID, "window.__messages.join(',')") == "payload|true|TypeError|boom|3|7|3"
	})
}

func TestJourneyWindowOpenNamedReuseProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/one":
			fmt.Fprint(w, `<!doctype html><html><body><script>window.__messages = []; window.addEventListener("message", function(event) { window.__messages.push(String(event.data)); });</script><h1 id="one-landed">One</h1></body></html>`)
		case "/two":
			fmt.Fprint(w, `<!doctype html><html><body><script>window.__messages = []; window.addEventListener("message", function(event) { window.__messages.push(String(event.data)); }); window.__closedBefore = window.closed;</script><h1 id="two-landed">Two</h1></body></html>`)
		default:
			fmt.Fprint(w, `<!doctype html><html><body><script>
window.__longName = "a-very-long-window-name-that-exceeds-sixteen-bytes";
window.name = window.__longName;
</script><button id="open-one" onclick="window.__w1 = window.open('/one', 'reuse-proxy')">one</button><button id="open-two" onclick="window.__w2 = window.open('/two', 'reuse-proxy'); window.__w2.postMessage('reused', '*'); window.__w1.postMessage('old-proxy', '*');">two</button></body></html>`)
		}
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "named proxy source is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "open-one")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()
	longName := evalTabJS(t, b, sourceID, "window.__longName")
	b.waitFor(t, "long window.name reaches the host tab", func() bool {
		tab := b.f.tabMgr.TabByID(sourceID)
		return tab != nil && tab.Name() == longName
	})

	b.clickElementByID(t, "open-one")
	b.waitFor(t, "first named proxy tab opens", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/one")
	})
	namedID := b.f.tabMgr.Active().ID()
	b.f.switchTab(sourceID)
	b.waitFor(t, "source is active before named reuse", func() bool { return b.activeURL() == sourceURL })

	b.clickElementByID(t, "open-two")
	b.waitFor(t, "named proxy reuses the existing tab", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() == namedID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/two")
	})
	b.waitFor(t, "messages from both proxies reach the reused document", func() bool {
		return evalTabJS(t, b, namedID, "window.__messages.slice().sort().join(',')") == "old-proxy,reused"
	})
	if got := evalTabJS(t, b, namedID, "window.__closedBefore"); got != "false" {
		t.Fatalf("window.closed before close = %q, want false", got)
	}

	evalTabJS(t, b, namedID, "window.close()")
	b.waitFor(t, "window.close closes the reused tab", func() bool {
		return b.f.tabMgr.Count() == 1 && b.f.tabMgr.TabByID(namedID) == nil
	})
	b.f.switchTab(sourceID)
	b.waitFor(t, "both reused proxies report closed", func() bool {
		return evalTabJS(t, b, sourceID, "window.__w1.closed && window.__w2.closed") == "true"
	})
}

func TestJourneyBackgroundWindowMessageDelivery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/background-popup" {
			fmt.Fprint(w, `<!doctype html><html><body><script>window.__messages = []; window.addEventListener("message", function(event) { window.__messages.push(String(event.data)); });</script><h1 id="background-landed">Background</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body><button id="open" onclick="window.__popup = window.open('/background-popup', 'background-child', 'popup')">open</button></body></html>`)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "background source is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "open")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()

	b.clickElementByID(t, "open")
	b.waitFor(t, "background popup opens", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/background-popup")
	})
	popupID := b.f.tabMgr.Active().ID()
	b.f.switchTab(sourceID)
	b.waitFor(t, "source is active while popup is backgrounded", func() bool { return b.activeURL() == sourceURL })
	evalTabJS(t, b, sourceID, "window.__popup.postMessage('background', '*')")
	b.waitFor(t, "background popup receives the message without activation", func() bool {
		if b.f.tabMgr.Active() == nil || b.f.tabMgr.Active().ID() != sourceID {
			return false
		}
		return evalTabJS(t, b, popupID, "window.__messages.join(',')") == "background"
	})
}

func TestJourneyWindowOpenFeatureTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/noopener-phrase":
			fmt.Fprint(w, `<!doctype html><html><body><script>window.__opener = window.opener === null ? "null" : "object"; window.__name = window.name;</script><h1 id="phrase-landed">Phrase</h1></body></html>`)
		case "/noopener-zero":
			fmt.Fprint(w, `<!doctype html><html><body><script>window.__opener = window.opener === null ? "null" : "object"; window.__name = window.name;</script><h1 id="zero-landed">Zero</h1></body></html>`)
		default:
			fmt.Fprint(w, `<!doctype html><html><body><button id="phrase" onclick="window.__phrase = window.open('/noopener-phrase', 'phrase-name', 'make me noopener') === null ? 'null' : 'proxy';">phrase</button><button id="zero" onclick="window.__zero = window.open('/noopener-zero', 'zero-name', 'noopener=0') === null ? 'null' : 'proxy';">zero</button></body></html>`)
		}
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)
	sourceURL := server.URL + "/source"
	b.navigateViaAddressBar(t, sourceURL)
	b.waitFor(t, "feature source is ready", func() bool {
		_, ok := findAXNode(b.window.AccessibilityTree(), "phrase")
		return ok && b.activeURL() == sourceURL
	})
	sourceID := b.f.tabMgr.Active().ID()

	b.clickElementByID(t, "phrase")
	b.waitFor(t, "whitespace noopener opens a popup", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 2 && active != nil && active.ID() != sourceID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/noopener-phrase")
	})
	phraseID := b.f.tabMgr.Active().ID()
	b.waitFor(t, "whitespace noopener popup is openerless and named", func() bool {
		return evalTabJS(t, b, phraseID, "window.__opener === 'null' && window.__name === 'phrase-name'") == "true"
	})
	b.f.switchTab(sourceID)
	if got := evalTabJS(t, b, sourceID, "window.__phrase"); got != "null" {
		t.Fatalf("window.open with whitespace noopener returned %q, want null", got)
	}

	b.clickElementByID(t, "zero")
	b.waitFor(t, "noopener=0 opens a popup", func() bool {
		active := b.f.tabMgr.Active()
		return b.f.tabMgr.Count() == 3 && active != nil && active.ID() != sourceID && active.ID() != phraseID &&
			strings.HasPrefix(b.activeURL(), server.URL+"/noopener-zero")
	})
	zeroID := b.f.tabMgr.Active().ID()
	b.waitFor(t, "noopener=0 popup keeps its opener", func() bool {
		return evalTabJS(t, b, zeroID, "window.__opener === 'object' && window.__name === 'zero-name'") == "true"
	})
	b.f.switchTab(sourceID)
	if got := evalTabJS(t, b, sourceID, "window.__zero"); got != "proxy" {
		t.Fatalf("window.open with noopener=0 returned %q, want proxy", got)
	}
}
