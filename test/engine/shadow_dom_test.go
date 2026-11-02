package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestShadowDOMAndCustomElementLifecycleStayScoped is the BR-118 engine oracle:
// a declarative shadow root must render its own tree plus slotted light
// children, the custom element callbacks around it must run once each, and
// none of that may disturb the sibling document tree.
//
// qc:BR-118
func TestShadowDOMAndCustomElementLifecycleStayScoped(t *testing.T) {
	html := `<html><head><style>
		body { margin: 0; font: 16px sans-serif; }
	</style></head><body>
		<my-card id="host">
			<template shadowrootmode="open"><span id="shadow-label">Shadow Label</span><slot></slot></template>
			<span id="light-child">Light Child</span>
		</my-card>
		<p id="outside">Outside Text</p>
		<script>
			globalThis.lifecycle = "";
			class TrackedElement extends HTMLElement {
				static get observedAttributes() { return ["data-state"]; }
				connectedCallback() { globalThis.lifecycle += "connected;"; }
				disconnectedCallback() { globalThis.lifecycle += "disconnected;"; }
				attributeChangedCallback(name, oldValue, newValue) {
					globalThis.lifecycle += "attr:" + name + "=" + newValue + ";";
				}
			}
			customElements.define("x-tracked", TrackedElement);
			var tracked = document.createElement("x-tracked");
			tracked.id = "tracked";
			document.body.appendChild(tracked);
			tracked.setAttribute("data-state", "on");
		</script>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(observersRuntime(t)), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)

	painted := paintedText(sess.Paint(1))
	for _, want := range []string{"Shadow Label", "Light Child", "Outside Text"} {
		if !strings.Contains(painted, want) {
			t.Fatalf("painted text %q is missing %q", painted, want)
		}
	}
	if strings.Contains(painted, "Shadow Label") && strings.Count(painted, "Shadow Label") != 1 {
		t.Fatalf("shadow content painted %d times in %q", strings.Count(painted, "Shadow Label"), painted)
	}

	got, err := sess.EvaluateJS(`(function(){
		var host = document.getElementById("host");
		return [
			globalThis.lifecycle,
			document.querySelector("#shadow-label") === null,
			host.shadowRoot.querySelector("#shadow-label") !== null,
			document.getElementById("outside").textContent,
			document.getElementById("light-child") !== null
		].join("|");
	})()`)
	if err != nil {
		t.Fatalf("EvaluateJS: %v", err)
	}
	const want = "connected;attr:data-state=on;|true|true|Outside Text|true"
	if got != want {
		t.Fatalf("lifecycle/isolation = %q, want %q", got, want)
	}

	if _, err := sess.EvaluateJS(`document.getElementById("tracked").remove()`); err != nil {
		t.Fatalf("remove tracked element: %v", err)
	}
	if got, err := sess.EvaluateJS(`globalThis.lifecycle`); err != nil || got != "connected;attr:data-state=on;disconnected;" {
		t.Fatalf("lifecycle after removal = %q (err %v), want one disconnect", got, err)
	}
}
