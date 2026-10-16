package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// eventDOM builds a DOM tree for event tests:
//
//	html
//	  body
//	    div#target
func eventDOM() *dom.Document {
	doc := &dom.Document{}
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	div := doc.NewElement("div")
	div.SetAttribute("id", "target")
	body.AppendChild(div)
	doc.Body = body
	doc.HTML = html
	return doc
}

// newEventRuntime creates a JS runtime with the event test DOM.
func newEventRuntime(t *testing.T, doc *dom.Document) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		URL: "https://example.test/",
		DOM: doc,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// TestAddEventListenerFires verifies that addEventListener("click", fn) fires
// when a click event is dispatched on the element.
func TestAddEventListenerFires(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var gotType = '';
		var el = document.getElementById('target');
		el.addEventListener('click', function(e) {
			gotType = e.type;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	// Dispatch a click event on the div from Go.
	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("gotType"); !ok || v.Str != "click" {
		t.Errorf("gotType = %+v, want %q", v, "click")
	}
}

// TestEventBubblesInJS verifies that an event dispatched on a child element
// bubbles up to the parent's listener.
func TestEventBubblesInJS(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var bubbled = false;
		var body = document.body;
		body.addEventListener('click', function(e) {
			bubbled = true;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	// Dispatch on the child div; the body listener should fire via bubbling.
	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("bubbled"); !ok || !v.Bool {
		t.Errorf("bubbled = %+v, want true", v)
	}
}

// TestStopPropagationFromJS verifies that event.stopPropagation() prevents
// the event from reaching ancestor listeners.
func TestStopPropagationFromJS(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var bodyFired = false;
		var body = document.body;
		body.addEventListener('click', function(e) {
			bodyFired = true;
		});
		var el = document.getElementById('target');
		el.addEventListener('click', function(e) {
			e.stopPropagation();
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("bodyFired"); !ok || v.Bool {
		t.Errorf("bodyFired = %+v, want false (stopPropagation should prevent bubbling)", v)
	}
}

// TestPreventDefaultFromJS verifies that event.preventDefault() marks the
// event as default-prevented.
func TestPreventDefaultFromJS(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var wasDefaultPrevented = false;
		var el = document.getElementById('target');
		el.addEventListener('click', function(e) {
			e.preventDefault();
			wasDefaultPrevented = e.defaultPrevented;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	result := dom.DispatchEvent(div, event)

	if v, ok := r.Global("wasDefaultPrevented"); !ok || !v.Bool {
		t.Errorf("wasDefaultPrevented = %+v, want true", v)
	}
	// DispatchEvent returns false when the event was cancelled.
	if result {
		t.Error("DispatchEvent returned true, want false (event was cancelled)")
	}
}

// TestEventTargetIsSet verifies that event.target is the element the listener
// is registered on (for at-target dispatch).
func TestEventTargetIsSet(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var gotTargetID = '';
		var el = document.getElementById('target');
		el.addEventListener('click', function(e) {
			gotTargetID = e.target.getAttribute('id');
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("gotTargetID"); !ok || v.Str != "target" {
		t.Errorf("gotTargetID = %+v, want %q", v, "target")
	}
}

// TestDispatchEventFromJS verifies that element.dispatchEvent(event) from JS
// fires the element's own listeners.
func TestDispatchEventFromJS(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var jsDispatched = false;
		var el = document.getElementById('target');
		el.addEventListener('custom', function(e) {
			jsDispatched = true;
		});
		var evt = { type: 'custom' };
		el.dispatchEvent(evt);
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("jsDispatched"); !ok || !v.Bool {
		t.Errorf("jsDispatched = %+v, want true", v)
	}
}
