package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestDocumentCurrentScriptNestedDocumentWriteRestoresOuter(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	err := r.Run(`
		document.open();
		document.write('<script id="outer" data-probe="outer">' +
			'window.outerBefore = document.currentScript.id;' +
			'window.outerIdentity = document.currentScript === document.getElementById("outer");' +
			'document.write(\'<script id="inner" data-probe="inner">window.innerCurrent = document.currentScript.id; window.innerIdentity = document.currentScript === document.getElementById("inner"); window.innerAttribute = document.currentScript.getAttribute("data-probe");</scr\' + \'ipt>\');' +
			'window.outerAfter = document.currentScript.id;' +
			'window.outerAttributeAfter = document.currentScript.getAttribute("data-probe");' +
			'</script>');
		document.close();
	`, "document-write-current-script")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"outerBefore":         "outer",
		"innerCurrent":        "inner",
		"innerAttribute":      "inner",
		"outerAfter":          "outer",
		"outerAttributeAfter": "outer",
	} {
		got, ok := r.Global(name)
		if !ok || got.Kind != js.KindString || got.Str != want {
			t.Errorf("%s = %+v, present %v; want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"outerIdentity", "innerIdentity"} {
		got, ok := r.Global(name)
		if !ok || got.Kind != js.KindBool || !got.Bool {
			t.Errorf("%s = %+v, present %v; want true", name, got, ok)
		}
	}
	got, err := r.Evaluate(`document.currentScript`)
	if err != nil || got != "null" {
		t.Fatalf("document.currentScript after nested execution = %q, %v; want null", got, err)
	}
}
