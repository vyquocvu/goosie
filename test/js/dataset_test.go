package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// HTMLElement.dataset (DOMStringMap) is how dynamic pages stash per-node
// state: Google reads b.dataset.ved off static markup, and a missing dataset
// takes its impression logger down.
func TestDatasetReads(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="b" data-ved="abc" data-trid="t1" data-multi-word="v"></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var b = document.getElementById("b");
var out = b.dataset.ved + "/" + b.dataset.trid + "/" + b.dataset.multiWord + "/" + b.dataset.missing;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "abc/t1/v/undefined" {
		t.Fatalf("dataset reads = %+v, want abc/t1/v/undefined", v)
	}
}

func TestDatasetWritesThrough(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="b"></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var b = document.getElementById("b");
b.dataset.ved = "xyz";
b.dataset.newKey = "n";
var out = b.dataset.ved + "/" + b.getAttribute("data-ved") + "/" + b.getAttribute("data-new-key");`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "xyz/xyz/n" {
		t.Fatalf("dataset writes = %+v, want xyz/xyz/n", v)
	}
}

func TestDatasetLiveAcrossAccesses(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="b" data-a="1"></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var b = document.getElementById("b");
var d1 = b.dataset;
b.setAttribute("data-a", "2");
var out = d1.a + "/" + b.dataset.a + "/" + Object.keys(b.dataset).length;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	// DOMStringMap is live: even an earlier handle reads the new value,
	// and keys enumerate the data-* attributes.
	if v, ok := r.Global("out"); !ok || v.Str != "2/2/1" {
		t.Fatalf("dataset liveness = %+v, want 2/2/1", v)
	}
}

func TestDatasetOwnPropertyNamesHideRegistryKey(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="b" data-first="1" data-second-value="2"></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	out := evalString(t, r, `
		var dataset = document.getElementById("b").dataset;
		[
			Object.getOwnPropertyNames(dataset).sort().join(","),
			Object.keys(dataset).sort().join(","),
			dataset.first + "/" + dataset.secondValue,
			"__nid__" in dataset
		].join("|");
	`)
	if out != "first,secondValue|first,secondValue|1/2|true" {
		t.Fatalf("dataset own property names = %q", out)
	}
}

func TestDatasetDeleteUsesDOMStringMapNames(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="b" data-foo="1" data--="2" data--foo="3" data---foo="4" data-="5"></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var b = document.getElementById("b");
delete b.dataset.foo;
delete b.dataset["-"];
delete b.dataset.Foo;
delete b.dataset["-Foo"];
delete b.dataset[""];
var out = [
  b.hasAttribute("data-foo"), b.hasAttribute("data--"),
  b.hasAttribute("data--foo"), b.hasAttribute("data---foo"),
  b.hasAttribute("data-")
].join("/");`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "false/false/false/false/false" {
		t.Fatalf("dataset delete/mapping = %+v, want all mapped data-* attributes removed", v)
	}
}

func TestDatasetUnmappableNameDoesNotAliasAttribute(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="b" data--foo="value"></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var b = document.getElementById("b");
var value = String(b.dataset["-foo"]);
var present = "-foo" in b.dataset;
delete b.dataset["-foo"];
var out = value + "/" + present + "/" + b.getAttribute("data--foo");`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "undefined/false/value" {
		t.Fatalf("noncanonical dataset name = %+v, want undefined/false/value", v)
	}
}

func TestHTMLElementTranslateAttributeAndInheritance(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="parent" translate="NO"><span id="inherited"></span><span id="enabled" translate="YeS"></span><span id="invalid" translate="yeſ"></span></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var parent = document.getElementById("parent");
var inherited = document.getElementById("inherited");
var enabled = document.getElementById("enabled");
var invalid = document.getElementById("invalid");
var before = [inherited.translate, enabled.translate, invalid.translate].join("/");
enabled.translate = false;
var out = before + "/" + enabled.getAttribute("translate") + "/" + parent.translate;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "false/true/false/no/false" {
		t.Fatalf("translate reflection = %+v, want false/true/false/no/false", v)
	}
}
