package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// ---------------------------------------------------------------------------
// Live NodeList (childNodes)
// ---------------------------------------------------------------------------

// TestLiveNodeListReflectsAppendChild pins the core live-NodeList contract:
// a childNodes reference taken before a mutation must see the new child in
// its length and indexed access without re-querying the DOM.
func TestLiveNodeListReflectsAppendChild(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="parent"><p>one</p><p>two</p></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var parent = document.getElementById("parent");
		var kids = parent.childNodes;
		var before = kids.length;
		var newChild = document.createElement("span");
		parent.appendChild(newChild);
		var after = kids.length;
		var item2 = kids.item(2) === newChild;
		var indexed = kids[2] === newChild;
		before + "/" + after + "/" + item2 + "/" + indexed;
	`)
	if out != "2/3/true/true" {
		t.Fatalf("live NodeList after appendChild = %q, want %q", out, "2/3/true/true")
	}
}

// TestLiveNodeListReflectsRemoveChild verifies that removing a child from the
// DOM shrinks the live NodeList.
func TestLiveNodeListReflectsRemoveChild(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="parent"><p>one</p><p>two</p><p>three</p></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var parent = document.getElementById("parent");
		var kids = parent.childNodes;
		var before = kids.length;
		var second = kids.item(1);
		parent.removeChild(second);
		var after = kids.length;
		var firstText = kids.item(0).textContent;
		var newSecond = kids.item(1).textContent;
		before + "/" + after + "/" + firstText + "/" + newSecond;
	`)
	if out != "3/2/one/three" {
		t.Fatalf("live NodeList after removeChild = %q, want %q", out, "3/2/one/three")
	}
}

// TestLiveNodeListItemOutOfBounds verifies item() returns null for invalid indices.
func TestLiveNodeListItemOutOfBounds(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="d"><p>one</p></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var d = document.getElementById("d");
		var kids = d.childNodes;
		var oob1 = kids.item(5);
		var oob2 = kids.item(-1);
		var ok = kids.item(0) !== null;
		(oob1 === null) + "/" + (oob2 === null) + "/" + ok;
	`)
	if out != "true/true/true" {
		t.Fatalf("item out of bounds = %q, want %q", out, "true/true/true")
	}
}

// TestLiveNodeListForEach verifies the forEach method works on live NodeLists.
func TestLiveNodeListForEach(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="d"><p>a</p><p>b</p><p>c</p></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var d = document.getElementById("d");
		var texts = [];
		d.childNodes.forEach(function(child) { texts.push(child.textContent); });
		texts.join(",");
	`)
	if out != "a,b,c" {
		t.Fatalf("forEach = %q, want %q", out, "a,b,c")
	}
}

// TestLiveNodeListForOf verifies Symbol.iterator works for for-of loops.
func TestLiveNodeListForOf(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="d"><p>x</p><p>y</p></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var d = document.getElementById("d");
		var texts = [];
		for (var child of d.childNodes) { texts.push(child.textContent); }
		texts.join(",");
	`)
	if out != "x,y" {
		t.Fatalf("for-of = %q, want %q", out, "x,y")
	}
}

func TestChildNodesIdentityAndNodeListIterators(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="d"><span>a</span><span>b</span></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var node = document.getElementById("d");
		var list = node.childNodes;
		var same = list === node.childNodes;
		var keys = Array.from(list.keys()).join(",");
		var values = Array.from(list.values()).map(function(n) { return n.localName; }).join(",");
		var entries = Array.from(list.entries()).map(function(pair) { return pair[0] + ":" + pair[1].localName; }).join(",");
		node.appendChild(document.createElement("em"));
		[same, keys, values, entries, list === node.childNodes, list.length].join("|");
	`)
	if out != "true|0,1|span,span|0:span,1:span|true|3" {
		t.Fatalf("childNodes identity and iterators = %q", out)
	}
}

// ---------------------------------------------------------------------------
// Live HTMLCollection (getElementsByTagName)
// ---------------------------------------------------------------------------

