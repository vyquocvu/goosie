package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// The Node child-list API: createDocumentFragment, insertBefore, appendChild of
// a fragment, replaceChild, cloneNode, and the tree reads.
//
// These are not "APIs that look standard". Each one is pinned to a call a
// measured page makes. On the saved vnexpress home page (201,234-byte copy) the
// inline boot has 9 `parentNode.insertBefore(...)` sites and the pre-change
// engine threw `Object has no member 'insertBefore'` at 2 of them; none passes a
// literal null, but two pass `x.nextSibling` (offsets 113,261, 195,744), which is
// null when the reference is the last child. jQuery makes the append form
// unconditional: `prepend` passes `t.firstChild` (50,065), `after` passes
// `this.nextSibling` (50,297) and `wrapAll` omits the argument (80,979). On
// www.gov.vn the release sweep's own row quotes the library's first DOM
// statement - `E.createDocumentFragment().appendChild(E.createElement("div"))`,
// offset 37,880 of the 89,501-byte jquery-3.6.0.min.js - as
// `.../scripts/jquery-3.6.0.min.js?v=1:1: TypeError: Object has no member
// 'createDocumentFragment'`, 21 uncaught errors, doc_height 0. The assertions
// below are the shapes those call patterns need, and every one of them fails
// when the mechanism is deleted (the mutation table in
// docs/tasks/TASK-047-DOM-JQUERY-APIS.md carries the captured output).

func nodeTreeRuntime(t *testing.T, html string) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(html)
	return newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
}

// A document fragment is a Node: nodeType 11, it takes children, and
// appendChild hands the child back. jQuery's whole boot starts on that last
// detail - `var ce = E.createDocumentFragment().appendChild(E.createElement("div"))`
// - so an appendChild that returned undefined would leave `ce` undefined and
// the next statement would throw inside the library's own setup.
// qc:BR-203
func TestCreateDocumentFragmentIsANodeThatTakesChildren(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><p>hi</p></body></html>`)
	out := evalString(t, rt, `
	var frag = document.createDocumentFragment();
	var div = document.createElement("div");
	var returned = frag.appendChild(div);
	[
		frag.nodeType,
		String(frag.nodeName),
		frag.childNodes.length,
		returned === div,
		div.parentNode === frag,
		frag.firstChild === div
	].join(",");`)
	if out != "11,#document-fragment,1,true,true,true" {
		t.Fatalf("fragment = %q, want %q", out, "11,#document-fragment,1,true,true,true")
	}
}

// Appending a fragment moves its children and empties it. This is the step that
// makes jQuery's buildFragment usable: it fills one fragment, hands it to
// appendChild/insertBefore, and the fragment must then be reusable and empty.
// The alternative - inserting the fragment node itself - would put a
// #document-fragment into the body, where no selector can reach it and paint
// cannot see any of its content.
// qc:BR-203
func TestAppendChildMovesFragmentChildrenAndEmptiesFragment(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box");
	var frag = document.createDocumentFragment();
	frag.appendChild(document.createElement("b"));
	frag.appendChild(document.createTextNode("mid"));
	frag.appendChild(document.createElement("i"));
	box.appendChild(frag);
	var names = [];
	for (var i = 0; i < box.childNodes.length; i++) names.push(box.childNodes[i].nodeName);
	[box.childNodes.length, names.join("|"), frag.childNodes.length, box.innerHTML].join(",");`)
	if out != "3,B|#text|I,0,<b></b>mid<i></i>" {
		t.Fatalf("fragment insert = %q, want %q", out, "3,B|#text|I,0,<b></b>mid<i></i>")
	}
}

