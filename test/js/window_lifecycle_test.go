package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func lifecycleRuntime(t *testing.T, html string) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(html)
	return newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
}

// qc:BR-204
func TestDOMContentLoadedReachesWindowAfterDocument(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body></body></html>`)
	if err := rt.Run(`
	var seen = [];
	window.addEventListener("DOMContentLoaded", function () { seen.push("window"); });
	document.addEventListener("DOMContentLoaded", function () { seen.push("document"); });`, "inline"); err != nil {
		t.Fatal(err)
	}
	if rt.FireDocumentLifecycle() {
		t.Fatalf("FireDocumentLifecycle reported a DOM mutation for a listener that only appended to an array")
	}
	if got := evalString(t, rt, `seen.join(",")`); got != "document,window" {
		t.Fatalf("listener order = %q, want %q", got, "document,window")
	}
}

// qc:BR-204
func TestReadyStateTracksTheLifecycle(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body></body></html>`)
	if got := evalString(t, rt, `document.readyState`); got != "loading" {
		t.Fatalf("readyState during script execution = %q, want %q", got, "loading")
	}
	if err := rt.Run(`
	window.readyStateInHandler = "unset";
	document.addEventListener("DOMContentLoaded", function () {
		window.readyStateInHandler = document.readyState;
	});`, "inline"); err != nil {
		t.Fatal(err)
	}
	rt.FireDocumentLifecycle()
	if got := evalString(t, rt, `document.readyState`); got != "complete" {
		t.Fatalf("readyState after the lifecycle = %q, want %q", got, "complete")
	}
	if got := evalString(t, rt, `window.readyStateInHandler`); got != "interactive" {
		t.Fatalf("readyState seen inside DOMContentLoaded = %q, want %q", got, "interactive")
	}
	if got := evalString(t, rt, `document.readyState = "loading"; document.readyState`); got != "complete" {
		t.Fatalf("readyState after a script write = %q, want %q", got, "complete")
	}
}

// qc:BR-204
func TestReadyStateGuardRunsHandlerAfterLoad(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body><div id="box"></div></body></html>`)
	src := `
	window.ran = 0;
	function h() { document.getElementById("box").id = "box"; window.ran++; }
	if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", h);
	else h();`
	if err := rt.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `String(window.ran)`); got != "0" {
		t.Fatalf("handler ran %s times before the lifecycle, want 0", got)
	}
	rt.FireDocumentLifecycle()
	if got := evalString(t, rt, `String(window.ran)`); got != "1" {
		t.Fatalf("handler ran %s times after the lifecycle, want 1", got)
	}
	rt.FireDocumentLifecycle()
	if got := evalString(t, rt, `String(window.ran)`); got != "1" {
		t.Fatalf("second FireDocumentLifecycle re-ran the handler (%s), want 1 total", got)
	}
}

// qc:BR-204
func TestLoadEventTargetsDocumentAndReachesWindow(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body></body></html>`)
	if err := rt.Run(`
	var hits = [];
	window.addEventListener("load", function () { hits.push("window"); });
	document.addEventListener("load", function () { hits.push("document"); });`, "inline"); err != nil {
		t.Fatal(err)
	}
	rt.FireDocumentLifecycle()
	if got := evalString(t, rt, `hits.join(",")`); got != "document,window" {
		t.Fatalf("load listeners = %q, want %q", got, "document,window")
	}
}

func TestDocumentLifecycleEventsExposeTrustedTargetsAndInterfaces(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body></body></html>`)
	if err := rt.Run(`
	window.lifecycleResults = [];
	document.addEventListener("DOMContentLoaded", function(e) {
		lifecycleResults.push([e.type, e.target === document, e.isTrusted, e.bubbles, e.cancelable, Object.prototype.toString.call(e)].join("|"));
	});
	window.addEventListener("load", function(e) {
		lifecycleResults.push([e.type, e.target === document, e.isTrusted, e.bubbles, e.cancelable, Object.prototype.toString.call(e)].join("|"));
	});
	window.addEventListener("pageshow", function(e) {
		lifecycleResults.push([e.type, e.target === document, e.isTrusted, e.bubbles, e.cancelable, e.persisted, Object.prototype.toString.call(e)].join("|"));
	});`, "inline"); err != nil {
		t.Fatal(err)
	}
	rt.FireDocumentLifecycle()
	want := `DOMContentLoaded|true|true|true|false|[object Event],load|true|true|false|false|[object Event],pageshow|true|true|true|true|false|[object PageTransitionEvent]`
	if got := evalString(t, rt, `lifecycleResults.join(",")`); got != want {
		t.Fatalf("lifecycle events = %q, want %q", got, want)
	}
}

func TestWindowEventHandlerIDLAttribute(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body></body></html>`)
	if err := rt.Run(`
var seen = [];
window.addEventListener("load", function (e) {
  seen.push("listener:" + (this === window) + ":" + (e.currentTarget === window));
});
window.onload = function (e) {
  seen.push("handler:" + (this === window) + ":" + (e.currentTarget === window));
};
window.onload = window.onload;
`, "inline"); err != nil {
		t.Fatal(err)
	}
	rt.FireDocumentLifecycle()
	if got, want := evalString(t, rt, `seen.join(",")`), "listener:true:true,handler:true:true"; got != want {
		t.Fatalf("window load handlers = %q, want %q", got, want)
	}
}

