package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// Elements are query roots too (DOM ParentNode): el.querySelector(All)
// searches the element's subtree, never outside it. Dynamic pages scope
// every lookup this way (Google: g.querySelector("div")).
func TestElementQuerySelector(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="outer"><p class="x">1</p><div id="inner"><p class="x">2</p></div></div><p class="x">3</p></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `
var outer = document.getElementById("outer");
var inner = document.getElementById("inner");
var out = outer.querySelector("p").textContent + "/" + outer.querySelectorAll("p").length + "/" + inner.querySelector("p").textContent + "/" + (outer.querySelector("#inner") !== null);`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "1/2/2/true" {
		t.Fatalf("element qs = %+v, want 1/2/2/true", v)
	}
}

func TestElementQuerySelectorExcludesOutside(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="outer"><span>a</span></div><span>b</span></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `
var outer = document.getElementById("outer");
var out = outer.querySelectorAll("span").length + "/" + (outer.querySelector("div") === null);`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "1/true" {
		t.Fatalf("scoped qs = %+v, want 1/true", v)
	}
}

func TestElementQuerySelectorAttr(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="g"><div><span jsname="sIJmDf">hit</span></div></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var out = document.getElementById("g").querySelector("[jsname=sIJmDf]").textContent;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "hit" {
		t.Fatalf("attr qs = %+v, want hit", v)
	}
}
