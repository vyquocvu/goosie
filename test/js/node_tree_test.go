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

func TestAppendChildWebIDLAndDocumentHierarchy(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"></div><p id="leaf">text</p></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box"), leaf = document.createTextNode("leaf");
	var nullError, leafError, documentError;
	try { box.appendChild(null); } catch (e) { nullError = e.name; }
	try { leaf.appendChild(null); } catch (e) { leafError = e.name; }
	var other = document.implementation.createHTMLDocument("other");
	try { document.body.appendChild(other); } catch (e) { documentError = e.name + "/" + e.code; }
	[nullError, leafError, documentError, other.parentNode === null].join("/");`)
	if out != "TypeError/TypeError/HierarchyRequestError/3/true" {
		t.Fatalf("appendChild WebIDL/hierarchy validation = %q", out)
	}
}

func TestRemoveChildUsesNotFoundDOMException(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="parent"><i></i></div></body></html>`)
	out := evalString(t, rt, `
	var parent = document.getElementById("parent");
	var detached = document.createElement("b");
	var notFound, leafNotFound, typeError;
	try { parent.removeChild(detached); } catch (e) {
		notFound = e.name + "/" + e.code + "/" + (e instanceof DOMException);
	}
	try { document.createTextNode("leaf").removeChild(document); } catch (e) {
		leafNotFound = e.name + "/" + e.code;
	}
	try { parent.removeChild(null); } catch (e) { typeError = e.name; }
	var child = parent.firstChild;
	var returned = parent.removeChild(child);
	[notFound, leafNotFound, typeError, returned === child, child.parentNode === null].join("/");`)
	want := "NotFoundError/8/true/NotFoundError/8/TypeError/true/true"
	if out != want {
		t.Fatalf("removeChild exceptions/result = %q, want %q", out, want)
	}
}

func TestNodeIsEqualNodeUsesAttributeLocalNamesAndDocumentTree(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var left = document.createElement('element');
	var right = document.createElement('element');
	left.setAttributeNS('urn:example', 'left:name', 'value');
	right.setAttributeNS('urn:example', 'right:name', 'value');
	var htmlDoc = document.implementation.createHTMLDocument();
	var doctype = document.implementation.createDocumentType('html', '', '');
	var constructed = document.implementation.createDocument('http://www.w3.org/1999/xhtml', 'html', doctype);
	constructed.documentElement.appendChild(constructed.createElement('head'));
	constructed.documentElement.appendChild(constructed.createElement('body'));
	[left.isEqualNode(right), constructed.isEqualNode(htmlDoc), htmlDoc.head.firstChild === null].join('|');`)
	if out != "true|true|true" {
		t.Fatalf("isEqualNode attribute/document comparisons = %q, want true|true|true", out)
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
	box.insertBefore(document.createTextNode("tail"), null);
	var names = [];
	for (var i = 0; i < box.childNodes.length; i++) names.push(box.childNodes[i].nodeName);
	names.join(",") + "/" + box.childNodes.length;`)
	if out != "SPAN,P,#text/3" {
		t.Fatalf("insertBefore(x, null) = %q, want %q", out, "SPAN,P,#text/3")
	}
}

func TestInsertBeforeRequiresSecondArgumentAndValidatesDocumentHierarchy(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box");
	var missingArgument, hierarchy;
	try { box.insertBefore(document.createTextNode("child")); }
	catch (e) { missingArgument = e.name; }
	var root = document.documentElement;
	try { document.insertBefore(document.createTextNode("invalid"), root); }
	catch (e) { hierarchy = e.name + "/" + e.code; }
	[Node.prototype.insertBefore.length, missingArgument, hierarchy, document.documentElement === root,
	 root.parentNode === document].join("/");`)
	if out != "2/TypeError/HierarchyRequestError/3/true/true" {
		t.Fatalf("insertBefore argument/hierarchy validation = %q", out)
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

func TestElementAfterInsertsNodesAndStringsInOrder(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"><b>before</b><i id="target">target</i><u>after</u></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box"), target = document.getElementById("target");
	var em = document.createElement("em"), fragment = document.createDocumentFragment();
	fragment.appendChild(document.createElement("strong"));
	target.after(em, "middle", fragment);
	var names = [];
	for (var i = 0; i < box.childNodes.length; i++) names.push(box.childNodes[i].nodeName);
	[names.join("|"), target.nextSibling === em, em.nextSibling.nodeType,
	 fragment.childNodes.length, em.parentNode === box].join("/");`)
	if out != "B|I|EM|#text|STRONG|U/true/3/0/true" {
		t.Fatalf("Element.after() = %q, want %q", out, "B|I|EM|#text|STRONG|U/true/3/0/true")
	}
}

