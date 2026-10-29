package js

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestParentNodeAppendRejectsInvalidDocumentSequenceAtomically(t *testing.T) {
	runtime, err := New(Options{DOM: dom.NewDocument()})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	result, err := runtime.vm.RunString(`
		const first = document.createElement("first");
		document.append(first);
		let threw = false;
		try { document.prepend(document.createElement("second")); }
		catch (error) { threw = error.name === "HierarchyRequestError"; }
		threw && document.childNodes.length === 1 && document.documentElement === first;
	`)
	if err != nil {
		t.Fatalf("run hierarchy regression: %v", err)
	}
	if !result.ToBoolean() {
		t.Fatal("invalid prepend did not throw HierarchyRequestError and preserve the Document")
	}
}