// `insertBefore(node, null)` appends. Both vnexpress call sites pass the second
// argument as a variable that is null on a fresh page, so a binding that read
// null as "no reference given, do nothing" would silently drop the injected
// markup instead of appending it.
// qc:BR-203
func TestInsertBeforeWithNullReferenceAppends(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"><span>a</span></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box");
	box.insertBefore(document.createElement("p"), null);
	box.insertBefore(document.createTextNode("tail"));
	var names = [];
	for (var i = 0; i < box.childNodes.length; i++) names.push(box.childNodes[i].nodeName);
	names.join(",") + "/" + box.childNodes.length;`)
	if out != "SPAN,P,#text/3" {
		t.Fatalf("insertBefore(x, null) = %q, want %q", out, "SPAN,P,#text/3")
	}
}

// insertBefore positions the node before the reference, and a reference that is
// not a child of this node throws NotFoundError like a browser does. The throw
// matters: silently appending would put the node at the end of the wrong parent
// and the page would render content in the wrong order with no signal at all.
// qc:BR-203
func TestInsertBeforePositionsAndRejectsForeignReference(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="a"><b></b><i></i></div><div id="b"><em></em></div></body></html>`)
	out := evalString(t, rt, `
	var a = document.getElementById("a"), b = document.getElementById("b");
	var ref = a.lastChild;
	a.insertBefore(document.createElement("u"), ref);
	var names = [];
	for (var i = 0; i < a.childNodes.length; i++) names.push(a.childNodes[i].nodeName);
	var err = "none";
	try { a.insertBefore(document.createElement("s"), b.firstChild); }
	catch (e) { err = e.name || "TypeError"; }
	names.join("|") + "/" + err + "/" + a.childNodes.length;`)
	if out != "B|U|I/NotFoundError/3" {
		t.Fatalf("insertBefore = %q, want %q", out, "B|U|I/NotFoundError/3")
	}
}

// A fragment argument to insertBefore contributes its children, in order, at
// the reference position. jQuery's `before`/`after` helpers call
// `target.insertBefore(node, target.firstChild)` with whatever buildFragment
// produced.
// qc:BR-203
func TestInsertBeforeMovesFragmentChildrenToPosition(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"><span>keep</span></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box");
	var frag = document.createDocumentFragment();
	frag.appendChild(document.createElement("b"));
	frag.appendChild(document.createElement("i"));
	box.insertBefore(frag, box.firstChild);
	var names = [];
	for (var i = 0; i < box.childNodes.length; i++) names.push(box.childNodes[i].nodeName);
	names.join("|") + "/" + frag.childNodes.length;`)
	if out != "B|I|SPAN/0" {
		t.Fatalf("fragment insertBefore = %q, want %q", out, "B|I|SPAN/0")
	}
}

// cloneNode(false) copies the node alone; cloneNode(true) copies the subtree.
// Both are needed by the same jQuery support test, run twice in a row:
//
//	ce.cloneNode(!0).cloneNode(!0).lastChild.checked
//
// - so a deep clone must carry children, and the clone's own children must be
// clones rather than the same nodes moved out of the original (which would
// empty `ce` and break every later statement that reads it).
// qc:BR-203
func TestCloneNodeShallowAndDeep(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="src" class="c"><b>x</b><i>y</i></div></body></html>`)
	out := evalString(t, rt, `
	var src = document.getElementById("src");
	var shallow = src.cloneNode(false);
	var deep = src.cloneNode(true);
	[
		shallow.childNodes.length,
		deep.childNodes.length,
		shallow.nodeName,
		shallow.getAttribute("class"),
		deep.firstChild === src.firstChild,
		src.childNodes.length,
		deep.ownerDocument === document
	].join(",");`)
	if out != "0,2,DIV,c,false,2,true" {
		t.Fatalf("cloneNode = %q, want %q", out, "0,2,DIV,c,false,2,true")
	}
}

// The exact clone chain jQuery 3.6 runs while it decides whether this engine
// can be trusted with a form control: a deep clone of a deep clone, then a read
// of the last child's `checked`. If any link in that chain is missing, the
// library's checkClone flag comes out false and it takes a different code path
// for every node it ever clones.
// qc:BR-203
func TestClonedFormControlKeepsCheckedState(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var ce = document.createDocumentFragment().appendChild(document.createElement("div"));
	var fe = document.createElement("input");
	fe.setAttribute("type", "radio");
	fe.setAttribute("checked", "checked");
	fe.setAttribute("name", "t");
	ce.appendChild(fe);
	String(ce.cloneNode(!0).cloneNode(!0).lastChild.checked);`)
	if out != "true" {
		t.Fatalf("checkClone chain = %q, want %q", out, "true")
	}
}

// replaceChild puts the new node exactly where the old one was and detaches the
// old one. jQuery's replaceWith is `t.replaceChild(e, this)`.
// qc:BR-203
func TestReplaceChildSwapsInPlace(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"><b>1</b><i>2</i></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box");
	var old = box.firstChild;
	var returned = box.replaceChild(document.createElement("u"), old);
	var names = [];
	for (var i = 0; i < box.childNodes.length; i++) names.push(box.childNodes[i].nodeName);
	names.join("|") + "/" + (returned === old) + "/" + old.parentNode + "/" + box.childNodes.length;`)
	if out != "U|I/true/null/2" {
		t.Fatalf("replaceChild = %q, want %q", out, "U|I/true/null/2")
	}
}