func TestChildNodeAfterKeepsReceiverWhenPassedAmongArguments(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box");
	var element = document.createElement("x");
	var comment = document.createComment("test");
	var text = document.createTextNode("test");
	box.append(comment, element, text);
	comment.after("text", comment);
	element.after(element, comment);
	text.after(text);
	[box.innerHTML, comment.parentNode === box, element.parentNode === box,
	 text.parentNode === box].join("/");`)
	want := `text<x></x><!--test-->test/true/true/true`
	if out != want {
		t.Fatalf("ChildNode.after() with receiver in arguments = %q, want %q", out, want)
	}
}

func TestChildNodeBeforeAndReplaceWithMutateInOrder(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"><b>first</b><i id="target">target</i><u>last</u></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box"), target = document.getElementById("target");
	var em = document.createElement("em"), fragment = document.createDocumentFragment();
	fragment.appendChild(document.createElement("strong"));
	target.before(em, "middle", fragment);
	var before = [];
	for (var i = 0; i < box.childNodes.length; i++) before.push(box.childNodes[i].nodeName);
	target.replaceWith("replacement", document.createElement("s"));
	var after = [];
	for (var i = 0; i < box.childNodes.length; i++) after.push(box.childNodes[i].nodeName);
	[before.join("|"), after.join("|"), fragment.childNodes.length, target.parentNode === null,
	 typeof document.createTextNode("text").before, typeof document.createTextNode("text").replaceWith].join("/");`)
	want := "B|EM|#text|STRONG|I|U/B|EM|#text|STRONG|#text|S|U/0/true/function/function"
	if out != want {
		t.Fatalf("ChildNode mutations = %q, want %q", out, want)
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
func TestChildNodeReplaceWithNoArgumentsRemovesReceiver(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box");
	var element = document.createElement("b");
	var comment = document.createComment("gone");
	var text = document.createTextNode("gone");
	box.append(element, comment, text);
	element.replaceWith();
	comment.replaceWith();
	text.replaceWith();
	[box.childNodes.length, element.parentNode === null,
	 comment.parentNode === null, text.parentNode === null].join("/");`)
	if out != "0/true/true/true" {
		t.Fatalf("replaceWith() without arguments = %q, want %q", out, "0/true/true/true")
	}
}

func TestChildNodeReplaceWithKeepsReceiverWhenPassedAsArgument(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"><i id="target"></i><x></x>text</div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box"), target = document.getElementById("target");
	var sibling = box.childNodes[1];
	target.replaceWith(sibling, target);
	[box.innerHTML, target.parentNode === box, sibling.nextSibling === target].join("/");`)
	if out != `<x></x><i id="target"></i>text/true/true` {
		t.Fatalf("replaceWith(sibling, receiver) = %q, want %q", out, `<x></x><i id="target"></i>text/true/true`)
	}
}

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

func TestCloneAttributeNodePreservesValueAndBrand(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var attr = document.createAttribute("class");
	attr.value = "original";
	var clone = attr.cloneNode();
	attr.value = "changed";
	[clone !== attr, clone instanceof Attr, clone instanceof Node,
	 clone.nodeType, clone.name, clone.value, clone.ownerElement === null,
	 clone.ownerDocument === document].join("/");`)
	want := "true/true/true/2/class/original/true/true"
	if out != want {
		t.Fatalf("Attr.cloneNode() = %q, want %q", out, want)
	}
}

func TestAdoptionUpdatesAttributeOwnerDocumentAndPreservesIdentity(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var source = document.implementation.createHTMLDocument("source");
	var adopted = source.createElement("div");
	adopted.setAttribute("data-a", "1");
	var attr = adopted.getAttributeNode("data-a");
	var sourceOwner = attr.ownerDocument === source;
	document.adoptNode(adopted);
	var explicitAdoption = attr.ownerDocument === document && adopted.getAttributeNode("data-a") === attr;
	var secondSource = document.implementation.createHTMLDocument("second");
	var inserted = secondSource.createElement("p");
	var insertedAttr = secondSource.createAttribute("data-b");
	insertedAttr.value = "2";
	inserted.setAttributeNode(insertedAttr);
	document.body.appendChild(inserted);
	[sourceOwner, explicitAdoption, insertedAttr.ownerDocument === document,
	 inserted.getAttributeNode("data-b") === insertedAttr].join("/");`)
	want := "true/true/true/true"
	if out != want {
		t.Fatalf("Attr adoption = %q, want %q", out, want)
	}
}

