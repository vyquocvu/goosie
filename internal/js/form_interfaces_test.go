package js

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestFormPostToNamedIframeDispatchesLoadAfterResponse(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc, URL: "https://example.test/form.html"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	calls := 0
	runtime.SetIFrameFormSubmitter(func(base, target, contentType string, body []byte) (*dom.Document, error) {
		calls++
		if base != "https://example.test/form.html" || target != "https://example.test/upload" {
			t.Errorf("POST target base=%q target=%q", base, target)
		}
		if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") || !strings.Contains(string(body), "name=\"field\"") {
			t.Errorf("POST encoding content-type=%q body=%q", contentType, body)
		}
		return dom.ParseBounded("<!doctype html><body>received</body>", dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 1024})
	})
	if err := runtime.Run(`
		var frame = document.createElement('iframe');
		frame.name = 'reply';
		document.body.appendChild(frame);
		var form = document.createElement('form');
		form.method = 'post';
		form.enctype = 'multipart/form-data';
		form.action = '/upload';
		form.target = 'reply';
		var input = document.createElement('input');
		input.name = 'field';
		input.value = 'value';
		form.appendChild(input);
		document.body.appendChild(form);
		window.formPostLoadCount = 0;
		frame.onload = function() { window.formPostLoadCount++; };
		form.submit();
		window.formPostLoadBeforeTick = window.formPostLoadCount;
	`, "form-post-iframe"); err != nil {
		t.Fatal(err)
	}
	if got, _ := runtime.Global("formPostLoadBeforeTick"); got.Num != 0 {
		t.Fatalf("iframe load fired synchronously: %v", got.Num)
	}
	if calls != 1 {
		t.Fatalf("form POST transport calls = %d, want 1", calls)
	}
	if got, _ := runtime.Global("formPostLoadCount"); got.Num != 1 {
		t.Fatalf("iframe load event count = %v, want 1", got.Num)
	}
	if err := runtime.Run(`window.formPostResponseText = frame.contentDocument.body.textContent`, "form-post-response"); err != nil {
		t.Fatal(err)
	}
	if got, _ := runtime.Global("formPostResponseText"); got.Str != "received" {
		t.Fatalf("iframe response body = %q, want received", got.Str)
	}
}

func TestMultipartFormUsesAcceptCharsetAndNumericFallback(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc, URL: "https://example.test/form.html"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	var posted []byte
	runtime.SetIFrameFormSubmitter(func(_, _, contentType string, body []byte) (*dom.Document, error) {
		if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
			t.Errorf("POST content type = %q", contentType)
		}
		posted = append([]byte(nil), body...)
		return dom.ParseBounded("<!doctype html><body>ok</body>", dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 1024})
	})
	if err := runtime.Run(`
		var frame = document.createElement('iframe');
		frame.name = 'reply'; document.body.appendChild(frame);
		var form = document.createElement('form');
		form.method = 'post'; form.enctype = 'multipart/form-data';
		form.acceptCharset = 'windows-1252'; form.target = 'reply'; form.action = '/upload';
		var charsetField = document.createElement('input'); charsetField.type = 'hidden'; charsetField.name = '_charset_'; form.append(charsetField);
		var field = document.createElement('input'); field.name = 'filename'; field.value = 'star★'; form.append(field);
		document.body.append(form); form.submit();
	`, "form-post-charset"); err != nil {
		t.Fatal(err)
	}
	if got := string(posted); !strings.Contains(got, "\r\n\r\nwindows-1252\r\n") || !strings.Contains(got, "star&#9733;") {
		t.Fatalf("charset multipart body = %q", got)
	}
}

func TestFormOwnerDoesNotCrossShadowTree(t *testing.T) {
	doc := dom.NewDocument()
	lightForm := doc.NewElement("form")
	lightForm.SetAttribute("id", "outside")
	doc.Node.AppendChild(lightForm)
	host := doc.NewElement("span")
	doc.Node.AppendChild(host)
	root := &dom.ShadowRoot{Host: host, Mode: "open"}
	host.ShadowRoot = root
	control := doc.NewElement("input")
	control.SetAttribute("form", "outside")
	root.AppendChild(control)
	runtime, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.SetDOM(doc)
	if got := runtime.shadowRootForNode(control); got != root {
		t.Fatal("control's containing shadow root was not found")
	}
	if got := runtime.formOwner(control); got != nil {
		t.Fatalf("control in shadow tree associated with a light-tree form: %#v", got)
	}
}

