package engine_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestIframeCreatesDistinctAboutBlankDocumentAndFiresLoad(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><html><body><script>
var frame = document.createElement('iframe');
var childWindow = frame.contentWindow;
var childDocument = frame.contentDocument;
var childDocumentIdentity = childWindow.document === childDocument;
var childDocumentIsDistinct = childDocument !== document;
var iframeLoaded = false;
frame.onload = function() { iframeLoaded = true; };
document.body.appendChild(frame);
</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if errors := sess.ScriptErrors(); len(errors) != 0 {
		t.Errorf("script errors: %+v", errors)
	}

	for name, want := range map[string]bool{
		"childDocumentIdentity":   true,
		"childDocumentIsDistinct": true,
		"iframeLoaded":            true,
	} {
		got, ok := rt.Global(name)
		if !ok || got.Bool != want {
			t.Errorf("%s = %v (present %v), want %v", name, got.Bool, ok, want)
		}
	}
}

func TestTreeWalkerReturnsNodesInIframeDocumentRealm(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><html><body><script>
var frame = document.createElement('iframe');
frame.srcdoc = '<!doctype html>';
document.body.appendChild(frame);
var treeWalker = document.createTreeWalker(frame.contentDocument, NodeFilter.SHOW_ELEMENT);
var returnedNode = treeWalker.nextNode();
var childNodeConstructor = frame.contentWindow.Node;
var returnedNodeInstanceOfChildNode = returnedNode instanceof childNodeConstructor;
</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if errors := sess.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("script errors: %+v", errors)
	}
	for name, want := range map[string]bool{
		"returnedNodeInstanceOfChildNode": true,
	} {
		got, ok := rt.Global(name)
		if !ok || got.Bool != want {
			t.Errorf("%s = %v (present %v), want %v", name, got.Bool, ok, want)
		}
	}
}

