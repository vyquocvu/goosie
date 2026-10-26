package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// The library-shaped boot: jQuery 3.6.0's own statements, run against this
// engine, with the support flags it derives from them pinned.
//
// Why this file exists separately from the per-API tests: the per-API clusters
// pin one member each, and a page does not call members, it calls a library that
// calls them and then branches on what it read. A missing member throws and is
// visible; a member that answers the wrong *shape* flips a flag and the library
// silently takes a different path. These cases run the minified statements verbatim
// (from the real jquery-3.6.0.min.js, 89,501 bytes; byte offsets in the comments)
// with the single-letter names restored to the objects they hold, and assert the
// flags jQuery stores.
//
// Where a flag cannot match a browser because of something this engine does not
// model, that is asserted as the divergence it is, with the direction the library
// then takes, instead of quietly wanting the browser's answer.

func bootRuntime(t *testing.T, html string) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(html)
	return newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
}

const bootDoc = `<!DOCTYPE html><html><head><title>T</title></head><body><p>text</p></body></html>`

// jQuery's attributes/forms support block, verbatim (min offsets 37880..38320):
//
//	ce = E.createDocumentFragment().appendChild(E.createElement("div")),
//	(fe = E.createElement("input")).setAttribute("type", "radio"),
//	fe.setAttribute("checked", "checked"),
//	fe.setAttribute("name", "t"),
//	ce.appendChild(fe),
//	y.checkClone = ce.cloneNode(!0).cloneNode(!0).lastChild.checked,
//	ce.innerHTML = "<textarea>x</textarea>",
//	y.noCloneChecked = !!ce.cloneNode(!0).lastChild.defaultValue,
//	ce.innerHTML = "<option></option>",
//	y.option = !!ce.lastChild
//
// Every link is an API in this task: createDocumentFragment, appendChild's return
// value, setAttribute, cloneNode(true), lastChild, innerHTML. If any is missing
// the block throws and `jQuery` never finishes installing - which is exactly the
// failure www.gov.vn shows today, where the shipped bundle's first statement is
// `E.createDocumentFragment().appendChild(E.createElement("div"))`.
func TestJQuerySupportBlockRunsAndSetsItsFlags(t *testing.T) {
	rt := bootRuntime(t, bootDoc)
	out := evalString(t, rt, `(function(){
		var E = document, y = {}, ce, fe;
		ce = E.createDocumentFragment().appendChild(E.createElement("div"));
		(fe = E.createElement("input")).setAttribute("type", "radio");
		fe.setAttribute("checked", "checked");
		fe.setAttribute("name", "t");
		ce.appendChild(fe);
		y.checkClone = ce.cloneNode(!0).cloneNode(!0).lastChild.checked;
		ce.innerHTML = "<textarea>x</textarea>";
		y.noCloneChecked = !!ce.cloneNode(!0).lastChild.defaultValue;
		ce.innerHTML = "<option></option>";
		y.option = !!ce.lastChild;
		return [String(y.checkClone), String(y.noCloneChecked), String(y.option), String(ce.childNodes.length), String(ce.firstChild.nodeType)]
	})()`)
	// checkClone true is the browser answer and the one worth having: a deep
	// clone of a deep clone keeps a radio input's checked state.
	//
	// noCloneChecked reads false here, not true as in a browser, because this
	// engine has no `defaultValue` reflection on form controls (nothing in the
	// five saved pages or in jQuery's boot path reads it beyond this test). The
	// consequence is one-directional and safe: with the flag false jQuery keeps
	// its fallback and re-assigns values after every clone, so cloning behaves as
	// it does on the browsers that fail the same probe. Pinned so a future
	// defaultValue implementation shows up as a flip here rather than as an
	// unexplained difference from a browser run.
	//
	// option true is the "can a detached <option> survive innerHTML" probe: it
	// must be an element, not text.
	if want := "true,false,true,1,1"; out != want {
		t.Errorf("jQuery support block: got %q want %q", out, want)
	}
}

// jQuery's buildFragment (`xe`, min offsets 39029..39560) is what every DOM
// insertion in the library goes through. The block that decides whether the
// engine can be trusted with a fragment is:
//
//	f = t.createDocumentFragment(), ..., a = a || f.appendChild(t.createElement("div"))
//	a.innerHTML = u[1] + S.htmlPrefilter(o) + u[2], c = u[0]
//	while (c--) a = a.lastChild
//	S.merge(p, a.childNodes), (a = f.firstChild).textContent = ""
//
// with `ge` the wrap map (table rows are wrapped in <table><tbody> and unwound
// by walking lastChild `u[0]` times). The assertions are the three things that
// make it work: appendChild returns the created div, lastChild walks down the
// wrapper chain to the row, and textContent = "" empties without removing.
func TestJQueryBuildFragmentWrapAndUnwindPath(t *testing.T) {
	rt := bootRuntime(t, bootDoc)
	out := evalString(t, rt, `(function(){
		var t = document;
		var f = t.createDocumentFragment();
		var a = f.appendChild(t.createElement("div"));
		// ge.tr from jQuery's wrap map: [1, "<table>", "</table>"]
		var wrap = [1, "<table>", "</table>"];
		var html = "<tr id='row'><td>cell</td></tr>";
		a.innerHTML = wrap[1] + html + wrap[2];
		var c = wrap[0];
		while (c--) a = a.lastChild;
		var p = [];
		for (var i = 0; i < a.childNodes.length; i++) p.push(a.childNodes[i]);
		var moved = p.length;
		(a = f.firstChild).textContent = "";
		return [String(moved), String(p[0].getAttribute("id")), String(f.childNodes.length), String(f.firstChild.childNodes.length), String(f.firstChild.textContent), String(a.tagName)]
	})()`)
	want := strings.Join([]string{
		"1",   // the <tr> was found by walking lastChild
		"row", // and it is the real row element, not a text blob
		"1",   // the fragment still holds exactly the div
		"0",   // textContent="" emptied the div without removing it
		"",    // and reads back empty
		"DIV", // a was re-pointed at f.firstChild
	}, ",")
	if out != want {
		t.Errorf("buildFragment wrap/unwind: got %q want %q", out, want)
	}
}

