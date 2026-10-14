package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// document.documentElement is the entry point boot scripts use for global
// listeners (Google: document.documentElement.addEventListener("click"...)).
func TestDocumentElementExists(t *testing.T) {
	doc := domtest.Parse(`<html><head></head><body><p>hi</p></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `var out = document.documentElement.tagName + "/" + (typeof document.documentElement.addEventListener) + "/" + (document.documentElement === document.querySelector("html"));`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "HTML/function/true" {
		t.Fatalf("documentElement = %+v, want HTML/function/true", v)
	}
}

func TestDocumentHeadExists(t *testing.T) {
	doc := domtest.Parse(`<html><head><title>t</title></head><body></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	if err := r.Run(`var out = document.head.tagName;`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "HEAD" {
		t.Fatalf("document.head = %+v, want HEAD", v)
	}
}

func TestDocumentElementListenerSurvives(t *testing.T) {
	doc := domtest.Parse(`<html><body></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `
var fired = false;
document.documentElement.addEventListener("click", function() { fired = true; });
document.documentElement.dispatchEvent({type: "click"});`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("fired"); !ok || !v.Bool {
		t.Fatalf("documentElement click fired = %+v, want true", v)
	}
}
