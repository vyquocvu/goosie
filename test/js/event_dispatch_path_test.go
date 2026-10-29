package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestEventPropagationPathSnapshottedBeforeWindowCapture(t *testing.T) {
	doc := domtest.Parse(`<html><body><table><tbody id="body"><tr id="original"><td id="target">text</td></tr><tr id="destination"></tr></tbody></table></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.test", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()

	got, err := rt.Evaluate(`(function() {
		var target = document.getElementById('target');
		var parent = document.getElementById('original');
		var destination = document.getElementById('destination');
		var targets = [window, document, document.documentElement, document.body,
			document.querySelector('table'), document.getElementById('body'), parent, target];
		var names = ['window', 'document', 'html', 'body', 'table', 'tbody', 'parent', 'target'];
		var actual = [];
		function listener(event) {
			if (event.currentTarget === window && target.parentNode === parent) {
				destination.appendChild(parent.removeChild(target));
			}
			actual.push(names[targets.indexOf(event.currentTarget)] + ':' + event.eventPhase);
		}
		targets.forEach(function(node) {
			node.addEventListener('move', listener, true);
			node.addEventListener('move', listener, false);
		});
		target.dispatchEvent(new Event('move', {bubbles: true}));
		return JSON.stringify(actual);
	})()`)
	if err != nil {
		t.Fatalf("dispatch event: %v", err)
	}
	const want = `[` +
		`"window:1","document:1","html:1","body:1","table:1","tbody:1","parent:1",` +
		`"target:2","target:2","parent:3","tbody:3","table:3","body:3","html:3","document:3","window:3"]`
	if got != want {
		t.Fatalf("event path after moving target during window capture = %s, want %s", got, want)
	}
}
