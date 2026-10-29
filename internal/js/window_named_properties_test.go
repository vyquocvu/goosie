package js

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestWindowNamedPropertyResolvesElementID(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	element := doc.NewElement("input")
	element.SetAttribute("id", "first")
	body.AppendChild(element)
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`window.namedElementWorks = first === document.getElementById('first')`, "window-named-property-test"); err != nil {
		t.Fatal(err)
	}
	if got, ok := runtime.Global("namedElementWorks"); !ok || !got.Bool {
		t.Fatalf("namedElementWorks = %+v, want true", got)
	}
}

func TestParserInsertedIDInstallsWindowNamedProperty(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	element := doc.NewElement("input")
	element.SetAttribute("id", "first")
	runtime.UpgradeParserElement(element)
	body.AppendChild(element)
	if err := runtime.Run(`window.namedElementWorks = first === document.getElementById('first')`, "parser-window-named-property-test"); err != nil {
		t.Fatal(err)
	}
	if got, ok := runtime.Global("namedElementWorks"); !ok || !got.Bool {
		t.Fatalf("namedElementWorks = %+v, want true", got)
	}
}

func TestSetDOMInstallsParsedIframeNameProperty(t *testing.T) {
	runtime, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	frame := doc.NewElement("iframe")
	frame.SetAttribute("name", "nodeFilterGlobalObject")
	body.AppendChild(frame)
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime.SetDOM(doc)

	if err := runtime.Run(`window.namedFramePropertyWorks = nodeFilterGlobalObject === document.body.firstChild.contentWindow`, "setdom-window-named-iframe-test"); err != nil {
		t.Fatal(err)
	}
	if got, ok := runtime.Global("namedFramePropertyWorks"); !ok || !got.Bool {
		t.Fatalf("namedFramePropertyWorks = %+v, want true", got)
	}
}

func TestInnerHTMLInsertedIDInstallsLiveWindowNamedProperty(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`document.body.innerHTML='<input id="later">'; window.namedElementWorks = later === document.getElementById('later')`, "innerhtml-window-named-property-test"); err != nil {
		t.Fatal(err)
	}
	if got, ok := runtime.Global("namedElementWorks"); !ok || !got.Bool {
		t.Fatalf("namedElementWorks = %+v, want true", got)
	}
}