func TestGlobalEventHandlerIDLAttributesAndReactivationOrder(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body><button id="target"></button></body></html>`)
	if err := rt.Run(`
var target = document.getElementById("target");
var order = [];
var firstHandler = function (event) {
  order.push("handler:" + (this === target) + ":" + (event.currentTarget === target));
};
var svgTarget = document.createElementNS("http://www.w3.org/2000/svg", "g");
var elementIDL = "onclick" in target && "onbeforeinput" in target && target.onclick === null;
var documentIDL = "onclick" in document && "onvisibilitychange" in document && document.onclick === null;
var windowIDL = "onclick" in window && "onpagehide" in window && window.onclick === null;
var prototypeIDL = HTMLElement.prototype.hasOwnProperty("onclick") &&
  SVGElement.prototype.hasOwnProperty("onclick") &&
  Document.prototype.hasOwnProperty("onclick") &&
  window.hasOwnProperty("onclick") && !Element.prototype.hasOwnProperty("onclick") &&
  svgTarget instanceof SVGElement && svgTarget.onclick === null;
target.addEventListener("click", function () { order.push("first"); });
target.onclick = firstHandler;
target.addEventListener("click", function () { order.push("second"); });
target.onclick = null;
target.addEventListener("click", function () { order.push("third"); });
target.onclick = firstHandler;
target.addEventListener("click", function () { order.push("fourth"); });
var dispatchSucceeds = target.dispatchEvent(new Event("click", {bubbles: true, cancelable: true}));
target.onclick = function () { return false; };
var canceledByHandler = target.dispatchEvent(new Event("click", {cancelable: true}));`, "inline"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"elementIDL", "documentIDL", "windowIDL", "prototypeIDL"} {
		if got, ok := rt.Global(name); !ok || !got.Bool {
			t.Errorf("%s = %v (present %v), want true", name, got.Bool, ok)
		}
	}
	if err := rt.Run(`
var windowOrder = [];
window.addEventListener("pageshow", function () { windowOrder.push("first"); });
window.onpageshow = function () { windowOrder.push("handler"); };
window.addEventListener("pageshow", function () { windowOrder.push("second"); });
window.onpageshow = null;
window.addEventListener("pageshow", function () { windowOrder.push("third"); });
window.onpageshow = function () { windowOrder.push("handler"); };
window.addEventListener("pageshow", function () { windowOrder.push("fourth"); });
window.oncontextmenu = function () { return false; };
var windowDispatchSucceeds = window.dispatchEvent(new Event("contextmenu", {cancelable: true}));
window.onerror = function () { return true; };
var errorDispatchSucceeds = window.dispatchEvent(new Event("error", {cancelable: true}));`, "window-handler-order"); err != nil {
		t.Fatal(err)
	}
	rt.FireDocumentLifecycle()
	if got, want := evalString(t, rt, `windowOrder.join(",")`), "first,second,third,handler,fourth"; got != want {
		t.Fatalf("window handler reactivation order = %q, want %q", got, want)
	}
	for name, want := range map[string]bool{"windowDispatchSucceeds": false, "errorDispatchSucceeds": false} {
		if got, ok := rt.Global(name); !ok || got.Bool != want {
			t.Errorf("%s = %v (present %v), want %v", name, got.Bool, ok, want)
		}
	}
	if got := evalString(t, rt, `order.join(",")`); got != "first,second,third,handler:true:true,fourth,first,second,third,fourth" {
		t.Fatalf("event handler order = %q, want first,second,third,handler:true:true,fourth,first,second,third,fourth", got)
	}
	for name, want := range map[string]bool{"dispatchSucceeds": true, "canceledByHandler": false} {
		if got, ok := rt.Global(name); !ok || got.Bool != want {
			t.Errorf("%s = %v (present %v), want %v", name, got.Bool, ok, want)
		}
	}
}

// qc:BR-204
func TestWindowListenerDedupAndRemoval(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body></body></html>`)
	if err := rt.Run(`
	window.count = 0;
	function h() { window.count++; }
	window.addEventListener("load", h);
	window.addEventListener("load", h);          // same function: ignored
	window.addEventListener("load", h, true);     // different phase: kept
	var before = window.count;`, "inline"); err != nil {
		t.Fatal(err)
	}
	rt.FireDocumentLifecycle()
	if got := evalString(t, rt, `String(window.count)`); got != "2" {
		t.Fatalf("load handler ran %s times with a duplicate registration, want 2 (one per phase)", got)
	}
	if err := rt.Run(`
	window.count = 0;
	window.removeEventListener("load", h);
	window.removeEventListener("load", h, true);`, "inline"); err != nil {
		t.Fatal(err)
	}
	rt2 := rt
	rt2.FireDocumentLifecycle()
	if got := evalString(t, rt, `String(window.count)`); got != "0" {
		t.Fatalf("count after a re-fire = %q, want 0 (the lifecycle is once per document)", got)
	}
	if got := evalString(t, rt, `String(before)`); got != "0" {
		t.Fatalf("count before the lifecycle = %q, want 0", got)
	}
}