func TestScriptCreatedShadowFormOwnerDoesNotCrossTree(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	form := doc.NewElement("form")
	form.SetAttribute("id", "outside")
	body.AppendChild(form)
	form2 := doc.NewElement("form")
	form2.SetAttribute("id", "form2")
	body.AppendChild(form2)
	runtime, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.SetDOM(doc)
	if err := runtime.Run(`
		var host = document.createElement('span');
		document.body.appendChild(host);
		var root = host.attachShadow({mode: 'open'});
		var input = document.createElement('input');
		input.setAttribute('form', 'outside');
		root.appendChild(input);
		var secondHost = document.createElement('span');
		document.body.appendChild(secondHost);
		var secondRoot = secondHost.attachShadow({mode: 'open'});
		var secondInput = document.createElement('input');
		secondInput.setAttribute('form', 'form2');
		secondRoot.appendChild(secondInput);
		window.formOwnerIsNull = input.form === null && secondInput.form === null;
	`, "test"); err != nil {
		t.Fatal(err)
	}
	if got, ok := runtime.Global("formOwnerIsNull"); !ok || !got.Bool {
		t.Fatalf("script-created shadow control form owner = %+v, want null", got)
	}
}

func TestFormCheckValidityRequiredControlAndInvalidEvent(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	form := doc.NewElement("form")
	form.SetAttribute("id", "f")
	input := doc.NewElement("input")
	input.SetAttribute("id", "i")
	input.SetAttribute("required", "")
	form.AppendChild(input)
	body.AppendChild(form)
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.SetDOM(doc)
	if err := runtime.Run(`
		var form = document.getElementById('f');
		var input = document.getElementById('i');
		var invalidEvents = 0;
		input.addEventListener('invalid', function() { invalidEvents++; });
		window.invalidStateWorks = form.checkValidity() === false && invalidEvents === 1;
		input.value = 'filled';
		window.validStateWorks = form.checkValidity() === true && invalidEvents === 1;
	`, "form-validation-test"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"invalidStateWorks", "validStateWorks"} {
		if value, ok := runtime.Global(name); !ok || !value.Bool {
			t.Fatalf("%s = %+v, want true", name, value)
		}
	}
}

func TestSubmitEventCarriesNullableSubmitter(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	form := doc.NewElement("form")
	form.SetAttribute("novalidate", "")
	button := doc.NewElement("button")
	form.AppendChild(button)
	body.AppendChild(form)
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.SetDOM(doc)
	if err := runtime.Run(`
		var form = document.querySelector('form');
		var button = document.querySelector('button');
		var observed;
		form.addEventListener('submit', function(event) {
			event.preventDefault();
			observed = event;
		});
		form.requestSubmit(button);
		var withSubmitter = observed instanceof SubmitEvent && observed.submitter === button;
		observed = undefined;
		form.requestSubmit();
		window.submitEventWorks = withSubmitter && observed instanceof SubmitEvent && observed.submitter === null;
	`, "submit-event-test"); err != nil {
		t.Fatal(err)
	}
	if got, ok := runtime.Global("submitEventWorks"); !ok || !got.Bool {
		t.Fatalf("SubmitEvent submitter semantics = %+v (present %v), want true", got, ok)
	}
}

func TestFormDataEventBubblesAndSharesIterableFormData(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	form := doc.NewElement("form")
	input := doc.NewElement("input")
	input.SetAttribute("name", "n1")
	input.SetAttribute("value", "v1")
	form.AppendChild(input)
	body.AppendChild(form)
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.SetDOM(doc)
	if err := runtime.Run(`
		var form = document.querySelector('form');
		var bubbled = false;
		var local = false;
		document.body.addEventListener('formdata', function(event) {
			bubbled = event.target === form && event.bubbles && !event.cancelable;
		});
		form.addEventListener('formdata', function(event) {
			local = event instanceof FormDataEvent && event.formData.get('n1') === 'v1';
			event.formData.append('h1', 'vh1');
		});
		var data = new FormData(form);
		var entries = Array.from(data);
		window.formDataEventWorks = local && bubbled && data.get('h1') === 'vh1' &&
			entries.length === 2 && entries[1][0] === 'h1' && entries[1][1] === 'vh1';
	`, "formdata-event-test"); err != nil {
		t.Fatal(err)
	}
	if got, ok := runtime.Global("formDataEventWorks"); !ok || !got.Bool {
		t.Fatalf("FormDataEvent/iterator behavior = %+v (present %v), want true", got, ok)
	}
}
