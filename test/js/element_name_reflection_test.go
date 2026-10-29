package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestNameReflectionForEmbedFrameObjectAndTextarea(t *testing.T) {
	doc := domtest.Parse(`<html><body><embed id="embed" name="initial"><object id="object" name="initial"></object><textarea id="textarea" name="initial"></textarea></body></html>`)
	rt := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	got := evalString(t, rt, `(function(){
		var elements = [document.getElementById('embed'), document.createElement('frame'),
			document.getElementById('object'), document.getElementById('textarea')];
		elements[1].setAttribute('name', 'initial');
		return elements.every(function(element) {
			if (element.name !== 'initial') return false;
			element.name = 'updated';
			if (element.getAttribute('name') !== 'updated') return false;
			element.setAttribute('name', 'attribute');
			return element.name === 'attribute';
		});
	})()`)
	if got != "true" {
		t.Fatalf("name reflected property/attribute round trip = %q, want true", got)
	}
}