// qc:BR-204
func TestWindowRemoveEventListenerBeforeDispatch(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body></body></html>`)
	if err := rt.Run(`
	window.hits = [];
	function h() { window.hits.push("hit"); }
	window.addEventListener("pageshow", h);
	window.removeEventListener("pageshow", h);
	window.dispatchEvent({ type: "pageshow" });`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `String(window.hits.length)`); got != "0" {
		t.Fatalf("removed listener fired %s times, want 0", got)
	}
	if err := rt.Run(`
	window.addEventListener("pageshow", h);
	window.dispatchEvent({ type: "pageshow" });`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `String(window.hits.length)`); got != "1" {
		t.Fatalf("listener re-registered after removal fired %s times, want 1", got)
	}
}

// qc:BR-204
func TestWindowDispatchEventDoesNotReachDocument(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body></body></html>`)
	if err := rt.Run(`
	var hits = [];
	document.addEventListener("changething", function () { hits.push("document"); });
	window.addEventListener("changething", function () { hits.push("window"); });
	window.dispatchEvent({ type: "changething" });
	window.changethingHits = hits.join(",");`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `window.changethingHits`); got != "window" {
		t.Fatalf("window.dispatchEvent reached %q, want %q", got, "window")
	}
}

// qc:BR-204
func TestBubblingEventReachesWindowUnlessPropagationStops(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body><div id="box"></div></body></html>`)
	src := `
	var seen = [];
	window.addEventListener("mybox", function () { seen.push("window"); });
	document.getElementById("box").addEventListener("mybox", function () { seen.push("box"); });
	document.addEventListener("mybox", function (e) {
		seen.push("document");
		if (window.stopIt) e.stopPropagation();
	});
	window.stopIt = false;
	document.getElementById("box").dispatchEvent({ type: "mybox" });
	var full = seen.join(",");
	seen = [];
	window.stopIt = true;
	document.getElementById("box").dispatchEvent({ type: "mybox" });
	var stopped = seen.join(",");`
	if err := rt.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `full`); got != "box,document,window" {
		t.Fatalf("bubbling path = %q, want %q", got, "box,document,window")
	}
	if got := evalString(t, rt, `stopped`); got != "box,document" {
		t.Fatalf("path after stopPropagation at the document = %q, want %q (window must not hear it)", got, "box,document")
	}
}

// qc:BR-204
func TestFireDocumentLifecycleReportsDomMutation(t *testing.T) {
	rt := lifecycleRuntime(t, `<html><head></head><body><div id="hidden" style="display:none">body</div></body></html>`)
	if err := rt.Run(`
	document.addEventListener("DOMContentLoaded", function () {
		document.getElementById("hidden").style.display = "block";
	});`, "inline"); err != nil {
		t.Fatal(err)
	}
	if !rt.FireDocumentLifecycle() {
		t.Fatalf("the lifecycle did not report the DOM write a ready handler made")
	}
	if got := evalString(t, rt, `document.getElementById("hidden").style.display`); got != "block" {
		t.Fatalf("display after the ready handler = %q, want %q", got, "block")
	}
	if rt.FireDocumentLifecycle() {
		t.Fatalf("a second lifecycle fire reported work; it must be once per document")
	}
}
