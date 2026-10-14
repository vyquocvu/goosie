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
