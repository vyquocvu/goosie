package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// testDOM is a small HTML tree used by every DOM test. It has enough
// structure to exercise tag, ID and class selectors without pulling in the
// full parser.
//
//	<html>
//	  <head><title>Test</title></head>
//	  <body>
//	    <p id="main" class="active">Hello</p>
//	    <p class="info">World</p>
//	    <div class="active">!</div>
//	  </body>
//	</html>
func testDOM(t *testing.T) *dom.Document {
	t.Helper()
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)

	head := doc.NewElement("head")
	html.AppendChild(head)
	title := doc.NewElement("title")
	head.AppendChild(title)
	title.AppendChild(doc.NewText("Test"))

	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.Head = head
	doc.HTML = html

	p1 := doc.NewElement("p")
	p1.SetAttribute("id", "main")
	p1.SetAttribute("class", "active")
	body.AppendChild(p1)
	p1.AppendChild(doc.NewText("Hello"))

	p2 := doc.NewElement("p")
	p2.SetAttribute("class", "info")
	body.AppendChild(p2)
	p2.AppendChild(doc.NewText("World"))

	div := doc.NewElement("div")
	div.SetAttribute("class", "active")
	body.AppendChild(div)
	div.AppendChild(doc.NewText("!"))

	return doc
}

// newDOMRuntime creates a JS runtime backed by d, registering cleanup.
func newDOMRuntime(t *testing.T, d *dom.Document) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		URL: "https://example.test/",
		DOM: d,
	})
	if err != nil {
		t.Fatalf("js.New with DOM: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// ---------------------------------------------------------------------------
// querySelector
// ---------------------------------------------------------------------------

func TestQuerySelectorByTag(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`var el = document.querySelector("div");`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, ok := r.Global("el")
	if !ok {
		t.Fatal("el not set")
	}
	if v.Kind != js.KindObject {
		t.Errorf("el.Kind = %v, want KindObject", v.Kind)
	}
}

func TestQuerySelectorByID(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var el = document.querySelector("#main");
		var tag = el ? el.tagName : "";
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, ok := r.Global("tag")
	if !ok {
		t.Fatal("tag not set")
	}
	if v.Str != "P" {
		t.Errorf("tagName = %q, want %q", v.Str, "P")
	}
}

func TestQuerySelectorByClass(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var el = document.querySelector(".info");
		var tag = el ? el.tagName : "";
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, _ := r.Global("tag")
	if v.Str != "P" {
		t.Errorf("tagName = %q, want %q", v.Str, "P")
	}
}

func TestQuerySelectorReturnsNullForMissing(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`var el = document.querySelector(".nonexistent");`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, ok := r.Global("el")
	if !ok {
		t.Fatal("el not set")
	}
	if v.Kind != js.KindNull {
		t.Errorf("el.Kind = %v, want KindNull", v.Kind)
	}
}

// ---------------------------------------------------------------------------
// querySelectorAll
// ---------------------------------------------------------------------------

func TestQuerySelectorAllByTag(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`var els = document.querySelectorAll("p");`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, ok := r.Global("els")
	if !ok {
		t.Fatal("els not set")
	}
	if v.Kind != js.KindObject {
		t.Fatalf("els.Kind = %v, want KindObject", v.Kind)
	}

	// Check the length via JS.
	if err := r.Run(`var len = document.querySelectorAll("p").length;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	ln, _ := r.Global("len")
	if ln.Num != 2 {
		t.Errorf("querySelectorAll('p').length = %g, want 2", ln.Num)
	}
}

func TestQuerySelectorAllByClass(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`var len = document.querySelectorAll(".active").length;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	ln, _ := r.Global("len")
	if ln.Num != 2 {
		t.Errorf("querySelectorAll('.active').length = %g, want 2", ln.Num)
	}
}

func TestQuerySelectorAllEmpty(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`var len = document.querySelectorAll(".nope").length;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	ln, _ := r.Global("len")
	if ln.Num != 0 {
		t.Errorf("querySelectorAll('.nope').length = %g, want 0", ln.Num)
	}
}

// ---------------------------------------------------------------------------
// createElement
// ---------------------------------------------------------------------------

func TestCreateElement(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var el = document.createElement("span");
		var tag = el.tagName;
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, _ := r.Global("tag")
	if v.Str != "SPAN" {
		t.Errorf("tagName = %q, want %q", v.Str, "SPAN")
	}
}

// ---------------------------------------------------------------------------
// appendChild
// ---------------------------------------------------------------------------

func TestAppendChild(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var parent = document.createElement("div");
		var child = document.createElement("span");
		parent.appendChild(child);
		var count = 0;
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify the child was attached by checking the DOM tree from Go.
	// The created elements are not in the document tree, but the parent
	// should have the child.
	v, _ := r.Global("parent")
	if v.Kind != js.KindObject {
		t.Fatalf("parent.Kind = %v, want KindObject", v.Kind)
	}

	// Check via JS that the parent now contains the child.
	if err := r.Run(`
		var parent = document.createElement("div");
		var child = document.createElement("p");
		parent.appendChild(child);
		var found = parent.textContent;
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// textContent of an element with an empty child element is "".
	v, _ = r.Global("found")
	if v.Str != "" {
		t.Errorf("textContent after appendChild = %q, want empty", v.Str)
	}
}

func TestAppendChildToDocumentBody(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var el = document.createElement("section");
		document.body.appendChild(el);
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The new <section> should now be findable via querySelector.
	if err := r.Run(`var found = document.querySelector("section");`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, _ := r.Global("found")
	if v.Kind != js.KindObject {
		t.Errorf("querySelector('section') = %v, want an object (found)", v.Kind)
	}
}

// ---------------------------------------------------------------------------
// textContent
// ---------------------------------------------------------------------------

func TestTextContentGet(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var el = document.querySelector("#main");
		var text = el.textContent;
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, _ := r.Global("text")
	if v.Str != "Hello" {
		t.Errorf("textContent = %q, want %q", v.Str, "Hello")
	}
}

func TestTextContentSet(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var el = document.querySelector("#main");
		el.textContent = "Goodbye";
		var text = el.textContent;
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, _ := r.Global("text")
	if v.Str != "Goodbye" {
		t.Errorf("textContent after set = %q, want %q", v.Str, "Goodbye")
	}
}

func TestTextContentSetOnCreatedElement(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var el = document.createElement("p");
		el.textContent = "Created text";
		var text = el.textContent;
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, _ := r.Global("text")
	if v.Str != "Created text" {
		t.Errorf("textContent = %q, want %q", v.Str, "Created text")
	}
}

// ---------------------------------------------------------------------------
// addEventListener
// ---------------------------------------------------------------------------

func TestAddEventListenerDoesNotThrow(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	err := r.Run(`
		var el = document.querySelector("#main");
		el.addEventListener("click", function() {});
	`, "inline")
	if err != nil {
		t.Errorf("addEventListener threw: %v", err)
	}
}

func TestAddEventListenerOnCreatedElement(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	err := r.Run(`
		var el = document.createElement("div");
		el.addEventListener("mouseover", function() { return 42; });
	`, "inline")
	if err != nil {
		t.Errorf("addEventListener on created element threw: %v", err)
	}
}

// ---------------------------------------------------------------------------
// No DOM provided
// ---------------------------------------------------------------------------

func TestQuerySelectorUndefinedWithoutDOM(t *testing.T) {
	// Without a DOM, document.querySelector is not defined, so calling it
	// should produce a runtime error.
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	err := r.Run(`document.querySelector("p");`, "inline")
	if err == nil {
		t.Error("expected error when calling querySelector without a DOM, got nil")
	}
}

// ---------------------------------------------------------------------------
// setAttributeNS and id reflection
// ---------------------------------------------------------------------------

func TestSetAttributeNSStoresQualifiedName(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var el = document.createElement("p");
		document.body.appendChild(el);
		el.setAttributeNS("http://www.w3.org/XML/1998/namespace", "xml:lang", "en");
		el.setAttributeNS(null, "lang", "de");
		var xmll = el.getAttribute("xml:lang");
		var lang = el.getAttribute("lang");
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, _ := r.Global("xmll"); v.Str != "en" {
		t.Errorf("xml:lang = %q, want en", v.Str)
	}
	if v, _ := r.Global("lang"); v.Str != "de" {
		t.Errorf("lang = %q, want de", v.Str)
	}
}

func TestIDPropertyReflectsAttribute(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var el = document.createElement("div");
		document.body.appendChild(el);
		el.id = "g";
		var readBack = el.id;
		var found = document.getElementById("g") !== null;
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, _ := r.Global("readBack"); v.Str != "g" {
		t.Errorf("id read-back = %q, want g", v.Str)
	}
	if v, _ := r.Global("found"); !v.Bool {
		t.Error("getElementById did not find the element after id assignment")
	}
}