func TestCloneNodePreservesDocumentTypeAndDocumentURL(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var doctype = document.implementation.createDocumentType("html", "public", "system");
	var doctypeClone = doctype.cloneNode();
	var xml = document.implementation.createDocument(null, null);
	var xmlClone = xml.cloneNode();
	[doctypeClone.name, doctypeClone.publicId, doctypeClone.systemId,
	 xmlClone instanceof Document, xmlClone.URL === xml.URL].join("/");`)
	want := "html/public/system/true/true"
	if out != want {
		t.Fatalf("DocumentType/Document clone metadata = %q, want %q", out, want)
	}
}

func TestCloneNodeUsesSharedHTMLInterfacePrototypes(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var cases = [
		["col", "HTMLTableColElement"], ["colgroup", "HTMLTableColElement"],
		["del", "HTMLModElement"], ["ins", "HTMLModElement"],
		["h1", "HTMLHeadingElement"], ["h2", "HTMLHeadingElement"],
		["h3", "HTMLHeadingElement"], ["h4", "HTMLHeadingElement"],
		["h5", "HTMLHeadingElement"], ["h6", "HTMLHeadingElement"],
		["td", "HTMLTableCellElement"], ["th", "HTMLTableCellElement"],
		["dir", "HTMLDirectoryElement"], ["font", "HTMLFontElement"]
	];
	var failure = "";
	for (var i = 0; i < cases.length; i++) {
		var element = document.createElement(cases[i][0]);
		var clone = element.cloneNode();
		var ctor = window[cases[i][1]];
		if (typeof ctor !== "function" || !(element instanceof ctor) || !(clone instanceof ctor)) failure = "bad:" + cases[i][0];
		if (Object.getPrototypeOf(element) !== Object.getPrototypeOf(clone)) failure = "clone-prototype:" + cases[i][0];
	}
	var shared = Object.getPrototypeOf(document.createElement("h1")) === Object.getPrototypeOf(document.createElement("h6")) &&
		Object.getPrototypeOf(document.createElement("td")) === Object.getPrototypeOf(document.createElement("th"));
	var dir = document.createElement("dir"), font = document.createElement("font");
	dir.compact = true;
	font.color = "red"; font.face = "serif"; font.size = "4";
	[failure, shared, dir.hasAttribute("compact"), font.getAttribute("color"), font.getAttribute("face"), font.getAttribute("size")].join("/");`)
	want := "/true/true/red/serif/4"
	if out != want {
		t.Fatalf("HTML interface prototypes/clones = %q, want %q", out, want)
	}
}

func TestNodeNormalizeDoesNotTreatEmptyNonTextNodesAsText(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var doc = new DOMParser().parseFromString("<div/>", "text/xml");
	var div = doc.documentElement;
	var t1 = div.appendChild(doc.createTextNode("a"));
	var pi = div.appendChild(doc.createProcessingInstruction("pi", ""));
	var t2 = div.appendChild(doc.createTextNode("b"));
	var cdata = div.appendChild(doc.createCDATASection(""));
	var t3 = div.appendChild(doc.createTextNode("c"));
	var comment = div.appendChild(doc.createComment(""));
	var t4 = div.appendChild(doc.createTextNode("d"));
	var element = div.appendChild(doc.createElement("el"));
	var t5 = div.appendChild(doc.createTextNode("e"));
	var before = div.childNodes.length;
	div.normalize();
	[before, div.childNodes.length, div.childNodes[1] === pi,
	 div.childNodes[3] === cdata, div.childNodes[5] === comment,
	 div.childNodes[7] === element].join("/");`)
	want := "9/9/true/true/true/true"
	if out != want {
		t.Fatalf("normalize around non-text children = %q, want %q", out, want)
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

func TestReplaceChildUsesDOMExceptionForMissingOldChild(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="parent"></div></body></html>`)
	out := evalString(t, rt, `
	var parent = document.getElementById("parent");
	var outside = document.createElement("i");
	var error;
	try { parent.replaceChild(document.createElement("b"), outside); }
	catch (e) { error = e; }
	[error.name, error.code, error instanceof DOMException].join("/");`)
	if out != "NotFoundError/8/true" {
		t.Fatalf("replaceChild missing child error = %q, want %q", out, "NotFoundError/8/true")
	}
}

