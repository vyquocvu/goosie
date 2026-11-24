package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestDocumentNamedFormPropertiesAreLive(t *testing.T) {
	doc := domtest.Parse(`<html><body>
<form id="form-id" name="f"><input name="q"></form>
<form name="dupe"></form><form name="dupe"></form>
</body></html>`)
	rt := newDOMRuntime(t, doc)
	if err := rt.Run(`
		window.namedByForm = document.f === document.forms.f;
		window.namedByID = document["form-id"] === document.f;
		window.namedControl = document.f.q.name === "q";
		window.namedCollectionLength = document.dupe.length;
		window.namedCollectionIdentity = document.dupe[0] === document.forms[1];
		window.documentNamedFormResult = window.namedByForm && window.namedByID && window.namedControl &&
			window.namedCollectionLength === 2 && window.namedCollectionIdentity;
		var added = document.createElement("form");
		added.name = "added";
		document.body.appendChild(added);
		window.documentNamedFormLive = document.added === added;
		added.remove();
		window.documentNamedFormLive = window.documentNamedFormLive && document.added === undefined;
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, ok := rt.Global("documentNamedFormResult"); !ok || !got.Bool {
		t.Fatalf("document named form properties = %+v, want true", got)
	}
	if got, ok := rt.Global("documentNamedFormLive"); !ok || !got.Bool {
		t.Fatalf("live document named form property = %+v, want true", got)
	}
}
