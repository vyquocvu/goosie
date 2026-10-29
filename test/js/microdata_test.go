package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// HTML microdata (HTML Living Standard §5.2) is the structured-data
// vocabulary built into HTML itself. These tests pin the DOM side: the five
// attribute reflections, the properties collection, and document.getItems.

// The reflections read their content attributes; the IDL names are reachable
// in both the content-attribute spelling (itemscope, itemprop) and the spec's
// camelCase IDL spelling (itemScope, itemProp). An element without the
// attributes reflects empty/false.
func TestMicrodataReflectionReads(t *testing.T) {
	doc := domtest.Parse(`<html><body>
		<div id="a" itemscope itemtype="http://schema.org/Person" itemid="https://example.test/alice"
			itemprop="name" itemref="partner home"></div>
		<div id="b"></div>
	</body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var a = document.getElementById("a");
var b = document.getElementById("b");
var out = a.itemscope + "/" + a.itemtype + "/" + a.itemid + "/" + a.itemprop + "/" + a.itemref + "/" +
	b.itemscope + "/" + b.itemtype + "/" + b.itemid + "/" +
	(typeof a.itemScope) + "/" + (typeof a.itemType) + "/" + (typeof a.itemProp) + "/" + (typeof a.itemRef);`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	want := "true/http://schema.org/Person/https://example.test/alice/name/partner home/false///" +
		"boolean/string/string/string"
	if v, ok := r.Global("out"); !ok || v.Str != want {
		t.Fatalf("microdata reflections = %q, want %q", v.Str, want)
	}
}

// Writing the reflections writes the content attributes: itemscope as a
// boolean reflects presence (false removes the attribute), the string
// reflections spell the value back, and either spelling drives the write.
func TestMicrodataReflectionWrites(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="a"></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var a = document.getElementById("a");
a.itemscope = true;
a.itemtype = "http://schema.org/Person";
a.itemProp = "name";
var wrote = a.hasAttribute("itemscope") + "/" + a.getAttribute("itemtype") + "/" +
	a.getAttribute("itemprop") + "/" + a.itemprop;
a.itemscope = false;
a.itemType = "http://schema.org/Event";
var out = wrote + "/" + a.hasAttribute("itemscope") + "/" + a.getAttribute("itemtype");`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	want := "true/http://schema.org/Person/name/name/false/http://schema.org/Event"
	if v, ok := r.Global("out"); !ok || v.Str != want {
		t.Fatalf("microdata reflection writes = %q, want %q", v.Str, want)
	}
}

// properties is the item's properties collection: every descendant with an
// itemprop attribute in tree order, with length, item(index), indexed access,
// and names in first-encounter order. Properties outside the item's subtree
// do not belong to it.
func TestMicrodataProperties(t *testing.T) {
	doc := domtest.Parse(`<html><body>
		<div id="item" itemscope itemtype="http://schema.org/Person">
			<span itemprop="name">Alice</span>
			<span itemprop="name">Alicia</span>
			<span itemprop="age">30</span>
			<meta itemprop="eyeColor" content="green">
		</div>
		<span itemprop="orphan"></span>
	</body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var props = document.getElementById("item").properties;
var out = props.length + "/" + props[0].itemprop + "/" + props.item(1).itemprop + "/" +
	props[2].itemprop + "/" + props.names.length + "/" + props.names.join(",") + "/" +
	(props.item(9) === null);`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	want := "4/name/name/age/3/name,age,eyeColor/true"
	if v, ok := r.Global("out"); !ok || v.Str != want {
		t.Fatalf("properties collection = %q, want %q", v.Str, want)
	}
}

// properties is live: it re-queries the tree on every read, so a property
// appended after the first read shows up in the next one.
func TestMicrodataPropertiesLive(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="item" itemscope></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var item = document.getElementById("item");
var before = item.properties.length;
var span = document.createElement("span");
span.setAttribute("itemprop", "name");
item.appendChild(span);
var out = before + "/" + item.properties.length + "/" + item.properties[0].itemprop;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "0/1/name" {
		t.Fatalf("properties liveness = %+v, want 0/1/name", v)
	}
}

// The walk stops at a nested item: the inner element's own properties belong
// to that item, not to the outer one.
func TestMicrodataPropertiesStopAtNestedItems(t *testing.T) {
	doc := domtest.Parse(`<html><body>
		<div id="outer" itemscope>
			<span itemprop="name">Alice</span>
			<div itemscope itemprop="address">
				<span itemprop="city">Springfield</span>
			</div>
		</div>
	</body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var outer = document.getElementById("outer");
var out = outer.properties.length + "/" + outer.properties.names.join(",");`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "2/name,address" {
		t.Fatalf("nested item scoping = %+v, want 2/name,address", v)
	}
}

func TestMicrodataPropertiesFollowItemRefAndKeepNestedScopesSeparate(t *testing.T) {
	doc := domtest.Parse(`<html><body>
		<div id="details">
			<span itemprop="job">engineer</span>
			<section itemscope><span itemprop="private">not outer</span></section>
		</div>
		<div id="outer" itemscope itemref="details address details">
			<span itemprop="name">Ada</span>
			<div id="nested" itemprop="child" itemscope itemref="extra">
				<span itemprop="kind">person</span>
			</div>
		</div>
		<span id="address" itemprop="home">London</span>
		<span id="extra" itemprop="city">London</span>
		<span id="details" itemprop="duplicate">ignored duplicate ID</span>
	</body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var outer = document.getElementById("outer");
var nested = document.getElementById("nested");
var out = outer.properties.length + "/" + outer.properties.names.join(",") + "/" +
	outer.properties[0].itemprop + "/" + outer.properties[3].itemprop + "/" +
	nested.properties.length + "/" + nested.properties.names.join(",");`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "4/job,name,child,home/job/home/2/kind,city" {
		t.Fatalf("itemref property traversal = %+v, want 4/job,name,child,home/job/home/2/kind,city", v)
	}
}

func TestMicrodataItemRefUsesItsTreeScope(t *testing.T) {
	doc := domtest.Parse(`<html><body>
		<div id="host"></div>
		<span id="ref" itemprop="documentScope">outside</span>
	</body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var host = document.getElementById("host");
var root = host.attachShadow({mode: "open"});
root.innerHTML = '<div id="item" itemscope itemref="ref"></div><span id="ref" itemprop="shadowScope">inside</span>';
var item = root.getElementById("item");
var out = item.properties.length + "/" + item.properties.names.join(",");`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "1/shadowScope" {
		t.Fatalf("itemref crossed shadow tree scope = %+v, want 1/shadowScope", v)
	}
}

func TestMicrodataTokensSplitOnlyASCIIWhitespace(t *testing.T) {
	doc := domtest.Parse(`<html><body>
		<div id="item" itemscope itemtype="details address" itemref="details address">
			<span itemprop="first second">value</span>
		</div>
		<span id="details" itemprop="details">outside</span>
		<span id="address" itemprop="address">outside</span>
	</body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var props = document.getElementById("item").properties;
var out = props.length + "/" + props.names.length + "/" + props.names[0] + "/" +
	(document.getItems("details address").length);`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "1/1/first second/0" {
		t.Fatalf("microdata token splitting = %+v, want one NBSP-containing token and no resolved itemref", v)
	}
}

// getItems returns the document's top-level items: elements with itemscope
// that are not themselves a property of another item. A space-separated
// argument requires every named type among the item's itemtype tokens; an
// absent or empty argument returns every top-level item.
func TestMicrodataGetItems(t *testing.T) {
	doc := domtest.Parse(`<html><body>
		<div id="p1" itemscope itemtype="http://schema.org/Person"></div>
		<div id="p2" itemscope itemtype="http://schema.org/Person http://schema.org/Agent"></div>
		<div id="e1" itemscope itemtype="http://schema.org/Event"></div>
		<div id="none" itemscope></div>
		<div id="nested" itemscope itemprop="child" itemtype="http://schema.org/Person"></div>
		<span id="plain"></span>
	</body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var all = document.getItems();
var people = document.getItems("http://schema.org/Person");
var both = document.getItems("http://schema.org/Person http://schema.org/Agent");
var out = all.length + "/" + people.length + "/" + people[0].id + "/" + both.length + "/" +
	document.getItems("http://schema.org/Event").length + "/" +
	document.getItems("http://schema.org/Nothing").length + "/" +
	document.getItems("").length + "/" + all.item(3).id;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	want := "4/2/p1/1/1/0/4/none"
	if v, ok := r.Global("out"); !ok || v.Str != want {
		t.Fatalf("getItems = %+v, want %q", v, want)
	}
}

// properties is a read-only accessor: a sloppy-mode assignment neither
// replaces it nor throws.
func TestMicrodataPropertiesReadOnly(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="item" itemscope><span itemprop="name">x</span></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var item = document.getElementById("item");
item.properties = null;
var out = item.properties.length;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Num != 1 {
		t.Fatalf("properties write-through = %+v, want 1", v)
	}
}