func TestIframeLoaderInstallsFetchedChildDocumentBeforeLoad(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	loader := func(base, src string) (*dom.Document, error) {
		if base != "https://example.test/parent" || src != "/child" {
			t.Fatalf("iframe loader got base=%q src=%q", base, src)
		}
		return dom.ParseBounded(`<!DOCTYPE ><p id="child">loaded child</p>`, dom.ParseLimits{
			Nodes: 64, Depth: 16, Attributes: 32, AttributeBytes: 1024,
		})
	}
	html := `<!doctype html><html><body><script>
var frame = document.createElement('iframe');
var childText = '';
var childDoctypeName = 'missing';
frame.onload = function() {
  childText = frame.contentDocument.body.textContent;
  childDoctypeName = frame.contentDocument.doctype.name;
};
frame.src = '/child';
document.body.appendChild(frame);
</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://example.test/parent", nil),
		engine.WithIFrameDocumentLoader(loader),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if errors := sess.ScriptErrors(); len(errors) != 0 {
		t.Errorf("script errors: %+v", errors)
	}

	if got, ok := rt.Global("childText"); !ok || got.Str != "loaded child" {
		t.Errorf("iframe child text = %q (present %v), want %q", got.Str, ok, "loaded child")
	}
	if got, ok := rt.Global("childDoctypeName"); !ok || got.Str != "" {
		t.Errorf("iframe doctype name = %q (present %v), want an empty string", got.Str, ok)
	}
}

func TestPumpJSProcessesIframeNavigationQueuedAfterStartup(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	loader := func(base, src string) (*dom.Document, error) {
		if base != "https://example.test/parent" || src != "/dynamic" {
			t.Fatalf("iframe loader got base=%q src=%q", base, src)
		}
		return dom.ParseBounded(`<!doctype html><body><p>dynamic child</p></body>`, dom.ParseLimits{
			Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 1024,
		})
	}
	sess, err := engine.NewSession(`<!doctype html><html><body><iframe id="child"></iframe><script>
window.childLoadCount = 0;
document.getElementById('child').addEventListener('load', function() {
  window.childLoadCount++;
  window.childText = document.getElementById('child').contentDocument.body.textContent;
});
</script></body></html>`, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://example.test/parent", nil),
		engine.WithIFrameDocumentLoader(loader),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if _, err := sess.EvaluateJS(`window.childLoadCount = 0; window.childText = ''; document.getElementById('child').src = '/dynamic'`); err != nil {
		t.Fatal(err)
	}
	if got, err := sess.EvaluateJS(`String(window.childLoadCount)`); err != nil || got != "0" {
		t.Fatalf("child load ran before an event-loop pump: %q, %v", got, err)
	}
	sess.PumpJS(800)
	if got, err := sess.EvaluateJS(`window.childLoadCount + '|' + window.childText`); err != nil || got != "1|dynamic child" {
		t.Fatalf("post-startup iframe navigation = %q, %v; want one load with child document", got, err)
	}
}

func TestExternalIframeRunsScriptsAndResolvesNestedFramesFromChildURL(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	parse := func(markup string) *dom.Document {
		t.Helper()
		doc, parseErr := dom.ParseBounded(markup, dom.ParseLimits{
			Nodes: 256, Depth: 32, Attributes: 256, AttributeBytes: 1 << 14,
		})
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		return doc
	}
	var loaderCalls []string
	loader := func(base, src string) (*dom.Document, error) {
		loaderCalls = append(loaderCalls, base+" -> "+src)
		switch {
		case base == "https://example.test/parent" && src == "/child":
			return parse(`<!doctype html><iframe src="/nested"></iframe><script src="script.js"></script>`), nil
		case base == "https://example.test/child" && src == "/nested":
			return parse(`<!doctype html><p id="nested">nested document</p>`), nil
		default:
			t.Fatalf("unexpected iframe loader request base=%q src=%q", base, src)
			return nil, nil
		}
	}
	scriptFetcher := func(base, src string) ([]byte, error) {
		if base != "https://example.test/child" || src != "https://example.test/script.js" {
			t.Fatalf("child script fetch got base=%q src=%q", base, src)
		}
		return []byte(`window.childScriptRan = true; var nestedFrame = document.querySelector("iframe"); nestedFrame.onload = function() { parent.postMessage("nested:" + nestedFrame.contentDocument.body.textContent, "*"); }; parent.postMessage("external-script", "*");`), nil
	}
	html := `<!doctype html><html><body><script>
window.childMessage = 'pending';
window.childNestedMessage = 'pending';
window.childScriptRan = false;
window.addEventListener('message', function(event) {
  var result = [event.data, event.origin, event.source === frame.contentWindow].join('|');
  if (event.data === 'external-script') window.childMessage = result;
  if (event.data.indexOf('nested:') === 0) window.childNestedMessage = result;
});
var frame = document.createElement('iframe');
frame.onload = function() {
  window.childScriptRan = frame.contentWindow.childScriptRan === true;
};
frame.src = '/child';
document.body.appendChild(frame);
</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://example.test/parent", nil),
		engine.WithScripts("https://example.test/parent", scriptFetcher),
		engine.WithIFrameDocumentLoader(loader),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if errors := sess.ScriptErrors(); len(errors) != 0 {
		t.Errorf("script errors: %+v", errors)
	}
	if got, ok := rt.Global("childMessage"); !ok || got.Str != "pending" {
		t.Fatalf("external child postMessage was synchronous: %+v", got)
	}
	if got, ok := rt.Global("childScriptRan"); !ok || !got.Bool {
		t.Fatalf("external iframe child script did not run: %+v", got)
	}
	for i := 0; i < 4; i++ {
		rt.Tick()
	}
	if got, ok := rt.Global("childMessage"); !ok || got.Str != "external-script|https://example.test|true" {
		t.Fatalf("external child postMessage result = %+v", got)
	}
	if got, ok := rt.Global("childNestedMessage"); !ok || got.Str != "nested:nested document|https://example.test|true" {
		t.Fatalf("nested child load/postMessage result = %+v", got)
	}
	wantCalls := []string{
		"https://example.test/parent -> /child",
		"https://example.test/child -> /nested",
	}
	if len(loaderCalls) != len(wantCalls) {
		t.Fatalf("iframe loader calls = %v, want %v", loaderCalls, wantCalls)
	}
	for i := range wantCalls {
		if loaderCalls[i] != wantCalls[i] {
			t.Fatalf("iframe loader calls = %v, want %v", loaderCalls, wantCalls)
		}
	}
}

