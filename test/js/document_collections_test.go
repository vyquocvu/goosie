package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func assertEval(t *testing.T, r *js.Runtime, expr, want string) {
	t.Helper()
	got, err := r.Evaluate(expr)
	if err != nil {
		t.Fatalf("%s: %v", expr, err)
	}
	if got != want {
		t.Fatalf("%s = %q, want %q", expr, got, want)
	}
}

const docCollectionsFixture = `<!DOCTYPE html><html><head>` +
	`<script id="s1" src="a.js"></script><script id="s2">var x=1;</script>` +
	`<link rel="stylesheet" href="a.css"></head><body>` +
	`<img id="i1" src="1.png"><a id="l1" href="#a">A</a>` +
	`<div><img id="i2"><a id="l2">no-href</a></div>` +
	`<map name="m"><area id="ar1" href="/z" alt="z"><area id="ar2" alt="nohref"></map>` +
	`<script id="s3">var y=2;</script>` +
	`<a id="l3" href="http://e.test/b">B</a></body></html>`

// qc:BR-295
func TestDocumentScriptsImagesLinksAreLiveHTMLCollections(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse(docCollectionsFixture))
	assertEval(t, r, `typeof document.scripts`, "object")
	assertEval(t, r, `typeof document.images`, "object")
	assertEval(t, r, `typeof document.links`, "object")
	assertEval(t, r, `Object.prototype.toString.call(document.scripts)`, "[object HTMLCollection]")
	assertEval(t, r, `document.scripts === document.scripts`, "true")
	assertEval(t, r, `document.links.length`, "3")
	assertEval(t, r, `document.scripts[0] ? document.scripts[0].id : "none"`, "s1")
	assertEval(t, r, `document.images.namedItem("i2") ? document.images.namedItem("i2").id : "none"`, "i2")
}

// qc:BR-295
func TestDocumentScriptsAndImagesAreInDocumentOrder(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse(docCollectionsFixture))
	assertEval(t, r, `Array.from(document.scripts).map(function(e){return e.id}).join(",")`, "s1,s2,s3")
	assertEval(t, r, `Array.from(document.images).map(function(e){return e.id}).join(",")`, "i1,i2")
}

// qc:BR-295
func TestDocumentLinksCollectOnlyHrefAnchorsAndAreasInOrder(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse(docCollectionsFixture))
	assertEval(t, r, `Array.from(document.links).map(function(e){return e.id}).join(",")`, "l1,ar1,l3")
	assertEval(t, r, `Array.from(document.links).some(function(e){return e.id==="l2"})`, "false")
	assertEval(t, r, `Array.from(document.links).some(function(e){return e.id==="ar2"})`, "false")
}

// qc:BR-295
func TestDocumentCollectionsAreEmptyOnADocumentWithoutMembers(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse(`<!DOCTYPE html><html><head><title>t</title></head><body><p>hi</p></body></html>`))
	assertEval(t, r, `document.scripts.length`, "0")
	assertEval(t, r, `document.images.length`, "0")
	assertEval(t, r, `document.links.length`, "0")
	assertEval(t, r, `Array.isArray(Array.from(document.scripts)) && document.scripts.length===0`, "true")
}