// Array-like childNodes, in the form jQuery's own support trick requires (min
// offsets 6083..6260):
//
//	try {
//	  push.apply((arr = slice.call(document.childNodes)), document.childNodes);
//	  arr[document.childNodes.length].nodeType;
//	} catch (e) { ...the slow makeArray fallback... }
//
// The trick has to *succeed* here: the catch installs jQuery's hand-rolled
// makeArray, which is slower but not wrong, so what is actually being pinned is
// that childNodes is an indexed, length-carrying sequence of node objects - the
// same contract the Sizzle context tests rely on.
func TestJQuerySlicePushTrickOnDocumentChildNodes(t *testing.T) {
	rt := bootRuntime(t, bootDoc)
	out := evalString(t, rt, `(function(){
		var push = [].push, slice = [].slice, arr;
		push.apply((arr = slice.call(document.childNodes)), document.childNodes);
		return [String(document.childNodes.length), String(arr.length), String(arr[document.childNodes.length].nodeType), String(arr[0] === document.firstChild), String(arr.map(function(n){return n.nodeType}).join("/"))]
	})()`)
	// The live document's children are [html, doctype] in this parser's order
	// (see TestCreateHTMLDocumentBuildsTheStandardSkeleton), so after the double
	// push the array is 4 entries long and the read jQuery makes at
	// `document.childNodes.length` is the third node - in range, and a node with a
	// nodeType. That is the whole point: the trick must not throw, or jQuery keeps
	// its hand-rolled makeArray for every collection it flattens.
	if want := "2,4,1,true,1/10/1/10"; out != want {
		t.Errorf("slice/push on childNodes: got %q want %q", out, want)
	}
}

// The ready branch, verbatim (min offset 32190):
//
//	"complete" === E.readyState || "loading" !== E.readyState && !E.documentElement.doScroll
//	  ? C.setTimeout(S.ready)
//	  : (E.addEventListener("DOMContentLoaded", B), C.addEventListener("load", B))
//
// Two things are pinned. First, the branch a page takes at script time: this
// engine reports readyState "loading" while scripts run and
// `documentElement.doScroll` is absent, so the condition is false and jQuery
// registers listeners - the browser answer for a non-IE engine. A stubbed
// readyState of "complete" would instead take the setTimeout path, which in a
// host that never pumps timers (cmd/goosie-headless) means ready callbacks that
// never run.
//
// Second, both registrations have to be real: `document.addEventListener` and
// `window.addEventListener`, the latter being the member that did not exist at
// all before this task, so the branch threw on its second call and lost the
// DOMContentLoaded handler with it.
func TestJQueryReadyBranchPicksTheListenerPathAndBothRegistrationsStick(t *testing.T) {
	rt := bootRuntime(t, bootDoc)
	branch := evalString(t, rt, `(function(){
		var E = document, C = window;
		var took = "complete" === E.readyState || "loading" !== E.readyState && !E.documentElement.doScroll ? "timeout" : "listeners";
		var registered = [];
		try {
			E.addEventListener("DOMContentLoaded", function(){});
			registered.push("dom");
		} catch (e) { registered.push("dom:" + e.message) }
		try {
			C.addEventListener("load", function(){});
			registered.push("win");
		} catch (e) { registered.push("win:" + e.message) }
		return [took, registered.join("+"), E.readyState, String(typeof E.documentElement.doScroll), String(window === document.defaultView)].join("|")
	})()`)
	if want := "listeners|dom+win|loading|undefined|true"; branch != want {
		t.Errorf("jQuery ready branch: got %q want %q", branch, want)
	}
	// Registering is inert - nothing dispatches until the document lifecycle
	// fires - and then the handlers jQuery hung up are reached in the standard's
	// order, each seeing the readyState a browser would report at that moment.
	evalString(t, rt, `window.__log = [];
		document.addEventListener("DOMContentLoaded", function(){ window.__log.push("doc:" + document.readyState) });
		window.addEventListener("load", function(){ window.__log.push("win:" + document.readyState) });
		String(window.__log.length)`)
	if before := evalString(t, rt, `window.__log.join(",")`); before != "" {
		t.Fatalf("adding a listener dispatched something: got %q", before)
	}
	if !rt.FireDocumentLifecycle() {
		t.Logf("FireDocumentLifecycle reported no DOM mutation; that is the expected case here, the handlers only record state")
	}
	after := evalString(t, rt, `window.__log.join(",")`)
	want := "doc:interactive,win:complete"
	if after != want {
		t.Errorf("ready handlers: got %q want %q", after, want)
	}
	// The lifecycle is once per document: the session drives it from
	// engine.NewSession, and a second call must not run the handlers again.
	rt.FireDocumentLifecycle()
	if again := evalString(t, rt, `window.__log.join(",")`); again != want {
		t.Errorf("lifecycle fired twice: got %q want %q", again, want)
	}
}
