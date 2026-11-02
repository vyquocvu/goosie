package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// TestAttachShadowValidatesHostAndOptions is the BR-118 oracle for the parts of
// the Shadow DOM surface this engine does not implement: an invalid call must
// fail with the standard exception, leave host.shadowRoot null, and leave the
// element usable instead of creating a half-attached root.
//
// qc:BR-118
func TestAttachShadowValidatesHostAndOptions(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body><div id="host"></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.test/", DOM: doc})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer rt.Close()

	if err := rt.Run(`
		var host = document.getElementById("host");
		function failureName(fn) {
			try { fn(); } catch (error) { return error && error.name; }
			return "no-throw";
		}
		globalThis.noOptions = failureName(function() { host.attachShadow(); });
		globalThis.noMode = failureName(function() { document.createElement("section").attachShadow({}); });
		globalThis.badMode = failureName(function() { document.createElement("section").attachShadow({mode:"shadow"}); });
		globalThis.badSlot = failureName(function() { document.createElement("section").attachShadow({mode:"open", slotAssignment:"auto"}); });
		globalThis.inputHost = failureName(function() { document.createElement("input").attachShadow({mode:"open"}); });
		globalThis.templateHost = failureName(function() { document.createElement("template").attachShadow({mode:"open"}); });
		globalThis.second = failureName(function() {
			var first = document.createElement("article");
			first.attachShadow({mode:"open"});
			first.attachShadow({mode:"open"});
		});
		globalThis.allowed = ["div","section","span","article","p","h1","x-widget"].every(function(name) {
			var element = document.createElement(name);
			try { element.attachShadow({mode:"closed"}); } catch (error) { return false; }
			return element.shadowRoot === null;
		});
		var input = document.createElement("input");
		failureName(function() { input.attachShadow({mode:"open"}); });
		input.value = "still-usable";
		document.body.appendChild(input);
		var appended = document.body.lastChild === input && input.value === "still-usable";
		var hostUsable = host.shadowRoot === null && host.appendChild(document.createElement("b")) !== null;
		var retried = host.attachShadow({mode:"open"}) !== null && host.shadowRoot.mode === "open";
	`, "attach-shadow-validation.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for name, want := range map[string]string{
		"noOptions":    "TypeError",
		"noMode":       "TypeError",
		"badMode":      "TypeError",
		"badSlot":      "TypeError",
		"inputHost":    "NotSupportedError",
		"templateHost": "NotSupportedError",
		"second":       "NotSupportedError",
	} {
		value, ok := rt.Global(name)
		if !ok || value.Str != want {
			t.Errorf("%s = %v (present %t), want %s", name, value, ok, want)
		}
	}
	for _, name := range []string{"allowed", "appended", "hostUsable", "retried"} {
		value, ok := rt.Global(name)
		if !ok || !value.Bool {
			t.Errorf("%s = %v (present %t), want true", name, value, ok)
		}
	}
}
