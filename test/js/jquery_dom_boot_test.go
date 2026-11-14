package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func bootRuntime(t *testing.T, html string) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(html)
	return newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
}

const bootDoc = `<!DOCTYPE html><html><head><title>T</title></head><body><p>text</p></body></html>`

// qc:BR-209
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
	if want := "true,true,true,1,1"; out != want {
		t.Errorf("jQuery support block: got %q want %q", out, want)
	}
}

// qc:BR-209
func TestJQueryBuildFragmentWrapAndUnwindPath(t *testing.T) {
	rt := bootRuntime(t, bootDoc)
	out := evalString(t, rt, `(function(){
		var t = document;
		var f = t.createDocumentFragment();
		var a = f.appendChild(t.createElement("div"));
		// ge.tr from jQuery's wrap map with HTML5 implied tbody
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
		"1",
		"null",
		"1",
		"0",
		"",
		"DIV",
	}, ",")
	if out != want {
		t.Errorf("buildFragment wrap/unwind: got %q want %q", out, want)
	}
}

// qc:BR-209
func TestJQuerySlicePushTrickOnDocumentChildNodes(t *testing.T) {
	rt := bootRuntime(t, bootDoc)
	out := evalString(t, rt, `(function(){
		var push = [].push, slice = [].slice, arr;
		push.apply((arr = slice.call(document.childNodes)), document.childNodes);
		return [String(document.childNodes.length), String(arr.length), String(arr[document.childNodes.length].nodeType), String(arr[0] === document.firstChild), String(arr.map(function(n){return n.nodeType}).join("/"))]
	})()`)
	if want := "2,4,10,true,10/1/10/1"; out != want {
		t.Errorf("slice/push on childNodes: got %q want %q", out, want)
	}
}

// qc:BR-209
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
	rt.FireDocumentLifecycle()
	if again := evalString(t, rt, `window.__log.join(",")`); again != want {
		t.Errorf("lifecycle fired twice: got %q want %q", again, want)
	}
}
