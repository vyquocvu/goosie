package js_test

import (
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
	"testing"
)

func TestEventConstructorNames(t *testing.T) {
	doc := domtest.Parse(`<!DOCTYPE html><html><head><title>t</title></head><body></body></html>`)
	rt := newRuntime(t, js.Options{DOM: doc})

	script := `
		(function() {
			const ctors = ['Event', 'MouseEvent', 'PointerEvent', 'UIEvent', 'WheelEvent', 'InputEvent', 'TouchEvent', 'Touch', 'TouchList', 'CustomEvent', 'KeyboardEvent', 'FocusEvent', 'ProgressEvent', 'ErrorEvent', 'SubmitEvent', 'ToggleEvent', 'FormDataEvent', 'MessageEvent', 'ClipboardEvent', 'CompositionEvent'];
			let failures = [];
			for (const name of ctors) {
			  const ctor = self[name];
			  if (!ctor) {
			    failures.push(name + ': not defined');
			    continue;
			  }
			  if (ctor.name !== name) {
			    failures.push(name + ': constructor.name is "' + ctor.name + '", expected "' + name + '"');
			  }
			}
			return failures.length === 0 ? 'PASS' : failures.join('; ');
		})()
	`

	result := evalString(t, rt, script)

	if result != "PASS" {
		t.Errorf("Constructor name check failed: %v", result)
	}
}