// childNodes is every node (text and comments included) while children stays
// element-only, and the position reads agree with the tree. jQuery reads
// childNodes 11 times and firstChild 12, and its `.text()` walks nodeValue.
// qc:BR-203
func TestChildListReadsCoverEveryNodeType(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box">a<b>B</b><!--c-->d</div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box");
	var last = box.lastChild;
	[
		box.childNodes.length,
		box.children.length,
		box.firstChild.nodeValue,
		box.lastChild === last,
		last.previousSibling.nodeName,
		box.firstChild.nextSibling.nodeName,
		box.hasChildNodes(),
		box.contains(last),
		box.contains(box),
		box.contains(document.head)
	].join(",");`)
	if out != "4,1,a,true,#comment,B,true,true,true,false" {
		t.Fatalf("child list = %q, want %q", out, "4,1,a,true,#comment,B,true,true,true,false")
	}
}

// ownerDocument and parentNode are the document object itself, not look-alike
// wrappers. Sizzle's setDocument compares `r != C` against window.document and
// re-binds its internal document cache to whatever comes back, so a second
// wrapper for the same node would be adopted as "the document" and then fail on
// the first property only the real one has. That is an identity check, which no
// amount of extra properties on the wrapper can satisfy.
// qc:BR-203
func TestOwnerDocumentAndParentNodeAreTheDocumentObject(t *testing.T) {
	rt := nodeTreeRuntime(t, `<!doctype html><html><head></head><body><p id="p">t</p></body></html>`)
	out := evalString(t, rt, `
	var p = document.getElementById("p");
	[
		p.ownerDocument === document,
		document.documentElement.parentNode === document,
		String(document.documentElement.parentElement),
		document.documentElement.ownerDocument === document,
		p.parentNode === document.body,
		String(document.ownerDocument),
		document.nodeType,
		p.firstChild.ownerDocument === document
	].join(",");`)
	if out != "true,true,null,true,true,null,9,true" {
		t.Fatalf("document identity = %q, want %q", out, "true,true,null,true,true,null,9,true")
	}
}

// isConnected separates a node still in the document from one a script removed,
// which is how a page decides whether to measure a box at all.
// qc:BR-203
func TestIsConnectedFollowsRemoval(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"><span>x</span></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box"), kid = box.firstChild;
	var made = document.createElement("p");
	[kid.isConnected, made.isConnected].join(",");`)
	if out != "true,false" {
		t.Fatalf("isConnected before removal = %q, want %q", out, "true,false")
	}
	after := evalString(t, rt, `
	document.getElementById("box").removeChild(document.getElementById("box").firstChild);
	document.getElementById("box").firstChild;`)
	if after != "null" {
		t.Fatalf("box still has a child after removeChild: %q", after)
	}
	gone := evalString(t, rt, `String(kid.isConnected)`)
	if strings.TrimSpace(gone) != "false" {
		t.Fatalf("isConnected after removal = %q, want %q", gone, "false")
	}
}

// nodeValue is the leaf data for text and comment nodes and null for the types
// that have none, and writing it only changes the leaf types. jQuery's `.text()`
// is a nodeValue read on every text node it walks.
// qc:BR-203
func TestNodeValueIsOnlyForLeaves(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box">keep</div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box"), t = box.firstChild;
	t.nodeValue = "changed";
	[String(box.nodeValue), t.nodeValue, box.textContent].join("|");`)
	if out != "null|changed|changed" {
		t.Fatalf("nodeValue = %q, want %q", out, "null|changed|changed")
	}
}
