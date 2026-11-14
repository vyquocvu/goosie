package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

const doctypeFullScript = `
	const d = document.doctype;
	JSON.stringify({
		present: !!d,
		name: d ? d.name : null,
		publicId: d ? d.publicId : null,
		systemId: d ? d.systemId : null,
		ctor: d && d.constructor ? d.constructor.name : null
	});
`

// qc:BR-291
func TestDocumentDoctypeExposesNamePublicIdSystemId(t *testing.T) {
	rt := newDOMRuntime(t, domtest.Parse(`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd"><html><body>x</body></html>`))
	got, err := rt.Evaluate(doctypeFullScript)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	want := `{"present":true,"name":"HTML","publicId":"-//W3C//DTD HTML 4.01//EN","systemId":"http://www.w3.org/TR/html4/strict.dtd","ctor":"DocumentType"}`
	if got != want {
		t.Errorf("document.doctype = %s, want %s", got, want)
	}
}

// qc:BR-291
func TestDocumentFirstChildIsTheDoctypeNotTheHtmlElement(t *testing.T) {
	rt := newDOMRuntime(t, domtest.Parse(`<!DOCTYPE html><html><head><title>t</title></head><body>b</body></html>`))
	got, err := rt.Evaluate(`
		const ords = [];
		for (let i = 0; i < document.childNodes.length; i++) ords.push(document.childNodes[i].nodeName);
		JSON.stringify({ ords: ords, firstIsDoctype: document.firstChild === document.doctype });
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	want := `{"ords":["#doctype","HTML"],"firstIsDoctype":true}`
	if got != want {
		t.Errorf("document child order = %s, want %s", got, want)
	}
}

// qc:BR-291
func TestDocumentDoctypeIsNullWhenAbsent(t *testing.T) {
	rt := newDOMRuntime(t, domtest.Parse(`<html><body>x</body></html>`))
	got, err := rt.Evaluate(`JSON.stringify({ isNull: document.doctype === null })`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got != `{"isNull":true}` {
		t.Errorf("document.doctype absent = %s, want {\"isNull\":true}", got)
	}
}