func TestReplaceChildChecksAncestorAndDocumentHierarchyAtomically(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="parent"><i></i></div></body></html>`)
	out := evalString(t, rt, `
	var parent = document.getElementById("parent");
	var missing = document.createElement("u");
	var ancestorError;
	try { parent.replaceChild(parent, missing); } catch (e) { ancestorError = e.name + "/" + e.code; }
	var root = document.documentElement;
	var hierarchyError;
	try { document.replaceChild(document.createTextNode("invalid"), root); }
	catch (e) { hierarchyError = e.name + "/" + e.code; }
	[ancestorError, hierarchyError, document.documentElement === root,
	 root.parentNode === document].join("/");`)
	if out != "HierarchyRequestError/3/HierarchyRequestError/3/true/true" {
		t.Fatalf("replaceChild validation = %q", out)
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

func TestElementSiblingAccessorsSkipNonElementsAndStayLive(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"><!-- before --><details id="first"></details>text<span id="last"></span></div></body></html>`)
	out := evalString(t, rt, `
	var first = document.getElementById("first"), last = document.getElementById("last");
	var initial = [first.previousElementSibling, first.nextElementSibling === last, last.previousElementSibling === first, last.nextElementSibling].join(",");
	var inserted = document.createElement("p");
	first.parentNode.insertBefore(inserted, last);
	[initial, first.nextElementSibling === inserted, inserted.previousElementSibling === first, inserted.nextElementSibling === last].join("|");`)
	if out != ",true,true,|true|true|true" {
		t.Fatalf("element sibling accessors = %q, want %q", out, ",true,true,|true|true|true")
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

func TestInnerAndOuterHTMLConvertNullToEmpty(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="parent"><p id="child">old</p></div></body></html>`)
	out := evalString(t, rt, `
	var p = document.createElement("p");
	p.innerHTML = null;
	var child = document.getElementById("child");
	child.outerHTML = null;
	[p.innerHTML, p.textContent, document.getElementById("parent").innerHTML,
	 document.getElementById("parent").textContent].join("|");`)
	if out != "|||" {
		t.Fatalf("innerHTML/outerHTML null conversion = %q, want empty values", out)
	}
}

func TestQuerySelectorFindsDescendantsOfDisconnectedOwnedElements(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var scratch = document.createElement("div");
	scratch.innerHTML = "<ul><li>a<li>b</ul>";
	var list = scratch.querySelector("ul");
	[list !== null, list && list.children.length,
	 list && list.children[0].textContent, list && list.children[1].textContent].join("|");`)
	if out != "true|2|a|b" {
		t.Fatalf("detached subtree query = %q, want true|2|a|b", out)
	}
}

func TestOuterHTMLRejectsDocumentChildAndReplacesUnderFragment(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var rootError = "";
	try { document.documentElement.outerHTML = "<html></html>"; }
	catch (error) { rootError = error.name; }
	var fragment = document.createDocumentFragment();
	var old = document.createElement("div");
	fragment.appendChild(old);
	old.outerHTML = "<body><strong>new</strong></body>";
	[rootError, fragment.firstChild.localName, fragment.firstChild.textContent,
	 old.outerHTML].join("|");`)
	if out != "NoModificationAllowedError|strong|new|<div></div>" {
		t.Fatalf("outerHTML replacement behavior = %q", out)
	}
}

func TestEmptyInnerHTMLOnHTMLElementKeepsBodyChild(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var root = document.createElement("html");
	root.innerHTML = "<head><title>foo</title></head><body></body>";
	root.innerHTML = "";
	root.getElementsByTagName("body")[0] instanceof HTMLBodyElement;`)
	if out != "true" {
		t.Fatalf("empty html.innerHTML body test = %q, want true", out)
	}
}