// TestLiveHTMLCollectionTagReflectsAppend pins the core live-HTMLCollection
// contract for getElementsByTagName: appending a matching element to the
// document must increase the collection's length.
func TestLiveHTMLCollectionTagReflectsAppend(t *testing.T) {
	doc := domtest.Parse(`<html><body><div>one</div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var items = document.getElementsByTagName("div");
		var before = items.length;
		document.body.appendChild(document.createElement("div"));
		var after = items.length;
		before + "/" + after;
	`)
	if out != "1/2" {
		t.Fatalf("live HTMLCollection(tag) after append = %q, want %q", out, "1/2")
	}
}

// TestLiveHTMLCollectionTagItem verifies item() returns the correct element.
func TestLiveHTMLCollectionTagItem(t *testing.T) {
	doc := domtest.Parse(`<html><body><p>first</p><p>second</p></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var items = document.getElementsByTagName("p");
		items.item(0).textContent + "/" + items.item(1).textContent + "/" + (items.item(5) === null);
	`)
	if out != "first/second/true" {
		t.Fatalf("item() = %q, want %q", out, "first/second/true")
	}
}

// TestLiveHTMLCollectionTagIndexed verifies numeric indexed access works.
func TestLiveHTMLCollectionTagIndexed(t *testing.T) {
	doc := domtest.Parse(`<html><body><span>a</span><span>b</span></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var items = document.getElementsByTagName("span");
		items[0].textContent + "/" + items[1].textContent;
	`)
	if out != "a/b" {
		t.Fatalf("indexed access = %q, want %q", out, "a/b")
	}
}

// HTMLCollection exposes its legacy iterator, but not NodeList's iterable
// helper methods.
func TestHTMLCollectionIterableSurface(t *testing.T) {
	doc := domtest.Parse(`<html><body><li>1</li><li>2</li><li>3</li></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var items = document.getElementsByTagName("li");
		["values" in items, "entries" in items, "forEach" in items, Symbol.iterator in items].join("/");
	`)
	if out != "false/false/false/true" {
		t.Fatalf("HTMLCollection iterable surface = %q", out)
	}
}

func TestHTMLCollectionNamedPropertiesAndIndexedWrites(t *testing.T) {
	doc := domtest.Parse(`<html><body><p id="named">value</p></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	out := evalString(t, r, `
		var collection = document.getElementsByTagName("p");
		var child = Object.create(collection);
		child.named = "shadow";
		var namedWriteThrows = false, indexWriteThrows = false, indexedDeleteThrows = false;
		try { (function() { "use strict"; collection.named = "changed"; })(); } catch (_) { namedWriteThrows = true; }
		try { (function() { "use strict"; collection[0] = "changed"; })(); } catch (_) { indexWriteThrows = true; }
		try { (function() { "use strict"; delete collection[0]; })(); } catch (_) { indexedDeleteThrows = true; }
		[
			Object.getOwnPropertyNames(collection).sort().join(","),
			collection.named.textContent,
			child.named,
			namedWriteThrows,
			indexWriteThrows,
			indexedDeleteThrows,
			(function() { "use strict"; return delete collection[5]; })()
		].join("|");
	`)
	if out != "0,named|value|shadow|true|true|true|true" {
		t.Fatalf("HTMLCollection named properties and indexed writes = %q", out)
	}
}

// TestLiveHTMLCollectionTagForOf verifies for-of works on HTMLCollections.
func TestLiveHTMLCollectionTagForOf(t *testing.T) {
	doc := domtest.Parse(`<html><body><em>x</em><em>y</em></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var items = document.getElementsByTagName("em");
		var texts = [];
		for (var el of items) { texts.push(el.textContent); }
		texts.join(",");
	`)
	if out != "x,y" {
		t.Fatalf("for-of = %q, want %q", out, "x,y")
	}
}

// ---------------------------------------------------------------------------
// Live HTMLCollection (getElementsByClassName)
// ---------------------------------------------------------------------------

// TestLiveHTMLCollectionClassReflectsMutation verifies getElementsByClassName
// returns a live collection that reflects class attribute changes.
func TestLiveHTMLCollectionClassReflectsMutation(t *testing.T) {
	doc := domtest.Parse(`<html><body><div class="a">1</div><div class="b">2</div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var items = document.getElementsByClassName("a");
		var before = items.length;
		// Add class "a" to the second div
		document.body.lastChild.className = "a";
		var after = items.length;
		before + "/" + after;
	`)
	if out != "1/2" {
		t.Fatalf("live HTMLCollection(class) after mutation = %q, want %q", out, "1/2")
	}
}

// ---------------------------------------------------------------------------
// namedItem
// ---------------------------------------------------------------------------

// TestHTMLCollectionNamedItemByID verifies namedItem finds elements by id.
func TestHTMLCollectionNamedItemByID(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="foo">found</div><div id="bar">other</div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var items = document.getElementsByTagName("div");
		var found = items.namedItem("foo");
		found !== null ? found.textContent : "null";
	`)
	if out != "found" {
		t.Fatalf("namedItem by id = %q, want %q", out, "found")
	}
}