func TestExternalIframePostMessageResolvesAsyncAwait(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	child, err := dom.ParseBounded(`<!doctype html><script type="module">
import values from "/types.js";
parent.postMessage(values[0], "*");
</script>`, dom.ParseLimits{
		Nodes: 64, Depth: 16, Attributes: 32, AttributeBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	fetchScript := func(base, src string) ([]byte, error) {
		if base != "https://example.test/child" || src != "https://example.test/types.js" {
			t.Fatalf("unexpected module fetch base=%q src=%q", base, src)
		}
		return []byte(`export default ["child-ready"];`), nil
	}
	session, err := engine.NewSession(`<!doctype html><iframe src="/child"></iframe><script>
window.awaitedMessage = "pending";
var message = new Promise(resolve => window.addEventListener("message", event => resolve(event.data), {once: true}));
(async function() { window.awaitedMessage = await message; })();
</script>`, nil, 800,
		engine.WithJS(rt),
		engine.WithScripts("https://example.test/parent", fetchScript),
		engine.WithIFrameDocumentLoader(func(base, src string) (*dom.Document, error) {
			if base != "https://example.test/parent" || src != "/child" {
				t.Fatalf("unexpected iframe request base=%q src=%q", base, src)
			}
			return child, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if errors := session.ScriptErrors(); len(errors) != 0 {
		t.Fatalf("child module script errors: %+v", errors)
	}
	for i := 0; i < 8; i++ {
		rt.Tick()
	}
	if got, ok := rt.Global("awaitedMessage"); !ok || got.Str != "child-ready" {
		t.Fatalf("async message result = %+v (present %v), want child-ready", got, ok)
	}
}

func TestWindowIndexedFramePostMessageTransfersMessagePortAcrossRuntimes(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	parse := func(markup string) *dom.Document {
		t.Helper()
		doc, parseErr := dom.ParseBounded(markup, dom.ParseLimits{
			Nodes: 256, Depth: 32, Attributes: 256, AttributeBytes: 1 << 14,
		})
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		return doc
	}
	sess, err := engine.NewSession(`<!doctype html><html><body><script>
window.receivedOverPort = 'pending';
window.indexedFrameMatches = false;
var channel = new MessageChannel();
channel.port2.onmessage = function(event) { window.receivedOverPort = event.data; };
var frame = document.createElement('iframe');
frame.onload = function() {
  window.indexedFrameMatches = window[0] === frame.contentWindow && window.frames === window && window.length === 1;
  window[0].postMessage('cross-runtime', '*', [channel.port1]);
};
frame.src = '/child';
document.body.appendChild(frame);
</script></body></html>`, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://example.test/parent", nil),
		engine.WithScripts("https://example.test/parent", nil),
		engine.WithIFrameDocumentLoader(func(base, src string) (*dom.Document, error) {
			if base != "https://example.test/parent" || src != "/child" {
				t.Fatalf("unexpected iframe request base=%q src=%q", base, src)
			}
			return parse(`<!doctype html><script>onmessage = function(event) { event.ports[0].postMessage(event.data + '-reply'); };</script>`), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if got := sess.ScriptErrors(); len(got) != 0 {
		t.Fatalf("script errors: %+v", got)
	}
	for i := 0; i < 8; i++ {
		rt.Tick()
	}
	if got, ok := rt.Global("indexedFrameMatches"); !ok || !got.Bool {
		t.Fatalf("Window indexed frame properties were incorrect: %+v", got)
	}
	if got, ok := rt.Global("receivedOverPort"); !ok || got.Str != "cross-runtime-reply" {
		t.Fatalf("cross-runtime MessagePort response = %+v, want cross-runtime-reply", got)
	}
}

func TestMessageChannelTransfersAcrossTwoSiblingFrames(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	parse := func(markup string) *dom.Document {
		t.Helper()
		doc, parseErr := dom.ParseBounded(markup, dom.ParseLimits{
			Nodes: 256, Depth: 32, Attributes: 256, AttributeBytes: 1 << 14,
		})
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		return doc
	}
	loader := func(base, src string) (*dom.Document, error) {
		if base != "https://example.test/parent" {
			t.Fatalf("unexpected iframe base %q", base)
		}
		switch src {
		case "/first":
			return parse(`<!doctype html><script>onmessage = function(event) { var port = event.ports[0]; if (event.ports.length !== 1 || !(port instanceof MessagePort) || Object.prototype.toString.call(port) !== '[object MessagePort]') { parent.postMessage('invalid first transferred port', '*'); return; } port.postMessage(event.data); };</script>`), nil
		case "/second":
			return parse(`<!doctype html><script>onmessage = function(event) { var port = event.ports[0]; if (event.ports.length !== 1 || !(port instanceof MessagePort) || Object.prototype.toString.call(port) !== '[object MessagePort]') { parent.postMessage('invalid second transferred port', '*'); return; } port.onmessage = function(message) { parent.postMessage('relay:' + message.data, '*'); }; };</script>`), nil
		default:
			t.Fatalf("unexpected iframe URL %q", src)
			return nil, nil
		}
	}
	html := `<!doctype html><html><body><script>
window.relayedMessage = 'pending'; window.messageError = '';
window.onmessage = function(event) { if (String(event.data).indexOf('invalid ') === 0) window.messageError = String(event.data); if (String(event.data).indexOf('relay:') === 0) window.relayedMessage = String(event.data); };
var first = document.createElement('iframe'); first.src = '/first'; document.body.appendChild(first);
var second = document.createElement('iframe'); second.src = '/second'; document.body.appendChild(second);
window.onload = function() {
  var channel = new MessageChannel();
  window[0].postMessage(1, '*', [channel.port1]);
  window[1].postMessage(2, '*', [channel.port2]);
};
</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://example.test/parent", nil),
		engine.WithScripts("https://example.test/parent", nil),
		engine.WithIFrameDocumentLoader(loader),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if scriptErrors := sess.ScriptErrors(); len(scriptErrors) != 0 {
		t.Fatalf("script errors: %+v", scriptErrors)
	}
	for i := 0; i < 8; i++ {
		rt.Tick()
	}
	if got, ok := rt.Global("relayedMessage"); !ok || got.Str != "relay:1" {
		t.Fatalf("sibling-frame MessagePort relay = %+v, want relay:1", got)
	}
	if got, ok := rt.Global("messageError"); !ok || got.Str != "" {
		t.Fatalf("transferred MessagePort wrapper validation failed: %+v", got)
	}
}

func TestIframeFormSubmitNavigatesAndCallsParent(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/forms/test.html"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	parse := func(markup string) *dom.Document {
		t.Helper()
		doc, parseErr := dom.ParseBounded(markup, dom.ParseLimits{Nodes: 256, Depth: 32, Attributes: 256, AttributeBytes: 1 << 14})
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		return doc
	}
	var requests []string
	loader := func(base, src string) (*dom.Document, error) {
		requests = append(requests, base+" -> "+src)
		u, parseErr := url.Parse(src)
		if parseErr != nil {
			return nil, parseErr
		}
		if strings.HasSuffix(u.Path, "/form.html") {
			return parse(`<form action="target.html"><input name="name" value="value"></form><script>if (location.search.startsWith("?name=value")) parent.success(location.href); else document.querySelector("form").submit();</script>`), nil
		}
		if strings.HasSuffix(u.Path, "/target.html") {
			return parse(`<script>parent.success(location.href);</script>`), nil
		}
		return nil, fmt.Errorf("unexpected iframe URL %q", src)
	}
	html := `<!doctype html><script>window.success = function(url) { window.formNavigationURL = url; };</script><iframe src="https://example.test/forms/form.html"></iframe>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt), engine.WithLinkedCSS("https://example.test/forms/test.html", nil), engine.WithIFrameDocumentLoader(loader))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	// Form submission is a planned navigation and runs on the child browsing
	// context's next event-loop turn.
	sess.PumpJS(800)
	if scriptErrors := sess.ScriptErrors(); len(scriptErrors) != 0 {
		t.Errorf("iframe script errors: %+v", scriptErrors)
	}
	if got, ok := rt.Global("formNavigationURL"); !ok || got.Str != "https://example.test/forms/target.html?name=value" {
		t.Errorf("form navigation callback = %+v (present %v)", got, ok)
	}
	if len(requests) != 2 {
		t.Errorf("iframe requests = %v, want initial document and submitted target", requests)
	}
}

func TestExternalIframeWindowOnErrorHandlerForwardsErrorEventArguments(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	childDoc, err := dom.ParseBounded(`<!doctype html><body></body><script>
document.body.outerHTML = "<body onerror='parent.captureError(event, source, lineno, colno, error, arguments.length, arguments.callee.length)'></body>";
window.dispatchEvent(parent.errorEvent);
</script>`, dom.ParseLimits{Nodes: 128, Depth: 32, Attributes: 128, AttributeBytes: 1 << 14})
	if err != nil {
		t.Fatal(err)
	}
	html := `<!doctype html><html><body><script>
window.errorObject = {marker: 'identity'};
window.errorEvent = new ErrorEvent('error', {message: 'child failure', filename: 'child.js', lineno: 7, colno: 3, error: errorObject});
window.captureError = function(event, source, lineno, colno, error, argumentCount, handlerLength) {
  window.capturedError = [event, source, lineno, colno, error, argumentCount, handlerLength];
};
var frame = document.createElement('iframe');
frame.src = '/child';
document.body.appendChild(frame);
</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://example.test/parent", nil),
		engine.WithIFrameDocumentLoader(func(base, src string) (*dom.Document, error) {
			if base != "https://example.test/parent" || src != "/child" {
				t.Fatalf("iframe loader got base=%q src=%q", base, src)
			}
			return childDoc, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if got, ok := rt.Evaluate(`capturedError && [capturedError[0], capturedError[1], capturedError[2], capturedError[3], capturedError[4] === errorObject, capturedError[5], capturedError[6]].join('|')`); ok != nil || got != "child failure|child.js|7|3|true|5|5" {
		t.Fatalf("cross-realm Window.onerror arguments = %q (error %v)", got, ok)
	}
}

func TestIframeSrcdocParsesChildDocumentBeforeLoad(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><html><body><script>
var frame = document.createElement('iframe');
frame.srcdoc = '<!doctype html><p id="child">srcdoc text</p>';
var srcdocReflected = frame.getAttribute('srcdoc') === frame.srcdoc;
var srcdocLoadedText = '';
frame.onload = function() { srcdocLoadedText = frame.contentDocument.getElementById('child').textContent; };
document.body.appendChild(frame);
</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if errors := sess.ScriptErrors(); len(errors) != 0 {
		t.Errorf("script errors: %+v", errors)
	}
	if got, ok := rt.Global("srcdocReflected"); !ok || !got.Bool {
		t.Errorf("srcdoc property reflection = %v (present %v), want true", got.Bool, ok)
	}
	if got, ok := rt.Global("srcdocLoadedText"); !ok || got.Str != "srcdoc text" {
		t.Errorf("srcdoc child text = %q (present %v), want %q", got.Str, ok, "srcdoc text")
	}
}

func TestIframeSrcdocScriptsRunInASeparateRealmDuringParsing(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><html><body><script>
window.parentMarker = 'parent';
var frame = document.createElement('iframe');
frame.srcdoc = '<!doctype html><body><p id="child">child</p><script>window.childMarker=window.parent.parentMarker;window.childNode=document.getElementById("child");window.readParentWrittenValue=function(){return window.parentWrittenValue||""};document.body.appendChild(document.createElement("i"))<\/script>';
var childValueFromProxy = '';
var childText = '';
var childRealmIsSeparate = false;
var childDocIdentity = false;
var childValueFromParentWrite = '';
var childBodyNodeListLength = -1;
var childBodyNodeListFirst = '';
var childHeadIdentity = false;
var childOwnerDocumentIdentity = false;
frame.onload = function() {
  childValueFromProxy = frame.contentWindow.childMarker;
  childText = frame.contentDocument.getElementById('child').textContent;
  childBodyNodeListLength = frame.contentDocument.body.childNodes.length;
  childBodyNodeListFirst = frame.contentDocument.body.childNodes[0].localName;
  childHeadIdentity = frame.contentWindow.document.head === frame.contentDocument.head;
  childOwnerDocumentIdentity = frame.contentDocument.body.firstChild.ownerDocument === frame.contentDocument;
  childRealmIsSeparate = typeof window.childMarker === 'undefined';
  childDocIdentity = frame.contentWindow.document === frame.contentDocument;
  frame.contentWindow.parentWrittenValue = 'written through WindowProxy';
  childValueFromParentWrite = frame.contentWindow.readParentWrittenValue();
};
document.body.appendChild(frame);
</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://example.test/parent", nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if errors := sess.ScriptErrors(); len(errors) != 0 {
		t.Errorf("script errors: %+v", errors)
	}
	for name, want := range map[string]string{
		"childValueFromProxy":       "parent",
		"childText":                 "child",
		"childBodyNodeListFirst":    "p",
		"childValueFromParentWrite": "written through WindowProxy",
	} {
		got, ok := rt.Global(name)
		if !ok || got.Str != want {
			t.Errorf("%s = %q (present %v), want %q", name, got.Str, ok, want)
		}
	}
	if got, ok := rt.Global("childBodyNodeListLength"); !ok || got.Num != 3 {
		t.Errorf("iframe body childNodes length = %v (present %v), want 3", got.Num, ok)
	}
	for name, want := range map[string]bool{
		"childRealmIsSeparate":       true,
		"childDocIdentity":           true,
		"childHeadIdentity":          true,
		"childOwnerDocumentIdentity": true,
	} {
		got, ok := rt.Global(name)
		if !ok || got.Bool != want {
			t.Errorf("%s = %v (present %v), want %v", name, got.Bool, ok, want)
		}
	}
}

func TestIframeSrcdocCustomElementConstructsBeforeParserInsertion(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><html><body><script>
var frame = document.createElement('iframe');
frame.srcdoc = '<!doctype html><script>customElements.define("x-a", class extends HTMLElement { constructor(){ super(); window.constructed=true; window.bodyBeforeInsert=document.body; document.body.remove(); } })<\/script><body><x-a><p>child</p></x-a>';
var customElementLoaded = false;
var childBodyIsNull = false;
var childBodyContainsElement = false;
frame.onload = function() {
  customElementLoaded = frame.contentWindow.constructed === true;
  childBodyIsNull = frame.contentDocument.body === null;
  childBodyContainsElement = frame.contentWindow.bodyBeforeInsert.firstChild.localName === 'x-a';
};
document.body.appendChild(frame);
</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if errors := sess.ScriptErrors(); len(errors) != 0 {
		t.Errorf("script errors: %+v", errors)
	}
	for name, want := range map[string]bool{
		"customElementLoaded":      true,
		"childBodyIsNull":          true,
		"childBodyContainsElement": true,
	} {
		got, ok := rt.Global(name)
		if !ok || got.Bool != want {
			t.Errorf("%s = %v (present %v), want %v", name, got.Bool, ok, want)
		}
	}
}
