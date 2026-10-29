package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// nodeType carries the DOM Standard's ordinals, not internal/dom's.
//
// internal/dom numbers its kinds from 0 with the document first
// (NodeDocument=0, NodeElement=1, NodeText=2, NodeComment=3, NodeDoctype=4,
// NodeDocumentFragment=5); the standard says 9, 1, 3, 8, 10, 11. A page cannot
// see the internal kind - it only ever sees this number - so passing the field
// through would give `document.nodeType === 0`, which is what killed a whole
// family of real pages: jQuery 3.6's Sizzle setup gate is
// `doc.nodeType !== 9 || !doc.documentElement` and returns on the first
// comparison, leaving the library un-configured. On www.gov.vn that made the
// reveal of `<form id="form1" style="display:none">` (the entire body, 11,859
// visible characters) never happen, and Goosie painted 0 ink pixels where
// Chromium painted 501,735 - with rc 0 and not one line on stderr.
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
	// `out` is an array; read it back through a second script instead of
	// forcing the host to marshal one.
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

// jQuery 3.6's setup gate is the consumer this property exists for. The script
// below is the gate and its consequence, written out: pass the gate and the
// hidden block is revealed, fail it and nothing after the gate runs. This is
// the shape measured on www.gov.vn, reduced to what can be checked offline.
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
