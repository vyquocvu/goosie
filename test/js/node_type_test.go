package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// qc:BR-188
func TestNodeTypeUsesDOMStandardOrdinals(t *testing.T) {
	doc := domtest.Parse(`<html><head><title>t</title></head><body><p id="p">hi</p></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `
	var out = [];
	out.push(document.nodeType);
	out.push(document.documentElement.nodeType);
	out.push(document.getElementById("p").nodeType);
	out.push(document.createElement("div").nodeType);
	out.push(document.createTextNode("t").nodeType);
	out.push(String(9 === document.nodeType));
	// The two values a passthrough of internal/dom's own numbering would
	// produce. Pinned because they are the numbers that break a library, not
	// because anyone would type them.
	out.push(String(document.nodeType === 0));
	out.push(String(document.createTextNode("t").nodeType === 2));`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(`var joined = out.slice(0, 5).join(","); var strict = out[5]+"/"+out[6]+"/"+out[7];`, "inline"); err != nil {
		t.Fatal(err)
	}
	joined, ok := r.Global("joined")
	if !ok || joined.Str != "9,1,1,1,3" {
		t.Fatalf("nodeType ordinals = %q, want %q", joined.Str, "9,1,1,1,3")
	}
	strict, ok := r.Global("strict")
	if !ok || strict.Str != "true/false/false" {
		t.Fatalf("internal-ordinal leak check = %q, want %q (9===document.nodeType true; no passthrough of 0 or 2)", strict.Str, "true/false/false")
	}
}

// qc:BR-188
func TestLibrarySetupGatePassesTheDocument(t *testing.T) {
	doc := domtest.Parse(`<html><head></head><body><div id="gate"></div></body></html>`)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	src := `
	var lib = (function () {
		var doc = null;
		function setDocument(node) {
			if (node == null || node.nodeType !== 9 || !node.documentElement) return;
			doc = node;
		}
		setDocument(document);
		return {
			configured: function () { return doc !== null; },
			root: function () { return doc.documentElement; }
		};
	})();
	var reached = 0;
	if (lib.configured()) {
		// Create through the Document interface; Node and Element do not expose
		// Document-only factories such as createElement().
		var root = lib.root();
		if (!root || root.nodeType !== 1) throw new Error("documentElement unavailable");
		document.createElement("fieldset");
		reached = 1;
	}
	var shown = 0;
	if (reached) {
		var gate = document.getElementById("gate");
		gate.style.display = "block";
		shown = gate.style.display === "block" ? 1 : 0;
	}`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatalf("library gate script threw: %v", err)
	}
	for name, want := range map[string]float64{"reached": 1, "shown": 1} {
		v, ok := r.Global(name)
		if !ok {
			t.Fatalf("%s was never assigned: the gate stopped the script", name)
		}
		if v.Num != want {
			t.Fatalf("%s = %v, want %v", name, v.Num, want)
		}
	}
}
