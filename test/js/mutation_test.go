package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// mutationDOM builds a minimal DOM tree for mutation tests:
//
//	html
//	  body
//	    div#main.container
//	      p
//	        "hello"
func mutationDOM() *dom.Document {
	doc := &dom.Document{}
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	div := doc.NewElement("div")
	div.SetAttribute("id", "main")
	div.SetAttribute("class", "container")
	body.AppendChild(div)
	p := doc.NewElement("p")
	p.AppendChild(doc.NewText("hello"))
	div.AppendChild(p)
	doc.Body = body
	doc.HTML = html
	return doc
}

// newMutationRuntime creates a JS runtime with the given DOM and optional
// OnMutation callback.
func newMutationRuntime(t *testing.T, doc *dom.Document, onMutation func()) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		URL:        "https://example.test/",
		DOM:        doc,
		OnMutation: onMutation,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// ---------------------------------------------------------------------------
// Attribute tests
// ---------------------------------------------------------------------------

func TestGetAttribute(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		var gotAttr = el.getAttribute('id');
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("gotAttr"); !ok || v.Str != "main" {
		t.Errorf("gotAttr = %+v, want %q", v, "main")
	}
}

func TestGetAttributeMissing(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		var gotNull = el.getAttribute('data-nonexistent');
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	v, ok := r.Global("gotNull")
	if !ok {
		t.Fatal("gotNull not found")
	}
	if v.Kind != js.KindNull {
		t.Errorf("gotNull kind = %v, want KindNull", v.Kind)
	}
}

func TestSetAttribute(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		el.setAttribute('data-x', '1');
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	// Verify on the Go side.
	div := doc.ElementByID("main")
	if got := div.GetAttribute("data-x"); got != "1" {
		t.Errorf("data-x = %q, want %q", got, "1")
	}
}

func TestRemoveAttribute(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		el.removeAttribute('id');
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	// Verify the attribute is gone on the original node.
	divs := doc.ElementsByTagName("div")
	if len(divs) == 0 {
		t.Fatal("no div found")
	}
	if divs[0].HasAttribute("id") {
		t.Error("id attribute still present after removeAttribute")
	}
}

func TestHasAttribute(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		var hasId = el.hasAttribute('id');
		var hasMissing = el.hasAttribute('data-nope');
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("hasId"); !ok || !v.Bool {
		t.Errorf("hasId = %+v, want true", v)
	}
	if v, ok := r.Global("hasMissing"); !ok || v.Bool {
		t.Errorf("hasMissing = %+v, want false", v)
	}
}

// ---------------------------------------------------------------------------
// classList tests
// ---------------------------------------------------------------------------

func TestClassListAdd(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		el.classList.add('active');
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("main")
	found := false
	for _, c := range div.ClassList() {
		if c == "active" {
			found = true
		}
	}
	if !found {
		t.Errorf("classList = %v, want it to contain %q", div.ClassList(), "active")
	}
}

func TestClassListRemove(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		el.classList.remove('container');
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("main")
	for _, c := range div.ClassList() {
		if c == "container" {
			t.Error("container class still present after classList.remove")
		}
	}
}

func TestClassListToggle(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		var r1 = el.classList.toggle('container');
		var r2 = el.classList.toggle('container');
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	// First toggle removes "container" (it was present), returns false.
	if v, ok := r.Global("r1"); !ok || v.Bool != false {
		t.Errorf("r1 = %+v, want false (removed)", v)
	}
	// Second toggle adds "container" back, returns true.
	if v, ok := r.Global("r2"); !ok || v.Bool != true {
		t.Errorf("r2 = %+v, want true (added)", v)
	}
}

func TestClassListContains(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		var hasContainer = el.classList.contains('container');
		var hasActive = el.classList.contains('active');
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("hasContainer"); !ok || !v.Bool {
		t.Errorf("hasContainer = %+v, want true", v)
	}
	if v, ok := r.Global("hasActive"); !ok || v.Bool {
		t.Errorf("hasActive = %+v, want false", v)
	}
}

// ---------------------------------------------------------------------------
// innerHTML tests
// ---------------------------------------------------------------------------

func TestInnerHTMLGetter(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		var gotHTML = el.innerHTML;
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("gotHTML"); !ok || v.Str != "<p>hello</p>" {
		t.Errorf("gotHTML = %+v, want %q", v, "<p>hello</p>")
	}
}

func TestInnerHTMLSetter(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		el.innerHTML = '<span>new</span>';
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("main")
	if div.FirstChild == nil {
		t.Fatal("div has no children after innerHTML set")
	}
	if div.FirstChild.Data != "span" {
		t.Errorf("first child tag = %q, want %q", div.FirstChild.Data, "span")
	}
}

// ---------------------------------------------------------------------------
// Element removal and query tests
// ---------------------------------------------------------------------------

func TestRemoveElement(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		el.remove();
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if doc.ElementByID("main") != nil {
		t.Error("element still found after remove()")
	}
}

func TestGetElementById(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('main');
		var gotTag = el.tagName;
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("gotTag"); !ok || v.Str != "DIV" {
		t.Errorf("gotTag = %+v, want %q", v, "DIV")
	}
}

func TestGetElementByIdMissing(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var el = document.getElementById('nonexistent');
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	v, ok := r.Global("el")
	if !ok {
		t.Fatal("el not found")
	}
	if v.Kind != js.KindNull {
		t.Errorf("el kind = %v, want KindNull", v.Kind)
	}
}

func TestGetElementsByClassName(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var els = document.getElementsByClassName('container');
		var count = els.length;
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("count"); !ok || v.Num != 1 {
		t.Errorf("count = %+v, want 1", v)
	}
}

func TestGetElementsByTagName(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var els = document.getElementsByTagName('p');
		var count = els.length;
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("count"); !ok || v.Num != 1 {
		t.Errorf("count = %+v, want 1", v)
	}
}

func TestCreateTextNode(t *testing.T) {
	doc := mutationDOM()
	r := newMutationRuntime(t, doc, nil)

	if err := r.Run(`
		var tn = document.createTextNode('world');
		var gotContent = tn.textContent;
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("gotContent"); !ok || v.Str != "world" {
		t.Errorf("gotContent = %+v, want %q", v, "world")
	}
}

// ---------------------------------------------------------------------------
// Mutation callback test
// ---------------------------------------------------------------------------

func TestSetAttributeTriggersMutation(t *testing.T) {
	doc := mutationDOM()
	mutationCount := 0
	r := newMutationRuntime(t, doc, func() { mutationCount++ })

	if err := r.Run(`
		var el = document.getElementById('main');
		el.setAttribute('data-new', 'value');
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if mutationCount == 0 {
		t.Error("OnMutation callback was not called after setAttribute")
	}
}