// TestHTMLCollectionNamedItemByName verifies namedItem finds elements by name attribute.
func TestHTMLCollectionNamedItemByName(t *testing.T) {
	doc := domtest.Parse(`<html><body><input name="username" /><input name="password" /></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var items = document.getElementsByTagName("input");
		var found = items.namedItem("password");
		found !== null ? found.tagName : "null";
	`)
	if out != "INPUT" {
		t.Fatalf("namedItem by name = %q, want %q", out, "INPUT")
	}
}

// TestHTMLCollectionNamedItemMissing verifies namedItem returns null for missing names.
func TestHTMLCollectionNamedItemMissing(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="a">1</div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var items = document.getElementsByTagName("div");
		var missing = items.namedItem("nonexistent");
		missing === null ? "null" : "found";
	`)
	if out != "null" {
		t.Fatalf("namedItem missing = %q, want %q", out, "null")
	}
}

// ---------------------------------------------------------------------------
// Element-level query methods
// ---------------------------------------------------------------------------

// TestElementGetElementsByTagName verifies elements have getElementsByTagName
// and it returns a live HTMLCollection scoped to the element's subtree.
func TestElementGetElementsByTagName(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="outer"><span>a</span><div id="inner"><span>b</span></div></div><span>c</span></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var outer = document.getElementById("outer");
		var spans = outer.getElementsByTagName("span");
		spans.length + "/" + spans.item(0).textContent + "/" + spans.item(1).textContent;
	`)
	if out != "2/a/b" {
		t.Fatalf("element.getElementsByTagName = %q, want %q", out, "2/a/b")
	}
}

// TestElementGetElementsByClassName verifies elements have getElementsByClassName.
func TestElementGetElementsByClassName(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="outer"><p class="x">1</p><p class="y">2</p><p class="x">3</p></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var outer = document.getElementById("outer");
		var items = outer.getElementsByClassName("x");
		items.length + "/" + items.item(0).textContent + "/" + items.item(1).textContent;
	`)
	if out != "2/1/3" {
		t.Fatalf("element.getElementsByClassName = %q, want %q", out, "2/1/3")
	}
}

// ---------------------------------------------------------------------------
// Empty collection edge cases
// ---------------------------------------------------------------------------

// TestLiveNodeListEmpty verifies an empty node has a childNodes with length 0.
func TestLiveNodeListEmpty(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="empty"></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var d = document.getElementById("empty");
		var kids = d.childNodes;
		kids.length + "/" + (kids.item(0) === null);
	`)
	if out != "0/true" {
		t.Fatalf("empty childNodes = %q, want %q", out, "0/true")
	}
}

// TestLiveHTMLCollectionNoMatches verifies an empty HTMLCollection.
func TestLiveHTMLCollectionNoMatches(t *testing.T) {
	doc := domtest.Parse(`<html><body><p>text</p></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})

	out := evalString(t, r, `
		var items = document.getElementsByTagName("div");
		items.length + "/" + (items.item(0) === null) + "/" + (items.namedItem("x") === null);
	`)
	if out != "0/true/true" {
		t.Fatalf("empty HTMLCollection = %q, want %q", out, "0/true/true")
	}
}
