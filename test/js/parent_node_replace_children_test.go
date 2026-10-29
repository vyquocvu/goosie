package js_test

import "testing"

func TestParentNodeReplaceChildrenConvertsAndMovesNodes(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"><b>old</b></div><div id="source"><i>I</i></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box");
	var old = box.firstChild;
	var source = document.getElementById("source");
	var moved = source.firstChild;
	var fragment = document.createDocumentFragment();
	fragment.appendChild(document.createElement("u"));
	fragment.appendChild(document.createTextNode("tail"));
	var result = box.replaceChildren("start", moved, fragment, 7);
	var names = [];
	for (var i = 0; i < box.childNodes.length; i++) names.push(box.childNodes[i].nodeName);
	[
		result === undefined,
		names.join("|"),
		box.textContent,
		old.parentNode === null,
		moved.parentNode === box,
		source.childNodes.length,
		fragment.childNodes.length,
		box.replaceChildren() === undefined,
		box.childNodes.length
	].join("/");`)
	if out != "true/#text|I|U|#text|#text/startItail7/true/true/0/0/true/0" {
		t.Fatalf("replaceChildren = %q", out)
	}
}

func TestParentNodeReplaceChildrenValidationIsAtomic(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body><div id="box"><b>keep</b></div></body></html>`)
	out := evalString(t, rt, `
	var box = document.getElementById("box");
	var cycle = "none";
	var documentChildren = "none";
	try { box.replaceChildren(box); } catch (error) { cycle = error.name; }
	try { document.replaceChildren(document.createElement("html"), document.createElement("html")); }
	catch (error) { documentChildren = error.name; }
	[
		cycle,
		box.innerHTML,
		documentChildren,
		document.documentElement.localName,
		document.body !== null
	].join("/");`)
	if out != "HierarchyRequestError/<b>keep</b>/HierarchyRequestError/html/true" {
		t.Fatalf("invalid replaceChildren = %q", out)
	}
}

func TestParentNodeReplaceChildrenAvailableOnDocumentFragment(t *testing.T) {
	rt := nodeTreeRuntime(t, `<html><head></head><body></body></html>`)
	out := evalString(t, rt, `
	var fragment = document.createDocumentFragment();
	fragment.appendChild(document.createElement("b"));
	fragment.replaceChildren("x", document.createElement("i"));
	fragment.firstChild.nodeName + "/" + fragment.lastChild.nodeName + "/" + fragment.textContent;`)
	if out != "#text/I/x" {
		t.Fatalf("fragment.replaceChildren = %q", out)
	}
}
